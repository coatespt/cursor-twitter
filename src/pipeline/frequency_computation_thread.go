package pipeline

import (
	"sync"
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
	shouldRebuild   bool
	rebuildMutex    sync.RWMutex
	rebuildComplete chan struct{}

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
		rebuildComplete:   make(chan struct{}, 1), // Buffered to avoid blocking
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
// Returns true if the rebuild was triggered, false if one was already in progress
func (fct *FrequencyComputationThread) TriggerRebuild() bool {
	fct.rebuildMutex.Lock()
	defer fct.rebuildMutex.Unlock()

	if fct.shouldRebuild {
		slog.Info("FCT: Rebuild flag already set, skipping trigger")
		return false // Already triggered
	}

	fct.shouldRebuild = true
	slog.Info("FCT: Rebuild flag SET - frequency boundary crossed")
	return true
}

// WaitForRebuildComplete blocks until the current rebuild is complete
func (fct *FrequencyComputationThread) WaitForRebuildComplete() {
	select {
	case <-fct.rebuildComplete:
		// Rebuild completed
	case <-time.After(30 * time.Second):
		slog.Warn("Timeout waiting for frequency rebuild to complete")
	}
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

			// Check if rebuild is needed first
			fct.rebuildMutex.RLock()
			shouldRebuild := fct.shouldRebuild
			fct.rebuildMutex.RUnlock()

			// Debug: Log the branch we're taking
			if loopCount%10000 == 0 {
				if shouldRebuild {
					slog.Debug("FCT: Taking rebuild branch", "loop_count", loopCount)
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

				slog.Info("FCT: Starting rebuild", "rebuild_count", currentRebuildCount)
				// Pause token processing and do rebuild
				fct.performRebuild()

				// Signal completion
				select {
				case fct.rebuildComplete <- struct{}{}:
				default:
					// Channel is full, which means someone is already waiting
				}

				// Reset the rebuild flag
				fct.rebuildMutex.Lock()
				fct.shouldRebuild = false
				fct.rebuildMutex.Unlock()
				slog.Info("FCT: Rebuild flag RESET - returning to token processing", "rebuild_count", currentRebuildCount)

				// Debug: Log queue sizes after rebuild to confirm we can process tokens
				inboundSizeAfter := fct.inboundTokenQueue.Len()
				oldSizeAfter := fct.oldTokenQueue.Len()
				slog.Info("FCT: Queue sizes after rebuild",
					"rebuild_count", currentRebuildCount,
					"inbound_queue_size", inboundSizeAfter,
					"old_queue_size", oldSizeAfter)
			} else {
				// Process tokens continuously
				inboundSize := fct.inboundTokenQueue.Len()
				oldSize := fct.oldTokenQueue.Len()
				if inboundSize > 0 || oldSize > 0 {
					slog.Info("FCT: Processing tokens", "inbound_queue_size", inboundSize, "old_queue_size", oldSize)
				}
				func() {
					defer func() {
						if r := recover(); r != nil {
							slog.Error("FCT processTokens panicked", "panic", r)
						}
					}()
					fct.processTokens()
				}()

				// Add a small delay when no tokens are being processed to prevent CPU spinning
				if inboundSize == 0 && oldSize == 0 {
					time.Sleep(1 * time.Millisecond)
				}

				// Debug: Log when we're in the token processing branch but queues are empty
				if loopCount%1000 == 0 && inboundSize == 0 && oldSize == 0 {
					slog.Debug("FCT: No tokens to process, waiting...",
						"loop_count", loopCount,
						"inbound_queue_size", inboundSize,
						"old_queue_size", oldSize)
				}
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
	slog.Info("Frequency class rebuild completed",
		"duration", duration,
		"token_count", len(tokenCounts),
		"filters_built", len(result.Filters))

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
