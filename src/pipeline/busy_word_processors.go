package pipeline

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"cursor-twitter/src/tweets"
	"log/slog"
)

// ThreePartKeyQueue is a thread-safe queue for ThreePartKeys
type ThreePartKeyQueue struct {
	items []tweets.ThreePartKey
	mu    sync.Mutex
}

// NewThreePartKeyQueue creates a new ThreePartKeyQueue
func NewThreePartKeyQueue() *ThreePartKeyQueue {
	return &ThreePartKeyQueue{
		items: make([]tweets.ThreePartKey, 0),
	}
}

// Enqueue adds ThreePartKeys to the queue
func (q *ThreePartKeyQueue) Enqueue(keys []tweets.ThreePartKey) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = append(q.items, keys...)
}

// Dequeue removes and returns ThreePartKeys from the queue
func (q *ThreePartKeyQueue) Dequeue() []tweets.ThreePartKey {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 {
		return nil
	}

	// Return up to 100 keys at a time
	batchSize := 100
	if len(q.items) < batchSize {
		batchSize = len(q.items)
	}

	result := q.items[:batchSize]
	q.items = q.items[batchSize:]
	return result
}

// Len returns the number of items in the queue
func (q *ThreePartKeyQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// BatchResult holds the busy words found by a processor in one batch
type BatchResult struct {
	ClassIndex int
	BusyWords  []tweets.ThreePartKey
	WordCount  int
	Error      error
	Timestamp  time.Time
}

// BatchSummary holds the complete results from all frequency classes
type BatchSummary struct {
	BatchNumber    int
	TotalBusyWords int
	ClassResults   map[int][]string // class index -> list of busy words
	Timestamp      time.Time
}

// FrequencyClassProcessor manages queues and processors for each frequency class
type FrequencyClassProcessor struct {
	queues     []*ThreePartKeyQueue
	processors []*BusyWordProcessor
	numClasses int
	stopChan   chan struct{}
	wg         sync.WaitGroup

	// Batch coordination
	batchResults chan BatchResult
	batchBarrier *Barrier
	batchMutex   sync.Mutex
	batchActive  bool
	batchNumber  int

	// Global token mapping reference (set from main.go)
	globalTokenMapping map[tweets.ThreePartKey]string
	globalMappingMutex *sync.RWMutex
}

// Barrier implements thread coordination barrier pattern
type Barrier struct {
	mu      sync.Mutex
	count   int
	waiting int
	cond    *sync.Cond
}

// NewBarrier creates a new barrier for N threads
func NewBarrier(count int) *Barrier {
	b := &Barrier{count: count}
	b.cond = sync.NewCond(&b.mu)
	return b
}

// Wait blocks until all N threads have called Wait()
func (b *Barrier) Wait() {
	b.mu.Lock()
	b.waiting++

	if b.waiting < b.count {
		// Not all threads have reached the barrier yet
		b.cond.Wait()
	} else {
		// All threads have reached the barrier, wake everyone up
		b.cond.Broadcast()
	}

	b.mu.Unlock()
}

// Reset resets the barrier for the next cycle
func (b *Barrier) Reset() {
	b.mu.Lock()
	b.waiting = 0
	b.mu.Unlock()
}

// IsComplete returns true if all threads have reached the barrier
func (b *Barrier) IsComplete() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.waiting == b.count
}

// BusyWordProcessor processes ThreePartKeys for a specific frequency class
type BusyWordProcessor struct {
	classIndex int
	queue      *ThreePartKeyQueue
	stopChan   chan struct{}
	wg         sync.WaitGroup
	tokenCount int
	mutex      sync.Mutex

	// Counter arrays for the three parts of 3PK
	part1Counters []int
	part2Counters []int
	part3Counters []int
	arrayLen      int

	// Configuration
	zScoreThreshold float64

	// Global token mapping reference (set from main.go)
	globalTokenMapping map[tweets.ThreePartKey]string
	globalMappingMutex *sync.RWMutex

	// Batch coordination
	freqClassProcessor *FrequencyClassProcessor
}

// NewFrequencyClassProcessor creates a new processor with the specified number of classes
func NewFrequencyClassProcessor(numClasses int, arrayLen int, zScoreThreshold float64) *FrequencyClassProcessor {
	queues := make([]*ThreePartKeyQueue, numClasses)
	processors := make([]*BusyWordProcessor, numClasses)

	fcp := &FrequencyClassProcessor{
		queues:       queues,
		processors:   processors,
		numClasses:   numClasses,
		stopChan:     make(chan struct{}),
		batchResults: make(chan BatchResult, numClasses), // Buffer for all processors
		batchBarrier: NewBarrier(numClasses),
	}

	for i := 0; i < numClasses; i++ {
		queues[i] = NewThreePartKeyQueue()
		processors[i] = NewBusyWordProcessor(i, queues[i], arrayLen, zScoreThreshold, fcp)
	}

	return fcp
}

