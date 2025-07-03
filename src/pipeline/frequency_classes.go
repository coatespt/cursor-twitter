package pipeline

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
)

// TokenCount holds a token and its count.
type TokenCount struct {
	Token string
	Count int
}

// FreqClassFilter interface for both set and Bloom filter implementations
type FreqClassFilter interface {
	Contains(token string) bool
}

// SetFilter implements FreqClassFilter using a simple hash set
type SetFilter struct {
	tokens map[string]bool
}

func (sf *SetFilter) Contains(token string) bool {
	return sf.tokens[token]
}

// BloomFilterWrapper implements FreqClassFilter using a Bloom filter
type BloomFilterWrapper struct {
	filter *bloom.BloomFilter
}

func (bf *BloomFilterWrapper) Contains(token string) bool {
	return bf.filter.TestString(token)
}

// Add a struct to hold the result
type FreqClassResult struct {
	Filters   []FreqClassFilter
	TopTokens []TokenCount
}

// AsyncFreqClassManager handles asynchronous frequency class calculations
type AsyncFreqClassManager struct {
	currentFilters []FreqClassFilter
	filtersMutex   sync.RWMutex
	rebuildChan    chan struct{}
	stopChan       chan struct{}
	wg             sync.WaitGroup
	tokenCounter   *TokenCounter

	// Throttling mechanism
	lastRebuildTime time.Time
	rebuildDuration time.Duration
	throttleMutex   sync.RWMutex
}

// NewAsyncFreqClassManager creates a new asynchronous frequency class manager
func NewAsyncFreqClassManager(tc *TokenCounter) *AsyncFreqClassManager {
	return &AsyncFreqClassManager{
		rebuildChan:  make(chan struct{}, 1), // Buffered to avoid blocking
		stopChan:     make(chan struct{}),
		tokenCounter: tc,
	}
}

// Start begins the background frequency calculation goroutine
func (afcm *AsyncFreqClassManager) Start() {
	afcm.wg.Add(1)
	go afcm.backgroundWorker()
}

// Stop gracefully stops the background worker
func (afcm *AsyncFreqClassManager) Stop() {
	close(afcm.stopChan)
	afcm.wg.Wait()
}

// TriggerRebuild signals that a frequency class rebuild is needed
// Returns true if rebuild was triggered, false if throttled
func (afcm *AsyncFreqClassManager) TriggerRebuild() bool {
	// Check if we should throttle based on recent rebuild time
	afcm.throttleMutex.RLock()
	timeSinceLastRebuild := time.Since(afcm.lastRebuildTime)
	afcm.throttleMutex.RUnlock()

	// Throttle if last rebuild was less than 2x the rebuild duration ago
	// This prevents overwhelming the system with rebuild requests
	minInterval := afcm.rebuildDuration * 2
	if timeSinceLastRebuild < minInterval {
		fmt.Printf("[AsyncFreqClass] Throttling rebuild request (last rebuild was %v ago, min interval is %v)\n",
			timeSinceLastRebuild, minInterval)
		return false
	}

	select {
	case afcm.rebuildChan <- struct{}{}:
		// Successfully queued rebuild request
		return true
	default:
		// Rebuild already queued, skip
		return false
	}
}

// GetCurrentFilters returns the current frequency class filters (thread-safe)
func (afcm *AsyncFreqClassManager) GetCurrentFilters() []FreqClassFilter {
	afcm.filtersMutex.RLock()
	defer afcm.filtersMutex.RUnlock()
	return afcm.currentFilters
}

// ShouldThrottleIngestion returns true if ingestion should be slowed down
// to allow frequency calculations to catch up
func (afcm *AsyncFreqClassManager) ShouldThrottleIngestion(maxRebuildTimeSeconds int) bool {
	afcm.throttleMutex.RLock()
	defer afcm.throttleMutex.RUnlock()

	// If we haven't done a rebuild yet, don't throttle
	if afcm.lastRebuildTime.IsZero() {
		return false
	}

	// If the last rebuild took longer than expected, throttle
	// This indicates the system is struggling to keep up
	expectedRebuildTime := time.Duration(maxRebuildTimeSeconds) * time.Second
	if afcm.rebuildDuration > expectedRebuildTime {
		return true
	}

	// If we're getting rebuild requests too frequently, throttle
	timeSinceLastRebuild := time.Since(afcm.lastRebuildTime)
	minInterval := afcm.rebuildDuration * 2
	return timeSinceLastRebuild < minInterval
}

