package pipeline

import (
	"cursor-twitter/src/tweets"
	"log/slog"
	"sort"
	"time"
)

// TweetCluster represents a cluster of tweets with shared busy words
type TweetCluster struct {
	Tweets     []*tweets.Tweet
	BusyWords  []string
	Size       int
	Modularity float64
}

// ClusteringResult holds the results of tweet clustering
type ClusteringResult struct {
	Clusters []TweetCluster
	Stats    ClusteringStats
}

// ClusteringStats holds performance and quality statistics
type ClusteringStats struct {
	TotalTweets     int
	TweetsWithWords int
	TotalEdges      int
	GraphDensity    float64
	ProcessingTime  float64
	ClustersFound   int
}

// OptimizedTweetClusterer performs efficient clustering using sparse graph construction to avoid O(n²) complexity
type OptimizedTweetClusterer struct {
	minJaccardSimilarity float64
	maxTweetsToCluster   int
}

// NewOptimizedTweetClusterer creates a new clusterer with the given parameters
func NewOptimizedTweetClusterer(minJaccardSimilarity float64, maxTweetsToCluster int) *OptimizedTweetClusterer {
	return &OptimizedTweetClusterer{
		minJaccardSimilarity: minJaccardSimilarity,
		maxTweetsToCluster:   maxTweetsToCluster,
	}
}

// ClusterTweets performs optimized clustering on tweets with busy words
func (c *OptimizedTweetClusterer) ClusterTweets(tweets []*tweets.Tweet, busyWords map[string]bool, minClusterSize int) ClusteringResult {
	startTime := time.Now()

	slog.Info("Starting optimized clustering",
		"total_tweets", len(tweets),
		"busy_words", len(busyWords),
		"min_jaccard", c.minJaccardSimilarity,
		"max_tweets", c.maxTweetsToCluster)

	// Step 1: Filter tweets to only those with busy words and apply size limit
	filterStart := time.Now()
	filteredTweets := c.filterTweetsForClustering(tweets, busyWords)
	filterDuration := time.Since(filterStart)

	if len(filteredTweets) == 0 {
		totalDuration := time.Since(startTime)
		return ClusteringResult{Stats: ClusteringStats{
			TotalTweets:    len(tweets),
			ProcessingTime: totalDuration.Seconds(),
		}}
	}

	// Step 2: Build inverted index (busy_word -> []tweet_indices)
	indexStart := time.Now()
	wordToTweets := c.buildInvertedIndex(filteredTweets, busyWords)
	indexDuration := time.Since(indexStart)

	// Step 3: Build sparse graph using only tweets that share busy words
	graphStart := time.Now()
	edges := c.buildSparseGraph(filteredTweets, wordToTweets, busyWords)
	graphDuration := time.Since(graphStart)

	// Step 4: Perform clustering (placeholder for now - will implement Louvain)
	clusterStart := time.Now()
	clusters := c.performClustering(filteredTweets, edges, busyWords, minClusterSize)
	clusterDuration := time.Since(clusterStart)

	// Calculate total time
	totalDuration := time.Since(startTime)

	// Calculate statistics
	stats := c.calculateStats(len(tweets), len(filteredTweets), len(edges), len(clusters))
	stats.ProcessingTime = totalDuration.Seconds()

	slog.Info("Clustering completed",
		"original_tweets", len(tweets),
		"filtered_tweets", len(filteredTweets),
		"edges_created", len(edges),
		"clusters_found", len(clusters),
		"graph_density", stats.GraphDensity,
		"total_time_ms", totalDuration.Milliseconds(),
		"filter_time_ms", filterDuration.Milliseconds(),
		"index_time_ms", indexDuration.Milliseconds(),
		"graph_time_ms", graphDuration.Milliseconds(),
		"cluster_time_ms", clusterDuration.Milliseconds())

	return ClusteringResult{
		Clusters: clusters,
		Stats:    stats,
	}
}

