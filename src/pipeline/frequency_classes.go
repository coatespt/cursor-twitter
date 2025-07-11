package pipeline

import (
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
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

// NewSetFilter creates a new SetFilter with the given tokens
func NewSetFilter(tokens []string) *SetFilter {
	sf := &SetFilter{tokens: make(map[string]bool, len(tokens))}
	for _, token := range tokens {
		sf.tokens[token] = true
	}
	return sf
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

// SerializableFilter represents a filter that can be saved/loaded
type SerializableFilter struct {
	Type   string   // "set" or "bloom"
	Tokens []string // For SetFilter
	// Note: Bloom filters are not easily serializable, so we'll focus on SetFilter for now
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

// BuildFrequencyClassHashSets divides tokens into F frequency classes and returns F hash set filters.
// Each class accounts for roughly the same number of token occurrences (not unique tokens).
func BuildFrequencyClassHashSets(tokenCounts map[string]int, F int, bloomSizes []uint, hashCounts []uint) FreqClassResult {
	// Step 1: Build a slice of (token, count) pairs
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

	// Step 4: Assign tokens to F classes
	classes := make([][]string, F)
	classIdx := 0
	runningTotal := 0

	// Distribute tokens to classes (removed excessive debug output for performance)
	for _, pair := range tokenCountsSlice {
		if classIdx < F-1 && runningTotal >= (classIdx+1)*C {
			classIdx++
		}
		classes[classIdx] = append(classes[classIdx], pair.Token)
		runningTotal += pair.Count
	}

	// Step 5: Create F hash set filters and insert tokens
	filters := make([]FreqClassFilter, F)
	for i := 0; i < F; i++ {
		setFilter := &SetFilter{tokens: make(map[string]bool, len(classes[i]))}
		for _, token := range classes[i] {
			setFilter.tokens[token] = true
		}
		filters[i] = setFilter
	}

	// Log final class distribution with usage counts
	fmt.Printf("*** FREQUENCY CLASS REBUILD: Built %d classes ***\n", F)
	fmt.Printf("*** DEBUG: Total tokens to distribute: %d, Target per class: %d ***\n", total, C)
	for i := 0; i < F; i++ {
		// Calculate total usage for this class (optimized - use map lookup)
		classUsage := 0
		for _, token := range classes[i] {
			classUsage += tokenCounts[token] // Direct map lookup - O(1)
		}
		fmt.Printf("  Class %d: %d distinct tokens, %d total usages\n", i+1, len(classes[i]), classUsage)
		if len(classes[i]) == 0 {
			fmt.Printf("  *** WARNING: Class %d is empty! ***\n", i+1)
		}
	}
	fmt.Printf("*** FREQUENCY CLASS REBUILD COMPLETE ***\n")

	return FreqClassResult{
		Filters:   filters,
		TopTokens: tokenCountsSlice[:min(10, len(tokenCountsSlice))],
	}
}

var globalFiltersMutex sync.RWMutex
var globalFilters []FreqClassFilter

// SetGlobalFilters sets the global frequency class filters
func SetGlobalFilters(filters []FreqClassFilter) {
	globalFiltersMutex.Lock()
	defer globalFiltersMutex.Unlock()
	globalFilters = filters
	fmt.Printf("*** SetGlobalFilters called with %d filters ***\n", len(filters))
}

// GetGlobalFilters returns the current global frequency class filters
func GetGlobalFilters() []FreqClassFilter {
	globalFiltersMutex.RLock()
	defer globalFiltersMutex.RUnlock()
	result := make([]FreqClassFilter, len(globalFilters))
	copy(result, globalFilters)
	//fmt.Printf("*** GetGlobalFilters called, returning %d filters ***\n", len(result))
	return result
}

// SaveToFile saves the frequency class filters to a file
// Note: This only saves SetFilter types, not BloomFilterWrapper
func (fcr *FreqClassResult) SaveToFile(filename string) error {
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

	// Convert filters to serializable format
	serializableFilters := make([]SerializableFilter, len(fcr.Filters))
	for i, filter := range fcr.Filters {
		if setFilter, ok := filter.(*SetFilter); ok {
			// Convert SetFilter to SerializableFilter
			tokens := make([]string, 0, len(setFilter.tokens))
			for token := range setFilter.tokens {
				tokens = append(tokens, token)
			}
			serializableFilters[i] = SerializableFilter{
				Type:   "set",
				Tokens: tokens,
			}
		} else {
			// Skip BloomFilterWrapper for now
			serializableFilters[i] = SerializableFilter{
				Type:   "bloom",
				Tokens: []string{}, // Bloom filters not serialized
			}
		}
	}

	// Create serializable result
	serializableResult := struct {
		Filters   []SerializableFilter
		TopTokens []TokenCount
	}{
		Filters:   serializableFilters,
		TopTokens: fcr.TopTokens,
	}

	// Encode and write to file
	encoder := gob.NewEncoder(file)
	if err := encoder.Encode(serializableResult); err != nil {
		return fmt.Errorf("failed to encode filters to %s: %v", filename, err)
	}

	return nil
}

// LoadFromFile loads frequency class filters from a file
func (fcr *FreqClassResult) LoadFromFile(filename string) error {
	// Open the file
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %v", filename, err)
	}
	defer file.Close()

	// Decode the serializable result
	var serializableResult struct {
		Filters   []SerializableFilter
		TopTokens []TokenCount
	}
	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&serializableResult); err != nil {
		return fmt.Errorf("failed to decode filters from %s: %v", filename, err)
	}

	// Convert back to FreqClassFilter
	filters := make([]FreqClassFilter, len(serializableResult.Filters))
	for i, sf := range serializableResult.Filters {
		if sf.Type == "set" {
			// Convert SerializableFilter back to SetFilter
			setFilter := &SetFilter{tokens: make(map[string]bool, len(sf.Tokens))}
			for _, token := range sf.Tokens {
				setFilter.tokens[token] = true
			}
			filters[i] = setFilter
		} else {
			// For bloom filters, create an empty SetFilter (placeholder)
			filters[i] = &SetFilter{tokens: make(map[string]bool)}
		}
	}

	// Update the FreqClassResult
	fcr.Filters = filters
	fcr.TopTokens = serializableResult.TopTokens

	return nil
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

	// Create a lookup map for O(1) token count access
	tokenCountMap := make(map[string]int, len(tokenCounts))
	for _, tc := range tokenCounts {
		tokenCountMap[tc.Token] = tc.Count
	}

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
				totalClassCount += tokenCountMap[token]
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
				totalClassCount += tokenCountMap[token]
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

// GetTokenFrequencyClass returns the frequency class index for a token.
// Checks filters in order from most frequent to least frequent.
// If the token is not found in any class, returns the last index (least frequent class).
func GetTokenFrequencyClass(token string) int {
	filters := GetGlobalFilters() // Thread-safe getter
	if len(filters) == 0 {
		fmt.Printf("WARNING: No frequency class filters available, assigning token '%s' to class 0\n", token)
		return 0 // Defensive: if no filters, return 0
	}

	// Debug: Log filter status occasionally (but not too often)
	// Note: This is a simple approach - in production you'd want a thread-safe counter
	// For now, we'll just log the first few calls to see what's happening
	if len(filters) > 0 {
		// Check if this is a SetFilter and has tokens
		if setFilter, ok := filters[0].(*SetFilter); ok {
			if len(setFilter.tokens) == 0 {
				fmt.Printf("WARNING: Frequency class filter 0 is empty\n")
			}
		}
	}

	for i, filter := range filters {
		if filter.Contains(token) {
			return i
		}
	}
	// Not found: assign to least frequent class
	return len(filters) - 1
}
