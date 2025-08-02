package pipeline

import (
	"testing"
)

func BenchmarkSliceVsQueue(b *testing.B) {
	// Benchmark slice-based approach (current implementation)
	b.Run("SliceBased", func(b *testing.B) {
		// Reset global state
		cleanupQueue = NewCleanupQueue()

		b.Run("AddToQueue", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				AddToCleanupQueue("token" + string(rune(i%1000)))
			}
		})

		b.Run("ProcessQueue", func(b *testing.B) {
			// Pre-populate
			for i := 0; i < 1000; i++ {
				AddToCleanupQueue("token" + string(rune(i)))
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ProcessCleanupQueue(100)
			}
		})

		b.Run("QueueSize", func(b *testing.B) {
			// Pre-populate
			for i := 0; i < 1000; i++ {
				AddToCleanupQueue("token" + string(rune(i)))
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				GetCleanupQueueSize()
			}
		})
	})

	// Benchmark queue-based approach
	b.Run("QueueBased", func(b *testing.B) {
		queue := NewCleanupQueue()

		b.Run("AddToQueue", func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				queue.Enqueue("token" + string(rune(i%1000)))
			}
		})

		b.Run("ProcessQueue", func(b *testing.B) {
			// Pre-populate
			for i := 0; i < 1000; i++ {
				queue.Enqueue("token" + string(rune(i)))
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				queue.DequeueBatch(100)
			}
		})

		b.Run("QueueSize", func(b *testing.B) {
			// Pre-populate
			for i := 0; i < 1000; i++ {
				queue.Enqueue("token" + string(rune(i)))
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				queue.Size()
			}
		})
	})
}

func TestQueueImplementation(t *testing.T) {
	queue := NewCleanupQueue()

	// Test basic operations
	queue.Enqueue("token1")
	queue.Enqueue("token2")
	queue.Enqueue("token3")

	if queue.Size() != 3 {
		t.Errorf("Expected size 3, got %d", queue.Size())
	}

	// Test batch dequeue
	items := queue.DequeueBatch(2)
	if len(items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(items))
	}
	if items[0] != "token1" || items[1] != "token2" {
		t.Errorf("Expected [token1, token2], got %v", items)
	}

	if queue.Size() != 1 {
		t.Errorf("Expected size 1, got %d", queue.Size())
	}

	// Test remaining item
	items = queue.DequeueBatch(10)
	if len(items) != 1 {
		t.Errorf("Expected 1 item, got %d", len(items))
	}
	if items[0] != "token3" {
		t.Errorf("Expected token3, got %s", items[0])
	}

	if !queue.IsEmpty() {
		t.Errorf("Expected empty queue")
	}
}
