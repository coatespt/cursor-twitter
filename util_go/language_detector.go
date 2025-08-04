package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/pemistahl/lingua-go"
)

// CSV structure: id_str, created_at, user_id_str, retweet_count, text, retweeted, at, http, hashtag, words, lang
const (
	CSV_ID_STR        = 0
	CSV_CREATED_AT    = 1
	CSV_USER_ID_STR   = 2
	CSV_RETWEET_COUNT = 3
	CSV_TEXT          = 4
	CSV_RETWEETED     = 5
	CSV_AT_COUNT      = 6
	CSV_HTTP_COUNT    = 7
	CSV_HASHTAG_COUNT = 8
	CSV_WORDS         = 9
	CSV_LANG          = 10
)

// FileResult represents the result of processing a single file
type FileResult struct {
	FileName       string
	TweetCount     int
	ProcessedCount int
	Error          error
}

func main() {
	inputDir := flag.String("input", "", "Input directory containing CSV files")
	outputDir := flag.String("output", "", "Output directory for updated CSV files")
	numWorkers := flag.Int("workers", runtime.NumCPU(), "Number of worker goroutines (default: number of CPU cores)")
	showProgress := flag.Bool("progress", true, "Show progress updates (may impact performance)")
	flag.Parse()

	if *inputDir == "" || *outputDir == "" {
		log.Fatal("Both -input and -output directories must be specified")
	}

	// Create output directory
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Get list of CSV files
	files, err := filepath.Glob(filepath.Join(*inputDir, "*.csv"))
	if err != nil {
		log.Fatalf("Failed to find CSV files: %v", err)
	}

	if len(files) == 0 {
		log.Fatalf("No CSV files found in %s", *inputDir)
	}

	fmt.Printf("Found %d CSV files to process with %d workers\n", len(files), *numWorkers)

	// Process files with multiple workers
	startTime := time.Now()
	results := processFilesConcurrently(files, *outputDir, *numWorkers, *showProgress)

	// Collect results
	totalTweets := 0
	totalProcessed := 0
	errors := 0

	for _, result := range results {
		if result.Error != nil {
			log.Printf("Error processing %s: %v", result.FileName, result.Error)
			errors++
		} else {
			totalTweets += result.TweetCount
			totalProcessed += result.ProcessedCount
		}
	}

	elapsed := time.Since(startTime)
	fmt.Printf("\nCompleted in %v\n", elapsed)
	fmt.Printf("Total tweets processed: %d\n", totalTweets)
	fmt.Printf("Total language detections: %d\n", totalProcessed)
	fmt.Printf("Files with errors: %d\n", errors)
	fmt.Printf("Average speed: %.0f tweets/sec\n", float64(totalTweets)/elapsed.Seconds())
}

func processFilesConcurrently(files []string, outputDir string, numWorkers int, showProgress bool) []FileResult {
	// Create channels for work distribution and result collection
	fileChan := make(chan string, len(files))
	resultChan := make(chan FileResult, len(files))

	// Start worker goroutines
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			worker(files, outputDir, fileChan, resultChan, workerID, showProgress)
		}(i)
	}

	// Send files to workers
	go func() {
		for _, file := range files {
			fileChan <- file
		}
		close(fileChan)
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	var results []FileResult
	for result := range resultChan {
		results = append(results, result)
	}

	return results
}

