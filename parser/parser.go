package main

import (
	"bufio"
	"compress/gzip"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// StringOrNumber handles JSON fields that may be a string or a number
type StringOrNumber string

func (s *StringOrNumber) UnmarshalJSON(b []byte) error {
	// Try as string
	var str string
	if err := json.Unmarshal(b, &str); err == nil {
		*s = StringOrNumber(str)
		return nil
	}
	// Try as number
	var num int
	if err := json.Unmarshal(b, &num); err == nil {
		*s = StringOrNumber(fmt.Sprintf("%d", num))
		return nil
	}
	return fmt.Errorf("StringOrNumber: could not unmarshal %s", string(b))
}

type Tweet struct {
	IDStr        string         `json:"id_str"`
	Text         string         `json:"text"`
	CreatedAt    string         `json:"created_at"`
	RetweetCount StringOrNumber `json:"retweet_count"`
	Retweeted    bool           `json:"retweeted"`
	User         struct {
		IDStr string `json:"id_str"`
		Name  string `json:"name"`
	} `json:"user"`
}

func processJSONFile(inputPath, outputPath string) (int, int, error) {
	fmt.Printf("Processing %s -> %s\n", inputPath, outputPath)

	// Remove existing output file if it exists
	if _, err := os.Stat(outputPath); err == nil {
		os.Remove(outputPath)
	}

	// Open gzipped file
	file, err := os.Open(inputPath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create gzip reader: %v", err)
	}
	defer gzReader.Close()

	// Create output CSV file
	outFile, err := os.Create(outputPath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create output file: %v", err)
	}
	defer outFile.Close()

	writer := csv.NewWriter(outFile)
	defer writer.Flush()

	// Write CSV header
	header := []string{"id_str", "created_at", "user_id_str", "retweet_count", "text", "retweeted", "at", "http", "hashtag", "words"}
	if err := writer.Write(header); err != nil {
		return 0, 0, fmt.Errorf("failed to write header: %v", err)
	}

	scanner := bufio.NewScanner(gzReader)
	tweetCount := 0
	skipCount := 0

	for lineNum := 1; scanner.Scan(); lineNum++ {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines
		if line == "" {
			continue
		}

		// Skip opening and closing brackets (like GPT code)
		if line == "[" || line == "]" {
			continue
		}

		// Remove trailing comma (like GPT code)
		line = strings.TrimRight(line, ",")

		var tweet Tweet
		if err := json.Unmarshal([]byte(line), &tweet); err != nil {
			fmt.Printf("Failed to decode JSON at line %d: %v\n", lineNum, err)
			skipCount++
			continue
		}

		// Check if we have the required fields
		if tweet.IDStr == "" || tweet.Text == "" || tweet.User.IDStr == "" {
			skipCount++
			continue
		}

		// Count patterns
		atCount := strings.Count(tweet.Text, "@")
		httpCount := strings.Count(tweet.Text, "http")
		hashtagCount := strings.Count(tweet.Text, "#")

		// Extract words (simple approach)
		words := extractWords(tweet.Text)

		// Write CSV row
		row := []string{
			tweet.IDStr,
			tweet.CreatedAt,
			tweet.User.IDStr,
			string(tweet.RetweetCount),
			tweet.Text,
			fmt.Sprintf("%t", tweet.Retweeted),
			fmt.Sprintf("%d", atCount),
			fmt.Sprintf("%d", httpCount),
			fmt.Sprintf("%d", hashtagCount),
			words,
		}

		if err := writer.Write(row); err != nil {
			fmt.Printf("Failed to write row at line %d: %v\n", lineNum, err)
			skipCount++
			continue
		}

		tweetCount++
		if tweetCount%1000 == 0 {
			fmt.Printf("Processed %d tweets so far...\n", tweetCount)
		}
	}

	if err := scanner.Err(); err != nil {
		return tweetCount, skipCount, fmt.Errorf("scanner error: %v", err)
	}

	fmt.Printf("Processed %d tweets, skipped %d objects\n", tweetCount, skipCount)
	return tweetCount, skipCount, nil
}

func extractWords(text string) string {
	words := strings.Fields(text)
	var cleanWords []string
	for _, word := range words {
		// Remove punctuation and convert to lowercase
		word = strings.ToLower(strings.Trim(word, ".,!?;:\"()[]{}"))
		if len(word) > 1 && isAlpha(word) {
			cleanWords = append(cleanWords, word)
		}
	}
	return strings.Join(cleanWords, " ")
}

func isAlpha(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			return false
		}
	}
	return true
}

func main() {
	if len(os.Args) != 3 {
		fmt.Println("Usage: go run parser.go <input_dir> <output_dir>")
		os.Exit(1)
	}

	inputDir := os.Args[1]
	outputDir := os.Args[2]

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Find all .json.gz files
	pattern := filepath.Join(inputDir, "*.json.gz")
	files, err := filepath.Glob(pattern)
	if err != nil {
		log.Fatalf("Failed to find files: %v", err)
	}

	if len(files) == 0 {
		fmt.Printf("No .json.gz files found in %s\n", inputDir)
		os.Exit(1)
	}

	totalTweets := 0
	totalSkipped := 0

	for _, gzFile := range files {
		baseName := filepath.Base(gzFile)
		csvFile := filepath.Join(outputDir, strings.Replace(baseName, ".json.gz", ".csv", 1))

		tweets, skipped, err := processJSONFile(gzFile, csvFile)
		if err != nil {
			fmt.Printf("Failed to process %s: %v\n", gzFile, err)
			continue
		}

		totalTweets += tweets
		totalSkipped += skipped
	}

	fmt.Printf("Total: %d tweets, %d skipped\n", totalTweets, totalSkipped)
}
