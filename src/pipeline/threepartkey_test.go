package pipeline

import (
	"testing"
)

func TestCleanupQueue(t *testing.T) {
	// Test basic cleanup queue functionality
	t.Run("BasicCleanupQueue", func(t *testing.T) {
		// First, add tokens to the 3PK mappings so they can be removed
		GenerateThreePartKey("test1")
		GenerateThreePartKey("test2")
		GenerateThreePartKey("test3")

		// Add some tokens to the cleanup queue
		AddToCleanupQueue("test1")
		AddToCleanupQueue("test2")
		AddToCleanupQueue("test3")

		// Check queue size
		queueSize := GetCleanupQueueSize()
		if queueSize != 3 {
			t.Errorf("Expected queue size 3, got %d", queueSize)
		}

		// Process 2 items from the queue
		removedCount := ProcessCleanupQueue(2)
		if removedCount != 2 {
			t.Errorf("Expected to remove 2 items, got %d", removedCount)
		}

		// Check remaining queue size
		queueSize = GetCleanupQueueSize()
		if queueSize != 1 {
			t.Errorf("Expected remaining queue size 1, got %d", queueSize)
		}

		// Process remaining items
		removedCount = ProcessCleanupQueue(10) // Process more than available
		if removedCount != 1 {
			t.Errorf("Expected to remove 1 item, got %d", removedCount)
		}

		// Check queue is empty
		queueSize = GetCleanupQueueSize()
		if queueSize != 0 {
			t.Errorf("Expected empty queue, got size %d", queueSize)
		}
	})

	t.Run("EmptyQueue", func(t *testing.T) {
		// Test processing empty queue
		removedCount := ProcessCleanupQueue(10)
		if removedCount != 0 {
			t.Errorf("Expected to remove 0 items from empty queue, got %d", removedCount)
		}
	})

	t.Run("LargeQueue", func(t *testing.T) {
		// First, add tokens to the 3PK mappings so they can be removed
		for i := 0; i < 100; i++ {
			GenerateThreePartKey("token" + string(rune(i)))
		}

		// Add many tokens to test batch processing
		for i := 0; i < 100; i++ {
			AddToCleanupQueue("token" + string(rune(i)))
		}

		// Process in batches
		removedCount := ProcessCleanupQueue(30)
		if removedCount != 30 {
			t.Errorf("Expected to remove 30 items, got %d", removedCount)
		}

		queueSize := GetCleanupQueueSize()
		if queueSize != 70 {
			t.Errorf("Expected remaining queue size 70, got %d", queueSize)
		}

		// Process remaining
		removedCount = ProcessCleanupQueue(100)
		if removedCount != 70 {
			t.Errorf("Expected to remove 70 items, got %d", removedCount)
		}
	})
}