// filterTweetsForClustering filters tweets to only those with busy words and applies size limits
func (c *OptimizedTweetClusterer) filterTweetsForClustering(tweetList []*tweets.Tweet, busyWords map[string]bool) []*tweets.Tweet {
	var filtered []*tweets.Tweet
	busyWordCounts := make(map[int]int) // count -> number of tweets

	for _, tweet := range tweetList {
		busyWordCount := 0
		for _, token := range tweet.Tokens {
			if busyWords[token] {
				busyWordCount++
			}
		}
		busyWordCounts[busyWordCount]++

		if busyWordCount > 0 {
			filtered = append(filtered, tweet)
		}
	}

	// Apply size limit if configured
	if c.maxTweetsToCluster > 0 && len(filtered) > c.maxTweetsToCluster {
		slog.Warn("Limiting tweets for clustering due to performance constraints",
			"original_count", len(filtered),
			"limited_count", c.maxTweetsToCluster)
		filtered = filtered[:c.maxTweetsToCluster]
	}

	// Log distribution
	slog.Info("Tweet filtering results",
		"total_tweets", len(tweetList),
		"tweets_with_busy_words", len(filtered),
		"busy_word_distribution", busyWordCounts)

	return filtered
}

// buildInvertedIndex creates a map from busy words to tweet indices
func (c *OptimizedTweetClusterer) buildInvertedIndex(tweetList []*tweets.Tweet, busyWords map[string]bool) map[string][]int {
	wordToTweets := make(map[string][]int)

	for tweetIdx, tweet := range tweetList {
		for _, token := range tweet.Tokens {
			if busyWords[token] {
				wordToTweets[token] = append(wordToTweets[token], tweetIdx)
			}
		}
	}

	slog.Debug("Built inverted index",
		"unique_busy_words", len(wordToTweets),
		"total_word_occurrences", func() int {
			total := 0
			for _, indices := range wordToTweets {
				total += len(indices)
			}
			return total
		}())

	return wordToTweets
}

// Edge represents a connection between two tweets with a similarity weight
type Edge struct {
	From   int
	To     int
	Weight float64
	Shared int
	Union  int
}

// buildSparseGraph creates edges only between tweets that share busy words
func (c *OptimizedTweetClusterer) buildSparseGraph(tweetList []*tweets.Tweet, wordToTweets map[string][]int, busyWords map[string]bool) []Edge {
	var edges []Edge
	edgeCount := 0
	comparisonCount := 0

	// For each busy word, compare all pairs of tweets that contain it
	for _, tweetIndices := range wordToTweets {
		if len(tweetIndices) < 2 {
			continue // Need at least 2 tweets to create edges
		}

		// Compare all pairs of tweets that share this word
		for i := 0; i < len(tweetIndices); i++ {
			for j := i + 1; j < len(tweetIndices); j++ {
				comparisonCount++

				tweet1Idx := tweetIndices[i]
				tweet2Idx := tweetIndices[j]

				// Calculate Jaccard similarity
				similarity := c.calculateJaccardSimilarity(tweetList[tweet1Idx], tweetList[tweet2Idx], busyWords)

				if similarity >= c.minJaccardSimilarity {
					edges = append(edges, Edge{
						From:   tweet1Idx,
						To:     tweet2Idx,
						Weight: similarity,
					})
					edgeCount++
				}
			}
		}
	}

	slog.Info("Sparse graph construction completed",
		"tweets", len(tweetList),
		"comparisons_made", comparisonCount,
		"edges_created", edgeCount,
		"edge_creation_rate", float64(edgeCount)/float64(comparisonCount))

	return edges
}

