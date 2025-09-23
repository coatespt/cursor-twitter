package pipeline

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cursor-twitter/src/tweets"
	"log/slog"
)

// BusyWordResult represents the output from a busy word processor
// Contains batch number, frequency class, list of busy word 3PKs, and z-score info
type BusyWordResult struct {
	BatchNumber    int64
	FrequencyClass int
	BusyWord3PKs   []tweets.ThreePartKey
	ZScoreInfo     string
	ZScores        []float64 // Min z-score for each busy word (parallel to BusyWord3PKs)
	Counts         []int     // Min count for each busy word (parallel to BusyWord3PKs)
	Mean           float64   // Mean count for this batch (same for all busy words)
}

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
	numClasses  int
	queues      []*ThreePartKeyQueue
	processors  []*BusyWordProcessor
	skipClasses map[int]bool
	stopChan    chan struct{}
	wg          sync.WaitGroup

	// CSV logging
	csvFile     *os.File
	csvWriter   *csv.Writer
	logDir      string
	startTime   time.Time
	batchNumber int64
	zScoreMin   float64

	// Coordination pattern: workers -> analytics -> workers
	Barrier        *Barrier    // Proper synchronization barrier
	classToBarrier map[int]int // Map class index to barrier index
}

// Barrier implements thread coordination barrier pattern
type Barrier struct {
	numProducers int
	results      []BusyWordResult
	submitted    int
	consumed     bool
	mu           sync.Mutex
	submitCond   *sync.Cond
	consumeCond  *sync.Cond
}

// NewBarrier creates a new barrier for N producers
func NewBarrier(numProducers int) *Barrier {
	b := &Barrier{
		numProducers: numProducers,
		results:      make([]BusyWordResult, numProducers),
		consumed:     true, // Start ready to accept first batch
	}
	b.submitCond = sync.NewCond(&b.mu)
	b.consumeCond = sync.NewCond(&b.mu)
	return b
}

// Submit a result from producer i. Blocks only if trying to submit next result before previous batch consumed.
func (b *Barrier) Submit(producerID int, result BusyWordResult) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Wait until previous batch has been consumed (only blocks on next submission)
	for !b.consumed {
		b.submitCond.Wait()
	}

	b.results[producerID] = result
	b.submitted++

	// If this completes the batch, mark as ready for consumption
	if b.submitted == b.numProducers {
		b.consumed = false
		b.consumeCond.Signal() // Wake up consumer
	}
}

// ConsumeBatch waits for all F producers to submit, then returns the batch.
// This marks the batch as consumed and allows producers to submit again.
func (b *Barrier) ConsumeBatch() []BusyWordResult {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Wait until all producers have submitted
	for b.submitted < b.numProducers {
		b.consumeCond.Wait()
	}

	// Copy the results
	batch := make([]BusyWordResult, len(b.results))
	copy(batch, b.results)

	// Reset for next batch
	b.submitted = 0
	b.consumed = true

	// Wake up any producers waiting to submit next batch
	b.submitCond.Broadcast()

	return batch
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

	// Batch coordination
	freqClassProcessor *FrequencyClassProcessor
}

// NewFrequencyClassProcessor creates a new processor with the specified number of classes
func NewFrequencyClassProcessor(numClasses int, arrayLen int, zScoreThreshold float64, skipClasses []int, logDir string) *FrequencyClassProcessor {
	// Create array with single z-score for all classes (legacy support)
	zScores := make([]float64, numClasses)
	for i := 0; i < numClasses; i++ {
		zScores[i] = zScoreThreshold
	}
	return NewFrequencyClassProcessorWithZScores(numClasses, arrayLen, zScores, skipClasses, logDir)
}

