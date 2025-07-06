package pipeline

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"log/slog"
)

// FrequencyComputationThread (FCT) manages token counting and frequency calculations
// in a separate goroutine to avoid blocking the main tweet processing pipeline.
type FrequencyComputationThread struct {
	// Token processing
	tokenCounter      *TokenCounter
	inboundTokenQueue *TokenQueue
	oldTokenQueue     *TokenQueue

	// Frequency calculation control
	shouldRebuild int32 // Atomic boolean (0 = false, 1 = true)

	// Thread control
	stopChan chan struct{}
	wg       sync.WaitGroup

	// Configuration
	freqClassInterval time.Duration
	freqClasses       int

	// Current filters (thread-safe access)
	currentFilters []FreqClassFilter
	filtersMutex   sync.RWMutex

	// Debug: Track rebuild count
	rebuildCount      int
	rebuildCountMutex sync.Mutex
}

// NewFrequencyComputationThread creates a new FCT with the specified configuration
func NewFrequencyComputationThread(
	tokenCounter *TokenCounter,
	inboundTokenQueue *TokenQueue,
	oldTokenQueue *TokenQueue,
	freqClassIntervalTweets int,
	freqClasses int,
) *FrequencyComputationThread {
	return &FrequencyComputationThread{
		tokenCounter:      tokenCounter,
		inboundTokenQueue: inboundTokenQueue,
		oldTokenQueue:     oldTokenQueue,
		stopChan:          make(chan struct{}),
		freqClassInterval: time.Duration(freqClassIntervalTweets) * time.Second, // Not used in current implementation
		freqClasses:       freqClasses,
	}
}

