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
	"sort"
	"strings"
	"sync"
	"time"

	"cursor-twitter/src/filter"
	"cursor-twitter/src/pipeline"
	"cursor-twitter/src/tweets"
	"log/slog"

	"os/signal"
	"syscall"

	"runtime/pprof"

	amqp "github.com/rabbitmq/amqp091-go"
	"gopkg.in/yaml.v3"
)

// Config struct for YAML config file (add log_dir)
type Config struct {
	Mode          string `yaml:"mode"`
	InputDir      string `yaml:"input"`
	MQHost        string `yaml:"mq_host"`
	MQPort        int    `yaml:"mq_port"`
	MQQueue       string `yaml:"mq_queue"`
	WindowSize    int    `yaml:"window"`
	BatchSize     int    `yaml:"batch"`
	WindowBatches int    `yaml:"window_batches"` // Number of batches to keep in tweet window

	Verbose              bool      `yaml:"verbose"`
	LogDir               string    `yaml:"log_dir"`
	FreqClasses          int       `yaml:"freq_classes"`
	BWArrayLen           int       `yaml:"bw_array_len"`
	ZScores              []float64 `yaml:"z_scores"`
	MinTokenLen          int       `yaml:"min_token_len"`
	SkipFrequencyClasses []int     `yaml:"skip_frequency_classes"`
	TokenPersistFiles    int       `yaml:"token_persist_files"`
	RebuildEveryFiles    int       `yaml:"rebuild_every_files"`
	MinCountThreshold    int       `yaml:"min_count_threshold"` // Minimum count for frequency class inclusion
	BusywordClasses      []int     `yaml:"busyword_classes"`    // Frequency classes to use for clustering

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

	Analysis struct {
		ClusteringWindowBatches      int     `yaml:"clustering_window_batches"`      // Number of batches of recent tweets to use for clustering
		MinBusyWordsPerTweet         int     `yaml:"min_busy_words_per_tweet"`       // Minimum number of busy words a tweet must contain to be included in clustering
		MinJaccardSimilarity         float64 `yaml:"min_jaccard_similarity"`         // Minimum Jaccard similarity threshold for creating edges between tweets
		MaxTweetsToCluster           int     `yaml:"max_tweets_to_cluster"`          // Maximum number of tweets to cluster (0 = no limit)
		SuppressDuplicates           bool    `yaml:"suppress_duplicates"`            // Suppress duplicate tweets in visualization
		DuplicateSimilarityThreshold float64 `yaml:"duplicate_similarity_threshold"` // Similarity threshold for duplicates
		LanguageFilter               string  `yaml:"language_filter"`                // Language filter: "en", "es", "all", etc.
	} `yaml:"analysis"`
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

// Global mappings for token <-> ThreePartKey relationships - COMMENTED OUT for on-the-fly generation test
// var (
// 	tokenToThreePK  map[string]tweets.ThreePartKey
// 	threePKToToken  map[tweets.ThreePartKey]string
// 	tokenMappingsMu sync.RWMutex
// )

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

// Global recent tweet window
var recentTweetWindow *RecentTweetWindow

// Pre-compiled regexes for tokenization (compiled once at startup)
var (
	urlRegex        *regexp.Regexp
	apostropheRegex *regexp.Regexp
)

// Add at the top-level globals:
var clusterOutputFilePath string
var clusterOutputFileOnce sync.Once

// Analysis thread for processing busy word results
func startAnalysisThread(resultChannel <-chan pipeline.BusyWordResult, cfg *Config) {
	go func() {
		resultCount := 0

		// Track results by batch
		currentBatch := make(map[int][]string) // class -> busy words
		currentBatchNumber := -1

		for result := range resultChannel {
			resultCount++

			// Check if this is a new batch
			if currentBatchNumber != result.BatchNumber {
				// Print summary of previous batch if it exists
				if currentBatchNumber >= 0 {
					printBatchSummary(currentBatch, currentBatchNumber, cfg)
				}

				// Start new batch
				currentBatch = make(map[int][]string)
				currentBatchNumber = result.BatchNumber
			}

			// Convert 3PKs to actual words
			busyWords := make([]string, 0, len(result.BusyWord3PKs))
			notFoundCount := 0
			for _, threePK := range result.BusyWord3PKs {
				// Convert 3PK to word using the global token mapping
				if word, exists := pipeline.GetWordFrom3PK(threePK); exists {
					busyWords = append(busyWords, word)
				} else {
					// Skip 3PKs that aren't in the mapping - they shouldn't exist
					notFoundCount++
				}
			}

			// Debug: Show if any 3PKs weren't found (this indicates a system problem)
			if notFoundCount > 0 {
				fmt.Printf("[ANALYSIS] ERROR: %d/%d 3PKs not found in mapping for class %d - this should not happen!\n",
					notFoundCount, len(result.BusyWord3PKs), result.FrequencyClass)
			}

			// Store results for this class
			currentBatch[result.FrequencyClass] = busyWords
		}

		// Print final batch summary
		if currentBatchNumber >= 0 {
			printBatchSummary(currentBatch, currentBatchNumber, cfg)
		}

		fmt.Printf("[ANALYSIS] Analysis thread stopped after processing %d results\n", resultCount)
	}()
}

// printBatchSummary prints a summary of all busy words found in a batch
func printBatchSummary(classResults map[int][]string, batchNumber int, cfg *Config) {
	// Create a buffer to capture all output
	var output strings.Builder

	// Helper function to write to both buffer and stdout
	writeOutput := func(format string, args ...interface{}) {
		line := fmt.Sprintf(format, args...)
		output.WriteString(line + "\n")
		fmt.Print(line + "\n")
	}
	totalBusyWords := 0
	classesWithWords := 0

	writeOutput("\n" + strings.Repeat("=", 80))
	writeOutput("BATCH %d ANALYSIS SUMMARY", batchNumber)
	writeOutput(strings.Repeat("=", 80))

	// Get sorted class indices to ensure consistent ordering
	classIndices := make([]int, 0, len(classResults))
	for classIndex := range classResults {
		classIndices = append(classIndices, classIndex)
	}
	sort.Ints(classIndices)

	// Print classes in sorted order
	for _, classIndex := range classIndices {
		words := classResults[classIndex]
		totalBusyWords += len(words)
		if len(words) > 0 {
			classesWithWords++
			writeOutput("Class %d: %d busy words - %s", classIndex, len(words), strings.Join(words, ", "))
		} else {
			writeOutput("Class %d: %d busy words", classIndex, len(words))
		}
	}

	writeOutput("\nTOTAL: %d busy words across %d classes", totalBusyWords, classesWithWords)
	writeOutput("Would search %d tweets for these busy words", recentTweetWindow.Len())

	// Get the recent tweets for clustering analysis
	// Use configured number of batches worth of tweets
	k := cfg.Analysis.ClusteringWindowBatches
	if k <= 0 {
		k = 1 // Default to 1 batch if not configured
	}
	recentTweets := recentTweetWindow.GetRecentTweets(k * cfg.BatchSize)
	writeOutput("*** CLUSTERING: Retrieved %d tweets from recent window (k=%d, batch=%d, total=%d) ***", len(recentTweets), k, cfg.BatchSize, k*cfg.BatchSize)

	// Filter tweets to only include those with busy words
	minBusyWords := cfg.Analysis.MinBusyWordsPerTweet
	if minBusyWords <= 0 {
		minBusyWords = 1 // Default to 1 if not configured
	}

	// Collect busy words only from the specified frequency classes
	allBusyWords := make(map[string]bool)
	allowedClasses := make(map[int]bool)

	// Validate busyword_classes are within valid range (1 to freq_classes)
	for _, class := range cfg.BusywordClasses {
		if class < 1 || class > cfg.FreqClasses {
			writeOutput("*** WARNING: Invalid busyword_class %d (valid range: 1-%d) - skipping ***", class, cfg.FreqClasses)
			continue
		}
		allowedClasses[class] = true
	}

	if len(allowedClasses) == 0 {
		writeOutput("*** ERROR: No valid busyword_classes found - all classes were out of range (1-%d) ***", cfg.FreqClasses)
		writeOutput(strings.Repeat("=", 80) + "\n")
		return
	}

	// Only include busy words from allowed classes
	for classIndex, words := range classResults {
		if allowedClasses[classIndex] {
			for _, word := range words {
				allBusyWords[word] = true
			}
		}
	}

	writeOutput("*** CLUSTERING: Using busy words from classes: %v ***", cfg.BusywordClasses)

	// Filter tweets that contain at least minBusyWords busy words
	var tweetsWithBusyWords []*tweets.Tweet
	busyWordDistribution := make(map[int]int) // count -> number of tweets

	for _, tweet := range recentTweets {
		busyWordCount := 0
		for _, token := range tweet.Tokens {
			if allBusyWords[token] {
				busyWordCount++
			}
		}
		busyWordDistribution[busyWordCount]++
		if busyWordCount >= minBusyWords {
			tweetsWithBusyWords = append(tweetsWithBusyWords, tweet)
		}
	}

	// Print breakdown of busy word distribution
	writeOutput("*** CLUSTERING: Busy word distribution:")
	for i := 0; i <= 10; i++ { // Show up to 10+ busy words
		if count, exists := busyWordDistribution[i]; exists && count > 0 {
			if i == 10 {
				writeOutput("  %d+ busy words: %d tweets", i, count)
			} else {
				writeOutput("  %d busy words: %d tweets", i, count)
			}
		}
	}

	writeOutput("*** CLUSTERING: Filtered to %d tweets with busy words (min=%d) ***", len(tweetsWithBusyWords), minBusyWords)

	// Sanity checks before proceeding with clustering
	if len(tweetsWithBusyWords) == 0 {
		writeOutput("*** CLUSTERING: No tweets with busy words found - skipping clustering ***")
		writeOutput(strings.Repeat("=", 80) + "\n")
		return
	}

	if len(tweetsWithBusyWords) < 2 {
		writeOutput("*** CLUSTERING: Only %d tweet with busy words - need at least 2 for clustering ***", len(tweetsWithBusyWords))
		writeOutput(strings.Repeat("=", 80) + "\n")
		return
	}

	if len(allBusyWords) == 0 {
		writeOutput("*** CLUSTERING: No busy words found - skipping clustering ***")
		writeOutput(strings.Repeat("=", 80) + "\n")
		return
	}

	writeOutput("*** CLUSTERING: Ready for clustering with %d tweets and %d busy words ***", len(tweetsWithBusyWords), len(allBusyWords))

	// Debug: Show some of the busy words being used
	writeOutput("*** CLUSTERING: Sample busy words: ")
	wordCount := 0
	for word := range allBusyWords {
		if wordCount < 10 { // Show first 10 busy words
			writeOutput("%s, ", word)
			wordCount++
		} else {
			writeOutput("... (and %d more)", len(allBusyWords)-10)
			break
		}
	}
	if wordCount <= 10 {
		writeOutput("")
	}

	// Perform optimized clustering
	clusterer := pipeline.NewOptimizedTweetClusterer(
		cfg.Analysis.MinJaccardSimilarity,
		cfg.Analysis.MaxTweetsToCluster,
	)

	result := clusterer.ClusterTweets(tweetsWithBusyWords, allBusyWords)

	// Print clustering results with ASCII visualization
	writeOutput("*** CLUSTERING RESULTS ***")
	writeOutput("Clusters found: %d", len(result.Clusters))
	writeOutput("Graph density: %.4f", result.Stats.GraphDensity)
	writeOutput("Total edges: %d", result.Stats.TotalEdges)
	writeOutput("Processing time: %.3f seconds", result.Stats.ProcessingTime)

	if len(result.Clusters) > 0 {
		writeOutput("\n📊 CLUSTER VISUALIZATION:")
		for i, cluster := range result.Clusters {
			// Show shared busy words if available
			busyWordsStr := ""
			if len(cluster.BusyWords) > 0 {
				busyWordsStr = fmt.Sprintf(" [%s]", strings.Join(cluster.BusyWords, ", "))
			} else {
				// Debug: Show why no busy words are displayed
				writeOutput("*** DEBUG: Cluster %d has no shared busy words across all %d tweets ***", i+1, cluster.Size)
			}
			writeOutput("┌─ Cluster %d (%d tweets)%s", i+1, cluster.Size, busyWordsStr)

			// Show first few tweets in each cluster
			maxTweetsToShow := 20
			if len(cluster.Tweets) < maxTweetsToShow {
				maxTweetsToShow = len(cluster.Tweets)
			}

			// Apply deduplication if enabled
			if cfg.Analysis.SuppressDuplicates {
				// Group tweets by normalized text
				groups := make(map[string][]*tweets.Tweet)

				for _, tweet := range cluster.Tweets {
					normalized := normalizeTweetForComparison(tweet.Text)
					groups[normalized] = append(groups[normalized], tweet)
				}

				// Convert to sorted list
				type tweetGroup struct {
					Tweet *tweets.Tweet
					Count int
				}
				var deduplicated []tweetGroup

				for _, group := range groups {
					deduplicated = append(deduplicated, tweetGroup{
						Tweet: group[0], // Use first tweet as representative
						Count: len(group),
					})
				}

				// Sort by count (descending) then by text
				sort.Slice(deduplicated, func(i, j int) bool {
					if deduplicated[i].Count != deduplicated[j].Count {
						return deduplicated[i].Count > deduplicated[j].Count
					}
					return deduplicated[i].Tweet.Text < deduplicated[j].Tweet.Text
				})

				for j, item := range deduplicated {
					if j >= maxTweetsToShow {
						break
					}

					prefix := "│  ├─"
					if j == len(deduplicated)-1 || j == maxTweetsToShow-1 {
						prefix = "│  └─"
					}

					// Show full tweet text without truncation
					text := item.Tweet.Text

					if item.Count > 1 {
						writeOutput("%s \"%s\" (%d instances)", prefix, text, item.Count)
					} else {
						writeOutput("%s \"%s\"", prefix, text)
					}
				}

				// Show if we have more deduplicated tweets than shown
				if len(deduplicated) > maxTweetsToShow {
					writeOutput("│  └─ ... and %d more unique tweets", len(deduplicated)-maxTweetsToShow)
				}
			} else {
				// Original behavior - show all tweets
				for j := 0; j < maxTweetsToShow; j++ {
					tweet := cluster.Tweets[j]
					prefix := "│  ├─"
					if j == maxTweetsToShow-1 {
						prefix = "│  └─"
					}

					// Show full tweet text without truncation
					text := tweet.Text

					writeOutput("%s \"%s\"", prefix, text)
				}

				// Show shared busy words if we have more tweets than shown
				if len(cluster.Tweets) > maxTweetsToShow {
					writeOutput("│  └─ ... and %d more tweets", len(cluster.Tweets)-maxTweetsToShow)
				}
			}

			writeOutput("│")
		}
		writeOutput("└─ End of clusters")
	}

	writeOutput(strings.Repeat("=", 80) + "\n")

	// Append the captured output to the global cluster output file
	clusterOutputFileOnce.Do(func() {
		// Ensure the file is created (truncated if exists)
		_ = os.WriteFile(clusterOutputFilePath, []byte{}, 0644)
	})
	f, err := os.OpenFile(clusterOutputFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("*** ERROR: Failed to write cluster output to %s: %v ***\n", clusterOutputFilePath, err)
	} else {
		_, _ = f.WriteString(output.String())
		f.Close()
	}
}

// normalizeTweetForComparison removes leading @mentions, RT prefixes, trailing URLs, and normalizes whitespace
func normalizeTweetForComparison(text string) string {
	// Remove leading "RT @username: " patterns
	rtRegex := regexp.MustCompile(`^RT\s+@\w+:\s*`)
	text = rtRegex.ReplaceAllString(text, "")

	// Remove leading @mentions at the start
	leadingMentionRegex := regexp.MustCompile(`^@\w+\s*`)
	text = leadingMentionRegex.ReplaceAllString(text, "")

	// Remove trailing URLs
	urlRegex := regexp.MustCompile(`\s+https?://\S+$`)
	text = urlRegex.ReplaceAllString(text, "")

	// Normalize whitespace (multiple spaces to single space, trim)
	text = strings.Join(strings.Fields(text), " ")

	return strings.TrimSpace(text)
}

// RecentTweetWindow is a thread-safe, fixed-size queue for recent tweets
// Holds up to maxSize tweets; oldest are removed as new ones arrive
// Provides thread-safe Add, GetAll, and Len methods

type RecentTweetWindow struct {
	mu      sync.RWMutex
	tweets  []*tweets.Tweet
	maxSize int
}

func NewRecentTweetWindow(maxSize int) *RecentTweetWindow {
	return &RecentTweetWindow{
		tweets:  make([]*tweets.Tweet, 0, maxSize),
		maxSize: maxSize,
	}
}

// Add adds a tweet to the window, removing the oldest if over capacity
func (w *RecentTweetWindow) Add(tweet *tweets.Tweet) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.tweets) >= w.maxSize {
		// Remove oldest (front)
		w.tweets = w.tweets[1:]
	}
	w.tweets = append(w.tweets, tweet)
}

