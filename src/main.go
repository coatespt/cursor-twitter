package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/streadway/amqp"
	"gopkg.in/yaml.v2"

	"cursor-twitter/src/filter"
	"cursor-twitter/src/pipeline"
	"cursor-twitter/src/tweets"
)

// LogLevel represents the logging level
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

// Logger provides thread-safe logging with lazy evaluation
type Logger struct {
	level   LogLevel
	verbose bool
	stderr  *os.File
	mu      sync.RWMutex
}

// Global logger instance
var globalLogger *Logger

// Global batch window for persistence tracking
var (
	batchWindow      []*Batch
	batchWindowMutex sync.RWMutex
)

// addBatchToWindow adds a new batch to the global batch window and maintains the window size
func addBatchToWindow(batch *Batch, cfg *Config) {
	batchWindowMutex.Lock()
	defer batchWindowMutex.Unlock()

	// Add the new batch
	batchWindow = append(batchWindow, batch)

	// Maintain window size by removing old batches
	maxBatches := cfg.Analysis.WindowBatchesPersistence
	if maxBatches <= 0 {
		maxBatches = 6 // Default value
	}

	// Remove oldest batches if we exceed the window size
	for len(batchWindow) > maxBatches {
		batchWindow = batchWindow[1:]
	}
}

// getBatchWindow returns a copy of the current batch window
func getBatchWindow() []*Batch {
	batchWindowMutex.RLock()
	defer batchWindowMutex.RUnlock()

	// Return a copy to avoid race conditions
	result := make([]*Batch, len(batchWindow))
	copy(result, batchWindow)
	return result
}

// Log logs a message with lazy evaluation to avoid expensive string formatting when disabled
func Log(level LogLevel, messageFn func() string) {
	if globalLogger == nil {
		return
	}

	globalLogger.mu.RLock()
	defer globalLogger.mu.RUnlock()

	// Check level BEFORE calling expensive function
	if level < globalLogger.level && !globalLogger.verbose {
		return // Exit early, messageFn never called
	}

	fmt.Fprintf(globalLogger.stderr, "%s\n", messageFn())
}

// Convenience functions for different log levels
func LogDebug(messageFn func() string) { Log(DEBUG, messageFn) }
func LogInfo(messageFn func() string)  { Log(INFO, messageFn) }
func LogWarn(messageFn func() string)  { Log(WARN, messageFn) }
func LogError(messageFn func() string) { Log(ERROR, messageFn) }

// InitializeLogger sets up the global logger
func InitializeLogger(verbose bool, level LogLevel, logDir string) {
	// Ensure log directory exists
	if err := os.MkdirAll(logDir, 0755); err != nil {
		// If we can't create the log directory, fall back to stderr
		globalLogger = &Logger{
			level:   level,
			verbose: verbose,
			stderr:  os.Stderr,
		}
		return
	}

	// Create log file with timestamp
	timestamp := time.Now().Format("20060102_150405")
	logFileName := fmt.Sprintf("pipeline_%s.log", timestamp)
	logFilePath := filepath.Join(logDir, logFileName)

	logFile, err := os.Create(logFilePath)
	if err != nil {
		// If we can't create the log file, fall back to stderr
		globalLogger = &Logger{
			level:   level,
			verbose: verbose,
			stderr:  os.Stderr,
		}
		return
	}

	globalLogger = &Logger{
		level:   level,
		verbose: verbose,
		stderr:  logFile,
	}
}

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
		RejectAtMentions                bool    `yaml:"reject_at_mentions"`
		RejectUrls                      bool    `yaml:"reject_urls"`
		RejectAllCapsLong               bool    `yaml:"reject_all_caps_long"`
		AllCapsLowerLimit               int     `yaml:"all_caps_lower_limit"`
		RemoveUrls                      bool    `yaml:"remove_urls"`
		ApostropheHandling              string  `yaml:"apostrophe_handling"`
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
		ClusteringMethod             string  `yaml:"clustering_method"`              // Method for clustering: "graph" or "kmeans"
		OutputMode                   string  `yaml:"output_mode"`                    // Output mode: "verbose" or "human"
		KmeansK                      int     `yaml:"kmeans_k"`                       // Number of clusters for k-means clustering
		KmeansUseAllWords            bool    `yaml:"kmeans_use_all_words"`           // Use all words in tweet vectors for k-means clustering
		MinClusterSize               int     `yaml:"min_cluster_size"`               // Minimum number of tweets in a cluster for it to be included in the output
		// Persistence window configuration for tracking clusters across multiple batches
		WindowBatchesPersistence      int `yaml:"window_batches_persistence"`       // M
		WindowBatchesPersistenceCheck int `yaml:"window_batches_persistence_check"` // K
		// Minimum number of shared busy words required for clusters to be considered related (for persistence tracking)
		MinSharedBusyWordsForPersistence int `yaml:"min_shared_busywords_for_persistence"` // Relationship strength threshold
		// Method for determining cluster relationships across batches: "busy_words" or "full_text"
		PersistenceClusteringMethod string `yaml:"persistence_clustering_method"` // Cross-batch relationship detection method
	} `yaml:"analysis"`
}

// Global stats counters
var (
	TotalTweetsRead    int
	TotalTokensCounted int
	lastStatsTime      time.Time
	lastTweetCount     int
	freqClasses        int // Number of frequency classes from config

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

// Pre-compiled regexes for tokenization (compiled once at startup)
var (
	urlRegex        *regexp.Regexp
	apostropheRegex *regexp.Regexp
)

// Add at the top-level globals:
var clusterOutputFilePath string
var clusterOutputFileOnce sync.Once

// Global tweet queue for clustering
type TweetQueue struct {
	mu      sync.RWMutex
	tweets  []*tweets.Tweet
	maxSize int
}

func NewTweetQueue(maxSize int) *TweetQueue {
	return &TweetQueue{
		tweets:  make([]*tweets.Tweet, 0, maxSize),
		maxSize: maxSize,
	}
}

func (q *TweetQueue) Enqueue(tweet *tweets.Tweet) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.tweets = append(q.tweets, tweet)

	// Maintain max size
	if len(q.tweets) > q.maxSize {
		q.tweets = q.tweets[1:]
	}
}

func (q *TweetQueue) GetRecentTweets(count int) []*tweets.Tweet {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if count >= len(q.tweets) {
		// Return copy of all tweets
		result := make([]*tweets.Tweet, len(q.tweets))
		copy(result, q.tweets)
		return result
	}

	// Return copy of most recent tweets
	start := len(q.tweets) - count
	result := make([]*tweets.Tweet, count)
	copy(result, q.tweets[start:])
	return result
}

