package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"cursor-twitter/src/pipeline"
	"cursor-twitter/src/tweets"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"
	"gopkg.in/yaml.v3"
)

// Config struct for YAML config file (add log_dir)
type Config struct {
	Mode                     string `yaml:"mode"`
	InputDir                 string `yaml:"input"`
	MQHost                   string `yaml:"mq_host"`
	MQPort                   int    `yaml:"mq_port"`
	MQQueue                  string `yaml:"mq_queue"`
	WindowSize               int    `yaml:"window"`
	BatchSize                int    `yaml:"batch"`
	Throttle                 int    `yaml:"throttle"`
	Verbose                  bool   `yaml:"verbose"`
	LogDir                   string `yaml:"log_dir"`
	FreqClassIntervalMinutes int    `yaml:"freq_class_interval_minutes"`
	FreqClasses              int    `yaml:"freq_classes"`
	EnableThrottling         bool   `yaml:"enable_throttling"`
	MaxRebuildTimeSeconds    int    `yaml:"max_rebuild_time_seconds"`
}

// GlobalTokenCounter keeps track of token counts in the current window.
var GlobalTokenCounter = pipeline.NewTokenCounter()

// Global stats counters
var (
	TotalTweetsRead    int
	TotalTokensCounted int
)

// Sliding window management
var (
	tweetQueue   []*tweets.Tweet // Queue of tweets in the current window
	tweetQueueMu sync.RWMutex    // Mutex for thread-safe access to tweet queue
)

// Add a global variable to hold the stats CSV file path
var statsCSVPath string

// Global Bloom filters and last rebuild time
var (
	GlobalFilters            []pipeline.FreqClassFilter
	lastFreqClassRebuildTime time.Time
)

// Add a global variable to hold the interval start time
var intervalStartUnix int64

// Global asynchronous frequency class manager
var asyncFreqClassManager *pipeline.AsyncFreqClassManager

func main() {
	// Add a command line flag to control printing of tweets
	printTweets := flag.Bool("print-tweets", true, "Print each parsed tweet to the console")
	configPath := flag.String("config", "../config/config.yaml", "Path to YAML config file")
	flag.Parse()

	// Load config from YAML file
	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Require log_dir to be present and non-empty
	if cfg.LogDir == "" {
		fmt.Fprintln(os.Stderr, "ERROR: 'log_dir' must be defined in the config file and cannot be empty.")
		os.Exit(1)
	}

	// Set up slog logger to write to a file in the specified log_dir
	logger, logFile, err := setupLogger(cfg.LogDir)
	if err != nil {
		log.Fatalf("Failed to set up logger: %v", err)
	}
	defer logFile.Close()
	slog.SetDefault(logger)

	// Set up stats CSV file path
	statsCSVPath = filepath.Join(cfg.LogDir, "stats.csv")
	ensureStatsCSVHeader(statsCSVPath)

	slog.Info("Starting simple RabbitMQ consumer")

	// Start periodic stats printer (prints every 30 seconds)
	startStatsPrinter()

	// Initialize and start the asynchronous frequency class manager
	asyncFreqClassManager = pipeline.NewAsyncFreqClassManager(GlobalTokenCounter)
	asyncFreqClassManager.Start()
	defer asyncFreqClassManager.Stop()

	// Connect to RabbitMQ
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		slog.Error("Failed to connect to RabbitMQ", "error", err)
		return
	}
	defer conn.Close()

	// Create channel
	ch, err := conn.Channel()
	if err != nil {
		slog.Error("Failed to open channel", "error", err)
		return
	}
	defer ch.Close()

	// Declare queue
	q, err := ch.QueueDeclare(
		"tweet_in", // name
		true,       // durable
		false,      // delete when unused
		false,      // exclusive
		false,      // no-wait
		nil,        // arguments
	)
	if err != nil {
		slog.Error("Failed to declare queue", "error", err)
		return
	}

	slog.Info("Connected to RabbitMQ. Waiting for messages...", "queue", q.Name)

	// Start blocking consumer
	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		slog.Error("Failed to register a consumer", "error", err)
		return
	}
// TODO:Add Global stats including tweet count, token count, distinct token count
//     (available from map size), 
//    number of W window cycles, number of pipeline cycles, etc.
//
// TODO: Nothing like maxRebuildTime should exist. Check that this does not exist.


