package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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

// Config struct for YAML config file (add log_dir)
type Config struct {
	Mode          string `yaml:"mode"`
	InputDir      string `yaml:"input"`
	FileSrcDir    string `yaml:"file_src_dir"` // Source directory for file input mode
	MQHost        string `yaml:"mq_host"`
	MQPort        int    `yaml:"mq_port"`
	MQQueue       string `yaml:"mq_queue"`
	WindowSize    int    `yaml:"window"`
	BatchSize     int    `yaml:"batch"`
	WindowBatches int    `yaml:"window_batches"` // Number of batches to keep in tweet window

	LogDir               string    `yaml:"log_dir"`
	LogLevel             string    `yaml:"log_level"` // DEBUG, INFO, WARN, ERROR
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
		FilterDir  string `yaml:"filter_dir"`
		FilterFile string `yaml:"filter_file"` // Keep for backward compatibility
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
		JaccardUseBusyWordsOnly      bool    `yaml:"jaccard_use_busy_words_only"`    // If true, Jaccard similarity uses only busy words; if false, uses all tokens
		MaxTweetsToCluster           int     `yaml:"max_tweets_to_cluster"`          // Maximum number of tweets to cluster (0 = no limit)
		SuppressDuplicates           bool    `yaml:"suppress_duplicates"`            // Suppress duplicate tweets in visualization
		DuplicateSimilarityThreshold float64 `yaml:"duplicate_similarity_threshold"` // Similarity threshold for duplicates
		LanguageFilter               string  `yaml:"language_filter"`                // Language filter: "en", "es", "all", etc.
		ClusteringMethod             string  `yaml:"clustering_method"`              // Method for clustering: "graph" (only valid option)
		OutputMode                   string  `yaml:"output_mode"`                    // Output mode: "verbose" or "human"
		MinClusterSize               int     `yaml:"min_cluster_size"`               // Minimum number of tweets in a cluster for it to be included in the output
		CreateFallbackClusters       bool    `yaml:"create_fallback_clusters"`       // Create fallback clusters when no clusters found but tweets exist
		// Persistence window configuration for tracking clusters across multiple batches
		WindowBatchesPersistence      int `yaml:"window_batches_persistence"`       // M
		WindowBatchesPersistenceCheck int `yaml:"window_batches_persistence_check"` // K
		// Minimum number of shared busy words required for clusters to be considered related (for persistence tracking)
		MinSharedBusyWordsForPersistence int `yaml:"min_shared_busywords_for_persistence"` // Relationship strength threshold
		// Method for determining cluster relationships across batches: "busy_words" or "full_text"
		PersistenceClusteringMethod    string           `yaml:"persistence_clustering_method"` // Cross-batch relationship detection method
		DropExcessiveQuestions         bool             `yaml:"drop_excessive_questions"`      // Drop tweets with excessive question marks
		MaxHumanTweetsDisplayed        int              `yaml:"max_human_tweets_displayed"`    // Maximum number of tweets to display in human-readable format
		FilterRepetitivePatterns       bool             `yaml:"filter_repetitive_patterns"`    // Filter out clusters with repetitive meme-like patterns
		BannedPhrasesDir               string           `yaml:"banned_phrases_dir"`            // Path to directory containing banned phrase files
		BannedPhrasesFile              string           `yaml:"banned_phrases_file"`           // Path to file containing banned phrases (backward compatibility)
		RepetitivePatternThreshold     float64          `yaml:"repetitive_pattern_threshold"`  // Threshold for filtering repetitive clusters
		CompiledBannedPatterns         []*regexp.Regexp // Compiled regex patterns (not in yaml)
		DeduplicateByUser              bool             `yaml:"deduplicate_by_user"`               // Deduplicate tweets by user within clusters
		UseLevenshteinDeduplication    bool             `yaml:"use_levenshtein_deduplication"`     // Use distance-based deduplication
		DistanceMethod                 string           `yaml:"distance_method"`                   // "character" or "word" distance method
		NearDuplicateThreshold         float64          `yaml:"near_duplicate_threshold"`          // Normalized distance threshold
		CleanupTriggerBatchSize        int              `yaml:"cleanup_trigger_batch_size"`        // Trigger cleanup every N tweets
		CleanupMaxItems                int              `yaml:"cleanup_max_items"`                 // Process up to M items per cleanup cycle
		ClusterSortDescending          bool             `yaml:"cluster_sort_descending"`           // Sort clusters by size: true=descending (biggest first), false=ascending (biggest last)
		SuppressIndividualTweets       bool             `yaml:"suppress_individual_tweets"`        // Suppress individual tweets in output, keep only metadata and medoid
		EnableMetaClustering           bool             `yaml:"enable_meta_clustering"`            // Enable clustering of clusters into meta-clusters
		MetaClusterSimilarityThreshold float64          `yaml:"meta_cluster_similarity_threshold"` // Similarity threshold for merging clusters (0.3-0.6)
		MetaClusterMinSize             int              `yaml:"meta_cluster_min_size"`             // Minimum total tweets for a meta-cluster
		UseMedoidSimilarity            bool             `yaml:"use_medoid_similarity"`             // Enable medoid similarity in meta-clustering
		UseBusyWordSimilarity          bool             `yaml:"use_busy_word_similarity"`          // Enable busy word similarity in meta-clustering
		UseUnionApproach               bool             `yaml:"use_union_approach"`                // Use union of medoid and busy word meta-clustering
		MedoidSimilarityThreshold      float64          `yaml:"medoid_similarity_threshold"`       // Separate threshold for medoid similarity
		BusyWordSimilarityThreshold    float64          `yaml:"busy_word_similarity_threshold"`    // Separate threshold for busy word similarity
		BWQueueMax                     float64          `yaml:"bw_queue_max"`                      // Multiplier for batch size to trigger busyword queue warnings
		BWThreadSlowDelay              int              `yaml:"bw_thread_slow_delay"`              // Total sleep time in milliseconds when busyword queues are backlogged
	} `yaml:"analysis"`
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
var (
	urlRegex        *regexp.Regexp
	apostropheRegex *regexp.Regexp
	tokenizeRegex   *regexp.Regexp
)

// init function to initialize regex variables for tests
func init() {
	urlRegex = regexp.MustCompile(`https?://[^\s]+|www\.[^\s]+`)
	apostropheRegex = regexp.MustCompile(`'.*$`)
	tokenizeRegex = regexp.MustCompile(`[^\w']+`)
}

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
func startAnalysisThread(resultChannel <-chan pipeline.BusyWordResult, cfg *Config, loadedState map[string]int) {
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
						result = pipeline.BuildFrequencyClassHashSets(loadedState, cfg.FreqClasses, nil, nil)
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

		// Track results by batch
		currentBatch := make(map[int][]string) // class -> busy words
		currentBatchNumber := -1
		expectedProcessors := len(cfg.BusywordClasses) // Number of frequency classes we expect results from

		// Initialize analytics batch count to current global batch count to avoid artificial lag
		analyticsBatchCount = globalBatchCount + 2 // Start 2 batches ahead to account for normal pipeline lag

		// Coordination pattern: collect exactly N results per batch, then release
		for {
			// Collect exactly N results for current batch
			resultsCollected := 0

			for resultsCollected < expectedProcessors {
				result, ok := <-resultChannel
				if !ok {
					// Channel closed, process final batch if exists
					if currentBatchNumber >= 0 && len(currentBatch) > 0 {
						recentTweets := globalTweetQueue.GetRecentTweets(cfg.BatchSize)
						runClusteringForBatch(currentBatch, recentTweets, currentBatchNumber, cfg)

					}
					return
				}

				resultCount++

				// Check if this is a new batch
				if currentBatchNumber != result.BatchNumber {
					// Run clustering for previous batch if it exists
					if currentBatchNumber >= 0 {
						recentTweets := globalTweetQueue.GetRecentTweets(cfg.BatchSize)
						runClusteringForBatch(currentBatch, recentTweets, currentBatchNumber, cfg)

					}

					// Start new batch
					currentBatch = make(map[int][]string)
					freqClassProcessor.IncrementBatchNumber()
					currentBatchNumber = result.BatchNumber
					analyticsBatchCount++
				}

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
				resultsCollected++
			}

			// All N results collected for this batch - run clustering
			recentTweets := globalTweetQueue.GetRecentTweets(cfg.BatchSize)
			runClusteringForBatch(currentBatch, recentTweets, currentBatchNumber, cfg)

			// Release barrier to allow processors to start next batch
			freqClassProcessor.ReleaseBarrier()
		}

		slog.Info("Analysis thread stopped", "results", resultCount)
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

	// Time the clustering operation
	clusteringStart := time.Now()
	timeSinceLastClustering := clusteringStart.Sub(lastClusteringTime)

	// Only graph clustering is supported now
	runGraphClustering(tweetsWithBusyWords, allBusyWords, cfg, batchNumber, classResults)

	clusteringDuration := time.Since(clusteringStart)

	// Log detailed timing information
	slog.Info("Clustering cycle timing",
		"batch", batchNumber,
		"time_since_last_clustering_ms", timeSinceLastClustering.Milliseconds(),
		"clustering_processing_time_ms", clusteringDuration.Milliseconds(),
		"tweets_processed", len(tweetsWithBusyWords),
		"busy_words_count", len(allBusyWords))

	// Update last clustering time
	lastClusteringTime = time.Now()

}

