package main

import (
	"compress/gzip"
	"encoding/csv"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var (
	CSV_COLS = []string{
		"id_str",
		"created_at",
		"user_id_str",
		"retweet_count",
		"text",
		"retweeted",
		"at",
		"http",
		"hashtag",
		"words",
		"threepk",
	}
	BIT_FIELD_LEN  = 1000
	UNIQUE_STRINGS = []string{"__0NE__", "__TW0__", "__THR33__"}
)

func main() {
	inputDir := flag.String("inputdir", "", "Path to input directory containing gzipped CSV files")
	outputDir := flag.String("outputdir", "", "Path to output directory for CSV files")
	flag.Parse()

	if *inputDir == "" || *outputDir == "" {
		log.Fatalf("Usage: parser -inputdir input_dir -outputdir output_dir")
	}

	files, err := filepath.Glob(filepath.Join(*inputDir, "*.gz"))
	if err != nil {
		log.Fatalf("Failed to list .gz files: %v", err)
	}
	if len(files) == 0 {
		log.Fatalf("No .gz files found in %s", *inputDir)
	}

	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	for _, gzFile := range files {
		base := filepath.Base(gzFile)
		outFile := filepath.Join(*outputDir, strings.TrimSuffix(base, ".gz")+".csv")
		log.Printf("Processing %s -> %s", gzFile, outFile)
		if err := processCSVFile(gzFile, outFile); err != nil {
			log.Printf("Failed to process %s: %v", gzFile, err)
		}
	}
}

func processCSVFile(inputPath, outputPath string) error {
	f, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("failed to open input: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to open gzip: %w", err)
	}
	defer gz.Close()

	reader := csv.NewReader(gz)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	outf, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output: %w", err)
	}
	defer outf.Close()
	writer := csv.NewWriter(outf)
	defer writer.Flush()

	head, err := reader.Read()
	if err != nil {
		return fmt.Errorf("failed to read header: %w", err)
	}
	if len(head) < 10 {
		return fmt.Errorf("expected at least 10 columns in header, got %d", len(head))
	}
	// Write header with threepk
	if err := writer.Write(CSV_COLS); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Skipping row: %v", err)
			continue
		}
		if len(row) < 10 {
			log.Printf("Skipping short row: %v", row)
			continue
		}
		// Generate three-part keys for words
		words := strings.Fields(row[9])
		wordDict := make(map[string][3]int)
		for _, word := range words {
			if isValidWord(word) {
				wordDict[word] = generateThreePartKey(word, BIT_FIELD_LEN)
			}
		}
		threepk := formatWordDictForCSV(wordDict)
		outRow := append(row, threepk)
		if err := writer.Write(outRow); err != nil {
			log.Printf("Failed to write row: %v", err)
		}
	}
	return nil
}

func isValidWord(word string) bool {
	return strings.TrimSpace(word) != ""
}

func generateThreePartKey(word string, maxValue int) [3]int {
	return [3]int{
		hashWithSuffix(word, UNIQUE_STRINGS[0], maxValue),
		hashWithSuffix(word, UNIQUE_STRINGS[1], maxValue),
		hashWithSuffix(word, UNIQUE_STRINGS[2], maxValue),
	}
}

func hashWithSuffix(word, suffix string, modulo int) int {
	h := fnv.New32a()
	h.Write([]byte(word + suffix))
	return int(h.Sum32()) % modulo
}

func formatWordDictForCSV(wordDict map[string][3]int) string {
	parts := make([]string, 0, len(wordDict)*4)
	for word, vals := range wordDict {
		parts = append(parts, word,
			fmt.Sprintf("%d", vals[0]),
			fmt.Sprintf("%d", vals[1]),
			fmt.Sprintf("%d", vals[2]))
	}
	return strings.Join(parts, " ")
}