// TODO: Create the TweetWindowQueue      This will not be allowed to grow beyond WindowSize
// TODO: Create the InboundTokenQueue
// TODO: Create the OldTokenQueue
// TODO: Create the InboundTweetQueue 


// TODO: Create a FrequencyComputationThread
//       THE FCT take tokens from the InboundTokenQueue and register them in 
//       the CountMap.
//       It also takes tokens from the OldTokenQueue and decrements the 
counts in the CountMap.
//       It also checks a flag to see if it should stop taking tokens and 
//       compute the frequency class filters.  If so, 
//             it copies the CountMap to a new array 
//             clears the existing CountMap.
//       It also sets the flag to resume taking tokens.
//       It also copies the CountMap to a new array and clears the CountMap.
//       It also sets the flag to resume taking tokens. 
//       It also sets the flag to resume taking tokens.
//       It also sets the flag to resume taking tokens.
 
	// Main loop runs forever
	for msg := range msgs {
		// TODOCheck if we should throttle ingestion. I don't think this is how to do it.
		maxRebuildTime := cfg.MaxRebuildTimeSeconds
		if maxRebuildTime <= 0 {
			maxRebuildTime = 5 // Default to 5 seconds
		}
		if cfg.EnableThrottling && asyncFreqClassManager.ShouldThrottleIngestion(maxRebuildTime) {
			delay := asyncFreqClassManager.GetThrottleDelay(maxRebuildTime)
			slog.Info("Throttling ingestion", "delay", delay)
			time.Sleep(delay)
		}
		tweet, err := parseCSVToTweet(string(msg.Body))
		if err != nil {
			//slog.Warn("Failed to parse tweet", "error", err, "raw_row", string(msg.Body))
			fmt.Printf("[PARSE ERROR] %v\nRaw: %s\n", err, string(msg.Body))
			continue
		}
		// TODO: Write tweet to the TweetWindowQueue
		// TODO: Write tweet tokens to the InboundTokenQueue
		// TODO: If TweetWindowQueue reaches WindowSize, remove the oldest tweet
		//     and write its tokens to the OldTokenQueue
		// TODO: 

		// Only print the tweet if the flag is set
		if *printTweets {
			fmt.Printf("Parsed Tweet: %+v\n", tweet)
		}

		// Manage the sliding window - add new tweet and remove old ones
		// TODO: This is wrong. WindowSize is a constant set in configuration. A queue called 
		// Inbound Tweet structs are stored on a Queue called TweetWindowQueue.
		// The new inbound tweet's tokens are placed on an InboundTokenQueue.
		// if the TweetWindowQueue reaches WindowSize, the oldest tweet is removed and
		// it's tokens are placed on an OldTokenQueue.


		windowSize := cfg.WindowSize
		if windowSize <= 0 {
			windowSize = 15 // Default to 15 minutes if not configured
		}
		manageSlidingWindow(tweet, windowSize)

		// Interval-based frequency class/Bloom filter rebuilding
		// When a frequency interval has passed we compute the global frequency table,
		// partition the tokens into F frequency classes, and build the Bloom filters.
		// The Bloom filters are used to filter the tokens into the correct frequency class.

		interval := int64(cfg.WindowSize) * 60 // interval in seconds
		if intervalStartUnix == 0 {
			intervalStartUnix = tweet.Unix - (tweet.Unix % interval)
		}
		if tweet.Unix >= intervalStartUnix+interval {
			// Trigger asynchronous frequency class rebuild
			rebuildTriggered := asyncFreqClassManager.TriggerRebuild()

			// Advance to the start of the interval containing this tweet
			intervalStartUnix = tweet.Unix - (tweet.Unix % interval)

			if rebuildTriggered {
				slog.Info("Triggered asynchronous frequency class rebuild", "tweet_time", tweet.CreatedAt.Format(time.RFC3339), "window_minutes", cfg.WindowSize)
			} else {
				slog.Info("Frequency class rebuild throttled", "tweet_time", tweet.CreatedAt.Format(time.RFC3339), "window_minutes", cfg.WindowSize)
			}
		}
	}
}

