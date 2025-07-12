package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestTokenCounterSaveLoad(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "tokencounter_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a TokenCounter with some test data
	tc := NewTokenCounter()
	testTokens := []string{"hello", "world", "test", "token", "example"}
	for _, token := range testTokens {
		tc.IncrementTokens([]string{token})
	}
	// Add some tokens multiple times
	tc.IncrementTokens([]string{"hello", "world"}) // hello=2, world=2
	tc.IncrementTokens([]string{"test"})           // test=2

	// Save to file
	filename := filepath.Join(tempDir, "test_counts.gob")
	if err := tc.SaveToFile(filename); err != nil {
		t.Fatalf("SaveToFile failed: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		t.Fatalf("Save file was not created: %s", filename)
	}

	// Create a new TokenCounter and load from file
	tc2 := NewTokenCounter()
	if err := tc2.LoadFromFile(filename); err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	// Verify the loaded data matches the original
	originalCounts := tc.Counts()
	loadedCounts := tc2.Counts()

	if !reflect.DeepEqual(originalCounts, loadedCounts) {
		t.Errorf("Loaded counts don't match original:\nOriginal: %v\nLoaded: %v", originalCounts, loadedCounts)
	}

	// Verify specific counts
	expectedCounts := map[string]int{
		"hello":   2,
		"world":   2,
		"test":    2,
		"token":   1,
		"example": 1,
	}

	for token, expectedCount := range expectedCounts {
		if actualCount := tc2.GetCount(token); actualCount != expectedCount {
			t.Errorf("Token '%s': expected count %d, got %d", token, expectedCount, actualCount)
		}
	}
}

func TestTokenCounterSaveLoadEmpty(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "tokencounter_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create an empty TokenCounter
	tc := NewTokenCounter()

	// Save to file
	filename := filepath.Join(tempDir, "empty_counts.gob")
	if err := tc.SaveToFile(filename); err != nil {
		t.Fatalf("SaveToFile failed for empty counter: %v", err)
	}

	// Load into a new counter
	tc2 := NewTokenCounter()
	if err := tc2.LoadFromFile(filename); err != nil {
		t.Fatalf("LoadFromFile failed for empty counter: %v", err)
	}

	// Verify it's still empty
	counts := tc2.Counts()
	if len(counts) != 0 {
		t.Errorf("Expected empty counts, got: %v", counts)
	}
}

func TestTokenCounterSaveLoadLargeDataset(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "tokencounter_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a TokenCounter with a large dataset
	tc := NewTokenCounter()

	// Add many tokens with various counts
	for i := 0; i < 1000; i++ {
		token := fmt.Sprintf("token_%d", i)
		count := (i % 10) + 1 // Counts from 1 to 10
		for j := 0; j < count; j++ {
			tc.IncrementTokens([]string{token})
		}
	}

	// Save to file
	filename := filepath.Join(tempDir, "large_counts.gob")
	if err := tc.SaveToFile(filename); err != nil {
		t.Fatalf("SaveToFile failed for large dataset: %v", err)
	}

	// Load into a new counter
	tc2 := NewTokenCounter()
	if err := tc2.LoadFromFile(filename); err != nil {
		t.Fatalf("LoadFromFile failed for large dataset: %v", err)
	}

	// Verify the loaded data matches the original
	originalCounts := tc.Counts()
	loadedCounts := tc2.Counts()

	if !reflect.DeepEqual(originalCounts, loadedCounts) {
		t.Errorf("Loaded counts don't match original for large dataset")
		t.Errorf("Original count: %d, Loaded count: %d", len(originalCounts), len(loadedCounts))
	}
}

func TestTokenCounterSaveLoadNonexistentFile(t *testing.T) {
	tc := NewTokenCounter()

	// Try to load from a nonexistent file
	err := tc.LoadFromFile("nonexistent_file.gob")
	if err == nil {
		t.Error("Expected error when loading from nonexistent file, got nil")
	}
}

func TestTokenCounterSaveLoadCorruptedFile(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "tokencounter_test")
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
	tc := NewTokenCounter()
	err = tc.LoadFromFile(filename)
	if err == nil {
		t.Error("Expected error when loading from corrupted file, got nil")
	}
}

