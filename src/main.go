package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/streadway/amqp"

	"cursor-twitter/src/config"
	"cursor-twitter/src/filter"
	"cursor-twitter/src/logging"
	"cursor-twitter/src/output"
	"cursor-twitter/src/pipeline"
	"cursor-twitter/src/regex"
	"cursor-twitter/src/tweets"
	"cursor-twitter/src/utils"
)

// ========================================================================
// CRITICAL CONCURRENCY PROTECTION - DO NOT MODIFY WITHOUT EXPLICIT APPROVAL
// ========================================================================
// The concurrency protection in this codebase has been carefully tuned and
// tested. Adding unnecessary mutexes, channels, or other synchronization
// primitives can cause:
// 1. Deadlocks
// 2. Performance degradation
// 3. Race conditions
// 4. Complex debugging issues
//
// COMMON ANTI-PATTERNS TO AVOID:
// 1. Wrapping thread-safe data structures in mutexes (e.g., sync.Map with mutex)
// 2. Using mutexes where atomic operations would suffice (e.g., counters, flags)
// 3. Adding mutexes to already thread-safe channels
// 4. Double-wrapping with multiple layers of synchronization
// 5. Adding mutexes to individual fields instead of protecting the whole struct
// 6. Using RWMutex when a regular Mutex would work
//
// IMPORTANT RULES:
// 1. DO NOT add mutexes without explicit approval
// 2. DO NOT add channels without explicit approval
// 3. DO NOT add wait groups without explicit approval
// 4. DO NOT modify existing synchronization without explicit approval
// 5. The current design uses minimal, targeted synchronization
// 6. If you think you need more concurrency protection, ASK FIRST
// 7. Consider atomic operations before reaching for mutexes
// 8. Use the most specific synchronization primitive for the job
//
// This has been broken multiple times by adding unnecessary synchronization.
// ========================================================================

// Global batch window for persistence tracking
var (
	batchWindow      []*Batch
	batchWindowMutex sync.RWMutex
)

