package filter

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// WordFilter holds a set of words to filter out
// Note: This is read-only after initialization, so no locking is needed
type WordFilter struct {
	filteredWords map[string]bool
}

// NewWordFilter creates a new empty WordFilter
func NewWordFilter() *WordFilter {
	return &WordFilter{
		filteredWords: make(map[string]bool),
	}
}

// LoadFromFile loads filtered words from a file
// Each line should contain one word, lines starting with # are comments
func (wf *WordFilter) LoadFromFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open filter file %s: %v", filename, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0
	loadedCount := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Convert to lowercase for case-insensitive matching
		word := strings.ToLower(line)
		wf.filteredWords[word] = true
		loadedCount++
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading filter file %s at line %d: %v", filename, lineNum, err)
	}

	return nil
}

// IsFiltered checks if a token should be filtered out
func (wf *WordFilter) IsFiltered(token string) bool {
	// Convert token to lowercase for case-insensitive matching
	token = strings.ToLower(token)
	return wf.filteredWords[token]
}

// GetFilteredCount returns the number of words in the filter
func (wf *WordFilter) GetFilteredCount() int {
	return len(wf.filteredWords)
}

// AddWord adds a single word to the filter
func (wf *WordFilter) AddWord(word string) {
	wf.filteredWords[strings.ToLower(word)] = true
}

// RemoveWord removes a word from the filter
func (wf *WordFilter) RemoveWord(word string) {
	delete(wf.filteredWords, strings.ToLower(word))
}
