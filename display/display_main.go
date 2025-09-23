package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// BusyWord represents a single busy word with its statistical data
type BusyWord struct {
	Word   string  `json:"word"`
	Class  int     `json:"class"`
	ZScore float64 `json:"z_score"`
	Count  int     `json:"count"`
	Mean   float64 `json:"mean"`
}

// Helper function to get keys from a map
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Config holds the server configuration
type Config struct {
	InputFile           string  `yaml:"input_file"`
	BatchSize           int     `yaml:"batch_size"`
	HistoricalBatches   int     `yaml:"historical_batches"`
	MinClusterSize      int     `yaml:"min_cluster_size"`
	RecurrenceThreshold float64 `yaml:"recurrence_threshold"`
	RecurrenceStrategy  string  `yaml:"recurrence_strategy"`
	BubbleBatches       int     `yaml:"bubble_batches"`
	BubbleColor         string  `yaml:"bubble_color"` // Base color for bubbles (e.g., "blue", "green", "purple")
}

// calculateNormalizedLevenshtein calculates the normalized Levenshtein distance between two strings
func calculateNormalizedLevenshtein(s1, s2 string) float64 {
	if s1 == s2 {
		return 0.0
	}

	len1, len2 := len(s1), len(s2)
	if len1 == 0 {
		return 1.0
	}
	if len2 == 0 {
		return 1.0
	}

	// Calculate Levenshtein distance
	matrix := make([][]int, len1+1)
	for i := range matrix {
		matrix[i] = make([]int, len2+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	for i := 1; i <= len1; i++ {
		for j := 1; j <= len2; j++ {
			if s1[i-1] == s2[j-1] {
				matrix[i][j] = matrix[i-1][j-1]
			} else {
				matrix[i][j] = min(min(matrix[i-1][j]+1, matrix[i][j-1]+1), matrix[i-1][j-1]+1)
			}
		}
	}

	ld := matrix[len1][len2]
	maxLen := max(len1, len2)

	// Normalize by maximum length
	return float64(ld) / float64(maxLen)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// calculateQualityScore computes a quality score for a cluster based on persistence, recurrence, size, and tweet count
func calculateQualityScore(historicalData map[string]string, recurrenceData map[string]bool, clusterSize int, maxClusterSize int, tweetCount int, maxTweetCount int, currentMedoid string, historicalBatches []int) float64 {
	// Persistence: percentage of historical batches where cluster appears
	appearances := 0
	for _, value := range historicalData {
		if value != "" {
			appearances++
		}
	}
	persistence := float64(appearances) / float64(len(historicalBatches))

	// Recurrence Strength: average similarity of recurring instances
	recurrenceCount := 0
	totalSimilarity := 0.0
	for _, isRecurrence := range recurrenceData {
		if isRecurrence {
			recurrenceCount++
			// For now, use a default similarity score of 0.7 for detected recurrences
			// In a more sophisticated version, we could store the actual similarity scores
			totalSimilarity += 0.7
		}
	}
	recurrenceStrength := 0.0
	if recurrenceCount > 0 {
		recurrenceStrength = totalSimilarity / float64(recurrenceCount)
	}

	// Size Weight: normalized cluster size
	sizeWeight := 0.0
	if maxClusterSize > 0 {
		sizeWeight = float64(clusterSize) / float64(maxClusterSize)
	}

	// Tweet Count Weight: logarithmic normalization
	tweetWeight := 0.0
	if maxTweetCount > 0 && tweetCount > 0 {
		// Use log scale: log(tweetCount) / log(maxTweetCount)
		// This gives diminishing returns for very large tweet counts
		tweetWeight = math.Log(float64(tweetCount)) / math.Log(float64(maxTweetCount))
	}

	// Consistency: how evenly distributed the recurrences are
	consistency := 1.0
	if recurrenceCount > 1 {
		// Simple consistency: if recurrences are spread across multiple batches, higher consistency
		consistency = float64(recurrenceCount) / float64(len(historicalBatches))
	}

	// Weighted composite score (adjusted weights to include tweet count)
	qualityScore := (0.35 * persistence) + (0.25 * recurrenceStrength) + (0.15 * sizeWeight) + (0.20 * tweetWeight) + (0.05 * consistency)

	return qualityScore
}

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

// PageData holds data for the HTML template
type PageData struct {
	CurrentBatch int
	Batch        Batch
	HasNext      bool
	HasPrev      bool
	BatchInfo    string
}

var (
	config      Config
	allBatches  []Batch
	templates   *template.Template
	fileHandle  *os.File
	fileOffset  int64
	partialJSON string // Store partial JSON data between chunks
)

// Global variables for batch navigation
var currentBatchIndex int = 0
var maxBatchesInMemory = 200 // Keep only last 200 batches in memory

// getNextBatch returns the next batch in the sequence, loading more if needed
func getNextBatch() *Batch {
	// If we can get current batch, return it and increment
	if currentBatchIndex < len(allBatches) {
		result := &allBatches[currentBatchIndex]
		currentBatchIndex++
		return result
	}

	// Try to load more batches from file
	fmt.Printf("Loading more batches from file...\n")
	if err := loadMoreBatches(); err != nil {
		fmt.Printf("Failed to load more batches: %v\n", err)
		return nil
	}

	// If we loaded more batches, return current and increment
	if currentBatchIndex < len(allBatches) {
		result := &allBatches[currentBatchIndex]
		currentBatchIndex++
		return result
	}

	return nil // No more batches available
}

// loadMoreBatches loads another 100 batches from the file
func loadMoreBatches() error {
	// Load one more chunk (which contains multiple batches)
	oldCount := len(allBatches)
	if err := loadNextChunk(); err != nil {
		if err == io.EOF {
			fmt.Printf("Reached end of file, no more batches\n")
			return err
		}
		return err
	}

	newCount := len(allBatches)
	loadedCount := newCount - oldCount
	fmt.Printf("Loaded %d more batches, total: %d\n", loadedCount, len(allBatches))

	// Implement sliding window: discard old batches if we exceed limit
	if len(allBatches) > maxBatchesInMemory {
		discardCount := len(allBatches) - maxBatchesInMemory
		allBatches = allBatches[discardCount:]
		currentBatchIndex -= discardCount
		fmt.Printf("Discarded %d old batches, kept %d, current index: %d\n",
			discardCount, len(allBatches), currentBatchIndex)
	}

	return nil
}

// getPreviousBatch returns the previous batch in the sequence
func getPreviousBatch() *Batch {
	if currentBatchIndex > 0 {
		currentBatchIndex--
		return &allBatches[currentBatchIndex]
	}
	return nil // No previous batches
}

// Navigation handler functions
func handleNext(w http.ResponseWriter, r *http.Request) {
	batch := getNextBatch()
	if batch == nil {
		http.Error(w, "No more batches available", http.StatusNotFound)
		return
	}

	// Return JSON response with batch info
	response := map[string]interface{}{
		"batch_number":   batch.Data.BatchNumber,
		"batch_time":     batch.Data.BatchTime,
		"total_tweets":   batch.Data.TotalTweets,
		"total_clusters": batch.Data.TotalClusters,
		"current_index":  currentBatchIndex,
		"total_loaded":   len(allBatches),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handlePrevious(w http.ResponseWriter, r *http.Request) {
	batch := getPreviousBatch()
	if batch == nil {
		http.Error(w, "No previous batches available", http.StatusNotFound)
		return
	}

	// Return JSON response with batch info
	response := map[string]interface{}{
		"batch_number":   batch.Data.BatchNumber,
		"batch_time":     batch.Data.BatchTime,
		"total_tweets":   batch.Data.TotalTweets,
		"total_clusters": batch.Data.TotalClusters,
		"current_index":  currentBatchIndex,
		"total_loaded":   len(allBatches),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleCurrent(w http.ResponseWriter, r *http.Request) {
	batch := getCurrentBatch()
	if batch == nil {
		http.Error(w, "No current batch available", http.StatusNotFound)
		return
	}

	// Return JSON response with batch info
	response := map[string]interface{}{
		"batch_number":   batch.Data.BatchNumber,
		"batch_time":     batch.Data.BatchTime,
		"total_tweets":   batch.Data.TotalTweets,
		"total_clusters": batch.Data.TotalClusters,
		"current_index":  currentBatchIndex,
		"total_loaded":   len(allBatches),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// getCurrentBatch returns the current batch
func getCurrentBatch() *Batch {
	if currentBatchIndex < len(allBatches) {
		return &allBatches[currentBatchIndex]
	}
	return nil
}

func main() {
	// Load configuration
	if err := loadConfig(); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Load initial chunk of JSON data
	if err := loadData(); err != nil {
		log.Fatalf("Failed to load data: %v", err)
	}

	fmt.Printf("Loaded %d batches, starting web server...\n", len(allBatches))

	// Parse templates with custom functions
	templates = template.New("").Funcs(template.FuncMap{
		"add":      func(a, b int) int { return a + b },
		"subtract": func(a, b int) int { return a - b },
		"prettyPrint": func(v interface{}) string {
			data, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				return fmt.Sprintf("Error formatting: %v", err)
			}
			return string(data)
		},
	})

	var err error
	templates, err = templates.ParseGlob("templates/*.html")
	if err != nil {
		log.Fatalf("Failed to parse templates: %v", err)
	}

	// Set up HTTP routes
	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/batch/", handleBatch)
	http.HandleFunc("/api/chart-data/", handleChartData)
	http.HandleFunc("/grid-text/", handleGridText)
	http.HandleFunc("/grid", handleGridDefault) // Default grid route
	http.HandleFunc("/grid/", handleGrid)       // Grid with batch number
	http.HandleFunc("/api/grid-data", handleGridDataAPI)
	http.HandleFunc("/medoid", handleMedoidDefault) // Default medoid route
	http.HandleFunc("/medoid/", handleMedoid)       // Medoid with batch number
	http.HandleFunc("/api/medoid-data", handleMedoidDataAPI)
	http.HandleFunc("/api/cluster-data/", handleClusterDataAPI)
	http.HandleFunc("/bubbles", handleBubblesDefault) // Default bubbles route
	http.HandleFunc("/bubbles/", handleBubbles)       // Bubbles with batch number
	http.HandleFunc("/api/bubble-data/", handleBubbleDataAPI)

	// Navigation endpoints
	http.HandleFunc("/api/next", handleNext)
	http.HandleFunc("/api/previous", handlePrevious)
	http.HandleFunc("/api/current", handleCurrent)

	fmt.Printf("Starting server on http://localhost:8080\n")
	fmt.Printf("Loaded %d batches from initial chunk of %s (chunked loading enabled)\n", len(allBatches), config.InputFile)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func loadConfig() error {
	content, err := os.ReadFile("config.yaml")
	if err != nil {
		return fmt.Errorf("failed to read config file: %v", err)
	}

	if err := yaml.Unmarshal(content, &config); err != nil {
		return fmt.Errorf("failed to parse config: %v", err)
	}

	return nil
}

func loadData() error {
	// Open file for reading
	file, err := os.Open(config.InputFile)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %v", config.InputFile, err)
	}
	fileHandle = file
	fileOffset = 0

	// Load initial chunks to get the desired number of batches
	fmt.Printf("Loading initial data to get %d batches...\n", config.BatchSize)
	chunkCount := 0
	for len(allBatches) < config.BatchSize {
		if err := loadNextChunk(); err != nil {
			if strings.Contains(err.Error(), "end of file") {
				fmt.Printf("Reached end of file after loading %d batches\n", len(allBatches))
				break // Reached end of file
			}
			return err
		}
		chunkCount++
		fmt.Printf("Loaded chunk %d, now have %d batches\n", chunkCount, len(allBatches))
	}

	fmt.Printf("Initial load complete: %d batches loaded\n", len(allBatches))
	return nil
}

func loadNextChunk() error {
	if fileHandle == nil {
		return fmt.Errorf("file not open")
	}

	fmt.Printf("loadNextChunk: reading chunk at offset %d\n", fileOffset)
	// Read 50MB chunk to get more batches
	chunk := make([]byte, 50*1024*1024) // 50MB
	n, err := fileHandle.ReadAt(chunk, fileOffset)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read chunk: %v", err)
	}

	fmt.Printf("loadNextChunk: read %d bytes\n", n)
	if n == 0 {
		// End of file
		fmt.Printf("loadNextChunk: end of file reached\n")
		return nil
	}

	// Parse complete JSON objects in this chunk
	contentStr := partialJSON + string(chunk[:n])
	pos := 0
	batchesInChunk := 0

	for pos < len(contentStr) {
		// Find the start of a JSON object
		start := strings.Index(contentStr[pos:], "{")
		if start == -1 {
			fmt.Printf("No more JSON objects found, pos: %d, contentStr length: %d\n", pos, len(contentStr))
			break
		}
		start += pos
		fmt.Printf("Found JSON object at position %d\n", start)

		// Find the matching closing brace
		braceCount := 0
		end := start
		for i := start; i < len(contentStr); i++ {
			if contentStr[i] == '{' {
				braceCount++
			} else if contentStr[i] == '}' {
				braceCount--
				if braceCount == 0 {
					end = i + 1
					break
				}
			}
		}

		if braceCount == 0 {
			// We found a complete JSON object
			jsonStr := contentStr[start:end]
			var batch Batch
			if err := json.Unmarshal([]byte(jsonStr), &batch); err == nil {
				allBatches = append(allBatches, batch)
				batchesInChunk++
				fmt.Printf("Successfully parsed batch %d\n", len(allBatches))
			} else {
				fmt.Printf("Failed to parse JSON object: %v\n", err)
				previewLen := 200
				if len(jsonStr) < previewLen {
					previewLen = len(jsonStr)
				}
				fmt.Printf("JSON length: %d, first %d chars: %s\n", len(jsonStr), previewLen, jsonStr[:previewLen])
			}
			pos = end
		} else {
			// Incomplete JSON object, save the partial data for next chunk
			partialJSON = contentStr[start:]
			break
		}
	}

	// Update file offset for next chunk
	fileOffset += int64(n)

	fmt.Printf("Loaded %d batches from chunk (total: %d)\n", batchesInChunk, len(allBatches))

	// If we read some data but didn't parse any batches, and we're at EOF, return an error
	if batchesInChunk == 0 && err == io.EOF {
		return fmt.Errorf("end of file reached, no more batches")
	}

	return nil
}

// loadMoreChunks loads additional chunks if needed for a specific batch number
func loadMoreChunks(requiredBatch int) error {
	fmt.Printf("loadMoreChunks: requested batch %d, currently have %d batches\n", requiredBatch, len(allBatches))

	// Calculate how many chunks we need to load
	chunksNeeded := (requiredBatch-len(allBatches))/300 + 1 // Rough estimate: ~300 batches per chunk
	if chunksNeeded > 3 {
		chunksNeeded = 3 // Cap at 3 chunks at a time to avoid long delays
	}

	fmt.Printf("loadMoreChunks: loading %d chunks to reach batch %d\n", chunksNeeded, requiredBatch)

	for i := 0; i < chunksNeeded && len(allBatches) <= requiredBatch; i++ {
		fmt.Printf("loadMoreChunks: loading chunk %d/%d...\n", i+1, chunksNeeded)
		previousBatchCount := len(allBatches)
		if err := loadNextChunk(); err != nil {
			fmt.Printf("loadMoreChunks: error loading chunk: %v\n", err)
			return err
		}
		// If no new batches were loaded, we've reached the end
		if len(allBatches) == previousBatchCount {
			fmt.Printf("loadMoreChunks: no more batches available, reached end of file (total batches: %d)\n", len(allBatches))
			return fmt.Errorf("no more batches available")
		}
		fmt.Printf("loadMoreChunks: now have %d batches\n", len(allBatches))
	}

	// If we still need more chunks, load them in smaller increments
	for len(allBatches) <= requiredBatch {
		fmt.Printf("loadMoreChunks: loading additional chunk...\n")
		previousBatchCount := len(allBatches)
		if err := loadNextChunk(); err != nil {
			fmt.Printf("loadMoreChunks: error loading chunk: %v\n", err)
			return err
		}
		// If no new batches were loaded, we've reached the end
		if len(allBatches) == previousBatchCount {
			fmt.Printf("loadMoreChunks: no more batches available, reached end of file (total batches: %d)\n", len(allBatches))
			return fmt.Errorf("no more batches available")
		}
		fmt.Printf("loadMoreChunks: now have %d batches\n", len(allBatches))
	}
	return nil
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	// Redirect to grid display by default
	http.Redirect(w, r, "/grid", http.StatusSeeOther)
}

func handleGridDefault(w http.ResponseWriter, r *http.Request) {
	// Redirect to grid with batch 0
	http.Redirect(w, r, "/grid/0", http.StatusSeeOther)
}

func handleBatch(w http.ResponseWriter, r *http.Request) {
	// Extract batch number from URL
	path := r.URL.Path
	batchNumStr := path[len("/batch/"):]

	batchNum, err := strconv.Atoi(batchNumStr)
	if err != nil {
		http.Error(w, "Invalid batch number", http.StatusBadRequest)
		return
	}

	if batchNum < 0 || batchNum >= len(allBatches) {
		http.Error(w, "Batch number out of range", http.StatusBadRequest)
		return
	}

	// Get the specific batch
	batch := allBatches[batchNum]

	// Prepare data for template
	data := PageData{
		CurrentBatch: batchNum,
		Batch:        batch,
		HasNext:      batchNum < len(allBatches)-1,
		HasPrev:      batchNum > 0,
		BatchInfo: fmt.Sprintf("Batch %d (Batch %d, %s)",
			batchNum+1, batch.Data.BatchNumber, batch.Data.BatchTime),
	}

	templates.ExecuteTemplate(w, "index.html", data)
}

// ChartData represents the data structure for the chart
type ChartData struct {
	Clusters []ClusterData `json:"clusters"`
}

// ClusterData represents a single cluster for the chart
type ClusterData struct {
	ClusterID   int      `json:"cluster_id"`
	Size        int      `json:"size"`
	BusyWords   []string `json:"busy_words"`
	WordCounts  []int    `json:"word_counts"`
	TotalTweets int      `json:"total_tweets"`
}

// GridRow represents a single row in the busy word grid
type GridRow struct {
	Word           string            `json:"word"`
	ClusterID      int               `json:"cluster_id"`
	ClusterSize    int               `json:"cluster_size"`
	QualityScore   float64           `json:"quality_score"`
	HistoricalData map[string]string `json:"historical_data"` // batch_number -> word or empty
	RecurrenceData map[string]bool   `json:"recurrence_data"` // batch_number -> true if similar medoid found
}

// TweetData represents tweet information for display
type TweetData struct {
	Text     string `json:"text"`
	IsMedoid bool   `json:"is_medoid"`
}

// ClusterTweetData represents tweet data for a cluster
type ClusterTweetData struct {
	ClusterID int         `json:"cluster_id"`
	Size      int         `json:"size"`
	BusyWords []string    `json:"busy_words"`
	Tweets    []TweetData `json:"tweets"`
}

// GridData represents the complete grid for display
type GridData struct {
	CurrentBatch      int                `json:"current_batch"`
	BatchTime         string             `json:"batch_time"`
	BatchDuration     string             `json:"batch_duration"`
	HistoricalBatches []int              `json:"historical_batches"`
	Rows              []GridRow          `json:"rows"`
	MinClusterSize    int                `json:"min_cluster_size"`
	TweetData         []ClusterTweetData `json:"tweet_data"`
}

func handleChartData(w http.ResponseWriter, r *http.Request) {
	// Extract batch number from URL
	path := r.URL.Path
	batchNumStr := path[len("/api/chart-data/"):]

	batchNum, err := strconv.Atoi(batchNumStr)
	if err != nil {
		http.Error(w, "Invalid batch number", http.StatusBadRequest)
		return
	}

	if batchNum < 0 || batchNum >= len(allBatches) {
		http.Error(w, "Batch number out of range", http.StatusBadRequest)
		return
	}

	batch := allBatches[batchNum]

	// Convert batch data to chart format
	var chartData ChartData

	// Type assert clusters to []interface{} first, then to map[string]interface{}
	clustersInterface, ok := batch.Data.Clusters.([]interface{})
	if !ok {
		http.Error(w, "Invalid clusters data format", http.StatusInternalServerError)
		return
	}

	fmt.Printf("Processing %d clusters for chart data\n", len(clustersInterface))

	for i, clusterInterface := range clustersInterface {
		clusterMap, ok := clusterInterface.(map[string]interface{})
		if !ok {
			fmt.Printf("Cluster %d: Failed to convert to map\n", i)
			continue
		}

		// Extract cluster data
		clusterID, _ := clusterMap["cluster_id"].(float64)
		size, _ := clusterMap["size"].(float64)

		fmt.Printf("Cluster %d: ID=%v, Size=%v\n", i, clusterID, size)

		// Extract busy words - handle new map structure
		var busyWords []string
		if busyWordsMap, ok := clusterMap["busy_words"].(map[string]interface{}); ok {
			// New format: busy_words is a map
			for word := range busyWordsMap {
				busyWords = append(busyWords, word)
			}
		} else if busyWordsInterface, ok := clusterMap["busy_words"].([]interface{}); ok {
			// Old format: busy_words is an array (fallback)
			for _, word := range busyWordsInterface {
				if wordStr, ok := word.(string); ok {
					busyWords = append(busyWords, wordStr)
				}
			}
		}

		fmt.Printf("Cluster %d: Found %d busy words: %v\n", i, len(busyWords), busyWords)

		// Extract tweet texts - handle new tweets structure
		var tweetTexts []string
		if tweetsInterface, ok := clusterMap["tweets"].([]interface{}); ok {
			// New format: tweets is an array of objects
			for _, tweetInterface := range tweetsInterface {
				if tweetMap, ok := tweetInterface.(map[string]interface{}); ok {
					if text, ok := tweetMap["text"].(string); ok {
						tweetTexts = append(tweetTexts, text)
					}
				}
			}
		} else if tweetTextsInterface, ok := clusterMap["tweet_texts"].([]interface{}); ok {
			// Old format: tweet_texts is an array of strings (fallback)
			for _, tweet := range tweetTextsInterface {
				if tweetStr, ok := tweet.(string); ok {
					tweetTexts = append(tweetTexts, tweetStr)
				}
			}
		}

		// Count occurrences of each busy word in tweet texts
		wordCounts := make([]int, len(busyWords))
		for i, word := range busyWords {
			count := 0
			for _, tweet := range tweetTexts {
				if strings.Contains(strings.ToLower(tweet), strings.ToLower(word)) {
					count++
				}
			}
			wordCounts[i] = count
		}

		clusterData := ClusterData{
			ClusterID:   int(clusterID),
			Size:        int(size),
			BusyWords:   busyWords,
			WordCounts:  wordCounts,
			TotalTweets: int(size), // This is the actual tweet count from the cluster
		}
		chartData.Clusters = append(chartData.Clusters, clusterData)
		fmt.Printf("Cluster %d: Added to chart with %d word counts: %v\n", i, len(wordCounts), wordCounts)
	}

	fmt.Printf("Final chart data: %d clusters\n", len(chartData.Clusters))

	// Set response headers
	w.Header().Set("Content-Type", "application/json")

	// Encode and send response
	if err := json.NewEncoder(w).Encode(chartData); err != nil {
		http.Error(w, "JSON encoding error", http.StatusInternalServerError)
		return
	}
}

// hasDataForBatch checks if a batch has any clusters that meet the minimum size requirement
func hasDataForBatch(batchIndex int, minClusterSize int) bool {
	if batchIndex < 0 {
		return false
	}

	fmt.Printf("hasDataForBatch: checking batch %d, currently have %d batches\n", batchIndex, len(allBatches))
	// Load more chunks if needed
	if batchIndex >= len(allBatches) {
		fmt.Printf("hasDataForBatch: batch %d >= %d, loading more chunks\n", batchIndex, len(allBatches))
		if err := loadMoreChunks(batchIndex); err != nil {
			if strings.Contains(err.Error(), "no more batches available") {
				fmt.Printf("hasDataForBatch: reached end of input file at batch %d (total batches: %d)\n", batchIndex, len(allBatches))
			} else {
				fmt.Printf("hasDataForBatch: actual error loading chunks: %v\n", err)
			}
			return false
		}
	}

	if batchIndex >= len(allBatches) {
		fmt.Printf("hasDataForBatch: still no data for batch %d\n", batchIndex)
		return false
	}

	batch := allBatches[batchIndex]
	clustersInterface, ok := batch.Data.Clusters.([]interface{})
	if !ok {
		fmt.Printf("hasDataForBatch: batch %d - clusters not a []interface{}\n", batchIndex)
		return false
	}

	fmt.Printf("hasDataForBatch: batch %d has %d clusters\n", batchIndex, len(clustersInterface))
	for i, clusterInterface := range clustersInterface {
		clusterMap, ok := clusterInterface.(map[string]interface{})
		if !ok {
			fmt.Printf("hasDataForBatch: batch %d cluster %d not a map\n", batchIndex, i)
			continue
		}

		size, _ := clusterMap["size"].(float64)
		fmt.Printf("hasDataForBatch: batch %d cluster %d size %v (min required: %d)\n", batchIndex, i, size, minClusterSize)
		if int(size) >= minClusterSize {
			fmt.Printf("hasDataForBatch: batch %d cluster %d meets minimum size, returning true\n", batchIndex, i)
			return true
		}
	}

	fmt.Printf("hasDataForBatch: batch %d no clusters meet minimum size, returning false\n", batchIndex)
	return false
}

// stripTimestamp removes the timestamp prefix from tweet text
func stripTimestamp(text string) string {
	// Look for timestamp pattern like "[2012-01-28 21:43:34 UTC] "
	if len(text) > 0 && text[0] == '[' {
		// Find the closing bracket and space
		endBracket := strings.Index(text, "] ")
		if endBracket > 0 {
			// Check if what follows looks like a timestamp (contains UTC and date format)
			timestampPart := text[1:endBracket]
			if strings.Contains(timestampPart, "UTC") && strings.Contains(timestampPart, "-") {
				// Return everything after "] "
				return text[endBracket+2:]
			}
		}
	}
	return text
}

// findNextBatchWithData finds the next batch (starting from startIndex) that has data
func findNextBatchWithData(startIndex int, direction int, minClusterSize int) int {
	if direction == 0 {
		return startIndex
	}

	currentIndex := startIndex
	visited := make(map[int]bool) // Track visited indices to prevent infinite loops

	fmt.Printf("findNextBatchWithData: starting from %d, direction %d, minClusterSize %d\n", startIndex, direction, minClusterSize)

	for {
		currentIndex += direction
		fmt.Printf("findNextBatchWithData: checking batch %d\n", currentIndex)

		if currentIndex < 0 {
			fmt.Printf("findNextBatchWithData: batch %d < 0, returning original %d\n", currentIndex, startIndex)
			return startIndex // Return original if we can't find any
		}

		// Prevent infinite loops
		if visited[currentIndex] {
			fmt.Printf("findNextBatchWithData: already visited batch %d, returning original %d\n", currentIndex, startIndex)
			return startIndex // Return original if we've already visited this index
		}
		visited[currentIndex] = true

		// Load more chunks if needed
		if currentIndex >= len(allBatches) {
			fmt.Printf("findNextBatchWithData: batch %d >= %d, loading more chunks\n", currentIndex, len(allBatches))
			if err := loadMoreChunks(currentIndex); err != nil {
				fmt.Printf("findNextBatchWithData: error loading chunks for batch %d: %v\n", currentIndex, err)
				return startIndex // Return original if we can't load more
			}
		}

		if currentIndex >= len(allBatches) {
			fmt.Printf("findNextBatchWithData: batch %d >= %d after loading, returning original %d\n", currentIndex, len(allBatches), startIndex)
			return startIndex // Return original if we can't find any
		}

		fmt.Printf("findNextBatchWithData: checking if batch %d has data\n", currentIndex)
		if hasDataForBatch(currentIndex, minClusterSize) {
			fmt.Printf("findNextBatchWithData: batch %d has data, returning it\n", currentIndex)
			return currentIndex
		} else {
			fmt.Printf("findNextBatchWithData: batch %d has no data, continuing\n", currentIndex)
		}
	}
}

// computeGridData creates the grid data structure for the busy word display
func computeGridData(currentBatchIndex int, historicalBatches int, minClusterSize int) GridData {
	fmt.Printf("computeGridData: called for batch %d, currently have %d batches\n", currentBatchIndex, len(allBatches))
	if currentBatchIndex < 0 {
		return GridData{}
	}

	// Load more chunks if needed
	if currentBatchIndex >= len(allBatches) {
		fmt.Printf("computeGridData: batch %d >= %d, loading more chunks\n", currentBatchIndex, len(allBatches))
		if err := loadMoreChunks(currentBatchIndex); err != nil {
			fmt.Printf("computeGridData: error loading chunks: %v\n", err)
			return GridData{}
		}
	}

	if currentBatchIndex >= len(allBatches) {
		fmt.Printf("computeGridData: still no data for batch %d\n", currentBatchIndex)
		return GridData{}
	}

	currentBatch := allBatches[currentBatchIndex]

	// Extract clusters from current batch
	clustersInterface, ok := currentBatch.Data.Clusters.([]interface{})
	if !ok {
		fmt.Printf("computeGridData: batch %d - clusters not a []interface{}\n", currentBatchIndex)
		return GridData{}
	}

	fmt.Printf("computeGridData: batch %d has %d clusters\n", currentBatchIndex, len(clustersInterface))

	// Convert to typed clusters and filter by size
	var clusters []map[string]interface{}
	for i, clusterInterface := range clustersInterface {
		clusterMap, ok := clusterInterface.(map[string]interface{})
		if !ok {
			fmt.Printf("computeGridData: batch %d cluster %d not a map\n", currentBatchIndex, i)
			continue
		}

		size, _ := clusterMap["size"].(float64)
		fmt.Printf("computeGridData: batch %d cluster %d size %v (min required: %d)\n", currentBatchIndex, i, size, minClusterSize)
		if int(size) >= minClusterSize {
			fmt.Printf("computeGridData: batch %d cluster %d meets minimum size, adding to clusters\n", currentBatchIndex, i)
			clusters = append(clusters, clusterMap)
		} else {
			fmt.Printf("computeGridData: batch %d cluster %d below minimum size, skipping\n", currentBatchIndex, i)
		}
	}

	fmt.Printf("computeGridData: batch %d filtered to %d clusters >= min size\n", currentBatchIndex, len(clusters))

	// Sort clusters by size (largest first)
	sort.Slice(clusters, func(i, j int) bool {
		sizeI, _ := clusters[i]["size"].(float64)
		sizeJ, _ := clusters[j]["size"].(float64)
		return sizeI > sizeJ
	})

	// Build grid rows from current batch clusters
	var gridRows []GridRow
	var clusterTweetData []ClusterTweetData

	fmt.Printf("computeGridData: processing %d clusters\n", len(clusters))
	for i, cluster := range clusters {
		fmt.Printf("computeGridData: processing cluster %d\n", i)
		clusterID, _ := cluster["cluster_id"].(float64)
		size, _ := cluster["size"].(float64)
		fmt.Printf("computeGridData: cluster %d ID=%v size=%v\n", i, clusterID, size)

		// Extract busy words from new struct format
		busyWordsInterface, ok := cluster["busy_words"].([]interface{})
		if !ok {
			fmt.Printf("computeGridData: cluster %d has no busy_words array\n", i)
			continue
		}

		var busyWords []string
		for _, wordInterface := range busyWordsInterface {
			wordObj, ok := wordInterface.(map[string]interface{})
			if !ok {
				continue
			}

			word, ok := wordObj["word"].(string)
			if !ok {
				continue
			}

			busyWords = append(busyWords, word)

			gridRow := GridRow{
				Word:           word,
				ClusterID:      int(clusterID),
				ClusterSize:    int(size),
				QualityScore:   0.0, // Will be calculated after historical data is filled
				HistoricalData: make(map[string]string),
			}
			gridRows = append(gridRows, gridRow)
		}

		// Extract tweet data
		var tweets []TweetData

		// Get medoid tweet
		medoidInterface, _ := cluster["medoid_tweet"].(string)
		if medoidInterface != "" {
			// Strip timestamp from medoid tweet
			medoidText := stripTimestamp(medoidInterface)
			tweets = append(tweets, TweetData{
				Text:     medoidText,
				IsMedoid: true,
			})
		}

		// Get sample tweets (tweet_texts field)
		tweetTextsInterface, _ := cluster["tweet_texts"].([]interface{})
		var tweetTexts []string
		for _, tweetInterface := range tweetTextsInterface {
			if tweetStr, ok := tweetInterface.(string); ok {
				tweetTexts = append(tweetTexts, tweetStr)
			}
		}

		// Calculate how many tweets to show (N total, where N = number of busy words)
		numBusyWords := len(busyWords)
		numTweetsToShow := numBusyWords

		// If we have fewer tweets than busy words, show all available tweets
		if len(tweetTexts) < numTweetsToShow {
			numTweetsToShow = len(tweetTexts)
		}

		// Add sample tweets (skip the first one if it's the same as medoid)
		tweetsAdded := 0
		for _, tweetText := range tweetTexts {
			if tweetsAdded >= numTweetsToShow {
				break
			}

			// Skip if this tweet is the same as the medoid
			if medoidInterface != "" && tweetText == medoidInterface {
				continue
			}

			// Strip timestamp from sample tweet
			cleanTweetText := stripTimestamp(tweetText)
			tweets = append(tweets, TweetData{
				Text:     cleanTweetText,
				IsMedoid: false,
			})
			tweetsAdded++
		}

		// Add cluster tweet data
		clusterTweetData = append(clusterTweetData, ClusterTweetData{
			ClusterID: int(clusterID),
			Size:      int(size),
			BusyWords: busyWords,
			Tweets:    tweets,
		})
	}

	// Build historical batch numbers
	var historicalBatchNumbers []int
	for i := 1; i <= historicalBatches; i++ {
		historicalIndex := currentBatchIndex - i
		if historicalIndex >= 0 {
			historicalBatchNumbers = append(historicalBatchNumbers, historicalIndex)
		}
	}

	// Fill in historical data and recurrence data for each row
	for i := range gridRows {
		word := gridRows[i].Word
		clusterID := gridRows[i].ClusterID

		// Initialize recurrence data map
		gridRows[i].RecurrenceData = make(map[string]bool)

		// Get current cluster's medoid for comparison
		var currentMedoid string
		for _, cluster := range clusters {
			if int(cluster["cluster_id"].(float64)) == clusterID {
				if medoid, ok := cluster["medoid_tweet"].(string); ok {
					currentMedoid = stripTimestamp(medoid)
				} else {
					// Fallback: use first tweet if no medoid
					if tweetTexts, ok := cluster["tweet_texts"].([]interface{}); ok && len(tweetTexts) > 0 {
						if firstTweet, ok := tweetTexts[0].(string); ok {
							currentMedoid = stripTimestamp(firstTweet)
						}
					}
				}
				break
			}
		}

		for _, historicalIndex := range historicalBatchNumbers {
			if historicalIndex >= 0 && historicalIndex < len(allBatches) {
				historicalBatch := allBatches[historicalIndex]

				// Check if word appears in this historical batch
				historicalClustersInterface, ok := historicalBatch.Data.Clusters.([]interface{})
				if !ok {
					gridRows[i].HistoricalData[fmt.Sprintf("%d", historicalIndex)] = ""
					gridRows[i].RecurrenceData[fmt.Sprintf("%d", historicalIndex)] = false
					continue
				}

				found := false
				recurrenceFound := false

				for _, clusterInterface := range historicalClustersInterface {
					clusterMap, ok := clusterInterface.(map[string]interface{})
					if !ok {
						continue
					}

					// Handle new map structure for busy_words
					var busyWordsInterface []interface{}
					if busyWordsMap, ok := clusterMap["busy_words"].(map[string]interface{}); ok {
						// New format: convert map keys to array
						for word := range busyWordsMap {
							busyWordsInterface = append(busyWordsInterface, word)
						}
					} else {
						// Old format: already an array
						busyWordsInterface, _ = clusterMap["busy_words"].([]interface{})
					}
					for _, wordInterface := range busyWordsInterface {
						historicalWord, ok := wordInterface.(string)
						if ok && historicalWord == word {
							found = true

							// Check for recurrence by comparing tweets
							if currentMedoid != "" {
								if config.RecurrenceStrategy == "medoid_only" {
									// Strategy 1: Compare only medoids
									var historicalMedoidClean string
									if historicalMedoid, ok := clusterMap["medoid_tweet"].(string); ok {
										historicalMedoidClean = stripTimestamp(historicalMedoid)
									} else {
										// Fallback: use first tweet if no medoid
										if tweetTexts, ok := clusterMap["tweet_texts"].([]interface{}); ok && len(tweetTexts) > 0 {
											if firstTweet, ok := tweetTexts[0].(string); ok {
												historicalMedoidClean = stripTimestamp(firstTweet)
											}
										}
									}

									if historicalMedoidClean != "" {
										distance := calculateNormalizedLevenshtein(currentMedoid, historicalMedoidClean)
										if distance <= config.RecurrenceThreshold {
											recurrenceFound = true
										}
									}
								} else {
									// Strategy 2: Compare to all tweets (default)
									if tweetTexts, ok := clusterMap["tweet_texts"].([]interface{}); ok {
										fmt.Printf("DEBUG: Comparing current medoid '%s' to %d historical tweets\n", currentMedoid, len(tweetTexts))
										for i, tweetInterface := range tweetTexts {
											if historicalTweet, ok := tweetInterface.(string); ok {
												historicalTweetClean := stripTimestamp(historicalTweet)
												distance := calculateNormalizedLevenshtein(currentMedoid, historicalTweetClean)
												fmt.Printf("  Tweet %d: '%s' -> distance %.3f\n", i, historicalTweetClean, distance)
												if distance <= config.RecurrenceThreshold {
													recurrenceFound = true
													fmt.Printf("  RECURRENCE DETECTED! (distance %.3f <= %.3f)\n", distance, config.RecurrenceThreshold)
													break // Found a match, no need to check more tweets
												}
											}
										}
									}
								}
							}
							break
						}
					}
					if found {
						break
					}
				}

				if found {
					gridRows[i].HistoricalData[fmt.Sprintf("%d", historicalIndex)] = word
				} else {
					gridRows[i].HistoricalData[fmt.Sprintf("%d", historicalIndex)] = ""
				}
				gridRows[i].RecurrenceData[fmt.Sprintf("%d", historicalIndex)] = recurrenceFound
			}
		}
	}

	// Calculate quality scores for each row
	maxClusterSize := 0
	maxTweetCount := 0
	for _, row := range gridRows {
		if row.ClusterSize > maxClusterSize {
			maxClusterSize = row.ClusterSize
		}
		// Get tweet count for this cluster
		for _, cluster := range clusters {
			if int(cluster["cluster_id"].(float64)) == row.ClusterID {
				if tweetTexts, ok := cluster["tweet_texts"].([]interface{}); ok {
					tweetCount := len(tweetTexts)
					if tweetCount > maxTweetCount {
						maxTweetCount = tweetCount
					}
				}
				break
			}
		}
	}

	for i := range gridRows {
		// Get current medoid for this row's cluster
		var currentMedoid string
		for _, cluster := range clusters {
			if int(cluster["cluster_id"].(float64)) == gridRows[i].ClusterID {
				if medoid, ok := cluster["medoid_tweet"].(string); ok {
					currentMedoid = stripTimestamp(medoid)
				} else {
					// Fallback: use first tweet if no medoid
					if tweetTexts, ok := cluster["tweet_texts"].([]interface{}); ok && len(tweetTexts) > 0 {
						if firstTweet, ok := tweetTexts[0].(string); ok {
							currentMedoid = stripTimestamp(firstTweet)
						}
					}
				}
				break
			}
		}

		// Get tweet count for this row's cluster
		tweetCount := 0
		for _, cluster := range clusters {
			if int(cluster["cluster_id"].(float64)) == gridRows[i].ClusterID {
				if tweetTexts, ok := cluster["tweet_texts"].([]interface{}); ok {
					tweetCount = len(tweetTexts)
				}
				break
			}
		}

		gridRows[i].QualityScore = calculateQualityScore(
			gridRows[i].HistoricalData,
			gridRows[i].RecurrenceData,
			gridRows[i].ClusterSize,
			maxClusterSize,
			tweetCount,
			maxTweetCount,
			currentMedoid,
			historicalBatchNumbers,
		)
	}

	// Calculate batch duration if we have previous batch
	var batchDuration string
	if currentBatchIndex > 0 && currentBatchIndex < len(allBatches) {
		// For now, we'll just show a placeholder since we need to parse timestamps
		// In a real implementation, you'd parse the timestamps and calculate the difference
		batchDuration = "~10 seconds" // Placeholder
	} else {
		batchDuration = "N/A"
	}

	return GridData{
		CurrentBatch:      currentBatchIndex,
		BatchTime:         currentBatch.Data.BatchTime,
		BatchDuration:     batchDuration,
		HistoricalBatches: historicalBatchNumbers,
		Rows:              gridRows,
		MinClusterSize:    minClusterSize,
		TweetData:         clusterTweetData,
	}
}

func handleGridText(w http.ResponseWriter, r *http.Request) {
	// Extract batch number from URL
	path := r.URL.Path
	batchNumStr := path[len("/grid-text/"):]

	batchNum, err := strconv.Atoi(batchNumStr)
	if err != nil {
		http.Error(w, "Invalid batch number", http.StatusBadRequest)
		return
	}

	if batchNum < 0 || batchNum >= len(allBatches) {
		http.Error(w, "Batch number out of range", http.StatusBadRequest)
		return
	}

	// Compute grid data
	gridData := computeGridData(batchNum, config.HistoricalBatches, config.MinClusterSize)

	// Output as simple text
	w.Header().Set("Content-Type", "text/plain")

	fmt.Fprintf(w, "=== BUSY WORD GRID FOR BATCH %d ===\n", batchNum)
	fmt.Fprintf(w, "Min Cluster Size: %d\n", gridData.MinClusterSize)
	fmt.Fprintf(w, "Historical Batches: %v\n", gridData.HistoricalBatches)
	fmt.Fprintf(w, "Total Rows: %d\n\n", len(gridData.Rows))

	// Print header
	fmt.Fprintf(w, "%-20s %-10s %-10s", "Word", "ClusterID", "Size")
	for _, histBatch := range gridData.HistoricalBatches {
		fmt.Fprintf(w, " %-8s", fmt.Sprintf("B%d", histBatch))
	}
	fmt.Fprintf(w, " %-8s\n", "B"+fmt.Sprintf("%d", batchNum))

	// Print separator
	fmt.Fprintf(w, "%-20s %-10s %-10s", "----", "---------", "----")
	for range gridData.HistoricalBatches {
		fmt.Fprintf(w, " %-8s", "----")
	}
	fmt.Fprintf(w, " %-8s\n", "----")

	// Print rows
	for _, row := range gridData.Rows {
		fmt.Fprintf(w, "%-20s %-10d %-10d", row.Word, row.ClusterID, row.ClusterSize)

		// Historical columns
		for _, histBatch := range gridData.HistoricalBatches {
			histKey := fmt.Sprintf("%d", histBatch)
			if value, exists := row.HistoricalData[histKey]; exists && value != "" {
				fmt.Fprintf(w, " %-8s", value)
			} else {
				fmt.Fprintf(w, " %-8s", "")
			}
		}

		// Current batch column (always shows the word)
		fmt.Fprintf(w, " %-8s\n", row.Word)
	}
}

func handleGrid(w http.ResponseWriter, r *http.Request) {
	// Extract batch number from URL
	path := r.URL.Path
	batchNumStr := path[len("/grid/"):]

	batchNum, err := strconv.Atoi(batchNumStr)
	if err != nil {
		// Default to batch 0 if no number provided
		batchNum = 0
	}

	if batchNum < 0 {
		http.Error(w, "Batch number out of range", http.StatusBadRequest)
		return
	}

	// Note: We don't auto-advance here to avoid infinite loops during page load
	// Auto-advancement happens in the API call instead

	// Parse the grid template
	tmpl, err := template.ParseFiles("templates/grid.html")
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	// Prepare template data
	data := struct {
		CurrentBatch int
	}{
		CurrentBatch: batchNum,
	}

	// Execute template
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "Template execution error", http.StatusInternalServerError)
		return
	}
}

func handleGridDataAPI(w http.ResponseWriter, r *http.Request) {
	// Use current batch from navigation instead of URL parameter
	currentBatch := getCurrentBatch()
	if currentBatch == nil {
		http.Error(w, "No current batch available", http.StatusNotFound)
		return
	}

	batchNum := currentBatch.Data.BatchNumber

	if batchNum < 0 {
		http.Error(w, "Batch number out of range", http.StatusBadRequest)
		return
	}

	// If batch number is beyond what we have, check if we can load more
	if batchNum >= len(allBatches) {
		fmt.Printf("handleGridDataAPI: batch %d >= %d, attempting to load more chunks\n", batchNum, len(allBatches))
		if err := loadMoreChunks(batchNum); err != nil {
			if strings.Contains(err.Error(), "no more batches available") {
				fmt.Printf("handleGridDataAPI: reached end of file at batch %d (total batches: %d)\n", batchNum, len(allBatches))
				// Return end-of-file indicator instead of error
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(GridData{
					CurrentBatch: -1, // Special value to indicate end of file
					Rows:         []GridRow{},
				})
				return
			} else {
				fmt.Printf("handleGridDataAPI: actual error loading chunks: %v\n", err)
				http.Error(w, "Error loading data", http.StatusInternalServerError)
				return
			}
		}
	}

	// Double-check that we now have the requested batch
	if batchNum >= len(allBatches) {
		fmt.Printf("handleGridDataAPI: batch %d still out of range after loading chunks (total: %d)\n", batchNum, len(allBatches))
		http.Error(w, "Batch number out of range", http.StatusBadRequest)
		return
	}

	// Check if the requested batch has data, if not find the next one that does
	originalBatch := batchNum
	hasData := hasDataForBatch(batchNum, config.MinClusterSize)
	fmt.Printf("handleGridDataAPI: batch %d hasDataForBatch returned %v\n", batchNum, hasData)
	if !hasData {
		fmt.Printf("Batch %d has no data (min cluster size: %d), looking for next batch with data\n", batchNum, config.MinClusterSize)
		nextBatch := findNextBatchWithData(batchNum, 1, config.MinClusterSize)
		if nextBatch != batchNum {
			batchNum = nextBatch
			fmt.Printf("Auto-advancing from batch %d to batch %d\n", originalBatch, batchNum)
		} else {
			// Try going backwards if no next batch found
			prevBatch := findNextBatchWithData(originalBatch, -1, config.MinClusterSize)
			if prevBatch != originalBatch {
				batchNum = prevBatch
				fmt.Printf("Auto-advancing from batch %d to previous batch %d\n", originalBatch, batchNum)
			} else {
				fmt.Printf("No batches with data found around batch %d\n", originalBatch)
				// Check if we've reached the end of the input file
				if len(allBatches) > 0 {
					lastBatch := len(allBatches) - 1
					fmt.Printf("Reached end of input file. Last available batch is %d (total batches loaded: %d)\n", lastBatch, len(allBatches))
					// Return empty grid data with end-of-file indicator
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(GridData{
						CurrentBatch: -1, // Special value to indicate end of file
						Rows:         []GridRow{},
					})
					return
				} else {
					fmt.Printf("No batches loaded at all - input file may be empty or invalid\n")
					// Return empty grid data if no batches with data are found
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(GridData{})
					return
				}
			}
		}
	}

	// Compute grid data
	gridData := computeGridData(batchNum, config.HistoricalBatches, config.MinClusterSize)

	// Set response headers
	w.Header().Set("Content-Type", "application/json")

	// Encode and send response
	if err := json.NewEncoder(w).Encode(gridData); err != nil {
		http.Error(w, "JSON encoding error", http.StatusInternalServerError)
		return
	}
}

// MedoidRow represents a single row in the medoid list
type MedoidRow struct {
	BatchNumber     int            `json:"batch_number"`
	BatchTime       string         `json:"batch_time"`
	ClusterID       int            `json:"cluster_id"`
	ClusterSize     int            `json:"cluster_size"`
	MedoidText      string         `json:"medoid_text"`
	BusyWords       []string       `json:"busy_words"`
	PersistenceData map[string]int `json:"persistence_data"` // batch_number -> persistence count
}

// MedoidData represents the complete medoid list for display
type MedoidData struct {
	CurrentBatch      int         `json:"current_batch"`
	BatchTime         string      `json:"batch_time"`
	BatchDuration     string      `json:"batch_duration"`
	HistoricalBatches []int       `json:"historical_batches"`
	Rows              []MedoidRow `json:"rows"`
	MinClusterSize    int         `json:"min_cluster_size"`
}

func handleMedoidDefault(w http.ResponseWriter, r *http.Request) {
	// Redirect to medoid with batch 0
	http.Redirect(w, r, "/medoid/0", http.StatusSeeOther)
}

func handleMedoid(w http.ResponseWriter, r *http.Request) {
	// Extract batch number from URL
	path := r.URL.Path
	batchNumStr := path[len("/medoid/"):]

	batchNum, err := strconv.Atoi(batchNumStr)
	if err != nil {
		// Default to batch 0 if no number provided
		batchNum = 0
	}

	if batchNum < 0 {
		http.Error(w, "Batch number out of range", http.StatusBadRequest)
		return
	}

	// Parse the medoid template
	tmpl, err := template.ParseFiles("templates/medoid.html")
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	// Prepare template data
	data := struct {
		CurrentBatch int
	}{
		CurrentBatch: batchNum,
	}

	// Execute template
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "Template execution error", http.StatusInternalServerError)
		return
	}
}

func handleMedoidDataAPI(w http.ResponseWriter, r *http.Request) {
	// Use current batch from navigation instead of URL parameter
	currentBatch := getCurrentBatch()
	if currentBatch == nil {
		http.Error(w, "No current batch available", http.StatusNotFound)
		return
	}

	batchNum := currentBatch.Data.BatchNumber

	if batchNum < 0 {
		http.Error(w, "Batch number out of range", http.StatusBadRequest)
		return
	}

	// If batch number is beyond what we have, check if we can load more
	if batchNum >= len(allBatches) {
		fmt.Printf("handleMedoidDataAPI: batch %d >= %d, attempting to load more chunks\n", batchNum, len(allBatches))
		if err := loadMoreChunks(batchNum); err != nil {
			if strings.Contains(err.Error(), "no more batches available") {
				fmt.Printf("handleMedoidDataAPI: reached end of file at batch %d (total batches: %d)\n", batchNum, len(allBatches))
				// Return end-of-file indicator instead of error
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(MedoidData{
					CurrentBatch: -1, // Special value to indicate end of file
					Rows:         []MedoidRow{},
				})
				return
			} else {
				fmt.Printf("handleMedoidDataAPI: actual error loading chunks: %v\n", err)
				http.Error(w, "Error loading data", http.StatusInternalServerError)
				return
			}
		}
	}

	// Double-check that we now have the requested batch
	if batchNum >= len(allBatches) {
		fmt.Printf("handleMedoidDataAPI: batch %d still out of range after loading chunks (total: %d)\n", batchNum, len(allBatches))
		http.Error(w, "Batch number out of range", http.StatusBadRequest)
		return
	}

	// Check if the requested batch has data, if not find the next one that does
	originalBatch := batchNum
	hasData := hasDataForBatch(batchNum, config.MinClusterSize)
	fmt.Printf("handleMedoidDataAPI: batch %d hasDataForBatch returned %v\n", batchNum, hasData)
	if !hasData {
		fmt.Printf("Batch %d has no data (min cluster size: %d), looking for next batch with data\n", batchNum, config.MinClusterSize)
		nextBatch := findNextBatchWithData(batchNum, 1, config.MinClusterSize)
		if nextBatch != batchNum {
			batchNum = nextBatch
			fmt.Printf("Auto-advancing from batch %d to batch %d\n", originalBatch, batchNum)
		} else {
			// Try going backwards if no next batch found
			prevBatch := findNextBatchWithData(originalBatch, -1, config.MinClusterSize)
			if prevBatch != originalBatch {
				batchNum = prevBatch
				fmt.Printf("Auto-advancing from batch %d to previous batch %d\n", originalBatch, batchNum)
			} else {
				fmt.Printf("No batches with data found around batch %d\n", originalBatch)
				// Check if we've reached the end of the input file
				if len(allBatches) > 0 {
					lastBatch := len(allBatches) - 1
					fmt.Printf("Reached end of input file. Last available batch is %d (total batches loaded: %d)\n", lastBatch, len(allBatches))
					// Return empty medoid data with end-of-file indicator
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(MedoidData{
						CurrentBatch: -1, // Special value to indicate end of file
						Rows:         []MedoidRow{},
					})
					return
				} else {
					fmt.Printf("No batches loaded at all - input file may be empty or invalid\n")
					// Return empty medoid data if no batches with data are found
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(MedoidData{})
					return
				}
			}
		}
	}

	// Compute medoid data
	medoidData := computeMedoidData(batchNum, config.HistoricalBatches, config.MinClusterSize)

	// Set response headers
	w.Header().Set("Content-Type", "application/json")

	// Encode and send response
	if err := json.NewEncoder(w).Encode(medoidData); err != nil {
		http.Error(w, "JSON encoding error", http.StatusInternalServerError)
		return
	}
}

func computeMedoidData(batchNum, historicalBatches, minClusterSize int) MedoidData {
	batch := allBatches[batchNum]

	// Get historical batch numbers
	historicalBatchNumbers := make([]int, 0)
	for i := 1; i <= historicalBatches; i++ {
		histBatch := batchNum - i
		if histBatch >= 0 {
			historicalBatchNumbers = append(historicalBatchNumbers, histBatch)
		}
	}

	// Extract clusters from batch data
	clustersInterface, ok := batch.Data.Clusters.([]interface{})
	if !ok {
		return MedoidData{
			CurrentBatch:      batchNum,
			BatchTime:         batch.Data.BatchTime,
			HistoricalBatches: historicalBatchNumbers,
			Rows:              []MedoidRow{},
			MinClusterSize:    minClusterSize,
		}
	}

	var medoidRows []MedoidRow

	for _, clusterInterface := range clustersInterface {
		clusterMap, ok := clusterInterface.(map[string]interface{})
		if !ok {
			continue
		}

		// Extract cluster data
		clusterID, _ := clusterMap["cluster_id"].(float64)
		size, _ := clusterMap["size"].(float64)

		// Skip clusters below minimum size
		if int(size) < minClusterSize {
			continue
		}

		// Extract medoid text - handle new structure
		medoidText := ""
		if medoid, ok := clusterMap["medoid"].(string); ok {
			// Old format: medoid field exists
			medoidText = medoid
		} else if medoidTweet, ok := clusterMap["medoid_tweet"].(string); ok {
			// Alternative medoid field
			medoidText = medoidTweet
		} else if tweetsInterface, ok := clusterMap["tweets"].([]interface{}); ok {
			// New format: find medoid in tweets array (first tweet is typically the medoid)
			if len(tweetsInterface) > 0 {
				if tweetMap, ok := tweetsInterface[0].(map[string]interface{}); ok {
					if text, ok := tweetMap["text"].(string); ok {
						medoidText = text
					}
				}
			}
		}

		// Extract busy words from new struct format
		var busyWords []string
		busyWordsInterface, ok := clusterMap["busy_words"].([]interface{})
		if ok {
			for _, wordInterface := range busyWordsInterface {
				wordObj, ok := wordInterface.(map[string]interface{})
				if !ok {
					continue
				}

				word, ok := wordObj["word"].(string)
				if !ok {
					continue
				}

				busyWords = append(busyWords, word)
			}
		}

		// Calculate persistence data (how many batches back this cluster can be traced)
		persistenceData := make(map[string]int)
		for _, histBatch := range historicalBatchNumbers {
			if histBatch >= 0 && histBatch < len(allBatches) {
				// Simple persistence calculation - could be enhanced with LD method
				persistenceCount := calculatePersistence(int(clusterID), histBatch, batchNum)
				persistenceData[fmt.Sprintf("%d", histBatch)] = persistenceCount
			}
		}

		medoidRow := MedoidRow{
			BatchNumber:     batchNum,
			BatchTime:       batch.Data.BatchTime,
			ClusterID:       int(clusterID),
			ClusterSize:     int(size),
			MedoidText:      medoidText,
			BusyWords:       busyWords,
			PersistenceData: persistenceData,
		}

		medoidRows = append(medoidRows, medoidRow)
	}

	return MedoidData{
		CurrentBatch:      batchNum,
		BatchTime:         batch.Data.BatchTime,
		HistoricalBatches: historicalBatchNumbers,
		Rows:              medoidRows,
		MinClusterSize:    minClusterSize,
	}
}

func calculatePersistence(clusterID, histBatch, currentBatch int) int {
	// Simple persistence calculation - returns 1 if cluster exists in historical batch
	// This could be enhanced with Levenshtein distance method as mentioned in the spec

	if histBatch < 0 || histBatch >= len(allBatches) {
		return 0
	}

	histBatchData := allBatches[histBatch]
	clustersInterface, ok := histBatchData.Data.Clusters.([]interface{})
	if !ok {
		return 0
	}

	for _, clusterInterface := range clustersInterface {
		clusterMap, ok := clusterInterface.(map[string]interface{})
		if !ok {
			continue
		}

		histClusterID, _ := clusterMap["cluster_id"].(float64)
		if int(histClusterID) == clusterID {
			return 1
		}
	}

	return 0
}

func handleBubblesDefault(w http.ResponseWriter, r *http.Request) {
	// Use current batch from navigation instead of redirecting to a specific batch
	currentBatch := getCurrentBatch()
	if currentBatch == nil {
		http.Error(w, "No current batch available", http.StatusNotFound)
		return
	}

	// Render the bubbles template with current batch data
	if err := templates.ExecuteTemplate(w, "bubbles.html", map[string]interface{}{
		"CurrentBatch": currentBatch,
		"BatchNumber":  currentBatch.Data.BatchNumber,
	}); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

func handleBubbles(w http.ResponseWriter, r *http.Request) {
	// Extract batch number from URL
	path := r.URL.Path
	batchNumStr := path[len("/bubbles/"):]
	batchNum, err := strconv.Atoi(batchNumStr)
	if err != nil {
		http.Error(w, "Invalid batch number", http.StatusBadRequest)
		return
	}

	if batchNum < 0 {
		http.Error(w, "Batch number out of range", http.StatusBadRequest)
		return
	}

	// Render the bubbles template
	data := map[string]interface{}{
		"CurrentBatch": batchNum,
		"TotalBatches": len(allBatches),
	}

	if err := templates.ExecuteTemplate(w, "bubbles.html", data); err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}
}

func handleBubbleDataAPI(w http.ResponseWriter, r *http.Request) {
	// Use current batch from navigation instead of URL parameter
	currentBatch := getCurrentBatch()
	if currentBatch == nil {
		http.Error(w, "No current batch available", http.StatusNotFound)
		return
	}

	batchNum := currentBatch.Data.BatchNumber
	if batchNum < 0 {
		http.Error(w, "Batch number out of range", http.StatusBadRequest)
		return
	}

	// If batch number is beyond what we have, check if we can load more
	if batchNum >= len(allBatches) {
		fmt.Printf("handleBubbleDataAPI: batch %d >= %d, attempting to load more chunks\n", batchNum, len(allBatches))
		if err := loadMoreChunks(batchNum); err != nil {
			if strings.Contains(err.Error(), "no more batches available") {
				fmt.Printf("handleBubbleDataAPI: reached end of file at batch %d (total batches: %d)\n", batchNum, len(allBatches))
				// Return end-of-file indicator instead of error
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"current_batch": -1, // Special value to indicate end of file
					"bubble_words":  []interface{}{},
				})
				return
			} else {
				fmt.Printf("handleBubbleDataAPI: actual error loading chunks: %v\n", err)
				http.Error(w, "Error loading data", http.StatusInternalServerError)
				return
			}
		}
	}

	// Double-check that we now have the requested batch
	if batchNum >= len(allBatches) {
		fmt.Printf("handleBubbleDataAPI: batch %d still out of range after loading chunks (total: %d)\n", batchNum, len(allBatches))
		http.Error(w, "Batch number out of range", http.StatusBadRequest)
		return
	}

	// Check if the requested batch has data, if not find the next one that does
	originalBatch := batchNum
	hasData := hasDataForBatch(batchNum, config.MinClusterSize)
	fmt.Printf("handleBubbleDataAPI: batch %d hasDataForBatch returned %v\n", batchNum, hasData)
	if !hasData {
		fmt.Printf("Batch %d has no data (min cluster size: %d), looking for next batch with data\n", batchNum, config.MinClusterSize)
		nextBatch := findNextBatchWithData(batchNum, 1, config.MinClusterSize)
		if nextBatch != batchNum {
			batchNum = nextBatch
			fmt.Printf("Auto-advancing from batch %d to batch %d\n", originalBatch, batchNum)
		} else {
			// Try going backwards if no next batch found
			prevBatch := findNextBatchWithData(originalBatch, -1, config.MinClusterSize)
			if prevBatch != originalBatch {
				batchNum = prevBatch
				fmt.Printf("Auto-advancing from batch %d to previous batch %d\n", originalBatch, batchNum)
			} else {
				fmt.Printf("No batches with data found around batch %d\n", originalBatch)
			}
		}
	}

	// Compute bubble data
	bubbleData := computeBubbleData(batchNum, config.BubbleBatches, config.MinClusterSize)

	// Set response headers
	w.Header().Set("Content-Type", "application/json")

	// Encode and send response
	if err := json.NewEncoder(w).Encode(bubbleData); err != nil {
		http.Error(w, "JSON encoding error", http.StatusInternalServerError)
		return
	}
}

func computeBubbleData(batchNum, historicalBatches, minClusterSize int) map[string]interface{} {
	// Get historical batch numbers (current batch and previous batches)
	historicalBatchNumbers := make([]int, 0)
	for i := 0; i < historicalBatches; i++ {
		histBatch := batchNum - i
		if histBatch >= 0 {
			historicalBatchNumbers = append(historicalBatchNumbers, histBatch)
		}
	}

	// Track active words and their positions across time
	activeWords := make(map[string]*BubbleWord)
	medoidClusters := make([]MedoidRow, 0)

	// Process batches from oldest to newest to track word evolution
	for i := len(historicalBatchNumbers) - 1; i >= 0; i-- {
		histBatch := historicalBatchNumbers[i]
		// batchPos := len(historicalBatchNumbers) - 1 - i // 0 = current (right), 1 = previous, etc.

		if histBatch >= len(allBatches) {
			continue
		}

		batch := allBatches[histBatch]
		clustersInterface, ok := batch.Data.Clusters.([]interface{})
		if !ok {
			continue
		}

		// Extract busy words from all clusters in this batch
		for _, clusterInterface := range clustersInterface {
			clusterMap, ok := clusterInterface.(map[string]interface{})
			if !ok {
				continue
			}

			size, _ := clusterMap["size"].(float64)
			if int(size) < minClusterSize {
				continue
			}

			// Get busy words and their frequency classes
			busyWordsInterface, ok := clusterMap["busy_words"].([]interface{})
			if !ok {
				continue
			}

			if len(busyWordsInterface) > 0 {
				for _, wordInterface := range busyWordsInterface {
					// Extract from object with word, class, z_score, count, mean
					wordObj, ok := wordInterface.(map[string]interface{})
					if !ok {
						continue
					}

					word, ok := wordObj["word"].(string)
					if !ok {
						continue
					}

					frequencyClass := 12 // Default
					if classFloat, ok := wordObj["class"].(float64); ok {
						frequencyClass = int(classFloat)
					}

					zScore := 0.0
					if zScoreFloat, ok := wordObj["z_score"].(float64); ok {
						zScore = zScoreFloat
					}

					// Calculate divergence score based on z_score (higher z_score = higher divergence)
					divergenceScore := zScore / 10.0 // Normalize z_score to 0-1 range
					if divergenceScore > 1.0 {
						divergenceScore = 1.0
					}

					// Check if this word is already being tracked
					if existingBubble, exists := activeWords[word]; exists {
						// Word reappeared - snap back to current position (right side)
						existingBubble.BatchPosition = 0
						existingBubble.FrequencyClass = frequencyClass
						existingBubble.DivergenceScore = divergenceScore
						existingBubble.ColorIntensity = float64(frequencyClass) / 24.0
						existingBubble.BubbleSize = divergenceScore
					} else {
						// New word - create bubble at current position
						colorIntensity := float64(frequencyClass) / 24.0
						bubbleSize := divergenceScore

						// Assign Y position based on frequency class (0 = top, 1 = bottom)
						yPosition := float64(frequencyClass) / 24.0

						bubbleWord := &BubbleWord{
							Word:            word,
							FrequencyClass:  frequencyClass,
							DivergenceScore: divergenceScore,
							BatchPosition:   0, // Current batch (right side)
							ColorIntensity:  colorIntensity,
							BubbleSize:      bubbleSize,
							YPosition:       yPosition,
						}

						activeWords[word] = bubbleWord
					}
				}
			}

			// Also collect medoid data for the side panel (only from current batch)
			if i == 0 {
				clusterID, _ := clusterMap["cluster_id"].(float64)
				medoidText := ""
				if medoid, ok := clusterMap["medoid"].(string); ok {
					medoidText = medoid
				}

				var busyWords []string
				if busyWordsArray, ok := clusterMap["busy_words"].([]interface{}); ok {
					for _, wordInterface := range busyWordsArray {
						if wordObj, ok := wordInterface.(map[string]interface{}); ok {
							if wordStr, ok := wordObj["word"].(string); ok {
								busyWords = append(busyWords, wordStr)
							}
						}
					}
				}

				medoidRow := MedoidRow{
					BatchNumber: batchNum,
					BatchTime:   batch.Data.BatchTime,
					ClusterID:   int(clusterID),
					ClusterSize: int(size),
					MedoidText:  medoidText,
					BusyWords:   busyWords,
				}

				medoidClusters = append(medoidClusters, medoidRow)
			}
		}

		// Age all existing words (move them left)
		for _, bubble := range activeWords {
			if bubble.BatchPosition < len(historicalBatchNumbers)-1 {
				bubble.BatchPosition++
			}
		}
	}

	// Convert map to slice and filter out words that have fallen off the left edge
	bubbleWords := make([]BubbleWord, 0)
	wordList := make([]map[string]interface{}, 0) // For the far-right column

	for _, bubble := range activeWords {
		if bubble.BatchPosition < len(historicalBatchNumbers) {
			bubbleWords = append(bubbleWords, *bubble)
		}

		// Add to word list for far-right column (all active words)
		// Calculate the actual batch where this word originated
		actualBatchId := batchNum - bubble.BatchPosition
		wordList = append(wordList, map[string]interface{}{
			"batch_id":        actualBatchId,
			"frequency_class": bubble.FrequencyClass,
			"word":            bubble.Word,
		})
	}

	// Sort word list by batch (latest first), then lexically within each batch
	sort.Slice(wordList, func(i, j int) bool {
		batchI := wordList[i]["batch_id"].(int)
		batchJ := wordList[j]["batch_id"].(int)
		if batchI != batchJ {
			return batchI > batchJ // Latest batch first
		}
		wordI := wordList[i]["word"].(string)
		wordJ := wordList[j]["word"].(string)
		return wordI < wordJ
	})

	// Create batch info string
	currentBatch := allBatches[batchNum]
	batchInfo := fmt.Sprintf("Batch %d - %s | Clusters: %d | Active Words: %d",
		batchNum, currentBatch.Data.BatchTime, len(medoidClusters), len(bubbleWords))

	return map[string]interface{}{
		"current_batch":   batchNum,
		"batch_time":      currentBatch.Data.BatchTime,
		"batch_info":      batchInfo,
		"bubble_words":    bubbleWords,
		"medoid_clusters": medoidClusters,
		"num_batches":     len(historicalBatchNumbers),
		"bubble_color":    config.BubbleColor,
		"word_list":       wordList,
	}
}

func handleClusterDataAPI(w http.ResponseWriter, r *http.Request) {
	// Extract batch number and cluster ID from URL
	path := r.URL.Path
	pathParts := strings.Split(path[len("/api/cluster-data/"):], "/")
	if len(pathParts) != 2 {
		http.Error(w, "Invalid URL format. Expected /api/cluster-data/batch/cluster", http.StatusBadRequest)
		return
	}

	batchNum, err := strconv.Atoi(pathParts[0])
	if err != nil {
		http.Error(w, "Invalid batch number", http.StatusBadRequest)
		return
	}

	clusterID, err := strconv.Atoi(pathParts[1])
	if err != nil {
		http.Error(w, "Invalid cluster ID", http.StatusBadRequest)
		return
	}

	if batchNum < 0 || batchNum >= len(allBatches) {
		http.Error(w, "Batch number out of range", http.StatusBadRequest)
		return
	}

	// Get the batch data
	batch := allBatches[batchNum]
	clustersInterface, ok := batch.Data.Clusters.([]interface{})
	if !ok {
		http.Error(w, "Invalid clusters data format", http.StatusInternalServerError)
		return
	}

	// Find the specific cluster
	var targetCluster map[string]interface{}
	for _, clusterInterface := range clustersInterface {
		clusterMap, ok := clusterInterface.(map[string]interface{})
		if !ok {
			continue
		}

		clusterIDFloat, _ := clusterMap["cluster_id"].(float64)
		if int(clusterIDFloat) == clusterID {
			targetCluster = clusterMap
			break
		}
	}

	if targetCluster == nil {
		http.Error(w, "Cluster not found", http.StatusNotFound)
		return
	}

	// Extract tweet data
	var tweets []map[string]interface{}
	if tweetTextsInterface, ok := targetCluster["tweet_texts"].([]interface{}); ok {
		for _, tweetInterface := range tweetTextsInterface {
			if tweetText, ok := tweetInterface.(string); ok {
				tweets = append(tweets, map[string]interface{}{
					"text": tweetText,
				})
			}
		}
	}

	// Create response
	response := map[string]interface{}{
		"batch_number": batchNum,
		"cluster_id":   clusterID,
		"tweets":       tweets,
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/json")

	// Encode and send response
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "JSON encoding error", http.StatusInternalServerError)
		return
	}
}

// BubbleWord represents a single busyword in the bubble graph
type BubbleWord struct {
	Word            string  `json:"word"`
	FrequencyClass  int     `json:"frequency_class"`
	DivergenceScore float64 `json:"divergence_score"`
	BatchPosition   int     `json:"batch_position"` // 0 = current, 1 = previous, etc.
	ColorIntensity  float64 `json:"color_intensity"`
	BubbleSize      float64 `json:"bubble_size"`
	YPosition       float64 `json:"y_position"` // Consistent Y position for tracking
}

// BubbleData represents the complete bubble graph data
type BubbleData struct {
	CurrentBatch   int          `json:"current_batch"`
	BatchTime      string       `json:"batch_time"`
	BatchInfo      string       `json:"batch_info"`
	BubbleWords    []BubbleWord `json:"bubble_words"`
	MedoidClusters []MedoidRow  `json:"medoid_clusters"`
	NumBatches     int          `json:"num_batches"`
}
