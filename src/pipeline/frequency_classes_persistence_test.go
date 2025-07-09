package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestFreqClassResultSaveLoad(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "freqclass_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test token counts
	tokenCounts := map[string]int{
		"the":  1000,
		"and":  800,
		"a":    600,
		"to":   500,
		"of":   400,
		"is":   300,
		"in":   250,
		"you":  200,
		"they": 150,
		"it":   100,
		"for":  80,
		"as":   70,
		"his":  60,
		"with": 50,
		"on":   40,
		"at":   30,
		"by":   25,
		"this": 20,
		"from": 15,
		"but":  10,
	}

	// Build frequency class result
	result := BuildFrequencyClassHashSets(tokenCounts, 3, nil, nil)

	// Save to file
	filename := filepath.Join(tempDir, "test_frequency_filters.gob")
	if err := result.SaveToFile(filename); err != nil {
		t.Fatalf("SaveToFile failed: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		t.Fatalf("Save file was not created: %s", filename)
	}

	// Create a new FreqClassResult and load from file
	var loadedResult FreqClassResult
	if err := loadedResult.LoadFromFile(filename); err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	// Verify the loaded data matches the original
	if len(loadedResult.Filters) != len(result.Filters) {
		t.Errorf("Filter count mismatch: original=%d, loaded=%d", len(result.Filters), len(loadedResult.Filters))
	}

	if len(loadedResult.TopTokens) != len(result.TopTokens) {
		t.Errorf("TopTokens count mismatch: original=%d, loaded=%d", len(result.TopTokens), len(loadedResult.TopTokens))
	}

	// Test that filters work correctly after loading
	for i, originalFilter := range result.Filters {
		if i >= len(loadedResult.Filters) {
			t.Errorf("Loaded result missing filter %d", i)
			continue
		}

		loadedFilter := loadedResult.Filters[i]

		// Test a few tokens that should be in each class
		for token := range tokenCounts {
			originalContains := originalFilter.Contains(token)
			loadedContains := loadedFilter.Contains(token)
			if originalContains != loadedContains {
				t.Errorf("Filter %d: token '%s' mismatch - original=%v, loaded=%v",
					i, token, originalContains, loadedContains)
			}
		}
	}

	// Test TopTokens
	for i, originalToken := range result.TopTokens {
		if i >= len(loadedResult.TopTokens) {
			t.Errorf("Loaded result missing TopToken %d", i)
			continue
		}

		loadedToken := loadedResult.TopTokens[i]
		if originalToken.Token != loadedToken.Token {
			t.Errorf("TopToken %d token mismatch: original='%s', loaded='%s'",
				i, originalToken.Token, loadedToken.Token)
		}
		if originalToken.Count != loadedToken.Count {
			t.Errorf("TopToken %d count mismatch: original=%d, loaded=%d",
				i, originalToken.Count, loadedToken.Count)
		}
	}
}

func TestFreqClassResultSaveLoadEmpty(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "freqclass_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create an empty FreqClassResult
	result := FreqClassResult{
		Filters:   []FreqClassFilter{},
		TopTokens: []TokenCount{},
	}

	// Save to file
	filename := filepath.Join(tempDir, "empty_frequency_filters.gob")
	if err := result.SaveToFile(filename); err != nil {
		t.Fatalf("SaveToFile failed for empty result: %v", err)
	}

	// Load into a new result
	var loadedResult FreqClassResult
	if err := loadedResult.LoadFromFile(filename); err != nil {
		t.Fatalf("LoadFromFile failed for empty result: %v", err)
	}

	// Verify it's still empty
	if len(loadedResult.Filters) != 0 {
		t.Errorf("Expected empty filters, got: %d", len(loadedResult.Filters))
	}
	if len(loadedResult.TopTokens) != 0 {
		t.Errorf("Expected empty TopTokens, got: %d", len(loadedResult.TopTokens))
	}
}

func TestFreqClassResultSaveLoadSingleClass(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "freqclass_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test token counts with only a few tokens
	tokenCounts := map[string]int{
		"hello": 10,
		"world": 5,
		"test":  3,
	}

	// Build frequency class result with only 1 class
	result := BuildFrequencyClassHashSets(tokenCounts, 1, nil, nil)

	// Save to file
	filename := filepath.Join(tempDir, "single_class_filters.gob")
	if err := result.SaveToFile(filename); err != nil {
		t.Fatalf("SaveToFile failed: %v", err)
	}

	// Load into a new result
	var loadedResult FreqClassResult
	if err := loadedResult.LoadFromFile(filename); err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	// Verify the loaded data works correctly
	if len(loadedResult.Filters) != 1 {
		t.Errorf("Expected 1 filter, got %d", len(loadedResult.Filters))
	}

	// Test that the filter contains the expected tokens
	filter := loadedResult.Filters[0]
	for token := range tokenCounts {
		if !filter.Contains(token) {
			t.Errorf("Expected filter to contain token '%s'", token)
		}
	}

	// Test that it doesn't contain unexpected tokens
	if filter.Contains("nonexistent") {
		t.Error("Filter should not contain 'nonexistent' token")
	}
}

func TestFreqClassResultSaveLoadLargeDataset(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "freqclass_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a large dataset
	tokenCounts := make(map[string]int)
	for i := 0; i < 1000; i++ {
		token := fmt.Sprintf("token_%d", i)
		count := (i % 100) + 1 // Counts from 1 to 100
		tokenCounts[token] = count
	}

	// Build frequency class result
	result := BuildFrequencyClassHashSets(tokenCounts, 5, nil, nil)

	// Save to file
	filename := filepath.Join(tempDir, "large_frequency_filters.gob")
	if err := result.SaveToFile(filename); err != nil {
		t.Fatalf("SaveToFile failed for large dataset: %v", err)
	}

	// Load into a new result
	var loadedResult FreqClassResult
	if err := loadedResult.LoadFromFile(filename); err != nil {
		t.Fatalf("LoadFromFile failed for large dataset: %v", err)
	}

	// Verify the loaded data has the right structure
	if len(loadedResult.Filters) != 5 {
		t.Errorf("Expected 5 filters, got %d", len(loadedResult.Filters))
	}

	// Test that filters work correctly after loading
	for _, filter := range loadedResult.Filters {
		// Test a sample of tokens
		for j := 0; j < 100; j += 10 {
			token := fmt.Sprintf("token_%d", j)
			// We can't easily verify exact containment without knowing the original distribution
			// But we can verify the filter works without crashing
			_ = filter.Contains(token)
		}
	}
}

func TestFreqClassResultSaveLoadNonexistentFile(t *testing.T) {
	var result FreqClassResult

	// Try to load from a nonexistent file
	err := result.LoadFromFile("nonexistent_file.gob")
	if err == nil {
		t.Error("Expected error when loading from nonexistent file, got nil")
	}
}

func TestFreqClassResultSaveLoadCorruptedFile(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "freqclass_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a corrupted file
	filename := filepath.Join(tempDir, "corrupted.gob")
	if err := os.WriteFile(filename, []byte("this is not a gob file"), 0644); err != nil {
		t.Fatalf("Failed to create corrupted file: %v", err)
	}

	// Try to load from corrupted file
	var result FreqClassResult
	err = result.LoadFromFile(filename)
	if err == nil {
		t.Error("Expected error when loading from corrupted file, got nil")
	}
}

func TestFreqClassResultSaveLoadConcurrentAccess(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "freqclass_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test token counts
	tokenCounts := map[string]int{
		"hello": 10,
		"world": 5,
		"test":  3,
	}

	// Build frequency class result
	result := BuildFrequencyClassHashSets(tokenCounts, 2, nil, nil)

	// Save to file while other operations are happening
	filename := filepath.Join(tempDir, "concurrent_test.gob")

	// Start a goroutine that continuously accesses the filters
	done := make(chan bool)
	go func() {
		for i := 0; i < 100; i++ {
			for _, filter := range result.Filters {
				_ = filter.Contains("hello")
				_ = filter.Contains("world")
				_ = filter.Contains("test")
			}
		}
		done <- true
	}()

	// Save while the goroutine is running
	if err := result.SaveToFile(filename); err != nil {
		t.Fatalf("SaveToFile failed during concurrent access: %v", err)
	}

	// Wait for goroutine to finish
	<-done

	// Load the saved data
	var loadedResult FreqClassResult
	if err := loadedResult.LoadFromFile(filename); err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	// Verify we can still access the loaded data
	if len(loadedResult.Filters) == 0 {
		t.Error("Loaded filters are empty after concurrent save")
	}
}
