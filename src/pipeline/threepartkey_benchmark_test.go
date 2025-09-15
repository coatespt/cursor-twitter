package pipeline

import (
	"testing"
)

func BenchmarkCleanupQueuePerformance(b *testing.B) {
	// Benchmark the cleanup queue performance with realistic data sizes

	b.Run("AddToQueue", func(b *testing.B) {
		// Benchmark adding tokens to cleanup queue
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			AddToCleanupQueue("token" + string(rune(i%1000)))
		}
	})

	b.Run("ProcessQueue", func(b *testing.B) {
		// Pre-populate queue with tokens
		for i := 0; i < 10000; i++ {
			GenerateThreePartKey("token"+string(rune(i)), 0)
			AddToCleanupQueue("token" + string(rune(i)))
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ProcessCleanupQueue(1000)
		}
	})

	b.Run("QueueSizeCheck", func(b *testing.B) {
		// Pre-populate queue
		for i := 0; i < 1000; i++ {
			AddToCleanupQueue("token" + string(rune(i)))
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			GetCleanupQueueSize()
		}
	})
}

func BenchmarkMemoryUsage(b *testing.B) {
	// Benchmark memory usage with and without cleanup

	b.Run("WithoutCleanup", func(b *testing.B) {
		// Simulate keeping all tokens in 3PK mappings
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			GenerateThreePartKey("token"+string(rune(i)), 0)
		}
	})

	b.Run("WithCleanup", func(b *testing.B) {
		// Simulate adding tokens and then cleaning them up
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			token := "token" + string(rune(i))
			GenerateThreePartKey(token, int64(i))
			AddToCleanupQueue(token)
		}
		// Process cleanup queue
		ProcessCleanupQueue(b.N)
	})
}

func TestMemoryEstimate(t *testing.T) {
	// Test to estimate memory usage per token mapping
	t.Run("MemoryPerToken", func(t *testing.T) {
		// Add some tokens and estimate memory usage
		tokens := []string{
			"hello", "world", "test", "token", "example",
			"verylongtokenname", "short", "mediumlength", "a", "supercalifragilisticexpialidocious",
		}

		for _, token := range tokens {
			GenerateThreePartKey(token, 0)
		}

		// Get current mapping sizes
		Token3PKMutex.RLock()
		tokenCount := len(TokenTo3PK)
		threePKCount := len(ThreePKToToken)
		Token3PKMutex.RUnlock()

		t.Logf("Token mappings: %d tokens, %d 3PKs", tokenCount, threePKCount)
		t.Logf("Average token length: ~%d characters", len(tokens[0]))
		t.Logf("Estimated memory per token mapping: ~%d bytes",
			len(tokens[0])*2+24) // Rough estimate: token string + 3PK struct
	})
}