// NewFrequencyClassProcessorWithZScores creates a new processor with per-class z-scores
func NewFrequencyClassProcessorWithZScores(numClasses int, arrayLen int, zScores []float64, skipClasses []int, logDir string) *FrequencyClassProcessor {
	queues := make([]*ThreePartKeyQueue, numClasses)
	processors := make([]*BusyWordProcessor, numClasses)

	// Create a map for quick lookup of skipped classes
	skipMap := make(map[int]bool)
	for _, class := range skipClasses {
		skipMap[class] = true
	}

	// Use the first z-score as the default for logging (legacy compatibility)
	zScoreMin := 0.0
	if len(zScores) > 0 {
		zScoreMin = zScores[0]
	}

	// Count active classes (non-skipped) and create mapping
	activeClasses := 0
	classToBarrier := make(map[int]int)
	for i := 0; i < numClasses; i++ {
		if !skipMap[i] {
			classToBarrier[i] = activeClasses
			activeClasses++
		}
	}

	fcp := &FrequencyClassProcessor{
		numClasses:     numClasses,
		queues:         queues,
		processors:     processors,
		skipClasses:    skipMap,
		stopChan:       make(chan struct{}),
		Barrier:        NewBarrier(activeClasses), // Proper synchronization barrier
		classToBarrier: classToBarrier,            // Map class indices to barrier indices
		startTime:      time.Now(),
		zScoreMin:      zScoreMin,
		logDir:         logDir,
		batchNumber:    1, // Start batch numbering at 1, not 0
	}

	for i := 0; i < numClasses; i++ {
		queues[i] = NewThreePartKeyQueue()
		// Use class-specific z-score, fallback to first z-score if not available
		zScore := 0.0
		if i < len(zScores) {
			zScore = zScores[i]
		} else if len(zScores) > 0 {
			zScore = zScores[0]
		}
		processors[i] = NewBusyWordProcessor(i, queues[i], arrayLen, zScore, fcp)
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
	slog.Info("Starting FrequencyClassProcessor", "num_classes", fcp.numClasses, "skip_classes", fcp.skipClasses)

	// Create CSV file for busy word logging
	if err := fcp.createCSVFile(); err != nil {
		slog.Error("Failed to create CSV file for busy words", "error", err)
	}

	for i, processor := range fcp.processors {
		// Only start processors for active (non-skipped) classes
		if fcp.IsClassActive(i) {
			fcp.wg.Add(1)
			go func(p *BusyWordProcessor, classIdx int) {
				defer fcp.wg.Done()
				p.run()
			}(processor, i)
			slog.Info("Started BusyWordProcessor", "class_index", i)
		} else {
			slog.Info("Skipping BusyWordProcessor", "class_index", i)
		}
	}

	// No batch result collector needed - BWPs send directly to barrier
}

// Stop gracefully stops all processors
func (fcp *FrequencyClassProcessor) Stop() {
	close(fcp.stopChan)
	fcp.wg.Wait()

	// Close CSV file
	if fcp.csvFile != nil {
		fcp.csvFile.Close()
	}

	slog.Info("FrequencyClassProcessor stopped")
}

// createCSVFile creates a CSV file for logging busy words
func (fcp *FrequencyClassProcessor) createCSVFile() error {
	// Create filename with timestamp (removed z-score since we now have per-class z-scores)
	timestamp := fcp.startTime.Format("20060102_150405")
	filename := fmt.Sprintf("bw_%s.csv", timestamp)
	filepath := filepath.Join(fcp.logDir, filename)

	// Create the file
	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create CSV file %s: %v", filepath, err)
	}

	fcp.csvFile = file
	fcp.csvWriter = csv.NewWriter(file)

	// Write header
	header := []string{"Batch", "Class", "Tokens"}

	if err := fcp.csvWriter.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %v", err)
	}
	fcp.csvWriter.Flush()

	slog.Info("Created busy word CSV file", "file", filepath)
	return nil
}