func (q *TweetQueue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.tweets)
}

var globalTweetQueue *TweetQueue

// Analysis thread for processing busy word results and running clustering
func startAnalysisThread(resultChannel <-chan pipeline.BusyWordResult, cfg *Config, loadedState map[string]int) {
	go func() {
		resultCount := 0

		// Handle loaded state if provided
		if loadedState != nil {
			LogInfo(func() string {
				totalTokens := 0
				for _, count := range loadedState {
					totalTokens += count
				}
				return fmt.Sprintf("Informing FCT to load state with %d total tokens...", totalTokens)
			})

			// Tell FCT to load its own state
			fct.LoadState(loadedState)

			// Rebuild frequency class filters from the loaded token counts
			LogInfo(func() string { return "Rebuilding frequency class filters from loaded token counts..." })
			rebuildStartTime := time.Now()
			var result pipeline.FreqClassResult
			if cfg.MinCountThreshold > 0 {
				result = pipeline.BuildFrequencyClassHashSetsAdaptive(loadedState, cfg.FreqClasses, cfg.MinCountThreshold)
			} else {
				result = pipeline.BuildFrequencyClassHashSets(loadedState, cfg.FreqClasses, nil, nil)
			}
			pipeline.SetGlobalFilters(result.Filters)
			rebuildDuration := time.Since(rebuildStartTime)
			LogInfo(func() string {
				return fmt.Sprintf("Frequency class filters rebuilt: %d classes in %v", len(result.Filters), rebuildDuration)
			})
		} else {
			// If no state loaded, we need to wait for the FCT to build initial filters
			// This is a temporary solution - ideally we'd have a proper synchronization mechanism
			LogInfo(func() string { return "No state loaded - waiting for FCT to build initial filters..." })
			for !pipeline.HasGlobalFilters() {
				time.Sleep(100 * time.Millisecond)
			}
			LogInfo(func() string { return "Initial filters are now available" })
		}

		// Track results by batch
		currentBatch := make(map[int][]string) // class -> busy words
		currentBatchNumber := -1

		for result := range resultChannel {
			resultCount++

			// Check if this is a new batch
			if currentBatchNumber != result.BatchNumber {
				// Run clustering for previous batch if it exists
				if currentBatchNumber >= 0 {
					// Get recent tweets from global queue for clustering
					recentTweets := globalTweetQueue.GetRecentTweets(cfg.WindowBatches * cfg.BatchSize)
					runClusteringForBatch(currentBatch, recentTweets, currentBatchNumber, cfg)
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
				LogError(func() string {
					return fmt.Sprintf("ERROR: %d/%d 3PKs not found in mapping for class %d - this should not happen!",
						notFoundCount, len(result.BusyWord3PKs), result.FrequencyClass)
				})
			}

			// Store results for this class
			currentBatch[result.FrequencyClass] = busyWords
		}

		// Run clustering for final batch
		if currentBatchNumber >= 0 {
			// Get recent tweets from global queue for clustering
			recentTweets := globalTweetQueue.GetRecentTweets(cfg.WindowBatches * cfg.BatchSize)
			runClusteringForBatch(currentBatch, recentTweets, currentBatchNumber, cfg)
		}

		LogInfo(func() string {
			return fmt.Sprintf("Analysis thread stopped after processing %d results", resultCount)
		})
	}()
}

// runClusteringForBatch runs clustering analysis for a batch of busy words and tweets
func runClusteringForBatch(classResults map[int][]string, recentTweets []*tweets.Tweet, batchNumber int, cfg *Config) {
	// Print busy word summary
	printBatchSummary(classResults, batchNumber, cfg)

	// Collect all busy words from specified classes
	allBusyWords := make(map[string]bool)
	allowedClasses := make(map[int]bool)

	// Validate busyword_classes are within valid range
	for _, class := range cfg.BusywordClasses {
		if class < 1 || class > cfg.FreqClasses {
			LogWarn(func() string {
				return fmt.Sprintf("Invalid busyword_class %d (valid range: 1-%d) - skipping", class, cfg.FreqClasses)
			})
			continue
		}
		allowedClasses[class] = true
	}

	if len(allowedClasses) == 0 {
		LogError(func() string {
			return fmt.Sprintf("No valid busyword_classes found - all classes were out of range (1-%d)", cfg.FreqClasses)
		})
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

	// Filter tweets that contain at least minBusyWords busy words
	minBusyWords := cfg.Analysis.MinBusyWordsPerTweet
	if minBusyWords <= 0 {
		minBusyWords = 1
	}

	var tweetsWithBusyWords []*tweets.Tweet
	for _, tweet := range recentTweets {
		busyWordCount := 0
		for _, token := range tweet.Tokens {
			if allBusyWords[token] {
				busyWordCount++
			}
		}
		if busyWordCount >= minBusyWords {
			tweetsWithBusyWords = append(tweetsWithBusyWords, tweet)
		}
	}

	// Sanity checks before proceeding with clustering
	if len(tweetsWithBusyWords) == 0 {
		return
	}

	if len(tweetsWithBusyWords) < 2 {
		return
	}

	if len(allBusyWords) == 0 {
		return
	}

	// Run clustering based on configured method
	clusteringMethod := "graph"
	if cfg.Analysis.ClusteringMethod != "" {
		clusteringMethod = cfg.Analysis.ClusteringMethod
	}

	switch clusteringMethod {
	case "kmeans":
		runKMeansClustering(tweetsWithBusyWords, allBusyWords, cfg, batchNumber)
	case "graph":
		fallthrough
	default:
		runGraphClustering(tweetsWithBusyWords, allBusyWords, cfg, batchNumber)
	}
}

// runKMeansClustering runs k-means clustering on tweets
func runKMeansClustering(tweetsWithBusyWords []*tweets.Tweet, allBusyWords map[string]bool, cfg *Config, batchNumber int) {
	// Use existing k-means clustering function with silent output
	writeOutput := func(format string, args ...interface{}) {
		// Silent - no output needed
	}

	clusters := runKMeansClusteringGo(tweetsWithBusyWords, allBusyWords, cfg, writeOutput, batchNumber)

	// Create a batch from the clusters and add it to the window
	batch := &Batch{
		BatchID:  batchNumber,
		Tweets:   tweetsWithBusyWords,
		Clusters: clusters,
	}
	addBatchToWindow(batch, cfg)

	// Get the current batch window for persistence tracking
	currentBatchWindow := getBatchWindow()

	// Get timestamp for the batch
	var batchTimeStr string
	if len(tweetsWithBusyWords) > 0 {
		firstTweet := tweetsWithBusyWords[0]
		batchTimeStr = time.Unix(firstTweet.Unix, 0).Format("2006-01-02 15:04:05")
	} else {
		batchTimeStr = time.Now().Format("2006-01-02 15:04:05")
	}

	// Collect all clusters for this batch
	var batchClusters []map[string]interface{}

	for i, cluster := range clusters {
		// Get busy words for this cluster
		clusterBusyWords := make(map[string]bool)
		for _, tweet := range cluster.Tweets {
			for _, token := range tweet.Tokens {
				if allBusyWords[token] {
					clusterBusyWords[token] = true
				}
			}
		}

		// Convert to sorted slice for display
		var busyWordsList []string
		for word := range clusterBusyWords {
			busyWordsList = append(busyWordsList, word)
		}
		sort.Strings(busyWordsList)

		// Get timestamp of first tweet for display
		firstTweet := cluster.Tweets[0]
		timeStr := time.Unix(firstTweet.Unix, 0).Format("2006-01-02 15:04:05")

		// Find the most typical tweet in this cluster
		var mostTypicalTweet *tweets.Tweet
		if len(cluster.Tweets) > 1 {
			_, medoidIdx, _, _ := findMostTypicalTweets(cluster.Tweets, cfg.Analysis.MinJaccardSimilarity)
			mostTypicalTweet = cluster.Tweets[medoidIdx]
		} else {
			mostTypicalTweet = cluster.Tweets[0]
		}

		// Get persistence information
		persistenceInfo := getContinuationInfo(cluster, currentBatchWindow, batchNumber, cfg)

		// Create cluster data
		clusterData := map[string]interface{}{
			"cluster_id":         i + 1,
			"size":               cluster.Size,
			"first_tweet_time":   timeStr,
			"busy_words":         busyWordsList,
			"tweets":             cluster.Tweets,
			"most_typical_tweet": mostTypicalTweet,
			"persistence_info":   persistenceInfo,
		}

		batchClusters = append(batchClusters, clusterData)
	}

	// Create batch-level data structure
	batchData := map[string]interface{}{
		"batch_number":   batchNumber,
		"batch_time":     batchTimeStr,
		"method":         "kmeans",
		"total_clusters": len(batchClusters),
		"total_tweets":   len(tweetsWithBusyWords),
		"clusters":       batchClusters,
	}

	OutputClusterWithConfig(batchData, cfg)
}

// runGraphClustering runs graph-based clustering on tweets
func runGraphClustering(tweetsWithBusyWords []*tweets.Tweet, allBusyWords map[string]bool, cfg *Config, batchNumber int) {
	// Perform optimized graph clustering
	clusterer := pipeline.NewOptimizedTweetClusterer(
		cfg.Analysis.MinJaccardSimilarity,
		cfg.Analysis.MaxTweetsToCluster,
	)

	result := clusterer.ClusterTweets(tweetsWithBusyWords, allBusyWords, batchNumber)

	// Create a batch from the clusters and add it to the window
	batch := &Batch{
		BatchID:  batchNumber,
		Tweets:   tweetsWithBusyWords,
		Clusters: result.Clusters,
	}
	addBatchToWindow(batch, cfg)

	// Get the current batch window for persistence tracking
	currentBatchWindow := getBatchWindow()

	// Get timestamp for the batch
	var batchTimeStr string
	if len(tweetsWithBusyWords) > 0 {
		firstTweet := tweetsWithBusyWords[0]
		batchTimeStr = time.Unix(firstTweet.Unix, 0).Format("2006-01-02 15:04:05")
	} else {
		batchTimeStr = time.Now().Format("2006-01-02 15:04:05")
	}

	// Collect all clusters for this batch
	var batchClusters []map[string]interface{}

	for i, cluster := range result.Clusters {
		// Get busy words for this cluster
		clusterBusyWords := make(map[string]bool)
		for _, tweet := range cluster.Tweets {
			for _, token := range tweet.Tokens {
				if allBusyWords[token] {
					clusterBusyWords[token] = true
				}
			}
		}

		// Convert to sorted slice for display
		var busyWordsList []string
		for word := range clusterBusyWords {
			busyWordsList = append(busyWordsList, word)
		}
		sort.Strings(busyWordsList)

		// Get timestamp of first tweet for display
		firstTweet := cluster.Tweets[0]
		timeStr := time.Unix(firstTweet.Unix, 0).Format("2006-01-02 15:04:05")

		// Find the most typical tweet in this cluster
		var mostTypicalTweet *tweets.Tweet
		if len(cluster.Tweets) > 1 {
			_, medoidIdx, _, _ := findMostTypicalTweets(cluster.Tweets, cfg.Analysis.MinJaccardSimilarity)
			mostTypicalTweet = cluster.Tweets[medoidIdx]
		} else {
			mostTypicalTweet = cluster.Tweets[0]
		}

		// Get persistence information
		persistenceInfo := getContinuationInfo(cluster, currentBatchWindow, batchNumber, cfg)

		// Create cluster data
		clusterData := map[string]interface{}{
			"cluster_id":         i + 1,
			"size":               cluster.Size,
			"first_tweet_time":   timeStr,
			"busy_words":         busyWordsList,
			"tweets":             cluster.Tweets,
			"most_typical_tweet": mostTypicalTweet,
			"persistence_info":   persistenceInfo,
		}

		batchClusters = append(batchClusters, clusterData)
	}

	// Create batch-level data structure
	batchData := map[string]interface{}{
		"batch_number":   batchNumber,
		"batch_time":     batchTimeStr,
		"method":         "graph",
		"total_clusters": len(batchClusters),
		"total_tweets":   len(tweetsWithBusyWords),
		"clusters":       batchClusters,
	}

	OutputClusterWithConfig(batchData, cfg)
}

// printBatchSummary prints a summary of all busy words found in a batch
func printBatchSummary(classResults map[int][]string, batchNumber int, cfg *Config) {
	totalBusyWords := 0
	classesWithWords := 0

	LogInfo(func() string { return fmt.Sprintf("BATCH %d ANALYSIS SUMMARY", batchNumber) })

	// Get sorted class indices to ensure consistent ordering
	classIndices := make([]int, 0, len(classResults))
	for classIndex := range classResults {
		classIndices = append(classIndices, classIndex)
	}
	sort.Ints(classIndices)

	// Log classes in sorted order
	for _, classIndex := range classIndices {
		words := classResults[classIndex]
		totalBusyWords += len(words)
		if len(words) > 0 {
			classesWithWords++
			LogInfo(func() string {
				return fmt.Sprintf("Class %d: %d busy words - %s", classIndex, len(words), strings.Join(words, ", "))
			})
		} else {
			LogInfo(func() string {
				return fmt.Sprintf("Class %d: %d busy words", classIndex, len(words))
			})
		}
	}

	LogInfo(func() string {
		return fmt.Sprintf("TOTAL: %d busy words across %d classes", totalBusyWords, classesWithWords)
	})
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
		pipeline.NewTokenCounter(),
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
		LogInfo(func() string { return "CPU profiling enabled - will create cpu.prof file" })
	}

	// Load config from YAML file.
	cfg, err := loadAndValidateConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize our custom logging framework
	InitializeLogger(cfg.Verbose, INFO, cfg.LogDir) // Default to INFO level, can be made configurable later

	LogInfo(func() string { return "*** CONFIG LOADED SUCCESSFULLY ***" })

	// Verbose mode test message and z-score array print
	if cfg.Verbose {
		LogInfo(func() string { return "*** VERBOSE MODE ENABLED (config.yaml) ***" })
		LogInfo(func() string { return fmt.Sprintf("Z-scores per frequency class: %v", cfg.ZScores) })
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

	// TODO: Analysis thread will handle tweet window management

	initializeGlobalState()

	startStatsPrinter()

	err = initializePipeline(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize pipeline: %v", err)
	}

	// Load persisted state if requested (after pipeline initialization)
	var loadedState map[string]int
	if *loadState {
		loadedState = loadPersistedState(cfg.Persistence.StateDir, cfg.FreqClasses, cfg)
	}

	defer fct.Stop()
	defer freqClassProcessor.Stop()

	// Initialize global tweet queue for clustering
	globalTweetQueue = NewTweetQueue(cfg.WindowBatches * cfg.BatchSize)

	// Start the analysis thread to process busy word results and run clustering
	startAnalysisThread(freqClassProcessor.GetResultChannel(), cfg, loadedState)

	// Wait for frequency class filters to become available before starting tweet processing
	LogInfo(func() string { return "Waiting for frequency class filters to become available..." })
	for !pipeline.HasGlobalFilters() {
		time.Sleep(100 * time.Millisecond)
	}
	LogInfo(func() string {
		return fmt.Sprintf("Frequency class filters are now available (%d classes)", pipeline.GetGlobalFiltersCount())
	})

	timestamp := time.Now().Format("20060102_150405")
	clusterFileName := fmt.Sprintf("clusters_%s.txt", timestamp)
	clusterOutputFilePath = filepath.Join(cfg.LogDir, clusterFileName)
	LogInfo(func() string { return fmt.Sprintf("Cluster output will be saved to: %s", clusterOutputFilePath) })

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

	for msg := range msgs {
		row := string(msg.Body)
		if cfg.Analysis.LanguageFilter != "" && cfg.Analysis.LanguageFilter != "all" {
			langSuffix := "," + strings.ToLower(cfg.Analysis.LanguageFilter)
			if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(row)), langSuffix) {
				msg.Ack(false)
				continue
			}
		}
		tweet, err := parseCSVToTweet(row, cfg)
		if err != nil {
			// Log parse errors and reject the message (don't requeue)
			slog.Warn("Failed to parse tweet, rejecting message", "error", err, "raw_row", row)
			msg.Reject(false) // false = don't requeue
			continue
		}
		if tweet == nil {
			// Tweet was filtered out (e.g., by language); acknowledge and continue
			msg.Ack(false)
			continue
		}
		if *printTweets {
			// Log to file instead of stdout
			LogDebug(func() string { return fmt.Sprintf("Parsed Tweet: %+v", tweet) })
		}

		// Add tweet to global queue for clustering
		globalTweetQueue.Enqueue(tweet)

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
				// 3. New tokens get 3PK created and assign to least frequent class (Class 6)
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

// createBatchFromClusters creates a batch from clustering results and adds it to the batch window
func createBatchFromClusters(clusters []pipeline.TweetCluster, batchNumber int, cfg *Config) {
	// Collect all tweets that were actually clustered
	var clusteredTweets []*tweets.Tweet
	for _, cluster := range clusters {
		// Assign batch ID to each tweet in the cluster
		for _, tweet := range cluster.Tweets {
			tweet.BatchID = batchNumber
		}
		clusteredTweets = append(clusteredTweets, cluster.Tweets...)
	}

	// TODO: Batch creation will be handled by analysis thread

	// TODO: Batch window management will be handled by analysis thread
}

// getContinuationInfo returns continuation information for a cluster
func getContinuationInfo(currentCluster pipeline.TweetCluster, batchWindow []*Batch, currentBatchID int, cfg *Config) string {
	var continuationBatches []int

	// Check each previous batch in the window
	for _, pastBatch := range batchWindow {
		if pastBatch.BatchID >= currentBatchID {
			continue // Skip current and future batches
		}

		// Check if any cluster in the past batch is similar to the current cluster
		for _, pastCluster := range pastBatch.Clusters {
			if clustersAreRelated(currentCluster, pastCluster, cfg) {
				continuationBatches = append(continuationBatches, pastBatch.BatchID)
				break // Found a continuation, no need to check other clusters in this batch
			}
		}
	}

	// Sort and deduplicate continuation batches
	if len(continuationBatches) > 0 {
		continuationBatches = deduplicateAndSort(continuationBatches)
		return fmt.Sprintf(" (continues from batches %v, current: %d)", continuationBatches, currentBatchID)
	}

	return " (new cluster)"
}

// clustersAreRelated checks if two clusters are related (similar busy words and tweets)
func clustersAreRelated(cluster1, cluster2 pipeline.TweetCluster, cfg *Config) bool {
	// Default to busy_words method if not specified
	method := cfg.Analysis.PersistenceClusteringMethod
	if method == "" {
		method = "busy_words"
	}

	switch method {
	case "full_text":
		return clustersAreRelatedByFullText(cluster1, cluster2, cfg)
	case "busy_words":
		fallthrough
	default:
		return clustersAreRelatedByBusyWords(cluster1, cluster2, cfg)
	}
}

func clustersAreRelatedByBusyWords(cluster1, cluster2 pipeline.TweetCluster, cfg *Config) bool {
	// Check if they share busy words
	sharedWords := 0
	for _, word1 := range cluster1.BusyWords {
		for _, word2 := range cluster2.BusyWords {
			if word1 == word2 {
				sharedWords++
				break
			}
		}
	}

	// Use configurable threshold for relationship strength
	minSharedWords := cfg.Analysis.MinSharedBusyWordsForPersistence
	if len(cluster1.BusyWords) < 3 || len(cluster2.BusyWords) < 3 {
		// Lower threshold for small clusters (use 1 if config value is higher)
		if minSharedWords > 1 {
			minSharedWords = 1
		}
	}

	return sharedWords >= minSharedWords
}

func clustersAreRelatedByFullText(cluster1, cluster2 pipeline.TweetCluster, cfg *Config) bool {
	// Get all unique token sets from both clusters
	tokenSets1 := make(map[string]bool)
	tokenSets2 := make(map[string]bool)

	for _, tweet := range cluster1.Tweets {
		// Use the already normalized and filtered tokens
		tokenKey := strings.Join(tweet.Tokens, " ")
		tokenSets1[tokenKey] = true
	}

	for _, tweet := range cluster2.Tweets {
		// Use the already normalized and filtered tokens
		tokenKey := strings.Join(tweet.Tokens, " ")
		tokenSets2[tokenKey] = true
	}

	// Count shared token sets (equivalent to shared normalized texts)
	sharedTokenSets := 0
	for tokenSet := range tokenSets1 {
		if tokenSets2[tokenSet] {
			sharedTokenSets++
		}
	}

	// Consider clusters related if they share at least one token set
	// This is equivalent to the previous text comparison but much faster
	return sharedTokenSets >= 1
}

// deduplicateAndSort removes duplicates and sorts a slice of integers
func deduplicateAndSort(slice []int) []int {
	seen := make(map[int]bool)
	var result []int

	for _, item := range slice {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	sort.Ints(result)
	return result
}

// processBatchPersistence analyzes the batch window for persistent clusters
func processBatchPersistence(batchWindow []*Batch, cfg *Config) {
	// TODO: This function should be moved to the analysis thread
	// The analysis thread should handle persistence tracking
	return

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
	// Get stats from FCT instead of accessing its internal TokenCounter
	stats := fct.GetStats()
	distinctTokens := stats["distinct_tokens"]

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

	// TODO: Token filter statistics will be handled by analysis thread

	// fmt.Printf("\n--- Pipeline Stats ---\n")
	// fmt.Printf("Total tweets read: %d\n", totalTweets)
	// fmt.Printf("Distinct tokens: %d\n", distinctTokens)
	// fmt.Printf("Inbound token queue size: %d\n", inboundQueueSize)
	// fmt.Printf("Processing rate: %.2f tweets/sec\n", processingRate)
	// fmt.Printf("--- Token Filter Stats ---\n")
	// fmt.Printf("Token filter statistics will be handled by analysis thread\n")

	// Print frequency class stats (ordered from lowest to highest class number)
	slog.Info("--- Frequency Class Stats ---")
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

		slog.Info("Frequency Class Stats", "class", i, "queue", queueSize, "processed", tokensProcessed, "distinct", distinctTokens)
	}
	slog.Info("----------------------")

	// Also log to slog
	slog.Info("Pipeline stats",
		"tweets", totalTweets,
		"tokens", totalTokens,
		"distinct", distinctTokens,
		// "window_size", windowSize, // Removed tweet-based window size
		"inbound_queue_size", inboundQueueSize,
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
			return nil, nil // Not an error, just skip this tweet
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

	// totalProcessed = len(tokens) // TODO: Statistics tracking moved to analysis thread
	for _, token := range tokens {
		// Reject URL tokens if configured to remove URLs
		if cfg.TokenFilters.RemoveUrls {
			if strings.HasPrefix(token, "http") || strings.HasPrefix(token, "www") {
				// rejectedByUrl++ // TODO: Statistics tracking moved to analysis thread
				continue
			}
		}

		// Handle apostrophes based on configuration
		// Note that M'kele M'beme is or O'Connor or O'clock are all legit words, not possessive.
		if strings.Contains(token, "'") {
			switch cfg.TokenFilters.ApostropheHandling {
			case "keep":
				// Leave token as-is
			case "truncate":
				// Remove from apostrophe onwards (e.g., "don't" -> "don", "Harry's" -> "Harry")
				token = apostropheRegex.ReplaceAllString(token, "")
				if token == "" {
					// rejectedByMinLength++ // TODO: Statistics tracking moved to analysis thread
					continue
				}
			case "remove":
				// Remove apostrophe only (e.g., "don't" -> "dont", "Harry's" -> "Harrys")
				token = strings.ReplaceAll(token, "'", "")
				if token == "" {
					// rejectedByMinLength++ // TODO: Statistics tracking moved to analysis thread
					continue
				}
			default:
				// Default to "remove" behavior
				token = strings.ReplaceAll(token, "'", "")
				if token == "" {
					// rejectedByMinLength++ // TODO: Statistics tracking moved to analysis thread
					continue
				}
			}
		}

		// Use the token as-is (punctuation removal removed due to conflict with apostrophe handling)
		cleanToken := token

		// Skip tokens that are too short
		if cfg.MinTokenLen > 0 && len(cleanToken) < cfg.MinTokenLen {
			// rejectedByMinLength++ // TODO: Statistics tracking moved to analysis thread
			continue
		}

		// Apply token filters if enabled - ordered by cost (cheapest first) with early exit
		if cfg.TokenFilters.Enabled {
			// 1. CHEAPEST: Hashtag filter (string prefix check)
			if cfg.TokenFilters.RejectHashtags && strings.HasPrefix(cleanToken, "#") {
				// rejectedByHashtag++ // TODO: Statistics tracking moved to analysis thread
				continue
			}

			// 2. CHEAPEST: At-mention filter (string prefix check)
			if cfg.TokenFilters.RejectAtMentions && strings.HasPrefix(cleanToken, "@") {
				// rejectedByAtMention++ // TODO: Statistics tracking moved to analysis thread
				continue
			}

			// 3. CHEAP: Word filter (map lookup)
			// O(1) lookup.
			if globalWordFilter != nil && globalWordFilter.IsFiltered(cleanToken) {
				// rejectedByWordFilter++ // TODO: Statistics tracking moved to analysis thread
				continue
			}
			// 4. CHEAP: Max length filter (simple integer comparison)
			//
			if cfg.TokenFilters.MaxLength > 0 && len(cleanToken) > cfg.TokenFilters.MaxLength {
				// rejectedByMaxLength++ // TODO: Statistics tracking moved to analysis thread
				continue
			}

			// 9. MOST EXPENSIVE: Character diversity filter (requires map creation)
			//
			if len(cleanToken) >= cfg.TokenFilters.MinCharacterDiversityLowerLimit && cfg.TokenFilters.MinCharacterDiversity > 0 {
				uniqueChars := make(map[rune]bool)
				for _, char := range cleanToken {
					uniqueChars[char] = true
				}
				diversity := float64(len(uniqueChars)) / float64(len(cleanToken))
				if diversity < cfg.TokenFilters.MinCharacterDiversity {
					// rejectedByDiversity++ // TODO: Statistics tracking moved to analysis thread
					continue
				}
			}

			// 7. EXPENSIVE: Character repetition filter (character scan)
			if cfg.TokenFilters.MaxCharacterRepetition > 0 {
				repetitionCount := 0
				for i := 1; i < len(cleanToken); i++ {
					if cleanToken[i] == cleanToken[i-1] {
						repetitionCount++
					}
				}
				repetitionRatio := float64(repetitionCount) / float64(len(cleanToken))
				if repetitionRatio > cfg.TokenFilters.MaxCharacterRepetition {
					// rejectedByRepetition++ // TODO: Statistics tracking moved to analysis thread
					continue
				}
			}

			// 8. EXPENSIVE: Case alternation filter (character scan)
			if cfg.TokenFilters.MaxCaseAlternations > 0 {
				caseChanges := 0
				for i := 1; i < len(cleanToken); i++ {
					if (cleanToken[i] >= 'A' && cleanToken[i] <= 'Z' && cleanToken[i-1] >= 'a' && cleanToken[i-1] <= 'z') ||
						(cleanToken[i] >= 'a' && cleanToken[i] <= 'z' && cleanToken[i-1] >= 'A' && cleanToken[i-1] <= 'Z') {
						caseChanges++
					}
				}
				caseChangeRatio := float64(caseChanges) / float64(len(cleanToken))
				if caseChangeRatio > cfg.TokenFilters.MaxCaseAlternations {
					// rejectedByCaseAlt++ // TODO: Statistics tracking moved to analysis thread
					continue
				}
			}

			// 6. EXPENSIVE: Number-letter mixing filter (character scan)
			if cfg.TokenFilters.MaxNumberLetterMix > 0 {
				numberLetterTransitions := 0
				for i := 1; i < len(cleanToken); i++ {
					prevIsLetter := (cleanToken[i-1] >= 'a' && cleanToken[i-1] <= 'z') || (cleanToken[i-1] >= 'A' && cleanToken[i-1] <= 'Z')
					currIsLetter := (cleanToken[i] >= 'a' && cleanToken[i] <= 'z') || (cleanToken[i] >= 'A' && cleanToken[i] <= 'Z')
					prevIsDigit := cleanToken[i-1] >= '0' && cleanToken[i-1] <= '9'
					currIsDigit := cleanToken[i] >= '0' && cleanToken[i] <= '9'

					if (prevIsLetter && currIsDigit) || (prevIsDigit && currIsLetter) {
						numberLetterTransitions++
					}
				}
				transitionRatio := float64(numberLetterTransitions) / float64(len(cleanToken))
				if transitionRatio > cfg.TokenFilters.MaxNumberLetterMix {
					// rejectedByNumberMix++ // TODO: Statistics tracking moved to analysis thread
					continue
				}
			}

			// 5. CHEAP: URL filter (string contains check)
			if cfg.TokenFilters.RejectUrls && (strings.Contains(cleanToken, "http") || strings.Contains(cleanToken, "www")) {
				// rejectedByUrl++ // TODO: Statistics tracking moved to analysis thread
				continue
			}

			// 10. MOST EXPENSIVE: All caps filter (character scan + length check)
			if cfg.TokenFilters.RejectAllCapsLong && len(cleanToken) >= cfg.TokenFilters.AllCapsLowerLimit {
				allCaps := true
				for _, char := range cleanToken {
					if char < 'A' || char > 'Z' {
						allCaps = false
						break
					}
				}
				if allCaps {
					// rejectedByAllCaps++ // TODO: Statistics tracking moved to analysis thread
					continue
				}
			}
		}

		// Convert to lowercase for consistency
		cleanToken = strings.ToLower(cleanToken)
		processedTokens = append(processedTokens, cleanToken)
	}

	// TODO: Token filter statistics will be handled by analysis thread

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
func loadPersistedState(stateDir string, freqClasses int, cfg *Config) map[string]int {
	LogInfo(func() string { return "=== LOADING PERSISTED STATE ===" })

	// Check if any of the files exist
	tokenCounterPath := filepath.Join(stateDir, "token_counter.json")
	freqClassPath := filepath.Join(stateDir, "frequency_classes.json")

	// If none of the files exist, just return and let the normal program run
	_, err1 := os.Stat(tokenCounterPath)
	_, err2 := os.Stat(freqClassPath)
	if os.IsNotExist(err1) && os.IsNotExist(err2) {
		LogInfo(func() string { return "No persisted state files found. Starting fresh." })
		LogInfo(func() string { return "=== PERSISTED STATE LOADING COMPLETE ===" })
		return nil
	}

	// Load TokenCounter if it exists
	tempTokenCounter := pipeline.NewTokenCounter()
	if err := tempTokenCounter.LoadFromFile(tokenCounterPath); err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			LogInfo(func() string { return fmt.Sprintf("TokenCounter file not found: %s", tokenCounterPath) })
		} else {
			LogInfo(func() string { return fmt.Sprintf("Failed to load TokenCounter: %v", err) })
		}
		return nil
	}

	loadStartTime := time.Now()
	counts := tempTokenCounter.Counts()
	totalTokens := 0
	for _, count := range counts {
		totalTokens += count
	}
	loadDuration := time.Since(loadStartTime)
	LogInfo(func() string {
		return fmt.Sprintf("TokenCounter loaded: %d total tokens (%d distinct tokens) in %v", totalTokens, len(counts), loadDuration)
	})

	// Load FrequencyClassResult if it exists
	var tempFreqClassResult pipeline.FreqClassResult
	if err := tempFreqClassResult.LoadFromFile(freqClassPath); err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			LogInfo(func() string { return fmt.Sprintf("FrequencyClassResult file not found: %s", freqClassPath) })
		} else {
			LogInfo(func() string { return fmt.Sprintf("Failed to load FrequencyClassResult: %v", err) })
		}
	} else {
		classes := len(tempFreqClassResult.Filters)
		LogInfo(func() string { return fmt.Sprintf("FrequencyClassResult loaded: %d classes", classes) })
	}

	LogInfo(func() string { return "=== PERSISTED STATE LOADING COMPLETE ===" })
	return counts
}

