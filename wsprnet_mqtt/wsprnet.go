package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// WSPRNet constants
const (
	WSPRServerHostname  = "wsprnet.org"
	WSPRServerPort      = 80
	WSPRMaxQueueSize    = 10000
	WSPRMaxRetries      = 3
	WSPRWorkerThreads   = 1   // Reduced to 1 for MEPT bulk uploads
	WSPRTimeoutSeconds  = 30  // Increased from 3 to handle slow WSPRnet responses
	WSPRMaxBatchSize    = 999 // Maximum spots per MEPT upload
	WSPRBatchWaitMillis = 500 // Wait time to accumulate spots for batching
)

// WSPR mode codes from http://www.wsprnet.org/drupal/node/8983
const (
	WSPRModeWSPR      = 2
	WSPRModeFST4W120  = 3
	WSPRModeFST4W300  = 5
	WSPRModeFST4W900  = 16
	WSPRModeFST4W1800 = 30
)

// WSPRReport represents a single WSPR spot report
type WSPRReport struct {
	Callsign      string
	Locator       string
	SNR           int
	Frequency     uint64 // Transmitter frequency in Hz
	ReceiverFreq  uint64 // Receiver frequency in Hz
	DT            float32
	Drift         int
	DBm           int
	EpochTime     time.Time
	Mode          string
	RetryCount    int
	NextRetryTime time.Time
}

// WSPRBatch represents a batch of reports to be uploaded together
type WSPRBatch struct {
	Reports       []WSPRReport
	RetryCount    int
	NextRetryTime time.Time
}