// GetAll returns a copy of all tweets in the window
func (w *RecentTweetWindow) GetAll() []*tweets.Tweet {
	w.mu.RLock()
	defer w.mu.RUnlock()
	copyTweets := make([]*tweets.Tweet, len(w.tweets))
	copy(copyTweets, w.tweets)
	return copyTweets
}

// Len returns the number of tweets in the window
func (w *RecentTweetWindow) Len() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.tweets)
}

// GetRecentTweets returns the k most recent tweets in the window
// Returns all tweets if k is greater than the window size
func (w *RecentTweetWindow) GetRecentTweets(k int) []*tweets.Tweet {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if k >= len(w.tweets) {
		// Return all tweets if k is greater than or equal to window size
		copyTweets := make([]*tweets.Tweet, len(w.tweets))
		copy(copyTweets, w.tweets)
		return copyTweets
	}

	// Return the k most recent tweets (from the end of the slice)
	startIndex := len(w.tweets) - k
	copyTweets := make([]*tweets.Tweet, k)
	copy(copyTweets, w.tweets[startIndex:])
	return copyTweets
}

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
	// Set queue size limits to enable flow control
	args := amqp.Table{
		"x-max-length":       int32(100000), // Maximum number of messages in queue
		"x-overflow":         "drop-head",   // Drop oldest messages when limit reached
		"x-max-length-bytes": int32(0),      // No byte limit (only message count)
	}

	q, err := ch.QueueDeclare(
		"tweet_in", // name
		true,       // durable
		false,      // delete when unused
		false,      // exclusive
		false,      // no-wait
		args,       // arguments with size limits
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, nil, amqp.Queue{}, err
	}
	return conn, ch, q, nil
}

