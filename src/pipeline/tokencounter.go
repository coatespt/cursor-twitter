package pipeline

import (
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
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

// SaveToFile saves the current token counts to a file using gob encoding
func (tc *TokenCounter) SaveToFile(filename string) error {
	// Ensure the directory exists
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %v", dir, err)
	}

	// Create the file
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %v", filename, err)
	}
	defer file.Close()

	// Get a snapshot of the counts
	counts := tc.CountsSnapshot()

	// Encode and write to file
	encoder := gob.NewEncoder(file)
	if err := encoder.Encode(counts); err != nil {
		return fmt.Errorf("failed to encode counts to %s: %v", filename, err)
	}

	return nil
}

// LoadFromFile loads token counts from a file using gob decoding
func (tc *TokenCounter) LoadFromFile(filename string) error {
	// Open the file
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %v", filename, err)
	}
	defer file.Close()

	// Decode the counts
	var counts map[string]int
	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&counts); err != nil {
		return fmt.Errorf("failed to decode counts from %s: %v", filename, err)
	}

	// Replace the current counts with the loaded ones
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.counts = counts

	return nil
}