// WSPRNet handles WSPRNet spot reporting using MEPT bulk upload
type WSPRNet struct {
	// Configuration
	receiverCallsign string
	receiverLocator  string
	programName      string
	programVersion   string
	dryRun           bool

	// Report queues - now batched
	reportQueue []WSPRReport
	queueMutex  sync.Mutex

	retryQueue []WSPRBatch
	retryMutex sync.Mutex

	// Statistics
	countSendsOK      int
	countSendsErrored int
	countRetries      int
	statsMutex        sync.Mutex

	// Threading
	running bool
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

// NewWSPRNet creates a new WSPRNet instance
func NewWSPRNet(callsign, locator, programName, programVersion string, dryRun bool) (*WSPRNet, error) {
	if callsign == "" || locator == "" || programName == "" {
		return nil, fmt.Errorf("callsign, locator, and program name are required")
	}

	wspr := &WSPRNet{
		receiverCallsign: callsign,
		receiverLocator:  locator,
		programName:      programName,
		programVersion:   programVersion,
		dryRun:           dryRun,
		reportQueue:      make([]WSPRReport, 0, WSPRMaxQueueSize),
		retryQueue:       make([]WSPRBatch, 0, WSPRMaxQueueSize),
		stopCh:           make(chan struct{}),
	}

	return wspr, nil
}

// Connect starts the WSPRNet processing threads
func (w *WSPRNet) Connect() error {
	w.running = true

	// Start worker threads for parallel HTTP requests
	for i := 0; i < WSPRWorkerThreads; i++ {
		w.wg.Add(1)
		go w.workerThread()
	}

	log.Printf("WSPRNet: Started %d worker threads for parallel uploads", WSPRWorkerThreads)

	return nil
}

// Submit adds a WSPR report to the queue
func (w *WSPRNet) Submit(report *WSPRReport) error {
	if !w.running {
		return fmt.Errorf("WSPRNet not running")
	}

	// Only accept WSPR reports
	if report.Mode != "WSPR" {
		return nil
	}

	if report.Callsign == "" || report.Locator == "" {
		return nil
	}

	// Filter out hashed callsigns
	if report.Callsign == "<...>" {
		return nil
	}

	w.queueMutex.Lock()
	defer w.queueMutex.Unlock()

	if len(w.reportQueue) >= WSPRMaxQueueSize {
		return fmt.Errorf("WSPRNet queue full")
	}

	w.reportQueue = append(w.reportQueue, *report)

	return nil
}

// workerThread processes reports from queue using MEPT bulk upload
func (w *WSPRNet) workerThread() {
	defer w.wg.Done()

	for w.running {
		var batch WSPRBatch
		haveBatch := false

		// First check retry queue
		currentTime := time.Now()
		w.retryMutex.Lock()
		if len(w.retryQueue) > 0 && w.retryQueue[0].NextRetryTime.Before(currentTime) {
			batch = w.retryQueue[0]
			w.retryQueue = w.retryQueue[1:]
			haveBatch = true
		}
		w.retryMutex.Unlock()

		// If no retry batch, try to build a new batch from main queue
		if !haveBatch {
			w.queueMutex.Lock()
			if len(w.reportQueue) > 0 {
				// Wait briefly to accumulate more spots for batching
				w.queueMutex.Unlock()
				time.Sleep(WSPRBatchWaitMillis * time.Millisecond)
				w.queueMutex.Lock()

				// Collect up to WSPRMaxBatchSize reports
				batchSize := len(w.reportQueue)
				if batchSize > WSPRMaxBatchSize {
					batchSize = WSPRMaxBatchSize
				}

				if batchSize > 0 {
					batch.Reports = make([]WSPRReport, batchSize)
					copy(batch.Reports, w.reportQueue[:batchSize])
					w.reportQueue = w.reportQueue[batchSize:]
					haveBatch = true
				}
			}
			w.queueMutex.Unlock()
		}

		// If we have a batch, send it
		if haveBatch {
			wasRetry := batch.RetryCount > 0
			spotsAccepted, spotsOffered, success := w.sendBatch(&batch)

			w.statsMutex.Lock()
			if success {
				w.countSendsOK += spotsAccepted
				if spotsAccepted < spotsOffered {
					log.Printf("WSPRNet: Partial success - %d of %d spots accepted", spotsAccepted, spotsOffered)
				}
				if wasRetry {
					log.Printf("WSPRNet: Successfully sent batch of %d spots (after %d retry/retries)",
						spotsAccepted, batch.RetryCount)
				}
			} else {
				// Check if we should retry
				if batch.RetryCount < WSPRMaxRetries {
					// Increased retry delays to 2 minutes to avoid retrying within same WSPR window
					retryDelays := []int{120, 240, 360} // 2, 4, 6 minutes
					delayIndex := batch.RetryCount
					if delayIndex >= len(retryDelays) {
						delayIndex = len(retryDelays) - 1
					}
					delay := retryDelays[delayIndex]
					batch.RetryCount++
					batch.NextRetryTime = time.Now().Add(time.Duration(delay) * time.Second)

					w.retryMutex.Lock()
					if len(w.retryQueue) < WSPRMaxQueueSize {
						w.retryQueue = append(w.retryQueue, batch)
						w.countRetries++
					}
					w.retryMutex.Unlock()

					log.Printf("WSPRNet: Failed to send batch of %d spots, will retry in %d seconds (attempt %d/%d)",
						len(batch.Reports), delay, batch.RetryCount, WSPRMaxRetries)
				} else {
					w.countSendsErrored += len(batch.Reports)
					log.Printf("WSPRNet: Failed to send batch of %d spots after %d attempts, giving up",
						len(batch.Reports), WSPRMaxRetries)
				}
			}
			w.statsMutex.Unlock()
		} else {
			// No batches available, sleep briefly
			select {
			case <-time.After(100 * time.Millisecond):
			case <-w.stopCh:
				return
			}
		}
	}
}

// sendBatch sends a batch of reports to WSPRNet using MEPT bulk upload
// Returns (spotsAccepted, spotsOffered, success)
func (w *WSPRNet) sendBatch(batch *WSPRBatch) (int, int, bool) {
	spotsOffered := len(batch.Reports)
	startTime := time.Now()

	// If dry run mode, just return success (logging is done by aggregator)
	if w.dryRun {
		log.Printf("WSPRNet: [DRY RUN] Would upload batch of %d spots", spotsOffered)
		return spotsOffered, spotsOffered, true
	}

	log.Printf("WSPRNet: Starting MEPT upload of %d spots to %s/meptspots.php", spotsOffered, WSPRServerHostname)
	log.Printf("WSPRNet: Receiver: %s at %s", w.receiverCallsign, w.receiverLocator)

	// Build MEPT format data
	meptData := w.buildMEPTData(batch.Reports)

	// Log first few spots for debugging
	lines := strings.Split(meptData, "\n")
	if len(lines) > 0 {
		log.Printf("WSPRNet: First spot: %s", lines[0])
		if len(lines) > 1 {
			log.Printf("WSPRNet: Second spot: %s", lines[1])
		}
	}

	// Create multipart form data
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// Add version field
	versionStr := w.programName
	if w.programVersion != "" {
		versionStr = fmt.Sprintf("%s_%s", w.programName, w.programVersion)
	}
	if err := writer.WriteField("version", versionStr); err != nil {
		log.Printf("WSPRNet: Failed to write version field: %v", err)
		return 0, spotsOffered, false
	}

	// Add call field
	if err := writer.WriteField("call", w.receiverCallsign); err != nil {
		log.Printf("WSPRNet: Failed to write call field: %v", err)
		return 0, spotsOffered, false
	}

	// Add grid field (can be 4 or 6 characters)
	if err := writer.WriteField("grid", w.receiverLocator); err != nil {
		log.Printf("WSPRNet: Failed to write grid field: %v", err)
		return 0, spotsOffered, false
	}

	// Add allmept field with spot data
	part, err := writer.CreateFormFile("allmept", "spots.txt")
	if err != nil {
		log.Printf("WSPRNet: Failed to create allmept field: %v", err)
		return 0, spotsOffered, false
	}
	if _, err := part.Write([]byte(meptData)); err != nil {
		log.Printf("WSPRNet: Failed to write allmept data: %v", err)
		return 0, spotsOffered, false
	}

	if err := writer.Close(); err != nil {
		log.Printf("WSPRNet: Failed to close multipart writer: %v", err)
		return 0, spotsOffered, false
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: WSPRTimeoutSeconds * time.Second,
	}

	// Build request to MEPT endpoint
	req, err := http.NewRequest("POST", fmt.Sprintf("http://%s/meptspots.php", WSPRServerHostname), &requestBody)
	if err != nil {
		log.Printf("WSPRNet: Failed to create request: %v", err)
		return 0, spotsOffered, false
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Connection", "Keep-Alive")
	req.Header.Set("Host", WSPRServerHostname)

	// Send request and measure time
	resp, err := client.Do(req)
	elapsed := time.Since(startTime)

	if err != nil {
		log.Printf("WSPRNet: Failed to send request after %.2f seconds: %v", elapsed.Seconds(), err)
		return 0, spotsOffered, false
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Error closing response body: %v", err)
		}
	}()

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("WSPRNet: Failed to read response body after %.2f seconds: %v", elapsed.Seconds(), err)
		return 0, spotsOffered, false
	}
	bodyStr := string(bodyBytes)

	// Check for upload limit reached
	if strings.Contains(bodyStr, "Upload limit") && strings.Contains(bodyStr, "reached") {
		log.Printf("WSPRNet: SUCCESS - Upload limit reached after %.2f seconds, treating as success to avoid retrying", elapsed.Seconds())
		return spotsOffered, spotsOffered, true
	}

	// Parse response for "X spot(s) added" or "X out of Y spot(s) added"
	// wsprdaemon checks for this pattern at line 309-310
	re := regexp.MustCompile(`(\d+)\s+(?:out of|spot.*added.*out of)\s+(\d+)`)
	matches := re.FindStringSubmatch(bodyStr)

	if len(matches) == 3 {
		spotsAccepted, err1 := strconv.Atoi(matches[1])
		spotsInResponse, err2 := strconv.Atoi(matches[2])

		if err1 == nil && err2 == nil {
			if spotsInResponse != spotsOffered {
				log.Printf("WSPRNet: Warning - response mentions %d spots but we offered %d", spotsInResponse, spotsOffered)
			}
			if spotsAccepted == 0 {
				log.Printf("WSPRNet: FAILED - 0 of %d spots accepted in %.2f seconds. Response: %s", spotsOffered, elapsed.Seconds(), bodyStr)
				return 0, spotsOffered, false
			}
			log.Printf("WSPRNet: SUCCESS - Uploaded %d of %d spots in %.2f seconds", spotsAccepted, spotsOffered, elapsed.Seconds())
			return spotsAccepted, spotsOffered, true
		}
	}

	// If we got a 200 response but couldn't parse the spot count, this is likely an error
	// The response should always include "X out of Y spot(s) added" if successful
	if resp.StatusCode == 200 {
		// Check if response indicates no spots were processed (just "Processing took X milliseconds")
		if strings.Contains(bodyStr, "Processing took") && !strings.Contains(bodyStr, "spot") {
			log.Printf("WSPRNet: FAILED - Server processed request but added no spots in %.2f seconds. Response: %s", elapsed.Seconds(), bodyStr)
			return 0, spotsOffered, false
		}
		log.Printf("WSPRNet: WARNING - Got 200 response in %.2f seconds but couldn't parse spot count. Response: %s", elapsed.Seconds(), bodyStr)
		// Don't assume success - return failure to trigger retry
		return 0, spotsOffered, false
	}

	log.Printf("WSPRNet: FAILED - Unexpected response after %.2f seconds: %d %s, body: %s", elapsed.Seconds(), resp.StatusCode, resp.Status, bodyStr)
	return 0, spotsOffered, false
}

