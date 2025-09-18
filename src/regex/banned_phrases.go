package regex

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// LoadBannedPhrasesFromFile loads banned phrase patterns from a single file
func LoadBannedPhrasesFromFile(filePath string) ([]*regexp.Regexp, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read banned phrases file %s: %w", filePath, err)
	}

	lines := strings.Split(string(content), "\n")
	var patterns []*regexp.Regexp

	for lineNum, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		// Compile the pattern (case-insensitive)
		pattern, err := CompileBannedPhrasePattern(line)
		if err != nil {
			return nil, fmt.Errorf("failed to compile pattern on line %d of %s: %w", lineNum+1, filePath, err)
		}
		patterns = append(patterns, pattern)
	}

	return patterns, nil
}

// LoadBannedPhrasesFromDirectory loads banned phrase patterns from all files in a directory
func LoadBannedPhrasesFromDirectory(dirPath string) ([]*regexp.Regexp, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read banned phrases directory %s: %w", dirPath, err)
	}

	var allPatterns []*regexp.Regexp

	for _, entry := range entries {
		if entry.IsDir() {
			continue // Skip subdirectories
		}

		filePath := filepath.Join(dirPath, entry.Name())
		patterns, err := LoadBannedPhrasesFromFile(filePath)
		if err != nil {
			return nil, err
		}
		allPatterns = append(allPatterns, patterns...)
	}

	return allPatterns, nil
}