// writeToCSV writes busy word results to the CSV file
func (fcp *FrequencyClassProcessor) writeToCSV(classResults map[int][]string) {
	if fcp.csvWriter == nil {
		return
	}

	// Write one row per class per batch
	for i := 0; i < fcp.numClasses; i++ {
		row := []string{
			fmt.Sprintf("%d", fcp.batchNumber),
			fmt.Sprintf("%d", i),
		}

		if words, exists := classResults[i]; exists && len(words) > 0 {
			// Join words with comma for CSV
			row = append(row, strings.Join(words, ","))
		} else {
			row = append(row, "")
		}

		if err := fcp.csvWriter.Write(row); err != nil {
			slog.Error("Failed to write to CSV", "error", err)
			return
		}
	}

	// Add an empty row after each batch for visual separation
	if err := fcp.csvWriter.Write([]string{}); err != nil {
		slog.Error("Failed to write empty row to CSV", "error", err)
		return
	}

	fcp.csvWriter.Flush()
}

// GetBatchNumber returns the current batch number
func (fcp *FrequencyClassProcessor) GetBatchNumber() int64 {
	return atomic.LoadInt64(&fcp.batchNumber)
}

// IncrementBatchNumber increments the batch number for the next batch
func (fcp *FrequencyClassProcessor) IncrementBatchNumber() {
	atomic.AddInt64(&fcp.batchNumber, 1)
}

