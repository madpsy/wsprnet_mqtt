package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// StoredSpot represents a spot stored in a file
type StoredSpot struct {
	Timestamp time.Time `json:"timestamp"`
	Callsign  string    `json:"callsign"`
	Locator   string    `json:"locator"`
	SNR       int       `json:"snr"`
	Frequency uint64    `json:"frequency"`
	Band      string    `json:"band"`
	DBm       int       `json:"dbm"`
	Drift     int       `json:"drift"`
	DT        float32   `json:"dt"`
	Country   string    `json:"country,omitempty"`
	// Fields only for deduped spots
	Instance  string  `json:"instance,omitempty"`  // Winning instance name
	Submitted bool    `json:"submitted,omitempty"` // True if HTTP request succeeded
	Error     *string `json:"error,omitempty"`     // Error message if submission failed
}

// SpotWriter manages writing spots to files
type SpotWriter struct {
	baseDir     string
	files       map[string]*os.File // instance name -> file handle
	dedupedFile *os.File
	mu          sync.Mutex

	// In-memory cache for queries (last 24 hours)
	rawSpots     map[string][]StoredSpot // instance name -> spots
	dedupedSpots []StoredSpot
	cacheMu      sync.RWMutex

	// Control
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewSpotWriter creates a new spot writer
func NewSpotWriter(baseDir string) (*SpotWriter, error) {
	// Create base directory if it doesn't exist
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create spots directory: %w", err)
	}

	sw := &SpotWriter{
		baseDir:      baseDir,
		files:        make(map[string]*os.File),
		rawSpots:     make(map[string][]StoredSpot),
		dedupedSpots: make([]StoredSpot, 0),
		stopChan:     make(chan struct{}),
	}

	// Open deduped file
	dedupedPath := filepath.Join(baseDir, "deduped.jsonl")
	f, err := os.OpenFile(dedupedPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open deduped file: %w", err)
	}
	sw.dedupedFile = f

	// Load existing spots from files
	if err := sw.loadExistingSpots(); err != nil {
		log.Printf("Warning: Failed to load existing spots: %v", err)
	}

	// Start cleanup goroutine
	sw.wg.Add(1)
	go sw.cleanupOldSpots()

	log.Printf("Spot writer initialized (directory: %s)", baseDir)
	return sw, nil
}

// WriteRaw writes a raw spot to an instance file
func (sw *SpotWriter) WriteRaw(spot *WSPRReportWithSource) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	instanceName := spot.InstanceName
	if instanceName == "" {
		instanceName = "unknown"
	}

	// Open file if not already open
	if sw.files[instanceName] == nil {
		filename := fmt.Sprintf("instance_%s.jsonl", instanceName)
		path := filepath.Join(sw.baseDir, filename)
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open instance file: %w", err)
		}
		sw.files[instanceName] = f
	}

	// Create stored spot
	stored := StoredSpot{
		Timestamp: spot.EpochTime,
		Callsign:  spot.Callsign,
		Locator:   spot.Locator,
		SNR:       spot.SNR,
		Frequency: spot.ReceiverFreq,
		Band:      frequencyToBand(spot.ReceiverFreq),
		DBm:       spot.DBm,
		Drift:     spot.Drift,
		DT:        spot.DT,
		Country:   spot.Country,
		Instance:  spot.InstanceName,
	}

	// Write to file
	data, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("failed to marshal spot: %w", err)
	}

	if _, err := sw.files[instanceName].Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write spot: %w", err)
	}

	// Flush to ensure data is written
	if err := sw.files[instanceName].Sync(); err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}

	// Add to in-memory cache
	sw.cacheMu.Lock()
	sw.rawSpots[instanceName] = append(sw.rawSpots[instanceName], stored)
	sw.cacheMu.Unlock()

	return nil
}

// WriteDeduped writes a deduped spot with submission status
func (sw *SpotWriter) WriteDeduped(spot *WSPRReportWithSource, submitted bool, errorMsg string) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	// Create stored spot
	stored := StoredSpot{
		Timestamp: spot.EpochTime,
		Callsign:  spot.Callsign,
		Locator:   spot.Locator,
		SNR:       spot.SNR,
		Frequency: spot.ReceiverFreq,
		Band:      frequencyToBand(spot.ReceiverFreq),
		DBm:       spot.DBm,
		Drift:     spot.Drift,
		DT:        spot.DT,
		Country:   spot.Country,
		Instance:  spot.InstanceName,
		Submitted: submitted,
	}

	if errorMsg != "" {
		stored.Error = &errorMsg
	}

	// Write to file
	data, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("failed to marshal deduped spot: %w", err)
	}

	if _, err := sw.dedupedFile.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write deduped spot: %w", err)
	}

	// Flush to ensure data is written
	if err := sw.dedupedFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync deduped file: %w", err)
	}

	// Add to in-memory cache
	sw.cacheMu.Lock()
	sw.dedupedSpots = append(sw.dedupedSpots, stored)
	sw.cacheMu.Unlock()

	return nil
}

