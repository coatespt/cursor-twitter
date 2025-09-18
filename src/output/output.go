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
	TotalTweets          int           `json:"total_tweets"`
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
func clusterSimilarity(cluster1, cluster2 map[string]interface{}, cfg *config.Config) float64 {
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

func PerformMetaClustering(clusters []map[string]interface{}, cfg *config.Config) []interface{} {
	if !cfg.Analysis.EnableMetaClustering || len(clusters) < 2 {
		// Return individual clusters if meta-clustering is disabled or not enough clusters
		result := make([]interface{}, len(clusters))
		for i, cluster := range clusters {
			result[i] = ConvertToIndividualCluster(cluster)
		}
		return result
	}

	// Use union approach if enabled
	if cfg.Analysis.UseUnionApproach {
		return PerformUnionMetaClustering(clusters, cfg)
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
			individualClusters = append(individualClusters, ConvertToIndividualCluster(subClusters[0]))
		}
		// If subClusters is empty, do nothing (this can happen if no clusters meet similarity criteria)
	}

	// Add any remaining unassigned clusters as individual clusters
	for i := 0; i < len(clusters); i++ {
		if !assigned[i] {
			individualClusters = append(individualClusters, ConvertToIndividualCluster(clusters[i]))
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
func PerformUnionMetaClustering(clusters []map[string]interface{}, cfg *config.Config) []interface{} {
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
			individualClusters = append(individualClusters, ConvertToIndividualCluster(subClusters[0]))
		}
	}

	// Add any remaining unassigned clusters as individual clusters
	for i := 0; i < len(clusters); i++ {
		if !assigned[i] {
			individualClusters = append(individualClusters, ConvertToIndividualCluster(clusters[i]))
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

func ConvertToIndividualCluster(cluster map[string]interface{}) *IndividualCluster {
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
func generateIDFromTheme(theme string) string {
	// Simple hash-like function
	hash := 0
	for _, char := range theme {
		hash = (hash*31 + int(char)) % 10000
	}
	return fmt.Sprintf("%s_%04d", theme, hash)
}

func ConvertBatchToHumanReadable(batchMap map[string]interface{}, cfg *config.Config) interface{} {
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
					if !ShouldFilterRepetitiveCluster(cluster, cfg) {
						filteredClusters = append(filteredClusters, cluster)
					}
				}
			}
		}
		// Convert all clusters to human-readable format first (for tweet text extraction)
		var humanReadableFilteredClusters []map[string]interface{}
		for _, cluster := range filteredClusters {
			humanReadableCluster := ConvertIndividualClusterToHumanReadable(cluster, cfg)
			if humanReadableCluster != nil {
				if clusterMap, ok := humanReadableCluster.(map[string]interface{}); ok {
					humanReadableFilteredClusters = append(humanReadableFilteredClusters, clusterMap)
				}
			}
		}

		// Apply meta-clustering if enabled
		if cfg.Analysis.EnableMetaClustering {
			humanReadableClusters = PerformMetaClustering(humanReadableFilteredClusters, cfg)
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
		if batchNum, ok := v.(int64); ok {
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
