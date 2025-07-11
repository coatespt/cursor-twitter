package main

import (
	"encoding/csv"
	"encoding/gob"
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

	"os/signal"
	"syscall"

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

	Verbose              bool    `yaml:"verbose"`
	LogDir               string  `yaml:"log_dir"`
	FreqClasses          int     `yaml:"freq_classes"`
	BWArrayLen           int     `yaml:"bw_array_len"`
	ZScore               float64 `yaml:"z_score"`
	MinTokenLen          int     `yaml:"min_token_len"`
	SkipFrequencyClasses []int   `yaml:"skip_frequency_classes"`
	TokenPersistFiles    int     `yaml:"token_persist_files"`
	RebuildEveryFiles    int     `yaml:"rebuild_every_files"`

	Filter struct {
		Enabled    bool   `yaml:"enabled"`
		FilterFile string `yaml:"filter_file"`
	} `yaml:"filter"`

	TokenFilters struct {
		Enabled                         bool    `yaml:"enabled"`
		MaxLength                       int     `yaml:"max_length"`
		MinCharacterDiversity           float64 `yaml:"min_character_diversity"`
		MinCharacterDiversityLowerLimit int     `yaml:"min_character_diversity_lower_limit"`
		MaxCharacterRepetition          float64 `yaml:"max_character_repetition"`
		MaxCaseAlternations             float64 `yaml:"max_case_alternations"`
		MaxNumberLetterMix              float64 `yaml:"max_number_letter_mix"`
		RejectHashtags                  bool    `yaml:"reject_hashtags"`
		RejectUrls                      bool    `yaml:"reject_urls"`
		RejectAllCapsLong               bool    `yaml:"reject_all_caps_long"`
		AllCapsLowerLimit               int     `yaml:"all_caps_lower_limit"`
		RemoveUrls                      bool    `yaml:"remove_urls"`
	} `yaml:"token_filters"`

	Persistence struct {
		StateDir string `yaml:"state_dir"`
	} `yaml:"persistence"`

	Sender struct {
		StatusFile string `yaml:"status_file"`
	} `yaml:"sender"`
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

	// Token filter rejection statistics
	TokenFilterStats struct {
		TotalTokensProcessed int
		TotalTokensRejected  int
		RejectedByMaxLength  int
		RejectedByDiversity  int
		RejectedByRepetition int
		RejectedByCaseAlt    int
		RejectedByNumberMix  int
		RejectedByHashtag    int
		RejectedByUrl        int
		RejectedByAllCaps    int
		mu                   sync.RWMutex
	}
)

// Global mappings for token <-> ThreePartKey relationships
var (
	tokenToThreePK  map[string]tweets.ThreePartKey
	threePKToToken  map[tweets.ThreePartKey]string
	tokenMappingsMu sync.RWMutex
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
	fct                *pipeline.FrequencyComputationThread
	freqClassProcessor *pipeline.FrequencyClassProcessor
)

// Global word filter
var globalWordFilter *filter.WordFilter

// getCurrentWorkingDir returns the current working directory for debugging
func getCurrentWorkingDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	return dir
}

// Helper: Load and validate config
func loadAndValidateConfig(path string) (*Config, error) {
	cfg, err := loadConfig(path)
	if err != nil {
		return nil, err
	}
	if cfg.LogDir == "" {
		return nil, fmt.Errorf("ERROR: 'log_dir' must be defined in the config file and cannot be empty.")
	}
	return cfg, nil
}

// Helper: Initialize logger
func initializeLogger(cfg *Config) (*slog.Logger, *os.File, error) {
	logger, logFile, err := setupLogger(cfg.LogDir)
	if err != nil {
		return nil, nil, err
	}
	return logger, logFile, nil
}

// Helper: Initialize stats CSV
func initializeStatsCSV(cfg *Config) string {
	statsCSVPath := filepath.Join(cfg.LogDir, "stats.csv")
	ensureStatsCSVHeader(statsCSVPath)
	return statsCSVPath
}

// Helper: Initialize word filter
func initializeWordFilter(cfg *Config) (*filter.WordFilter, error) {
	if cfg.Filter.Enabled {
		globalWordFilter := filter.NewWordFilter()
		if err := globalWordFilter.LoadFromFile(cfg.Filter.FilterFile); err != nil {
			return nil, err
		}
		return globalWordFilter, nil
	}
	return nil, nil
}