// NewBusyWordProcessor creates a new busy word processor for a specific class
func NewBusyWordProcessor(classIndex int, queue *ThreePartKeyQueue, arrayLen int, zScoreThreshold float64, fcp *FrequencyClassProcessor) *BusyWordProcessor {
	// Initialize counter arrays to zero
	part1Counters := make([]int, arrayLen)
	part2Counters := make([]int, arrayLen)
	part3Counters := make([]int, arrayLen)

	return &BusyWordProcessor{
		classIndex:         classIndex,
		queue:              queue,
		stopChan:           make(chan struct{}),
		tokenCount:         0,
		part1Counters:      part1Counters,
		part2Counters:      part2Counters,
		part3Counters:      part3Counters,
		arrayLen:           arrayLen,
		zScoreThreshold:    zScoreThreshold,
		freqClassProcessor: fcp,
	}
}

// Start begins all the busy word processors
func (fcp *FrequencyClassProcessor) Start() {
	slog.Info("Starting FrequencyClassProcessor", "num_classes", fcp.numClasses)

	for i, processor := range fcp.processors {
		fcp.wg.Add(1)
		go func(p *BusyWordProcessor, classIdx int) {
			defer fcp.wg.Done()
			p.run()
		}(processor, i)
		slog.Info("Started BusyWordProcessor", "class_index", i)
	}

	// Start the batch result collector
	go fcp.collectBatchResults()
}

// Stop gracefully stops all processors
func (fcp *FrequencyClassProcessor) Stop() {
	close(fcp.stopChan)
	fcp.wg.Wait()
	slog.Info("FrequencyClassProcessor stopped")
}

// collectBatchResults collects and merges results from all processors
func (fcp *FrequencyClassProcessor) collectBatchResults() {
	// Track results for current batch
	currentBatch := make(map[int][]string)
	resultsReceived := 0

	for {
		select {
		case <-fcp.stopChan:
			return
		case result := <-fcp.batchResults:
			if result.Error != nil {
				slog.Error("Busy word processor error",
					"class_index", result.ClassIndex,
					"error", result.Error)
				continue
			}

			// Convert 3PKs to actual words
			busyWords := fcp.convert3PKsToWords(result.BusyWords)

			// Store results for this class
			currentBatch[result.ClassIndex] = busyWords
			resultsReceived++

			// Immediate feedback for this class (simplified)
			fmt.Printf("Class %d: %d busy words\n", result.ClassIndex, len(busyWords))

			// Check if all classes have reported
			if resultsReceived == fcp.numClasses {
				// Print batch summary
				fcp.printBatchSummary(currentBatch)

				// Reset for next batch
				currentBatch = make(map[int][]string)
				resultsReceived = 0
				fcp.batchNumber++
			}
		}
	}
}

// printBatchSummary prints a summary of all busy words found in this batch
func (fcp *FrequencyClassProcessor) printBatchSummary(classResults map[int][]string) {
	fmt.Printf("\n" + strings.Repeat("=", 60) + "\n")
	fmt.Printf("BATCH %d SUMMARY: %d frequency classes completed\n",
		fcp.batchNumber, fcp.numClasses)

	totalBusyWords := 0
	for classIndex, words := range classResults {
		totalBusyWords += len(words)
		fmt.Printf("Class %d: %d busy words\n", classIndex, len(words))
	}

	fmt.Printf("TOTAL: %d busy words\n", totalBusyWords)

	// Print busy words by class (only if there are any)
	for classIndex, words := range classResults {
		if len(words) > 0 {
			fmt.Printf("\nClass %d busy words:\n", classIndex)
			for i, word := range words {
				fmt.Printf("  %d. %s\n", i+1, word)
			}
		}
	}

	fmt.Printf(strings.Repeat("=", 60) + "\n\n")
}

// convert3PKsToWords converts ThreePartKeys back to actual word strings
func (fcp *FrequencyClassProcessor) convert3PKsToWords(threePKs []tweets.ThreePartKey) []string {
	words := make([]string, 0, len(threePKs))

	for _, threePK := range threePKs {
		// Get the word from the global token mapping
		if word, exists := fcp.getWordFrom3PK(threePK); exists {
			words = append(words, word)
		}
	}

	return words
}