// runGraphClustering runs graph-based clustering on tweets
func runGraphClustering(tweetsWithBusyWords []*tweets.Tweet, allBusyWords map[string]bool, cfg *Config, batchNumber int, classResults map[int][]string) {
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
	var batchTimeStr string
	if len(tweetsWithBusyWords) > 0 {
		firstTweet := tweetsWithBusyWords[0]
		batchTimeStr = time.Unix(firstTweet.Unix, 0).Format("2006-01-02 15:04:05 UTC")
	} else {
		batchTimeStr = time.Now().UTC().Format("2006-01-02 15:04:05 UTC")
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

	slog.Info("Fallback cluster check",
		"batchClusters", len(batchClusters),
		"clustersAboveMinSize", clustersAboveMinSize,
		"tweetsWithBusyWords", len(tweetsWithBusyWords),
		"CreateFallbackClusters", cfg.Analysis.CreateFallbackClusters)

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
	totalClusters := len(batchClusters)

	// Recalculate clusters above min size after fallback cluster might have been added
	clustersAboveMinSize = 0
	for i, cluster := range batchClusters {
		if size, ok := cluster["size"].(int); ok {
			slog.Info("Cluster size check", "clusterIndex", i, "size", size, "minClusterSize", cfg.Analysis.MinClusterSize, "meetsMinSize", size >= cfg.Analysis.MinClusterSize)
			if size >= cfg.Analysis.MinClusterSize {
				clustersAboveMinSize++
			}
		} else {
			slog.Warn("Cluster size not found or not int", "clusterIndex", i, "cluster", cluster)
		}
	}

	slog.Info("Final batch data",
		"batchNumber", batchNumber,
		"totalClusters", totalClusters,
		"clustersAboveMinSize", clustersAboveMinSize,
		"totalTweets", len(tweetsWithBusyWords))

	batchData := map[string]interface{}{
		"batch_number":            batchNumber,
		"batch_time":              batchTimeStr,
		"method":                  "graph",
		"total_clusters":          totalClusters,
		"clusters_above_min_size": clustersAboveMinSize,
		"total_tweets":            len(tweetsWithBusyWords),
		"clusters":                batchClusters,
	}

	OutputClusterWithConfig(batchData, cfg)
}

// printBatchSummary prints a summary of all busy words found in a batch
func printBatchSummary(classResults map[int][]string, batchNumber int, cfg *Config) {
	// Note: Busy word summary is now logged in the structured format in runClusteringForBatch
	// This function is kept for compatibility but no longer outputs duplicate information
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

	// Note: Banned phrases loading moved to after path resolution
	// to handle relative paths correctly

	return cfg, nil
}

// Helper: Initialize logger
func initializeLogger(cfg *Config) (*slog.Logger, *os.File, error) {
	// Set the slog level based on config
	var slogLevel slog.Level
	switch strings.ToUpper(cfg.LogLevel) {
	case "DEBUG":
		slogLevel = slog.LevelDebug // DEBUG shows all messages (DEBUG, INFO, WARN, ERROR)
	case "INFO":
		slogLevel = slog.LevelInfo // INFO shows INFO, WARN, ERROR
	case "WARN":
		slogLevel = slog.LevelWarn // WARN shows WARN, ERROR
	case "ERROR":
		slogLevel = slog.LevelError // ERROR shows only ERROR
	default:
		slogLevel = slog.LevelInfo
	}

	logger, logFile, err := setupLogger(cfg.LogDir, slogLevel)
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

func main() {
	// Add a command line flag to control printing of tweets
	printTweets := flag.Bool("print-tweets", true, "Print each parsed tweet to the console")
	configPath := flag.String("config", "config/config.yaml", "Path to YAML config file")
	overridePath := flag.String("override", "", "Path to YAML override config file (optional)")
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
		slog.Info("CPU profiling enabled - will create cpu.prof file")
	}

	// Load config from YAML file with path resolution and optional override.
	var cfg *Config
	var err error
	if *overridePath != "" {
		cfg, err = loadConfigWithOverride(*configPath, *overridePath)
	} else {
		cfg, err = loadConfig(*configPath)
	}
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Set default values for new configuration options
	// CreateFallbackClusters defaults to true for safety (avoid losing data)
	// Users can set it to false in their config if they want to drop batches with no clusters
	if cfg.Analysis.CreateFallbackClusters == false {
		cfg.Analysis.CreateFallbackClusters = true // Default to true
	}

	// Logging is now handled entirely by slog

	slog.Info("*** CONFIG LOADED SUCCESSFULLY ***")

	// Print key configuration values to stderr for user visibility
	fmt.Fprintf(os.Stderr, "\n=== TWITTER SUBJECT DETECTION PIPELINE STARTUP ===\n")
	fmt.Fprintf(os.Stderr, "Config file: %s\n", *configPath)
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
	fmt.Fprintf(os.Stderr, "Output mode: %s\n", cfg.Analysis.OutputMode)
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
	fmt.Fprintf(os.Stderr, "Load state: %v\n", *loadState)
	fmt.Fprintf(os.Stderr, "Print tweets: %v\n", *printTweets)
	fmt.Fprintf(os.Stderr, "\n--- Additional Settings ---\n")
	fmt.Fprintf(os.Stderr, "Token filtering parameters available in config.yaml (token_filters section)\n")
	// Initialize logger first, before any slog calls
	logger, logFile, err := initializeLogger(cfg)
	if err != nil {
		log.Fatalf("Failed to set up logger: %v", err)
	}
	defer logFile.Close()
	slog.SetDefault(logger)

	fmt.Fprintf(os.Stderr, "\n=== STARTUP PROGRESS ===\n")
	fmt.Fprintf(os.Stderr, "Building frequency class filters... (this may take a few minutes)\n")
	fmt.Fprintf(os.Stderr, "Progress: ")

	// Also log the same config information to the log file
	slog.Info("=== TWITTER SUBJECT DETECTION PIPELINE STARTUP ===")
	slog.Info("Config file", "path", *configPath)
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
	slog.Info("Output mode", "mode", cfg.Analysis.OutputMode)
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
	slog.Info("Load state", "enabled", *loadState)
	slog.Info("Print tweets", "enabled", *printTweets)
	slog.Info("--- Additional Settings ---")
	slog.Info("Token filtering parameters available in config.yaml (token_filters section)")

	slog.Info("=== STARTUP PROGRESS ===")
	slog.Info("Building frequency class filters... (this may take a few minutes)")

	// Log z-score array
	slog.Info("Z-scores per frequency class", "scores", cfg.ZScores)

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
	if globalWordFilter != nil {
		slog.Info("Word filter initialized", "words", globalWordFilter.GetFilteredCount())
	} else {
		slog.Info("Word filter is nil (disabled)")
	}

	// Initialize pre-compiled regexes for tokenization
	urlRegex = regexp.MustCompile(`(https?://[^\s]+|www\.[^\s]+)`)
	apostropheRegex = regexp.MustCompile(`'.*`)
	tokenizeRegex = regexp.MustCompile(`[^\w']+`)

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

	// Initialize last clustering time
	lastClusteringTime = time.Now()

	// Start the analysis thread to process busy word results and run clustering
	startAnalysisThread(freqClassProcessor.GetResultChannel(), cfg, loadedState)

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
	fmt.Fprintf(os.Stderr, "DEBUG: Config mode = '%s'\n", cfg.Mode)
	slog.Info("DEBUG: Config mode", "mode", cfg.Mode)

	switch cfg.Mode {
	case "mqj":
		fmt.Fprintf(os.Stderr, "DEBUG: Starting RabbitMQ mode\n")
		processFromRabbitMQ(cfg, *printTweets)
	case "files":
		fmt.Fprintf(os.Stderr, "DEBUG: Starting file reading mode\n")
		processFromFiles(cfg, *printTweets)
	default:
		slog.Error("Invalid mode specified", "mode", cfg.Mode, "valid_modes", []string{"mqj", "files"})
		os.Exit(1)
	}
}

// processFromRabbitMQ handles RabbitMQ input mode
func processFromRabbitMQ(cfg *Config, printTweets bool) {
	fmt.Fprintf(os.Stderr, "Consuming tweets from RabbitMQ...\n")
	slog.Info("Consuming tweets from RabbitMQ...")

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
		if printTweets {
			// Log to file instead of stdout
			slog.Debug("Parsed Tweet", "tweet", tweet)
		}

		// Add tweet to global queue for clustering
		globalTweetQueue.Enqueue(tweet)

		// ========================================================================
		// CRITICAL TOKEN PROCESSING LOGIC - DO NOT MODIFY WITHOUT EXPLICIT APPROVAL
		// ========================================================================
		// This section handles token processing and 3PK mapping population.
		//
		// IMPORTANT DESIGN PRINCIPLES:
		// 1. ALWAYS put tokens on inbound queue (regardless of filter availability)
		// 2. ALWAYS populate 3PK mappings (regardless of filter availability)
		// 3. Only route tokens to frequency classes if filters exist
		// 4. If no filters, just continue reading tweets (no waiting)
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
				threePK, freqClass, exists := pipeline.GetTokenInfo(token)
				if !exists {
					// New token: create 3PK, insert into mapping
					threePK = pipeline.GenerateThreePartKey(token) // This inserts into the mapping
					if pipeline.HasGlobalFilters() {
						freqClass = pipeline.GetGlobalFiltersCount() - 1 // Least frequent class (highest number)
					}
				}

				// Route tokens to frequency classes only if filters are available
				if pipeline.HasGlobalFilters() {

					// Enqueue to appropriate frequency class (skip if class is in skip list)
					if !freqClassProcessor.IsClassActive(freqClass) {
						// Skip this frequency class - don't enqueue tokens
						continue
					}
					freqClassProcessor.EnqueueToFrequencyClass(freqClass, threePK)
				} else {
					// No filters available yet - this is normal during startup
					// The FCT will build filters as tokens arrive via the inboundTokenQueue
				}
			}
		}

		// ========================================================================
		// END CRITICAL TOKEN PROCESSING LOGIC
		// ========================================================================

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

				// Increment global batch count when signal 3PK is sent (this represents actual batches sent for processing)
				globalBatchCount++

			} else {
				// No filters available yet - this is normal during startup
				// Skip batch termination until filters are built
			}
		}

		// Process cleanup queue every N tweets to remove zero-count tokens
		// This mitigates 3pk collisions by cleaning up tokens that are no longer active
		if cfg.Analysis.CleanupTriggerBatchSize > 0 && TotalTweetsRead%cfg.Analysis.CleanupTriggerBatchSize == 0 && TotalTweetsRead > 0 {
			// Get queue size before processing
			queueSizeBefore := pipeline.GetCleanupQueueSize()

			// Use dynamic safety buffer - never remove the last N items from cleanup queue
			cleanupLeaveAtLeast := int(pipeline.GetDynamicCleanupLeaveAtLeast())
			if cleanupLeaveAtLeast <= 0 {
				cleanupLeaveAtLeast = 100000 // Fallback default safety buffer
			}

			// Only process cleanup if queue is large enough to leave safety buffer
			if queueSizeBefore <= cleanupLeaveAtLeast {
				// Queue too small - skip cleanup to avoid race conditions
				continue
			}

			cleanupMaxItems := cfg.Analysis.CleanupMaxItems
			if cleanupMaxItems <= 0 {
				cleanupMaxItems = 2000 // Default if not configured
			}

			// Don't remove more items than would leave us below the safety buffer
			maxRemovable := queueSizeBefore - cleanupLeaveAtLeast
			if cleanupMaxItems > maxRemovable {
				cleanupMaxItems = maxRemovable
			}

			removedCount := pipeline.ProcessCleanupQueue(cleanupMaxItems)

			// Get queue size after processing
			queueSizeAfter := pipeline.GetCleanupQueueSize()

			// Log cleanup performance (debug level - only shows in verbose mode)
			// Only log when items were actually processed (queue wasn't empty)
			if removedCount > 0 {
				slog.Debug("3PK cleanup performance", "tweet_count", TotalTweetsRead, "queue_size_before", queueSizeBefore, "queue_size_after", queueSizeAfter, "items_processed", removedCount, "max_items_per_cycle", cleanupMaxItems, "cleanup_trigger_batch_size", cfg.Analysis.CleanupTriggerBatchSize)
			}

			// Monitor busyword processor queues for potential system instability
			if cfg.Analysis.BWQueueMax > 0 && pipeline.HasGlobalFilters() {
				threshold := int(float64(cfg.BatchSize) * cfg.Analysis.BWQueueMax)
				queueStats := freqClassProcessor.GetQueueStats()

				for classIndex := 0; classIndex < freqClasses; classIndex++ {
					queueKey := fmt.Sprintf("freq_class_%d_queue_size", classIndex)
					queueSize := queueStats[queueKey]

					if queueSize > threshold {
						warningMsg := fmt.Sprintf("BW Processor queue %d has %d items (threshold: %d, batch_size: %d, bw_queue_max: %.2f)",
							classIndex, queueSize, threshold, cfg.BatchSize, cfg.Analysis.BWQueueMax)

						// Log to file
						slog.Warn("Busyword processor queue backlog detected",
							"frequency_class", classIndex,
							"queue_size", queueSize,
							"threshold", threshold,
							"batch_size", cfg.BatchSize,
							"bw_queue_max", cfg.Analysis.BWQueueMax,
							"tweet_count", TotalTweetsRead)

						// Echo to stderr
						fmt.Fprintf(os.Stderr, "*** %s ***\n", warningMsg)

						// Sleep immediately to let this processor catch up
						if cfg.Analysis.BWThreadSlowDelay > 0 {
							sleepMs := cfg.Analysis.BWThreadSlowDelay / cfg.FreqClasses
							slog.Warn("Main thread sleeping due to busy word queue backlog",
								"frequency_class", classIndex,
								"queue_size", queueSize,
								"threshold", threshold,
								"sleep_ms", sleepMs)
							time.Sleep(time.Duration(sleepMs) * time.Millisecond)
						}

					}
				}
			}
		}

		// Acknowledge successful message processing
		msg.Ack(false) // false = single acknowledgment
	}
}