// Simple k-means implementation for busy word vectors
func runKMeansClusteringGo(tweetList []*tweets.Tweet, busyWords map[string]bool, cfg *Config, writeOutput func(string, ...interface{}), batchNumber int) []pipeline.TweetCluster {
	writeOutput("*** K-MEANS CLUSTERING (Go implementation) ***")
	if len(tweetList) == 0 || len(busyWords) == 0 {
		writeOutput("No tweets or busy words to cluster.")
		return nil
	}

	useAllWords := false
	if cfg.Analysis.KmeansUseAllWords {
		useAllWords = true
	}
	if useAllWords {
		writeOutput("[KMEANS] Using ALL words in tweet vectors.")
	} else {
		writeOutput("[KMEANS] Using only BUSY words in tweet vectors.")
	}

	// Build vocabulary: busy word -> index
	wordToIndex := make(map[string]int)
	idx := 0
	if useAllWords {
		// Collect all unique tokens from all tweets
		uniqueWords := make(map[string]bool)
		for _, tweet := range tweetList {
			for _, token := range tweet.Tokens {
				uniqueWords[token] = true
			}
		}
		for word := range uniqueWords {
			wordToIndex[word] = idx
			idx++
		}
	} else {
		for word := range busyWords {
			wordToIndex[word] = idx
			idx++
		}
	}

	// Build vectors for each tweet (binary vector)
	vectors := make([][]int, len(tweetList))
	for i, tweet := range tweetList {
		vec := make([]int, len(wordToIndex))
		for _, token := range tweet.Tokens {
			if j, ok := wordToIndex[token]; ok {
				vec[j] = 1
			}
		}
		vectors[i] = vec
	}

	k := cfg.Analysis.KmeansK
	if k <= 0 {
		k = 10 // fallback default
	}
	if k > len(tweetList) {
		k = len(tweetList)
	}

	clusterAssignments := kMeansClusteringGo(vectors, k)

	// Map cluster index to tweet indices
	clusterToTweets := make(map[int][]int)
	for i, c := range clusterAssignments {
		clusterToTweets[c] = append(clusterToTweets[c], i)
	}

	// Convert k-means results to TweetCluster format first
	var clusters []pipeline.TweetCluster
	for _, tweetIndices := range clusterToTweets {
		if len(tweetIndices) < cfg.Analysis.MinClusterSize {
			continue
		}

		// Create cluster tweets slice
		clusterTweets := make([]*tweets.Tweet, len(tweetIndices))
		for i, idx := range tweetIndices {
			clusterTweets[i] = tweetList[idx]
		}

		// Find shared busy words for this cluster
		sharedWords := make([]string, 0)
		for word := range busyWords {
			// Check if this word appears in at least half the tweets in the cluster
			count := 0
			for _, idx := range tweetIndices {
				for _, token := range tweetList[idx].Tokens {
					if token == word {
						count++
						break
					}
				}
			}
			if count >= len(tweetIndices)/2 {
				sharedWords = append(sharedWords, word)
			}
		}

		cluster := pipeline.TweetCluster{
			Tweets:    clusterTweets,
			BusyWords: sharedWords,
			Size:      len(clusterTweets),
		}
		clusters = append(clusters, cluster)
	}

	writeOutput("Clusters found: %d", len(clusters))
	writeOutput("")
	// For each cluster, print header, top busy words, and tweets
	for i, cluster := range clusters {
		busyWordsStr := ""
		if len(cluster.BusyWords) > 0 {
			busyWordsStr = fmt.Sprintf(" [%s]", strings.Join(cluster.BusyWords, ", "))
		}
		// Get the date/time of the first tweet in the cluster
		firstTweet := cluster.Tweets[0]
		timeStr := time.Unix(firstTweet.Unix, 0).Format("2006-01-02 15:04:05")

		// TODO: Continuation info will be handled by analysis thread
		continuationInfo := ""

		writeOutput("┌─ Cluster %d (%d tweets, first tweet: %s)%s%s", i+1, cluster.Size, timeStr, busyWordsStr, continuationInfo)

		maxTweetsToShow := 20
		for j, tweet := range cluster.Tweets {
			if j >= maxTweetsToShow {
				writeOutput("│  └─ ... and %d more tweets", len(cluster.Tweets)-maxTweetsToShow)
				break
			}
			prefix := "│  ├─"
			if j == maxTweetsToShow-1 || j == len(cluster.Tweets)-1 {
				prefix = "│  └─"
			}
			writeOutput("%s \"%s\"", prefix, tweet.Text)
		}
		writeOutput("│")
	}
	writeOutput("└─ End of clusters")

	return clusters
}