// Start begins the FCT goroutine
func (fct *FrequencyComputationThread) Start() {
	slog.Info("FCT Start method called")
	fct.wg.Add(1)
	go fct.run()
	slog.Info("FCT goroutine launched")
	slog.Info("FrequencyComputationThread started",
		"freq_class_interval_tweets", fct.freqClassInterval.Seconds(),
		"freq_classes", fct.freqClasses)
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

	loopCount := 0
	for {
		select {
		case <-fct.stopChan:
			slog.Info("FCT run loop stopping")
			return
		default:
			loopCount++
			if loopCount%10000 == 0 { // Log every 10,000 iterations
				slog.Info("FCT loop iteration", "count", loopCount)
			}
			if loopCount%100000 == 0 { // Log every 100,000 iterations to confirm it's running
				slog.Info("FCT: Loop is running, iteration", "count", loopCount)
			}

			// Check if rebuild is needed first
			shouldRebuild := atomic.LoadInt32(&fct.shouldRebuild) == 1

			// Debug: Log rebuild flag status more frequently
			if loopCount%1000 == 0 {
				slog.Info("FCT: Rebuild flag check",
					"loop_count", loopCount,
					"should_rebuild", shouldRebuild)
			}

			// ALWAYS log when rebuild flag is true (this should be rare)
			if shouldRebuild {
				slog.Info("FCT: REBUILD FLAG DETECTED!",
					"loop_count", loopCount,
					"should_rebuild", shouldRebuild)
			}

			// Debug: Log the branch we're taking more frequently
			if loopCount%1000 == 0 {
				if shouldRebuild {
					slog.Info("FCT: Taking rebuild branch", "loop_count", loopCount)
				} else {
					slog.Debug("FCT: Taking token processing branch", "loop_count", loopCount)
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
				oldSizeAfter := fct.oldTokenQueue.Len()
				slog.Info("FCT: Queue sizes after rebuild",
					"rebuild_count", currentRebuildCount,
					"inbound_queue_size", inboundSizeAfter,
					"old_queue_size", oldSizeAfter)

			}

			// Process a small amount of tokens (main loop body - always happens)
			func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("FCT processTokens panicked", "panic", r)
					}
				}()
				fct.processTokens()
			}()

			// Add a small delay when no tokens are being processed to prevent CPU spinning
			inboundSize := fct.inboundTokenQueue.Len()
			oldSize := fct.oldTokenQueue.Len()
			if inboundSize == 0 && oldSize == 0 {
				time.Sleep(1 * time.Millisecond)
			}

			// Debug: Log when queues are empty
			if loopCount%1000 == 0 && inboundSize == 0 && oldSize == 0 {
				slog.Debug("FCT: No tokens to process, waiting...",
					"loop_count", loopCount,
					"inbound_queue_size", inboundSize,
					"old_queue_size", oldSize)
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
	}

	// Process old tokens (decrement counts)
	oldProcessed := 0
	for {
		tokens := fct.oldTokenQueue.Dequeue()
		if tokens == nil {
			break // Queue is empty
		}
		fct.tokenCounter.DecrementTokens(tokens)
		oldProcessed += len(tokens)
	}

	// Only log if we processed tokens or if queues are getting backed up
	if inboundProcessed > 0 || oldProcessed > 0 {
		slog.Info("FCT processed tokens",
			"inbound_processed", inboundProcessed,
			"old_processed", oldProcessed,
			"inbound_queue_size_after", fct.inboundTokenQueue.Len(),
			"old_queue_size_after", fct.oldTokenQueue.Len())
	} else {
		// Check if queues are getting backed up but we didn't process anything
		currentInboundSize := fct.inboundTokenQueue.Len()
		currentOldSize := fct.oldTokenQueue.Len()
		if currentInboundSize > 100 || currentOldSize > 100 {
			slog.Warn("FCT queues have backlog but processed no tokens",
				"inbound_queue_size", currentInboundSize,
				"old_queue_size", currentOldSize)
		}
	}

	// Debug: Log every 1000 calls to processTokens to confirm it's being called
	// Note: This is a simple counter for debugging - in production this would be removed
	// or made thread-safe if needed
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
	SetGlobalFilters(result.Filters)

	duration := time.Since(startTime)
	completionTime := time.Now()
	slog.Info("Frequency class rebuild completed",
		"duration", duration,
		"completion_time", completionTime.Format("15:04:05"),
		"token_count", len(tokenCounts),
		"filters_built", len(result.Filters))

	// Also print to stdout for immediate visibility
	fmt.Printf("*** FREQUENCY REBUILD COMPLETED at %s (duration: %v) ***\n",
		completionTime.Format("15:04:05"), duration)

	// Add diagnostic line to show filters are installed
	slog.Info("INSTALLED: frequency class filters are now active",
		"num_filters", len(result.Filters))

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
		"old_token_queue_size":     fct.oldTokenQueue.Len(),
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
	oldQueueSize := fct.oldTokenQueue.Len()

	slog.Info("FCT: Starting token consumption with queue snapshot",
		"inbound_queue_size", inboundQueueSize,
		"old_queue_size", oldQueueSize)

	inboundProcessed := 0
	oldProcessed := 0

	// Process exactly inboundQueueSize tokens from inbound queue
	for i := 0; i < inboundQueueSize; i++ {
		tokens := fct.inboundTokenQueue.Dequeue()
		if tokens == nil {
			break // Queue is empty (shouldn't happen given our size check)
		}
		fct.tokenCounter.IncrementTokens(tokens)
		inboundProcessed += len(tokens)
	}

	// Process exactly oldQueueSize tokens from old queue
	for i := 0; i < oldQueueSize; i++ {
		tokens := fct.oldTokenQueue.Dequeue()
		if tokens == nil {
			break // Queue is empty (shouldn't happen given our size check)
		}
		fct.tokenCounter.DecrementTokens(tokens)
		oldProcessed += len(tokens)
	}

	slog.Info("FCT: Consumed tokens using queue snapshot",
		"inbound_processed", inboundProcessed,
		"old_processed", oldProcessed,
		"inbound_queue_size_after", fct.inboundTokenQueue.Len(),
		"old_queue_size_after", fct.oldTokenQueue.Len())
}
