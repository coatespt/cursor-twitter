package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CSV structure: id_str, created_at, user_id_str, retweet_count, text, retweeted, at, http, hashtag, words, lang
const (
	TF_CSV_ID_STR        = 0
	TF_CSV_CREATED_AT    = 1
	TF_CSV_USER_ID_STR   = 2
	TF_CSV_RETWEET_COUNT = 3
	TF_CSV_TEXT          = 4
	TF_CSV_RETWEETED     = 5
	TF_CSV_AT_COUNT      = 6
	TF_CSV_HTTP_COUNT    = 7
	TF_CSV_HASHTAG_COUNT = 8
	TF_CSV_WORDS         = 9 // Pre-parsed tokens field
	TF_CSV_LANG          = 10
)

// TokenCount represents a token and its count
type TokenCount struct {
	Token string
	Count int
}

func main() {
	inputDir := flag.String("input", "", "Input directory containing CSV files")
	outputFile := flag.String("output", "global_frequency.csv", "Output file for frequency analysis")
	languageFilter := flag.String("lang", "", "Filter by language (e.g., 'en' for English only, empty for all)")
	flag.Parse()

	if *inputDir == "" {
		fmt.Println("Usage: ./token_frequency_analyzer -input <directory> [-output <file>] [-lang <language>]")
		fmt.Println("Example: ./token_frequency_analyzer -input /path/to/csv -lang en")
		os.Exit(1)
	}

	// Get all CSV files in the directory
	files, err := filepath.Glob(filepath.Join(*inputDir, "*.csv"))
	if err != nil {
		log.Fatalf("Failed to find CSV files: %v", err)
	}

	if len(files) == 0 {
		log.Fatalf("No CSV files found in %s", *inputDir)
	}

	fmt.Printf("Found %d CSV files to process\n", len(files))
	if *languageFilter != "" {
		fmt.Printf("Filtering for language: %s\n", *languageFilter)
	}

	// Process all files and collect token counts
	tokenCounts := make(map[string]int)
	totalTweets := 0
	totalTokens := 0

	for i, file := range files {
		fmt.Printf("Processing file %d/%d: %s\n", i+1, len(files), filepath.Base(file))

		tweets, tokens, err := processCSVFile(file, tokenCounts, *languageFilter)
		if err != nil {
			log.Printf("Error processing %s: %v", file, err)
			continue
		}

		totalTweets += tweets
		totalTokens += tokens
	}

	fmt.Printf("\nProcessed %d tweets with %d total tokens\n", totalTweets, totalTokens)
	fmt.Printf("Found %d unique tokens\n", len(tokenCounts))

	// Convert to slice for sorting
	var tokenList []TokenCount
	for token, count := range tokenCounts {
		tokenList = append(tokenList, TokenCount{Token: token, Count: count})
	}

	// Sort by count (descending)
	sort.Slice(tokenList, func(i, j int) bool {
		return tokenList[i].Count > tokenList[j].Count
	})

	// Write results to output file
	if err := writeFrequencyFile(*outputFile, tokenList, totalTokens); err != nil {
		log.Fatalf("Failed to write output file: %v", err)
	}

	fmt.Printf("Frequency analysis written to: %s\n", *outputFile)
	fmt.Printf("Top 10 tokens:\n")
	for i := 0; i < 10 && i < len(tokenList); i++ {
		relativeFreq := float64(tokenList[i].Count) / float64(totalTokens)
		fmt.Printf("  %s: %d (%.6f)\n", tokenList[i].Token, tokenList[i].Count, relativeFreq)
	}
}

func processCSVFile(filePath string, tokenCounts map[string]int, languageFilter string) (int, int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)

	// Read header
	_, err = reader.Read()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read header: %v", err)
	}

	tweetCount := 0
	totalTokens := 0

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return tweetCount, totalTokens, fmt.Errorf("failed to read record: %v", err)
		}

		// Ensure we have enough fields
		if len(record) < 11 {
			continue
		}

		// Apply language filter if specified
		if languageFilter != "" {
			if strings.ToLower(record[TF_CSV_LANG]) != strings.ToLower(languageFilter) {
				continue
			}
		}

		tweetCount++

		// Get tokens from the words field (pre-parsed tokens)
		tokensStr := record[TF_CSV_WORDS]
		if tokensStr == "" {
			continue
		}

		// Split tokens (they're space-separated in the field)
		tokens := strings.Fields(tokensStr)
		for _, token := range tokens {
			// Normalize token (already lowercase from parser, but just in case)
			normalizedToken := strings.ToLower(strings.TrimSpace(token))
			if normalizedToken != "" {
				tokenCounts[normalizedToken]++
				totalTokens++
			}
		}
	}

	return tweetCount, totalTokens, nil
}

func writeFrequencyFile(outputPath string, tokenList []TokenCount, totalTokens int) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer file.Close()

	// Write header
	header := "rank\ttoken\tcount\tfrequency\tcumulative_frequency\n"
	if _, err := file.WriteString(header); err != nil {
		return fmt.Errorf("failed to write header: %v", err)
	}

	// Write data with cumulative frequency
	cumulativeFreq := 0.0
	for i, tc := range tokenList {
		relativeFreq := float64(tc.Count) / float64(totalTokens)
		cumulativeFreq += relativeFreq

		row := fmt.Sprintf("%d\t%s\t%d\t%.8f\t%.8f\n",
			i+1, // Rank (1-based)
			tc.Token,
			tc.Count,
			relativeFreq,
			cumulativeFreq,
		)
		if _, err := file.WriteString(row); err != nil {
			return fmt.Errorf("failed to write row: %v", err)
		}
	}

	return nil
}