// kMeansClusteringGo clusters binary vectors using a simple k-means algorithm (Hamming distance)
func kMeansClusteringGo(vectors [][]int, k int) []int {
	if len(vectors) == 0 || k <= 0 {
		return nil
	}
	N := len(vectors)
	D := len(vectors[0])
	// Initialize centroids randomly
	centroids := make([][]float64, k)
	for i := 0; i < k; i++ {
		centroids[i] = make([]float64, D)
		idx := i % N
		for j := 0; j < D; j++ {
			centroids[i][j] = float64(vectors[idx][j])
		}
	}
	assignments := make([]int, N)
	changed := true
	maxIters := 100
	for iter := 0; iter < maxIters && changed; iter++ {
		changed = false
		// Assignment step
		for i, vec := range vectors {
			minDist := math.MaxFloat64
			best := 0
			for c, centroid := range centroids {
				dist := hammingDistanceFloat(vec, centroid)
				if dist < minDist {
					minDist = dist
					best = c
				}
			}
			if assignments[i] != best {
				assignments[i] = best
				changed = true
			}
		}
		// Update step
		counts := make([]int, k)
		newCentroids := make([][]float64, k)
		for c := 0; c < k; c++ {
			newCentroids[c] = make([]float64, D)
		}
		for i, vec := range vectors {
			c := assignments[i]
			counts[c]++
			for j, v := range vec {
				newCentroids[c][j] += float64(v)
			}
		}
		for c := 0; c < k; c++ {
			if counts[c] > 0 {
				for j := 0; j < D; j++ {
					newCentroids[c][j] /= float64(counts[c])
				}
			} else {
				// Reinitialize empty cluster to a random vector
				idx := c % N
				for j := 0; j < D; j++ {
					newCentroids[c][j] = float64(vectors[idx][j])
				}
			}
		}
		centroids = newCentroids
	}
	return assignments
}

