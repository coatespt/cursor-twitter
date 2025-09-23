package json_parser

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"cursor-twitter-display/types"
)

// Parser handles chunked JSON file reading and parsing
type Parser struct {
	fileHandle *os.File
	decoder    *json.Decoder
}

// NewParser creates a new JSON parser for the given file
func NewParser(filePath string) (*Parser, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %v", filePath, err)
	}

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()

	return &Parser{
		fileHandle: file,
		decoder:    decoder,
	}, nil
}

// Close closes the underlying file
func (p *Parser) Close() error {
	if p.fileHandle != nil {
		return p.fileHandle.Close()
	}
	return nil
}

// LoadNextChunk reads the next batch from the JSON stream
func (p *Parser) LoadNextChunk() ([]types.Batch, error) {
	if p.decoder == nil {
		return nil, fmt.Errorf("decoder not initialized")
	}

	var batch types.Batch
	err := p.decoder.Decode(&batch)
	if err != nil {
		if err == io.EOF {
			// End of file
			return nil, nil
		}
		return nil, fmt.Errorf("failed to decode JSON: %v", err)
	}

	return []types.Batch{batch}, nil
}

// LoadNextChunkContinuous reads the next batch and waits for new data if at end of file
func (p *Parser) LoadNextChunkContinuous() ([]types.Batch, error) {
	batches, err := p.LoadNextChunk()
	if err != nil {
		return nil, err
	}

	// If we got batches, return them immediately
	if len(batches) > 0 {
		return batches, nil
	}

	// No batches means we're at end of file
	fmt.Printf("Reached end of file, processing complete.\n")
	return nil, nil
}

// ParseClusters extracts cluster data from a batch
func ParseClusters(batch types.Batch) ([]types.Cluster, error) {
	// Handle null clusters (empty batches)
	if batch.Data.Clusters == nil {
		return []types.Cluster{}, nil
	}

	// The clusters are already parsed into the correct types.Batch structure
	// Just return them directly
	return batch.Data.Clusters, nil
}