// loadConfig loads the YAML config file into a Config struct.
func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// setupLogger creates the log directory if needed and returns a slog.Logger that writes to a file.
func setupLogger(logDir string) (*slog.Logger, *os.File, error) {
	// No default! logDir must be set by config and checked in main()
	if logDir == "" {
		return nil, nil, fmt.Errorf("logDir must be set in config; refusing to use a default")
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, nil, err
	}
	logPath := filepath.Join(logDir, "pipeline.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, nil, err
	}
	logger := slog.New(slog.NewTextHandler(logFile, nil))
	return logger, logFile, nil
}

// startStatsPrinter launches a goroutine that prints stats every 30 seconds.
func startStatsPrinter() {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for range ticker.C {
			printStats()
		}
	}()
}

// ensureStatsCSVHeader creates the stats CSV file and writes the header if it doesn't exist.
func ensureStatsCSVHeader(path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Printf("Failed to create stats CSV: %v", err)
			return
		}
		defer f.Close()
		writer := csv.NewWriter(f)
		writer.Write([]string{"timestamp", "total_tweets", "total_tokens", "distinct_tokens"})
		writer.Flush()
	}
}

// printStats prints the current pipeline statistics and logs them as CSV.
func printStats() {
	timestamp := time.Now().Format(time.RFC3339)
	totalTweets := TotalTweetsRead
	totalTokens := TotalTokensCounted
	distinctTokens := len(GlobalTokenCounter.Counts())

	// Get sliding window stats
	tweetQueueMu.RLock()
	windowSize := len(tweetQueue)
	tweetQueueMu.RUnlock()

	fmt.Printf("\n--- Pipeline Stats ---\n")
	fmt.Printf("Total tweets read: %d\n", totalTweets)
	fmt.Printf("Total tokens counted: %d\n", totalTokens)
	fmt.Printf("Distinct tokens: %d\n", distinctTokens)
	fmt.Printf("Tweets in current window: %d\n", windowSize)
	fmt.Printf("----------------------\n")
	// Also log to slog
	slog.Info("Pipeline stats", "tweets", totalTweets, "tokens", totalTokens, "distinct", distinctTokens, "window_size", windowSize)

	// Log as CSV for machine consumption
	f, err := os.OpenFile(statsCSVPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("Failed to open stats CSV: %v", err)
		return
	}
	defer f.Close()
	writer := csv.NewWriter(f)
	writer.Write([]string{
		timestamp,
		fmt.Sprintf("%d", totalTweets),
		fmt.Sprintf("%d", totalTokens),
		fmt.Sprintf("%d", distinctTokens),
	})
	writer.Flush()
}

// parseCSVToTweet parses a CSV row string into a Tweet struct, 
// tokenizes the text, generates ThreePartKeys, and updates the global 
// token counter.
func parseCSVToTweet(row string) (*tweets.Tweet, error) {
	reader := csv.NewReader(strings.NewReader(row))
	reader.FieldsPerRecord = -1
	record, err := reader.Read()
	if err != nil {
		return nil, err
	}
	if len(record) < 10 {
		return nil, fmt.Errorf("expected at least 10 fields, got %d", len(record))
	}
	// Skip header rows
	if record[0] == "id_str" || record[1] == "created_at" {
		return nil, fmt.Errorf("header row detected, skipping")
	}

	// Normalize all whitespace to a single space
	cleanTime := normalizeWhitespace(record[1])

	createdAt, err := time.Parse("Mon Jan 2 15:04:05 -0700 2006", cleanTime)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CreatedAt: %v", err)
	}

	// Create the Tweet struct and fill in the basic fields from the CSV
	tweet := &tweets.Tweet{
		IDStr:        record[0],
		CreatedAt:    createdAt,
		Unix:         createdAt.Unix(),
		UserIDStr:    record[2],
		RetweetCount: record[3],
		Text:         record[4],
		Retweeted:    record[5],
		At:           record[6],
		Http:         record[7],
		Hashtag:      record[8],
		ThreePKs:     nil, // We'll fill this in below
		Tokens:       nil, // We'll fill this in below
	}

	// --- BEGINNER-FRIENDLY COMMENTS BELOW ---

	// Step 1: Tokenize the tweet text.
	// - Convert to lowercase
	// - Remove punctuation
	// - Remove apostrophes and what follows
	// - Split on whitespace
	tokens := simpleTokenize(tweet.Text)
	tweet.Tokens = tokens // Store tokens in the Tweet struct

	// TODO: Check the global mapping for tokens-3pk's and 3pk's-tokens.
	//    If the lookup fails, generate one and insert it into the mapping
	//    before inserting it into the tweet.
	// TODO: Verify that this is saving multiple copies if a word is repeated.
	// 
	var threePKs []tweets.ThreePartKey
	for _, token := range tokens {
		threePK := pipeline.GenerateThreePartKey(token)
		threePKs = append(threePKs, threePK)
	}
	tweet.ThreePKs = threePKs

	// Step 3: Update the global token counter for these tokens
	GlobalTokenCounter.IncrementTokens(tokens)

	// Step 4: Update global stats counters
	TotalTweetsRead++
	TotalTokensCounted += len(tokens)

	// Step 5: Manage the sliding window (remove old tweets and decrement their tokens)
	// Note: We'll call this after parsing, but we need to pass the window size
	// For now, we'll use a default of 15 minutes if not configured

	return tweet, nil
}