// hammingDistanceFloat computes Hamming distance between int vector and float centroid
func hammingDistanceFloat(vec []int, centroid []float64) float64 {
	dist := 0.0
	for i, v := range vec {
		if (centroid[i] >= 0.5 && v == 0) || (centroid[i] < 0.5 && v == 1) {
			dist += 1.0
		}
	}
	return dist
}

// topBusyWordsFromCentroidGo returns the top N busy words from a centroid vector
func topBusyWordsFromCentroidGo(centroid []float64, wordToIndex map[string]int, n int) []string {
	type wordScore struct {
		word  string
		score float64
	}
	var scores []wordScore
	for word, idx := range wordToIndex {
		scores = append(scores, wordScore{word, centroid[idx]})
	}
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})
	result := make([]string, 0, n)
	for i := 0; i < n && i < len(scores); i++ {
		result = append(result, scores[i].word)
	}
	return result
}

// Helper: Compute Jaccard similarity between two string slices (tokens)
func jaccard(tokensA, tokensB []string) float64 {
	setA := make(map[string]struct{}, len(tokensA))
	setB := make(map[string]struct{}, len(tokensB))
	for _, t := range tokensA {
		setA[t] = struct{}{}
	}
	for _, t := range tokensB {
		setB[t] = struct{}{}
	}
	intersection := 0
	for t := range setA {
		if _, ok := setB[t]; ok {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0.0
	}
	return float64(intersection) / float64(union)
}

// Helper: Find most connected and medoid tweet indices in a cluster
func findMostTypicalTweets(tweets []*tweets.Tweet, threshold float64) (mostConnectedIdx, medoidIdx int, maxConnections int, maxSimSum float64) {
	n := len(tweets)
	connections := make([]int, n)
	simSums := make([]float64, n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if i == j {
				continue
			}
			sim := jaccard(tweets[i].Tokens, tweets[j].Tokens)
			simSums[i] += sim
			if sim >= threshold {
				connections[i]++
			}
		}
	}
	mostConnectedIdx, medoidIdx = 0, 0
	maxConnections, maxSimSum = connections[0], simSums[0]
	for i := 1; i < n; i++ {
		if connections[i] > maxConnections {
			mostConnectedIdx = i
			maxConnections = connections[i]
		}
		if simSums[i] > maxSimSum {
			medoidIdx = i
			maxSimSum = simSums[i]
		}
	}
	return
}

