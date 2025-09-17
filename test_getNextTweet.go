//go:build ignore

// Test program for getNextTweet() function
// Run with: go run test_getNextTweet.go -config config/config.yaml
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Config struct (simplified version for testing)
type Config struct {
	Mode       string `yaml:"mode"`
	FileSrcDir string `yaml:"file_src_dir"`
}

// TweetInputSource represents the different input sources for tweets
type TweetInputSource interface {
	GetNextTweet() (string, error)
	Close() error
}

// FileInputSource implements TweetInputSource for CSV files
type FileInputSource struct {
	files      []string
	fileIndex  int
	reader     *csv.Reader
	file       *os.File
	headerRead bool
}

// NewFileInputSource creates a new file input source
func NewFileInputSource(cfg *Config) (*FileInputSource, error) {
	// Get list of CSV files in the directory
	files, err := filepath.Glob(filepath.Join(cfg.FileSrcDir, "*.csv"))
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", cfg.FileSrcDir, err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no CSV files found in directory %s", cfg.FileSrcDir)
	}

	// Sort files for consistent processing order
	sort.Strings(files)

	return &FileInputSource{
		files:      files,
		fileIndex:  0,
		headerRead: false,
	}, nil
}

// GetNextTweet returns the next line from the current CSV file
func (f *FileInputSource) GetNextTweet() (string, error) {
	for {
		// If we don't have a file open, try to open the next one
		if f.reader == nil {
			if f.fileIndex >= len(f.files) {
				return "", io.EOF
			}

			filePath := f.files[f.fileIndex]
			file, err := os.Open(filePath)
			if err != nil {
				return "", fmt.Errorf("failed to open file %s: %w", filePath, err)
			}

			f.file = file
			f.reader = csv.NewReader(file)
			f.headerRead = false
		}

		// Skip header if present (only once per file)
		if !f.headerRead {
			_, err := f.reader.Read()
			if err != nil {
				if err == io.EOF {
					// Empty file, move to next
					f.closeCurrentFile()
					f.fileIndex++
					continue
				}
				return "", fmt.Errorf("failed to read header from file %s: %w", f.files[f.fileIndex], err)
			}
			f.headerRead = true
		}

		// Read the next record
		record, err := f.reader.Read()
		if err == io.EOF {
			// End of current file, move to next
			f.closeCurrentFile()
			f.fileIndex++
			continue
		}
		if err != nil {
			return "", fmt.Errorf("failed to read CSV record from file %s: %w", f.files[f.fileIndex], err)
		}

		// Convert record to CSV row format (comma-separated string)
		return strings.Join(record, ","), nil
	}
}

// closeCurrentFile closes the currently open file and resets reader state
func (f *FileInputSource) closeCurrentFile() {
	if f.file != nil {
		f.file.Close()
		f.file = nil
	}
	f.reader = nil
	f.headerRead = false
}

// Close closes any open files
func (f *FileInputSource) Close() error {
	f.closeCurrentFile()
	return nil
}

// CreateTweetInputSource creates the appropriate input source based on config
func CreateTweetInputSource(cfg *Config) (TweetInputSource, error) {
	switch cfg.Mode {
	case "files":
		return NewFileInputSource(cfg)
	default:
		return nil, fmt.Errorf("invalid mode specified: %s (valid modes: files)", cfg.Mode)
	}
}

// Global input source - initialized on first call to getNextTweet()
var inputSource TweetInputSource

// getNextTweet() returns the next CSV tweet row
// Initializes input source on first call if needed
func getNextTweet(cfg *Config) (string, error) {
	if inputSource == nil {
		var err error
		inputSource, err = CreateTweetInputSource(cfg)
		if err != nil {
			return "", err
		}
	}
	return inputSource.GetNextTweet()
}

// Simple config loader for testing
func loadTestConfig(configPath string) (*Config, error) {
	// For testing, we'll just create a simple config
	// In real implementation, this would load from YAML
	return &Config{
		Mode:       "files",
		FileSrcDir: "../twits/test_language_detect_out/", // Use actual data directory
	}, nil
}

func main() {
	configPath := flag.String("config", "config/config.yaml", "Path to YAML config file")
	flag.Parse()

	// Load config
	cfg, err := loadTestConfig(*configPath)
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	fmt.Printf("Testing getNextTweet() function...\n")
	fmt.Printf("Making a few hundred thousand calls to ensure multiple file reading...\n\n")

	tweetCount := 0

	for {
		// Call getNextTweet() - this is what we're testing
		row, err := getNextTweet(cfg)
		if err != nil {
			if err == io.EOF {
				fmt.Printf("\nReached end of input after %d tweets\n", tweetCount)
				break
			}
			slog.Error("Error reading tweet", "error", err)
			continue
		}

		tweetCount++

		// Progress reporting
		if tweetCount%50000 == 0 {
			fmt.Printf("Read %d tweets...\n", tweetCount)
		}

		// Print first few tweets for verification
		if tweetCount <= 3 {
			fmt.Printf("Tweet %d: %s\n", tweetCount, row)
		}

		// Stop after reading a few hundred thousand tweets for testing
		if tweetCount >= 300000 {
			fmt.Printf("Stopping after %d tweets for testing\n", tweetCount)
			break
		}
	}

	fmt.Printf("\nTest completed successfully!\n")
	fmt.Printf("Total tweets read: %d\n", tweetCount)
}
