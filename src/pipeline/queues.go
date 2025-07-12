package pipeline

import (
	"sync"
)

// TokenQueue is a thread-safe ring buffer queue for storing token arrays
// Uses a circular buffer to avoid memory allocation on dequeue operations
type TokenQueue struct {
	items    [][]string
	head     int // Index of next item to dequeue
	tail     int // Index of next position to enqueue
	size     int // Current number of items
	capacity int // Maximum number of items
	mu       sync.RWMutex
}

// NewTokenQueue creates a new empty TokenQueue with initial capacity
func NewTokenQueue() *TokenQueue {
	return &TokenQueue{
		items:    make([][]string, 1000), // Start with 1000 slots
		capacity: 1000,
	}
}

// Enqueue adds a slice of tokens to the end of the queue
func (tq *TokenQueue) Enqueue(tokens []string) {
	tq.mu.Lock()
	defer tq.mu.Unlock()

	// Check if we need to grow the buffer
	if tq.size >= tq.capacity {
		tq.grow()
	}

	// Add item at tail
	tq.items[tq.tail] = tokens
	tq.tail = (tq.tail + 1) % tq.capacity
	tq.size++
}

// Dequeue removes and returns the first slice of tokens from the queue
// Returns nil if the queue is empty
func (tq *TokenQueue) Dequeue() []string {
	tq.mu.Lock()
	defer tq.mu.Unlock()

	if tq.size == 0 {
		return nil
	}

	// Get item at head
	tokens := tq.items[tq.head]
	tq.items[tq.head] = nil // Clear the slot to help GC

	// Move head forward
	tq.head = (tq.head + 1) % tq.capacity
	tq.size--

	return tokens
}

// Len returns the number of token arrays in the queue
func (tq *TokenQueue) Len() int {
	tq.mu.RLock()
	defer tq.mu.RUnlock()
	return tq.size
}

// Clear removes all items from the queue
func (tq *TokenQueue) Clear() {
	tq.mu.Lock()
	defer tq.mu.Unlock()

	// Clear all slots to help GC
	for i := 0; i < tq.capacity; i++ {
		tq.items[i] = nil
	}

	tq.head = 0
	tq.tail = 0
	tq.size = 0
}

// grow doubles the capacity of the queue
func (tq *TokenQueue) grow() {
	newCapacity := tq.capacity * 2
	newItems := make([][]string, newCapacity)

	// Copy existing items to new buffer
	for i := 0; i < tq.size; i++ {
		index := (tq.head + i) % tq.capacity
		newItems[i] = tq.items[index]
	}

	tq.items = newItems
	tq.head = 0
	tq.tail = tq.size
	tq.capacity = newCapacity
}