// Helper function
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Batch represents a collection of tweets processed together
type Batch struct {
	BatchID  int
	Tweets   []*tweets.Tweet
	Clusters []pipeline.TweetCluster
}

// OutputType represents the type of structured output
type OutputType string

const (
	OUTPUT_CLUSTER OutputType = "cluster"
	OUTPUT_STATS   OutputType = "stats"
	OUTPUT_ERROR   OutputType = "error"
	OUTPUT_INFO    OutputType = "info"
)

// OutputData represents structured output data
type OutputData struct {
	Type OutputType  `json:"type"`
	Data interface{} `json:"data"`
}

// Output functions for structured data (goes to stdout)
func OutputCluster(cluster interface{}) {
	// Get the global config to check output mode
	// Since we don't have direct access to config here, we'll use a global variable
	// or modify the function signature. For now, let's create a new function that takes config.
	OutputClusterWithConfig(cluster, nil) // Will be called with proper config from clustering functions
}

// OutputClusterWithConfig outputs cluster data based on the configured output mode
func OutputClusterWithConfig(cluster interface{}, cfg *Config) {
	// Default to verbose mode if no config provided
	outputMode := "verbose"
	if cfg != nil {
		outputMode = cfg.Analysis.OutputMode
	}

	// Process cluster data based on output mode
	var processedData interface{}

	if outputMode == "human" {
		// Convert to human-readable format
		processedData = convertToHumanReadable(cluster)
	} else {
		// Use original data for verbose mode
		processedData = cluster
	}

	data := OutputData{
		Type: OUTPUT_CLUSTER,
		Data: processedData,
	}
	jsonData, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(jsonData))
}

