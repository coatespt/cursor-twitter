package pipeline

import (
	"sync"
)

// CleanupQueue is a simple thread-safe FIFO queue for token cleanup
type CleanupQueue struct {
	items []string
	mu    sync.Mutex
}

// NewCleanupQueue creates a new empty cleanup queue
func NewCleanupQueue() *CleanupQueue {
	return &CleanupQueue{
		items: make([]string, 0, 10000), // Pre-allocate capacity
	}
}

// Enqueue adds a token to the end of the queue
func (cq *CleanupQueue) Enqueue(token string) {
	cq.mu.Lock()
	defer cq.mu.Unlock()
	cq.items = append(cq.items, token)
}

// Dequeue removes and returns the first token from the queue
// Returns empty string if queue is empty
func (cq *CleanupQueue) Dequeue() string {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	if len(cq.items) == 0 {
		return ""
	}

	token := cq.items[0]
	cq.items = cq.items[1:] // Remove first element
	return token
}

// DequeueBatch removes and returns up to maxItems tokens from the queue
func (cq *CleanupQueue) DequeueBatch(maxItems int) []string {
	cq.mu.Lock()
	defer cq.mu.Unlock()

	if len(cq.items) == 0 {
		return nil
	}

	itemsToProcess := maxItems
	if len(cq.items) < maxItems {
		itemsToProcess = len(cq.items)
	}

	// Extract items to process
	items := make([]string, itemsToProcess)
	copy(items, cq.items[:itemsToProcess])

	// Remove processed items
	cq.items = cq.items[itemsToProcess:]

	return items
}

// Size returns the number of items in the queue
func (cq *CleanupQueue) Size() int {
	cq.mu.Lock()
	defer cq.mu.Unlock()
	return len(cq.items)
}

// IsEmpty returns true if the queue is empty
func (cq *CleanupQueue) IsEmpty() bool {
	cq.mu.Lock()
	defer cq.mu.Unlock()
	return len(cq.items) == 0
}
