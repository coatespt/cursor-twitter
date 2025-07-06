package pipeline

import (
	"fmt"
	"sort"
	"sync"

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

// CRITICAL: ONLY the FCT (FrequencyComputationThread) should ever touch token counters or do frequency calculations.
// DO NOT create any other threads, managers, or background processes that access token counters.
// The FCT is the single source of truth for all token counting and frequency computation.

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

// BuildFrequencyClassHashSets is a dummy implementation that creates simple hash set filters
// This is a placeholder until the real implementation is developed
func BuildFrequencyClassHashSets(tokenCounts map[string]int, F int, bloomSizes []uint, hashCounts []uint) FreqClassResult {
	// Create F dummy filters using hash sets
	filters := make([]FreqClassFilter, F)

	// For now, just create empty hash sets for each class
	for i := 0; i < F; i++ {
		filters[i] = &SetFilter{tokens: make(map[string]bool)}
	}

	// Create some dummy top tokens for debugging
	topTokens := make([]TokenCount, 0)
	if len(tokenCounts) > 0 {
		// Just take the first few tokens as "top" tokens
		count := 0
		for token, freq := range tokenCounts {
			if count >= 10 { // Limit to 10 top tokens
				break
			}
			topTokens = append(topTokens, TokenCount{Token: token, Count: freq})
			count++
		}
	}

	return FreqClassResult{
		Filters:   filters,
		TopTokens: topTokens,
	}
}

// GlobalFilters holds the current frequency class filters for the main thread to access
var GlobalFilters []FreqClassFilter
var globalFiltersMutex sync.RWMutex

// SetGlobalFilters sets the global frequency class filters (thread-safe)
func SetGlobalFilters(filters []FreqClassFilter) {
	globalFiltersMutex.Lock()
	defer globalFiltersMutex.Unlock()
	GlobalFilters = filters
}

// GetGlobalFilters returns the current global frequency class filters (thread-safe)
func GetGlobalFilters() []FreqClassFilter {
	globalFiltersMutex.RLock()
	defer globalFiltersMutex.RUnlock()
	return GlobalFilters
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
