package pipeline

import (
	"bufio"
	"cursor-twitter/src/tweets"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"log/slog"
)

// Global flag to track when persistence is in progress
var persistenceInProgress int32 // Atomic flag: 0 = not in progress, 1 = in progress

// FrequencyComputationThread (FCT) manages token counting and frequency calculations
// in a separate goroutine to avoid blocking the main tweet processing pipeline.
type FrequencyComputationThread struct {
	// Token processing
	tokenCounter      *TokenCounter
	inboundTokenQueue *TokenQueue

	// Frequency calculation control
	shouldRebuild int32 // Atomic boolean (0 = false, 1 = true)

	// Thread control
	stopChan chan struct{}
	wg       sync.WaitGroup

	// Configuration
	freqClasses int
	windowSize  int // Number of tokens to process before triggering rebuild

	// Current filters (thread-safe access)
	currentFilters []FreqClassFilter
	filtersMutex   sync.RWMutex

	// Debug: Track rebuild count
	rebuildCount      int
	rebuildCountMutex sync.Mutex

	// Token file rotation
	tokenBatchSize       int      // window / token_persist_files
	tokenPersistFiles    int      // Number of token files to keep
	rebuildEveryFiles    int      // Rebuild frequency filters every N files
	tokensSinceLastWrite int      // Tokens processed since last file write
	tokenFileCounter     int      // Counter for token file naming
	stateDir             string   // Directory for token files
	accumulatedTokens    []string // Tokens accumulated since last file write

	// Debug: Track loop iterations for logging
	loopCount int
}

// NewFrequencyComputationThread creates a new FCT with the specified configuration
func NewFrequencyComputationThread(
	tokenCounter *TokenCounter,
	inboundTokenQueue *TokenQueue,
	freqClasses int,
	windowSize int,
	tokenPersistFiles int,
	rebuildEveryFiles int,
	stateDir string,
) *FrequencyComputationThread {
	tokenBatchSize := windowSize / tokenPersistFiles
	if tokenBatchSize < 1 {
		tokenBatchSize = 1 // Ensure at least 1 token per batch
	}

	// Initialize tokenFileCounter based on existing files
	tokenFileCounter := 0
	files, err := filepath.Glob(filepath.Join(stateDir, "token_batch_*.txt"))
	if err == nil && len(files) > 0 {
		maxNum := -1
		for _, file := range files {
			base := filepath.Base(file)
			var num int
			_, scanErr := fmt.Sscanf(base, "token_batch_%06d.txt", &num)
			if scanErr == nil && num > maxNum {
				maxNum = num
			}
		}
		if maxNum >= 0 {
			tokenFileCounter = maxNum + 1
		}
	}

	return &FrequencyComputationThread{
		tokenCounter:      tokenCounter,
		inboundTokenQueue: inboundTokenQueue,
		stopChan:          make(chan struct{}),
		freqClasses:       freqClasses,
		windowSize:        windowSize,
		tokenBatchSize:    tokenBatchSize,
		tokenPersistFiles: tokenPersistFiles,
		rebuildEveryFiles: rebuildEveryFiles,
		stateDir:          stateDir,
		tokenFileCounter:  tokenFileCounter,
	}
}

// Start begins the FCT goroutine
func (fct *FrequencyComputationThread) Start() {
	slog.Info("FCT Start method called")
	fct.wg.Add(1)
	go fct.run()
	slog.Info("FCT goroutine launched")
	slog.Info("FrequencyComputationThread started",
		"freq_classes", fct.freqClasses,
		"token_batch_size", fct.tokenBatchSize,
		"token_persist_files", fct.tokenPersistFiles,
		"state_dir", fct.stateDir)
}

// Stop gracefully stops the FCT goroutine
func (fct *FrequencyComputationThread) Stop() {
	close(fct.stopChan)
	fct.wg.Wait()
	slog.Info("FrequencyComputationThread stopped")
}

// TriggerRebuild signals that a frequency class rebuild is needed
// The FCT will handle the rebuild autonomously when it's ready
func (fct *FrequencyComputationThread) TriggerRebuild() {
	atomic.StoreInt32(&fct.shouldRebuild, 1)
	slog.Info("FCT: Rebuild flag SET - frequency boundary crossed")
}

