package main

import (
	"fmt"
	"log"
	"math/rand"
	"sort"
	"sync"
	"time"
)

// SpotAggregator aggregates and deduplicates WSPR spots within 2-minute windows
type SpotAggregator struct {
	wsprNet         *WSPRNet
	pskReporter     *PSKReporter
	stats           *StatisticsTracker
	persistenceFile string
	spotWriter      *SpotWriter

	// Map of 2-minute windows to spots
	// Key: timestamp rounded to 2-minute boundary
	// Value: map of dedup key to report with source info
	windows   map[int64]map[string]*WSPRReportWithSource
	windowsMu sync.Mutex

	// Track duplicates for reporting
	// Key: window timestamp
	// Value: map of callsign to list of duplicate reports
	duplicates   map[int64]map[string][]*WSPRReportWithSource
	duplicatesMu sync.Mutex

	// Channel for incoming spots
	spotChan chan *WSPRReportWithSource

	// Control
	running   bool
	startTime time.Time
	stopChan  chan struct{}
	wg        sync.WaitGroup
}

// WSPRReportWithSource wraps a WSPR report with its source information
type WSPRReportWithSource struct {
	*WSPRReport
	InstanceName string
	Country      string
}

// NewSpotAggregator creates a new spot aggregator
func NewSpotAggregator(wsprNet *WSPRNet, pskReporter *PSKReporter, stats *StatisticsTracker, persistenceFile string, spotWriter *SpotWriter) *SpotAggregator {
	return &SpotAggregator{
		wsprNet:         wsprNet,
		pskReporter:     pskReporter,
		stats:           stats,
		persistenceFile: persistenceFile,
		spotWriter:      spotWriter,
		windows:         make(map[int64]map[string]*WSPRReportWithSource),
		duplicates:      make(map[int64]map[string][]*WSPRReportWithSource),
		spotChan:        make(chan *WSPRReportWithSource, 1000),
		stopChan:        make(chan struct{}),
	}
}

// Start starts the aggregator
func (sa *SpotAggregator) Start() {
	sa.running = true
	sa.startTime = time.Now()

	// Start spot processing goroutine
	sa.wg.Add(1)
	go sa.processSpots()

	// Start window flushing goroutine
	sa.wg.Add(1)
	go sa.flushWindows()

	log.Println("Spot aggregator started")
}

// Stop stops the aggregator
func (sa *SpotAggregator) Stop() {
	if !sa.running {
		return
	}

	sa.running = false
	close(sa.stopChan)
	sa.wg.Wait()

	// Flush any remaining windows
	sa.flushAllWindows()

	log.Println("Spot aggregator stopped")
}

// AddSpot adds a spot to the aggregator
func (sa *SpotAggregator) AddSpot(report *WSPRReport, instanceName, country string) {
	if !sa.running {
		return
	}

	reportWithSource := &WSPRReportWithSource{
		WSPRReport:   report,
		InstanceName: instanceName,
		Country:      country,
	}

	select {
	case sa.spotChan <- reportWithSource:
	default:
		log.Println("Aggregator: Spot channel full, dropping spot")
	}
}

// processSpots processes incoming spots
func (sa *SpotAggregator) processSpots() {
	defer sa.wg.Done()

	for {
		select {
		case <-sa.stopChan:
			return
		case report := <-sa.spotChan:
			sa.addToWindow(report)
		}
	}
}

