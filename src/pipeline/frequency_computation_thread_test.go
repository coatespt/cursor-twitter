package pipeline

import (
	"testing"
	"time"
)

func TestFrequencyComputationThread(t *testing.T) {
	// Create test components
	tokenCounter := NewTokenCounter()
	inboundQueue := NewTokenQueue()
	oldQueue := NewTokenQueue()

	// Create FCT
	fct := NewFrequencyComputationThread(
		tokenCounter,
		inboundQueue,
		oldQueue,
		1, // 1 minute interval
		3, // 3 frequency classes
	)

	// Start FCT
	fct.Start()
	defer fct.Stop()

	// Give FCT time to start
	time.Sleep(100 * time.Millisecond)

	// Test 1: Process inbound tokens
	t.Run("ProcessInboundTokens", func(t *testing.T) {
		// Add tokens to inbound queue
		inboundQueue.Enqueue([]string{"hello", "world"})
		inboundQueue.Enqueue([]string{"hello", "test"})

		// Give FCT time to process
		time.Sleep(200 * time.Millisecond)

		// Check that tokens were counted
		if count := tokenCounter.GetCount("hello"); count != 2 {
			t.Errorf("Expected 'hello' count to be 2, got %d", count)
		}
		if count := tokenCounter.GetCount("world"); count != 1 {
			t.Errorf("Expected 'world' count to be 1, got %d", count)
		}
		if count := tokenCounter.GetCount("test"); count != 1 {
			t.Errorf("Expected 'test' count to be 1, got %d", count)
		}
	})

	// Test 2: Process old tokens (decrement)
	t.Run("ProcessOldTokens", func(t *testing.T) {
		// Add tokens to old queue
		oldQueue.Enqueue([]string{"hello", "world"})

		// Give FCT time to process
		time.Sleep(200 * time.Millisecond)

		// Check that tokens were decremented
		if count := tokenCounter.GetCount("hello"); count != 1 {
			t.Errorf("Expected 'hello' count to be 1 after decrement, got %d", count)
		}
		if count := tokenCounter.GetCount("world"); count != 0 {
			t.Errorf("Expected 'world' count to be 0 after decrement, got %d", count)
		}
		if count := tokenCounter.GetCount("test"); count != 1 {
			t.Errorf("Expected 'test' count to remain 1, got %d", count)
		}
	})

	// Test 3: Trigger rebuild
	t.Run("TriggerRebuild", func(t *testing.T) {
		// Add some tokens first
		inboundQueue.Enqueue([]string{"new", "tokens"})
		time.Sleep(100 * time.Millisecond)

		// Trigger rebuild (no return value to check)
		fct.TriggerRebuild()

		// Trigger rebuild again (should be fine - FCT handles it autonomously)
		fct.TriggerRebuild()

		// Give FCT time to process
		time.Sleep(500 * time.Millisecond)
	})

	// Test 4: Queue stats
	t.Run("QueueStats", func(t *testing.T) {
		stats := fct.GetQueueStats()

		// Should have stats for all queues
		if _, exists := stats["inbound_token_queue_size"]; !exists {
			t.Error("Expected inbound_token_queue_size in stats")
		}
		if _, exists := stats["old_token_queue_size"]; !exists {
			t.Error("Expected old_token_queue_size in stats")
		}
		if _, exists := stats["distinct_tokens"]; !exists {
			t.Error("Expected distinct_tokens in stats")
		}
	})
}

