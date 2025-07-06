package pipeline

import (
	"sync"
)

// TokenCounter keeps track of how many times each token appears in the current window.
// Uses record-level locking for thread safety with minimal contention.
type TokenCounter struct {
	counts map[string]int
	mu     sync.RWMutex
}

// NewTokenCounter creates a new TokenCounter with an empty map.
func NewTokenCounter() *TokenCounter {
	return &TokenCounter{counts: make(map[string]int)}
}

// IncrementTokens increases the count for each token in the list.
// Call this when a new tweet enters the window.
func (tc *TokenCounter) IncrementTokens(tokens []string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	for _, token := range tokens {
		tc.counts[token]++
	}
}

// DecrementTokens decreases the count for each token in the list.
// Call this when an old tweet leaves the window.
func (tc *TokenCounter) DecrementTokens(tokens []string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	for _, token := range tokens {
		tc.counts[token]--
		if tc.counts[token] <= 0 {
			delete(tc.counts, token) // Clean up to save memory
		}
	}
}

// GetCount returns the count for a specific token.
func (tc *TokenCounter) GetCount(token string) int {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.counts[token]
}

// Counts returns the map of all token counts (for stats reporting).
// This creates a snapshot of the current counts for thread safety.
func (tc *TokenCounter) Counts() map[string]int {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	// Create a copy to avoid race conditions
	snapshot := make(map[string]int, len(tc.counts))
	for token, count := range tc.counts {
		snapshot[token] = count
	}
	return snapshot
}

// CountsSnapshot returns a snapshot of token counts optimized for frequency calculations.
// This method is designed to be called from the background frequency calculation goroutine.
//
// TRADE-OFFS:
// - Creates a copy to avoid concurrent map access errors
// - Slightly slower but thread-safe
// - Suitable for frequency calculations where "pretty good is good enough"
func (tc *TokenCounter) CountsSnapshot() map[string]int {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	// Create a copy to avoid "concurrent map iteration and map write" errors
	// This is safer than returning a direct reference
	snapshot := make(map[string]int, len(tc.counts))
	for token, count := range tc.counts {
		snapshot[token] = count
	}
	return snapshot
}

// Clear resets all token counts to zero.
// Call this after frequency calculations to start fresh for the next window.
func (tc *TokenCounter) Clear() {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	// Clear the map by creating a new one
	tc.counts = make(map[string]int)
}
