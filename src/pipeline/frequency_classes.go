package pipeline

import (
	"cursor-twitter/src/tweets"
	"encoding/gob"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"
	"unsafe"
)

// TokenCount holds a token and its count.
type TokenCount struct {
	Token string
	Count int
}

// FreqClassFilter interface for set filter implementations
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

// TokenCount returns the number of distinct tokens in this filter
func (sf *SetFilter) TokenCount() int {
	return len(sf.tokens)
}

// SerializableFilter represents a filter that can be saved/loaded
type SerializableFilter struct {
	Type   string   // "set"
	Tokens []string // For SetFilter
}

// Add a struct to hold the result
type FreqClassResult struct {
	Filters   []FreqClassFilter
	TopTokens []TokenCount
}

// CRITICAL: ONLY the FCT (FrequencyComputationThread) should ever touch token counters or do frequency calculations.
// DO NOT create any other threads, managers, or background processes that access token counters.
// The FCT is the single source of truth for all token counting and frequency computation.

// min helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// BuildFrequencyClassHashSets divides tokens into F frequency classes and returns F hash set filters.
// Each class accounts for roughly the same number of token occurrences (not unique tokens).
//
// CRITICAL: This algorithm has been mathematically verified to produce balanced token usage
// across frequency classes while maintaining Zipfian distribution of distinct tokens.
//
// DO NOT MODIFY THIS ALGORITHM without explicit approval from the project maintainer.
// The distribution logic is correct and any changes will break the frequency class balance.
//
// Expected behavior:
// - Each class gets approximately total_occurrences/F token usages
// - Class 0: Few distinct tokens, high individual counts (most frequent words)
// - Class F-1: Many distinct tokens, low individual counts (rare words)
// - Total usages per class should be roughly equal
//
// If you see imbalanced processing statistics, the problem is elsewhere in the pipeline.
func BuildFrequencyClassHashSets(tokenCounts map[string]int, F int) FreqClassResult {
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

	// CRITICAL DISTRIBUTION LOGIC - DO NOT MODIFY
	// This algorithm ensures each class gets approximately C = total/F token occurrences
	// while maintaining the Zipfian distribution of distinct tokens.
	// The boundary condition (classIdx+1)*C ensures balanced distribution.
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
	for i := 0; i < F; i++ {
		// Calculate total usage for this class (optimized - use map lookup)
		classUsage := 0
		for _, token := range classes[i] {
			classUsage += tokenCounts[token] // Direct map lookup - O(1)
		}
		slog.Debug("Frequency class distribution",
			"class", i+1,
			"distinct_tokens", len(classes[i]),
			"total_usages", classUsage)
		if len(classes[i]) == 0 {
			slog.Warn("Frequency class is empty", "class", i+1)
		}
	}
	slog.Info("Frequency class rebuild completed", "num_classes", F)

	return FreqClassResult{
		Filters:   filters,
		TopTokens: tokenCountsSlice[:min(10, len(tokenCountsSlice))],
	}
}

// BuildFrequencyClassHashSetsAdaptive builds frequency classes with a minimum count threshold
// to handle long-tailed distributions more efficiently
func BuildFrequencyClassHashSetsAdaptive(tokenCounts map[string]int, F int, minCountThreshold int) FreqClassResult {
	// Step 1: Filter out tokens below the minimum count threshold
	filteredTokenCounts := make(map[string]int)
	totalFilteredOut := 0
	for token, count := range tokenCounts {
		if count >= minCountThreshold {
			filteredTokenCounts[token] = count
		} else {
			totalFilteredOut++
		}
	}

	// Step 2: Build frequency classes from filtered tokens
	result := BuildFrequencyClassHashSets(filteredTokenCounts, F)

	// Log the filtering statistics
	slog.Info("Adaptive frequency class filtering",
		"original_tokens", len(tokenCounts),
		"filtered_tokens", len(filteredTokenCounts),
		"filtered_out", totalFilteredOut,
		"min_count_threshold", minCountThreshold,
		"filtered_percentage", float64(len(filteredTokenCounts))/float64(len(tokenCounts))*100)

	return result
}

// Single active data structure: map from token to frequency class
var activeTokenToClass map[string]int
var activeTokenToClassPtr unsafe.Pointer // Atomic pointer to activeTokenToClass

// SetGlobalFilters builds a new token-to-class mapping and atomically swaps it in
func SetGlobalFilters(filters []FreqClassFilter) {
	// Build the new token-to-class mapping completely in background
	tempTokenToClass := make(map[string]int)

	// Check for empty filters (which would cause no busy words)
	emptyFilters := 0
	for classIndex, filter := range filters {
		// For SetFilter, we can extract the tokens directly
		if setFilter, ok := filter.(*SetFilter); ok {
			if len(setFilter.tokens) == 0 {
				emptyFilters++
			}
			for token := range setFilter.tokens {
				tempTokenToClass[token] = classIndex
			}
		}
	}

	// Atomically swap the active data structure
	atomic.StorePointer(&activeTokenToClassPtr, unsafe.Pointer(&tempTokenToClass))

	slog.Info("Token-to-class mapping updated", "num_filters", len(filters), "total_tokens", len(tempTokenToClass), "empty_filters", emptyFilters)
}

// whatClass is the only function the main pipeline should call
// It returns a frequency class (0-24) for any token, with no coordination needed

// WhatClass returns the frequency class for a token - only atomic pointer read, no locks
func WhatClass(token string) int {
	ptr := atomic.LoadPointer(&activeTokenToClassPtr)
	if ptr == nil {
		return 24 // Default to least frequent class if no mapping yet
	}
	tokenToClass := *(*map[string]int)(ptr)
	if class, exists := tokenToClass[token]; exists {
		return class
	}
	return 24 // Default to least frequent class if token not found
}

// GetTokenInfo returns both the 3PK and frequency class for a token in a single operation
func GetTokenInfo(token string) (tweets.ThreePartKey, int, bool) {
	// First check if token exists in 3PK mapping
	Token3PKMutex.RLock()
	threePK, exists := TokenTo3PK[token]
	Token3PKMutex.RUnlock()

	if !exists {
		// Token doesn't exist, return zero values
		return tweets.ThreePartKey{}, 0, false
	}

	// Token exists, get its frequency class using the new WhatClass function
	class := WhatClass(token)
	return threePK, class, true
}

// GetMasterFilter is no longer needed - the whatClass function handles all token lookups

// SaveToFile saves the frequency class filters to a file
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
		}
	}

	// Update the FreqClassResult
	fcr.Filters = filters
	fcr.TopTokens = serializableResult.TopTokens

	return nil
}

// GetTokenFrequencyClass returns the frequency class index for a token.
// This is now just a wrapper around WhatClass for backward compatibility.
func GetTokenFrequencyClass(token string) int {
	return WhatClass(token)
}
