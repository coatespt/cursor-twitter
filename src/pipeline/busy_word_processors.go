package pipeline

import (
	"fmt"
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

// FrequencyClassProcessor manages queues and processors for each frequency class
type FrequencyClassProcessor struct {
	queues     []*ThreePartKeyQueue
	processors []*BusyWordProcessor
	numClasses int
	stopChan   chan struct{}
	wg         sync.WaitGroup
}

// BusyWordProcessor processes ThreePartKeys for a specific frequency class
type BusyWordProcessor struct {
	classIndex int
	queue      *ThreePartKeyQueue
	stopChan   chan struct{}
	wg         sync.WaitGroup
	tokenCount int
	mutex      sync.Mutex
}

// NewFrequencyClassProcessor creates a new processor with the specified number of classes
func NewFrequencyClassProcessor(numClasses int) *FrequencyClassProcessor {
	queues := make([]*ThreePartKeyQueue, numClasses)
	processors := make([]*BusyWordProcessor, numClasses)

	for i := 0; i < numClasses; i++ {
		queues[i] = NewThreePartKeyQueue()
		processors[i] = NewBusyWordProcessor(i, queues[i])
	}

	return &FrequencyClassProcessor{
		queues:     queues,
		processors: processors,
		numClasses: numClasses,
		stopChan:   make(chan struct{}),
	}
}

// NewBusyWordProcessor creates a new busy word processor for a specific class
func NewBusyWordProcessor(classIndex int, queue *ThreePartKeyQueue) *BusyWordProcessor {
	return &BusyWordProcessor{
		classIndex: classIndex,
		queue:      queue,
		stopChan:   make(chan struct{}),
		tokenCount: 0,
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
}

// Stop gracefully stops all processors
func (fcp *FrequencyClassProcessor) Stop() {
	close(fcp.stopChan)
	fcp.wg.Wait()
	slog.Info("FrequencyClassProcessor stopped")
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

	slog.Info("BusyWordProcessor started", "class_index", bwp.classIndex)

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

				// Debug: Log when queue is empty occasionally
				if loopCount%10000 == 0 {
					slog.Debug("BusyWordProcessor queue empty",
						"class_index", bwp.classIndex,
						"loop_count", loopCount)
				}
				continue
			}

			// Process the ThreePartKeys
			for range keys {
				bwp.mutex.Lock()
				bwp.tokenCount++
				currentCount := bwp.tokenCount
				bwp.mutex.Unlock()

				// Log every 10000 tokens
				if currentCount%10000 == 0 {
					fmt.Printf("BusyWordProcessor-%d: Processed %d 3pks\n", bwp.classIndex, currentCount)
					slog.Info("BusyWordProcessor milestone",
						"class_index", bwp.classIndex,
						"tokens_processed", currentCount)
				}
			}

			// Log when we process tokens
			if loopCount%1000 == 0 {
				slog.Debug("BusyWordProcessor processed batch",
					"class_index", bwp.classIndex,
					"batch_size", len(keys),
					"total_processed", bwp.GetTokenCount())
			}
		}
	}
}

// GetTokenCount returns the number of tokens processed by this processor
func (bwp *BusyWordProcessor) GetTokenCount() int {
	bwp.mutex.Lock()
	defer bwp.mutex.Unlock()
	return bwp.tokenCount
}
