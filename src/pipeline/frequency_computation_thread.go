package pipeline

import (
	"bufio"
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
	shouldRebuild bool // Internal rebuild flag

	// Thread control
	stopChan chan struct{}
	wg       sync.WaitGroup

	// Configuration
	freqClasses   int
	windowSize    int // Number of tokens to process before triggering rebuild
	windowBatches int // Number of batches in the window (for dynamic safety buffer calculation)

	// Debug: Track rebuild count
	rebuildCount int

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

	// Adaptive frequency class building configuration
	minCountThreshold int
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
	minCountThreshold int,
	windowBatches int,
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
		windowBatches:     windowBatches,
		tokenBatchSize:    tokenBatchSize,
		tokenPersistFiles: tokenPersistFiles,
		rebuildEveryFiles: rebuildEveryFiles,
		stateDir:          stateDir,
		minCountThreshold: minCountThreshold,
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

// CheckAndTriggerInitialRebuild checks if the token counter has been populated with persisted data
// and triggers an immediate rebuild if it has, to avoid waiting for the full window
func (fct *FrequencyComputationThread) CheckAndTriggerInitialRebuild() {
	// Check if the token counter has been populated with persisted data
	tokensProcessed := fct.tokenCounter.GetTotalTokens()
	if tokensProcessed >= fct.windowSize {
		fmt.Printf("*** FCT: Token counter populated with persisted data, triggering immediate rebuild (tokens: %d >= window: %d) ***\n",
			tokensProcessed, fct.windowSize)
		slog.Info("FCT: Token counter already populated with persisted data, triggering immediate rebuild",
			"tokens_processed", tokensProcessed,
			"window_size", fct.windowSize)
		fct.TriggerRebuild()
	} else {
		slog.Info("FCT: Token counter not yet populated, will wait for full window to build filters",
			"tokens_processed", tokensProcessed,
			"window_size", fct.windowSize)
	}
}

// Stop gracefully stops the FCT goroutine
func (fct *FrequencyComputationThread) Stop() {
	close(fct.stopChan)
	fct.wg.Wait()
	slog.Info("FrequencyComputationThread stopped")
}

// GetTokensAddedToCleanupQueue returns the number of tokens added to cleanup queue since last call
// This is used by the main package to calculate dynamic safety buffer
func (fct *FrequencyComputationThread) GetTokensAddedToCleanupQueue() int64 {
	// This would need to be implemented to track tokens added during rebuilds
	// For now, we'll use the global counter approach
	return 0 // Placeholder - will be implemented via global counter
}

// TriggerRebuild signals that a frequency class rebuild is needed
// The FCT will handle the rebuild autonomously when it's ready
func (fct *FrequencyComputationThread) TriggerRebuild() {
	fct.shouldRebuild = true
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
			shouldRebuild := fct.shouldRebuild

			// Debug: Log rebuild flag status more frequently
			if fct.loopCount%1000 == 0 {
				slog.Debug("FCT: Rebuild flag check",
					"loop_count", fct.loopCount,
					"should_rebuild", shouldRebuild)
			}

			// ALWAYS log when rebuild flag is true (this should be rare)
			if shouldRebuild {
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
				fct.rebuildCount++
				currentRebuildCount := fct.rebuildCount

				// Consume all accumulated tokens before starting computation
				fct.consumeAllAccumulatedTokens()

				rebuildStartTime := time.Now()
				fmt.Fprintf(os.Stderr, "*** FCT REBUILD STARTED at %s ***\n", rebuildStartTime.Format("15:04:05"))
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
			if tokensProcessed >= fct.windowSize && !fct.shouldRebuild {
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
		slog.Debug("FCT writing token file",
			"tokens_since_last_write", fct.tokensSinceLastWrite,
			"token_batch_size", fct.tokenBatchSize)
		// Write tokens to file (only the batch size worth)
		tokensForFile := fct.accumulatedTokens[:fct.tokenBatchSize]
		if err := fct.writeTokenFile(tokensForFile); err != nil {
			slog.Error("Failed to write token file", "error", err)
		} else {
			slog.Debug("Token file written successfully",
				"tokens_written", len(tokensForFile),
				"file_counter", fct.tokenFileCounter-1)
		}

		// Reset counter and remaining tokens
		fct.tokensSinceLastWrite = 0
		fct.accumulatedTokens = fct.accumulatedTokens[fct.tokenBatchSize:]

		// If we have remaining tokens, write them too (should be rare)
		for len(fct.accumulatedTokens) >= fct.tokenBatchSize {
			tokensForFile = fct.accumulatedTokens[:fct.tokenBatchSize]
			slog.Debug("FCT writing additional token file",
				"accumulated_tokens", len(fct.accumulatedTokens),
				"token_batch_size", fct.tokenBatchSize)
			if err := fct.writeTokenFile(tokensForFile); err != nil {
				slog.Error("Failed to write additional token file", "error", err)
			} else {
				slog.Debug("Additional token file written successfully",
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
			slog.Debug("FCT processed tokens",
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
	fct.shouldRebuild = false

	startTime := time.Now()

	// Get current token file count for logging
	tokenFileCount := fct.tokenFileCounter

	slog.Debug("FCT rebuild starting",
		"start_time", startTime.Format("15:04:05"),
		"token_files_written", tokenFileCount)

	// Get a snapshot of current token counts for frequency calculations
	tokenCounts := fct.tokenCounter.CountsSnapshot()

	// Perform the frequency class calculation

	// Use adaptive frequency class building if min count threshold is configured
	var result FreqClassResult
	if fct.minCountThreshold > 0 {
		result = BuildFrequencyClassHashSetsAdaptive(tokenCounts, fct.freqClasses, fct.minCountThreshold)
	} else {
		result = BuildFrequencyClassHashSets(tokenCounts, fct.freqClasses)
	}

	slog.Info("FCT: BuildFrequencyClassHashSets returned", "filters_built", len(result.Filters))

	// Update dynamic cleanup safety buffer based on actual token obsolescence rate
	// Calculate tokens added to cleanup queue during this rebuild
	tokensAdded := fct.tokenCounter.GetTokensAddedToCleanupQueue()
	if tokensAdded > 0 {
		// Calculate new safety buffer: tokens_added * window_batches * 1.5 safety factor
		newSafetyBuffer := int64(float64(tokensAdded) * float64(fct.windowBatches) * 1.5)

		// Update the global dynamic safety buffer
		atomic.StoreInt64(&globalDynamicCleanupLeaveAtLeast, newSafetyBuffer)

		slog.Info("Updated dynamic cleanup safety buffer",
			"tokens_added_during_rebuild", tokensAdded,
			"window_batches", fct.windowBatches,
			"new_safety_buffer", newSafetyBuffer)
	}

	// Clear the token counter after frequency calculation
	// This ensures each rebuild is based only on the current window's tokens
	fct.tokenCounter.Clear()

	// Install filters globally for main thread
	slog.Info("Installing filters globally")
	SetGlobalFilters(result.Filters)
	slog.Info("Filters installed globally", "num_filters", len(result.Filters))

	// Save the data structures to files
	savePersistedState(result, tokenCounts)

	duration := time.Since(startTime)
	completionTime := time.Now()

	fmt.Fprintf(os.Stderr, "*** FCT REBUILD COMPLETED at %s (duration: %v, token files written: %d) ***\n",
		completionTime.Format("15:04:05"), duration, tokenFileCount)

	slog.Info("Frequency class rebuild completed",
		"duration", duration,
		"completion_time", completionTime.Format("15:04:05"),
		"token_count", len(tokenCounts),
		"filters_built", len(result.Filters),
		"token_files_written", tokenFileCount)

	// Add diagnostic line to show filters are installed
	slog.Info("Frequency class filters are now active",
		"num_filters", len(result.Filters),
		"completion_time", completionTime.Format("15:04:05"),
		"duration", duration,
		"token_files_written", tokenFileCount)

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

// GetRebuildCount returns the number of rebuilds performed (for debugging)
func (fct *FrequencyComputationThread) GetRebuildCount() int {
	return fct.rebuildCount
}

// LoadState loads persisted token counts into the FCT's internal state
func (fct *FrequencyComputationThread) LoadState(counts map[string]int) {
	fct.tokenCounter.SetCountsDirectly(counts)
	slog.Info("FCT loaded state", "distinct_tokens", len(counts))
}

// GetStats returns statistics about the FCT's current state
func (fct *FrequencyComputationThread) GetStats() map[string]int {
	return map[string]int{
		"distinct_tokens": len(fct.tokenCounter.Counts()),
		"total_tokens":    fct.tokenCounter.GetTotalTokens(),
		"rebuild_count":   fct.rebuildCount,
	}
}

// consumeAllAccumulatedTokens processes tokens from both queues using queue size snapshots
func (fct *FrequencyComputationThread) consumeAllAccumulatedTokens() {
	// Get current queue sizes (snapshot to prevent infinite catch-up)
	inboundQueueSize := fct.inboundTokenQueue.Len()

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

	// Save ThreePartKey mappings - COMMENTED OUT for on-the-fly generation test
	// threePKPath := filepath.Join(stateDir, "threepartkey_mappings.json")
	// if err := saveThreePartKeyMappingsToFile(threePKPath, TokenTo3PK); err != nil {
	// 	slog.Error("Failed to save ThreePartKey mappings", "error", err, "path", threePKPath)
	// } else {
	// 	slog.Info("ThreePartKey mappings saved", "num_mappings", len(TokenTo3PK))
	// }

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
				slog.Debug("Deleted old token file", "file", oldestFile)
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

	slog.Debug("Decremented tokens from file", "file", filename, "token_count", len(tokens))
	return nil
}