// processFromFiles handles file input mode
func processFromFiles(cfg *Config, printTweets bool) {
	fmt.Fprintf(os.Stderr, "Reading tweets from files in: %s\n", cfg.FileSrcDir)
	slog.Info("Reading tweets from files", "dir", cfg.FileSrcDir)

	// Get list of CSV files in the directory
	files, err := filepath.Glob(filepath.Join(cfg.FileSrcDir, "*.csv"))
	if err != nil {
		slog.Error("Failed to read directory", "error", err, "directory", cfg.FileSrcDir)
		os.Exit(1)
	}

	if len(files) == 0 {
		slog.Error("No CSV files found in directory", "directory", cfg.FileSrcDir)
		os.Exit(1)
	}

	// Sort files for consistent processing order
	sort.Strings(files)

	for _, filePath := range files {
		processCSVFile(filePath, cfg, printTweets)
	}
}

// processCSVFile processes a single CSV file
func processCSVFile(filePath string, cfg *Config, printTweets bool) {
	slog.Info("Processing file", "file", filePath)

	file, err := os.Open(filePath)
	if err != nil {
		slog.Error("Failed to open file", "error", err, "file", filePath)
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)

	// Skip header if present
	_, err = reader.Read()
	if err != nil {
		slog.Error("Failed to read header", "error", err, "file", filePath)
		return
	}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			slog.Warn("Failed to read CSV record", "error", err, "file", filePath)
			continue
		}

		// Convert record to CSV row format (comma-separated string)
		row := strings.Join(record, ",")

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
			slog.Warn("Failed to parse tweet", "error", err, "file", filePath, "row", row)
			continue
		}
		if tweet == nil {
			// Tweet was filtered out (e.g., by language)
			continue
		}

		if printTweets {
			slog.Debug("Parsed Tweet", "tweet", tweet)
		}

		// Add tweet to global queue for clustering
		globalTweetQueue.Enqueue(tweet)

		// ========================================================================
		// CRITICAL TOKEN PROCESSING LOGIC - DO NOT MODIFY WITHOUT EXPLICIT APPROVAL
		// ========================================================================
		// This section handles token processing and 3PK mapping population.
		//
		// IMPORTANT DESIGN PRINCIPLES:
		// 1. ALWAYS put tokens on inbound queue (regardless of filter availability)
		// 2. ALWAYS populate 3PK mappings (regardless of filter availability)
		// 3. Only route tokens to frequency classes if filters exist
		// 4. If no filters, just continue reading tweets (no waiting)
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
				threePK, freqClass, exists := pipeline.GetTokenInfo(token)
				if !exists {
					// New token: create 3PK, insert into mapping
					threePK = pipeline.GenerateThreePartKey(token) // This inserts into the mapping
					if pipeline.HasGlobalFilters() {
						freqClass = pipeline.GetGlobalFiltersCount() - 1 // Least frequent class (highest number)
					}
				}

				// Route tokens to frequency classes only if filters are available
				if pipeline.HasGlobalFilters() {

					// Enqueue to appropriate frequency class (skip if class is in skip list)
					if !freqClassProcessor.IsClassActive(freqClass) {
						// Skip this frequency class - don't enqueue tokens
						continue
					}
					freqClassProcessor.EnqueueToFrequencyClass(freqClass, threePK)
				} else {
					// No filters available yet - this is normal during startup
					// The FCT will build filters as tokens arrive via the inboundTokenQueue
				}
			}
		}

		// ========================================================================
		// END CRITICAL TOKEN PROCESSING LOGIC
		// ========================================================================

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

				// Increment global batch count when signal 3PK is sent (this represents actual batches sent for processing)
				globalBatchCount++

			} else {
				// No filters available yet - this is normal during startup
				// Skip batch termination until filters are built
			}
		}

		// Process cleanup queue every N tweets to remove zero-count tokens
		// This mitigates 3pk collisions by cleaning up tokens that are no longer active
		if cfg.Analysis.CleanupTriggerBatchSize > 0 && TotalTweetsRead%cfg.Analysis.CleanupTriggerBatchSize == 0 && TotalTweetsRead > 0 {
			// Get queue size before processing
			queueSizeBefore := pipeline.GetCleanupQueueSize()

			// Use dynamic safety buffer - never remove the last N items from cleanup queue
			cleanupLeaveAtLeast := int(pipeline.GetDynamicCleanupLeaveAtLeast())
			if cleanupLeaveAtLeast <= 0 {
				cleanupLeaveAtLeast = 100000 // Fallback default safety buffer
			}

			// Only process cleanup if queue is large enough to leave safety buffer
			if queueSizeBefore <= cleanupLeaveAtLeast {
				// Queue too small - skip cleanup to avoid race conditions
				continue
			}

			cleanupMaxItems := cfg.Analysis.CleanupMaxItems
			if cleanupMaxItems <= 0 {
				cleanupMaxItems = 2000 // Default if not configured
			}

			// Don't remove more items than would leave us below the safety buffer
			maxRemovable := queueSizeBefore - cleanupLeaveAtLeast
			if cleanupMaxItems > maxRemovable {
				cleanupMaxItems = maxRemovable
			}

			removedCount := pipeline.ProcessCleanupQueue(cleanupMaxItems)

			// Get queue size after processing
			queueSizeAfter := pipeline.GetCleanupQueueSize()

			// Log cleanup performance (debug level - only shows in verbose mode)
			// Only log when items were actually processed (queue wasn't empty)
			if removedCount > 0 {
				slog.Debug("3PK cleanup performance", "tweet_count", TotalTweetsRead, "queue_size_before", queueSizeBefore, "queue_size_after", queueSizeAfter, "items_processed", removedCount, "max_items_per_cycle", cleanupMaxItems, "cleanup_trigger_batch_size", cfg.Analysis.CleanupTriggerBatchSize)
			}

			// Monitor busyword processor queues for potential system instability
			if cfg.Analysis.BWQueueMax > 0 && pipeline.HasGlobalFilters() {
				threshold := int(float64(cfg.BatchSize) * cfg.Analysis.BWQueueMax)
				queueStats := freqClassProcessor.GetQueueStats()

				for classIndex := 0; classIndex < freqClasses; classIndex++ {
					queueKey := fmt.Sprintf("freq_class_%d_queue_size", classIndex)
					queueSize := queueStats[queueKey]

					if queueSize > threshold {
						warningMsg := fmt.Sprintf("BW Processor queue %d has %d items (threshold: %d, batch_size: %d, bw_queue_max: %.2f)",
							classIndex, queueSize, threshold, cfg.BatchSize, cfg.Analysis.BWQueueMax)

						// Log to file
						slog.Warn("Busyword processor queue backlog detected",
							"frequency_class", classIndex,
							"queue_size", queueSize,
							"threshold", threshold,
							"batch_size", cfg.BatchSize,
							"bw_queue_max", cfg.Analysis.BWQueueMax,
							"tweet_count", TotalTweetsRead)

						// Echo to stderr
						fmt.Fprintf(os.Stderr, "*** %s ***\n", warningMsg)

						// Sleep immediately to let this processor catch up
						if cfg.Analysis.BWThreadSlowDelay > 0 {
							sleepMs := cfg.Analysis.BWThreadSlowDelay / cfg.FreqClasses
							slog.Warn("Main thread sleeping due to busy word queue backlog",
								"frequency_class", classIndex,
								"queue_size", queueSize,
								"threshold", threshold,
								"sleep_ms", sleepMs)
							time.Sleep(time.Duration(sleepMs) * time.Millisecond)
						}

					}
				}
			}
		}
	}

	slog.Info("Completed processing file", "file", filePath)
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
func setupLogger(logDir string, level slog.Level) (*slog.Logger, *os.File, error) {
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
		Level: level,
	}))
	return logger, logFile, nil
}