// simpleTokenize splits text into tokens for this project.
// - Converts to lowercase
// - Removes punctuation
// - Removes apostrophes and what follows
// - Splits on whitespace
func simpleTokenize(text string) []string {
	// Convert to lowercase
	text = strings.ToLower(text)

	// Remove apostrophes and what follows (e.g., "don't" -> "don")
	apostropheRe := regexp.MustCompile(`'\w*`)
	text = apostropheRe.ReplaceAllString(text, "")

	// Remove all punctuation (except spaces)
	punctRe := regexp.MustCompile(`[^a-z0-9 ]`)
	text = punctRe.ReplaceAllString(text, "")

	// Split on whitespace
	tokens := strings.Fields(text)
	return tokens
}

// Normalize all whitespace to a single space
func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// manageSlidingWindow adds a new tweet to the queue and removes old tweets that fall outside the window
func manageSlidingWindow(tweet *tweets.Tweet, windowSizeMinutes int) {
	tweetQueueMu.Lock()
	defer tweetQueueMu.Unlock()

	// Add the new tweet to the queue
	tweetQueue = append(tweetQueue, tweet)

	// Calculate the cutoff time for the sliding window
	cutoffTime := tweet.Unix - int64(windowSizeMinutes*60)

	// Remove old tweets that fall outside the window
	removedCount := 0
	for len(tweetQueue) > 0 && tweetQueue[0].Unix < cutoffTime {
		oldTweet := tweetQueue[0]
		tweetQueue = tweetQueue[1:] // Remove from front of queue

		// Decrement tokens from the old tweet
		if len(oldTweet.Tokens) > 0 {
			GlobalTokenCounter.DecrementTokens(oldTweet.Tokens)
			removedCount++
		}
	}

	// Log window management stats periodically
	if removedCount > 0 {
		slog.Info("Sliding window management",
			"tweets_removed", removedCount,
			"queue_size", len(tweetQueue),
			"window_minutes", windowSizeMinutes)
	}
}

// setupBloomFilterParams returns the expected number of tokens and number of hashes for each frequency class.
// This allows for different Bloom filter sizes based on the expected number of tokens in each class.
func setupBloomFilterParams(numClasses int) ([]int, []uint) {
	// Expected number of tokens in each frequency class (from most frequent to least frequent)
	// Based on actual data showing exponential growth: 15, 90, 576, 6076, 60373 for 5 classes
	expectedTokens := make([]int, numClasses)
	hashCounts := make([]uint, numClasses)

	// Use exponential growth based on actual data pattern
	// For 5 classes: 15, 90, 576, 6076, 60373
	// Growth factor is approximately 6x per class
	baseTokens := 15
	growthFactor := 6.0

	for i := 0; i < numClasses; i++ {
		expectedTokens[i] = int(float64(baseTokens) * math.Pow(growthFactor, float64(i)))

		// Number of hash functions - higher counts for larger filters to maintain low false positive rate
		if expectedTokens[i] < 100 {
			hashCounts[i] = 7
		} else if expectedTokens[i] < 1000 {
			hashCounts[i] = 8
		} else if expectedTokens[i] < 10000 {
			hashCounts[i] = 10
		} else if expectedTokens[i] < 100000 {
			hashCounts[i] = 12
		} else {
			hashCounts[i] = 14
		}
	}

	return expectedTokens, hashCounts
}