// GetThrottleDelay returns the recommended delay for ingestion throttling
func (afcm *AsyncFreqClassManager) GetThrottleDelay(maxRebuildTimeSeconds int) time.Duration {
	if !afcm.ShouldThrottleIngestion(maxRebuildTimeSeconds) {
		return 0
	}

	// Return a delay proportional to how far behind we are
	afcm.throttleMutex.RLock()
	timeSinceLastRebuild := time.Since(afcm.lastRebuildTime)
	afcm.throttleMutex.RUnlock()

	// Base delay of 100ms, plus additional time if we're falling behind
	baseDelay := 100 * time.Millisecond
	additionalDelay := timeSinceLastRebuild / 10 // Proportional to how long since last rebuild

	return baseDelay + additionalDelay
}

// backgroundWorker runs the frequency calculation in the background
func (afcm *AsyncFreqClassManager) backgroundWorker() {
	defer afcm.wg.Done()

	for {
		select {
		case <-afcm.stopChan:
			return
		case <-afcm.rebuildChan:
			afcm.performRebuild(afcm.tokenCounter)
		}
	}
}

// performRebuild does the actual frequency class calculation
func (afcm *AsyncFreqClassManager) performRebuild(tc *TokenCounter) {
	startTime := time.Now()
	fmt.Printf("[AsyncFreqClass] Starting background frequency calculation at %s\n", startTime.Format(time.RFC3339))

	// Get a snapshot of current token counts (optimized for frequency calculations)
	// We use the direct snapshot since we can tolerate some inconsistency
	tokenCounts := tc.CountsSnapshot()

	// Perform the calculation
	result := BuildFrequencyClassBloomFiltersOptimized(tokenCounts, 7, nil, nil)

	// Update filters atomically
	afcm.filtersMutex.Lock()
	afcm.currentFilters = result.Filters
	afcm.filtersMutex.Unlock()

	duration := time.Since(startTime)

	// Update rebuild timing information
	afcm.throttleMutex.Lock()
	afcm.lastRebuildTime = time.Now()
	afcm.rebuildDuration = duration
	afcm.throttleMutex.Unlock()

	fmt.Printf("[AsyncFreqClass] Completed background calculation in %v\n", duration)
}

// BuildFrequencyClassBloomFiltersOptimized is an optimized version that doesn't require TokenCounter
func BuildFrequencyClassBloomFiltersOptimized(tokenCounts map[string]int, F int, bloomSizes []uint, hashCounts []uint) FreqClassResult {
	// Step 1: Build a slice of (token, count) pairs (pre-allocate for efficiency)
	tokenCountsSlice := make([]TokenCount, 0, len(tokenCounts))
	for token, count := range tokenCounts {
		tokenCountsSlice = append(tokenCountsSlice, TokenCount{Token: token, Count: count})
	}

	// Step 2: Sort by count descending (most frequent first)
	sort.Slice(tokenCountsSlice, func(i, j int) bool {
		return tokenCountsSlice[i].Count > tokenCountsSlice[j].Count
	})

	// Step 3: Calculate total count and class size
	total := 0
	for _, tc := range tokenCountsSlice {
		total += tc.Count
	}
	if F <= 0 {
		F = 1
	}
	C := total / F

	// Step 4: Assign tokens to F classes (optimized)
	classes := make([][]string, F)
	classIdx := 0
	runningTotal := 0

	for _, pair := range tokenCountsSlice {
		if classIdx < F-1 && runningTotal >= (classIdx+1)*C {
			classIdx++
		}
		classes[classIdx] = append(classes[classIdx], pair.Token)
		runningTotal += pair.Count
	}

	// Step 5: Create F filters and insert tokens (parallel processing)
	filters := make([]FreqClassFilter, F)
	var wg sync.WaitGroup

	for i := 0; i < F; i++ {
		wg.Add(1)
		go func(classIndex int) {
			defer wg.Done()
			tokenCount := len(classes[classIndex])

			// Use hash set for small classes (threshold: 1000 tokens)
			if tokenCount < 1000 {
				setFilter := &SetFilter{tokens: make(map[string]bool, tokenCount)}
				for _, token := range classes[classIndex] {
					setFilter.tokens[token] = true
				}
				filters[classIndex] = setFilter
			} else {
				// Use Bloom filter for large classes
				bloomSize := uint(tokenCount * 10) // Simple sizing
				numHashes := uint(10)              // Simple hash count
				if bloomSizes != nil && classIndex < len(bloomSizes) {
					bloomSize = bloomSizes[classIndex]
				}
				if hashCounts != nil && classIndex < len(hashCounts) {
					numHashes = hashCounts[classIndex]
				}

				bf := bloom.New(bloomSize, numHashes)
				for _, token := range classes[classIndex] {
					bf.AddString(token)
				}
				filters[classIndex] = &BloomFilterWrapper{filter: bf}
			}
		}(i)
	}

	wg.Wait()

	// Log final class distribution
	for i := 0; i < F; i++ {
		fmt.Printf("[AsyncFreqClass] Class %d assigned %d tokens\n", i+1, len(classes[i]))
	}

	return FreqClassResult{
		Filters:   filters,
		TopTokens: tokenCountsSlice[:min(10, len(tokenCountsSlice))],
	}
}