// getWordFrom3PK safely retrieves a word from the global token mapping
func (fcp *FrequencyClassProcessor) getWordFrom3PK(threePK tweets.ThreePartKey) (string, bool) {
	if fcp.globalMappingMutex == nil || fcp.globalTokenMapping == nil {
		// Fallback to placeholder if mapping not available
		return fmt.Sprintf("word_%d_%d_%d", threePK.Part1, threePK.Part2, threePK.Part3), true
	}

	fcp.globalMappingMutex.RLock()
	defer fcp.globalMappingMutex.RUnlock()

	word, exists := fcp.globalTokenMapping[threePK]
	return word, exists
}

// EnqueueToFrequencyClass routes a ThreePartKey to the appropriate frequency class queue
func (fcp *FrequencyClassProcessor) EnqueueToFrequencyClass(classIndex int, key tweets.ThreePartKey) {
	if classIndex < 0 || classIndex >= fcp.numClasses {
		slog.Warn("Invalid frequency class index", "class_index", classIndex, "num_classes", fcp.numClasses)
		return
	}
	fcp.queues[classIndex].Enqueue([]tweets.ThreePartKey{key})

	// Debug: Log enqueuing occasionally
	queueSize := fcp.queues[classIndex].Len()
	if queueSize%1000 == 0 && queueSize > 0 {
		slog.Info("3pk enqueued to frequency class",
			"class_index", classIndex,
			"queue_size", queueSize,
			"three_pk", key)
	}
}

// EnqueueKeysToFrequencyClass routes multiple ThreePartKeys to a frequency class queue
func (fcp *FrequencyClassProcessor) EnqueueKeysToFrequencyClass(classIndex int, keys []tweets.ThreePartKey) {
	if classIndex < 0 || classIndex >= fcp.numClasses {
		slog.Warn("Invalid frequency class index", "class_index", classIndex, "num_classes", fcp.numClasses)
		return
	}
	fcp.queues[classIndex].Enqueue(keys)
}

// GetQueueStats returns statistics about all frequency class queues
func (fcp *FrequencyClassProcessor) GetQueueStats() map[string]int {
	stats := make(map[string]int)
	for i, queue := range fcp.queues {
		stats[fmt.Sprintf("freq_class_%d_queue_size", i)] = queue.Len()
	}
	return stats
}

// GetProcessorStats returns statistics about all busy word processors
func (fcp *FrequencyClassProcessor) GetProcessorStats() map[string]int {
	stats := make(map[string]int)
	for i, processor := range fcp.processors {
		stats[fmt.Sprintf("freq_class_%d_tokens_processed", i)] = processor.GetTokenCount()
	}
	return stats
}

// run is the main loop for a busy word processor
func (bwp *BusyWordProcessor) run() {
	defer bwp.wg.Done()

	slog.Info("BusyWordProcessor started", "class_index", bwp.classIndex, "array_len", bwp.arrayLen)

	loopCount := 0
	for {
		select {
		case <-bwp.stopChan:
			slog.Info("BusyWordProcessor stopping", "class_index", bwp.classIndex)
			return
		default:
			loopCount++

			// Try to get ThreePartKeys from the queue
			keys := bwp.queue.Dequeue()
			if keys == nil {
				// No keys available, wait a bit
				time.Sleep(1 * time.Millisecond)
				continue
			}

			// Process the ThreePartKeys
			for _, key := range keys {
				// Check for termination signal: (-1, -1, -1)
				if key.Part1 == -1 && key.Part2 == -1 && key.Part3 == -1 {
					// Perform coordinated z-computation
					bwp.performCoordinatedZComputation()
					continue
				}

				// Increment counters for each part of the 3PK
				index1 := key.Part1
				index2 := key.Part2
				index3 := key.Part3

				bwp.part1Counters[index1]++
				bwp.part2Counters[index2]++
				bwp.part3Counters[index3]++

				bwp.mutex.Lock()
				bwp.tokenCount++
				bwp.mutex.Unlock()
			}
		}
	}
}