// Helper: Initialize global variables and mappings - COMMENTED OUT for on-the-fly generation test
func initializeGlobalState() {
	// tokenToThreePK = make(map[string]tweets.ThreePartKey)
	// threePKToToken = make(map[tweets.ThreePartKey]string)
}

// Helper: Initialize pipeline components
func initializePipeline(cfg *Config) error {
	pipeline.SetGlobalArrayLen(cfg.BWArrayLen)

	inboundTokenQueue = pipeline.NewTokenQueue()

	freqClasses = cfg.FreqClasses
	if freqClasses <= 0 {
		return fmt.Errorf("freq_classes must be > 0 in config, got %d", freqClasses)
	}

	// Initialize the FrequencyComputationThread
	fct = pipeline.NewFrequencyComputationThread(
		GlobalTokenCounter,
		inboundTokenQueue,
		cfg.FreqClasses,
		cfg.WindowSize,
		cfg.TokenPersistFiles,
		cfg.RebuildEveryFiles,
		cfg.Persistence.StateDir,
		cfg.MinCountThreshold,
	)
	fct.Start()

	// Check if filters are already available from persisted state and trigger immediate rebuild if so
	fct.CheckAndTriggerInitialRebuild()

	// Use z-score array if provided, otherwise fall back to single z-score
	if len(cfg.ZScores) > 0 {
		if len(cfg.ZScores) != freqClasses {
			log.Fatalf("z_scores array length (%d) does not match freq_classes (%d); must provide one z-score per class (freq_classes=%d)",
				len(cfg.ZScores), freqClasses, freqClasses)
		}
		freqClassProcessor = pipeline.NewFrequencyClassProcessorWithZScores(freqClasses, cfg.BWArrayLen, cfg.ZScores, cfg.SkipFrequencyClasses, cfg.LogDir)
	} else {
		log.Fatalf("z_scores array must be provided in config.yaml; single z_score is obsolete.")
	}
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
		// Stop CPU profiling before exiting to ensure profile data is written
		pprof.StopCPUProfile()
		os.Exit(0)
	}()
}

