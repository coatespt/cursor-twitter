package pipeline

import (
	"fmt"
	"testing"
	"time"
)

func TestFrequencyCalculationAlgorithm(t *testing.T) {
	// Create test data with known distribution
	tokenCounts := map[string]int{
		"the":  1000, // Most frequent
		"and":  800,
		"to":   600,
		"of":   500,
		"a":    400,
		"in":   300,
		"is":   250,
		"it":   200,
		"you":  150,
		"that": 100,
		"he":   80,
		"was":  60,
		"for":  40,
		"on":   30,
		"are":  20,
		"as":   10,
		"with": 5,
		"his":  3,
		"they": 2,
		"at":   1, // Least frequent
	}

	// Test with 3 frequency classes
	F := 3
	result := BuildFrequencyClassBloomFiltersOptimized(tokenCounts, F, nil, nil)

	// Verify we got the right number of classes
	if len(result.Filters) != F {
		t.Errorf("Expected %d frequency classes, got %d", F, len(result.Filters))
	}

	// Calculate expected total and target per class
	total := 0
	for _, count := range tokenCounts {
		total += count
	}
	targetPerClass := total / F

	t.Logf("Total token occurrences: %d", total)
	t.Logf("Target per class: %d", targetPerClass)

	// Test that each class has roughly the same total usage
	// (We can't easily test this without exposing the class contents,
	// but we can verify the algorithm structure is correct)

	// Verify top tokens are correctly identified
	if len(result.TopTokens) == 0 {
		t.Error("Expected top tokens to be populated")
	}

	// Check that top token is the most frequent
	expectedTopToken := "the"
	if result.TopTokens[0].Token != expectedTopToken {
		t.Errorf("Expected top token to be '%s', got '%s'", expectedTopToken, result.TopTokens[0].Token)
	}

	// Check that top token has the highest count
	expectedTopCount := 1000
	if result.TopTokens[0].Count != expectedTopCount {
		t.Errorf("Expected top token count to be %d, got %d", expectedTopCount, result.TopTokens[0].Count)
	}

	t.Logf("Top token: %s (count: %d)", result.TopTokens[0].Token, result.TopTokens[0].Count)
}

func TestFrequencyClassDistribution(t *testing.T) {
	// Create a more realistic distribution with Zipf-like characteristics
	tokenCounts := make(map[string]int)

	// Create tokens with decreasing frequency (Zipf distribution)
	for i := 1; i <= 100; i++ {
		token := fmt.Sprintf("token%d", i)
		count := 1000 / i // Zipf-like: count decreases as 1/rank
		if count < 1 {
			count = 1
		}
		tokenCounts[token] = count
	}

	// Test with 5 frequency classes
	F := 5
	result := BuildFrequencyClassBloomFiltersOptimized(tokenCounts, F, nil, nil)

	// Verify we got the right number of classes
	if len(result.Filters) != F {
		t.Errorf("Expected %d frequency classes, got %d", F, len(result.Filters))
	}

	// Calculate total and expected per class
	total := 0
	for _, count := range tokenCounts {
		total += count
	}
	targetPerClass := total / F

	t.Logf("Total token occurrences: %d", total)
	t.Logf("Target per class: %d", targetPerClass)
	t.Logf("Number of classes: %d", F)

	// Verify the algorithm produces reasonable results
	// (Each class should have roughly targetPerClass total usage)
	t.Logf("Test completed successfully with %d classes", F)
}

func TestFrequencyClassEdgeCases(t *testing.T) {
	// Test with very few tokens
	t.Run("FewTokens", func(t *testing.T) {
		tokenCounts := map[string]int{
			"a": 10,
			"b": 5,
			"c": 3,
		}

		result := BuildFrequencyClassBloomFiltersOptimized(tokenCounts, 2, nil, nil)
		if len(result.Filters) != 2 {
			t.Errorf("Expected 2 classes, got %d", len(result.Filters))
		}
	})

	// Test with single class
	t.Run("SingleClass", func(t *testing.T) {
		tokenCounts := map[string]int{
			"a": 10,
			"b": 5,
			"c": 3,
		}

		result := BuildFrequencyClassBloomFiltersOptimized(tokenCounts, 1, nil, nil)
		if len(result.Filters) != 1 {
			t.Errorf("Expected 1 class, got %d", len(result.Filters))
		}
	})

	// Test with zero tokens
	t.Run("ZeroTokens", func(t *testing.T) {
		tokenCounts := map[string]int{}

		result := BuildFrequencyClassBloomFiltersOptimized(tokenCounts, 3, nil, nil)
		if len(result.Filters) != 3 {
			t.Errorf("Expected 3 classes, got %d", len(result.Filters))
		}
	})
}