// convertToHumanReadable converts cluster data to human-readable format
func convertToHumanReadable(cluster interface{}) interface{} {
	// Type assert to get the cluster data
	clusterMap, ok := cluster.(map[string]interface{})
	if !ok {
		return cluster // Return original if not the expected format
	}

	// Check if this is a batch-level structure
	if _, hasBatchNumber := clusterMap["batch_number"]; hasBatchNumber {
		// This is a batch-level structure
		return convertBatchToHumanReadable(clusterMap)
	}

	// This is an individual cluster (legacy format)
	return convertIndividualClusterToHumanReadable(clusterMap)
}

// convertBatchToHumanReadable converts batch-level data to human-readable format
func convertBatchToHumanReadable(batchMap map[string]interface{}) interface{} {
	// Create a new map for human-readable output
	humanReadable := make(map[string]interface{})

	// Copy batch-level metadata
	for key, value := range batchMap {
		if key != "clusters" {
			humanReadable[key] = value
		}
	}

	// Convert clusters to human-readable format
	if clusters, ok := batchMap["clusters"].([]map[string]interface{}); ok {
		var humanReadableClusters []interface{}
		for _, cluster := range clusters {
			humanReadableClusters = append(humanReadableClusters, convertIndividualClusterToHumanReadable(cluster))
		}
		humanReadable["clusters"] = humanReadableClusters
	}

	return humanReadable
}