// startStatsPrinter launches a goroutine that prints stats every 30 seconds.
func startStatsPrinter() {
	lastStatsTime = time.Now()
	lastTweetCount = 0
	pipelineStartTime = time.Now() // Initialize pipeline start time
	ticker := time.NewTicker(5 * time.Second)
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

	// Calculate processing rate (recent period)
	timeDiff := now.Sub(lastStatsTime).Seconds()
	tweetDiff := totalTweets - lastTweetCount
	processingRate := float64(tweetDiff) / timeDiff

	// Calculate total rate for the whole run
	totalTimeDiff := now.Sub(pipelineStartTime).Seconds()
	totalRate := float64(totalTweets) / totalTimeDiff

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
	slog.Debug("--- Frequency Class Stats ---")
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

		// Use lazy evaluation for expensive formatting
		classIndex := i
		slog.Debug("Frequency Class Stats", "class", classIndex, "queue", queueSize, "processed", tokensProcessed, "distinct", distinctTokens)
	}
	slog.Debug("----------------------")

	// Also log to slog
	slog.Info("Pipeline stats ",
		"tweets", totalTweets,
		"tokens", totalTokens,
		"distinct", distinctTokens,
		// "window_size", windowSize, // Removed tweet-based window size
		"inbound_queue_size", inboundQueueSize,
		"total_rate_tweets_per_sec", totalRate,
		"processing_rate_tweets_per_sec", processingRate,
		"<------")

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
// - Splits on non-word characters (including periods, commas, etc.)
// - Removes apostrophes and what follows
// - Filters out offensive words if word filtering is enabled
// - Filters out tokens shorter than min_token_len if specified
func simpleTokenize(text string, cfg *Config) []string {
	// Use regex to split on non-word characters (including periods, commas, etc.)
	// This matches the approach used in analyze_tokens.go
	tokens := tokenizeRegex.Split(strings.ToLower(text), -1)

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
func loadPersistedState(stateDir string, freqClasses int, cfg *Config) map[string]int {
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
			timeDiff := abs(tweet.Unix - medianTimestamp)
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

// Helper function for absolute value
func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// Helper function
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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
				dp[i][j] = 1 + min(dp[i-1][j], min(dp[i][j-1], dp[i-1][j-1]))
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
				dp[i][j] = 1 + min(dp[i-1][j], min(dp[i][j-1], dp[i-1][j-1]))
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
	maxWords := max(len(words1), len(words2))

	return float64(distance) / float64(maxWords)
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
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
		processedData = convertToHumanReadable(cluster, cfg)
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
func convertToHumanReadable(cluster interface{}, cfg *Config) interface{} {
	// Type assert to get the cluster data
	clusterMap, ok := cluster.(map[string]interface{})
	if !ok {
		return cluster // Return original if not the expected format
	}

	// Check if this is a batch-level structure
	if _, hasBatchNumber := clusterMap["batch_number"]; hasBatchNumber {
		// This is a batch-level structure
		return convertBatchToHumanReadable(clusterMap, cfg)
	}

	// This is an individual cluster (legacy format)
	return convertIndividualClusterToHumanReadable(clusterMap, cfg)
}

// convertBatchToHumanReadable converts batch-level data to human-readable format
func convertBatchToHumanReadable(batchMap map[string]interface{}, cfg *Config) interface{} {
	// Convert clusters to human-readable format
	var totalClusters, clustersAboveMinSize int
	var humanReadableClusters []interface{}
	if clusters, ok := batchMap["clusters"].([]map[string]interface{}); ok {
		totalClusters = len(clusters)
		var filteredClusters []map[string]interface{}
		// First, apply user deduplication to all clusters if enabled
		// Deduplication is now handled in convertIndividualClusterToHumanReadable

		// Now filter by size and repetitive patterns
		for _, cluster := range clusters {
			if size, ok := cluster["size"].(int); ok {
				if size >= cfg.Analysis.MinClusterSize {
					// Apply repetitive pattern filtering
					if !shouldFilterRepetitiveCluster(cluster, cfg) {
						filteredClusters = append(filteredClusters, cluster)
					}
				}
			}
		}
		// Convert all clusters to human-readable format first (for tweet text extraction)
		var humanReadableFilteredClusters []map[string]interface{}
		for _, cluster := range filteredClusters {
			humanReadableCluster := convertIndividualClusterToHumanReadable(cluster, cfg)
			if humanReadableCluster != nil {
				if clusterMap, ok := humanReadableCluster.(map[string]interface{}); ok {
					humanReadableFilteredClusters = append(humanReadableFilteredClusters, clusterMap)
				}
			}
		}

		// Apply meta-clustering if enabled
		if cfg.Analysis.EnableMetaClustering {
			humanReadableClusters = performMetaClustering(humanReadableFilteredClusters, cfg)
		} else {
			// Use the already-converted clusters
			for _, cluster := range humanReadableFilteredClusters {
				humanReadableClusters = append(humanReadableClusters, cluster)
			}
		}

		// Sort clusters by size (works for both individual and meta clusters)
		sort.Slice(humanReadableClusters, func(i, j int) bool {
			var sizeI, sizeJ int

			// Handle both individual clusters and meta-clusters
			if metaCluster, ok := humanReadableClusters[i].(*MetaCluster); ok {
				sizeI = metaCluster.TotalTweets
			} else if individualCluster, ok := humanReadableClusters[i].(*IndividualCluster); ok {
				sizeI = individualCluster.Size
			} else if clusterMap, ok := humanReadableClusters[i].(map[string]interface{}); ok {
				if size, ok := clusterMap["size"].(int); ok {
					sizeI = size
				}
			}

			if metaCluster, ok := humanReadableClusters[j].(*MetaCluster); ok {
				sizeJ = metaCluster.TotalTweets
			} else if individualCluster, ok := humanReadableClusters[j].(*IndividualCluster); ok {
				sizeJ = individualCluster.Size
			} else if clusterMap, ok := humanReadableClusters[j].(map[string]interface{}); ok {
				if size, ok := clusterMap["size"].(int); ok {
					sizeJ = size
				}
			}

			if cfg.Analysis.ClusterSortDescending {
				return sizeI > sizeJ // Descending order (biggest first)
			} else {
				return sizeI < sizeJ // Ascending order (biggest last)
			}
		})

		// Reassign cluster IDs to avoid gaps after filtering/deduplication and sorting
		for i, cluster := range humanReadableClusters {
			if clusterMap, ok := cluster.(map[string]interface{}); ok {
				clusterMap["cluster_id"] = i + 1
			} else if individualCluster, ok := cluster.(*IndividualCluster); ok {
				individualCluster.ClusterID = i + 1
			} else if metaCluster, ok := cluster.(*MetaCluster); ok {
				metaCluster.MetaClusterID = fmt.Sprintf("meta_%d", i+1)
			}
		}

		clustersAboveMinSize = len(humanReadableClusters)
	}

	// Always update totalClusters to match actual output
	totalClusters = len(humanReadableClusters)

	// Create batch output with guaranteed field ordering
	batchOutput := &BatchOutput{
		TotalClusters:        totalClusters,
		ClustersAboveMinSize: clustersAboveMinSize,
		Clusters:             humanReadableClusters,
	}

	// Set optional fields if they exist
	if v, ok := batchMap["batch_number"]; ok {
		if batchNum, ok := v.(int); ok {
			batchOutput.BatchNumber = batchNum
		}
	}
	if v, ok := batchMap["batch_time"]; ok {
		if batchTime, ok := v.(string); ok {
			batchOutput.BatchTime = batchTime
		}
	}
	if v, ok := batchMap["method"]; ok {
		if method, ok := v.(string); ok {
			batchOutput.Method = method
		}
	}
	if v, ok := batchMap["total_tweets"]; ok {
		if totalTweets, ok := v.(int); ok {
			batchOutput.TotalTweets = totalTweets
		}
	}

	return batchOutput
}

// convertIndividualClusterToHumanReadable converts individual cluster data to human-readable format
// Returns nil if the cluster should be suppressed (not enough unique tweets after deduplication)
func convertIndividualClusterToHumanReadable(clusterMap map[string]interface{}, cfg *Config) interface{} {
	// Create a new map for human-readable output
	humanReadable := make(map[string]interface{})

	// Copy all the metadata fields (except size, which we'll calculate)
	for key, value := range clusterMap {
		if key != "tweets" && key != "most_typical_tweet" && key != "size" {
			humanReadable[key] = value
		}
	}

	// Convert tweets to just their texts
	var tweetTexts []string
	maxToShow := cfg.Analysis.MaxHumanTweetsDisplayed
	if maxToShow <= 0 {
		maxToShow = 10 // Default value
	}

	// For fallback clusters, show 3x the normal amount of tweets
	if fallbackCluster, ok := clusterMap["fallback_cluster"].(bool); ok && fallbackCluster {
		maxToShow = maxToShow * 3
	}

	// Track the total number of unique tweets after deduplication
	var uniqueTweetCount int
	var originalTweetCount int
	var nearDuplicateRemovedCount int

	// Check if tweet_texts is already stored in the cluster (after deduplication)
	if storedTexts, ok := clusterMap["tweet_texts"].([]string); ok {
		uniqueTweetCount = len(storedTexts)
		originalTweetCount = uniqueTweetCount // For pre-deduplicated data, they're the same
		for i, text := range storedTexts {
			if i >= maxToShow {
				break
			}
			// If the stored text already includes a timestamp, use it as-is
			// Otherwise, we can't add timestamp since we don't have the original tweet
			tweetTexts = append(tweetTexts, text)
		}
	} else if tweetsInterface, ok := clusterMap["tweets"].([]interface{}); ok {
		// Handle tweets stored as []interface{} (before deduplication)
		originalTweetCount = len(tweetsInterface)

		// Apply user deduplication if enabled
		var deduplicatedTweets []interface{}
		if cfg.Analysis.DeduplicateByUser {
			// Map to track seen tweets by content (not by user)
			seenTweetTexts := make(map[string]bool)

			for _, tweetInterface := range tweetsInterface {
				// Type assert to get tweet fields
				if tweetMap, ok := tweetInterface.(map[string]interface{}); ok {
					tweetText, _ := tweetMap["text"].(string)

					// Check if we've already seen this exact tweet text
					if !seenTweetTexts[tweetText] {
						seenTweetTexts[tweetText] = true
						deduplicatedTweets = append(deduplicatedTweets, tweetInterface)
					}
				}
			}
		} else {
			// No deduplication, use all tweets
			deduplicatedTweets = tweetsInterface
		}

		// Count unique tweets after deduplication
		uniqueTweetCount = len(deduplicatedTweets)

		// Convert deduplicated tweets to texts with timestamps (only if not suppressed)
		if !cfg.Analysis.SuppressIndividualTweets {
			for i, tweetInterface := range deduplicatedTweets {
				if i >= maxToShow {
					break
				}
				if tweetMap, ok := tweetInterface.(map[string]interface{}); ok {
					if text, ok := tweetMap["text"].(string); ok {
						if createdAt, ok := tweetMap["created_at"].(string); ok {
							tweetWithTime := fmt.Sprintf("[%s] %s", createdAt, text)
							tweetTexts = append(tweetTexts, tweetWithTime)
						} else {
							tweetTexts = append(tweetTexts, text)
						}
					}
				}
			}
		}
	} else if tweetsStruct, ok := clusterMap["tweets"].([]*tweets.Tweet); ok {
		// Handle tweets stored as []*tweets.Tweet (before deduplication)
		originalTweetCount = len(tweetsStruct)

		// Apply user deduplication if enabled
		var deduplicatedTweets []*tweets.Tweet
		if cfg.Analysis.DeduplicateByUser {
			// Map to track seen tweets by content (not by user)
			seenTweetTexts := make(map[string]bool)

			for _, tweet := range tweetsStruct {
				tweetText := tweet.Text

				// Check if we've already seen this exact tweet text
				if !seenTweetTexts[tweetText] {
					seenTweetTexts[tweetText] = true
					deduplicatedTweets = append(deduplicatedTweets, tweet)
				}
			}
		} else {
			// No deduplication, use all tweets
			deduplicatedTweets = tweetsStruct
		}

		// Apply distance-based deduplication if enabled
		if cfg.Analysis.UseLevenshteinDeduplication && len(deduplicatedTweets) > 1 {
			distanceMethod := cfg.Analysis.DistanceMethod
			if distanceMethod == "" {
				distanceMethod = "word" // Default to word distance
			}

			deduplicatedTweets, nearDuplicateRemovedCount = removeNearDuplicates(deduplicatedTweets, cfg.Analysis.NearDuplicateThreshold, distanceMethod)
		}

		// Count unique tweets after deduplication
		uniqueTweetCount = len(deduplicatedTweets)

		// Convert deduplicated tweets to texts with timestamps (only if not suppressed)
		if !cfg.Analysis.SuppressIndividualTweets {
			for i, tweet := range deduplicatedTweets {
				if i >= maxToShow {
					break
				}
				tweetWithTime := fmt.Sprintf("[%s] %s", tweet.CreatedAt, tweet.Text)
				tweetTexts = append(tweetTexts, tweetWithTime)
			}
		}

	}

	// Check if we have enough unique tweets after deduplication
	if uniqueTweetCount < cfg.Analysis.MinClusterSize {
		return nil // Suppress this cluster
	}

	// Add size information to show both original and deduplicated counts
	humanReadable["size"] = uniqueTweetCount
	humanReadable["original_size"] = originalTweetCount
	humanReadable["size_info"] = fmt.Sprintf("%d/%d", uniqueTweetCount, originalTweetCount)

	// Add individual tweets if we have any (they're only built when not suppressed)
	if len(tweetTexts) > 0 {
		humanReadable["tweet_texts"] = tweetTexts
	}

	// Add the most typical tweet text with timestamp
	if mostTypicalTweet, ok := clusterMap["most_typical_tweet"].(*tweets.Tweet); ok && mostTypicalTweet != nil {
		medoidWithTime := fmt.Sprintf("[%s] %s", mostTypicalTweet.CreatedAt, mostTypicalTweet.Text)
		humanReadable["medoid_tweet_text"] = medoidWithTime
		// Preserve the original medoid for meta-clustering
		humanReadable["medoid"] = mostTypicalTweet.Text

	} else {
		// Fallback: try to get medoid from other sources
		if medoidText, ok := clusterMap["medoid"].(string); ok && medoidText != "" {
			humanReadable["medoid_tweet_text"] = medoidText
			humanReadable["medoid"] = medoidText
		} else if len(tweetTexts) > 0 {
			// Use first tweet as medoid if available
			humanReadable["medoid_tweet_text"] = tweetTexts[0]
			humanReadable["medoid"] = tweetTexts[0]
		}
	}

	// Add persistence information if available
	if persistenceInfo, ok := clusterMap["persistence_info"].(string); ok {
		humanReadable["persistence_info"] = persistenceInfo
	}

	// Add metadata about near-duplicate removal if any were removed
	if nearDuplicateRemovedCount > 0 {
		humanReadable["near_duplicates_removed"] = nearDuplicateRemovedCount
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

// shouldFilterRepetitiveCluster checks if a cluster should be filtered out due to repetitive patterns
func shouldFilterRepetitiveCluster(cluster map[string]interface{}, cfg *Config) bool {
	if !cfg.Analysis.FilterRepetitivePatterns || len(cfg.Analysis.CompiledBannedPatterns) == 0 {
		return false
	}

	tweets, ok := cluster["tweets"].([]*tweets.Tweet)
	if !ok {
		return false
	}

	if len(tweets) == 0 {
		return false
	}

	// Count tweets that match banned patterns
	matchingTweets := 0
	for _, tweet := range tweets {
		tweetTextLower := strings.ToLower(tweet.Text)
		for _, pattern := range cfg.Analysis.CompiledBannedPatterns {
			if pattern.MatchString(tweetTextLower) {
				matchingTweets++
				break
			}
		}
	}

	// Check if the percentage exceeds the threshold
	percentage := float64(matchingTweets) / float64(len(tweets))
	return percentage >= cfg.Analysis.RepetitivePatternThreshold
}

// loadBannedPhrases loads and compiles banned phrase patterns from a file
func loadBannedPhrases(filePath string) ([]*regexp.Regexp, error) {
	if filePath == "" {
		return nil, nil
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read banned phrases file %s: %v", filePath, err)
	}

	var patterns []*regexp.Regexp
	lines := strings.Split(string(content), "\n")

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		// Compile the pattern (case-insensitive)
		pattern, err := regexp.Compile("(?i)" + line)
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern on line %d: %s - %v", i+1, line, err)
		}
		patterns = append(patterns, pattern)
	}

	return patterns, nil
}

// loadBannedPhrasesFromDirectory loads and compiles banned phrase patterns from all .txt files in a directory
func loadBannedPhrasesFromDirectory(dirPath string) ([]*regexp.Regexp, error) {
	if dirPath == "" {
		return nil, nil
	}

	// Read all .txt files in directory
	files, err := filepath.Glob(filepath.Join(dirPath, "*.txt"))
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %v", dirPath, err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no .txt files found in directory %s", dirPath)
	}

	slog.Info("Loading banned phrases", "files", len(files), "dir", dirPath)
	var allPatterns []*regexp.Regexp
	for _, file := range files {
		slog.Info("Loading banned phrase file", "file", filepath.Base(file))
		patterns, err := loadBannedPhrases(file)
		if err != nil {
			return nil, fmt.Errorf("failed to load %s: %v", file, err)
		}
		allPatterns = append(allPatterns, patterns...)
	}
	slog.Info("Loaded banned phrase patterns", "count", len(allPatterns))
	return allPatterns, nil
}