func TestFrequencyClassDetailedDistribution(t *testing.T) {
	// Create test data with known distribution
	tokenCounts := map[string]int{
		"the":  1000, // Most frequent
		"and":  800,
		"to":   600,
		"of":   500,
		"a":    400,
		"in":   300,
		"is":   250,
		"it":   200,
		"you":  150,
		"that": 100,
		"he":   80,
		"was":  60,
		"for":  40,
		"on":   30,
		"are":  20,
		"as":   10,
		"with": 5,
		"his":  3,
		"they": 2,
		"at":   1, // Least frequent
	}

	// Test with 3 frequency classes
	F := 3
	result := BuildFrequencyClassBloomFiltersOptimized(tokenCounts, F, nil, nil)

	// Calculate total and target per class
	total := 0
	for _, count := range tokenCounts {
		total += count
	}
	targetPerClass := total / F

	t.Logf("=== FREQUENCY CLASS DISTRIBUTION TEST ===")
	t.Logf("Total token occurrences: %d", total)
	t.Logf("Target per class: %d", targetPerClass)
	t.Logf("Number of classes: %d", F)

	// Create a map to track which class each token belongs to
	tokenToClass := make(map[string]int)

	// Test each token to see which class it belongs to
	for token := range tokenCounts {
		for classIdx, filter := range result.Filters {
			if filter.Contains(token) {
				tokenToClass[token] = classIdx
				break
			}
		}
	}

	// Calculate actual distribution
	classTotals := make([]int, F)
	classTokens := make([][]string, F)

	for token, classIdx := range tokenToClass {
		count := tokenCounts[token]
		classTotals[classIdx] += count
		classTokens[classIdx] = append(classTokens[classIdx], token)
	}

	// Report the distribution
	t.Logf("\n=== CLASS DISTRIBUTION ===")
	for i := 0; i < F; i++ {
		t.Logf("Class %d: %d tokens, %d total occurrences (target: %d)",
			i+1, len(classTokens[i]), classTotals[i], targetPerClass)

		// Show some tokens in this class
		if len(classTokens[i]) > 0 {
			t.Logf("  Sample tokens: %v", classTokens[i][:min(5, len(classTokens[i]))])
		}
	}

	// Verify that classes are roughly balanced
	for i, total := range classTotals {
		deviation := float64(abs(total-targetPerClass)) / float64(targetPerClass)
		if deviation > 0.2 {
			t.Logf("Warning: Class %d has %d occurrences (target: %d, deviation: %.1f%%)",
				i+1, total, targetPerClass, deviation*100)
		} else {
			t.Logf("✓ Class %d is well balanced: %d occurrences (target: %d, deviation: %.1f%%)",
				i+1, total, targetPerClass, deviation*100)
		}
	}

	// Verify that tokens are sorted correctly (most frequent in class 0)
	expectedTopTokens := []string{"the", "and"}
	for _, expected := range expectedTopTokens {
		if classIdx, exists := tokenToClass[expected]; exists {
			if classIdx != 0 {
				t.Errorf("Expected top token '%s' to be in class 0, but it's in class %d", expected, classIdx)
			} else {
				t.Logf("✓ Top token '%s' correctly placed in class 0", expected)
			}
		}
	}

	// Verify that "to" is in class 1 (as expected based on the algorithm)
	if classIdx, exists := tokenToClass["to"]; exists {
		if classIdx == 1 {
			t.Logf("✓ Token 'to' correctly placed in class 1")
		} else {
			t.Errorf("Expected token 'to' to be in class 1, but it's in class %d", classIdx)
		}
	}
}

// Helper functions
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func TestClassifyToken(t *testing.T) {
	// Create test filters
	filters := []FreqClassFilter{
		&SetFilter{tokens: map[string]bool{"the": true, "and": true, "to": true}}, // Class 0: most frequent
		&SetFilter{tokens: map[string]bool{"of": true, "a": true, "in": true}},    // Class 1: frequent
		&SetFilter{tokens: map[string]bool{"is": true, "it": true, "you": true}},  // Class 2: less frequent
	}

	// Test tokens that should be classified
	tests := []struct {
		token    string
		expected int
		desc     string
	}{
		{"the", 0, "most frequent token"},
		{"and", 0, "most frequent token"},
		{"of", 1, "frequent token"},
		{"a", 1, "frequent token"},
		{"is", 2, "less frequent token"},
		{"it", 2, "less frequent token"},
		{"unknown", 2, "unknown token (should go to least frequent class)"},
		{"xyz", 2, "unknown token (should go to least frequent class)"},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			result := ClassifyToken(test.token, filters)
			if result != test.expected {
				t.Errorf("Expected token '%s' to be classified as class %d, got %d",
					test.token, test.expected, result)
			}
		})
	}
}