// parseMainClusters parses the main clusters from the clusters interface
func parseMainClusters(clustersInterface interface{}) ([]types.Cluster, error) {
	// Type assert clusters to []interface{} first, then to map[string]interface{}
	clustersList, ok := clustersInterface.([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid clusters data format")
	}

	var clusters []types.Cluster
	for i, clusterInterface := range clustersList {
		clusterMap, ok := clusterInterface.(map[string]interface{})
		if !ok {
			fmt.Printf("Cluster %d: Failed to convert to map\n", i)
			continue
		}

		// Simple approach: process any cluster that has a cluster_id field
		// This handles individual_cluster, meta_cluster sub-clusters, etc.
		cluster := extractClusterFromMap(clusterMap)
		if cluster != nil {
			clusters = append(clusters, *cluster)
		}
	}

	return clusters, nil
}

// parseSubClusters parses sub-clusters from meta clusters
func parseSubClusters(clustersInterface interface{}) ([]types.Cluster, error) {
	clustersList, ok := clustersInterface.([]interface{})
	if !ok {
		return []types.Cluster{}, nil // Not an error, just no sub-clusters
	}

	var subClusters []types.Cluster
	for _, clusterInterface := range clustersList {
		clusterMap, ok := clusterInterface.(map[string]interface{})
		if !ok {
			continue
		}

		// Check for sub-clusters in meta clusters
		if subClustersInterface, ok := clusterMap["sub_clusters"].([]interface{}); ok {
			for _, subClusterInterface := range subClustersInterface {
				if subClusterMap, ok := subClusterInterface.(map[string]interface{}); ok {
					subCluster := extractClusterFromMap(subClusterMap)
					if subCluster != nil {
						subClusters = append(subClusters, *subCluster)
					}
				}
			}
		}
	}

	return subClusters, nil
}

// extractClusterFromMap extracts cluster data from a map, returns nil if invalid
// This function is robust and handles missing fields gracefully
func extractClusterFromMap(clusterMap map[string]interface{}) *types.Cluster {
	// Extract basic cluster information
	clusterID, size := extractBasicClusterInfo(clusterMap)
	if clusterID == 0 {
		return nil // Invalid cluster
	}

	// Extract busy words
	busyWords := extractBusyWords(clusterMap)

	// Extract tweets
	tweets := extractTweets(clusterMap)

	// Extract optional fields
	_, medoidTweet, _, busyWordClasses := extractOptionalFields(clusterMap)

	return &types.Cluster{
		ClusterID:       clusterID,
		Size:            size,
		BusyWords:       convertBusyWordsToTypes(busyWords),
		Tweets:          tweets,
		MedoidTweet:     medoidTweet,
		BusyWordClasses: convertBusyWordClassesToTypes(busyWordClasses),
	}
}

// extractBasicClusterInfo extracts cluster ID and size from the cluster map
func extractBasicClusterInfo(clusterMap map[string]interface{}) (int, int) {
	// Extract cluster data - cluster_id is required
	clusterIDFloat, hasClusterID := clusterMap["cluster_id"].(float64)
	if !hasClusterID {
		// No cluster_id means this isn't a valid cluster - skip silently
		return 0, 0
	}
	clusterID := int(clusterIDFloat)

	// Extract size with default
	size, _ := clusterMap["size"].(float64)
	if size == 0 {
		size = 0 // Default to 0 if missing
	}

	return clusterID, int(size)
}

// extractBusyWords extracts busy words from the cluster map
func extractBusyWords(clusterMap map[string]interface{}) map[string]bool {
	busyWords := make(map[string]bool)
	if busyWordsInterface, ok := clusterMap["busy_words"].(map[string]interface{}); ok {
		for word := range busyWordsInterface {
			busyWords[word] = true
		}
	}
	return busyWords
}

// extractTweets extracts tweets from the cluster map
func extractTweets(clusterMap map[string]interface{}) []types.Tweet {
	var tweets []types.Tweet
	if tweetsInterface, ok := clusterMap["tweets"].([]interface{}); ok {
		for _, tweetInterface := range tweetsInterface {
			if tweetMap, ok := tweetInterface.(map[string]interface{}); ok {
				tweet := types.Tweet{
					Text:     getString(tweetMap, "text"),
					IsMedoid: getBool(tweetMap, "is_medoid"),
				}
				tweets = append(tweets, tweet)
			}
		}
	}
	return tweets
}

// extractOptionalFields extracts optional fields from the cluster map
func extractOptionalFields(clusterMap map[string]interface{}) (string, string, float64, map[string]interface{}) {
	medoid, _ := clusterMap["medoid"].(string)
	medoidTweet, _ := clusterMap["medoid_tweet"].(string)
	qualityScore, _ := clusterMap["quality_score"].(float64)
	busyWordClasses, _ := clusterMap["busy_word_classes"].(map[string]interface{})

	return medoid, medoidTweet, qualityScore, busyWordClasses
}

// Helper functions for safe type conversion
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

func getInt(m map[string]interface{}, key string) int {
	if val, ok := m[key].(float64); ok {
		return int(val)
	}
	return 0
}

func getInt64(m map[string]interface{}, key string) int64 {
	if val, ok := m[key].(float64); ok {
		return int64(val)
	}
	return 0
}

func getBool(m map[string]interface{}, key string) bool {
	if val, ok := m[key].(bool); ok {
		return val
	}
	return false
}

// convertBusyWordsToTypes converts old format busy words to new format
func convertBusyWordsToTypes(oldBusyWords map[string]bool) []types.BusyWord {
	var busyWords []types.BusyWord
	for word := range oldBusyWords {
		busyWords = append(busyWords, types.BusyWord{
			Word:   word,
			Class:  0, // Default class, will need to be extracted from JSON
			ZScore: 0, // Default z-score, will need to be extracted from JSON
			Count:  0, // Default count, will need to be extracted from JSON
			Mean:   0, // Default mean, will need to be extracted from JSON
		})
	}
	return busyWords
}

// convertTweetsToTypes converts old format tweets to new format
func convertTweetsToTypes(oldTweets []types.Tweet) []types.Tweet {
	var tweets []types.Tweet
	for _, oldTweet := range oldTweets {
		tweets = append(tweets, types.Tweet{
			Text:     oldTweet.Text,
			IsMedoid: false, // Default, will need to be determined from context
		})
	}
	return tweets
}

// convertBusyWordClassesToTypes converts old format busy word classes to new format
func convertBusyWordClassesToTypes(oldClasses map[string]interface{}) map[string]int {
	newClasses := make(map[string]int)
	for word, classInterface := range oldClasses {
		if classInt, ok := classInterface.(int); ok {
			newClasses[word] = classInt
		} else if classFloat, ok := classInterface.(float64); ok {
			newClasses[word] = int(classFloat)
		}
	}
	return newClasses
}

// LoadInitialData loads the first few chunks to get initial data
func (p *Parser) LoadInitialData(chunkCount int) ([]types.Batch, error) {
	var allBatches []types.Batch

	for i := 0; i < chunkCount; i++ {
		batches, err := p.LoadNextChunk()
		if err != nil {
			return nil, err
		}
		if len(batches) == 0 {
			break // Reached end of file
		}
		allBatches = append(allBatches, batches...)
	}

	return allBatches, nil
}