func TestFrequencyComputationThreadConcurrency(t *testing.T) {
	// Test that FCT can handle concurrent token additions
	tokenCounter := NewTokenCounter()
	inboundQueue := NewTokenQueue()
	oldQueue := NewTokenQueue()

	fct := NewFrequencyComputationThread(
		tokenCounter,
		inboundQueue,
		oldQueue,
		1,
		3,
	)

	fct.Start()
	defer fct.Stop()

	// Give FCT time to start
	time.Sleep(100 * time.Millisecond)

	// Add tokens concurrently
	go func() {
		for i := 0; i < 100; i++ {
			inboundQueue.Enqueue([]string{"concurrent", "test"})
		}
	}()

	go func() {
		for i := 0; i < 50; i++ {
			oldQueue.Enqueue([]string{"concurrent", "test"})
		}
	}()

	// Wait for processing to complete
	// Use a longer wait time to ensure all tokens are processed
	time.Sleep(1 * time.Second)

	// Wait for queues to be empty to ensure all processing is done
	for inboundQueue.Len() > 0 || oldQueue.Len() > 0 {
		time.Sleep(100 * time.Millisecond)
	}

	// Check final count (100 added, 50 removed = 50 remaining)
	if count := tokenCounter.GetCount("concurrent"); count != 50 {
		t.Errorf("Expected 'concurrent' count to be 50, got %d", count)
	}
	if count := tokenCounter.GetCount("test"); count != 50 {
		t.Errorf("Expected 'test' count to be 50, got %d", count)
	}
}

func TestFrequencyComputationThreadTokenProcessing(t *testing.T) {
	// Create test components
	tokenCounter := NewTokenCounter()
	inboundQueue := NewTokenQueue()
	oldQueue := NewTokenQueue()

	// Create FCT
	fct := NewFrequencyComputationThread(
		tokenCounter,
		inboundQueue,
		oldQueue,
		1, // 1 minute interval
		3, // 3 frequency classes
	)

	// Start FCT
	fct.Start()
	defer fct.Stop()

	// Give FCT time to start
	time.Sleep(100 * time.Millisecond)

	// Add tokens to inbound queue
	inboundQueue.Enqueue([]string{"test", "token"})
	inboundQueue.Enqueue([]string{"another", "test"})

	// Give FCT time to process
	time.Sleep(200 * time.Millisecond)

	// Check that tokens were counted
	if count := tokenCounter.GetCount("test"); count != 2 {
		t.Errorf("Expected 'test' count to be 2, got %d", count)
	}
	if count := tokenCounter.GetCount("token"); count != 1 {
		t.Errorf("Expected 'token' count to be 1, got %d", count)
	}

	// Check queue sizes
	stats := fct.GetQueueStats()
	if stats["inbound_token_queue_size"] != 0 {
		t.Errorf("Expected inbound queue to be empty, got %d", stats["inbound_token_queue_size"])
	}
	if stats["distinct_tokens"] != 3 {
		t.Errorf("Expected 3 distinct tokens, got %d", stats["distinct_tokens"])
	}

	t.Logf("Queue stats: %+v", stats)
}

func TestFrequencyComputationThreadRunLoop(t *testing.T) {
	// Create test components
	tokenCounter := NewTokenCounter()
	inboundQueue := NewTokenQueue()
	oldQueue := NewTokenQueue()

	// Create FCT
	fct := NewFrequencyComputationThread(
		tokenCounter,
		inboundQueue,
		oldQueue,
		1, // 1 minute interval
		3, // 3 frequency classes
	)

	// Start FCT
	fct.Start()
	defer fct.Stop()

	// Give FCT time to start and run
	time.Sleep(200 * time.Millisecond)

	// Add tokens to inbound queue
	inboundQueue.Enqueue([]string{"test", "token"})

	// Give FCT time to process
	time.Sleep(200 * time.Millisecond)

	// Check that tokens were processed
	if count := tokenCounter.GetCount("test"); count != 1 {
		t.Errorf("Expected 'test' count to be 1, got %d", count)
	}

	// Check queue sizes
	stats := fct.GetQueueStats()
	if stats["inbound_token_queue_size"] != 0 {
		t.Errorf("Expected inbound queue to be empty, got %d", stats["inbound_token_queue_size"])
	}

	t.Logf("Queue stats: %+v", stats)
}