// run is the main loop of the FCT goroutine
func (fct *FrequencyComputationThread) run() {
	defer fct.wg.Done()

	// Add panic recovery
	defer func() {
		if r := recover(); r != nil {
			slog.Error("FCT run loop panicked", "panic", r)
		}
		slog.Info("FCT run method exiting")
	}()

	slog.Info("FCT run method entered")
	slog.Info("FCT run loop starting")

	for {
		select {
		case <-fct.stopChan:
			slog.Info("FCT run loop stopping")
			return
		default:
			fct.loopCount++
			if fct.loopCount%10000 == 0 { // Log every 10,000 iterations
				slog.Info("FCT loop iteration", "count", fct.loopCount)
			}
			if fct.loopCount%100000 == 0 { // Log every 100,000 iterations to confirm it's running
				slog.Info("FCT: Loop is running, iteration", "count", fct.loopCount)
			}

			// Check if rebuild is needed first
			shouldRebuild := atomic.LoadInt32(&fct.shouldRebuild) == 1

			// Debug: Log rebuild flag status more frequently
			if fct.loopCount%1000 == 0 {
				slog.Info("FCT: Rebuild flag check",
					"loop_count", fct.loopCount,
					"should_rebuild", shouldRebuild)
			}

			// ALWAYS log when rebuild flag is true (this should be rare)
			if shouldRebuild {
				slog.Info("FCT: REBUILD FLAG DETECTED!",
					"loop_count", fct.loopCount,
					"should_rebuild", shouldRebuild)
			}

			// Debug: Log the branch we're taking more frequently
			if fct.loopCount%1000 == 0 {
				if shouldRebuild {
					slog.Info("FCT: Taking rebuild branch", "loop_count", fct.loopCount)
				} else {
					slog.Debug("FCT: Taking token processing branch", "loop_count", fct.loopCount)
				}
			}

			if shouldRebuild {
				// Increment rebuild count
				fct.rebuildCountMutex.Lock()
				fct.rebuildCount++
				currentRebuildCount := fct.rebuildCount
				fct.rebuildCountMutex.Unlock()

				// Consume all accumulated tokens before starting computation
				slog.Info("FCT: Consuming accumulated tokens before rebuild", "rebuild_count", currentRebuildCount)
				fct.consumeAllAccumulatedTokens()

				rebuildStartTime := time.Now()
				slog.Info("FCT: Starting rebuild", "rebuild_count", currentRebuildCount, "start_time", rebuildStartTime.Format("15:04:05"))
				fmt.Printf("*** FCT REBUILD STARTED at %s ***\n", rebuildStartTime.Format("15:04:05"))
				// Pause token processing and do rebuild
				fct.performRebuild()

				// Debug: Log queue sizes after rebuild
				inboundSizeAfter := fct.inboundTokenQueue.Len()
				slog.Info("FCT: Queue sizes after rebuild",
					"rebuild_count", currentRebuildCount,
					"inbound_queue_size", inboundSizeAfter)

			}

			// Process a small amount of tokens (main loop body - always happens)
			fct.processTokens()

			// Check if we should trigger a rebuild based on token count or file count
			// This is the FCT's autonomous rebuild triggering logic
			tokensProcessed := fct.tokenCounter.GetTotalTokens()
			if tokensProcessed >= fct.windowSize && atomic.LoadInt32(&fct.shouldRebuild) == 0 {
				slog.Info("FCT: Autonomous rebuild trigger - token count threshold reached",
					"tokens_processed", tokensProcessed,
					"window_size", fct.windowSize)
				fct.TriggerRebuild()
			}

			// Add a small delay when no tokens are being processed to prevent CPU spinning
			inboundSize := fct.inboundTokenQueue.Len()
			if inboundSize == 0 {
				time.Sleep(1 * time.Millisecond)
			}

			// Debug: Log when queues are empty
			if fct.loopCount%1000 == 0 && inboundSize == 0 {
				slog.Debug("FCT: No tokens to process, waiting...",
					"loop_count", fct.loopCount,
					"inbound_queue_size", inboundSize)
			}
		}
	}
}

