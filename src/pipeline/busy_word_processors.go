package pipeline

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cursor-twitter/src/tweets"
	"log/slog"
)

// BusyWordResult represents the output from a busy word processor
// Contains batch number, frequency class, and list of busy word 3PKs
type BusyWordResult struct {
	BatchNumber    int
	FrequencyClass int
	BusyWord3PKs   []tweets.ThreePartKey
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
	batchNumber int
	zScoreMin   float64

	// Batch results collection
	batchResults chan BusyWordResult

	// Analysis thread communication
	resultChannel chan BusyWordResult
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

	// Batch coordination
	freqClassProcessor *FrequencyClassProcessor
}

// NewFrequencyClassProcessor creates a new processor with the specified number of classes
func NewFrequencyClassProcessor(numClasses int, arrayLen int, zScoreThreshold float64, skipClasses []int, logDir string) *FrequencyClassProcessor {
	queues := make([]*ThreePartKeyQueue, numClasses)
	processors := make([]*BusyWordProcessor, numClasses)

	// Create a map for quick lookup of skipped classes
	skipMap := make(map[int]bool)
	for _, class := range skipClasses {
		skipMap[class] = true
	}

	fcp := &FrequencyClassProcessor{
		numClasses:    numClasses,
		queues:        queues,
		processors:    processors,
		skipClasses:   skipMap,
		stopChan:      make(chan struct{}),
		batchResults:  make(chan BusyWordResult, numClasses), // Buffer for all processors
		resultChannel: make(chan BusyWordResult, numClasses), // Buffer for analysis thread
		startTime:     time.Now(),
		zScoreMin:     zScoreThreshold,
		logDir:        logDir,
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

	// Start the batch result collector
	go fcp.collectBatchResults()
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
	// Create filename with timestamp and z-score
	timestamp := fcp.startTime.Format("20060102_150405")
	filename := fmt.Sprintf("bw_%s_%.1f.csv", timestamp, fcp.zScoreMin)
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

	fcp.csvWriter.Flush()
}

// collectBatchResults collects and merges results from all processors
func (fcp *FrequencyClassProcessor) collectBatchResults() {
	// Track results for current batch
	currentBatch := make(map[int][]string)
	resultsReceived := 0

	fmt.Printf("[BATCH_COLLECTOR] Batch collector started, waiting for results...\n")

	for {
		select {
		case <-fcp.stopChan:
			fmt.Printf("[BATCH_COLLECTOR] Batch collector stopping\n")
			return
		case result := <-fcp.batchResults:
			fmt.Printf("[BATCH_COLLECTOR] Received result from class %d: %d busy words\n",
				result.FrequencyClass, len(result.BusyWord3PKs))

			// Convert 3PKs to actual words
			busyWords := fcp.convert3PKsToWords(result.BusyWord3PKs)

			// Store results for this class
			currentBatch[result.FrequencyClass] = busyWords
			resultsReceived++

			// Send result to analysis thread
			fcp.resultChannel <- result

			// Immediate feedback for this class (simplified)
			fmt.Printf("Class %d: %d busy words\n", result.FrequencyClass, len(busyWords))

			// Check if all active classes have reported
			activeClassCount := fcp.numClasses - len(fcp.skipClasses)
			if resultsReceived == activeClassCount {
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
	totalBusyWords := 0

	// Print to console
	fmt.Printf("\n" + strings.Repeat("-", 60) + "\n")
	fmt.Printf("BATCH %d SUMMARY: %d frequency classes completed\n",
		fcp.batchNumber, fcp.numClasses)

	for classIndex, words := range classResults {
		totalBusyWords += len(words)
		if len(words) > 0 {
			// Print class count and busy words on the same line
			fmt.Printf("Class %d: %d busy words - %s\n", classIndex, len(words), strings.Join(words, ", "))
		} else {
			fmt.Printf("Class %d: %d busy words\n", classIndex, len(words))
		}
	}

	fmt.Printf("TOTAL: %d busy words\n", totalBusyWords)
	fmt.Printf(strings.Repeat("-", 60) + "\n\n")

	// Also log to file
	for classIndex, words := range classResults {
		slog.Info("Batch class results", "batch", fcp.batchNumber, "class", classIndex, "busy_words", len(words))
	}

	slog.Info("Batch summary completed",
		"batch", fcp.batchNumber,
		"num_classes", fcp.numClasses,
		"total_busy_words", totalBusyWords)

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

// GetResultChannel returns the channel for receiving busy word results
func (fcp *FrequencyClassProcessor) GetResultChannel() <-chan BusyWordResult {
	return fcp.resultChannel
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
					fmt.Printf("[BWP-%d] Received termination signal, starting z-computation\n", bwp.classIndex)
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
	fmt.Printf("[BWP-%d] COORDINATED Z-COMPUTATION STARTED\n", bwp.classIndex)

	// Calculate statistics for each array
	part1Stats := bwp.CalculateArrayStats(bwp.part1Counters)
	part2Stats := bwp.CalculateArrayStats(bwp.part2Counters)
	part3Stats := bwp.CalculateArrayStats(bwp.part3Counters)

	// Debug: Show counter totals
	fmt.Printf("[BWP-%d] Counter totals: part1=%d, part2=%d, part3=%d\n",
		bwp.classIndex, part1Stats.Total, part2Stats.Total, part3Stats.Total)

	// Calculate z-scores and find high-scoring positions
	part1HighZScores := bwp.CalculateZScores(bwp.part1Counters, part1Stats, bwp.zScoreThreshold)
	part2HighZScores := bwp.CalculateZScores(bwp.part2Counters, part2Stats, bwp.zScoreThreshold)
	part3HighZScores := bwp.CalculateZScores(bwp.part3Counters, part3Stats, bwp.zScoreThreshold)

	// Debug: Show highest z-scores found
	maxZ1 := 0.0
	maxZ2 := 0.0
	maxZ3 := 0.0
	for _, z := range part1HighZScores {
		if z > maxZ1 {
			maxZ1 = z
		}
	}
	for _, z := range part2HighZScores {
		if z > maxZ2 {
			maxZ2 = z
		}
	}
	for _, z := range part3HighZScores {
		if z > maxZ3 {
			maxZ3 = z
		}
	}
	fmt.Printf("[BWP-%d] Highest z-scores: part1=%.2f, part2=%.2f, part3=%.2f (threshold=%.2f)\n",
		bwp.classIndex, maxZ1, maxZ2, maxZ3, bwp.zScoreThreshold)

	// Find busy words
	busyWords := bwp.FindBusyWords(part1HighZScores, part2HighZScores, part3HighZScores)

	fmt.Printf("[BWP-%d] Found %d busy words\n", bwp.classIndex, len(busyWords))

	// Wipe all counter arrays to zero
	for i := 0; i < bwp.arrayLen; i++ {
		bwp.part1Counters[i] = 0
		bwp.part2Counters[i] = 0
		bwp.part3Counters[i] = 0
	}

	// Report results to coordinator
	result := BusyWordResult{
		BatchNumber:    bwp.freqClassProcessor.batchNumber,
		FrequencyClass: bwp.classIndex,
		BusyWord3PKs:   busyWords,
	}

	// Send result to batch coordinator
	fmt.Printf("[BWP-%d] Sending result to batch coordinator\n", bwp.classIndex)
	bwp.freqClassProcessor.batchResults <- result
	fmt.Printf("[BWP-%d] Result sent successfully\n", bwp.classIndex)

	slog.Info("Coordinated z-computation completed and arrays reset",
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

	slog.Info("Arrays reset, ready for next batch", "class_index", bwp.classIndex)

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

		slog.Info("Z-score stats", "class_index", bwp.classIndex, "min_z", minZ, "max_z", maxZ)
	}

	return highZScores
}

// ArrayStats holds statistical information about an array
type ArrayStats struct {
	Total  int
	Mean   float64
	StdDev float64
}

// CalculateArrayStats calculates mean, standard deviation, and z-scores for an array
func (bwp *BusyWordProcessor) CalculateArrayStats(counts []int) ArrayStats {
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
		Total:  total,
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

	// Debug: Show filtering statistics
	if totalCombinations > 0 {
		fmt.Printf("[BWP-%d] 3PK filtering: %d/%d combinations valid (%.1f%%)\n",
			bwp.classIndex, validCombinations, totalCombinations,
			float64(validCombinations)/float64(totalCombinations)*100.0)

		// Show a few example 3PKs that were checked
		exampleCount := 0
		for pos1, _ := range part1HighZScores {
			for pos2, _ := range part2HighZScores {
				for pos3, _ := range part3HighZScores {
					if exampleCount < 3 {
						key := tweets.ThreePartKey{Part1: pos1, Part2: pos2, Part3: pos3}
						exists := bwp.existsInGlobalTokenMapping(key)
						fmt.Printf("[BWP-%d] Example 3PK: %v exists=%v\n", bwp.classIndex, key, exists)
						exampleCount++
					}
				}
			}
		}
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
