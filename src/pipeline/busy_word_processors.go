package pipeline

import (
	"fmt"
	"sync"
	"time"

	"log/slog"
)

// FrequencyClassProcessor manages queues and processors for each frequency class
type FrequencyClassProcessor struct {
	queues     []*TokenQueue
	processors []*BusyWordProcessor
	numClasses int
	stopChan   chan struct{}
	wg         sync.WaitGroup
}

// BusyWordProcessor processes tokens for a specific frequency class
type BusyWordProcessor struct {
	classIndex int
	queue      *TokenQueue
	stopChan   chan struct{}
	wg         sync.WaitGroup
	tokenCount int
	mutex      sync.Mutex
}

// NewFrequencyClassProcessor creates a new processor with the specified number of classes
func NewFrequencyClassProcessor(numClasses int) *FrequencyClassProcessor {
	queues := make([]*TokenQueue, numClasses)
	processors := make([]*BusyWordProcessor, numClasses)

	for i := 0; i < numClasses; i++ {
		queues[i] = NewTokenQueue()
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
func NewBusyWordProcessor(classIndex int, queue *TokenQueue) *BusyWordProcessor {
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

// EnqueueToFrequencyClass routes a token to the appropriate frequency class queue
func (fcp *FrequencyClassProcessor) EnqueueToFrequencyClass(classIndex int, token string) {
	if classIndex < 0 || classIndex >= fcp.numClasses {
		slog.Warn("Invalid frequency class index", "class_index", classIndex, "num_classes", fcp.numClasses)
		return
	}
	fcp.queues[classIndex].Enqueue([]string{token})
}

// EnqueueTokensToFrequencyClass routes multiple tokens to a frequency class queue
func (fcp *FrequencyClassProcessor) EnqueueTokensToFrequencyClass(classIndex int, tokens []string) {
	if classIndex < 0 || classIndex >= fcp.numClasses {
		slog.Warn("Invalid frequency class index", "class_index", classIndex, "num_classes", fcp.numClasses)
		return
	}
	fcp.queues[classIndex].Enqueue(tokens)
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

			// Try to get tokens from the queue
			tokens := bwp.queue.Dequeue()
			if tokens == nil {
				// No tokens available, wait a bit
				time.Sleep(1 * time.Millisecond)
				continue
			}

			// Process the tokens
			for range tokens {
				bwp.mutex.Lock()
				bwp.tokenCount++
				currentCount := bwp.tokenCount
				bwp.mutex.Unlock()

				// Log every 10000 tokens
				if currentCount%10000 == 0 {
					fmt.Printf("BusyWordProcessor-%d: Processed %d tokens\n", bwp.classIndex, currentCount)
					slog.Info("BusyWordProcessor milestone",
						"class_index", bwp.classIndex,
						"tokens_processed", currentCount)
				}
			}

			// Log when we process tokens
			if loopCount%1000 == 0 {
				slog.Debug("BusyWordProcessor processed batch",
					"class_index", bwp.classIndex,
					"batch_size", len(tokens),
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