// BatchOutput represents the human-readable batch output with guaranteed field ordering
type BatchOutput struct {
	BatchNumber          int         `json:"batch_number"`
	BatchTime            string      `json:"batch_time"`
	Method               string      `json:"method"`
	TotalTweets          int         `json:"total_tweets"`
	TotalClusters        int         `json:"total_clusters"`
	ClustersAboveMinSize int         `json:"clusters_above_min_size"`
	Clusters             interface{} `json:"clusters"`
}

// MetaCluster represents a group of similar clusters
type MetaCluster struct {
	Type          string                   `json:"type"`
	MetaClusterID string                   `json:"meta_cluster_id"`
	Theme         string                   `json:"theme"`
	TotalTweets   int                      `json:"total_tweets"`
	Medoid        string                   `json:"medoid"`
	BusyWords     []string                 `json:"busy_words"`
	SubClusters   []map[string]interface{} `json:"sub_clusters"`
}

// IndividualCluster represents a standalone cluster
type IndividualCluster struct {
	Type            string   `json:"type"`
	ClusterID       int      `json:"cluster_id"`
	Size            int      `json:"size"`
	Medoid          string   `json:"medoid"`
	BusyWords       []string `json:"busy_words"`
	TweetTexts      []string `json:"tweet_texts,omitempty"`
	FallbackCluster bool     `json:"fallback_cluster,omitempty"`
	ClusteringNote  string   `json:"clustering_note,omitempty"`
}

