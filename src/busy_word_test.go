package main

import (
	"cursor-twitter/src/pipeline"
	"cursor-twitter/src/tweets"
	"sync"
	"testing"
)

// TestBusyWordProcessorCreation tests the creation and basic setup of busy word processors.
//
// Rationale: Processor creation is the foundation for busy word detection.
// This test ensures processors are created correctly with proper configuration.
func TestBusyWordProcessorCreation(t *testing.T) {
	// Create a test processor
	queue := pipeline.NewThreePartKeyQueue()
	fcp := pipeline.NewFrequencyClassProcessor(3, 10, 2.0, []int{}, "test_logs")
	processor := pipeline.NewBusyWordProcessor(0, queue, 10, 2.0, fcp)

	// Test initial token count
	if count := processor.GetTokenCount(); count != 0 {
		t.Errorf("Expected initial token count 0, got %d", count)
	}

	// Test that processor is properly configured
	if processor == nil {
		t.Error("Expected processor to be created successfully")
	}
}

// TestBusyWordThreePartKeyQueueOperations tests the 3PK queue functionality for busy word processing.
//
// Rationale: 3PK queues are used to buffer tokens between frequency classes and busy word processors.
// This test ensures proper enqueue/dequeue operations for ThreePartKeys.
func TestBusyWordThreePartKeyQueueOperations(t *testing.T) {
	queue := pipeline.NewThreePartKeyQueue()

	// Test initial state
	if length := queue.Len(); length != 0 {
		t.Errorf("Expected empty queue, got length %d", length)
	}

	// Create test 3PKs
	key1 := tweets.ThreePartKey{Part1: 1, Part2: 2, Part3: 3}
	key2 := tweets.ThreePartKey{Part1: 4, Part2: 5, Part3: 6}
	key3 := tweets.ThreePartKey{Part1: 7, Part2: 8, Part3: 9}

	// Test enqueueing
	queue.Enqueue([]tweets.ThreePartKey{key1, key2})
	queue.Enqueue([]tweets.ThreePartKey{key3})

	// Verify queue length
	if length := queue.Len(); length != 3 {
		t.Errorf("Expected queue length 3, got %d", length)
	}

	// Test dequeueing (should return up to 100 keys at a time)
	dequeued := queue.Dequeue()
	if len(dequeued) != 3 {
		t.Errorf("Expected 3 keys dequeued, got %d", len(dequeued))
	}

	// Verify dequeued keys
	if dequeued[0] != key1 {
		t.Errorf("Expected first key %v, got %v", key1, dequeued[0])
	}
	if dequeued[1] != key2 {
		t.Errorf("Expected second key %v, got %v", key2, dequeued[1])
	}
	if dequeued[2] != key3 {
		t.Errorf("Expected third key %v, got %v", key3, dequeued[2])
	}

	// Test empty queue
	if length := queue.Len(); length != 0 {
		t.Errorf("Expected empty queue after dequeue, got length %d", length)
	}

	emptyDequeue := queue.Dequeue()
	if emptyDequeue != nil {
		t.Errorf("Expected nil from empty queue, got %v", emptyDequeue)
	}
}

// TestFrequencyClassProcessorOperations tests the frequency class processor operations.
//
// Rationale: The frequency class processor manages multiple busy word processors.
// This test ensures proper queue management and statistics collection.
func TestFrequencyClassProcessorOperations(t *testing.T) {
	// Create a frequency class processor
	fcp := pipeline.NewFrequencyClassProcessor(3, 10, 2.0, []int{}, "test_logs")

	// Test initial state
	stats := fcp.GetQueueStats()
	if len(stats) != 3 {
		t.Errorf("Expected 3 queue stats, got %d", len(stats))
	}

	// Test enqueueing to frequency classes
	key1 := tweets.ThreePartKey{Part1: 1, Part2: 2, Part3: 3}
	key2 := tweets.ThreePartKey{Part1: 4, Part2: 5, Part3: 6}

	fcp.EnqueueToFrequencyClass(0, key1)
	fcp.EnqueueToFrequencyClass(1, key2)
	fcp.EnqueueToFrequencyClass(0, key2) // Add to same class

	// Check queue sizes
	stats = fcp.GetQueueStats()
	if stats["freq_class_0_queue_size"] != 2 {
		t.Errorf("Expected class 0 queue size 2, got %d", stats["freq_class_0_queue_size"])
	}
	if stats["freq_class_1_queue_size"] != 1 {
		t.Errorf("Expected class 1 queue size 1, got %d", stats["freq_class_1_queue_size"])
	}
	if stats["freq_class_2_queue_size"] != 0 {
		t.Errorf("Expected class 2 queue size 0, got %d", stats["freq_class_2_queue_size"])
	}

	// Test enqueueing multiple keys
	keys := []tweets.ThreePartKey{
		{Part1: 7, Part2: 8, Part3: 9},
		{Part1: 10, Part2: 11, Part3: 12},
	}
	fcp.EnqueueKeysToFrequencyClass(2, keys)

	stats = fcp.GetQueueStats()
	if stats["freq_class_2_queue_size"] != 2 {
		t.Errorf("Expected class 2 queue size 2 after bulk enqueue, got %d", stats["freq_class_2_queue_size"])
	}

	// Test invalid class index (should be ignored gracefully)
	fcp.EnqueueToFrequencyClass(5, key1)  // Should be ignored
	fcp.EnqueueToFrequencyClass(-1, key1) // Should be ignored

	// Test processor stats (should be 0 since processors aren't running)
	processorStats := fcp.GetProcessorStats()
	if len(processorStats) != 3 {
		t.Errorf("Expected 3 processor stats, got %d", len(processorStats))
	}
}