func TestTokenCounterSaveLoadConcurrentAccess(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "tokencounter_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a TokenCounter with some data
	tc := NewTokenCounter()
	tc.IncrementTokens([]string{"test1", "test2", "test3"})

	// Save to file while other operations are happening
	filename := filepath.Join(tempDir, "concurrent_test.gob")

	// Start a goroutine that continuously modifies the counter
	done := make(chan bool)
	go func() {
		for i := 0; i < 100; i++ {
			tc.IncrementTokens([]string{fmt.Sprintf("concurrent_%d", i)})
		}
		done <- true
	}()

	// Save while the goroutine is running
	if err := tc.SaveToFile(filename); err != nil {
		t.Fatalf("SaveToFile failed during concurrent access: %v", err)
	}

	// Wait for goroutine to finish
	<-done

	// Load the saved data
	tc2 := NewTokenCounter()
	if err := tc2.LoadFromFile(filename); err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	// Verify we can still access the loaded data
	counts := tc2.Counts()
	if len(counts) == 0 {
		t.Error("Loaded counts are empty after concurrent save")
	}
}

func TestTokenCounterRunningTotal(t *testing.T) {
	// Create a TokenCounter
	tc := NewTokenCounter()

	// Test initial state
	if total := tc.GetTotalTokens(); total != 0 {
		t.Errorf("Expected initial total 0, got %d", total)
	}

	// Test incrementing tokens
	tokens := []string{"hello", "world", "hello", "test"}
	tc.IncrementTokens(tokens)

	// Verify total is correct
	if total := tc.GetTotalTokens(); total != 4 {
		t.Errorf("Expected total 4 after incrementing, got %d", total)
	}

	// Test decrementing tokens
	tc.DecrementTokens([]string{"hello", "world"})

	// Verify total is updated correctly
	if total := tc.GetTotalTokens(); total != 2 {
		t.Errorf("Expected total 2 after decrementing, got %d", total)
	}

	// Test clearing
	tc.Clear()
	if total := tc.GetTotalTokens(); total != 0 {
		t.Errorf("Expected total 0 after clearing, got %d", total)
	}

	// Test with large numbers
	largeTokens := make([]string, 1000)
	for i := range largeTokens {
		largeTokens[i] = fmt.Sprintf("token%d", i)
	}
	tc.IncrementTokens(largeTokens)
	if total := tc.GetTotalTokens(); total != 1000 {
		t.Errorf("Expected total 1000 after large increment, got %d", total)
	}
}

func TestTokenCounterSetCountsDirectly(t *testing.T) {
	// Create a TokenCounter
	tc := NewTokenCounter()

	// Test initial state
	if total := tc.GetTotalTokens(); total != 0 {
		t.Errorf("Expected initial total 0, got %d", total)
	}

	// Create test counts
	testCounts := map[string]int{
		"hello": 5,
		"world": 3,
		"test":  2,
	}

	// Set counts directly
	tc.SetCountsDirectly(testCounts)

	// Verify total is correct
	expectedTotal := 5 + 3 + 2
	if total := tc.GetTotalTokens(); total != expectedTotal {
		t.Errorf("Expected total %d after SetCountsDirectly, got %d", expectedTotal, total)
	}

	// Verify individual counts
	if count := tc.GetCount("hello"); count != 5 {
		t.Errorf("Expected count 5 for 'hello', got %d", count)
	}
	if count := tc.GetCount("world"); count != 3 {
		t.Errorf("Expected count 3 for 'world', got %d", count)
	}
	if count := tc.GetCount("test"); count != 2 {
		t.Errorf("Expected count 2 for 'test', got %d", count)
	}

	// Test with large dataset
	largeCounts := make(map[string]int, 10000)
	for i := 0; i < 10000; i++ {
		largeCounts[fmt.Sprintf("token%d", i)] = i + 1
	}

	startTime := time.Now()
	tc.SetCountsDirectly(largeCounts)
	duration := time.Since(startTime)

	// Verify it's fast (should be under 100ms for 10k tokens)
	if duration > 100*time.Millisecond {
		t.Errorf("SetCountsDirectly took too long: %v for 10k tokens", duration)
	}

	// Verify total is correct
	expectedLargeTotal := 0
	for _, count := range largeCounts {
		expectedLargeTotal += count
	}
	if total := tc.GetTotalTokens(); total != expectedLargeTotal {
		t.Errorf("Expected total %d for large dataset, got %d", expectedLargeTotal, total)
	}
}