// convertIndividualClusterToHumanReadable converts individual cluster data to human-readable format
func convertIndividualClusterToHumanReadable(clusterMap map[string]interface{}) interface{} {
	// Create a new map for human-readable output
	humanReadable := make(map[string]interface{})

	// Copy all the metadata fields
	for key, value := range clusterMap {
		if key != "tweets" && key != "most_typical_tweet" {
			humanReadable[key] = value
		}
	}

	// Convert tweets to just their texts
	if tweets, ok := clusterMap["tweets"].([]*tweets.Tweet); ok {
		var tweetTexts []string
		for _, tweet := range tweets {
			tweetTexts = append(tweetTexts, tweet.Text)
		}
		humanReadable["tweet_texts"] = tweetTexts
	}

	// Add the most typical tweet text
	if mostTypicalTweet, ok := clusterMap["most_typical_tweet"].(*tweets.Tweet); ok && mostTypicalTweet != nil {
		humanReadable["medoid_tweet_text"] = mostTypicalTweet.Text
	}

	// Add persistence information if available
	if persistenceInfo, ok := clusterMap["persistence_info"].(string); ok {
		humanReadable["persistence_info"] = persistenceInfo
	}

	return humanReadable
}

func OutputStats(stats interface{}) {
	data := OutputData{
		Type: OUTPUT_STATS,
		Data: stats,
	}
	jsonData, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(jsonData))
}

func OutputError(err interface{}) {
	data := OutputData{
		Type: OUTPUT_ERROR,
		Data: err,
	}
	jsonData, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(jsonData))
}

func OutputInfo(info interface{}) {
	data := OutputData{
		Type: OUTPUT_INFO,
		Data: info,
	}
	jsonData, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(jsonData))
}

// Raw output function for backward compatibility (can be changed later)
func OutputRaw(format string, args ...interface{}) {
	fmt.Fprintf(os.Stdout, format+"\n", args...)
}
