package json_parser

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Batch represents a single batch from the JSON data
type Batch struct {
	Type string `json:"type"`
	Data struct {
		BatchNumber          int         `json:"batch_number"`
		BatchTime            string      `json:"batch_time"`
		Method               string      `json:"method"`
		TotalTweets          int         `json:"total_tweets"`
		TotalClusters        int         `json:"total_clusters"`
		ClustersAboveMinSize int         `json:"clusters_above_min_size"`
		Clusters             interface{} `json:"clusters"`
	} `json:"data"`
}

// Cluster represents a parsed cluster with its data
type Cluster struct {
	ClusterID       int                    `json:"cluster_id"`
	Size            int                    `json:"size"`
	BusyWords       []string               `json:"busy_words"`
	TweetTexts      []string               `json:"tweet_texts"`
	Medoid          string                 `json:"medoid,omitempty"`       // Primary medoid field
	MedoidTweet     string                 `json:"medoid_tweet,omitempty"` // Alternative medoid field (will be merged with Medoid)
	QualityScore    float64                `json:"quality_score,omitempty"`
	BusyWordClasses map[string]interface{} `json:"busy_word_classes,omitempty"` // Frequency class mappings
}

// GetMedoidText returns the medoid text, preferring MedoidTweet over Medoid
func (c *Cluster) GetMedoidText() string {
	if c.MedoidTweet != "" {
		return c.MedoidTweet
	}
	return c.Medoid
}

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
func (p *Parser) LoadNextChunk() ([]Batch, error) {
	if p.decoder == nil {
		return nil, fmt.Errorf("decoder not initialized")
	}

	var batch Batch
	err := p.decoder.Decode(&batch)
	if err != nil {
		if err == io.EOF {
			// End of file
			return nil, nil
		}
		return nil, fmt.Errorf("failed to decode JSON: %v", err)
	}

	return []Batch{batch}, nil
}

// LoadNextChunkContinuous reads the next batch and waits for new data if at end of file
func (p *Parser) LoadNextChunkContinuous() ([]Batch, error) {
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
func ParseClusters(batch Batch) ([]Cluster, error) {
	// Handle null clusters (empty batches)
	if batch.Data.Clusters == nil {
		return []Cluster{}, nil
	}

	// Type assert clusters to []interface{} first, then to map[string]interface{}
	clustersInterface, ok := batch.Data.Clusters.([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid clusters data format")
	}

	var clusters []Cluster
	for i, clusterInterface := range clustersInterface {
		clusterMap, ok := clusterInterface.(map[string]interface{})
		if !ok {
			fmt.Printf("Cluster %d: Failed to convert to map\n", i)
			continue
		}

		// Extract cluster data
		clusterID, _ := clusterMap["cluster_id"].(float64)
		size, _ := clusterMap["size"].(float64)

		// Extract busy words
		busyWordsInterface, _ := clusterMap["busy_words"].([]interface{})
		var busyWords []string
		for _, word := range busyWordsInterface {
			if wordStr, ok := word.(string); ok {
				busyWords = append(busyWords, wordStr)
			}
		}

		// Extract tweet texts
		tweetTextsInterface, _ := clusterMap["tweet_texts"].([]interface{})
		var tweetTexts []string
		for _, tweet := range tweetTextsInterface {
			if tweetStr, ok := tweet.(string); ok {
				tweetTexts = append(tweetTexts, tweetStr)
			}
		}

		// Extract medoid if present
		medoid, _ := clusterMap["medoid"].(string)
		medoidTweet, _ := clusterMap["medoid_tweet"].(string)

		// Extract quality score if present
		qualityScore, _ := clusterMap["quality_score"].(float64)

		// Extract busy word classes if present
		busyWordClasses, _ := clusterMap["busy_word_classes"].(map[string]interface{})

		cluster := Cluster{
			ClusterID:       int(clusterID),
			Size:            int(size),
			BusyWords:       busyWords,
			TweetTexts:      tweetTexts,
			Medoid:          medoid,
			MedoidTweet:     medoidTweet,
			QualityScore:    qualityScore,
			BusyWordClasses: busyWordClasses,
		}
		clusters = append(clusters, cluster)
	}

	return clusters, nil
}

// LoadInitialData loads the first few chunks to get initial data
func (p *Parser) LoadInitialData(chunkCount int) ([]Batch, error) {
	var allBatches []Batch

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