// Helper: Setup RabbitMQ
func setupRabbitMQ(cfg *Config) (*amqp.Connection, *amqp.Channel, amqp.Queue, error) {
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		return nil, nil, amqp.Queue{}, err
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, nil, amqp.Queue{}, err
	}
	q, err := ch.QueueDeclare(
		"tweet_in", // name
		true,       // durable
		false,      // delete when unused
		false,      // exclusive
		false,      // no-wait
		nil,        // arguments
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, nil, amqp.Queue{}, err
	}
	return conn, ch, q, nil
}

// Helper: Initialize global variables and mappings
func initializeGlobalState() {
	tokenToThreePK = make(map[string]tweets.ThreePartKey)
	threePKToToken = make(map[tweets.ThreePartKey]string)
}

// Helper: Initialize pipeline components
func initializePipeline(cfg *Config) error {
	pipeline.SetGlobalArrayLen(cfg.BWArrayLen)

	inboundTokenQueue = pipeline.NewTokenQueue()

	freqClasses = cfg.FreqClasses
	if freqClasses <= 0 {
		return fmt.Errorf("freq_classes must be > 0 in config, got %d", freqClasses)
	}

	fct = pipeline.NewFrequencyComputationThread(
		GlobalTokenCounter,
		inboundTokenQueue,
		freqClasses,
		cfg.WindowSize,
		cfg.TokenPersistFiles,
		cfg.RebuildEveryFiles,
		cfg.Persistence.StateDir,
	)
	fct.Start()

	freqClassProcessor = pipeline.NewFrequencyClassProcessor(freqClasses, cfg.BWArrayLen, float64(cfg.ZScore), cfg.SkipFrequencyClasses)
	freqClassProcessor.SetGlobalTokenMappingForAll(threePKToToken, &tokenMappingsMu)
	freqClassProcessor.Start()

	return nil
}

// Helper: Setup signal handling
func setupSignalHandling() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		slog.Info("Received shutdown signal", "signal", sig.String())
		if pipeline.IsPersistenceInProgress() {
			for pipeline.IsPersistenceInProgress() {
				time.Sleep(100 * time.Millisecond)
			}
		}
		os.Exit(0)
	}()
}

// Helper: Setup RabbitMQ consumer
func setupRabbitMQConsumer(ch *amqp.Channel, q amqp.Queue) (<-chan amqp.Delivery, error) {
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
		return nil, fmt.Errorf("failed to register a consumer: %w", err)
	}
	return msgs, nil
}

