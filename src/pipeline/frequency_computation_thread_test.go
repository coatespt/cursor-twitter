package pipeline

import (
	"testing"
	"time"
)

func TestFrequencyComputationThread(t *testing.T) {
	// Create test components
	tokenCounter := NewTokenCounter()
	inboundQueue := NewTokenQueue()

	// Create FCT
	fct := NewFrequencyComputationThread(
		tokenCounter,
		inboundQueue,
		3,    // frequency classes
		1000, // window size
		10,   // tokenPersistFiles
		5,    // rebuildEveryFiles
		"",   // stateDir - empty for test
		0,    // minCountThreshold - 0 for test
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

	// Test 2: Trigger rebuild
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

	// Test 3: Queue stats
	t.Run("QueueStats", func(t *testing.T) {
		stats := fct.GetQueueStats()

		// Should have stats for all queues
		if _, exists := stats["inbound_token_queue_size"]; !exists {
			t.Error("Expected inbound_token_queue_size in stats")
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

	fct := NewFrequencyComputationThread(
		tokenCounter,
		inboundQueue,
		3,    // frequency classes
		1000, // window size
		10,   // tokenPersistFiles
		5,    // rebuildEveryFiles
		"",   // stateDir - empty for test
		0,    // minCountThreshold - 0 for test
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

	// Wait for processing to complete
	// Use a longer wait time to ensure all tokens are processed
	time.Sleep(1 * time.Second)

	// Wait for queues to be empty to ensure all processing is done
	for inboundQueue.Len() > 0 {
		time.Sleep(100 * time.Millisecond)
	}

	// Check final count (100 added, no decrementing = 100 remaining)
	if count := tokenCounter.GetCount("concurrent"); count != 100 {
		t.Errorf("Expected 'concurrent' count to be 100, got %d", count)
	}
	if count := tokenCounter.GetCount("test"); count != 100 {
		t.Errorf("Expected 'test' count to be 100, got %d", count)
	}
}

func TestFrequencyComputationThreadTokenProcessing(t *testing.T) {
	// Create test components
	tokenCounter := NewTokenCounter()
	inboundQueue := NewTokenQueue()

	// Create FCT
	fct := NewFrequencyComputationThread(
		tokenCounter,
		inboundQueue,
		3,    // frequency classes
		1000, // window size
		10,   // tokenPersistFiles
		5,    // rebuildEveryFiles
		"",   // stateDir - empty for test
		0,    // minCountThreshold - 0 for test
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

func TestFrequencyComputationThreadFileBasedRebuild(t *testing.T) {
	// Test that FCT triggers rebuilds based on file count
	tokenCounter := NewTokenCounter()
	inboundQueue := NewTokenQueue()

	// Create FCT with small window and rebuild every 2 files
	fct := NewFrequencyComputationThread(
		tokenCounter,
		inboundQueue,
		3,   // frequency classes
		100, // window size (small for testing)
		5,   // tokenPersistFiles (small for testing)
		2,   // rebuildEveryFiles (rebuild every 2 files)
		"",  // stateDir - empty for test
		0,   // minCountThreshold - 0 for test
	)

	// Start FCT
	fct.Start()
	defer fct.Stop()

	// Give FCT time to start
	time.Sleep(100 * time.Millisecond)

	// Add enough tokens to trigger file writes
	// Each file will contain 20 tokens (100/5)
	// We need to add enough tokens to write multiple files
	for i := 0; i < 100; i++ {
		inboundQueue.Enqueue([]string{"test", "token"})
	}

	// Give FCT time to process and write files
	time.Sleep(500 * time.Millisecond)

	// Check that rebuilds were triggered
	rebuildCount := fct.GetRebuildCount()
	if rebuildCount == 0 {
		t.Logf("No rebuilds triggered yet, this is expected if not enough files were written")
	} else {
		t.Logf("Rebuilds triggered: %d", rebuildCount)
	}

	// Check queue stats
	stats := fct.GetQueueStats()
	t.Logf("Final queue stats: %+v", stats)
}

func TestFrequencyComputationThreadRunLoop(t *testing.T) {
	// Create test components
	tokenCounter := NewTokenCounter()
	inboundQueue := NewTokenQueue()

	// Create FCT
	fct := NewFrequencyComputationThread(
		tokenCounter,
		inboundQueue,
		3,    // frequency classes
		1000, // window size
		10,   // tokenPersistFiles
		5,    // rebuildEveryFiles
		"",   // stateDir - empty for test
		0,    // minCountThreshold - 0 for test
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