// performCoordinatedZComputation performs z-computation and reports results to coordinator
func (bwp *BusyWordProcessor) performCoordinatedZComputation() {
	// fmt.Printf("*** BusyWordProcessor-%d: COORDINATED Z-COMPUTATION STARTED ***\n", bwp.classIndex)

	// Calculate statistics for each array
	part1Stats := bwp.calculateArrayStats(bwp.part1Counters)
	part2Stats := bwp.calculateArrayStats(bwp.part2Counters)
	part3Stats := bwp.calculateArrayStats(bwp.part3Counters)

	// Calculate z-scores and find high-scoring positions
	part1HighZScores := bwp.calculateZScores(bwp.part1Counters, part1Stats, bwp.zScoreThreshold)
	part2HighZScores := bwp.calculateZScores(bwp.part2Counters, part2Stats, bwp.zScoreThreshold)
	part3HighZScores := bwp.calculateZScores(bwp.part3Counters, part3Stats, bwp.zScoreThreshold)

	// Find busy words
	busyWords := bwp.findBusyWords(part1HighZScores, part2HighZScores, part3HighZScores)

	// Report results to coordinator
	result := BatchResult{
		ClassIndex: bwp.classIndex,
		BusyWords:  busyWords,
		WordCount:  len(busyWords),
		Error:      nil,
		Timestamp:  time.Now(),
	}

	bwp.freqClassProcessor.batchResults <- result

	// fmt.Printf("*** BusyWordProcessor-%d: COORDINATED Z-COMPUTATION COMPLETED ***\n", bwp.classIndex)
	// fmt.Printf("*** BusyWordProcessor-%d: Array totals - Part1: %d, Part2: %d, Part3: %d ***\n",
	// 	bwp.classIndex, part1Stats.total, part2Stats.total, part3Stats.total)
	// fmt.Printf("*** BusyWordProcessor-%d: Part1 stats - Mean: %.2f, StdDev: %.2f ***\n",
	// 	bwp.classIndex, part1Stats.mean, part1Stats.stdDev)
	// fmt.Printf("*** BusyWordProcessor-%d: Part2 stats - Mean: %.2f, StdDev: %.2f ***\n",
	// 	bwp.classIndex, part2Stats.mean, part2Stats.stdDev)
	// fmt.Printf("*** BusyWordProcessor-%d: Part3 stats - Mean: %.2f, StdDev: %.2f ***\n",
	// 	bwp.classIndex, part3Stats.mean, part3Stats.stdDev)
	// fmt.Printf("*** BusyWordProcessor-%d: High Z-scores (>=%.1f) - Part1: %d, Part2: %d, Part3: %d ***\n",
	// 	bwp.classIndex, bwp.zScoreThreshold, len(part1HighZScores), len(part2HighZScores), len(part3HighZScores))
	// fmt.Printf("*** BusyWordProcessor-%d: Busy words found: %d ***\n", bwp.classIndex, len(busyWords))

	// Wipe all counter arrays to zero
	for i := 0; i < bwp.arrayLen; i++ {
		bwp.part1Counters[i] = 0
		bwp.part2Counters[i] = 0
		bwp.part3Counters[i] = 0
	}

	// fmt.Printf("*** BusyWordProcessor-%d: Arrays reset, waiting at barrier ***\n", bwp.classIndex)

	// Wait at barrier until all processors have completed
	bwp.freqClassProcessor.batchBarrier.Wait()

	// Reset barrier for next cycle (only the last processor to reach barrier does this)
	bwp.freqClassProcessor.batchMutex.Lock()
	if bwp.freqClassProcessor.batchBarrier.IsComplete() {
		bwp.freqClassProcessor.batchBarrier.Reset()
		fmt.Printf("*** BARRIER RESET: All %d processors synchronized, starting next cycle ***\n",
			bwp.freqClassProcessor.numClasses)
	}
	bwp.freqClassProcessor.batchMutex.Unlock()

	// fmt.Printf("*** BusyWordProcessor-%d: Barrier cleared, starting next cycle ***\n", bwp.classIndex)

	// slog.Info("Coordinated z-computation completed and arrays reset",
	// 	"class_index", bwp.classIndex,
	// 	"array_len", bwp.arrayLen,
	// 	"part1_total", part1Stats.total,
	// 	"part2_total", part2Stats.total,
	// 	"part3_total", part3Stats.total,
	// 	"part1_mean", part1Stats.mean,
	// 	"part1_stddev", part1Stats.stdDev,
	// 	"part2_mean", part2Stats.mean,
	// 	"part2_stddev", part2Stats.stdDev,
	// 	"part3_mean", part3Stats.mean,
	// 	"part3_stddev", part3Stats.stdDev,
	// 	"z_score_threshold", bwp.zScoreThreshold,
	// 	"part1_high_z_scores", len(part1HighZScores),
	// 	"part2_high_z_scores", len(part2HighZScores),
	// 	"part3_high_z_scores", len(part3HighZScores))
}