// clusterSimilarity calculates similarity between two clusters based on their medoids and busy words
func clusterSimilarity(cluster1, cluster2 map[string]interface{}, cfg *Config) float64 {
	// Check if any similarity measures are enabled
	if !cfg.Analysis.UseMedoidSimilarity && !cfg.Analysis.UseBusyWordSimilarity {
		return 0.0
	}

	var medoidSimilarity, busyWordSimilarity float64
	var medoidWeight, busyWordWeight float64

	// Calculate medoid similarity if enabled
	if cfg.Analysis.UseMedoidSimilarity {
		medoid1, ok1 := cluster1["medoid_tweet_text"].(string)
		medoid2, ok2 := cluster2["medoid_tweet_text"].(string)

		if !ok1 || !ok2 {
			return 0.0
		}

		medoidSimilarity = 1.0 - normalizedWordDistance(medoid1, medoid2)
	}

	// Calculate busy word similarity if enabled
	if cfg.Analysis.UseBusyWordSimilarity {
		busyWords1, ok1 := cluster1["busy_words"].([]string)
		busyWords2, ok2 := cluster2["busy_words"].([]string)

		if !ok1 || !ok2 {
			return 0.0
		}

		busyWordSimilarity = jaccard(busyWords1, busyWords2)
	}

	// Determine weights based on which measures are enabled
	if cfg.Analysis.UseMedoidSimilarity && cfg.Analysis.UseBusyWordSimilarity {
		// Both enabled: use original 60%/40% split
		medoidWeight = 0.6
		busyWordWeight = 0.4
	} else if cfg.Analysis.UseMedoidSimilarity {
		// Only medoid enabled: 100% weight
		medoidWeight = 1.0
		busyWordWeight = 0.0
	} else {
		// Only busy word enabled: 100% weight
		medoidWeight = 0.0
		busyWordWeight = 1.0
	}

	combinedSimilarity := (medoidSimilarity * medoidWeight) + (busyWordSimilarity * busyWordWeight)
	return combinedSimilarity
}