// buildMEPTData builds the MEPT format data for bulk upload
// Format: YYMMDD HHMM Sync SNR DT FREQ CALL GRID PWR Drift DecCycles Jitter BlocksCorrected AudioPeak Decode (14 fields)
// This matches the WSJT-X ALL_WSPR.TXT format
func (w *WSPRNet) buildMEPTData(reports []WSPRReport) string {
	var lines []string

	for _, report := range reports {
		tm := report.EpochTime.UTC()
		date := tm.Format("060102")
		timeStr := tm.Format("1504")

		// Frequency in MHz with 7 decimal places
		freqMHz := fmt.Sprintf("%.7f", float64(report.Frequency)/1000000.0)

		// Grid must be exactly 4 characters (truncate 6-char grids)
		grid := report.Locator
		if len(grid) > 4 {
			grid = grid[:4]
		}

		// Fix DT to avoid -0.0 (round very small negative values to 0.0)
		dt := report.DT
		if dt > -0.05 && dt < 0.0 {
			dt = 0.0
		}

		// WSJT-X ALL_WSPR.TXT format (14 fields):
		// Date Time Sync SNR DT Freq Call Grid Power Drift DecCycles Jitter BlocksCorrected AudioPeak Decode
		// Example: 170711 2234   1 -28  1.26  14.0970558  VE7XT CN88 20           0   190    0
		line := fmt.Sprintf("%s %s %3d %3d %5.2f %12s  %s %s %2d %11d %5d %4d",
			date,            // Date (YYMMDD)
			timeStr,         // Time (HHMM)
			1,               // Sync quality (placeholder)
			report.SNR,      // SNR in dB
			dt,              // DT (time offset in seconds)
			freqMHz,         // Frequency in MHz (7 decimals)
			report.Callsign, // Transmitter callsign
			grid,            // Transmitter grid (4 chars)
			report.DBm,      // Power in dBm
			report.Drift,    // Drift in Hz/minute
			0,               // DecCycles (placeholder)
			0)               // Jitter (placeholder)
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// Stop stops the WSPRNet processing
func (w *WSPRNet) Stop() {
	if !w.running {
		return
	}

	log.Println("WSPRNet: Stopping...")

	w.running = false
	close(w.stopCh)

	// Wait for all worker threads to finish
	w.wg.Wait()

	// Print statistics
	w.statsMutex.Lock()
	log.Printf("WSPRNet: Successful reports: %d, Failed reports: %d, Retries: %d",
		w.countSendsOK, w.countSendsErrored, w.countRetries)
	w.statsMutex.Unlock()

	log.Println("WSPRNet: Stopped")
}

// GetStats returns current statistics
func (w *WSPRNet) GetStats() map[string]interface{} {
	w.statsMutex.Lock()
	defer w.statsMutex.Unlock()

	return map[string]interface{}{
		"successful": w.countSendsOK,
		"failed":     w.countSendsErrored,
		"retries":    w.countRetries,
	}
}

// SetStats restores statistics from persistence
func (w *WSPRNet) SetStats(successful, failed, retries int) {
	w.statsMutex.Lock()
	defer w.statsMutex.Unlock()

	w.countSendsOK = successful
	w.countSendsErrored = failed
	w.countRetries = retries
}

// ResetStats clears all statistics
func (w *WSPRNet) ResetStats() {
	w.statsMutex.Lock()
	defer w.statsMutex.Unlock()

	w.countSendsOK = 0
	w.countSendsErrored = 0
	w.countRetries = 0

	log.Println("WSPRNet: Statistics reset to zero")
}