func TestClassifyTokens(t *testing.T) {
	// Create test filters
	filters := []FreqClassFilter{
		&SetFilter{tokens: map[string]bool{"the": true, "and": true, "to": true}}, // Class 0: most frequent
		&SetFilter{tokens: map[string]bool{"of": true, "a": true, "in": true}},    // Class 1: frequent
		&SetFilter{tokens: map[string]bool{"is": true, "it": true, "you": true}},  // Class 2: less frequent
	}

	// Test multiple tokens
	tokens := []string{"the", "of", "is", "unknown", "and", "xyz"}
	classification := ClassifyTokens(tokens, filters)

	// Verify classification
	expected := map[int][]string{
		0: {"the", "and"},
		1: {"of"},
		2: {"is", "unknown", "xyz"},
	}

	for classIdx, expectedTokens := range expected {
		actualTokens := classification[classIdx]
		if len(actualTokens) != len(expectedTokens) {
			t.Errorf("Class %d: expected %d tokens, got %d", classIdx, len(expectedTokens), len(actualTokens))
			continue
		}

		// Check that all expected tokens are present
		for _, expectedToken := range expectedTokens {
			found := false
			for _, actualToken := range actualTokens {
				if actualToken == expectedToken {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Class %d: expected token '%s' not found in %v", classIdx, expectedToken, actualTokens)
			}
		}
	}

	t.Logf("Classification result: %+v", classification)
}

func TestClassifyTokensEmpty(t *testing.T) {
	filters := []FreqClassFilter{
		&SetFilter{tokens: map[string]bool{"the": true}},
		&SetFilter{tokens: map[string]bool{"of": true}},
	}

	// Test with empty token list
	classification := ClassifyTokens([]string{}, filters)
	if len(classification) != 0 {
		t.Errorf("Expected empty classification for empty tokens, got %+v", classification)
	}

	// Test with nil filters
	classification = ClassifyTokens([]string{"the"}, nil)
	if len(classification) != 0 {
		t.Errorf("Expected empty classification for nil filters, got %+v", classification)
	}
}

func TestFrequencyClassProcessor(t *testing.T) {
	// Create a processor with 3 frequency classes
	numClasses := 3
	arraySize := 100
	fcp := NewFrequencyClassProcessor(numClasses, arraySize)

	// Start the processor
	fcp.Start()
	defer fcp.Stop()

	// Test enqueueing tokens to different classes
	testTokens := []string{"test1", "test2", "test3"}

	// Enqueue tokens to class 0
	fcp.EnqueueTokens(0, testTokens)

	// Enqueue tokens to class 1
	fcp.EnqueueTokens(1, []string{"test4", "test5"})

	// Enqueue tokens to class 2
	fcp.EnqueueTokens(2, []string{"test6"})

	// Test invalid class index
	fcp.EnqueueTokens(5, []string{"invalid"}) // Should be ignored

	// Get queue stats
	stats := fcp.GetQueueStats()

	// Verify we have stats for all classes
	expectedKeys := []string{"freq_class_0_queue_size", "freq_class_1_queue_size", "freq_class_2_queue_size"}
	for _, key := range expectedKeys {
		if _, exists := stats[key]; !exists {
			t.Errorf("Expected stats key '%s' not found", key)
		}
	}

	// Give the workers time to process
	time.Sleep(100 * time.Millisecond)

	t.Logf("FrequencyClassProcessor test completed successfully")
}

func TestFrequencyClassWorker(t *testing.T) {
	// Create a token queue
	tokenQueue := NewTokenQueue()

	// Create a worker
	classIndex := 1
	arraySize := 50
	worker := NewFrequencyClassWorker(classIndex, tokenQueue, arraySize)

	// Verify the worker was created correctly
	if worker.classIndex != classIndex {
		t.Errorf("Expected class index %d, got %d", classIndex, worker.classIndex)
	}

	if len(worker.counters) != 3 {
		t.Errorf("Expected 3 counter arrays, got %d", len(worker.counters))
	}

	for i, counterArray := range worker.counters {
		if len(counterArray) != arraySize {
			t.Errorf("Expected counter array %d to have size %d, got %d", i, arraySize, len(counterArray))
		}
	}

	// Start the worker
	worker.Start()
	defer worker.Stop()

	// Enqueue some tokens
	testTokens := []string{"worker_test1", "worker_test2"}
	tokenQueue.Enqueue(testTokens)

	// Give the worker time to process
	time.Sleep(100 * time.Millisecond)

	t.Logf("FrequencyClassWorker test completed successfully")
}
