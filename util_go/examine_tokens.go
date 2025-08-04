package main

import (
	"encoding/gob"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func main() {
	stateDir := flag.String("state-dir", "data/state", "Directory containing token batch files")
	flag.Parse()

	// Find all token batch files
	files, err := filepath.Glob(filepath.Join(*stateDir, "token_batch_*.gob"))
	if err != nil {
		fmt.Printf("Error finding token files: %v\n", err)
		os.Exit(1)
	}

	if len(files) == 0 {
		fmt.Printf("No token batch files found in %s\n", *stateDir)
		os.Exit(1)
	}

	// Sort files by name
	sort.Strings(files)

	// Examine the most recent file
	latestFile := files[len(files)-1]
	fmt.Printf("Examining latest token file: %s\n", latestFile)

	file, err := os.Open(latestFile)
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)

	// Try to decode as a slice of strings (tokens)
	var tokens []string
	err = decoder.Decode(&tokens)
	if err != nil {
		fmt.Printf("Error decoding tokens: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d tokens in file\n", len(tokens))

	// Count unique tokens
	tokenSet := make(map[string]int)
	for _, token := range tokens {
		tokenSet[token]++
	}

	fmt.Printf("Found %d unique tokens\n", len(tokenSet))

	// Show first 20 unique tokens as examples
	count := 0
	for token := range tokenSet {
		if count >= 20 {
			break
		}
		fmt.Printf("%s\n", token)
		count++
	}

	// Check for any tokens that look like they have whitespace issues
	fmt.Printf("\nChecking for potential whitespace issues...\n")
	issues := 0
	for token := range tokenSet {
		if len(token) > 20 && !containsSpace(token) {
			// Check if it looks like multiple words run together
			hasMixedCase := false
			prevUpper := false
			for i, char := range token {
				if i == 0 {
					prevUpper = char >= 'A' && char <= 'Z'
					continue
				}
				currentUpper := char >= 'A' && char <= 'Z'
				if prevUpper != currentUpper && char >= 'a' && char <= 'z' {
					hasMixedCase = true
					break
				}
				prevUpper = currentUpper
			}

			if hasMixedCase {
				fmt.Printf("Potential issue: '%s' (long token with mixed case)\n", token)
				issues++
			}
		}
	}

	if issues == 0 {
		fmt.Printf("No obvious whitespace issues found.\n")
	} else {
		fmt.Printf("Found %d potential whitespace issues.\n", issues)
	}
}

func containsSpace(s string) bool {
	for _, char := range s {
		if char == ' ' || char == '\t' || char == '\n' {
			return true
		}
	}
	return false
}