// performMetaClustering groups similar clusters into meta-clusters
func performMetaClustering(clusters []map[string]interface{}, cfg *Config) []interface{} {
	if !cfg.Analysis.EnableMetaClustering || len(clusters) < 2 {
		// Return individual clusters if meta-clustering is disabled or not enough clusters
		result := make([]interface{}, len(clusters))
		for i, cluster := range clusters {
			result[i] = convertToIndividualCluster(cluster)
		}
		return result
	}

	// Use union approach if enabled
	if cfg.Analysis.UseUnionApproach {
		return performUnionMetaClustering(clusters, cfg)
	}

	// Use traditional weighted approach
	threshold := cfg.Analysis.MetaClusterSimilarityThreshold

	// Track which clusters have been assigned to meta-clusters
	assigned := make([]bool, len(clusters))
	var metaClusters []*MetaCluster
	var individualClusters []interface{}

	// Try to group clusters into meta-clusters
	for i := 0; i < len(clusters); i++ {
		if assigned[i] {
			continue
		}

		// Start a new meta-cluster with this cluster
		totalTweets := 0
		var subClusters []map[string]interface{}

		// Find all clusters similar to this one
		for j := i; j < len(clusters); j++ {
			if assigned[j] {
				continue
			}

			// Check if clusters i and j are similar
			similarity := clusterSimilarity(clusters[i], clusters[j], cfg)
			if similarity >= threshold {
				assigned[j] = true
				subClusters = append(subClusters, clusters[j])

				// Get size of this sub-cluster
				if size, ok := clusters[j]["size"].(int); ok {
					totalTweets += size
				}
			}
		}

		// If we have multiple clusters, create a meta-cluster (regardless of total tweets)
		if len(subClusters) > 1 {
			metaCluster := createMetaCluster(subClusters, totalTweets)
			metaClusters = append(metaClusters, metaCluster)
		} else if len(subClusters) == 1 {
			// Single cluster becomes individual cluster
			individualClusters = append(individualClusters, convertToIndividualCluster(subClusters[0]))
		}
		// If subClusters is empty, do nothing (this can happen if no clusters meet similarity criteria)
	}

	// Add any remaining unassigned clusters as individual clusters
	for i := 0; i < len(clusters); i++ {
		if !assigned[i] {
			individualClusters = append(individualClusters, convertToIndividualCluster(clusters[i]))
		}
	}

	// Combine meta-clusters and individual clusters
	result := make([]interface{}, 0, len(metaClusters)+len(individualClusters))

	// Add meta-clusters first
	for _, mc := range metaClusters {
		result = append(result, mc)
	}

	// Add individual clusters
	result = append(result, individualClusters...)

	return result
}