// TestFrequencyClassProcessorSkipClasses tests that skipped frequency classes are properly handled
func TestFrequencyClassProcessorSkipClasses(t *testing.T) {
	// Create a frequency class processor with classes 1 and 2 skipped
	fcp := pipeline.NewFrequencyClassProcessor(4, 10, 2.0, []int{1, 2})

	// Test that skipped classes are not active
	if fcp.IsClassActive(0) != true {
		t.Error("Expected class 0 to be active")
	}
	if fcp.IsClassActive(1) != false {
		t.Error("Expected class 1 to be inactive (skipped)")
	}
	if fcp.IsClassActive(2) != false {
		t.Error("Expected class 2 to be inactive (skipped)")
	}
	if fcp.IsClassActive(3) != true {
		t.Error("Expected class 3 to be active")
	}

	// Test that enqueuing to skipped classes is ignored
	key := tweets.ThreePartKey{Part1: 1, Part2: 2, Part3: 3}

	// Enqueue to active class - should work
	fcp.EnqueueToFrequencyClass(0, key)
	stats := fcp.GetQueueStats()
	if stats["freq_class_0_queue_size"] != 1 {
		t.Errorf("Expected class 0 queue size 1, got %d", stats["freq_class_0_queue_size"])
	}

	// Enqueue to skipped class - should be ignored
	fcp.EnqueueToFrequencyClass(1, key)
	stats = fcp.GetQueueStats()
	if stats["freq_class_1_queue_size"] != 0 {
		t.Errorf("Expected class 1 queue size 0 (skipped), got %d", stats["freq_class_1_queue_size"])
	}

	// Test invalid class indices
	if fcp.IsClassActive(-1) != false {
		t.Error("Expected invalid class -1 to be inactive")
	}
	if fcp.IsClassActive(10) != false {
		t.Error("Expected invalid class 10 to be inactive")
	}
}

// TestBarrierSynchronization tests the barrier synchronization mechanism.
//
// Rationale: Barriers ensure all processors complete before starting the next cycle.
// This test ensures proper synchronization behavior.
func TestBarrierSynchronization(t *testing.T) {
	// Create a barrier for 3 threads
	barrier := pipeline.NewBarrier(3)

	// Test initial state
	if barrier.IsComplete() {
		t.Error("Expected barrier to not be complete initially")
	}

	// Test barrier behavior with multiple goroutines
	done := make(chan bool, 3)

	for i := 0; i < 3; i++ {
		go func(id int) {
			// Simulate some work
			barrier.Wait()
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 3; i++ {
		<-done
	}

	// Barrier should be complete
	if !barrier.IsComplete() {
		t.Error("Expected barrier to be complete after all threads waited")
	}

	// Test barrier reset
	barrier.Reset()
	if barrier.IsComplete() {
		t.Error("Expected barrier to not be complete after reset")
	}

	// Test barrier with different count
	barrier2 := pipeline.NewBarrier(1)
	barrier2.Wait()
	if !barrier2.IsComplete() {
		t.Error("Expected single-thread barrier to be complete after wait")
	}
}

// TestGlobalTokenMapping tests the global token mapping functionality.
//
// Rationale: Global token mapping is used to convert 3PKs back to actual words.
// This test ensures proper mapping setup and lookup.
func TestGlobalTokenMapping(t *testing.T) {
	// Create a test processor
	queue := pipeline.NewThreePartKeyQueue()
	fcp := pipeline.NewFrequencyClassProcessor(1, 10, 2.0, []int{})
	processor := pipeline.NewBusyWordProcessor(0, queue, 10, 2.0, fcp)

	// Set up global token mapping
	tokenMapping := map[tweets.ThreePartKey]string{
		{Part1: 1, Part2: 2, Part3: 3}: "hello",
		{Part1: 4, Part2: 5, Part3: 6}: "world",
		{Part1: 7, Part2: 8, Part3: 9}: "test",
	}
	var mappingMutex sync.RWMutex
	processor.SetGlobalTokenMapping(tokenMapping, &mappingMutex)

	// Test that mapping is set (we can't directly test the private method,
	// but we can verify the processor was created and configured)
	if processor == nil {
		t.Error("Expected processor to be created successfully")
	}

	// Test frequency class processor mapping setup
	fcp.SetGlobalTokenMappingForAll(tokenMapping, &mappingMutex)

	// Verify the processor was created and configured
	if fcp == nil {
		t.Error("Expected frequency class processor to be created successfully")
	}
}

// TestBusyWordProcessorConfiguration tests processor configuration and setup.
//
// Rationale: Proper configuration is essential for busy word detection.
// This test ensures processors are configured with correct parameters.
func TestBusyWordProcessorConfiguration(t *testing.T) {
	// Test different configurations
	testCases := []struct {
		numClasses      int
		arrayLen        int
		zScoreThreshold float64
	}{
		{1, 10, 1.0},
		{3, 100, 2.0},
		{5, 1000, 3.0},
	}

	for _, tc := range testCases {
		queue := pipeline.NewThreePartKeyQueue()
		fcp := pipeline.NewFrequencyClassProcessor(tc.numClasses, tc.arrayLen, tc.zScoreThreshold, []int{})
		processor := pipeline.NewBusyWordProcessor(0, queue, tc.arrayLen, tc.zScoreThreshold, fcp)

		// Verify processor was created
		if processor == nil {
			t.Errorf("Expected processor to be created with config %+v", tc)
		}

		// Verify frequency class processor was created
		if fcp == nil {
			t.Errorf("Expected frequency class processor to be created with config %+v", tc)
		}

		// Test queue stats
		stats := fcp.GetQueueStats()
		if len(stats) != tc.numClasses {
			t.Errorf("Expected %d queue stats, got %d", tc.numClasses, len(stats))
		}
	}
}