// loadExistingSpots loads spots from existing files into memory
func (sw *SpotWriter) loadExistingSpots() error {
	cutoff := time.Now().Add(-24 * time.Hour)

	// Load instance files
	entries, err := os.ReadDir(sw.baseDir)
	if err != nil {
		return fmt.Errorf("failed to read spots directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		path := filepath.Join(sw.baseDir, filename)

		if filename == "deduped.jsonl" {
			// Load deduped spots
			spots, err := sw.loadSpotsFromFile(path, cutoff)
			if err != nil {
				log.Printf("Warning: Failed to load deduped spots: %v", err)
				continue
			}
			sw.dedupedSpots = spots
			log.Printf("Loaded %d deduped spots from file", len(spots))
		} else if len(filename) > 9 && filename[:9] == "instance_" && filename[len(filename)-6:] == ".jsonl" {
			// Extract instance name
			instanceName := filename[9 : len(filename)-6]

			// Load instance spots
			spots, err := sw.loadSpotsFromFile(path, cutoff)
			if err != nil {
				log.Printf("Warning: Failed to load spots for instance %s: %v", instanceName, err)
				continue
			}
			sw.rawSpots[instanceName] = spots
			log.Printf("Loaded %d spots for instance %s", len(spots), instanceName)
		}
	}

	return nil
}

// loadSpotsFromFile loads spots from a JSONL file, filtering to last 24 hours
func (sw *SpotWriter) loadSpotsFromFile(path string, cutoff time.Time) ([]StoredSpot, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []StoredSpot{}, nil
		}
		return nil, err
	}
	defer file.Close()

	var spots []StoredSpot
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		var spot StoredSpot
		if err := json.Unmarshal(scanner.Bytes(), &spot); err != nil {
			log.Printf("Warning: Failed to parse spot line: %v", err)
			continue
		}

		// Only keep spots from last 24 hours
		if spot.Timestamp.After(cutoff) {
			spots = append(spots, spot)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return spots, nil
}

// cleanupOldSpots periodically removes spots older than 24 hours
func (sw *SpotWriter) cleanupOldSpots() {
	defer sw.wg.Done()

	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-sw.stopChan:
			return
		case <-ticker.C:
			sw.performCleanup()
		}
	}
}

// performCleanup removes spots older than 24 hours from memory and rewrites files
func (sw *SpotWriter) performCleanup() {
	cutoff := time.Now().Add(-24 * time.Hour)

	sw.cacheMu.Lock()
	defer sw.cacheMu.Unlock()

	// Clean up raw spots in memory
	for instance, spots := range sw.rawSpots {
		filtered := make([]StoredSpot, 0, len(spots))
		for _, spot := range spots {
			if spot.Timestamp.After(cutoff) {
				filtered = append(filtered, spot)
			}
		}
		sw.rawSpots[instance] = filtered
	}

	// Clean up deduped spots in memory
	filtered := make([]StoredSpot, 0, len(sw.dedupedSpots))
	for _, spot := range sw.dedupedSpots {
		if spot.Timestamp.After(cutoff) {
			filtered = append(filtered, spot)
		}
	}
	sw.dedupedSpots = filtered

	// Rewrite files (do this in background to avoid blocking)
	go sw.rewriteFiles()

	log.Printf("Cleanup: Kept spots from last 24 hours (cutoff: %s)", cutoff.Format("2006-01-02 15:04:05"))
}

// rewriteFiles rewrites all files with only the spots in memory (last 24 hours)
func (sw *SpotWriter) rewriteFiles() {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	sw.cacheMu.RLock()
	defer sw.cacheMu.RUnlock()

	// Rewrite instance files
	for instance, spots := range sw.rawSpots {
		filename := fmt.Sprintf("instance_%s.jsonl", instance)
		path := filepath.Join(sw.baseDir, filename)

		if err := sw.rewriteFile(path, spots); err != nil {
			log.Printf("Warning: Failed to rewrite file for instance %s: %v", instance, err)
		}
	}

	// Rewrite deduped file
	dedupedPath := filepath.Join(sw.baseDir, "deduped.jsonl")
	if err := sw.rewriteFile(dedupedPath, sw.dedupedSpots); err != nil {
		log.Printf("Warning: Failed to rewrite deduped file: %v", err)
	}
}