// performUnionMetaClustering performs meta-clustering using union of medoid and busy word similarities
func performUnionMetaClustering(clusters []map[string]interface{}, cfg *Config) []interface{} {
	// Create adjacency matrices for both similarity measures
	medoidAdjacency := make([][]bool, len(clusters))
	busyWordAdjacency := make([][]bool, len(clusters))

	for i := range clusters {
		medoidAdjacency[i] = make([]bool, len(clusters))
		busyWordAdjacency[i] = make([]bool, len(clusters))
	}

	// Calculate medoid similarities
	if cfg.Analysis.UseMedoidSimilarity {
		for i := 0; i < len(clusters); i++ {
			for j := i; j < len(clusters); j++ {
				medoidSim := calculateMedoidSimilarity(clusters[i], clusters[j])
				medoidAdjacency[i][j] = medoidSim >= cfg.Analysis.MedoidSimilarityThreshold
				medoidAdjacency[j][i] = medoidAdjacency[i][j] // Symmetric
			}
		}
	}

	// Calculate busy word similarities
	if cfg.Analysis.UseBusyWordSimilarity {
		for i := 0; i < len(clusters); i++ {
			for j := i; j < len(clusters); j++ {
				busyWordSim := calculateBusyWordSimilarity(clusters[i], clusters[j])
				busyWordAdjacency[i][j] = busyWordSim >= cfg.Analysis.BusyWordSimilarityThreshold
				busyWordAdjacency[j][i] = busyWordAdjacency[i][j] // Symmetric
			}
		}
	}

	// Create union adjacency matrix (OR operation)
	unionAdjacency := make([][]bool, len(clusters))
	for i := range clusters {
		unionAdjacency[i] = make([]bool, len(clusters))
		for j := range clusters {
			unionAdjacency[i][j] = medoidAdjacency[i][j] || busyWordAdjacency[i][j]
		}
	}

	// Find connected components in the union graph
	assigned := make([]bool, len(clusters))
	var metaClusters []*MetaCluster
	var individualClusters []interface{}

	for i := 0; i < len(clusters); i++ {
		if assigned[i] {
			continue
		}

		// Start a new meta-cluster with this cluster
		totalTweets := 0
		var subClusters []map[string]interface{}

		// Find all clusters connected to this one (including itself)
		queue := []int{i}
		assigned[i] = true

		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]

			subClusters = append(subClusters, clusters[current])
			if size, ok := clusters[current]["size"].(int); ok {
				totalTweets += size
			}

			// Find all unassigned clusters connected to current
			for j := 0; j < len(clusters); j++ {
				if !assigned[j] && unionAdjacency[current][j] {
					assigned[j] = true
					queue = append(queue, j)
				}
			}
		}

		// If we have multiple clusters, create a meta-cluster
		if len(subClusters) > 1 {
			metaCluster := createMetaCluster(subClusters, totalTweets)
			metaClusters = append(metaClusters, metaCluster)
		} else if len(subClusters) == 1 {
			// Single cluster becomes individual cluster
			individualClusters = append(individualClusters, convertToIndividualCluster(subClusters[0]))
		}
	}

	// Add any remaining unassigned clusters as individual clusters
	for i := 0; i < len(clusters); i++ {
		if !assigned[i] {
			individualClusters = append(individualClusters, convertToIndividualCluster(clusters[i]))
		}
	}

	// Combine meta-clusters and individual clusters
	result := make([]interface{}, 0, len(metaClusters)+len(individualClusters))

	// Add meta-clusters first
	for _, mc := range metaClusters {
		result = append(result, mc)
	}

	// Add individual clusters
	result = append(result, individualClusters...)

	return result
}

// calculateMedoidSimilarity calculates similarity between two clusters based on medoid tweets only
func calculateMedoidSimilarity(cluster1, cluster2 map[string]interface{}) float64 {
	medoid1, ok1 := cluster1["medoid_tweet_text"].(string)
	medoid2, ok2 := cluster2["medoid_tweet_text"].(string)

	if !ok1 || !ok2 {
		return 0.0
	}

	return 1.0 - normalizedWordDistance(medoid1, medoid2)
}

// calculateBusyWordSimilarity calculates similarity between two clusters based on busy words only
func calculateBusyWordSimilarity(cluster1, cluster2 map[string]interface{}) float64 {
	busyWords1, ok1 := cluster1["busy_words"].([]string)
	busyWords2, ok2 := cluster2["busy_words"].([]string)

	if !ok1 || !ok2 {
		return 0.0
	}

	return jaccard(busyWords1, busyWords2)
}

// createMetaCluster creates a meta-cluster from a group of similar clusters
func createMetaCluster(subClusters []map[string]interface{}, totalTweets int) *MetaCluster {
	if len(subClusters) == 0 {
		return nil
	}

	// Use the largest cluster's medoid as the meta-cluster medoid
	var largestCluster map[string]interface{}
	maxSize := 0

	for _, cluster := range subClusters {
		if size, ok := cluster["size"].(int); ok && size > maxSize {
			maxSize = size
			largestCluster = cluster
		}
	}

	// Get medoid from largest cluster
	medoid := ""
	if largestCluster != nil {
		if medoidText, ok := largestCluster["medoid_tweet_text"].(string); ok {
			medoid = medoidText
		}
	}

	// Combine busy words from all sub-clusters
	allBusyWords := make(map[string]bool)
	for _, cluster := range subClusters {
		if busyWords, ok := cluster["busy_words"].([]string); ok {
			for _, word := range busyWords {
				allBusyWords[word] = true
			}
		}
	}

	// Convert to slice and sort for consistency
	busyWords := make([]string, 0, len(allBusyWords))
	for word := range allBusyWords {
		busyWords = append(busyWords, word)
	}
	sort.Strings(busyWords)

	// Generate theme from medoid (simple approach - could be enhanced)
	theme := generateThemeFromMedoid(medoid)

	// Create meta-cluster ID
	metaClusterID := fmt.Sprintf("meta_%s", generateIDFromTheme(theme))

	return &MetaCluster{
		Type:          "meta_cluster",
		MetaClusterID: metaClusterID,
		Theme:         theme,
		TotalTweets:   totalTweets,
		Medoid:        medoid,
		BusyWords:     busyWords,
		SubClusters:   subClusters,
	}
}

// convertToIndividualCluster converts a cluster map to IndividualCluster struct
func convertToIndividualCluster(cluster map[string]interface{}) *IndividualCluster {
	clusterID := 0
	if id, ok := cluster["cluster_id"].(int); ok {
		clusterID = id
	}

	size := 0
	if s, ok := cluster["size"].(int); ok {
		size = s
	}

	medoid := ""
	if medoidText, ok := cluster["medoid_tweet_text"].(string); ok {
		medoid = medoidText
	} else if medoidText, ok := cluster["medoid"].(string); ok {
		medoid = medoidText
	}

	busyWords := []string{}
	if words, ok := cluster["busy_words"].([]string); ok {
		busyWords = words
	}

	tweetTexts := []string{}
	if texts, ok := cluster["tweet_texts"].([]string); ok {
		tweetTexts = texts
	}

	fallbackCluster := false
	if fc, ok := cluster["fallback_cluster"].(bool); ok {
		fallbackCluster = fc
	}

	clusteringNote := ""
	if note, ok := cluster["clustering_note"].(string); ok {
		clusteringNote = note
	}

	// Determine the type based on whether it's a fallback cluster
	clusterType := "individual_cluster"
	if fallbackCluster {
		clusterType = "fallback_cluster"
	}

	return &IndividualCluster{
		Type:            clusterType,
		ClusterID:       clusterID,
		Size:            size,
		Medoid:          medoid,
		BusyWords:       busyWords,
		TweetTexts:      tweetTexts,
		FallbackCluster: fallbackCluster,
		ClusteringNote:  clusteringNote,
	}
}

// generateThemeFromMedoid creates a simple theme from the medoid text
func generateThemeFromMedoid(medoid string) string {
	if medoid == "" {
		return "unknown_theme"
	}

	// Simple approach: extract key words and create a theme
	// This could be enhanced with NLP or more sophisticated analysis
	words := strings.Fields(strings.ToLower(medoid))

	// Filter out common words
	commonWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
		"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
		"with": true, "by": true, "is": true, "are": true, "was": true, "were": true,
		"i": true, "you": true, "he": true, "she": true, "it": true, "we": true, "they": true,
		"this": true, "that": true, "these": true, "those": true,
	}

	var keyWords []string
	for _, word := range words {
		if len(word) > 2 && !commonWords[word] {
			keyWords = append(keyWords, word)
		}
	}

	if len(keyWords) == 0 {
		return "general_discussion"
	}

	// Take first few key words for theme
	if len(keyWords) > 3 {
		keyWords = keyWords[:3]
	}

	return strings.Join(keyWords, "_")
}

// generateIDFromTheme creates a simple ID from a theme
func generateIDFromTheme(theme string) string {
	// Simple hash-like function
	hash := 0
	for _, char := range theme {
		hash = (hash*31 + int(char)) % 10000
	}
	return fmt.Sprintf("%s_%04d", theme, hash)
}