// processTokens processes tokens from both queues
func (fct *FrequencyComputationThread) processTokens() {
	// Only log when we actually process tokens or when queues have significant backlog
	inboundProcessed := 0

	for {
		tokens := fct.inboundTokenQueue.Dequeue()
		if tokens == nil {
			break // Queue is empty
		}
		fct.tokenCounter.IncrementTokens(tokens)
		inboundProcessed += len(tokens)

		// Collect tokens for potential file writing
		fct.accumulatedTokens = append(fct.accumulatedTokens, tokens...)
	}

	// Update token count for file rotation
	fct.tokensSinceLastWrite += inboundProcessed

	// Check if we need to write a token file
	if fct.tokensSinceLastWrite >= fct.tokenBatchSize {
		fmt.Printf("[DIAG] FCT writing token file: tokensSinceLastWrite=%d >= tokenBatchSize=%d\n", fct.tokensSinceLastWrite, fct.tokenBatchSize)
		// Write tokens to file (only the batch size worth)
		tokensForFile := fct.accumulatedTokens[:fct.tokenBatchSize]
		if err := fct.writeTokenFile(tokensForFile); err != nil {
			slog.Error("Failed to write token file", "error", err)
		} else {
			slog.Info("Token file written successfully",
				"tokens_written", len(tokensForFile),
				"file_counter", fct.tokenFileCounter-1)
		}

		// Reset counter and remaining tokens
		fct.tokensSinceLastWrite = 0
		fct.accumulatedTokens = fct.accumulatedTokens[fct.tokenBatchSize:]

		// If we have remaining tokens, write them too (should be rare)
		for len(fct.accumulatedTokens) >= fct.tokenBatchSize {
			tokensForFile = fct.accumulatedTokens[:fct.tokenBatchSize]
			fmt.Printf("[DIAG] FCT writing additional token file: accumulatedTokens=%d >= tokenBatchSize=%d\n", len(fct.accumulatedTokens), fct.tokenBatchSize)
			if err := fct.writeTokenFile(tokensForFile); err != nil {
				slog.Error("Failed to write additional token file", "error", err)
			} else {
				slog.Info("Additional token file written successfully",
					"tokens_written", len(tokensForFile),
					"file_counter", fct.tokenFileCounter-1)
			}
			fct.accumulatedTokens = fct.accumulatedTokens[fct.tokenBatchSize:]
		}

		// Update counter for any remaining tokens
		fct.tokensSinceLastWrite = len(fct.accumulatedTokens)
	}

	// Only log if we processed tokens or if queues are getting backed up
	if inboundProcessed > 0 {
		// Only log every 1000 iterations to reduce logging overhead
		if fct.loopCount%1000 == 0 {
			slog.Info("FCT processed tokens",
				"inbound_processed", inboundProcessed,
				"inbound_queue_size_after", fct.inboundTokenQueue.Len(),
				"tokens_since_last_write", fct.tokensSinceLastWrite,
				"token_batch_size", fct.tokenBatchSize)
		}
	} else {
		// Check if queues are getting backed up but we didn't process anything
		currentInboundSize := fct.inboundTokenQueue.Len()
		if currentInboundSize > 100 {
			slog.Warn("FCT inbound queue has backlog but processed no tokens",
				"inbound_queue_size", currentInboundSize)
		}
	}
}

// checkForRebuild method removed - rebuild logic now integrated into main loop

// performRebuild performs the actual frequency class calculation and filter building
func (fct *FrequencyComputationThread) performRebuild() {
	// Reset the rebuild flag immediately so loop can detect new rebuild requests
	atomic.StoreInt32(&fct.shouldRebuild, 0)
	slog.Info("FCT: Rebuild flag RESET at start of performRebuild")

	startTime := time.Now()
	slog.Info("Starting frequency class rebuild")

	// Get a snapshot of current token counts for frequency calculations
	tokenCounts := fct.tokenCounter.CountsSnapshot()

	// Perform the frequency class calculation
	slog.Info("FCT: About to call BuildFrequencyClassHashSets", "token_count", len(tokenCounts), "freq_classes", fct.freqClasses)
	result := BuildFrequencyClassHashSets(tokenCounts, fct.freqClasses, nil, nil)
	slog.Info("FCT: BuildFrequencyClassHashSets returned", "filters_built", len(result.Filters))

	// Clear the token counter after frequency calculation
	// This ensures each rebuild is based only on the current window's tokens
	fct.tokenCounter.Clear()

	// Store the filters for the main thread to access
	fct.filtersMutex.Lock()
	fct.currentFilters = result.Filters
	fct.filtersMutex.Unlock()

	// Also install globally for main thread
	slog.Info("Installing filters globally")
	SetGlobalFilters(result.Filters)
	slog.Info("Filters installed globally", "num_filters", len(result.Filters))

	// Save the data structures to files
	savePersistedState(result, tokenCounts)

	duration := time.Since(startTime)
	completionTime := time.Now()
	slog.Info("Frequency class rebuild completed",
		"duration", duration,
		"completion_time", completionTime.Format("15:04:05"),
		"token_count", len(tokenCounts),
		"filters_built", len(result.Filters))

	// Add diagnostic line to show filters are installed
	slog.Info("Frequency class filters are now active",
		"num_filters", len(result.Filters),
		"completion_time", completionTime.Format("15:04:05"),
		"duration", duration)

	// Log top tokens for debugging
	if len(result.TopTokens) > 0 {
		slog.Debug("Top tokens after rebuild",
			"top_token", result.TopTokens[0].Token,
			"top_count", result.TopTokens[0].Count)
	}
}

