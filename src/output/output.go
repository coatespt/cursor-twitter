package output

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"cursor-twitter/src/config"
	"cursor-twitter/src/pipeline"
	"cursor-twitter/src/tweets"
)

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

// shouldFilterRepetitiveCluster checks if a cluster should be filtered based on repetitive patterns
func ShouldFilterRepetitiveCluster(cluster map[string]interface{}, cfg *config.Config) bool {
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

// BatchOutput represents the human-readable batch output with guaranteed field ordering
type BatchOutput struct {
	BatchNumber          int64         `json:"batch_number"`
	BatchTime            string        `json:"batch_time"`
	Method               string        `json:"method"`
	TotalClusters        int           `json:"total_clusters"`
	ClustersAboveMinSize int           `json:"clusters_above_min_size"`
	Clusters             []interface{} `json:"clusters"`
	MetaClusters         []interface{} `json:"meta_clusters,omitempty"`
	FallbackCluster      bool          `json:"fallback_cluster,omitempty"`
	ClusteringNote       string        `json:"clustering_note,omitempty"`
}

// OutputCluster outputs cluster data based on the configured output mode
func OutputCluster(cluster interface{}, cfg *config.Config) {
	// Default to verbose mode if no config provided
	outputMode := "verbose"
	if cfg != nil {
		outputMode = cfg.Analysis.OutputMode
	}

	// Process cluster data based on output mode
	var processedData interface{}

	if outputMode == "human" {
		// Convert to human-readable format
		processedData = ConvertToHumanReadable(cluster, cfg)
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

// OutputStats outputs statistics data
func OutputStats(stats interface{}) {
	data := OutputData{
		Type: OUTPUT_STATS,
		Data: stats,
	}
	jsonData, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(jsonData))
}

// OutputError outputs error data
func OutputError(err interface{}) {
	data := OutputData{
		Type: OUTPUT_ERROR,
		Data: err,
	}
	jsonData, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(jsonData))
}

// OutputInfo outputs info data
func OutputInfo(info interface{}) {
	data := OutputData{
		Type: OUTPUT_INFO,
		Data: info,
	}
	jsonData, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(jsonData))
}

// OutputRaw outputs raw formatted data for backward compatibility
func OutputRaw(format string, args ...interface{}) {
	fmt.Fprintf(os.Stdout, format+"\n", args...)
}

// ConvertToHumanReadable converts cluster data to human-readable format
func ConvertToHumanReadable(cluster interface{}, cfg *config.Config) interface{} {
	// Type assert to get the cluster data
	clusterMap, ok := cluster.(map[string]interface{})
	if !ok {
		return cluster // Return original if not the expected format
	}

	// Check if this is a batch-level structure
	if _, hasBatchNumber := clusterMap["batch_number"]; hasBatchNumber {
		// This is a batch-level structure
		// TODO: Call the function from main.go
		return cluster
	}

	// This is an individual cluster (legacy format)
	// TODO: Call the function from main.go
	return cluster
}
func ConvertIndividualClusterToHumanReadable(clusterMap map[string]interface{}, cfg *config.Config) interface{} {
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
	BatchID  int64
	Tweets   []*tweets.Tweet
	Clusters []pipeline.TweetCluster
}

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
