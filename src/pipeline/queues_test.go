package pipeline

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestTokenQueueRingBuffer(t *testing.T) {
	queue := NewTokenQueue()

	// Test initial state
	if length := queue.Len(); length != 0 {
		t.Errorf("Expected empty queue, got length %d", length)
	}

	// Test enqueueing tokens
	tokens1 := []string{"hello", "world"}
	tokens2 := []string{"test", "data"}

	queue.Enqueue(tokens1)
	queue.Enqueue(tokens2)

	// Verify queue length
	if length := queue.Len(); length != 2 {
		t.Errorf("Expected queue length 2, got %d", length)
	}

	// Test dequeueing tokens
	dequeued1 := queue.Dequeue()
	if len(dequeued1) != 2 || dequeued1[0] != "hello" || dequeued1[1] != "world" {
		t.Errorf("Expected dequeued tokens ['hello', 'world'], got %v", dequeued1)
	}

	dequeued2 := queue.Dequeue()
	if len(dequeued2) != 2 || dequeued2[0] != "test" || dequeued2[1] != "data" {
		t.Errorf("Expected dequeued tokens ['test', 'data'], got %v", dequeued2)
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

func TestTokenQueueRingBufferWraparound(t *testing.T) {
	queue := NewTokenQueue()

	// Fill the queue to test wraparound
	for i := 0; i < 1500; i++ {
		queue.Enqueue([]string{fmt.Sprintf("token%d", i)})
	}

	// Verify we can dequeue all items
	for i := 0; i < 1500; i++ {
		tokens := queue.Dequeue()
		if len(tokens) != 1 || tokens[0] != fmt.Sprintf("token%d", i) {
			t.Errorf("Expected token%d, got %v", i, tokens)
		}
	}

	// Queue should be empty
	if length := queue.Len(); length != 0 {
		t.Errorf("Expected empty queue, got length %d", length)
	}
}

func TestTokenQueueConcurrency(t *testing.T) {
	queue := NewTokenQueue()
	var wg sync.WaitGroup

	// Start multiple goroutines enqueueing
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				queue.Enqueue([]string{fmt.Sprintf("goroutine%d_token%d", id, j)})
			}
		}(i)
	}

	// Start multiple goroutines dequeuing
	dequeuedCount := 0
	var dequeuedMu sync.Mutex
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				tokens := queue.Dequeue()
				if tokens == nil {
					time.Sleep(1 * time.Millisecond)
					continue
				}
				dequeuedMu.Lock()
				dequeuedCount++
				if dequeuedCount >= 1000 { // Total expected items
					dequeuedMu.Unlock()
					return
				}
				dequeuedMu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Verify all items were processed
	if dequeuedCount != 1000 {
		t.Errorf("Expected 1000 items to be dequeued, got %d", dequeuedCount)
	}
}

func TestTokenQueueClear(t *testing.T) {
	queue := NewTokenQueue()

	// Add some items
	queue.Enqueue([]string{"test1"})
	queue.Enqueue([]string{"test2"})

	// Verify items are there
	if length := queue.Len(); length != 2 {
		t.Errorf("Expected queue length 2, got %d", length)
	}

	// Clear the queue
	queue.Clear()

	// Verify queue is empty
	if length := queue.Len(); length != 0 {
		t.Errorf("Expected empty queue after clear, got length %d", length)
	}

	// Verify we can still enqueue after clear
	queue.Enqueue([]string{"test3"})
	if length := queue.Len(); length != 1 {
		t.Errorf("Expected queue length 1 after enqueue, got %d", length)
	}
}