func worker(files []string, outputDir string, fileChan <-chan string, resultChan chan<- FileResult, workerID int, showProgress bool) {
	// Initialize language detector for this worker
	detector := lingua.NewLanguageDetectorBuilder().
		FromLanguages(
			lingua.English,
			lingua.Spanish,
			lingua.French,
			lingua.German,
			lingua.Portuguese,
			lingua.Italian,
			lingua.Dutch,
			lingua.Swedish,
			lingua.Danish,
			lingua.Finnish,
			lingua.Polish,
			lingua.Russian,
			lingua.Japanese,
			lingua.Chinese,
			lingua.Korean,
			lingua.Arabic,
			lingua.Hindi,
			lingua.Turkish,
			lingua.Hebrew,
		).
		WithLowAccuracyMode().
		Build()

	for file := range fileChan {
		baseName := filepath.Base(file)
		outputFile := filepath.Join(outputDir, baseName)

		// Check if output file already exists (skip if already processed)
		if _, err := os.Stat(outputFile); err == nil {
			if showProgress {
				fmt.Printf("[Worker %d] Skipping %s (already processed)\n", workerID, baseName)
			}
			resultChan <- FileResult{
				FileName:       baseName,
				TweetCount:     0,
				ProcessedCount: 0,
				Error:          nil,
			}
			continue
		}

		if showProgress {
			fmt.Printf("[Worker %d] Processing %s...\n", workerID, baseName)
		}

		tweets, processed, err := processCSVFile(file, outputFile, detector, showProgress)

		resultChan <- FileResult{
			FileName:       baseName,
			TweetCount:     tweets,
			ProcessedCount: processed,
			Error:          err,
		}
	}
}

func processCSVFile(inputPath, outputPath string, detector lingua.LanguageDetector, showProgress bool) (int, int, error) {
	inputFile, err := os.Open(inputPath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to open input file: %v", err)
	}
	defer inputFile.Close()

	outputFile, err := os.Create(outputPath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create output file: %v", err)
	}
	defer outputFile.Close()

	reader := csv.NewReader(inputFile)
	writer := csv.NewWriter(outputFile)
	defer writer.Flush()

	// Read and write header
	header, err := reader.Read()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read header: %v", err)
	}
	if err := writer.Write(header); err != nil {
		return 0, 0, fmt.Errorf("failed to write header: %v", err)
	}

	tweetCount := 0
	processedCount := 0
	startTime := time.Now()
	lastReportTime := startTime

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return tweetCount, processedCount, fmt.Errorf("failed to read record: %v", err)
		}

		tweetCount++

		// Report progress every 5000 tweets or every 10 seconds (less frequent for better performance)
		if tweetCount%5000 == 0 || time.Since(lastReportTime) > 10*time.Second {
			elapsed := time.Since(startTime).Seconds()
			if elapsed > 0 {
				tps := float64(tweetCount) / elapsed
				if showProgress {
					fmt.Printf("\r  [%s] Processed %d tweets [%.0f tweets/sec]",
						filepath.Base(inputPath), tweetCount, tps)
				}
			}
			lastReportTime = time.Now()
		}

		// Ensure we have enough fields
		if len(record) < 11 {
			// Write record as-is if it doesn't have enough fields
			if err := writer.Write(record); err != nil {
				return tweetCount, processedCount, fmt.Errorf("failed to write record: %v", err)
			}
			continue
		}

		// Get text from the text field
		text := record[CSV_TEXT]

		// Skip empty or very short text
		if len(strings.TrimSpace(text)) < 10 {
			// Write record as-is for short text
			if err := writer.Write(record); err != nil {
				return tweetCount, processedCount, fmt.Errorf("failed to write record: %v", err)
			}
			continue
		}

		// Detect language
		detectedLang, _ := detector.DetectLanguageOf(text)
		if detectedLang == lingua.Unknown {
			// Keep original language if detection fails
			detectedLang = lingua.English // Default to English
		}

		// Update the language field (last field)
		record[CSV_LANG] = strings.ToLower(detectedLang.IsoCode639_1().String())

		// Write updated record
		if err := writer.Write(record); err != nil {
			return tweetCount, processedCount, fmt.Errorf("failed to write record: %v", err)
		}

		processedCount++
	}

	// Final progress report
	elapsed := time.Since(startTime).Seconds()
	if elapsed > 0 {
		tps := float64(tweetCount) / elapsed
		if showProgress {
			fmt.Printf("\r  [%s] Completed: %d tweets, %d detections [%.0f tweets/sec]\n",
				filepath.Base(inputPath), tweetCount, processedCount, tps)
		}
	}

	return tweetCount, processedCount, nil
}