// addBatchToWindow adds a new batch to the global batch window and maintains the window size
func addBatchToWindow(batch *Batch, cfg *config.Config) {
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

// Global stats counters
var (
	TotalTweetsRead     int
	TotalTokensCounted  int
	globalBatchCount    int // Track batches sent for processing (incremented when signal 3PK sent)
	lastStatsTime       time.Time
	lastTweetCount      int
	pipelineStartTime   time.Time // Track when pipeline started for total rate calculation
	freqClasses         int       // Number of frequency classes from config
	analyticsBatchCount int       // Track which batch the analytics thread has completed

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
// Global regex patterns for tokenization - now in regex package

// init function to initialize regex variables for tests - now in regex package

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
var lastClusteringTime time.Time

// Analysis thread for processing busy word results and running clustering
func startAnalysisThread(cfg *config.Config, loadedState map[string]int) {
	go func() {
		resultCount := 0

		// ========================================================================
		// CRITICAL STATE LOADING LOGIC - DO NOT MODIFY WITHOUT EXPLICIT APPROVAL
		// ========================================================================
		// This section handles state loading and consistency checking for the FCT.
		// The main thread NEVER knows about this - it just continues reading tweets.
		//
		// IMPORTANT DESIGN PRINCIPLES:
		// 1. Main thread never waits for filters
		// 2. Main thread never coordinates with FCT
		// 3. Main thread never knows about state files
		// 4. FCT handles all state loading independently
		// 5. If state loading fails, fall back to building from scratch
		//
		// This logic has been broken multiple times by adding coordination between
		// main thread and FCT. DO NOT add waiting, timeouts, or coordination here.
		// ========================================================================

		// Handle loaded state if provided
		if loadedState != nil {
			totalTokens := 0
			for _, count := range loadedState {
				totalTokens += count
			}
			slog.Info("Informing FCT to load state", "total_tokens", totalTokens)

			// Check for state consistency - verify that we have the expected number of token files
			expectedTokenFiles := cfg.TokenPersistFiles
			if expectedTokenFiles > 0 {
				// Count how many token files actually exist in the state directory
				tokenFilePattern := filepath.Join(cfg.Persistence.StateDir, "token_*.json")
				tokenFiles, err := filepath.Glob(tokenFilePattern)
				if err == nil {
					actualTokenFiles := len(tokenFiles)
					if actualTokenFiles != expectedTokenFiles {
						fmt.Fprintf(os.Stderr, "⚠️  WARNING: State inconsistency detected - expected %d token files, found %d\n", expectedTokenFiles, actualTokenFiles)
						fmt.Fprintf(os.Stderr, "   Falling back to building filters from scratch...\n")
						slog.Info("State inconsistency detected - falling back to building from scratch")
						loadedState = nil // Treat as if no state was loaded
					} else {
						slog.Info("State consistency verified", "token_files", actualTokenFiles)
					}
				}
			}

			// Only proceed with state loading if state is consistent
			if loadedState != nil {
				// Make a copy of the state map to avoid concurrent modification issues
				stateCopy := make(map[string]int, len(loadedState))
				for k, v := range loadedState {
					stateCopy[k] = v
				}

				// Tell FCT to load its own state (using the copy)
				fct.LoadState(stateCopy)

				// Rebuild frequency class filters from the loaded token counts (using the original)
				slog.Info("Rebuilding frequency class filters from loaded token counts...")
				rebuildStartTime := time.Now()

				// Add panic recovery for corrupted state data
				func() {
					defer func() {
						if r := recover(); r != nil {
							fmt.Fprintf(os.Stderr, "⚠️  ERROR: Failed to rebuild frequency class filters from loaded state: %v\n", r)
							fmt.Fprintf(os.Stderr, "   The state files appear to be corrupted. Falling back to building from scratch.\n")
							// Don't set any filters - let the FCT build them from scratch
							slog.Info("State corruption detected - FCT will build filters from scratch")
						}
					}()

					var result pipeline.FreqClassResult
					if cfg.MinCountThreshold > 0 {
						result = pipeline.BuildFrequencyClassHashSetsAdaptive(loadedState, cfg.FreqClasses, cfg.MinCountThreshold)
					} else {
						result = pipeline.BuildFrequencyClassHashSets(loadedState, cfg.FreqClasses)
					}
					pipeline.SetGlobalFilters(result.Filters)
					rebuildDuration := time.Since(rebuildStartTime)
					slog.Info("Frequency class filters rebuilt", "classes", len(result.Filters), "duration", rebuildDuration)
				}()
			}
		} else {
			// If no state loaded, don't wait for filters - let FCT build them as tokens arrive
			slog.Info("No state loaded - FCT will build filters as tokens arrive")
		}

		// ========================================================================
		// END CRITICAL STATE LOADING LOGIC
		// ========================================================================

		// CRITICAL: Analytics thread gets input ONLY from the barrier (resultChannel)
		// NEVER accept input from any other source - this is a show-stopping error
		// The barrier is the ONLY mechanism that should provide input to analytics thread

		// Simple loop: collect batch from barrier, process it, repeat
		for {
			// Consume complete batch from barrier - this blocks until all BWPs submit
			results := freqClassProcessor.Barrier.ConsumeBatch()

			// Process the batch results
			currentBatch := make(map[int][]string) // class -> busy words
			var batchNumber int64

			for i, result := range results {
				resultCount++

				// Convert 3PKs to actual words
				busyWords := make([]string, 0, len(result.BusyWord3PKs))
				notFoundCount := 0
				for _, threePK := range result.BusyWord3PKs {
					if word, exists := pipeline.GetWordFrom3PK(threePK); exists {
						busyWords = append(busyWords, word)
					} else {
						notFoundCount++
					}
				}

				if notFoundCount > 0 {
					slog.Error("ERROR: 3PKs not found in mapping", "not_found", notFoundCount, "total", len(result.BusyWord3PKs), "class", result.FrequencyClass)
				}

				// Store results for this class
				currentBatch[result.FrequencyClass] = busyWords

				// Check batch number consistency
				if i > 0 && batchNumber != result.BatchNumber {
					slog.Error("BATCH NUMBER MISMATCH!", "expected", batchNumber, "got", result.BatchNumber, "class", result.FrequencyClass)
				}
				batchNumber = result.BatchNumber
			}

			// Process the batch
			recentTweets := globalTweetQueue.GetRecentTweets(cfg.BatchSize)

			runClusteringForBatch(currentBatch, recentTweets, batchNumber, cfg)

		}

		slog.Info("Analysis thread stopped", "results", resultCount)
	}()
}

// runClusteringForBatch runs clustering analysis for a batch of busy words and tweets
func runClusteringForBatch(classResults map[int][]string, recentTweets []*tweets.Tweet, batchNumber int64, cfg *config.Config) {
	fmt.Fprintf(os.Stderr, "DEBUG: runClusteringForBatch called for batch %d\n", batchNumber)
	slog.Info("Clustering: Starting batch analysis", "batch_number", batchNumber, "class_results", len(classResults), "recent_tweets", len(recentTweets))

	// Print busy word summary
	logging.PrintBatchSummary(classResults, batchNumber, cfg)

	// Collect all busy words from specified classes
	allBusyWords := make(map[string]bool)
	allowedClasses := make(map[int]bool)

	// Validate busyword_classes are within valid range
	for _, class := range cfg.BusywordClasses {
		if class < 1 || class > cfg.FreqClasses {
			slog.Warn("Invalid busyword_class", "class", class, "valid_range", fmt.Sprintf("1-%d", cfg.FreqClasses))
			continue
		}
		allowedClasses[class] = true
	}

	if len(allowedClasses) == 0 {
		slog.Error("No valid busyword_classes found", "valid_range", fmt.Sprintf("1-%d", cfg.FreqClasses))
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
	minBusyWords := 3  // Hardcoded for testing
	if minBusyWords <= 0 {
		minBusyWords = 1
	}
	fmt.Fprintf(os.Stderr, "DEBUG: Using minBusyWords = %d (from config: %d)\n", minBusyWords, cfg.Analysis.MinBusyWordsPerTweet)

	var tweetsWithBusyWords []*tweets.Tweet
	for _, tweet := range recentTweets {
		busyWordCount := 0
		var busyTokens []string
		for _, token := range tweet.Tokens {
			if allBusyWords[token] {
				busyWordCount++
				busyTokens = append(busyTokens, token)
			}
		}
		if busyWordCount >= minBusyWords {
			tweetsWithBusyWords = append(tweetsWithBusyWords, tweet)
		} else {
			// Debug: Show tweets that were filtered out
			if len(busyTokens) > 0 {
				fmt.Fprintf(os.Stderr, "DEBUG: Filtered out tweet with %d busy words: %v\n", busyWordCount, busyTokens)
			}
		}
	}

	// Debug: Show filtering results
	fmt.Fprintf(os.Stderr, "DEBUG: Tweet filtering results - total_tweets: %d, tweets_with_busy_words: %d, min_busy_words: %d, total_busy_words: %d\n",
		len(recentTweets), len(tweetsWithBusyWords), minBusyWords, len(allBusyWords))
	slog.Info("Tweet filtering results", "total_tweets", len(recentTweets), "tweets_with_busy_words", len(tweetsWithBusyWords), "min_busy_words", minBusyWords, "total_busy_words", len(allBusyWords))

	// Sanity checks before proceeding with clustering
	if len(tweetsWithBusyWords) == 0 {
		slog.Warn("Clustering skipped: no tweets with busy words", "batch", batchNumber, "total_tweets", len(recentTweets), "busy_words", len(allBusyWords))
		return
	}

	if len(tweetsWithBusyWords) < 2 {
		slog.Warn("Clustering skipped: not enough tweets with busy words", "batch", batchNumber, "tweets_with_busy_words", len(tweetsWithBusyWords), "min_required", 2)
		return
	}

	if len(allBusyWords) == 0 {
		slog.Warn("Clustering skipped: no busy words found", "batch", batchNumber)
		return
	}

	// Only graph clustering is supported now
	output.RunGraphClustering(tweetsWithBusyWords, allBusyWords, cfg, batchNumber, classResults)

	// Clustering cycle timing - diagnostic logging removed for cleaner output

	// Update last clustering time
	lastClusteringTime = time.Now()

}

// runGraphClustering is deprecated - using output.RunGraphClustering directly now
func runGraphClusteringDeprecated(tweetsWithBusyWords []*tweets.Tweet, allBusyWords map[string]bool, cfg *config.Config, batchNumber int64, classResults map[int][]string) {
	// Perform optimized graph clustering
	clusterer := pipeline.NewOptimizedTweetClusterer(
		cfg.Analysis.MinJaccardSimilarity,
		cfg.Analysis.MaxTweetsToCluster,
		cfg.Analysis.JaccardUseBusyWordsOnly,
	)

	result := clusterer.ClusterTweets(tweetsWithBusyWords, allBusyWords, cfg.Analysis.MinClusterSize)

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
	// var batchTimeStr string // unused
	// if len(tweetsWithBusyWords) > 0 {
	//	firstTweet := tweetsWithBusyWords[0]
	//	batchTimeStr = time.Unix(firstTweet.Unix, 0).Format("2006-01-02 15:04:05 UTC")
	// } else {
	//	batchTimeStr = time.Now().UTC().Format("2006-01-02 15:04:05 UTC")
	// }

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

		// Create frequency class mapping for busy words
		busyWordClasses := make(map[string]int)
		for word := range clusterBusyWords {
			// Find which frequency class this word belongs to
			for classIndex, words := range classResults {
				for _, classWord := range words {
					if classWord == word {
						busyWordClasses[word] = classIndex
						break
					}
				}
			}
		}

		// Find most typical tweet (medoid)
		_, medoidIdx, _, _ := findMostTypicalTweets(cluster.Tweets, cfg.Analysis.MinJaccardSimilarity)
		var mostTypicalTweet *tweets.Tweet
		if len(cluster.Tweets) > 1 {
			mostTypicalTweet = cluster.Tweets[medoidIdx]
		} else {
			mostTypicalTweet = cluster.Tweets[0]
		}

		// Get persistence information
		persistenceInfo := getContinuationInfo(cluster, currentBatchWindow, batchNumber, cfg)

		// Create cluster data for output
		// Find the earliest tweet chronologically (not just the first in DFS order)
		firstTweet := cluster.Tweets[0]
		earliestTime := firstTweet.Unix
		latestTime := firstTweet.Unix
		for _, tweet := range cluster.Tweets {
			if tweet.Unix < earliestTime {
				earliestTime = tweet.Unix
				firstTweet = tweet
			}
			if tweet.Unix > latestTime {
				latestTime = tweet.Unix
			}
		}
		timeStr := firstTweet.CreatedAt

		// Debug: Log the time span of this cluster
		timeSpan := latestTime - earliestTime
		if timeSpan > 300 { // More than 5 minutes
			slog.Warn("Large time span in cluster",
				"cluster_id", i+1,
				"time_span_seconds", timeSpan,
				"earliest", time.Unix(earliestTime, 0).Format("2006-01-02 15:04:05 UTC"),
				"latest", time.Unix(latestTime, 0).Format("2006-01-02 15:04:05 UTC"),
				"cluster_size", len(cluster.Tweets))
		}

		clusterData := map[string]interface{}{
			"cluster_id":         i + 1,
			"size":               len(cluster.Tweets),
			"tweets":             cluster.Tweets,
			"busy_words":         busyWordsList,
			"busy_word_classes":  busyWordClasses,
			"first_tweet_time":   timeStr,
			"most_typical_tweet": mostTypicalTweet,
			"persistence_info":   persistenceInfo,
		}
		batchClusters = append(batchClusters, clusterData)
	}

	// Check if we need to create a fallback cluster
	// First count clusters above minimum size
	clustersAboveMinSize := 0
	for _, cluster := range batchClusters {
		if size, ok := cluster["size"].(int); ok {
			if size >= cfg.Analysis.MinClusterSize {
				clustersAboveMinSize++
			}
		}
	}

	// Fallback cluster check - diagnostic logging removed for cleaner output

	if clustersAboveMinSize == 0 && len(tweetsWithBusyWords) > 0 && cfg.Analysis.CreateFallbackClusters {
		// Create a fallback cluster with all tweets
		// Ensure the fallback cluster meets minimum size requirement
		fallbackSize := len(tweetsWithBusyWords)
		if fallbackSize < cfg.Analysis.MinClusterSize {
			fallbackSize = cfg.Analysis.MinClusterSize // Force it to meet minimum
		}

		// Convert allBusyWords from map to sorted slice for consistency with normal clusters
		var fallbackBusyWords []string
		for word := range allBusyWords {
			fallbackBusyWords = append(fallbackBusyWords, word)
		}
		sort.Strings(fallbackBusyWords)

		fallbackCluster := map[string]interface{}{
			"type":             "fallback_cluster",
			"cluster_id":       0,
			"size":             fallbackSize,
			"medoid":           tweetsWithBusyWords[0].Text,
			"busy_words":       fallbackBusyWords,
			"tweet_texts":      make([]string, len(tweetsWithBusyWords)),
			"fallback_cluster": true,
			"clustering_note":  "No clusters found - created fallback cluster",
		}

		// Add all tweet texts
		for i, tweet := range tweetsWithBusyWords {
			fallbackCluster["tweet_texts"].([]string)[i] = tweet.Text
		}

		batchClusters = append(batchClusters, fallbackCluster)
		slog.Info("Fallback cluster created", "totalClusters", len(batchClusters))
	}

	// Create batch-level data structure
	// totalClusters := len(batchClusters) // unused

	// Recalculate clusters above min size after fallback cluster might have been added
	clustersAboveMinSize = 0
	for i, cluster := range batchClusters {
		if size, ok := cluster["size"].(int); ok {
			// Cluster size check - diagnostic logging removed for cleaner output
			if size >= cfg.Analysis.MinClusterSize {
				clustersAboveMinSize++
			}
		} else {
			slog.Warn("Cluster size not found or not int", "clusterIndex", i, "cluster", cluster)
		}
	}

	// ANALYTICS: About to output final batch data
	// Analytics about to output final batch data - diagnostic logging removed for cleaner output

	// Final batch data - diagnostic logging removed for cleaner output

	// This function is deprecated - using output.RunGraphClustering directly now
}

// getCurrentWorkingDir returns the current working directory for debugging

// Helper: Initialize logger

// printStats prints the current pipeline statistics and logs them as CSV.
func printStats() {
	// Use the logging package wrapper function
	lastStatsTime, lastTweetCount = logging.PrintStatsWrapper(
		fct,
		inboundTokenQueue,
		freqClassProcessor,
		TotalTweetsRead,
		TotalTokensCounted,
		lastStatsTime,
		pipelineStartTime,
		lastTweetCount,
		freqClasses,
		statsCSVPath,
	)
}

// Helper: Initialize stats CSV
// Helper: Initialize word filter
func initializeWordFilter(cfg *config.Config) (*filter.WordFilter, error) {
	if cfg.Filter.Enabled {
		slog.Info("Initializing word filter...")
		globalWordFilter := filter.NewWordFilter()

		// Try directory first (new approach)
		if cfg.Filter.FilterDir != "" {
			slog.Info("Using filter directory", "dir", cfg.Filter.FilterDir)
			if err := globalWordFilter.LoadFromDirectory(cfg.Filter.FilterDir); err != nil {
				return nil, err
			}
		} else if cfg.Filter.FilterFile != "" {
			// Fall back to single file (backward compatibility)
			slog.Info("Using filter file", "file", cfg.Filter.FilterFile)
			if err := globalWordFilter.LoadFromFile(cfg.Filter.FilterFile); err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("neither filter_dir nor filter_file specified in config")
		}
		return globalWordFilter, nil
	}
	slog.Info("Word filter disabled in config")
	return nil, nil
}

// Helper: Setup RabbitMQ
func setupRabbitMQ(cfg *config.Config) (*amqp.Connection, *amqp.Channel, amqp.Queue, error) {
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
func initializePipeline(cfg *config.Config) error {
	pipeline.SetGlobalArrayLen(cfg.BWArrayLen)

	inboundTokenQueue = pipeline.NewTokenQueue()

	freqClasses = cfg.FreqClasses
	if freqClasses <= 0 {
		return fmt.Errorf("freq_classes must be > 0 in config, got %d", freqClasses)
	}

	// Set the global frequency classes count in the pipeline package
	// This allows WhatClass() function to use the correct value instead of hardcoded 24
	pipeline.SetGlobalFreqClasses(freqClasses)

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
		cfg.WindowBatches,
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

// meaningless comment

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

// parseCommandLineFlags parses command line arguments and returns the flag values
func parseCommandLineFlags() (*bool, *string, *string, *bool, *bool) {
	// Add a command line flag to control printing of tweets
	printTweets := flag.Bool("print-tweets", true, "Print each parsed tweet to the console")
	configPath := flag.String("config", "config/config.yaml", "Path to YAML config file")
	overridePath := flag.String("override", "", "Path to YAML override config file (optional)")
	loadState := flag.Bool("load-state", false, "Load persisted state from files on startup")
	enableProfiling := flag.Bool("profile", false, "Enable CPU profiling (creates cpu.prof)")
	flag.Parse()

	return printTweets, configPath, overridePath, loadState, enableProfiling
}

// setupCPUProfiling sets up CPU profiling if enabled
func setupCPUProfiling(enableProfiling bool) {
	if enableProfiling {
		cpuProfile, err := os.Create("cpu.prof")
		if err != nil {
			log.Fatalf("Failed to create CPU profile: %v", err)
		}
		defer cpuProfile.Close()
		if err := pprof.StartCPUProfile(cpuProfile); err != nil {
			log.Fatalf("Failed to start CPU profile: %v", err)
		}
		defer pprof.StopCPUProfile()
		slog.Info("CPU profiling enabled - will create cpu.prof file")
	}
}

// loadConfiguration loads the configuration from YAML file with optional override
func loadConfiguration(configPath, overridePath string) *config.Config {
	var cfg *config.Config
	var err error
	if overridePath != "" {
		cfg, err = config.LoadConfigWithOverride(configPath, overridePath)
	} else {
		cfg, err = config.LoadConfig(configPath)
	}
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Set default values for new configuration options
	// CreateFallbackClusters defaults to true for safety (avoid losing data)
	// Users can set it to false in their config if they want to drop batches with no clusters
	// Note: Removed override - respect user's explicit config setting

	// Logging is now handled entirely by slog

	slog.Info("*** CONFIG LOADED SUCCESSFULLY ***")
	return cfg
}

// printStartupInfo prints key configuration values to stderr for user visibility
func printStartupInfo(cfg *config.Config, configPath string, loadState, printTweets bool) {
	fmt.Fprintf(os.Stderr, "\n=== TWITTER SUBJECT DETECTION PIPELINE STARTUP ===\n")
	fmt.Fprintf(os.Stderr, "Config file: %s\n", configPath)
	fmt.Fprintf(os.Stderr, "Log level: %s\n", cfg.LogLevel)
	fmt.Fprintf(os.Stderr, "\n--- Core Pipeline Settings ---\n")
	fmt.Fprintf(os.Stderr, "Frequency classes: %d\n", cfg.FreqClasses)
	fmt.Fprintf(os.Stderr, "Batch size: %d tweets\n", cfg.BatchSize)
	fmt.Fprintf(os.Stderr, "Window batches: %d\n", cfg.WindowBatches)
	fmt.Fprintf(os.Stderr, "Window size: %d\n", cfg.WindowSize)
	fmt.Fprintf(os.Stderr, "BW array length: %d\n", cfg.BWArrayLen)
	fmt.Fprintf(os.Stderr, "Min token length: %d\n", cfg.MinTokenLen)
	fmt.Fprintf(os.Stderr, "Z-scores: %v\n", cfg.ZScores)
	fmt.Fprintf(os.Stderr, "Skip frequency classes: %v\n", cfg.SkipFrequencyClasses)
	fmt.Fprintf(os.Stderr, "Busyword classes: %v\n", cfg.BusywordClasses)
	fmt.Fprintf(os.Stderr, "\n--- Persistence Settings ---\n")
	fmt.Fprintf(os.Stderr, "Token persist files: %d\n", cfg.TokenPersistFiles)
	fmt.Fprintf(os.Stderr, "Rebuild every files: %d\n", cfg.RebuildEveryFiles)
	fmt.Fprintf(os.Stderr, "Window batches persistence: %d\n", cfg.Analysis.WindowBatchesPersistence)
	fmt.Fprintf(os.Stderr, "Window batches persistence check: %d\n", cfg.Analysis.WindowBatchesPersistenceCheck)
	fmt.Fprintf(os.Stderr, "Min shared busywords for persistence: %d\n", cfg.Analysis.MinSharedBusyWordsForPersistence)
	fmt.Fprintf(os.Stderr, "\n--- Clustering Settings ---\n")
	fmt.Fprintf(os.Stderr, "Clustering method: %s\n", cfg.Analysis.ClusteringMethod)
	fmt.Fprintf(os.Stderr, "Output mode: human\n")
	fmt.Fprintf(os.Stderr, "Min busy words per tweet: %d\n", cfg.Analysis.MinBusyWordsPerTweet)
	fmt.Fprintf(os.Stderr, "Min Jaccard similarity: %.3f\n", cfg.Analysis.MinJaccardSimilarity)
	fmt.Fprintf(os.Stderr, "Duplicate similarity threshold: %.3f\n", cfg.Analysis.DuplicateSimilarityThreshold)
	fmt.Fprintf(os.Stderr, "Min cluster size: %d\n", cfg.Analysis.MinClusterSize)
	fmt.Fprintf(os.Stderr, "Language filter: %s\n", cfg.Analysis.LanguageFilter)
	fmt.Fprintf(os.Stderr, "Cluster sort descending: %v\n", cfg.Analysis.ClusterSortDescending)
	fmt.Fprintf(os.Stderr, "Suppress individual tweets: %v\n", cfg.Analysis.SuppressIndividualTweets)
	fmt.Fprintf(os.Stderr, "Enable meta-clustering: %v\n", cfg.Analysis.EnableMetaClustering)
	if cfg.Analysis.EnableMetaClustering {
		fmt.Fprintf(os.Stderr, "Meta-cluster similarity threshold: %.3f\n", cfg.Analysis.MetaClusterSimilarityThreshold)
		fmt.Fprintf(os.Stderr, "Meta-cluster min size: %d\n", cfg.Analysis.MetaClusterMinSize)
	}
	fmt.Fprintf(os.Stderr, "\n--- Filter Settings ---\n")
	fmt.Fprintf(os.Stderr, "Filter enabled: %v\n", cfg.Filter.Enabled)
	fmt.Fprintf(os.Stderr, "RabbitMQ: %s:%d/%s\n", cfg.MQHost, cfg.MQPort, cfg.MQQueue)
	fmt.Fprintf(os.Stderr, "Load state: %v\n", loadState)
	fmt.Fprintf(os.Stderr, "Print tweets: %v\n", printTweets)
	fmt.Fprintf(os.Stderr, "\n--- Additional Settings ---\n")
	fmt.Fprintf(os.Stderr, "Token filtering parameters available in config.yaml (token_filters section)\n")
}

// logStartupInfo logs the same configuration information to the log file in structured format
func logStartupInfo(cfg *config.Config, configPath string, loadState, printTweets bool) {
	slog.Info("=== TWITTER SUBJECT DETECTION PIPELINE STARTUP ===")
	slog.Info("Config file", "path", configPath)
	slog.Info("Log level", "level", cfg.LogLevel)
	slog.Info("--- Core Pipeline Settings ---")
	slog.Info("Frequency classes", "count", cfg.FreqClasses)
	slog.Info("Batch size", "tweets", cfg.BatchSize)
	slog.Info("Window batches", "count", cfg.WindowBatches)
	slog.Info("Window size", "size", cfg.WindowSize)
	slog.Info("BW array length", "length", cfg.BWArrayLen)
	slog.Info("Min token length", "length", cfg.MinTokenLen)
	slog.Info("Z-scores", "scores", cfg.ZScores)
	slog.Info("Skip frequency classes", "classes", cfg.SkipFrequencyClasses)
	slog.Info("Busyword classes", "classes", cfg.BusywordClasses)
	slog.Info("--- Persistence Settings ---")
	slog.Info("Token persist files", "count", cfg.TokenPersistFiles)
	slog.Info("Rebuild every files", "count", cfg.RebuildEveryFiles)
	slog.Info("Window batches persistence", "count", cfg.Analysis.WindowBatchesPersistence)
	slog.Info("Window batches persistence check", "count", cfg.Analysis.WindowBatchesPersistenceCheck)
	slog.Info("Min shared busywords for persistence", "count", cfg.Analysis.MinSharedBusyWordsForPersistence)
	slog.Info("--- Clustering Settings ---")
	slog.Info("Clustering method", "method", cfg.Analysis.ClusteringMethod)
	slog.Info("Output mode", "mode", "human")
	slog.Info("Min busy words per tweet", "count", cfg.Analysis.MinBusyWordsPerTweet)
	slog.Info("Min Jaccard similarity", "similarity", cfg.Analysis.MinJaccardSimilarity)
	slog.Info("Duplicate similarity threshold", "threshold", cfg.Analysis.DuplicateSimilarityThreshold)
	slog.Info("Min cluster size", "size", cfg.Analysis.MinClusterSize)
	slog.Info("Language filter", "filter", cfg.Analysis.LanguageFilter)
	slog.Info("Cluster sort descending", "descending", cfg.Analysis.ClusterSortDescending)
	slog.Info("--- Filter Settings ---")
	slog.Info("Filter enabled", "enabled", cfg.Filter.Enabled)
	if cfg.Analysis.FilterRepetitivePatterns {
		if cfg.Analysis.BannedPhrasesDir != "" {
			slog.Info("Banned phrases directory", "dir", cfg.Analysis.BannedPhrasesDir)
		} else if cfg.Analysis.BannedPhrasesFile != "" {
			slog.Info("Banned phrases file", "file", cfg.Analysis.BannedPhrasesFile)
		}
		slog.Info("Repetitive pattern threshold", "threshold", cfg.Analysis.RepetitivePatternThreshold)
	}
	slog.Info("RabbitMQ", "host", cfg.MQHost, "port", cfg.MQPort, "queue", cfg.MQQueue)
	slog.Info("Load state", "enabled", loadState)
	slog.Info("Print tweets", "enabled", printTweets)
	slog.Info("--- Additional Settings ---")
	slog.Info("Token filtering parameters available in config.yaml (token_filters section)")
}

// printStartupProgress prints startup progress information to stderr
func printStartupProgress() {
	fmt.Fprintf(os.Stderr, "\n=== STARTUP PROGRESS ===\n")
	fmt.Fprintf(os.Stderr, "Building frequency class filters... (this may take a few minutes)\n")
	fmt.Fprintf(os.Stderr, "Progress: ")
}

func main() {
	// Parse command line flags
	printTweets, configPath, overridePath, loadState, enableProfiling := parseCommandLineFlags()

	// Start CPU profiling if enabled
	setupCPUProfiling(*enableProfiling)

	// Load configuration
	cfg := loadConfiguration(*configPath, *overridePath)

	// Print startup information
	printStartupInfo(cfg, *configPath, *loadState, *printTweets)
	// Initialize logger first, before any slog calls
	logger, logFile, err := logging.InitializeLogger(cfg)
	if err != nil {
		log.Fatalf("Failed to set up logger: %v", err)
	}
	defer logFile.Close()
	slog.SetDefault(logger)

	// Print startup progress
	printStartupProgress()

	// Log startup information to structured log
	logStartupInfo(cfg, *configPath, *loadState, *printTweets)

	// Log z-score array
	slog.Info("Z-scores per frequency class", "scores", cfg.ZScores)

	// Log startup information
	slog.Info("Application started",
		"config_path", *configPath,
		"print_tweets", *printTweets,
		"load_state", *loadState)

	statsCSVPath = logging.InitializeStatsCSV(cfg.LogDir)

	globalWordFilter, err = initializeWordFilter(cfg)
	if err != nil {
		log.Fatalf("Failed to load word filter: %v", err)
	}
	if globalWordFilter != nil {
		slog.Info("Word filter initialized", "words", globalWordFilter.GetFilteredCount())
	} else {
		slog.Info("Word filter is nil (disabled)")
	}

	// Initialize pre-compiled regexes for tokenization - now in regex package

	// TODO: Analysis thread will handle tweet window management

	initializeGlobalState()

	logging.StartStatsPrinter(printStats)

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

	// Initialize last clustering time
	lastClusteringTime = time.Now()

	// Start the analysis thread to process busy word results and run clustering
	startAnalysisThread(cfg, loadedState)

	timestamp := time.Now().Format("20060102_150405")
	clusterFileName := fmt.Sprintf("clusters_%s.txt", timestamp)
	clusterOutputFilePath = filepath.Join(cfg.LogDir, clusterFileName)
	slog.Info("Cluster output will be saved to", "file", clusterOutputFilePath)

	fmt.Fprintf(os.Stderr, "✓ Pipeline ready!\n")
	fmt.Fprintf(os.Stderr, "✓ Clustering output will be printed to stdout\n")
	fmt.Fprintf(os.Stderr, "✓ Logs saved to: %s\n", cfg.LogDir)
	fmt.Fprintf(os.Stderr, "=== STARTUP COMPLETE ===\n\n")

	// Also log the completion messages to the log file
	slog.Info("✓ Pipeline ready!")
	slog.Info("✓ Clustering output will be printed to stdout")
	slog.Info("✓ Logs saved to", "dir", cfg.LogDir)
	slog.Info("=== STARTUP COMPLETE ===")

	setupSignalHandling()

	// Process based on input mode
	slog.Info("DEBUG: Config mode", "mode", cfg.Mode)

	switch cfg.Mode {
	case "mqj":
		processTweets(cfg, *printTweets)
	case "files":
		processTweets(cfg, *printTweets)
	default:
		slog.Error("Invalid mode specified", "mode", cfg.Mode, "valid_modes", []string{"mqj", "files"})
		os.Exit(1)
	}
}

// processTweets() is the unified main processing pipeline
// It processes tweets from any source via getNextTweet() abstraction
func processTweets(cfg *config.Config, printTweets bool) {
	for {
		// Get next tweet using the abstraction
		row, err := getNextTweet(cfg)
		if err != nil {
			if err == io.EOF {
				slog.Info("Reached end of input")
				break
			}
			slog.Warn("Failed to read tweet", "error", err)
			continue
		}

		// Apply language filter if configured
		if cfg.Analysis.LanguageFilter != "" && cfg.Analysis.LanguageFilter != "all" {
			langSuffix := "," + strings.ToLower(cfg.Analysis.LanguageFilter)
			if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(row)), langSuffix) {
				continue
			}
		}

		// Parse tweet using existing function
		tweet, err := parseCSVToTweet(row, cfg)
		if err != nil {
			slog.Warn("Failed to parse tweet", "error", err, "row", row)
			continue
		}
		if tweet == nil {
			// Tweet was filtered out (e.g., by language)
			continue
		}

		// Increment counters after successful parsing
		TotalTweetsRead++
		TotalTokensCounted += len(tweet.Tokens)

		if printTweets {
			slog.Debug("Parsed Tweet", "tweet", tweet)
		}

		// Add tweet to global queue for clustering
		globalTweetQueue.Enqueue(tweet)

		// ========================================================================
		// CRITICAL TOKEN PROCESSING LOGIC
		// ========================================================================
		// This section processes tokens and enqueues them to frequency class processors.
		// It is critical that this logic remains unchanged and is not modified
		// without careful consideration of the following constraints:
		//
		// 1. Token processing must happen for every tweet regardless of filter availability
		// 2. Frequency class determination must use the current active filters
		// 3. Tokens must be enqueued to the appropriate frequency class processors
		// 4. The main thread must never wait for FCT (Frequency Class Thread) operations
		// 5. Main thread never waits or coordinates with FCT
		//
		// This logic has been broken multiple times by adding filter availability
		// checks before token processing. DO NOT add waiting or coordination here.
		// ========================================================================

		// Always add new tweet tokens to the inbound queue for FCT to build frequency filters
		if len(tweet.Tokens) > 0 {
			inboundTokenQueue.Enqueue(tweet.Tokens)

			// Always populate 3PK mappings for all tokens (regardless of filter availability)
			for _, token := range tweet.Tokens {
				// Get token info (3PK and frequency class) in a single operation
				threePK, _, exists := pipeline.GetTokenInfo(token)
				if !exists {
					// Token not found in current filters - skip it
					continue
				}

				// Get frequency class for this token
				freqClass := pipeline.WhatClass(token)

				// Only enqueue to frequency class processors if the class is active
				if !freqClassProcessor.IsClassActive(freqClass) {
					// Skip this frequency class - don't enqueue tokens
					continue
				}
				freqClassProcessor.EnqueueToFrequencyClass(freqClass, threePK)
			}
		}

		// ========================================================================
		// END CRITICAL TOKEN PROCESSING LOGIC
		// ========================================================================

		// Send termination signals to busy word processors every batch number of tweets
		// Only send if frequency class filters are available
		if TotalTweetsRead%cfg.BatchSize == 0 && TotalTweetsRead > 0 {
			{
				// Increment batch number FIRST, then create termination signal
				freqClassProcessor.IncrementBatchNumber()
				newBatchNum := freqClassProcessor.GetBatchNumber()

				// Create termination signal with the incremented batch number
				terminationSignal := tweets.ThreePartKey{
					Part1:   -1,
					Part2:   -1,
					Part3:   -1,
					BatchID: newBatchNum,
				}

				// Send termination signal to active frequency class processors only
				activeCount := 0
				for i := 0; i < freqClasses; i++ {
					if freqClassProcessor.IsClassActive(i) {
						freqClassProcessor.EnqueueToFrequencyClass(i, terminationSignal)
						activeCount++
					}
				}

				// Increment global batch count when signal 3PK is sent (this represents actual batches sent for processing)
				globalBatchCount++

			}
		}

		// Cleanup trigger - remove obsolete tokens from 3PK mappings
		if cfg.Analysis.CleanupTriggerBatchSize > 0 && TotalTweetsRead%cfg.Analysis.CleanupTriggerBatchSize == 0 && TotalTweetsRead > 0 {
			// Get queue size before processing
			queueSize := inboundTokenQueue.Len()
			if queueSize > 0 {
				// Calculate dynamic threshold based on actual token obsolescence rate
				// Leave at least window_batches worth of obsolete tokens on the queue
				threshold := queueSize - (cfg.WindowBatches * 1000) // Conservative estimate
				if threshold < 0 {
					threshold = 0
				}

				// Process cleanup with dynamic threshold
				cleanupCount := pipeline.ProcessCleanupQueue(threshold)
				if cleanupCount > 0 {
					slog.Debug("Cleaned up obsolete tokens", "count", cleanupCount, "queue_size", queueSize, "threshold", threshold)
				}
			}
		}

		// Throttling logic - sleep if busy word processor queues are getting too long
		if cfg.Analysis.BWQueueMax > 0 {
			maxAllowedQueueLength := int(float64(cfg.BatchSize) * cfg.Analysis.BWQueueMax)
			queueStats := freqClassProcessor.GetQueueStats()
			for i := 0; i < freqClasses; i++ {
				if freqClassProcessor.IsClassActive(i) {
					queueKey := fmt.Sprintf("freq_class_%d_queue_size", i)
					queueLength := queueStats[queueKey]
					if queueLength > maxAllowedQueueLength {
						// Calculate sleep time distributed across all active processors
						sleepMs := cfg.Analysis.BWThreadSlowDelay / freqClasses
						if sleepMs < 1 {
							sleepMs = 1
						}
						slog.Debug("Busy word processor queue too long, sleeping",
							"class", i,
							"queue_length", queueLength,
							"max_allowed", maxAllowedQueueLength,
							"sleep_ms", sleepMs)
						time.Sleep(time.Duration(sleepMs) * time.Millisecond)
					}
				}
			}
		}
	}
}

// createBatchFromClusters creates a batch from clustering results and adds it to the batch window
func createBatchFromClusters(clusters []pipeline.TweetCluster, batchNumber int, cfg *config.Config) {
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
func getContinuationInfo(currentCluster pipeline.TweetCluster, batchWindow []*Batch, currentBatchID int64, cfg *config.Config) string {
	var continuationBatches []int64

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
func clustersAreRelated(cluster1, cluster2 pipeline.TweetCluster, cfg *config.Config) bool {
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

func clustersAreRelatedByBusyWords(cluster1, cluster2 pipeline.TweetCluster, cfg *config.Config) bool {
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

func clustersAreRelatedByFullText(cluster1, cluster2 pipeline.TweetCluster, cfg *config.Config) bool {
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
func deduplicateAndSort(slice []int64) []int64 {
	seen := make(map[int64]bool)
	var result []int64

	for _, item := range slice {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i] < result[j]
	})
	return result
}

// processBatchPersistence analyzes the batch window for persistent clusters
func processBatchPersistence(batchWindow []*Batch, cfg *config.Config) {
	// TODO: This function should be moved to the analysis thread
	// The analysis thread should handle persistence tracking
	return

}

// parseCSVToTweet parses a CSV row string into a Tweet struct,
// tokenizes the text, generates ThreePartKeys, and updates the global
// token counter.
func parseCSVToTweet(row string, cfg *config.Config) (*tweets.Tweet, error) {
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
		CreatedAt:    createdAt.Format("2006-01-02 15:04:05 UTC"),
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

	// Filter out tweets with excessive question marks (similar to Python parser)
	if cfg.Analysis.DropExcessiveQuestions {
		questionCount := strings.Count(tweet.Text, "?")
		if questionCount >= 10 && float64(questionCount)/float64(len(tweet.Text)) > 0.2 {
			return nil, nil // Skip this tweet
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
		threePK := pipeline.GenerateThreePartKey(token, int64(globalBatchCount))
		threePKs = append(threePKs, threePK)
	}
	// Note: ThreePKs not stored in Tweet struct but still generated for other uses

	// Step 3: Update global stats counters (token counting is now handled by FCT)
	// Note: TotalTweetsRead and TotalTokensCounted are incremented by the calling function, not here

	return tweet, nil
}

// simpleTokenize splits text into tokens for this project.
// - Converts to lowercase
// - Splits on non-word characters (including periods, commas, etc.)
// - Removes apostrophes and what follows
// - Filters out offensive words if word filtering is enabled
// - Filters out tokens shorter than min_token_len if specified
func simpleTokenize(text string, cfg *config.Config) []string {
	// Use regex to split on non-word characters (including periods, commas, etc.)
	// This matches the approach used in analyze_tokens.go
	tokens := regex.TokenizeRegex.Split(strings.ToLower(text), -1)

	// Process each token individually
	var processedTokens []string

	// totalProcessed = len(tokens) // TODO: Statistics tracking moved to analysis thread
	for _, token := range tokens {
		// Skip empty tokens
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

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
				token = regex.ApostropheRegex.ReplaceAllString(token, "")
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
func shouldFilterToken(token string, cfg *config.Config) bool {
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

	// At-mention filter
	if cfg.TokenFilters.RejectAtMentions && strings.HasPrefix(token, "@") {
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
func loadPersistedState(stateDir string, freqClasses int, cfg *config.Config) map[string]int {
	slog.Info("=== LOADING PERSISTED STATE ===")

	// Check if any of the files exist
	tokenCounterPath := filepath.Join(stateDir, "token_counter.json")
	freqClassPath := filepath.Join(stateDir, "frequency_classes.json")

	// If none of the files exist, just return and let the normal program run
	_, err1 := os.Stat(tokenCounterPath)
	_, err2 := os.Stat(freqClassPath)
	if os.IsNotExist(err1) && os.IsNotExist(err2) {
		slog.Info("No persisted state files found. Starting fresh.")
		slog.Info("=== PERSISTED STATE LOADING COMPLETE ===")
		return nil
	}

	// Load TokenCounter if it exists
	tempTokenCounter := pipeline.NewTokenCounter()
	if err := tempTokenCounter.LoadFromFile(tokenCounterPath); err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			slog.Info("TokenCounter file not found", "file", tokenCounterPath)
		} else {
			slog.Info("Failed to load TokenCounter", "error", err)
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
	slog.Info("TokenCounter loaded", "total_tokens", totalTokens, "distinct_tokens", len(counts), "duration", loadDuration)

	// Load FrequencyClassResult if it exists
	var tempFreqClassResult pipeline.FreqClassResult
	if err := tempFreqClassResult.LoadFromFile(freqClassPath); err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			slog.Info("FrequencyClassResult file not found", "file", freqClassPath)
		} else {
			slog.Info("Failed to load FrequencyClassResult", "error", err)
		}
	} else {
		classes := len(tempFreqClassResult.Filters)
		slog.Info("FrequencyClassResult loaded", "classes", classes)
	}

	slog.Info("=== PERSISTED STATE LOADING COMPLETE ===")
	return counts
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

	// Calculate temporal weights based on tweet timestamps
	temporalWeights := make([]float64, n)
	if n > 1 {
		// Find the median timestamp to use as reference
		timestamps := make([]int64, n)
		for i, tweet := range tweets {
			timestamps[i] = tweet.Unix
		}

		// Sort timestamps to find median
		sort.Slice(timestamps, func(i, j int) bool {
			return timestamps[i] < timestamps[j]
		})
		medianTimestamp := timestamps[n/2]

		// Calculate temporal weights (closer to median = higher weight)
		maxTimeDiff := int64(300) // 5 minutes in seconds
		for i, tweet := range tweets {
			timeDiff := utils.Abs(tweet.Unix - medianTimestamp)
			if timeDiff > maxTimeDiff {
				temporalWeights[i] = 0.1 // Very low weight for tweets far from median time
			} else {
				// Weight decreases linearly with time difference
				temporalWeights[i] = 1.0 - (float64(timeDiff)/float64(maxTimeDiff))*0.5
			}
		}
	} else {
		// Single tweet gets full weight
		temporalWeights[0] = 1.0
	}

	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if i == j {
				continue
			}
			sim := jaccard(tweets[i].Tokens, tweets[j].Tokens)
			// Apply temporal weight to similarity score
			weightedSim := sim * temporalWeights[i]
			simSums[i] += weightedSim
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

// removeNearDuplicates removes near-duplicate tweets using configurable distance method
// This catches spam variations like "same tweet with different URL/number at end"
// Uses the medoid (most typical tweet) as the reference point for comparison
// Returns the deduplicated tweets and the count of removed tweets
func removeNearDuplicates(tweetList []*tweets.Tweet, threshold float64, distanceMethod string) ([]*tweets.Tweet, int) {
	if len(tweetList) <= 1 {
		return tweetList, 0
	}

	// Find the medoid (most typical tweet) as the reference point
	_, medoidIdx, _, _ := findMostTypicalTweets(tweetList, 0.4)
	medoidTweet := tweetList[medoidIdx]

	var deduplicatedTweets []*tweets.Tweet
	removedCount := 0

	// Keep the medoid tweet
	deduplicatedTweets = append(deduplicatedTweets, medoidTweet)

	// Check each other tweet against the medoid
	for i, tweet := range tweetList {
		if i == medoidIdx {
			continue // Skip the medoid itself
		}

		// Calculate normalized distance based on method
		var normalizedDistance float64
		if distanceMethod == "word" {
			normalizedDistance = normalizedWordDistance(medoidTweet.Text, tweet.Text)
		} else {
			// Default to character-based Levenshtein distance
			distance := levenshteinDistance(medoidTweet.Text, tweet.Text)
			normalizedDistance = float64(distance) / float64(max(len(medoidTweet.Text), len(tweet.Text)))
		}

		// Keep tweet if it's different enough
		if normalizedDistance > threshold {
			deduplicatedTweets = append(deduplicatedTweets, tweet)
		} else {
			removedCount++
		}
	}

	return deduplicatedTweets, removedCount
}

// levenshteinDistance computes the Levenshtein distance between two strings
func levenshteinDistance(s1, s2 string) int {
	len1, len2 := len(s1), len(s2)

	// Create a 2D slice for the dynamic programming table
	dp := make([][]int, len1+1)
	for i := range dp {
		dp[i] = make([]int, len2+1)
	}

	// Initialize the first row and column
	for i := 0; i <= len1; i++ {
		dp[i][0] = i
	}
	for j := 0; j <= len2; j++ {
		dp[0][j] = j
	}

	// Fill the DP table
	for i := 1; i <= len1; i++ {
		for j := 1; j <= len2; j++ {
			if s1[i-1] == s2[j-1] {
				dp[i][j] = dp[i-1][j-1]
			} else {
				dp[i][j] = 1 + utils.Min(dp[i-1][j], utils.Min(dp[i][j-1], dp[i-1][j-1]))
			}
		}
	}

	return dp[len1][len2]
}

// wordDistance computes the minimum number of word edits to transform one tweet into another
// Uses the same dynamic programming approach as Levenshtein but operates on words instead of characters
func wordDistance(tweet1, tweet2 string) int {
	// Split tweets into words
	words1 := strings.Fields(strings.ToLower(tweet1))
	words2 := strings.Fields(strings.ToLower(tweet2))

	len1, len2 := len(words1), len(words2)

	// Create a 2D slice for the dynamic programming table
	dp := make([][]int, len1+1)
	for i := range dp {
		dp[i] = make([]int, len2+1)
	}

	// Initialize the first row and column
	for i := 0; i <= len1; i++ {
		dp[i][0] = i // Delete i words
	}
	for j := 0; j <= len2; j++ {
		dp[0][j] = j // Insert j words
	}

	// Fill the DP table
	for i := 1; i <= len1; i++ {
		for j := 1; j <= len2; j++ {
			if words1[i-1] == words2[j-1] {
				// Words match, no edit needed
				dp[i][j] = dp[i-1][j-1]
			} else {
				// Take minimum of: delete, insert, or substitute
				dp[i][j] = 1 + utils.Min(dp[i-1][j], utils.Min(dp[i][j-1], dp[i-1][j-1]))
			}
		}
	}

	return dp[len1][len2]
}

// normalizedWordDistance returns the word distance normalized by the maximum number of words
func normalizedWordDistance(tweet1, tweet2 string) float64 {
	words1 := strings.Fields(strings.ToLower(tweet1))
	words2 := strings.Fields(strings.ToLower(tweet2))

	if len(words1) == 0 && len(words2) == 0 {
		return 0.0
	}

	distance := wordDistance(tweet1, tweet2)
	maxWords := utils.Max(len(words1), len(words2))

	return float64(distance) / float64(maxWords)
}

// Batch represents a collection of tweets processed together
type Batch struct {
	BatchID  int64
	Tweets   []*tweets.Tweet
	Clusters []pipeline.TweetCluster
}

// OutputType represents the type of structured output
type OutputType string

const (
	OUTPUT_CLUSTER OutputType = "batch"
	OUTPUT_STATS   OutputType = "stats"
	OUTPUT_ERROR   OutputType = "error"
	OUTPUT_INFO    OutputType = "info"
)

// OutputData represents structured output data
type OutputData struct {
	Type OutputType  `json:"type"`
	Data interface{} `json:"data"`
}