// rewriteFile rewrites a file with the given spots
func (sw *SpotWriter) rewriteFile(path string, spots []StoredSpot) error {
	// Write to temporary file first
	tmpPath := path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	for _, spot := range spots {
		data, err := json.Marshal(spot)
		if err != nil {
			f.Close()
			os.Remove(tmpPath)
			return err
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			f.Close()
			os.Remove(tmpPath)
			return err
		}
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	// Rename temporary file to actual file
	return os.Rename(tmpPath, path)
}

// GetRawSpots returns raw spots for an instance with optional filters
func (sw *SpotWriter) GetRawSpots(instance, band string, startTime, endTime time.Time) []StoredSpot {
	sw.cacheMu.RLock()
	defer sw.cacheMu.RUnlock()

	var spots []StoredSpot
	if instance == "" || instance == "all" {
		// Get spots from all instances
		for _, instanceSpots := range sw.rawSpots {
			spots = append(spots, instanceSpots...)
		}
	} else {
		spots = sw.rawSpots[instance]
	}

	// Apply filters
	return sw.filterSpots(spots, band, startTime, endTime)
}

// GetDedupedSpots returns deduped spots with optional filters
func (sw *SpotWriter) GetDedupedSpots(band string, startTime, endTime time.Time, submittedOnly *bool) []StoredSpot {
	sw.cacheMu.RLock()
	defer sw.cacheMu.RUnlock()

	spots := sw.filterSpots(sw.dedupedSpots, band, startTime, endTime)

	// Filter by submission status if specified
	if submittedOnly != nil {
		filtered := make([]StoredSpot, 0, len(spots))
		for _, spot := range spots {
			if spot.Submitted == *submittedOnly {
				filtered = append(filtered, spot)
			}
		}
		return filtered
	}

	return spots
}

// filterSpots applies band and time range filters
func (sw *SpotWriter) filterSpots(spots []StoredSpot, band string, startTime, endTime time.Time) []StoredSpot {
	filtered := make([]StoredSpot, 0, len(spots))

	for _, spot := range spots {
		// Band filter
		if band != "" && band != "all" && spot.Band != band {
			continue
		}

		// Time range filter
		if !startTime.IsZero() && spot.Timestamp.Before(startTime) {
			continue
		}
		if !endTime.IsZero() && spot.Timestamp.After(endTime) {
			continue
		}

		filtered = append(filtered, spot)
	}

	return filtered
}

// GetInstanceNames returns all instance names that have spots
func (sw *SpotWriter) GetInstanceNames() []string {
	sw.cacheMu.RLock()
	defer sw.cacheMu.RUnlock()

	names := make([]string, 0, len(sw.rawSpots))
	for name := range sw.rawSpots {
		names = append(names, name)
	}
	return names
}

// GapInfo represents information about missing WSPR cycles
type GapInfo struct {
	Instance      string   `json:"instance"`
	Band          string   `json:"band"`
	MissingCycles []string `json:"missing_cycles"` // e.g., ["18:20", "18:22"]
	GapCount      int      `json:"gap_count"`
	TotalCycles   int      `json:"total_cycles"`
	CoverageRate  float64  `json:"coverage_rate"` // percentage
}

// AnalyzeGaps analyzes spots to find missing WSPR cycles
// WSPR cycles occur every 2 minutes at even minutes (00, 02, 04, etc.)
func (sw *SpotWriter) AnalyzeGaps(hoursBack int) map[string][]GapInfo {
	sw.cacheMu.RLock()
	defer sw.cacheMu.RUnlock()

	result := make(map[string][]GapInfo)

	// Calculate time range
	endTime := time.Now()
	startTime := endTime.Add(-time.Duration(hoursBack) * time.Hour)

	// Round start time to nearest 2-minute boundary
	startTime = time.Unix((startTime.Unix()/120)*120, 0)

	// Generate all expected WSPR cycles in the time range
	expectedCycles := make(map[int64]bool)
	for t := startTime.Unix(); t <= endTime.Unix(); t += 120 {
		expectedCycles[t] = true
	}
	totalExpectedCycles := len(expectedCycles)

	// Analyze raw spots per instance per band
	for instance, spots := range sw.rawSpots {
		// Group spots by band
		bandSpots := make(map[string][]StoredSpot)
		for _, spot := range spots {
			if spot.Timestamp.After(startTime) && spot.Timestamp.Before(endTime) {
				bandSpots[spot.Band] = append(bandSpots[spot.Band], spot)
			}
		}

		// Analyze each band
		for band, spots := range bandSpots {
			// Find which cycles have spots
			cyclesWithSpots := make(map[int64]bool)
			for _, spot := range spots {
				cycleTime := (spot.Timestamp.Unix() / 120) * 120
				cyclesWithSpots[cycleTime] = true
			}

			// Find missing cycles and collect them with timestamps for sorting
			type missingCycle struct {
				timestamp int64
				formatted string
			}
			var missing []missingCycle
			for cycle := range expectedCycles {
				if !cyclesWithSpots[cycle] {
					t := time.Unix(cycle, 0).UTC()
					missing = append(missing, missingCycle{
						timestamp: cycle,
						formatted: t.Format("15:04"),
					})
				}
			}

			// Sort by timestamp
			sort.Slice(missing, func(i, j int) bool {
				return missing[i].timestamp < missing[j].timestamp
			})

			// Extract formatted times
			missingCycles := make([]string, len(missing))
			for i, m := range missing {
				missingCycles[i] = m.formatted
			}

			// Only include if there are gaps
			if len(missingCycles) > 0 {
				coverageRate := float64(len(cyclesWithSpots)) / float64(totalExpectedCycles) * 100

				result[instance] = append(result[instance], GapInfo{
					Instance:      instance,
					Band:          band,
					MissingCycles: missingCycles,
					GapCount:      len(missingCycles),
					TotalCycles:   totalExpectedCycles,
					CoverageRate:  coverageRate,
				})
			}
		}
	}

	// Analyze deduped spots
	bandSpots := make(map[string][]StoredSpot)
	for _, spot := range sw.dedupedSpots {
		if spot.Timestamp.After(startTime) && spot.Timestamp.Before(endTime) {
			bandSpots[spot.Band] = append(bandSpots[spot.Band], spot)
		}
	}

	for band, spots := range bandSpots {
		// Find which cycles have spots
		cyclesWithSpots := make(map[int64]bool)
		for _, spot := range spots {
			cycleTime := (spot.Timestamp.Unix() / 120) * 120
			cyclesWithSpots[cycleTime] = true
		}

		// Find missing cycles and collect them with timestamps for sorting
		type missingCycle struct {
			timestamp int64
			formatted string
		}
		var missing []missingCycle
		for cycle := range expectedCycles {
			if !cyclesWithSpots[cycle] {
				t := time.Unix(cycle, 0).UTC()
				missing = append(missing, missingCycle{
					timestamp: cycle,
					formatted: t.Format("15:04"),
				})
			}
		}

		// Sort by timestamp
		sort.Slice(missing, func(i, j int) bool {
			return missing[i].timestamp < missing[j].timestamp
		})

		// Extract formatted times
		missingCycles := make([]string, len(missing))
		for i, m := range missing {
			missingCycles[i] = m.formatted
		}

		// Only include if there are gaps
		if len(missingCycles) > 0 {
			coverageRate := float64(len(cyclesWithSpots)) / float64(totalExpectedCycles) * 100

			result["deduped"] = append(result["deduped"], GapInfo{
				Instance:      "deduped",
				Band:          band,
				MissingCycles: missingCycles,
				GapCount:      len(missingCycles),
				TotalCycles:   totalExpectedCycles,
				CoverageRate:  coverageRate,
			})
		}
	}

	return result
}

// ClearAllSpots clears all spot logs from memory and disk
func (sw *SpotWriter) ClearAllSpots() error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	sw.cacheMu.Lock()
	defer sw.cacheMu.Unlock()

	log.Println("Clearing all spot logs...")

	// Clear in-memory caches
	sw.rawSpots = make(map[string][]StoredSpot)
	sw.dedupedSpots = make([]StoredSpot, 0)

	// Close all open files
	for _, f := range sw.files {
		f.Close()
	}
	sw.files = make(map[string]*os.File)

	if sw.dedupedFile != nil {
		sw.dedupedFile.Close()
	}

	// Delete all spot files
	entries, err := os.ReadDir(sw.baseDir)
	if err != nil {
		return fmt.Errorf("failed to read spots directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		// Delete all .jsonl files (instance files and deduped file)
		if len(filename) > 6 && filename[len(filename)-6:] == ".jsonl" {
			path := filepath.Join(sw.baseDir, filename)
			if err := os.Remove(path); err != nil {
				log.Printf("Warning: Failed to delete spot file %s: %v", filename, err)
			} else {
				log.Printf("Deleted spot file: %s", filename)
			}
		}
	}

	// Reopen deduped file
	dedupedPath := filepath.Join(sw.baseDir, "deduped.jsonl")
	f, err := os.OpenFile(dedupedPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to reopen deduped file: %w", err)
	}
	sw.dedupedFile = f

	log.Println("All spot logs cleared successfully")
	return nil
}

// Stop stops the spot writer and closes all files
func (sw *SpotWriter) Stop() {
	close(sw.stopChan)
	sw.wg.Wait()

	sw.mu.Lock()
	defer sw.mu.Unlock()

	// Close all instance files
	for _, f := range sw.files {
		f.Close()
	}

	// Close deduped file
	if sw.dedupedFile != nil {
		sw.dedupedFile.Close()
	}

	log.Println("Spot writer stopped")
}