// GetTokenCount returns the number of tokens processed by this processor
func (bwp *BusyWordProcessor) GetTokenCount() int {
	bwp.mutex.Lock()
	defer bwp.mutex.Unlock()
	return bwp.tokenCount
}

// performZComputation performs statistical analysis on the counter arrays
// Calculates z-scores using Gaussian statistics for each array position
func (bwp *BusyWordProcessor) performZComputation() {
	fmt.Printf("*** BusyWordProcessor-%d: Z-COMPUTATION STARTED ***\n", bwp.classIndex)

	// Calculate statistics for each array
	part1Stats := bwp.calculateArrayStats(bwp.part1Counters)
	part2Stats := bwp.calculateArrayStats(bwp.part2Counters)
	part3Stats := bwp.calculateArrayStats(bwp.part3Counters)

	// Calculate z-scores and find high-scoring positions
	part1HighZScores := bwp.calculateZScores(bwp.part1Counters, part1Stats, bwp.zScoreThreshold)
	part2HighZScores := bwp.calculateZScores(bwp.part2Counters, part2Stats, bwp.zScoreThreshold)
	part3HighZScores := bwp.calculateZScores(bwp.part3Counters, part3Stats, bwp.zScoreThreshold)

	// Take cartesian product of high z-score positions to generate candidate 3PKs
	// For each combination (pos1, pos2, pos3) where pos1 ∈ part1HighZScores, pos2 ∈ part2HighZScores, pos3 ∈ part3HighZScores
	// Generate 3PK and check if it exists in the global token mapping table
	// This will identify the "busy words" for this frequency class
	busyWords := bwp.findBusyWords(part1HighZScores, part2HighZScores, part3HighZScores)

	fmt.Printf("*** BusyWordProcessor-%d: Z-COMPUTATION COMPLETED ***\n", bwp.classIndex)
	fmt.Printf("*** BusyWordProcessor-%d: Array totals - Part1: %d, Part2: %d, Part3: %d ***\n",
		bwp.classIndex, part1Stats.total, part2Stats.total, part3Stats.total)
	fmt.Printf("*** BusyWordProcessor-%d: Part1 stats - Mean: %.2f, StdDev: %.2f ***\n",
		bwp.classIndex, part1Stats.mean, part1Stats.stdDev)
	fmt.Printf("*** BusyWordProcessor-%d: Part2 stats - Mean: %.2f, StdDev: %.2f ***\n",
		bwp.classIndex, part2Stats.mean, part2Stats.stdDev)
	fmt.Printf("*** BusyWordProcessor-%d: Part3 stats - Mean: %.2f, StdDev: %.2f ***\n",
		bwp.classIndex, part3Stats.mean, part3Stats.stdDev)
	fmt.Printf("*** BusyWordProcessor-%d: High Z-scores (>=%.1f) - Part1: %d, Part2: %d, Part3: %d ***\n",
		bwp.classIndex, bwp.zScoreThreshold, len(part1HighZScores), len(part2HighZScores), len(part3HighZScores))
	fmt.Printf("*** BusyWordProcessor-%d: Busy words found: %d ***\n", bwp.classIndex, len(busyWords))
	fmt.Printf("*** BusyWordProcessor-%d: Wiping counter arrays to zero ***\n", bwp.classIndex)

	// Wipe all counter arrays to zero
	for i := 0; i < bwp.arrayLen; i++ {
		bwp.part1Counters[i] = 0
		bwp.part2Counters[i] = 0
		bwp.part3Counters[i] = 0
	}

	fmt.Printf("*** BusyWordProcessor-%d: Arrays reset, ready for next batch ***\n", bwp.classIndex)

	slog.Info("Z-computation completed and arrays reset",
		"class_index", bwp.classIndex,
		"array_len", bwp.arrayLen,
		"part1_total", part1Stats.total,
		"part2_total", part2Stats.total,
		"part3_total", part3Stats.total,
		"part1_mean", part1Stats.mean,
		"part1_stddev", part1Stats.stdDev,
		"part2_mean", part2Stats.mean,
		"part2_stddev", part2Stats.stdDev,
		"part3_mean", part3Stats.mean,
		"part3_stddev", part3Stats.stdDev,
		"z_score_threshold", bwp.zScoreThreshold,
		"part1_high_z_scores", len(part1HighZScores),
		"part2_high_z_scores", len(part2HighZScores),
		"part3_high_z_scores", len(part3HighZScores))
}