// min helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// BuildFrequencyClassBloomFilters divides tokens into F frequency classes and returns F filters.
// Each class accounts for roughly the same number of token occurrences (not unique tokens).
// Small classes use hash sets, large classes use Bloom filters.
// bloomSizes and hashCounts are arrays of length F, one for each frequency class.
func BuildFrequencyClassBloomFilters(tc *TokenCounter, F int, bloomSizes []uint, hashCounts []uint) FreqClassResult {
	// Step 1: Build a slice of (token, count) pairs
	tokenCounts := make([]TokenCount, 0, len(tc.Counts()))
	for token, count := range tc.Counts() {
		tokenCounts = append(tokenCounts, TokenCount{Token: token, Count: count})
	}

	// Step 2: Sort by count descending (most frequent first)
	sort.Slice(tokenCounts, func(i, j int) bool {
		return tokenCounts[i].Count > tokenCounts[j].Count
	})

	// Log the top 10 most frequent tokens
	topN := 10
	if len(tokenCounts) < topN {
		topN = len(tokenCounts)
	}
	fmt.Printf("[FreqClass] Top %d tokens: ", topN)
	for i := 0; i < topN; i++ {
		fmt.Printf("%s(%d) ", tokenCounts[i].Token, tokenCounts[i].Count)
	}
	fmt.Println()

	// Step 3: Calculate total count and class size
	total := 0
	for _, tc := range tokenCounts {
		total += tc.Count
	}
	if F <= 0 {
		F = 1
	}
	C := total / F
	fmt.Printf("[FreqClass] Total tokens: %d, Classes: %d, Target per class: %d\n", total, F, C)

	// Step 4: Assign tokens to F classes
	classes := make([][]string, F)
	classIdx := 0
	runningTotal := 0
	for _, pair := range tokenCounts {
		if classIdx < F-1 && runningTotal >= (classIdx+1)*C {
			fmt.Printf("[FreqClass] Advancing to class %d at running total %d\n", classIdx+1, runningTotal)
			classIdx++
		}
		classes[classIdx] = append(classes[classIdx], pair.Token)
		runningTotal += pair.Count
	}

	// Log final class distribution
	for i := 0; i < F; i++ {
		fmt.Printf("[FreqClass] Class %d assigned %d tokens\n", i+1, len(classes[i]))
	}

	// Step 5: Create F filters and insert tokens
	filters := make([]FreqClassFilter, F)
	for i := 0; i < F; i++ {
		tokenCount := len(classes[i])

		// Use hash set for small classes (threshold: 1000 tokens)
		if tokenCount < 1000 {
			setFilter := &SetFilter{tokens: make(map[string]bool)}
			for _, token := range classes[i] {
				setFilter.tokens[token] = true
			}
			filters[i] = setFilter

			// Log the number of tokens and total occurrences in this class
			totalClassCount := 0
			for _, token := range classes[i] {
				for _, tc := range tokenCounts {
					if tc.Token == token {
						totalClassCount += tc.Count
					}
				}
			}
			fmt.Printf("[FreqClass] Class %d: %d tokens, %d total occurrences, using HASH SET\n",
				i+1, len(classes[i]), totalClassCount)
		} else {
			// Use Bloom filter for large classes
			bloomSize := bloomSizes[i]
			numHashes := hashCounts[i]
			bf := bloom.New(bloomSize, numHashes)
			for _, token := range classes[i] {
				bf.AddString(token)
			}
			filters[i] = &BloomFilterWrapper{filter: bf}

			// Log the number of tokens and total occurrences in this class
			totalClassCount := 0
			for _, token := range classes[i] {
				for _, tc := range tokenCounts {
					if tc.Token == token {
						totalClassCount += tc.Count
					}
				}
			}
			fmt.Printf("[FreqClass] Class %d: %d tokens, %d total occurrences, bloom_size=%d, hash_count=%d\n",
				i+1, len(classes[i]), totalClassCount, bloomSize, numHashes)
		}
	}

	return FreqClassResult{
		Filters:   filters,
		TopTokens: tokenCounts[:topN],
	}
}
