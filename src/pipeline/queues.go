package pipeline

import (
	"sync"
)

// TokenQueue is a thread-safe queue for storing token arrays
type TokenQueue struct {
	items [][]string
	mu    sync.RWMutex
}

// NewTokenQueue creates a new empty TokenQueue
func NewTokenQueue() *TokenQueue {
	return &TokenQueue{
		items: make([][]string, 0),
	}
}

// Enqueue adds a slice of tokens to the end of the queue
func (tq *TokenQueue) Enqueue(tokens []string) {
	tq.mu.Lock()
	defer tq.mu.Unlock()
	tq.items = append(tq.items, tokens)
}

// Dequeue removes and returns the first slice of tokens from the queue
// Returns nil if the queue is empty
func (tq *TokenQueue) Dequeue() []string {
	tq.mu.Lock()
	defer tq.mu.Unlock()

	if len(tq.items) == 0 {
		return nil
	}

	// Get the first item
	tokens := tq.items[0]

	// Remove it from the queue
	tq.items = tq.items[1:]

	return tokens
}

// Len returns the number of token arrays in the queue
func (tq *TokenQueue) Len() int {
	tq.mu.RLock()
	defer tq.mu.RUnlock()
	return len(tq.items)
}

// Clear removes all items from the queue
func (tq *TokenQueue) Clear() {
	tq.mu.Lock()
	defer tq.mu.Unlock()
	tq.items = make([][]string, 0)
}