func main() {
	// Add a command line flag to control printing of tweets
	printTweets := flag.Bool("print-tweets", true, "Print each parsed tweet to the console")
	configPath := flag.String("config", "config/config.yaml", "Path to YAML config file")
	loadState := flag.Bool("load-state", false, "Load persisted state from files on startup")
	flag.Parse()

	// Load config from YAML file.

	cfg, err := loadAndValidateConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	fmt.Printf("*** CONFIG LOADED SUCCESSFULLY ***\n")

	logger, logFile, err := initializeLogger(cfg)
	if err != nil {
		log.Fatalf("Failed to set up logger: %v", err)
	}
	defer logFile.Close()
	slog.SetDefault(logger)

	// Log startup information
	slog.Info("Application started",
		"config_path", *configPath,
		"print_tweets", *printTweets,
		"load_state", *loadState)

	statsCSVPath = initializeStatsCSV(cfg)

	globalWordFilter, err = initializeWordFilter(cfg)
	if err != nil {
		log.Fatalf("Failed to load word filter: %v", err)
	}

	initializeGlobalState()

	err = initializePipeline(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize pipeline: %v", err)
	}
	defer fct.Stop()
	defer freqClassProcessor.Stop()

	setupSignalHandling()

	conn, ch, q, err := setupRabbitMQ(cfg)
	if err != nil {
		slog.Error("Failed to set up RabbitMQ", "error", err)
		os.Exit(1)
	}
	defer conn.Close()
	defer ch.Close()

	msgs, err := setupRabbitMQConsumer(ch, q)
	if err != nil {
		slog.Error("Failed to set up RabbitMQ consumer", "error", err)
		os.Exit(1)
	}

	startStatsPrinter()

	for msg := range msgs {

		tweet, err := parseCSVToTweet(string(msg.Body), cfg)
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

			// Route each token to its appropriate frequency class (only if filters are available)
			filters := pipeline.GetGlobalFilters()
			if len(filters) > 0 {
				// Debug: Log when filters are available
				if TotalTweetsRead%10000 == 0 {
					slog.Info("Filters are available for token routing",
						"tweet_count", TotalTweetsRead,
						"num_filters", len(filters))
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
						// Check if this frequency class is active (not skipped)
						if !freqClassProcessor.IsClassActive(freqClass) {
							continue
						}
					} else {
						// Token not found in any frequency class filter - assign to highest numbered class
						freqClass = len(filters) - 1
						// Check if this frequency class is active (not skipped)
						if !freqClassProcessor.IsClassActive(freqClass) {
							continue
						}
					}

					// Generate or get the 3pk for this token
					tokenMappingsMu.RLock()
					threePK, exists := tokenToThreePK[token]
					tokenMappingsMu.RUnlock()

					if !exists {
						threePK = pipeline.GenerateThreePartKey(token)
						tokenMappingsMu.Lock()
						tokenToThreePK[token] = threePK
						threePKToToken[threePK] = token
						tokenMappingsMu.Unlock()
					}

					// Enqueue to appropriate frequency class
					freqClassProcessor.EnqueueToFrequencyClass(freqClass, threePK)
				}
			} else {
				// No filters available yet - log occasionally
				if TotalTweetsRead%10000 == 0 {
					slog.Info("No frequency class filters available yet",
						"tweet_count", TotalTweetsRead)
				}
			}
		}

		// Send termination signals to busy word processors every batch number of tweets
		// Only send if frequency class filters are available
		if TotalTweetsRead%cfg.BatchSize == 0 && TotalTweetsRead > 0 {
			filters := pipeline.GetGlobalFilters()
			if len(filters) > 0 {
				terminationSignal := tweets.ThreePartKey{Part1: -1, Part2: -1, Part3: -1}

				// Send termination signal to active frequency class processors only
				activeCount := 0
				for i := 0; i < freqClasses; i++ {
					if freqClassProcessor.IsClassActive(i) {
						freqClassProcessor.EnqueueToFrequencyClass(i, terminationSignal)
						activeCount++
					}
				}

				slog.Info("Main: Sent termination signals to busy word processors",
					"tweet_count", TotalTweetsRead,
					"batch_size", cfg.BatchSize,
					"total_freq_classes", freqClasses,
					"active_freq_classes", activeCount)
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
	logger := slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
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
	// tweetQueueMu.RLock()
	// windowSize := len(tweetQueue)
	// tweetQueueMu.RUnlock()

	// Get queue lengths
	inboundQueueSize := inboundTokenQueue.Len()

	// Get frequency class stats
	freqClassQueueStats := freqClassProcessor.GetQueueStats()
	freqClassProcessorStats := freqClassProcessor.GetProcessorStats()

	// Get token filter statistics
	TokenFilterStats.mu.RLock()
	totalProcessed := TokenFilterStats.TotalTokensProcessed
	totalRejected := TokenFilterStats.TotalTokensRejected
	rejectionRate := 0.0
	if totalProcessed > 0 {
		rejectionRate = float64(totalRejected) / float64(totalProcessed) * 100.0
	}

	// Get breakdown by filter type
	rejectedByMaxLength := TokenFilterStats.RejectedByMaxLength
	rejectedByDiversity := TokenFilterStats.RejectedByDiversity
	rejectedByRepetition := TokenFilterStats.RejectedByRepetition
	rejectedByCaseAlt := TokenFilterStats.RejectedByCaseAlt
	rejectedByNumberMix := TokenFilterStats.RejectedByNumberMix
	rejectedByHashtag := TokenFilterStats.RejectedByHashtag
	rejectedByUrl := TokenFilterStats.RejectedByUrl
	rejectedByAllCaps := TokenFilterStats.RejectedByAllCaps
	TokenFilterStats.mu.RUnlock()

	fmt.Printf("\n--- Pipeline Stats ---\n")
	fmt.Printf("Total tweets read: %d\n", totalTweets)
	fmt.Printf("Total tokens counted: %d\n", totalTokens)
	fmt.Printf("Distinct tokens: %d\n", distinctTokens)
	// fmt.Printf("Tweets in current window: %d\n", windowSize) // Removed tweet-based window size
	fmt.Printf("Inbound token queue size: %d\n", inboundQueueSize)
	fmt.Printf("Processing rate: %.2f tweets/sec\n", processingRate)
	fmt.Printf("--- Token Filter Stats ---\n")
	fmt.Printf("Tokens processed: %d\n", totalProcessed)
	fmt.Printf("Tokens rejected: %d\n", totalRejected)
	fmt.Printf("Rejection rate: %.2f%%\n", rejectionRate)
	if totalRejected > 0 {
		fmt.Printf("  Rejected by max length: %d (%.1f%%)\n", rejectedByMaxLength, float64(rejectedByMaxLength)/float64(totalRejected)*100)
		fmt.Printf("  Rejected by diversity: %d (%.1f%%)\n", rejectedByDiversity, float64(rejectedByDiversity)/float64(totalRejected)*100)
		fmt.Printf("  Rejected by repetition: %d (%.1f%%)\n", rejectedByRepetition, float64(rejectedByRepetition)/float64(totalRejected)*100)
		fmt.Printf("  Rejected by case alternation: %d (%.1f%%)\n", rejectedByCaseAlt, float64(rejectedByCaseAlt)/float64(totalRejected)*100)
		fmt.Printf("  Rejected by number mix: %d (%.1f%%)\n", rejectedByNumberMix, float64(rejectedByNumberMix)/float64(totalRejected)*100)
		fmt.Printf("  Rejected by hashtag: %d (%.1f%%)\n", rejectedByHashtag, float64(rejectedByHashtag)/float64(totalRejected)*100)
		fmt.Printf("  Rejected by URL: %d (%.1f%%)\n", rejectedByUrl, float64(rejectedByUrl)/float64(totalRejected)*100)
		fmt.Printf("  Rejected by all caps: %d (%.1f%%)\n", rejectedByAllCaps, float64(rejectedByAllCaps)/float64(totalRejected)*100)
	}

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
		// "window_size", windowSize, // Removed tweet-based window size
		"inbound_queue_size", inboundQueueSize,
		"processing_rate_tweets_per_sec", processingRate,
		"tokens_processed", totalProcessed,
		"tokens_rejected", totalRejected,
		"rejection_rate_pct", fmt.Sprintf("%.2f", rejectionRate))

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
func parseCSVToTweet(row string, cfg *Config) (*tweets.Tweet, error) {
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

	// Step 1: Tokenize the tweet text.
	// - Convert to lowercase
	// - Remove punctuation
	// - Remove apostrophes and what follows
	// - Split on whitespace
	tokens := simpleTokenize(tweet.Text, cfg)
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
// - Filters out tokens shorter than min_token_len if specified
func simpleTokenize(text string, cfg *Config) []string {
	// Use strings.Fields() to get a slice of substrings
	tokens := strings.Fields(text)

	// Process each token individually
	var processedTokens []string
	for _, token := range tokens {
		// Remove URLs from this token if enabled
		if cfg.TokenFilters.RemoveUrls {
			urlRe := regexp.MustCompile(`(https?://[^\s]+|www\.[^\s]+)`)
			token = urlRe.ReplaceAllString(token, "")
			if token == "" {
				continue
			}
		}

		// Remove apostrophes and what follows (e.g., "don't" -> "don", "Harry's" -> "Harry")
		apostropheRe := regexp.MustCompile(`'.*`)
		token = apostropheRe.ReplaceAllString(token, "")
		if token == "" {
			continue
		}

		// Remove punctuation from the token
		cleanToken := removePunctuation(token)

		// Skip empty tokens after punctuation removal
		if cleanToken == "" {
			continue
		}

		// Skip tokens that are too short
		if cfg.MinTokenLen > 0 && len(cleanToken) < cfg.MinTokenLen {
			continue
		}

		// Filter out offensive words if word filtering is enabled
		if globalWordFilter != nil && globalWordFilter.IsFiltered(cleanToken) {
			continue
		}

		// Apply token filters if enabled
		if shouldFilterToken(cleanToken, cfg) {
			continue
		}

		// Convert to lowercase for final output
		cleanToken = strings.ToLower(cleanToken)
		processedTokens = append(processedTokens, cleanToken)
	}

	return processedTokens
}

// removePunctuation removes punctuation from a token while preserving alphanumeric characters
func removePunctuation(token string) string {
	var result strings.Builder
	for _, char := range token {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			result.WriteRune(char)
		}
	}
	return result.String()
}

// Normalize all whitespace to a single space
func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// shouldFilterToken applies all configured token filters and tracks rejection statistics
func shouldFilterToken(token string, cfg *Config) bool {
	if !cfg.TokenFilters.Enabled {
		return false
	}

	// Track that we're processing a token
	TokenFilterStats.mu.Lock()
	TokenFilterStats.TotalTokensProcessed++
	TokenFilterStats.mu.Unlock()

	// Max length filter
	if cfg.TokenFilters.MaxLength > 0 && len(token) > cfg.TokenFilters.MaxLength {
		TokenFilterStats.mu.Lock()
		TokenFilterStats.TotalTokensRejected++
		TokenFilterStats.RejectedByMaxLength++
		TokenFilterStats.mu.Unlock()
		return true
	}

	// Character diversity filter (only for long tokens)
	if len(token) >= cfg.TokenFilters.MinCharacterDiversityLowerLimit && cfg.TokenFilters.MinCharacterDiversity > 0 {
		uniqueChars := make(map[rune]bool)
		for _, char := range token {
			uniqueChars[char] = true
		}
		diversity := float64(len(uniqueChars)) / float64(len(token))
		if diversity < cfg.TokenFilters.MinCharacterDiversity {
			TokenFilterStats.mu.Lock()
			TokenFilterStats.TotalTokensRejected++
			TokenFilterStats.RejectedByDiversity++
			TokenFilterStats.mu.Unlock()
			return true
		}
	}

	// Character repetition filter
	if cfg.TokenFilters.MaxCharacterRepetition > 0 {
		repetitionCount := 0
		for i := 1; i < len(token); i++ {
			if token[i] == token[i-1] {
				repetitionCount++
			}
		}
		repetitionRatio := float64(repetitionCount) / float64(len(token))
		if repetitionRatio > cfg.TokenFilters.MaxCharacterRepetition {
			TokenFilterStats.mu.Lock()
			TokenFilterStats.TotalTokensRejected++
			TokenFilterStats.RejectedByRepetition++
			TokenFilterStats.mu.Unlock()
			return true
		}
	}

	// Case alternation filter
	if cfg.TokenFilters.MaxCaseAlternations > 0 {
		caseChanges := 0
		for i := 1; i < len(token); i++ {
			if (token[i] >= 'A' && token[i] <= 'Z' && token[i-1] >= 'a' && token[i-1] <= 'z') ||
				(token[i] >= 'a' && token[i] <= 'z' && token[i-1] >= 'A' && token[i-1] <= 'Z') {
				caseChanges++
			}
		}
		caseChangeRatio := float64(caseChanges) / float64(len(token))
		if caseChangeRatio > cfg.TokenFilters.MaxCaseAlternations {
			TokenFilterStats.mu.Lock()
			TokenFilterStats.TotalTokensRejected++
			TokenFilterStats.RejectedByCaseAlt++
			TokenFilterStats.mu.Unlock()
			return true
		}
	}

	// Number-letter mixing filter
	if cfg.TokenFilters.MaxNumberLetterMix > 0 {
		digitCount := 0
		for _, char := range token {
			if char >= '0' && char <= '9' {
				digitCount++
			}
		}
		digitRatio := float64(digitCount) / float64(len(token))
		if digitRatio > cfg.TokenFilters.MaxNumberLetterMix {
			TokenFilterStats.mu.Lock()
			TokenFilterStats.TotalTokensRejected++
			TokenFilterStats.RejectedByNumberMix++
			TokenFilterStats.mu.Unlock()
			return true
		}
	}

	// Hashtag filter
	if cfg.TokenFilters.RejectHashtags && strings.HasPrefix(token, "#") {
		TokenFilterStats.mu.Lock()
		TokenFilterStats.TotalTokensRejected++
		TokenFilterStats.RejectedByHashtag++
		TokenFilterStats.mu.Unlock()
		return true
	}

	// URL filter
	if cfg.TokenFilters.RejectUrls && (strings.HasPrefix(token, "http") || strings.HasPrefix(token, "www")) {
		TokenFilterStats.mu.Lock()
		TokenFilterStats.TotalTokensRejected++
		TokenFilterStats.RejectedByUrl++
		TokenFilterStats.mu.Unlock()
		return true
	}

	// All caps long filter
	if cfg.TokenFilters.RejectAllCapsLong && len(token) >= cfg.TokenFilters.AllCapsLowerLimit {
		allCaps := true
		for _, char := range token {
			if char < 'A' || char > 'Z' {
				allCaps = false
				break
			}
		}
		if allCaps {
			TokenFilterStats.mu.Lock()
			TokenFilterStats.TotalTokensRejected++
			TokenFilterStats.RejectedByAllCaps++
			TokenFilterStats.mu.Unlock()
			return true
		}
	}

	return false
}

// manageSlidingWindow adds a new tweet to the queue and removes old tweets that fall outside the window
func manageSlidingWindow(tweet *tweets.Tweet, windowSize int) {
	// This function is no longer needed as the tweet queue is removed.
	// The FCT handles the sliding window for tokens.
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

// loadPersistedState loads the persisted data structures from files and logs statistics
func loadPersistedState(stateDir string) {
	fmt.Println("=== LOADING PERSISTED STATE ===")

	// Check if any of the files exist
	tokenCounterPath := filepath.Join(stateDir, "token_counter.json")
	freqClassPath := filepath.Join(stateDir, "frequency_classes.json")
	threePKPath := filepath.Join(stateDir, "threepartkey_mappings.json")

	// If none of the files exist, just return and let the normal program run
	_, err1 := os.Stat(tokenCounterPath)
	_, err2 := os.Stat(freqClassPath)
	_, err3 := os.Stat(threePKPath)
	if os.IsNotExist(err1) && os.IsNotExist(err2) && os.IsNotExist(err3) {
		fmt.Println("No persisted state files found. Starting fresh.")
		fmt.Println("=== PERSISTED STATE LOADING COMPLETE ===")
		return
	}

	// Load TokenCounter if it exists
	tempTokenCounter := pipeline.NewTokenCounter()
	if err := tempTokenCounter.LoadFromFile(tokenCounterPath); err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			fmt.Printf("TokenCounter file not found: %s\n", tokenCounterPath)
		} else {
			fmt.Printf("Failed to load TokenCounter: %v\n", err)
		}
	} else {
		counts := tempTokenCounter.Counts()
		totalTokens := 0
		for _, count := range counts {
			totalTokens += count
		}
		fmt.Printf("TokenCounter loaded: %d total tokens (%d distinct tokens)\n", totalTokens, len(counts))
	}

	// Load FrequencyClassResult if it exists
	var tempFreqClassResult pipeline.FreqClassResult
	if err := tempFreqClassResult.LoadFromFile(freqClassPath); err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			fmt.Printf("FrequencyClassResult file not found: %s\n", freqClassPath)
		} else {
			fmt.Printf("Failed to load FrequencyClassResult: %v\n", err)
		}
	} else {
		classes := len(tempFreqClassResult.Filters)
		fmt.Printf("FrequencyClassResult loaded: %d classes\n", classes)
	}

	// Load ThreePartKey mappings if they exist
	// TODO: Implement ThreePartKey mapping loading when needed
	fmt.Printf("ThreePartKey mappings loading not yet implemented\n")

	fmt.Println("=== PERSISTED STATE LOADING COMPLETE ===")
}

// loadThreePartKeyMappingsFromFile loads ThreePartKey mappings from a file
func loadThreePartKeyMappingsFromFile(filename string, mappings map[string]tweets.ThreePartKey) error {
	// Open the file
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %v", filename, err)
	}
	defer file.Close()

	// Decode into a temporary map first
	var tempMappings map[string]tweets.ThreePartKey
	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&tempMappings); err != nil {
		return fmt.Errorf("failed to decode mappings from %s: %v", filename, err)
	}

	// Copy the data to the target map
	for k, v := range tempMappings {
		mappings[k] = v
	}

	return nil
}