// addToWindow adds a report to the appropriate 2-minute window
func (sa *SpotAggregator) addToWindow(report *WSPRReportWithSource) {
	// Check message age to filter out retained messages
	messageAge := time.Since(report.EpochTime)
	if messageAge > 5*time.Minute {
		// Message is too old (> 5 minutes) - likely a retained message
		log.Printf("Aggregator: Rejecting old spot for %s (age: %.1f minutes)", report.Callsign, messageAge.Minutes())
		return
	}

	// Determine band for statistics and deduplication
	band := frequencyToBand(report.ReceiverFreq)

	// Round timestamp to 2-minute boundary (WSPR cycle time)
	// WSPR transmissions start at even minutes (00, 02, 04, etc.)
	timestamp := report.EpochTime.Unix()
	windowKey := (timestamp / 120) * 120

	// Create deduplication key: callsign + mode + window + band
	// This ensures we only keep one spot per callsign per 2-minute window per band
	// Using band instead of exact frequency handles slight frequency variations
	dedupKey := fmt.Sprintf("%s_%s_%d_%s", report.Callsign, report.Mode, windowKey, band)

	// Write raw spot to file
	if sa.spotWriter != nil {
		if err := sa.spotWriter.WriteRaw(report); err != nil {
			log.Printf("Warning: Failed to write raw spot: %v", err)
		}
	}

	// Record spot in statistics
	sa.stats.RecordSpot(report.InstanceName, band, report.Callsign, report.Country, report.Locator, report.SNR)

	sa.windowsMu.Lock()
	defer sa.windowsMu.Unlock()

	// Create window if it doesn't exist
	if sa.windows[windowKey] == nil {
		sa.windows[windowKey] = make(map[string]*WSPRReportWithSource)
	}

	// Check if we already have this spot
	if existing, exists := sa.windows[windowKey][dedupKey]; exists {
		// Keep the spot with better SNR
		if report.SNR > existing.SNR {
			// New report is better - track the old one as rejected
			sa.trackDuplicate(windowKey, existing)
			sa.windows[windowKey][dedupKey] = report
			// Record that this instance won
			sa.stats.RecordBestSNR(report.InstanceName, band)
			// Record duplicate relationship (both directions)
			sa.stats.RecordDuplicate(report.InstanceName, band, existing.InstanceName)
			sa.stats.RecordDuplicate(existing.InstanceName, band, report.InstanceName)
			if DebugMode {
				log.Printf("Aggregator: Updated spot for %s (better SNR: %d > %d)",
					report.Callsign, report.SNR, existing.SNR)
			}
		} else if report.SNR == existing.SNR {
			// Tied SNR - track both instances as having tied with each other
			sa.trackDuplicate(windowKey, report)
			sa.stats.RecordTiedSNR(report.InstanceName, band, existing.InstanceName)
			sa.stats.RecordTiedSNR(existing.InstanceName, band, report.InstanceName)
			// Also record as general duplicate relationship
			sa.stats.RecordDuplicate(report.InstanceName, band, existing.InstanceName)
			sa.stats.RecordDuplicate(existing.InstanceName, band, report.InstanceName)
			if DebugMode {
				log.Printf("Aggregator: Tied spot for %s (SNR: %d = %d) - [%s] vs [%s]",
					report.Callsign, report.SNR, existing.SNR, existing.InstanceName, report.InstanceName)
			}
		} else {
			// Existing is better - track the new one as rejected
			sa.trackDuplicate(windowKey, report)
			sa.stats.RecordBestSNR(existing.InstanceName, band)
			// Record duplicate relationship (both directions)
			sa.stats.RecordDuplicate(report.InstanceName, band, existing.InstanceName)
			sa.stats.RecordDuplicate(existing.InstanceName, band, report.InstanceName)
			if DebugMode {
				log.Printf("Aggregator: Duplicate spot for %s (keeping existing SNR: %d > %d)",
					report.Callsign, existing.SNR, report.SNR)
			}
		}
	} else {
		// New spot for this window
		sa.windows[windowKey][dedupKey] = report
		if DebugMode {
			log.Printf("Aggregator: Added spot for %s to window %d",
				report.Callsign, windowKey)
		}
	}
}

// flushWindows periodically flushes old windows
// Synchronized to run at WSPR cycle boundaries (every 2 minutes at :00, :02, :04, etc.)
// with a random 3-20 second offset to spread load on wsprnet.org
func (sa *SpotAggregator) flushWindows() {
	defer sa.wg.Done()

	// Seed random number generator
	rand.Seed(time.Now().UnixNano())

	// Generate random offset between 3-20 seconds
	randomOffset := 3 + rand.Intn(18) // 3 + [0-17] = 3-20 seconds

	// Calculate time until next 2-minute boundary
	now := time.Now()
	secondsIntoMinute := now.Second()
	minuteMod := now.Minute() % 2

	// Calculate seconds until next even 2-minute mark
	var secondsUntilNext int
	if minuteMod == 0 {
		// We're in an even minute (0, 2, 4, etc.)
		if secondsIntoMinute == 0 {
			secondsUntilNext = 120 // Next cycle is 2 minutes away
		} else {
			secondsUntilNext = 120 - secondsIntoMinute // Wait until next even 2-minute mark
		}
	} else {
		// We're in an odd minute (1, 3, 5, etc.)
		secondsUntilNext = 60 - secondsIntoMinute // Wait until next even minute
	}

	// Add random offset
	secondsUntilNext += randomOffset

	log.Printf("Aggregator: Synchronizing to WSPR cycles with %d second offset, next flush in %d seconds", randomOffset, secondsUntilNext)

	// Wait until the next 2-minute boundary + offset
	time.Sleep(time.Duration(secondsUntilNext) * time.Second)

	// Now create a ticker that fires every 2 minutes (120 seconds)
	ticker := time.NewTicker(120 * time.Second)
	defer ticker.Stop()

	// Flush immediately at the first boundary
	sa.flushOldWindows()

	for {
		select {
		case <-sa.stopChan:
			return
		case <-ticker.C:
			sa.flushOldWindows()
		}
	}
}