// printBatchSummary prints a summary of all busy words found in this batch
func (fcp *FrequencyClassProcessor) printBatchSummary(classResults map[int][]string) {
	totalBusyWords := 0

	// Get sorted class indices to ensure consistent ordering
	classIndices := make([]int, 0, len(classResults))
	for classIndex := range classResults {
		classIndices = append(classIndices, classIndex)
	}
	sort.Ints(classIndices)

	// Log classes in sorted order
	for _, classIndex := range classIndices {
		words := classResults[classIndex]
		totalBusyWords += len(words)
		if len(words) > 0 {
		} else {
		}
	}

	// Write to CSV file
	fcp.writeToCSV(classResults)
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

// getWordFrom3PK retrieves a word from the pipeline's global 3PK mapping
func (fcp *FrequencyClassProcessor) getWordFrom3PK(threePK tweets.ThreePartKey) (string, bool) {
	// Use the global mapping from the pipeline package
	return GetWordFrom3PK(threePK)
}

// EnqueueToFrequencyClass routes a ThreePartKey to the appropriate frequency class queue
func (fcp *FrequencyClassProcessor) EnqueueToFrequencyClass(classIndex int, key tweets.ThreePartKey) {
	if classIndex < 0 || classIndex >= fcp.numClasses {
		slog.Warn("Invalid frequency class index", "class_index", classIndex, "num_classes", fcp.numClasses)
		return
	}
	fcp.queues[classIndex].Enqueue([]tweets.ThreePartKey{key})

	// Debug: Log enqueuing occasionally - REMOVED due to excessive noise
	// queueSize := fcp.queues[classIndex].Len()
	// if queueSize%1000 == 0 && queueSize > 0 {
	// 	slog.Debug("3pk enqueued to frequency class",
	// 		"class_index", classIndex,
	// 		"queue_size", queueSize,
	// 		"three_pk", key)
	// }
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

// IsClassActive returns true if the specified frequency class is not skipped
func (fcp *FrequencyClassProcessor) IsClassActive(classIndex int) bool {
	if classIndex < 0 || classIndex >= fcp.numClasses {
		return false
	}
	return !fcp.skipClasses[classIndex]
}

// GetProcessorStats returns statistics about all busy word processors
func (fcp *FrequencyClassProcessor) GetProcessorStats() map[string]int {
	stats := make(map[string]int)
	for i, processor := range fcp.processors {
		stats[fmt.Sprintf("freq_class_%d_tokens_processed", i)] = processor.GetTokenCount()
	}
	return stats
}

// ReleaseBarrier releases the barrier to allow processors to start the next batch
func (fcp *FrequencyClassProcessor) ReleaseBarrier() {
	// The new barrier pattern handles this automatically in ConsumeBatch()
	// No manual release needed - the barrier coordinates everything
}

// run is the main loop for a busy word processor
func (bwp *BusyWordProcessor) run() {
	defer bwp.wg.Done()

	slog.Info("BusyWordProcessor started", "class_index", bwp.classIndex, "array_len", bwp.arrayLen)
	slog.Info("Processor entering main loop", "class_index", bwp.classIndex)

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
					// Perform coordinated z-computation using batch number from termination signal
					// BWP received termination signal - logging removed for cleaner output
					bwp.performCoordinatedZComputation(key.BatchID)
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
func (bwp *BusyWordProcessor) performCoordinatedZComputation(batchNumber int64) {
	// Calculate statistics for each array
	part1Stats := bwp.CalculateArrayStats(bwp.part1Counters)
	part2Stats := bwp.CalculateArrayStats(bwp.part2Counters)
	part3Stats := bwp.CalculateArrayStats(bwp.part3Counters)

	// Calculate z-scores and find high-scoring positions
	part1HighZScores := bwp.CalculateZScores(bwp.part1Counters, part1Stats, bwp.zScoreThreshold)
	part2HighZScores := bwp.CalculateZScores(bwp.part2Counters, part2Stats, bwp.zScoreThreshold)
	part3HighZScores := bwp.CalculateZScores(bwp.part3Counters, part3Stats, bwp.zScoreThreshold)

	// Find busy words
	busyWords := bwp.FindBusyWords(part1HighZScores, part2HighZScores, part3HighZScores)

	// Compute statistical values for each busy word and populate arrays
	var zScores []float64
	var counts []int

	// Use part1 mean as representative mean (same for all busy words in this batch)
	mean := part1Stats.Mean

	for _, threePK := range busyWords {
		// Get z-scores for each part of the 3PK
		zScore1, hasZ1 := part1HighZScores[threePK.Part1]
		zScore2, hasZ2 := part2HighZScores[threePK.Part2]
		zScore3, hasZ3 := part3HighZScores[threePK.Part3]

		// Get actual counts for each part of the 3PK
		count1 := bwp.part1Counters[threePK.Part1]
		count2 := bwp.part2Counters[threePK.Part2]
		count3 := bwp.part3Counters[threePK.Part3]

		// Find minimum z-score (lowest = cleanest signal)
		var minZScore float64
		if hasZ1 && hasZ2 && hasZ3 {
			minZScore = zScore1
			if zScore2 < minZScore {
				minZScore = zScore2
			}
			if zScore3 < minZScore {
				minZScore = zScore3
			}
		} else {
			minZScore = -999.0 // Error indicator
		}

		// Find minimum count
		minCount := count1
		if count2 < minCount {
			minCount = count2
		}
		if count3 < minCount {
			minCount = count3
		}

		// Store in parallel arrays
		zScores = append(zScores, minZScore)
		counts = append(counts, minCount)

		// DEBUG: Print debug line
		word := "unknown"
		if wordStr, exists := GetWordFrom3PK(threePK); exists {
			word = wordStr
		}

		slog.Info("DEBUG: Busy word stats",
			"word", word,
			"class", bwp.classIndex,
			"batch", batchNumber,
			"3pk", fmt.Sprintf("(%d,%d,%d)", threePK.Part1, threePK.Part2, threePK.Part3),
			"z_scores", fmt.Sprintf("(%.2f,%.2f,%.2f)", zScore1, zScore2, zScore3),
			"counts", fmt.Sprintf("(%d,%d,%d)", count1, count2, count3),
			"min_z_score", minZScore,
			"min_count", minCount,
			"mean", mean)
	}

	// BusyWordProcessor z-computation completed - logging removed for cleaner output

	// Create z-score info string
	zScoreInfo := fmt.Sprintf("Class %d: part1_high=%d, part2_high=%d, part3_high=%d, threshold=%.2f",
		bwp.classIndex, len(part1HighZScores), len(part2HighZScores), len(part3HighZScores), bwp.zScoreThreshold)

	// Report results to coordinator
	result := BusyWordResult{
		BatchNumber:    batchNumber,
		FrequencyClass: bwp.classIndex,
		BusyWord3PKs:   busyWords,
		ZScoreInfo:     zScoreInfo,
		ZScores:        zScores,
		Counts:         counts,
		Mean:           mean,
	}

	// Only log if there are busy words found
	if len(result.BusyWord3PKs) > 0 {
		busyWordStrings := make([]string, 0, len(result.BusyWord3PKs))
		for _, threePK := range result.BusyWord3PKs {
			if word, exists := GetWordFrom3PK(threePK); exists {
				busyWordStrings = append(busyWordStrings, word)
			}
		}
		slog.Info("BWP found busy words", "class", bwp.classIndex, "batch", result.BatchNumber, "words", busyWordStrings)
	}

	// Wipe all counter arrays to zero
	for i := 0; i < bwp.arrayLen; i++ {
		bwp.part1Counters[i] = 0
		bwp.part2Counters[i] = 0
		bwp.part3Counters[i] = 0
	}

	// CRITICAL: Send result ONLY to the barrier - NEVER send to analytics thread directly
	// Submit result to barrier - this blocks until previous batch is consumed
	// CRITICAL: BWPs should ONLY send results to the barrier
	// and NEVER directly to the analytics thread
	barrierIndex, exists := bwp.freqClassProcessor.classToBarrier[bwp.classIndex]
	if !exists {
		slog.Error("BWP class index not found in barrier mapping", "class_index", bwp.classIndex, "batch", result.BatchNumber)
		return
	}
	bwp.freqClassProcessor.Barrier.Submit(barrierIndex, result)
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
	slog.Info("Z-computation started", "class_index", bwp.classIndex)

	// Calculate statistics for each array
	part1Stats := bwp.CalculateArrayStats(bwp.part1Counters)
	part2Stats := bwp.CalculateArrayStats(bwp.part2Counters)
	part3Stats := bwp.CalculateArrayStats(bwp.part3Counters)

	// Calculate z-scores and find high-scoring positions
	part1HighZScores := bwp.CalculateZScores(bwp.part1Counters, part1Stats, bwp.zScoreThreshold)
	part2HighZScores := bwp.CalculateZScores(bwp.part2Counters, part2Stats, bwp.zScoreThreshold)
	part3HighZScores := bwp.CalculateZScores(bwp.part3Counters, part3Stats, bwp.zScoreThreshold)

	// Take cartesian product of high z-score positions to generate candidate 3PKs
	// For each combination (pos1, pos2, pos3) where pos1 ∈ part1HighZScores, pos2 ∈ part2HighZScores, pos3 ∈ part3HighZScores
	// Generate 3PK and check if it exists in the global token mapping table
	// This will identify the "busy words" for this frequency class
	busyWords := bwp.FindBusyWords(part1HighZScores, part2HighZScores, part3HighZScores)

	slog.Info("Z-computation completed", "class_index", bwp.classIndex, "busy_words_found", len(busyWords))

	// Wipe all counter arrays to zero
	for i := 0; i < bwp.arrayLen; i++ {
		bwp.part1Counters[i] = 0
		bwp.part2Counters[i] = 0
		bwp.part3Counters[i] = 0
	}

	slog.Info("Z-computation completed and arrays reset",
		"class_index", bwp.classIndex,
		"array_len", bwp.arrayLen,
		"part1_total", part1Stats.Total,
		"part2_total", part2Stats.Total,
		"part3_total", part3Stats.Total,
		"part1_mean", part1Stats.Mean,
		"part1_stddev", part1Stats.StdDev,
		"part2_mean", part2Stats.Mean,
		"part2_stddev", part2Stats.StdDev,
		"part3_mean", part3Stats.Mean,
		"part3_stddev", part3Stats.StdDev,
		"z_score_threshold", bwp.zScoreThreshold,
		"part1_high_z_scores", len(part1HighZScores),
		"part2_high_z_scores", len(part2HighZScores),
		"part3_high_z_scores", len(part3HighZScores))
}

// CalculateZScores calculates z-scores for each position and returns set of high-scoring positions
// Also calculates min, max, and mean z-scores for threshold tuning
func (bwp *BusyWordProcessor) CalculateZScores(counts []int, stats ArrayStats, threshold float64) map[int]float64 {
	highZScores := make(map[int]float64)

	// Avoid division by zero
	if stats.StdDev == 0 {
		return highZScores
	}

	// Calculate all z-scores and track statistics
	var allZScores []float64
	for i, count := range counts {
		zScore := (float64(count) - stats.Mean) / stats.StdDev
		allZScores = append(allZScores, zScore)
		if zScore >= threshold {
			highZScores[i] = zScore
		}
	}

	// Calculate z-score statistics
	if len(allZScores) > 0 {
		minZ := allZScores[0]
		maxZ := allZScores[0]

		for _, z := range allZScores {
			if z < minZ {
				minZ = z
			}
			if z > maxZ {
				maxZ = z
			}
		}

		slog.Debug("Z-score stats", "class_index", bwp.classIndex, "min_z", minZ, "max_z", maxZ)
	}

	return highZScores
}

// ArrayStats holds statistical information about an array
type ArrayStats struct {
	Total  int
	Min    int
	Max    int
	Mean   float64
	StdDev float64
}

// CalculateArrayStats calculates mean, standard deviation, and z-scores for an array
func (bwp *BusyWordProcessor) CalculateArrayStats(counts []int) ArrayStats {
	if len(counts) == 0 {
		return ArrayStats{Total: 0, Min: 0, Max: 0, Mean: 0, StdDev: 0}
	}

	// Calculate total, min, max, and mean
	total := 0
	min := counts[0]
	max := counts[0]
	for _, count := range counts {
		total += count
		if count < min {
			min = count
		}
		if count > max {
			max = count
		}
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
		Total:  total,
		Min:    min,
		Max:    max,
		Mean:   mean,
		StdDev: stdDev,
	}
}

// FindBusyWords finds busy words from high z-score positions
func (bwp *BusyWordProcessor) FindBusyWords(part1HighZScores, part2HighZScores, part3HighZScores map[int]float64) []tweets.ThreePartKey {
	busyWords := []tweets.ThreePartKey{}

	totalCombinations := 0
	validCombinations := 0

	// Iterate over all combinations of high z-score positions
	for pos1, _ := range part1HighZScores {
		for pos2, _ := range part2HighZScores {
			for pos3, _ := range part3HighZScores {
				totalCombinations++

				// Generate 3PK from high z-score positions
				key := tweets.ThreePartKey{
					Part1: pos1,
					Part2: pos2,
					Part3: pos3,
				}

				// Check if the generated 3PK exists in the global token mapping table
				if bwp.existsInGlobalTokenMapping(key) {
					validCombinations++
					busyWords = append(busyWords, key)
				}
			}
		}
	}

	// Log filtering statistics (reduced verbosity)
	if totalCombinations > 0 {
		slog.Debug("3PK filtering completed",
			"class_index", bwp.classIndex,
			"valid_combinations", validCombinations,
			"total_combinations", totalCombinations,
			"valid_percentage", float64(validCombinations)/float64(totalCombinations)*100.0)
	}

	return busyWords
}

// existsInGlobalTokenMapping checks if a ThreePartKey exists in the pipeline's global mapping
// Returns true only if the 3PK already exists in the mapping
func (bwp *BusyWordProcessor) existsInGlobalTokenMapping(key tweets.ThreePartKey) bool {
	// Use the global mapping directly, not a copy
	Token3PKMutex.RLock()
	_, exists := ThreePKToToken[key]
	Token3PKMutex.RUnlock()

	return exists
}
