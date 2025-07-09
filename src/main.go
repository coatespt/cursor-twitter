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

	"cursor-twitter/src/filter"
	"cursor-twitter/src/pipeline"
	"cursor-twitter/src/tweets"
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"
	"gopkg.in/yaml.v3"
)

// Config struct for YAML config file (add log_dir)
type Config struct {
	Mode       string `yaml:"mode"`
	InputDir   string `yaml:"input"`
	MQHost     string `yaml:"mq_host"`
	MQPort     int    `yaml:"mq_port"`
	MQQueue    string `yaml:"mq_queue"`
	WindowSize int    `yaml:"window"`
	BatchSize  int    `yaml:"batch"`

	Verbose     bool    `yaml:"verbose"`
	LogDir      string  `yaml:"log_dir"`
	FreqClasses int     `yaml:"freq_classes"`
	BWArrayLen  int     `yaml:"bw_array_len"`
	ZScore      float64 `yaml:"z_score"`

	Filter struct {
		Enabled    bool   `yaml:"enabled"`
		FilterFile string `yaml:"filter_file"`
	} `yaml:"filter"`
}

// GlobalTokenCounter keeps track of token counts in the current window.
var GlobalTokenCounter = pipeline.NewTokenCounter()

// Global stats counters
var (
	TotalTweetsRead    int
	TotalTokensCounted int
	lastStatsTime      time.Time
	lastTweetCount     int
	freqClasses        int // Number of frequency classes from config

)

// Global mappings for token <-> ThreePartKey relationships
var (
	tokenToThreePK  map[string]tweets.ThreePartKey
	threePKToToken  map[tweets.ThreePartKey]string
	tokenMappingsMu sync.RWMutex
)

// Sliding window management
var (
	tweetQueue   []*tweets.Tweet // Queue of tweets in the current window
	tweetQueueMu sync.RWMutex    // Mutex for thread-safe access to tweet queue
)

// Add a global variable to hold the stats CSV file path
var statsCSVPath string

// Global Bloom filters
var (
	GlobalFilters []pipeline.FreqClassFilter
)

// Global FCT and queues
var (
	inboundTokenQueue  *pipeline.TokenQueue
	oldTokenQueue      *pipeline.TokenQueue
	fct                *pipeline.FrequencyComputationThread
	freqClassProcessor *pipeline.FrequencyClassProcessor
)

// Global word filter
var globalWordFilter *filter.WordFilter