// calculateJaccardSimilarity calculates Jaccard similarity between two tweets based on busy words
func (c *OptimizedTweetClusterer) calculateJaccardSimilarity(tweet1, tweet2 *tweets.Tweet, busyWords map[string]bool) float64 {
	// Create sets of busy words for each tweet
	set1 := make(map[string]bool)
	set2 := make(map[string]bool)

	// For now, we'll use all tokens as potential busy words
	// In practice, this should be filtered to only actual busy words
	for _, token := range tweet1.Tokens {
		set1[token] = true
	}
	for _, token := range tweet2.Tokens {
		set2[token] = true
	}

	// Calculate intersection and union
	intersection := 0
	union := len(set1) + len(set2)

	for word := range set1 {
		if set2[word] {
			intersection++
			union-- // Adjust for double counting
		}
	}

	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// performClustering performs the actual clustering (placeholder for Louvain implementation)
func (c *OptimizedTweetClusterer) performClustering(tweetList []*tweets.Tweet, edges []Edge, busyWords map[string]bool, minClusterSize int) []TweetCluster {
	// TODO: Implement Louvain clustering using gonum/graph
	// For now, return a simple clustering based on connected components

	slog.Info("Performing clustering (placeholder implementation)",
		"tweets", len(tweetList),
		"edges", len(edges))

	// Simple connected components clustering as placeholder
	clusters := c.simpleConnectedComponents(tweetList, edges, busyWords, minClusterSize)

	return clusters
}

// simpleConnectedComponents implements basic connected components clustering
func (c *OptimizedTweetClusterer) simpleConnectedComponents(tweetList []*tweets.Tweet, edges []Edge, busyWords map[string]bool, minClusterSize int) []TweetCluster {
	// Build adjacency list
	adjacency := make(map[int][]int)
	for _, edge := range edges {
		adjacency[edge.From] = append(adjacency[edge.From], edge.To)
		adjacency[edge.To] = append(adjacency[edge.To], edge.From)
	}

	// Find connected components using DFS
	visited := make(map[int]bool)
	var clusters []TweetCluster

	for tweetIdx := range tweetList {
		if !visited[tweetIdx] {
			var component []int
			c.dfs(tweetIdx, adjacency, visited, &component)
			if len(component) > 1 { // Only clusters with multiple tweets
				clusterTweets := make([]*tweets.Tweet, len(component))
				for i, idx := range component {
					clusterTweets[i] = tweetList[idx]
				}
				sharedWords := c.findSharedBusyWords(clusterTweets, busyWords)
				clusters = append(clusters, TweetCluster{
					Tweets:    clusterTweets,
					BusyWords: sharedWords,
					Size:      len(component),
				})
			}
		}
	}

	return clusters
}

// dfs performs depth-first search for connected components
func (c *OptimizedTweetClusterer) dfs(node int, adjacency map[int][]int, visited map[int]bool, component *[]int) {
	visited[node] = true
	*component = append(*component, node)

	for _, neighbor := range adjacency[node] {
		if !visited[neighbor] {
			c.dfs(neighbor, adjacency, visited, component)
		}
	}
}

// findSharedBusyWords finds all busy words that appear in any tweet in the cluster (union of all busy words)
func (c *OptimizedTweetClusterer) findSharedBusyWords(tweets []*tweets.Tweet, busyWords map[string]bool) []string {
	if len(tweets) == 0 {
		return []string{}
	}

	// Collect all busy words that appear in any tweet in the cluster
	clusterWords := make(map[string]bool)

	for _, tweet := range tweets {
		for _, token := range tweet.Tokens {
			if busyWords[token] {
				clusterWords[token] = true
			}
		}
	}

	// Convert to slice and sort alphabetically for consistent display
	var allWords []string
	for word := range clusterWords {
		allWords = append(allWords, word)
	}
	sort.Strings(allWords)

	return allWords
}

// calculateStats calculates clustering performance statistics
func (c *OptimizedTweetClusterer) calculateStats(totalTweets, filteredTweets, edgeCount, clusterCount int) ClusteringStats {
	graphDensity := 0.0
	if filteredTweets > 1 {
		maxPossibleEdges := float64(filteredTweets*(filteredTweets-1)) / 2.0
		graphDensity = float64(edgeCount) / maxPossibleEdges
	}

	return ClusteringStats{
		TotalTweets:     totalTweets,
		TweetsWithWords: filteredTweets,
		TotalEdges:      edgeCount,
		GraphDensity:    graphDensity,
		ClustersFound:   clusterCount,
	}
}