// calculateZScores calculates z-scores for each position and returns set of high-scoring positions
// Also calculates min, max, and mean z-scores for threshold tuning
func (bwp *BusyWordProcessor) calculateZScores(counts []int, stats ArrayStats, threshold float64) map[int]float64 {
	highZScores := make(map[int]float64)

	// Avoid division by zero
	if stats.stdDev == 0 {
		return highZScores
	}

	// Calculate all z-scores and track statistics
	var allZScores []float64
	for i, count := range counts {
		zScore := (float64(count) - stats.mean) / stats.stdDev
		allZScores = append(allZScores, zScore)
		if zScore >= threshold {
			highZScores[i] = zScore
		}
	}

	// Calculate z-score statistics
	if len(allZScores) > 0 {
		minZ := allZScores[0]
		maxZ := allZScores[0]
		sumZ := 0.0

		for _, z := range allZScores {
			if z < minZ {
				minZ = z
			}
			if z > maxZ {
				maxZ = z
			}
			sumZ += z
		}
		meanZ := sumZ / float64(len(allZScores))

		fmt.Printf("*** BusyWordProcessor-%d: Z-score stats - Min: %.2f, Max: %.2f, Mean: %.2f ***\n",
			bwp.classIndex, minZ, maxZ, meanZ)
	}

	return highZScores
}

// ArrayStats holds statistical information about an array
type ArrayStats struct {
	total  int
	mean   float64
	stdDev float64
}

// calculateArrayStats calculates mean, standard deviation, and z-scores for an array
func (bwp *BusyWordProcessor) calculateArrayStats(counts []int) ArrayStats {
	// Calculate total and mean
	total := 0
	for _, count := range counts {
		total += count
	}
	mean := float64(total) / float64(bwp.arrayLen)

	// Calculate variance (sum of squared differences from mean)
	variance := 0.0
	for _, count := range counts {
		diff := float64(count) - mean
		variance += diff * diff
	}
	variance = variance / float64(bwp.arrayLen)

	// Calculate standard deviation
	stdDev := math.Sqrt(variance)

	return ArrayStats{
		total:  total,
		mean:   mean,
		stdDev: stdDev,
	}
}

// findBusyWords finds busy words from high z-score positions
func (bwp *BusyWordProcessor) findBusyWords(part1HighZScores, part2HighZScores, part3HighZScores map[int]float64) []tweets.ThreePartKey {
	busyWords := []tweets.ThreePartKey{}

	// Iterate over all combinations of high z-score positions
	for pos1, _ := range part1HighZScores {
		for pos2, _ := range part2HighZScores {
			for pos3, _ := range part3HighZScores {
				// Generate 3PK from high z-score positions
				key := tweets.ThreePartKey{
					Part1: pos1,
					Part2: pos2,
					Part3: pos3,
				}

				// Check if the generated 3PK exists in the global token mapping table
				if bwp.existsInGlobalTokenMapping(key) {
					busyWords = append(busyWords, key)
				}
			}
		}
	}

	return busyWords
}

// SetGlobalTokenMapping sets the reference to the global token mapping
func (bwp *BusyWordProcessor) SetGlobalTokenMapping(mapping map[tweets.ThreePartKey]string, mutex *sync.RWMutex) {
	bwp.globalTokenMapping = mapping
	bwp.globalMappingMutex = mutex
}

// existsInGlobalTokenMapping checks if a ThreePartKey exists in the global token mapping table
func (bwp *BusyWordProcessor) existsInGlobalTokenMapping(key tweets.ThreePartKey) bool {
	if bwp.globalMappingMutex == nil || bwp.globalTokenMapping == nil {
		return false
	}

	bwp.globalMappingMutex.RLock()
	defer bwp.globalMappingMutex.RUnlock()

	_, exists := bwp.globalTokenMapping[key]
	return exists
}

// SetGlobalTokenMappingForAll sets the global token mapping for all busy word processors
func (fcp *FrequencyClassProcessor) SetGlobalTokenMappingForAll(mapping map[tweets.ThreePartKey]string, mutex *sync.RWMutex) {
	// Set the mapping in the FrequencyClassProcessor itself
	fcp.globalTokenMapping = mapping
	fcp.globalMappingMutex = mutex

	// Also set it in all individual processors
	for _, processor := range fcp.processors {
		processor.SetGlobalTokenMapping(mapping, mutex)
	}
}