// flushOldWindows flushes windows that are older than 60 seconds
// This gives time for all instances to report their spots for a given window
// Retained messages are filtered by age check in addToWindow()
func (sa *SpotAggregator) flushOldWindows() {
	now := time.Now().Unix()
	// Flush windows that are 2-4 minutes old (120-240 seconds)
	// This ensures we only flush one window per cycle
	flushThreshold := now - 120  // 2 minutes ago
	tooOldThreshold := now - 240 // 4 minutes ago

	sa.windowsMu.Lock()

	windowsToFlush := make(map[int64]map[string]*WSPRReportWithSource)
	for windowKey, spots := range sa.windows {
		age := now - windowKey
		// Flush windows that are 2-4 minutes old
		if windowKey < flushThreshold && windowKey >= tooOldThreshold {
			windowsToFlush[windowKey] = spots
			delete(sa.windows, windowKey)
		} else if windowKey < tooOldThreshold {
			// Window is too old (>4 minutes), flush it anyway but log warning
			log.Printf("WARNING: Window %d is %d seconds old, flushing late", windowKey, age)
			windowsToFlush[windowKey] = spots
			delete(sa.windows, windowKey)
		}
	}
	sa.windowsMu.Unlock()

	// Flush each window
	for windowKey, spots := range windowsToFlush {
		sa.flushWindow(windowKey, spots)
	}
}

// flushAllWindows flushes all remaining windows (called on shutdown)
func (sa *SpotAggregator) flushAllWindows() {
	sa.windowsMu.Lock()
	windowsToFlush := make(map[int64]map[string]*WSPRReportWithSource)
	for windowKey, spots := range sa.windows {
		windowsToFlush[windowKey] = spots
	}
	sa.windows = make(map[int64]map[string]*WSPRReportWithSource)
	sa.windowsMu.Unlock()

	for windowKey, spots := range windowsToFlush {
		log.Printf("Aggregator: Flushing remaining window %d with %d unique spots",
			windowKey, len(spots))
		sa.flushWindow(windowKey, spots)
	}
}

// flushWindow flushes a single window with detailed reporting
func (sa *SpotAggregator) flushWindow(windowKey int64, spots map[string]*WSPRReportWithSource) {
	if len(spots) == 0 {
		return
	}

	// Get window time
	windowTime := time.Unix(windowKey, 0).UTC()

	// Start statistics window
	sa.stats.StartWindow(windowTime)

	// Group spots by band
	bandSpots := make(map[string][]*WSPRReportWithSource)
	bandBreakdown := make(map[string]int)
	for _, report := range spots {
		// Determine band from frequency
		band := frequencyToBand(report.ReceiverFreq)
		bandSpots[band] = append(bandSpots[band], report)
		bandBreakdown[band]++
	}

	// Get duplicates for this window
	sa.duplicatesMu.Lock()
	windowDuplicates := sa.duplicates[windowKey]
	totalDuplicates := 0
	for _, dups := range windowDuplicates {
		totalDuplicates += len(dups)
	}
	delete(sa.duplicates, windowKey)
	sa.duplicatesMu.Unlock()

	// Track unique spots per instance
	instanceCallsigns := make(map[string]map[string]bool)
	for _, report := range spots {
		if instanceCallsigns[report.InstanceName] == nil {
			instanceCallsigns[report.InstanceName] = make(map[string]bool)
		}
		instanceCallsigns[report.InstanceName][report.Callsign] = true
	}

	// Find unique callsigns per instance
	allCallsigns := make(map[string]bool)
	for _, callsigns := range instanceCallsigns {
		for callsign := range callsigns {
			allCallsigns[callsign] = true
		}
	}

	for instance, callsigns := range instanceCallsigns {
		for callsign := range callsigns {
			// Check if this callsign is unique to this instance
			isUnique := true
			for otherInstance, otherCallsigns := range instanceCallsigns {
				if otherInstance != instance && otherCallsigns[callsign] {
					isUnique = false
					break
				}
			}
			if isUnique {
				band := ""
				for _, report := range spots {
					if report.Callsign == callsign && report.InstanceName == instance {
						band = frequencyToBand(report.ReceiverFreq)
						break
					}
				}
				sa.stats.RecordUnique(instance, band, callsign)
			}
		}
	}

	// Group duplicates by band
	bandDuplicates := make(map[string]map[string][]*WSPRReportWithSource)
	for callsign, dups := range windowDuplicates {
		if len(dups) > 0 {
			band := frequencyToBand(dups[0].ReceiverFreq)
			if bandDuplicates[band] == nil {
				bandDuplicates[band] = make(map[string][]*WSPRReportWithSource)
			}
			bandDuplicates[band][callsign] = dups
		}
	}

	// Sort bands for consistent output
	bands := make([]string, 0, len(bandSpots))
	for band := range bandSpots {
		bands = append(bands, band)
	}
	sort.Strings(bands)

	// Submit all spots to WSPRNet and PSKReporter
	log.Printf("WSPR Window %s: Submitting %d unique spots to WSPRNet", windowTime.Format("15:04 UTC"), len(spots))

	for _, band := range bands {
		reports := bandSpots[band]

		// Sort reports by callsign
		sort.Slice(reports, func(i, j int) bool {
			return reports[i].Callsign < reports[j].Callsign
		})

		for _, report := range reports {
			// Submit to WSPRNet
			err := sa.wsprNet.Submit(report.WSPRReport)
			submitted := (err == nil)
			errorMsg := ""
			if err != nil {
				errorMsg = err.Error()
				log.Printf("ERROR: Failed to queue %s for WSPRNet: %v", report.Callsign, err)
			}

			// Submit to PSKReporter if enabled
			if sa.pskReporter != nil {
				if pskErr := sa.pskReporter.Submit(report.WSPRReport); pskErr != nil {
					log.Printf("ERROR: Failed to queue %s for PSKReporter: %v", report.Callsign, pskErr)
				}
			}

			// Write deduped spot with submission status
			if sa.spotWriter != nil {
				if writeErr := sa.spotWriter.WriteDeduped(report, submitted, errorMsg); writeErr != nil {
					log.Printf("Warning: Failed to write deduped spot for %s: %v", report.Callsign, writeErr)
				}
			}
		}
	}

	// Finish statistics window (pass 0 for failed count since failures are tracked separately by WSPRNet)
	sa.stats.FinishWindow(len(spots), totalDuplicates, 0, bandBreakdown)

	// Save statistics to disk if persistence is enabled
	if sa.persistenceFile != "" {
		// Get WSPRNet and PSKReporter stats and save them
		wsprnetStats := sa.wsprNet.GetStats()
		var pskReporterStats map[string]interface{}
		if sa.pskReporter != nil {
			pskReporterStats = sa.pskReporter.GetStats()
		}
		if err := sa.stats.SaveToFileWithReporters(sa.persistenceFile, wsprnetStats, pskReporterStats); err != nil {
			log.Printf("Warning: Failed to save statistics: %v", err)
		}
	}
}