// Helper: Setup RabbitMQ consumer
func setupRabbitMQConsumer(ch *amqp.Channel, q amqp.Queue) (<-chan amqp.Delivery, error) {
	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		false,  // auto-ack (changed to false for manual acknowledgments)
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
	enableProfiling := flag.Bool("profile", false, "Enable CPU profiling (creates cpu.prof)")
	flag.Parse()

	// Start CPU profiling if enabled
	if *enableProfiling {
		cpuProfile, err := os.Create("cpu.prof")
		if err != nil {
			log.Fatalf("Failed to create CPU profile: %v", err)
		}
		defer cpuProfile.Close()
		if err := pprof.StartCPUProfile(cpuProfile); err != nil {
			log.Fatalf("Failed to start CPU profile: %v", err)
		}
		defer pprof.StopCPUProfile()
		fmt.Println("CPU profiling enabled - will create cpu.prof file")
	}

	// Load config from YAML file.
	cfg, err := loadAndValidateConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	fmt.Printf("*** CONFIG LOADED SUCCESSFULLY ***\n")

	// Verbose mode test message and z-score array print
	if cfg.Verbose {
		fmt.Printf("*** VERBOSE MODE ENABLED (config.yaml) ***\n")
		fmt.Printf("Z-scores per frequency class: %v\n", cfg.ZScores)
	}

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

	// Initialize pre-compiled regexes for tokenization
	urlRegex = regexp.MustCompile(`(https?://[^\s]+|www\.[^\s]+)`)
	apostropheRegex = regexp.MustCompile(`'.*`)

	// Initialize the recent tweet window
	windowSize := cfg.WindowBatches * cfg.BatchSize
	recentTweetWindow = NewRecentTweetWindow(windowSize)

	// Load persisted state if requested
	if *loadState {
		loadPersistedState(cfg.Persistence.StateDir, cfg.FreqClasses, cfg)
	}

	initializeGlobalState()

	err = initializePipeline(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize pipeline: %v", err)
	}
	defer fct.Stop()
	defer freqClassProcessor.Stop()

	// Start the analysis thread
	startAnalysisThread(freqClassProcessor.GetResultChannel(), cfg)

	timestamp := time.Now().Format("20060102_150405")
	clusterFileName := fmt.Sprintf("clusters_%s.txt", timestamp)
	clusterOutputFilePath = filepath.Join(cfg.LogDir, clusterFileName)
	fmt.Printf("*** CLUSTER OUTPUT WILL BE SAVED TO: %s ***\n", clusterOutputFilePath)

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
			// Log parse errors and reject the message (don't requeue)
			slog.Warn("Failed to parse tweet, rejecting message", "error", err, "raw_row", string(msg.Body))
			msg.Reject(false) // false = don't requeue
			continue
		}
		// Only print the tweet if the flag is set
		if *printTweets {
			fmt.Printf("Parsed Tweet: %+v\n", tweet)
		}

		// Add tweet to recent tweet window
		recentTweetWindow.Add(tweet)

		// Always add new tweet tokens to the inbound queue for FCT to build frequency filters
		if len(tweet.Tokens) > 0 {
			inboundTokenQueue.Enqueue(tweet.Tokens)

			// Route each token to its appropriate frequency class (only if filters are available)
			if pipeline.HasGlobalFilters() {
				// Debug: Log when filters are available (verbose only)
				if cfg.Verbose && TotalTweetsRead%10000 == 0 {
					slog.Info("Filters are available for token routing",
						"tweet_count", TotalTweetsRead,
						"num_filters", pipeline.GetGlobalFiltersCount())
				}

				// CRITICAL TOKEN ROUTING LOGIC - DO NOT MODIFY
				// This section has been the source of multiple bugs where tokens were incorrectly dropped.
				// The logic is now correct and verified:
				// 1. Every token gets processed (no filtering/dropping)
				// 2. Existing tokens get their assigned frequency class
				// 3. New tokens get 3PK created and assigned to least frequent class (Class 6)
				// 4. All tokens are routed to their appropriate frequency class
				//
				// DO NOT add any master filter checks or token filtering here.
				// DO NOT modify the token routing logic without explicit approval.
				// The GetTokenInfo function handles all the necessary logic correctly.

				// Route tokens to frequency classes
				for _, token := range tweet.Tokens {
					// Get token info (3PK and frequency class) in a single operation
					threePK, freqClass, exists := pipeline.GetTokenInfo(token)
					if !exists {
						// New token: create 3PK, insert into mapping, assign to least frequent class
						threePK = pipeline.GenerateThreePartKey(token)   // This inserts into the mapping
						freqClass = pipeline.GetGlobalFiltersCount() - 1 // Least frequent class (highest number)
					}

					// Enqueue to appropriate frequency class
					freqClassProcessor.EnqueueToFrequencyClass(freqClass, threePK)
				}

			} else {
				// No filters available yet - log occasionally (verbose only)
				if cfg.Verbose && TotalTweetsRead%10000 == 0 {
					slog.Info("No frequency class filters available yet",
						"tweet_count", TotalTweetsRead)
				}
				// No filters available yet - log occasionally (verbose only)
				if cfg.Verbose && TotalTweetsRead%10000 == 0 {
					slog.Info("Skipping batch termination - no frequency class filters available yet",
						"tweet_count", TotalTweetsRead,
						"batch_size", cfg.BatchSize)
				}
			}
		}

		// Send termination signals to busy word processors every batch number of tweets
		// Only send if frequency class filters are available
		if TotalTweetsRead%cfg.BatchSize == 0 && TotalTweetsRead > 0 {
			if pipeline.HasGlobalFilters() {
				terminationSignal := tweets.ThreePartKey{Part1: -1, Part2: -1, Part3: -1}

				// Send termination signal to active frequency class processors only
				activeCount := 0
				for i := 0; i < freqClasses; i++ {
					if freqClassProcessor.IsClassActive(i) {
						freqClassProcessor.EnqueueToFrequencyClass(i, terminationSignal)
						activeCount++
					}
				}

				if cfg.Verbose {
					slog.Info("Main: Sent termination signals to busy word processors",
						"tweet_count", TotalTweetsRead,
						"batch_size", cfg.BatchSize,
						"total_freq_classes", freqClasses,
						"active_freq_classes", activeCount)
				}
			} else {
				// No filters available yet - log occasionally
				if TotalTweetsRead%10000 == 0 {
					slog.Info("Skipping batch termination - no frequency class filters available yet",
						"tweet_count", TotalTweetsRead,
						"batch_size", cfg.BatchSize)
				}
			}
		}

		// Acknowledge successful message processing
		msg.Ack(false) // false = single acknowledgment
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

	// Create timestamped log filename for sortability
	timestamp := time.Now().Format("20060102_150405")
	logPath := filepath.Join(logDir, fmt.Sprintf("pipeline_%s.log", timestamp))

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

	// Print frequency class stats (ordered from lowest to highest class number)
	fmt.Printf("--- Frequency Class Stats ---\n")
	for i := 0; i < freqClasses; i++ {
		queueKey := fmt.Sprintf("freq_class_%d_queue_size", i)
		processorKey := fmt.Sprintf("freq_class_%d_tokens_processed", i)
		queueSize := freqClassQueueStats[queueKey]
		tokensProcessed := freqClassProcessorStats[processorKey]

		// Get distinct token count for this frequency class
		distinctTokens := 0
		if pipeline.HasGlobalFilters() {
			filters := pipeline.GetGlobalFilters()
			if i < len(filters) {
				if setFilter, ok := filters[i].(*pipeline.SetFilter); ok {
					distinctTokens = setFilter.TokenCount()
				}
			}
		}

		fmt.Printf("Class %2d: Queue=%6d, Processed=%8d, Distinct=%6d\n", i, queueSize, tokensProcessed, distinctTokens)
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
	if len(record) < 11 {
		return nil, fmt.Errorf("expected at least 11 fields, got %d", len(record))
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
		RetweetCount: 0,          // TODO: parse record[3] as int
		Language:     record[10], // Language field from CSV
		Tokens:       nil,        // We'll fill this in below
	}

	// Filter by language if language filtering is enabled
	if cfg.Analysis.LanguageFilter != "" && cfg.Analysis.LanguageFilter != "all" {
		// Case-insensitive comparison
		if strings.ToLower(tweet.Language) != strings.ToLower(cfg.Analysis.LanguageFilter) {
			return nil, fmt.Errorf("tweet language '%s' filtered out (filter: %s)", tweet.Language, cfg.Analysis.LanguageFilter)
		}
	}

	// Step 1: Tokenize the tweet text.
	// - Convert to lowercase
	// - Remove punctuation
	// - Remove apostrophes and what follows
	// - Split on whitespace
	tokens := simpleTokenize(tweet.Text, cfg)
	tweet.Tokens = tokens // Store tokens in the Tweet struct

	// Generate ThreePartKeys on-the-fly (no global mappings)
	var threePKs []tweets.ThreePartKey
	for _, token := range tokens {
		threePK := pipeline.GenerateThreePartKey(token)
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
	totalProcessed := 0
	totalRejected := 0
	rejectedByMaxLength := 0
	rejectedByDiversity := 0
	rejectedByRepetition := 0
	rejectedByCaseAlt := 0
	rejectedByNumberMix := 0
	rejectedByHashtag := 0
	rejectedByUrl := 0
	rejectedByAllCaps := 0

	for _, token := range tokens {
		totalProcessed++

		// Remove URLs from this token if enabled
		if cfg.TokenFilters.RemoveUrls {
			token = urlRegex.ReplaceAllString(token, "")
			if token == "" {
				continue
			}
		}

		// Remove apostrophes and what follows (e.g., "don't" -> "don", "Harry's" -> "Harry")
		token = apostropheRegex.ReplaceAllString(token, "")
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

		// Apply token filters if enabled and track rejections
		if cfg.TokenFilters.Enabled {
			rejected := false

			// Max length filter
			if cfg.TokenFilters.MaxLength > 0 && len(cleanToken) > cfg.TokenFilters.MaxLength {
				rejectedByMaxLength++
				rejected = true
			}

			// Character diversity filter (only for long tokens)
			if !rejected && len(cleanToken) >= cfg.TokenFilters.MinCharacterDiversityLowerLimit && cfg.TokenFilters.MinCharacterDiversity > 0 {
				uniqueChars := make(map[rune]bool)
				for _, char := range cleanToken {
					uniqueChars[char] = true
				}
				diversity := float64(len(uniqueChars)) / float64(len(cleanToken))
				if diversity < cfg.TokenFilters.MinCharacterDiversity {
					rejectedByDiversity++
					rejected = true
				}
			}

			// Character repetition filter
			if !rejected && cfg.TokenFilters.MaxCharacterRepetition > 0 {
				repetitionCount := 0
				for i := 1; i < len(cleanToken); i++ {
					if cleanToken[i] == cleanToken[i-1] {
						repetitionCount++
					}
				}
				repetitionRatio := float64(repetitionCount) / float64(len(cleanToken))
				if repetitionRatio > cfg.TokenFilters.MaxCharacterRepetition {
					rejectedByRepetition++
					rejected = true
				}
			}

			// Case alternation filter
			if !rejected && cfg.TokenFilters.MaxCaseAlternations > 0 {
				caseChanges := 0
				for i := 1; i < len(cleanToken); i++ {
					if (cleanToken[i] >= 'A' && cleanToken[i] <= 'Z' && cleanToken[i-1] >= 'a' && cleanToken[i-1] <= 'z') ||
						(cleanToken[i] >= 'a' && cleanToken[i] <= 'z' && cleanToken[i-1] >= 'A' && cleanToken[i-1] <= 'Z') {
						caseChanges++
					}
				}
				caseChangeRatio := float64(caseChanges) / float64(len(cleanToken))
				if caseChangeRatio > cfg.TokenFilters.MaxCaseAlternations {
					rejectedByCaseAlt++
					rejected = true
				}
			}

			// Number-letter mixing filter
			if !rejected && cfg.TokenFilters.MaxNumberLetterMix > 0 {
				digitCount := 0
				for _, char := range cleanToken {
					if char >= '0' && char <= '9' {
						digitCount++
					}
				}
				digitRatio := float64(digitCount) / float64(len(cleanToken))
				if digitRatio > cfg.TokenFilters.MaxNumberLetterMix {
					rejectedByNumberMix++
					rejected = true
				}
			}

			// Hashtag filter
			if !rejected && cfg.TokenFilters.RejectHashtags && strings.HasPrefix(cleanToken, "#") {
				rejectedByHashtag++
				rejected = true
			}

			// URL filter
			if !rejected && cfg.TokenFilters.RejectUrls && (strings.HasPrefix(cleanToken, "http") || strings.HasPrefix(cleanToken, "www")) {
				rejectedByUrl++
				rejected = true
			}

			// All caps long filter
			if !rejected && cfg.TokenFilters.RejectAllCapsLong && len(cleanToken) >= cfg.TokenFilters.AllCapsLowerLimit {
				allCaps := true
				for _, char := range cleanToken {
					if char < 'A' || char > 'Z' {
						allCaps = false
						break
					}
				}
				if allCaps {
					rejectedByAllCaps++
					rejected = true
				}
			}

			if rejected {
				totalRejected++
				continue
			}
		}

		// Convert to lowercase for final output
		cleanToken = strings.ToLower(cleanToken)
		processedTokens = append(processedTokens, cleanToken)
	}

	// Update statistics in a single batch operation
	if cfg.TokenFilters.Enabled {
		TokenFilterStats.mu.Lock()
		TokenFilterStats.TotalTokensProcessed += totalProcessed
		TokenFilterStats.TotalTokensRejected += totalRejected
		TokenFilterStats.RejectedByMaxLength += rejectedByMaxLength
		TokenFilterStats.RejectedByDiversity += rejectedByDiversity
		TokenFilterStats.RejectedByRepetition += rejectedByRepetition
		TokenFilterStats.RejectedByCaseAlt += rejectedByCaseAlt
		TokenFilterStats.RejectedByNumberMix += rejectedByNumberMix
		TokenFilterStats.RejectedByHashtag += rejectedByHashtag
		TokenFilterStats.RejectedByUrl += rejectedByUrl
		TokenFilterStats.RejectedByAllCaps += rejectedByAllCaps
		TokenFilterStats.mu.Unlock()
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

	// Max length filter
	if cfg.TokenFilters.MaxLength > 0 && len(token) > cfg.TokenFilters.MaxLength {
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
			return true
		}
	}

	// Hashtag filter
	if cfg.TokenFilters.RejectHashtags && strings.HasPrefix(token, "#") {
		return true
	}

	// URL filter
	if cfg.TokenFilters.RejectUrls && (strings.HasPrefix(token, "http") || strings.HasPrefix(token, "www")) {
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
func loadPersistedState(stateDir string, freqClasses int, cfg *Config) {
	fmt.Println("=== LOADING PERSISTED STATE ===")

	// Check if any of the files exist
	tokenCounterPath := filepath.Join(stateDir, "token_counter.json")
	freqClassPath := filepath.Join(stateDir, "frequency_classes.json")

	// If none of the files exist, just return and let the normal program run
	_, err1 := os.Stat(tokenCounterPath)
	_, err2 := os.Stat(freqClassPath)
	if os.IsNotExist(err1) && os.IsNotExist(err2) {
		fmt.Println("No persisted state files found. Starting fresh.")
		fmt.Println("=== PERSISTED STATE LOADING COMPLETE ===")
		return
	}

	// Load TokenCounter if it exists and rebuild frequency class filters
	tempTokenCounter := pipeline.NewTokenCounter()
	if err := tempTokenCounter.LoadFromFile(tokenCounterPath); err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			fmt.Printf("TokenCounter file not found: %s\n", tokenCounterPath)
		} else {
			fmt.Printf("Failed to load TokenCounter: %v\n", err)
		}
	} else {
		loadStartTime := time.Now()
		counts := tempTokenCounter.Counts()
		totalTokens := 0
		for _, count := range counts {
			totalTokens += count
		}
		loadDuration := time.Since(loadStartTime)
		fmt.Printf("TokenCounter loaded: %d total tokens (%d distinct tokens) in %v\n", totalTokens, len(counts), loadDuration)

		// Load the token counts into the global token counter for the FCT to use
		populateStartTime := time.Now()
		fmt.Printf("Starting to populate global token counter with %d total tokens...\n", totalTokens)

		// Use the fast direct set method instead of incrementing millions of times
		GlobalTokenCounter.SetCountsDirectly(counts)

		populateDuration := time.Since(populateStartTime)
		fmt.Printf("Global token counter populated with %d total tokens in %v\n", totalTokens, populateDuration)

		// Rebuild frequency class filters from the loaded token counts
		rebuildStartTime := time.Now()
		fmt.Printf("Rebuilding frequency class filters from loaded token counts...\n")
		var result pipeline.FreqClassResult
		if cfg.MinCountThreshold > 0 {
			result = pipeline.BuildFrequencyClassHashSetsAdaptive(counts, freqClasses, cfg.MinCountThreshold)
		} else {
			result = pipeline.BuildFrequencyClassHashSets(counts, freqClasses, nil, nil)
		}
		pipeline.SetGlobalFilters(result.Filters)
		rebuildDuration := time.Since(rebuildStartTime)
		fmt.Printf("Frequency class filters rebuilt: %d classes in %v\n", len(result.Filters), rebuildDuration)
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

	fmt.Println("=== PERSISTED STATE LOADING COMPLETE ===")
}