// GetQueueStats returns statistics about the current queue states
func (fct *FrequencyComputationThread) GetQueueStats() map[string]int {
	return map[string]int{
		"inbound_token_queue_size": fct.inboundTokenQueue.Len(),
		"distinct_tokens":          len(fct.tokenCounter.Counts()),
	}
}

// GetCurrentFilters returns the current frequency class filters (thread-safe)
func (fct *FrequencyComputationThread) GetCurrentFilters() []FreqClassFilter {
	fct.filtersMutex.RLock()
	defer fct.filtersMutex.RUnlock()
	return fct.currentFilters
}

// GetRebuildCount returns the number of rebuilds performed (for debugging)
func (fct *FrequencyComputationThread) GetRebuildCount() int {
	fct.rebuildCountMutex.Lock()
	defer fct.rebuildCountMutex.Unlock()
	return fct.rebuildCount
}

// consumeAllAccumulatedTokens processes tokens from both queues using queue size snapshots
func (fct *FrequencyComputationThread) consumeAllAccumulatedTokens() {
	// Get current queue sizes (snapshot to prevent infinite catch-up)
	inboundQueueSize := fct.inboundTokenQueue.Len()

	slog.Info("FCT: Starting token consumption with queue snapshot",
		"inbound_queue_size", inboundQueueSize)

	inboundProcessed := 0

	// Process exactly inboundQueueSize tokens from inbound queue
	for i := 0; i < inboundQueueSize; i++ {
		tokens := fct.inboundTokenQueue.Dequeue()
		if tokens == nil {
			break // Queue is empty (shouldn't happen given our size check)
		}
		fct.tokenCounter.IncrementTokens(tokens)
		inboundProcessed += len(tokens)
	}

	slog.Info("FCT: Consumed tokens using queue snapshot",
		"inbound_processed", inboundProcessed,
		"inbound_queue_size_after", fct.inboundTokenQueue.Len())
}

// savePersistedState saves the data structures to files
func savePersistedState(result FreqClassResult, tokenCounts map[string]int) {
	// Set persistence flag
	atomic.StoreInt32(&persistenceInProgress, 1)
	defer atomic.StoreInt32(&persistenceInProgress, 0)

	slog.Info("Starting to save persisted state")
	saveStartTime := time.Now()

	// Get the state directory from config (hardcoded for now, could be made configurable)
	stateDir := "../data/state"

	// Save TokenCounter directly from the counts map
	tokenCounterPath := filepath.Join(stateDir, "token_counter.json")
	if err := saveTokenCountsToFile(tokenCounterPath, tokenCounts); err != nil {
		slog.Error("Failed to save TokenCounter", "error", err, "path", tokenCounterPath)
	} else {
		totalTokens := 0
		for _, count := range tokenCounts {
			totalTokens += count
		}
		slog.Info("TokenCounter saved", "total_tokens", totalTokens, "distinct_tokens", len(tokenCounts))
	}

	// Save FrequencyClassResult
	freqClassPath := filepath.Join(stateDir, "frequency_classes.json")
	if err := result.SaveToFile(freqClassPath); err != nil {
		slog.Error("Failed to save FrequencyClassResult", "error", err, "path", freqClassPath)
	} else {
		slog.Info("FrequencyClassResult saved", "num_classes", len(result.Filters))
	}

	// Save ThreePartKey mappings
	threePKPath := filepath.Join(stateDir, "threepartkey_mappings.json")
	if err := saveThreePartKeyMappingsToFile(threePKPath, TokenTo3PK); err != nil {
		slog.Error("Failed to save ThreePartKey mappings", "error", err, "path", threePKPath)
	} else {
		slog.Info("ThreePartKey mappings saved", "num_mappings", len(TokenTo3PK))
	}

	saveDuration := time.Since(saveStartTime)
	slog.Info("Persisted state saving completed", "duration", saveDuration.String())
}

// IsPersistenceInProgress returns true if persistence operations are currently running
func IsPersistenceInProgress() bool {
	return atomic.LoadInt32(&persistenceInProgress) == 1
}

