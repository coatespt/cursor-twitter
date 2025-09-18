package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/streadway/amqp"

	"cursor-twitter/src/config"
)

// TweetInputSource represents the different input sources for tweets
type TweetInputSource interface {
	GetNextTweet() (string, error)
	Close() error
}

// RabbitMQInputSource implements TweetInputSource for RabbitMQ
type RabbitMQInputSource struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	msgs    <-chan amqp.Delivery
}

// NewRabbitMQInputSource creates a new RabbitMQ input source
func NewRabbitMQInputSource(cfg *config.Config) (*RabbitMQInputSource, error) {
	conn, ch, q, err := setupRabbitMQ(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to set up RabbitMQ: %w", err)
	}

	msgs, err := setupRabbitMQConsumer(ch, q)
	if err != nil {
		conn.Close()
		ch.Close()
		return nil, fmt.Errorf("failed to set up RabbitMQ consumer: %w", err)
	}

	return &RabbitMQInputSource{
		conn:    conn,
		channel: ch,
		msgs:    msgs,
	}, nil
}

// GetNextTweet returns the next message from RabbitMQ
func (r *RabbitMQInputSource) GetNextTweet() (string, error) {
	msg, ok := <-r.msgs
	if !ok {
		return "", io.EOF
	}

	// Acknowledge the message
	msg.Ack(false)
	return string(msg.Body), nil
}

// Close closes the RabbitMQ connection
func (r *RabbitMQInputSource) Close() error {
	if r.channel != nil {
		r.channel.Close()
	}
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil
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
func NewFileInputSource(cfg *config.Config) (*FileInputSource, error) {
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
func CreateTweetInputSource(cfg *config.Config) (TweetInputSource, error) {
	switch cfg.Mode {
	case "mqj":
		return NewRabbitMQInputSource(cfg)
	case "files":
		return NewFileInputSource(cfg)
	default:
		return nil, fmt.Errorf("invalid mode specified: %s (valid modes: mqj, files)", cfg.Mode)
	}
}

// Global input source - initialized on first call to getNextTweet()
var inputSource TweetInputSource

// getNextTweet() returns the next CSV tweet row
// Initializes input source on first call if needed
func getNextTweet(cfg *config.Config) (string, error) {
	if inputSource == nil {
		var err error
		inputSource, err = CreateTweetInputSource(cfg)
		if err != nil {
			return "", err
		}
	}
	return inputSource.GetNextTweet()
}
