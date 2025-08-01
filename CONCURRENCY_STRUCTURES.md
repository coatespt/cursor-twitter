# Concurrency Management Structures in Twitter Data Processing Pipeline

## Mutexes and Locks

### 1. FrequencyComputationThread (`src/pipeline/frequency_computation_thread.go`)

**Line 40: `filtersMutex sync.RWMutex`**
- Protects `currentFilters []FreqClassFilter` 
- Used for thread-safe access to frequency class filters
- Read-write mutex allows concurrent reads, exclusive writes

**Line 44: `rebuildCountMutex sync.Mutex`**
- Protects `rebuildCount int` (debug counter)
- Simple mutex for incrementing rebuild statistics

### 2. TokenQueue (`src/pipeline/queues.go`)

**Line 14: `mu sync.RWMutex`**
- Protects the entire queue structure (`items`, `head`, `tail`, `size`, `capacity`)
- Used in `Enqueue()`, `Dequeue()`, `Len()`, `Clear()`, `grow()` methods
- Read-write mutex allows concurrent length checks with exclusive enqueue/dequeue

### 3. TokenCounter (`src/pipeline/tokencounter.go`)

**Line 16: `mu sync.RWMutex`**
- Protects `counts map[string]int` and `totalCount int64`
- Used in all token counting operations (`IncrementTokens`, `DecrementTokens`, `GetCount`, `Counts`, `CountsSnapshot`, `Clear`, `SetCountsDirectly`)
- Read-write mutex allows concurrent reads with exclusive writes

## Atomic Operations

### 1. FrequencyComputationThread

**Line 18: `persistenceInProgress int32` (global)**
- Atomic flag to track when persistence operations are running
- Used with `atomic.StoreInt32()` and `atomic.LoadInt32()`

**Line 32: `shouldRebuild int32`**
- Atomic boolean flag for rebuild signaling
- Used with `atomic.StoreInt32()` and `atomic.LoadInt32()`

### 2. TokenCounter

**Line 13: `totalCount int64`**
- Atomic running total of all token counts
- Used with `atomic.AddInt64()` and `atomic.LoadInt64()`

## Channels

### 1. FrequencyComputationThread

**Line 35: `stopChan chan struct{}`**
- Signal channel for graceful shutdown
- Used in main loop with `select` statement

**Line 36: `wg sync.WaitGroup`**
- Synchronizes goroutine lifecycle
- Used in `Start()` and `Stop()` methods

## Thread-Safe Data Structures

### 1. TokenQueue
- Circular buffer implementation with mutex protection
- Dynamic growth with `grow()` method
- Thread-safe enqueue/dequeue operations

### 2. TokenCounter  
- Map-based counting with RWMutex protection
- Snapshot methods for safe concurrent access
- Atomic total count for O(1) performance

## Concurrency Patterns

### 1. Producer-Consumer
- Main pipeline produces tokens → TokenQueue → FrequencyComputationThread consumes
- Queue-based decoupling between pipeline stages

### 2. Background Processing
- FrequencyComputationThread runs in separate goroutine
- Non-blocking token processing with periodic rebuilds

### 3. Graceful Shutdown
- Channel-based stop signaling
- WaitGroup synchronization for clean termination

### 4. Snapshot Access
- `CountsSnapshot()` creates safe copies for concurrent access
- Prevents "concurrent map iteration and map write" errors

## Critical Sections

1. **Token counting operations** - Protected by RWMutex in TokenCounter
2. **Queue operations** - Protected by RWMutex in TokenQueue  
3. **Filter updates** - Protected by RWMutex in FrequencyComputationThread
4. **Rebuild operations** - Atomic flag coordination
5. **Persistence operations** - Global atomic flag protection 