// saveTokenCountsToFile saves token counts directly to a file using gob encoding
func saveTokenCountsToFile(filename string, counts map[string]int) error {
	// Ensure the directory exists
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %v", dir, err)
	}

	// Create the file
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %v", filename, err)
	}
	defer file.Close()

	// Encode and write to file
	encoder := gob.NewEncoder(file)
	if err := encoder.Encode(counts); err != nil {
		return fmt.Errorf("failed to encode counts to %s: %v", filename, err)
	}

	return nil
}

// saveThreePartKeyMappingsToFile saves ThreePartKey mappings to a file
func saveThreePartKeyMappingsToFile(filename string, mapping map[string]tweets.ThreePartKey) error {
	token3PKMutex.RLock()
	snapshot := make(map[string]tweets.ThreePartKey, len(mapping))
	for k, v := range mapping {
		snapshot[k] = v
	}
	token3PKMutex.RUnlock()
	// Ensure the directory exists
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %v", dir, err)
	}

	// Create the file
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %v", filename, err)
	}
	defer file.Close()

	// Encode and write to file (use the snapshot, not the live map)
	encoder := gob.NewEncoder(file)
	if err := encoder.Encode(snapshot); err != nil {
		return fmt.Errorf("failed to encode mappings to %s: %v", filename, err)
	}

	return nil
}

// writeTokenFile writes a batch of tokens to a file and manages rotation
func (fct *FrequencyComputationThread) writeTokenFile(tokens []string) error {
	// Create filename with counter
	filename := filepath.Join(fct.stateDir, fmt.Sprintf("token_batch_%06d.txt", fct.tokenFileCounter))

	// Ensure directory exists
	if err := os.MkdirAll(fct.stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create token file directory: %v", err)
	}

	// Write tokens to file
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create token file %s: %v", filename, err)
	}
	defer file.Close()

	// Write tokens as text, one per line
	for _, token := range tokens {
		if _, err := fmt.Fprintln(file, token); err != nil {
			return fmt.Errorf("failed to write token to %s: %v", filename, err)
		}
	}

	// Increment file counter
	fct.tokenFileCounter++

	// Check if we should trigger a rebuild based on file count
	// Only trigger rebuilds after we have the full window of files (tokenPersistFiles)
	if fct.tokenFileCounter > fct.tokenPersistFiles &&
		(fct.tokenFileCounter-fct.tokenPersistFiles)%fct.rebuildEveryFiles == 0 {
		slog.Info("FCT: File-based rebuild trigger",
			"file_counter", fct.tokenFileCounter,
			"rebuild_every_files", fct.rebuildEveryFiles,
			"files_since_window_full", fct.tokenFileCounter-fct.tokenPersistFiles)
		fct.TriggerRebuild()
	}

	// Check if we need to rotate files
	return fct.rotateTokenFiles()
}

// rotateTokenFiles manages the rolling window of token files
func (fct *FrequencyComputationThread) rotateTokenFiles() error {
	// Get list of existing token files
	files, err := filepath.Glob(filepath.Join(fct.stateDir, "token_batch_*.txt"))
	if err != nil {
		return fmt.Errorf("failed to list token files: %v", err)
	}

	// If we have more files than allowed, delete the oldest
	if len(files) > fct.tokenPersistFiles {
		// Sort files by name (which includes the counter, so oldest first)
		sort.Strings(files)

		// Delete the oldest file(s) to get back to the limit
		filesToDelete := len(files) - fct.tokenPersistFiles
		for i := 0; i < filesToDelete; i++ {
			oldestFile := files[i]

			// Read tokens from the oldest file and decrement counters
			if err := fct.decrementTokensFromFile(oldestFile); err != nil {
				slog.Error("Failed to decrement tokens from file", "file", oldestFile, "error", err)
				// Continue with deletion even if decrement fails
			}

			// Delete the file
			if err := os.Remove(oldestFile); err != nil {
				slog.Error("Failed to delete old token file", "file", oldestFile, "error", err)
			} else {
				slog.Info("Deleted old token file", "file", oldestFile)
			}
		}
	}

	return nil
}

// decrementTokensFromFile reads tokens from a file and decrements their counts
func (fct *FrequencyComputationThread) decrementTokensFromFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open token file %s: %v", filename, err)
	}
	defer file.Close()

	var tokens []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		token := strings.TrimSpace(scanner.Text())
		if token != "" {
			tokens = append(tokens, token)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read tokens from %s: %v", filename, err)
	}

	// Decrement token counts
	fct.tokenCounter.DecrementTokens(tokens)

	slog.Info("Decremented tokens from file", "file", filename, "token_count", len(tokens))
	return nil
}