// trackDuplicate tracks a rejected duplicate report for later reporting
func (sa *SpotAggregator) trackDuplicate(windowKey int64, rejected *WSPRReportWithSource) {
	sa.duplicatesMu.Lock()
	defer sa.duplicatesMu.Unlock()

	if sa.duplicates[windowKey] == nil {
		sa.duplicates[windowKey] = make(map[string][]*WSPRReportWithSource)
	}

	// Store only the rejected report
	sa.duplicates[windowKey][rejected.Callsign] = append(
		sa.duplicates[windowKey][rejected.Callsign],
		rejected,
	)
}

// frequencyToBand converts a frequency in Hz to a band name
func frequencyToBand(freq uint64) string {
	freqMHz := float64(freq) / 1000000.0

	switch {
	case freqMHz >= 0.1357 && freqMHz < 0.1378:
		return "2200m"
	case freqMHz >= 0.472 && freqMHz < 0.479:
		return "630m"
	case freqMHz >= 1.8 && freqMHz < 2.0:
		return "160m"
	case freqMHz >= 3.5 && freqMHz < 4.0:
		return "80m"
	case freqMHz >= 5.25 && freqMHz < 5.45:
		return "60m"
	case freqMHz >= 7.0 && freqMHz < 7.3:
		return "40m"
	case freqMHz >= 10.1 && freqMHz < 10.15:
		return "30m"
	case freqMHz >= 14.0 && freqMHz < 14.35:
		return "20m"
	case freqMHz >= 18.068 && freqMHz < 18.168:
		return "17m"
	case freqMHz >= 21.0 && freqMHz < 21.45:
		return "15m"
	case freqMHz >= 24.89 && freqMHz < 24.99:
		return "12m"
	case freqMHz >= 28.0 && freqMHz < 29.7:
		return "10m"
	default:
		return fmt.Sprintf("%.3fMHz", freqMHz)
	}
}

// GetStats returns aggregator statistics
func (sa *SpotAggregator) GetStats() map[string]interface{} {
	sa.windowsMu.Lock()
	defer sa.windowsMu.Unlock()

	totalSpots := 0
	for _, spots := range sa.windows {
		totalSpots += len(spots)
	}

	return map[string]interface{}{
		"active_windows": len(sa.windows),
		"pending_spots":  totalSpots,
	}
}

// DebugMode can be set to enable debug logging
var DebugMode = false