func main() {
	// Add a command line flag to control printing of tweets
	printTweets := flag.Bool("print-tweets", true, "Print each parsed tweet to the console")
	configPath := flag.String("config", "../config/config.yaml", "Path to YAML config file")
	flag.Parse()

	// Load config from YAML file.

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

	// Initialize global token mappings
	tokenToThreePK = make(map[string]tweets.ThreePartKey)
	threePKToToken = make(map[tweets.ThreePartKey]string)

	// Initialize word filter if enabled
	if cfg.Filter.Enabled {
		globalWordFilter = filter.NewWordFilter()
		if err := globalWordFilter.LoadFromFile(cfg.Filter.FilterFile); err != nil {
			slog.Error("Failed to load word filter", "error", err, "file", cfg.Filter.FilterFile)
			return
		}
		slog.Info("Word filter initialized", "filtered_words_count", globalWordFilter.GetFilteredCount(), "file", cfg.Filter.FilterFile)
	} else {
		slog.Info("Word filtering disabled")
	}

	// Set global array length for 3PK generation
	pipeline.SetGlobalArrayLen(cfg.BWArrayLen)

	// Create the token queues and FCT
	inboundTokenQueue = pipeline.NewTokenQueue()
	oldTokenQueue = pipeline.NewTokenQueue()

	// Create FCT with configuration
	freqClasses = cfg.FreqClasses
	if freqClasses <= 0 {
		slog.Error("Main: freq_classes must be > 0 in config, got", "value", freqClasses)
		os.Exit(1)
	}

	fct = pipeline.NewFrequencyComputationThread(
		GlobalTokenCounter,
		inboundTokenQueue,
		oldTokenQueue,
		0, // Not used - frequency rebuilds happen based on window size
		freqClasses,
	)

	// Start the FCT
	fct.Start()
	defer fct.Stop()

	slog.Info("FCT created and started",
		"freq_classes", freqClasses)

	// Create and start the frequency class processor
	freqClassProcessor = pipeline.NewFrequencyClassProcessor(freqClasses, cfg.BWArrayLen, float64(cfg.ZScore))
	freqClassProcessor.SetGlobalTokenMappingForAll(threePKToToken, &tokenMappingsMu)
	freqClassProcessor.Start()
	defer freqClassProcessor.Stop()

	slog.Info("FrequencyClassProcessor created and started",
		"num_classes", freqClasses,
		"bw_array_len", cfg.BWArrayLen,
		"z_score_threshold", cfg.ZScore)

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
	//       It also takes tokens from the OldTokenQueue and decrements the counts in the CountMap.
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

		tweet, err := parseCSVToTweet(string(msg.Body))
		if err != nil {
			//slog.Warn("Failed to parse tweet", "error", err, "raw_row", string(msg.Body))
			//fmt.Printf("[PARSE ERROR] %v\nRaw: %s\n", err, string(msg.Body))
			continue
		}
		// Only print the tweet if the flag is set
		if *printTweets {
			fmt.Printf("Parsed Tweet: %+v\n", tweet)
		}

		// Always add new tweet tokens to the inbound queue for FCT to build frequency filters
		if len(tweet.Tokens) > 0 {
			inboundTokenQueue.Enqueue(tweet.Tokens)
			slog.Info("Added tokens to InboundTokenQueue",
				"tweet_id", tweet.IDStr,
				"token_count", len(tweet.Tokens),
				"queue_size_after", inboundTokenQueue.Len())

			// Route each token to its appropriate frequency class (only if filters are available)
			filters := pipeline.GetGlobalFilters()
			if len(filters) > 0 {
				// Debug: Log when filters are available
				if TotalTweetsRead%10000 == 0 {
					slog.Info("Filters are available for token routing",
						"tweet_count", TotalTweetsRead,
						"num_filters", len(filters))
					fmt.Printf("*** TOKEN ROUTING ACTIVE: %d filters available ***\n", len(filters))
				}

				// Route tokens to frequency classes
				for _, token := range tweet.Tokens {
					// Find which frequency class this token belongs to
					freqClass := -1
					for i, filter := range filters {
						if filter.Contains(token) {
							freqClass = i
							break
						}
					}

					if freqClass >= 0 {
						// Generate or get the 3pk for this token
						tokenMappingsMu.RLock()
						threePK, exists := tokenToThreePK[token]
						tokenMappingsMu.RUnlock()

						if !exists {
							// Generate new ThreePartKey and store in mappings
							threePK = pipeline.GenerateThreePartKey(token)
							tokenMappingsMu.Lock()
							tokenToThreePK[token] = threePK
							threePKToToken[threePK] = token
							tokenMappingsMu.Unlock()
						}

						// Enqueue to appropriate frequency class
						freqClassProcessor.EnqueueToFrequencyClass(freqClass, threePK)
					} else {
						// Token not in any frequency class (shouldn't happen with proper filters)
						slog.Warn("Token not found in any frequency class filter", "token", token)
					}
				}
			} else {
				// No filters available yet - log occasionally
				if TotalTweetsRead%10000 == 0 {
					slog.Info("No frequency class filters available yet",
						"tweet_count", TotalTweetsRead)
					fmt.Printf("*** NO FILTERS AVAILABLE: waiting for first rebuild ***\n")
				}
			}
		}

		// Manage the sliding window - add new tweet and remove old ones
		// TODO: This is wrong. WindowSize is a constant set in configuration. A queue called
		// Inbound Tweet structs are stored on a Queue called TweetWindowQueue.
		// The new inbound tweet's tokens are placed on an InboundTokenQueue.
		// if the TweetWindowQueue reaches WindowSize, the oldest tweet is removed and
		// it's tokens are placed on an OldTokenQueue.

		if cfg.WindowSize <= 0 {
			slog.Error("Main: window must be > 0 in config, got", "value", cfg.WindowSize)
			os.Exit(1)
		}
		manageSlidingWindow(tweet, cfg.WindowSize)

		// Frequency class rebuilding - happens every time we cross the window boundary
		// When we've processed window-size tweets, we rebuild the frequency classes
		// NOTE: TotalTweetsRead is incremented in parseCSVToTweet, so we check after parsing

		// Check if we've crossed a window boundary
		currentWindow := TotalTweetsRead / cfg.WindowSize

		// Debug: Log every 1000 tweets to see what's happening
		if TotalTweetsRead%1000 == 0 {
			slog.Info("Main: Window boundary debug",
				"tweet_count", TotalTweetsRead,
				"window_size", cfg.WindowSize,
				"current_window", currentWindow,
				"modulo_check", TotalTweetsRead%cfg.WindowSize)
		}

		// Debug: Always log the window boundary check
		if TotalTweetsRead%10000 == 0 {
			slog.Info("Main: Window boundary check",
				"tweet_count", TotalTweetsRead,
				"window_size", cfg.WindowSize,
				"current_window", currentWindow,
				"modulo_result", TotalTweetsRead%cfg.WindowSize,
				"should_trigger", TotalTweetsRead%cfg.WindowSize == 0 && TotalTweetsRead > 0)
		}

		// Check if we should trigger a rebuild (window boundary crossed)
		if TotalTweetsRead%cfg.WindowSize == 0 && TotalTweetsRead > 0 {
			// We've crossed into a new window, give FCT permission to rebuild
			rebuildStartTime := time.Now()
			fmt.Printf("*** WINDOW BOUNDARY CROSSED: Tweet %d, Window %d at %s ***\n",
				TotalTweetsRead, currentWindow, rebuildStartTime.Format("15:04:05"))
			slog.Info("Main: Window boundary crossed, giving FCT permission to rebuild",
				"tweet_count", TotalTweetsRead,
				"window_size", cfg.WindowSize,
				"current_window", currentWindow,
				"rebuild_start_time", rebuildStartTime.Format("15:04:05"))

			if fct == nil {
				slog.Error("Main: FCT is nil! Cannot trigger rebuild")
			} else {
				slog.Info("Main: About to call fct.TriggerRebuild()")
				fct.TriggerRebuild()
				slog.Info("Main: fct.TriggerRebuild() completed")
			}
			fmt.Printf("*** REBUILD FLAG SET at %s ***\n", rebuildStartTime.Format("15:04:05"))
			slog.Info("Main: Rebuild flag set for FCT",
				"tweet_count", TotalTweetsRead,
				"window_size", cfg.WindowSize,
				"current_window", currentWindow,
				"rebuild_start_time", rebuildStartTime.Format("15:04:05"))
		}

		// Send termination signals to busy word processors every batch number of tweets
		// Only send if frequency class filters are available
		if TotalTweetsRead%cfg.BatchSize == 0 && TotalTweetsRead > 0 {
			filters := pipeline.GetGlobalFilters()
			if len(filters) > 0 {
				terminationSignal := tweets.ThreePartKey{Part1: -1, Part2: -1, Part3: -1}

				// Send termination signal to all frequency class processors
				for i := 0; i < freqClasses; i++ {
					freqClassProcessor.EnqueueToFrequencyClass(i, terminationSignal)
				}

				fmt.Printf("*** BATCH BOUNDARY: Sent termination signals to all %d busy word processors at tweet %d ***\n",
					freqClasses, TotalTweetsRead)
				slog.Info("Main: Sent termination signals to busy word processors",
					"tweet_count", TotalTweetsRead,
					"batch_size", cfg.BatchSize,
					"num_freq_classes", freqClasses)
			} else {
				// No filters available yet - log occasionally
				if TotalTweetsRead%10000 == 0 {
					slog.Info("Skipping batch termination - no frequency class filters available yet",
						"tweet_count", TotalTweetsRead,
						"batch_size", cfg.BatchSize)
				}
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
	lastStatsTime = time.Now()
	lastTweetCount = 0
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
	now := time.Now()
	timestamp := now.Format(time.RFC3339)
	totalTweets := TotalTweetsRead
	totalTokens := TotalTokensCounted
	distinctTokens := len(GlobalTokenCounter.Counts())

	// Calculate processing rate
	timeDiff := now.Sub(lastStatsTime).Seconds()
	tweetDiff := totalTweets - lastTweetCount
	processingRate := float64(tweetDiff) / timeDiff

	// Get sliding window stats
	tweetQueueMu.RLock()
	windowSize := len(tweetQueue)
	tweetQueueMu.RUnlock()

	// Get queue lengths
	inboundQueueSize := inboundTokenQueue.Len()
	oldQueueSize := oldTokenQueue.Len()

	// Get frequency class stats
	freqClassQueueStats := freqClassProcessor.GetQueueStats()
	freqClassProcessorStats := freqClassProcessor.GetProcessorStats()

	fmt.Printf("\n--- Pipeline Stats ---\n")
	fmt.Printf("Total tweets read: %d\n", totalTweets)
	fmt.Printf("Total tokens counted: %d\n", totalTokens)
	fmt.Printf("Distinct tokens: %d\n", distinctTokens)
	fmt.Printf("Tweets in current window: %d\n", windowSize)
	fmt.Printf("Inbound token queue size: %d\n", inboundQueueSize)
	fmt.Printf("Old token queue size: %d\n", oldQueueSize)
	fmt.Printf("Processing rate: %.2f tweets/sec\n", processingRate)

	// Print frequency class stats
	fmt.Printf("--- Frequency Class Stats ---\n")
	for i := 0; i < freqClasses; i++ {
		queueKey := fmt.Sprintf("freq_class_%d_queue_size", i)
		processorKey := fmt.Sprintf("freq_class_%d_tokens_processed", i)
		queueSize := freqClassQueueStats[queueKey]
		tokensProcessed := freqClassProcessorStats[processorKey]
		fmt.Printf("Class %d: Queue=%d, Processed=%d\n", i, queueSize, tokensProcessed)
	}
	fmt.Printf("----------------------\n")
	// Also log to slog
	slog.Info("Pipeline stats",
		"tweets", totalTweets,
		"tokens", totalTokens,
		"distinct", distinctTokens,
		"window_size", windowSize,
		"inbound_queue_size", inboundQueueSize,
		"old_queue_size", oldQueueSize,
		"processing_rate_tweets_per_sec", processingRate)

	// Update for next calculation
	lastStatsTime = now
	lastTweetCount = totalTweets

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
		Unix:         createdAt.Unix(),
		UserIDStr:    record[2],
		Text:         record[4],
		Retweeted:    record[5] == "True",
		RetweetCount: 0,   // TODO: parse record[3] as int
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

	// Generate ThreePartKeys and store in global mappings
	var threePKs []tweets.ThreePartKey
	for _, token := range tokens {
		// Check if we already have a mapping for this token
		tokenMappingsMu.RLock()
		threePK, exists := tokenToThreePK[token]
		tokenMappingsMu.RUnlock()

		if !exists {
			// Generate new ThreePartKey and store in mappings
			threePK = pipeline.GenerateThreePartKey(token)
			tokenMappingsMu.Lock()
			tokenToThreePK[token] = threePK
			threePKToToken[threePK] = token
			tokenMappingsMu.Unlock()
		}

		threePKs = append(threePKs, threePK)
	}
	// Note: ThreePKs not stored in Tweet struct but still generated for other uses

	// Step 3: Update global stats counters (token counting is now handled by FCT)
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
// - Filters out offensive words if word filtering is enabled
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

	// Filter out offensive words if word filtering is enabled
	if globalWordFilter != nil {
		var filteredTokens []string
		for _, token := range tokens {
			if !globalWordFilter.IsFiltered(token) {
				filteredTokens = append(filteredTokens, token)
			}
		}
		return filteredTokens
	}

	return tokens
}

// Normalize all whitespace to a single space
func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// manageSlidingWindow adds a new tweet to the queue and removes old tweets that fall outside the window
func manageSlidingWindow(tweet *tweets.Tweet, windowSize int) {
	tweetQueueMu.Lock()
	defer tweetQueueMu.Unlock()

	// Add the new tweet to the queue
	tweetQueue = append(tweetQueue, tweet)

	// Keep only the most recent windowSize tweets
	if len(tweetQueue) > windowSize {
		removedCount := len(tweetQueue) - windowSize
		oldTweets := tweetQueue[:removedCount]
		tweetQueue = tweetQueue[removedCount:]

		// Add old tweet tokens to the old token queue for FCT to process
		for _, oldTweet := range oldTweets {
			if len(oldTweet.Tokens) > 0 {
				oldTokenQueue.Enqueue(oldTweet.Tokens)
				slog.Info("Added old tokens to OldTokenQueue",
					"tweet_id", oldTweet.IDStr,
					"token_count", len(oldTweet.Tokens),
					"queue_size_after", oldTokenQueue.Len())
			}
		}

		// Log window management stats
		if removedCount > 0 {
			slog.Info("Sliding window management",
				"tweets_removed", removedCount,
				"queue_size", len(tweetQueue),
				"window_size", windowSize)
		}
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
