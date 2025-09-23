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

	"cursor-twitter-display/types"

	"gopkg.in/yaml.v3"
)

// Helper function to get keys from a map
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
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

var (
	config      types.Config
	allBatches  []types.Batch
	templates   *template.Template
	fileHandle  *os.File
	fileOffset  int64
	partialJSON string // Store partial JSON data between chunks
)

// Global variables for batch navigation
var currentBatchIndex int = 0
var maxBatchesInMemory = 200 // Keep only last 200 batches in memory

// getNextBatch returns the next batch in the sequence, loading more if needed
func getNextBatch() *types.Batch {
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
func getPreviousBatch() *types.Batch {
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
func getCurrentBatch() *types.Batch {
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
			var batch types.Batch
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
	data := types.PageData{
		CurrentBatch: batchNum,
		Batch:        batch,
		HasNext:      batchNum < len(allBatches)-1,
		HasPrev:      batchNum > 0,
		BatchInfo: fmt.Sprintf("Batch %d (Batch %d, %s)",
			batchNum+1, batch.Data.BatchNumber, batch.Data.BatchTime),
	}

	templates.ExecuteTemplate(w, "index.html", data)
}

// extractChartClustersFromBatch extracts clusters from batch for chart data
func extractChartClustersFromBatch(batch types.Batch) ([]types.Cluster, error) {
	return batch.Data.Clusters, nil
}

// processChartCluster processes a single cluster for chart data
func processChartCluster(cluster types.Cluster, clusterIndex int) types.ClusterData {
	fmt.Printf("Cluster %d: ID=%v, Size=%v\n", clusterIndex, cluster.ClusterID, cluster.Size)

	// Extract busy words
	var busyWords []string
	for _, busyWord := range cluster.BusyWords {
		busyWords = append(busyWords, busyWord.Word)
	}

	fmt.Printf("Cluster %d: Found %d busy words: %v\n", clusterIndex, len(busyWords), busyWords)

	// Extract tweet texts
	var tweetTexts []string
	for _, tweet := range cluster.Tweets {
		tweetTexts = append(tweetTexts, tweet.Text)
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

	clusterData := types.ClusterData{
		ClusterID:   cluster.ClusterID,
		Size:        cluster.Size,
		BusyWords:   busyWords,
		WordCounts:  wordCounts,
		TotalTweets: cluster.Size, // This is the actual tweet count from the cluster
	}

	fmt.Printf("Cluster %d: Added to chart with %d word counts: %v\n", clusterIndex, len(wordCounts), wordCounts)
	return clusterData
}

// buildChartDataFromClusters builds the complete chart data from clusters
func buildChartDataFromClusters(clusters []types.Cluster) types.ChartData {
	var chartData types.ChartData

	fmt.Printf("Processing %d clusters for chart data\n", len(clusters))

	for i, cluster := range clusters {
		clusterData := processChartCluster(cluster, i)
		chartData.Clusters = append(chartData.Clusters, clusterData)
	}

	fmt.Printf("Final chart data: %d clusters\n", len(chartData.Clusters))
	return chartData
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

	// Extract clusters from batch
	clusters, err := extractChartClustersFromBatch(batch)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Build chart data from clusters
	chartData := buildChartDataFromClusters(clusters)

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
	fmt.Printf("hasDataForBatch: batch %d has %d clusters\n", batchIndex, len(batch.Data.Clusters))
	for i, cluster := range batch.Data.Clusters {
		fmt.Printf("hasDataForBatch: batch %d cluster %d size %v (min required: %d)\n", batchIndex, i, cluster.Size, minClusterSize)
		if cluster.Size >= minClusterSize {
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

// extractClustersFromBatch extracts and validates clusters from a batch
func extractClustersFromBatch(batch types.Batch, minClusterSize int) []types.Cluster {
	var clusters []types.Cluster
	for _, cluster := range batch.Data.Clusters {
		if cluster.Size >= minClusterSize {
			clusters = append(clusters, cluster)
		}
	}
	return clusters
}

// buildGridRowsFromClusters creates GridRow structs from cluster data
func buildGridRowsFromClusters(clusters []types.Cluster) ([]types.GridRow, []types.ClusterTweetData) {
	var gridRows []types.GridRow
	var clusterTweetData []types.ClusterTweetData

	for _, cluster := range clusters {
		// Extract busy words
		var busyWords []string
		for _, busyWord := range cluster.BusyWords {
			busyWords = append(busyWords, busyWord.Word)

			gridRow := types.GridRow{
				Word:           busyWord.Word,
				ClusterID:      cluster.ClusterID,
				ClusterSize:    cluster.Size,
				QualityScore:   0.0,
				HistoricalData: make(map[string]string),
			}
			gridRows = append(gridRows, gridRow)
		}

		// Extract tweet data
		var tweets []types.TweetData
		if cluster.MedoidTweet != "" {
			medoidText := stripTimestamp(cluster.MedoidTweet)
			tweets = append(tweets, types.TweetData{
				Text:     medoidText,
				IsMedoid: true,
			})
		}

		var tweetTexts []string
		for _, tweet := range cluster.Tweets {
			tweetTexts = append(tweetTexts, tweet.Text)
		}

		numBusyWords := len(busyWords)
		numTweetsToShow := numBusyWords
		if len(tweetTexts) < numTweetsToShow {
			numTweetsToShow = len(tweetTexts)
		}

		tweetsAdded := 0
		for _, tweetText := range tweetTexts {
			if tweetsAdded >= numTweetsToShow {
				break
			}

			if cluster.MedoidTweet != "" && tweetText == cluster.MedoidTweet {
				continue
			}

			cleanTweetText := stripTimestamp(tweetText)
			tweets = append(tweets, types.TweetData{
				Text:     cleanTweetText,
				IsMedoid: false,
			})
			tweetsAdded++
		}

		clusterTweetData = append(clusterTweetData, types.ClusterTweetData{
			ClusterID: cluster.ClusterID,
			Size:      cluster.Size,
			BusyWords: busyWords,
			Tweets:    tweets,
		})
	}

	return gridRows, clusterTweetData
}

// extractCurrentMedoid extracts the current cluster's medoid for comparison
func extractCurrentMedoid(clusters []types.Cluster, clusterID int) string {
	for _, cluster := range clusters {
		if cluster.ClusterID == clusterID {
			if cluster.MedoidTweet != "" {
				return stripTimestamp(cluster.MedoidTweet)
			} else if len(cluster.Tweets) > 0 {
				return stripTimestamp(cluster.Tweets[0].Text)
			}
			break
		}
	}
	return ""
}

// processHistoricalBatchForWord processes a single historical batch for a word
func processHistoricalBatchForWord(word string, historicalIndex int, currentMedoid string) (bool, bool) {
	if historicalIndex < 0 || historicalIndex >= len(allBatches) {
		return false, false
	}

	historicalBatch := allBatches[historicalIndex]
	found := false
	recurrenceFound := false

	for _, cluster := range historicalBatch.Data.Clusters {
		for _, busyWord := range cluster.BusyWords {
			if busyWord.Word == word {
				found = true
				recurrenceFound = detectRecurrence(cluster, currentMedoid)
				break
			}
		}
		if found {
			break
		}
	}

	return found, recurrenceFound
}

// detectRecurrence handles the complex recurrence detection logic
func detectRecurrence(cluster types.Cluster, currentMedoid string) bool {
	if currentMedoid == "" {
		return false
	}

	if config.RecurrenceStrategy == "medoid_only" {
		// Strategy 1: Compare only medoids
		var historicalMedoidClean string
		if cluster.MedoidTweet != "" {
			historicalMedoidClean = stripTimestamp(cluster.MedoidTweet)
		} else if len(cluster.Tweets) > 0 {
			historicalMedoidClean = stripTimestamp(cluster.Tweets[0].Text)
		}

		if historicalMedoidClean != "" {
			distance := calculateNormalizedLevenshtein(currentMedoid, historicalMedoidClean)
			return distance <= config.RecurrenceThreshold
		}
	} else {
		// Strategy 2: Compare to all tweets (default)
		for _, tweet := range cluster.Tweets {
			historicalTweetClean := stripTimestamp(tweet.Text)
			distance := calculateNormalizedLevenshtein(currentMedoid, historicalTweetClean)
			if distance <= config.RecurrenceThreshold {
				return true
			}
		}
	}

	return false
}

// processHistoricalData fills in historical data for grid rows
func processHistoricalData(gridRows []types.GridRow, clusters []types.Cluster, historicalBatchNumbers []int, currentBatchIndex int) {
	for i := range gridRows {
		word := gridRows[i].Word
		clusterID := gridRows[i].ClusterID

		gridRows[i].RecurrenceData = make(map[string]bool)

		// Extract current medoid for comparison
		currentMedoid := extractCurrentMedoid(clusters, clusterID)

		// Process each historical batch
		for _, historicalIndex := range historicalBatchNumbers {
			found, recurrenceFound := processHistoricalBatchForWord(word, historicalIndex, currentMedoid)

			if found {
				gridRows[i].HistoricalData[fmt.Sprintf("%d", historicalIndex)] = word
			} else {
				gridRows[i].HistoricalData[fmt.Sprintf("%d", historicalIndex)] = ""
			}
			gridRows[i].RecurrenceData[fmt.Sprintf("%d", historicalIndex)] = recurrenceFound
		}
	}
}

// calculateQualityScores computes quality scores for all grid rows
func calculateQualityScores(gridRows []types.GridRow, clusters []types.Cluster, historicalBatchNumbers []int) {
	maxClusterSize := 0
	maxTweetCount := 0
	for _, row := range gridRows {
		if row.ClusterSize > maxClusterSize {
			maxClusterSize = row.ClusterSize
		}
		for _, cluster := range clusters {
			if cluster.ClusterID == row.ClusterID {
				tweetCount := len(cluster.Tweets)
				if tweetCount > maxTweetCount {
					maxTweetCount = tweetCount
				}
				break
			}
		}
	}

	for i := range gridRows {
		var currentMedoid string
		for _, cluster := range clusters {
			if cluster.ClusterID == gridRows[i].ClusterID {
				if cluster.MedoidTweet != "" {
					currentMedoid = stripTimestamp(cluster.MedoidTweet)
				} else if len(cluster.Tweets) > 0 {
					currentMedoid = stripTimestamp(cluster.Tweets[0].Text)
				}
				break
			}
		}

		tweetCount := 0
		for _, cluster := range clusters {
			if cluster.ClusterID == gridRows[i].ClusterID {
				tweetCount = len(cluster.Tweets)
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
}

// computeGridData creates the grid data structure for the busy word display
func computeGridData(currentBatchIndex int, historicalBatches int, minClusterSize int) types.GridData {
	fmt.Printf("computeGridData: called for batch %d, currently have %d batches\n", currentBatchIndex, len(allBatches))
	if currentBatchIndex < 0 {
		return types.GridData{}
	}

	// Load more chunks if needed
	if currentBatchIndex >= len(allBatches) {
		fmt.Printf("computeGridData: batch %d >= %d, loading more chunks\n", currentBatchIndex, len(allBatches))
		if err := loadMoreChunks(currentBatchIndex); err != nil {
			fmt.Printf("computeGridData: error loading chunks: %v\n", err)
			return types.GridData{}
		}
	}

	if currentBatchIndex >= len(allBatches) {
		fmt.Printf("computeGridData: still no data for batch %d\n", currentBatchIndex)
		return types.GridData{}
	}

	currentBatch := allBatches[currentBatchIndex]

	// Extract clusters from current batch
	clusters := extractClustersFromBatch(currentBatch, minClusterSize)
	fmt.Printf("computeGridData: batch %d filtered to %d clusters >= min size\n", currentBatchIndex, len(clusters))

	// Sort clusters by size (largest first)
	sort.Slice(clusters, func(i, j int) bool {
		return clusters[i].Size > clusters[j].Size
	})

	// Build grid rows from clusters
	gridRows, clusterTweetData := buildGridRowsFromClusters(clusters)

	// Build historical batch numbers
	var historicalBatchNumbers []int
	for i := 1; i <= historicalBatches; i++ {
		historicalIndex := currentBatchIndex - i
		if historicalIndex >= 0 {
			historicalBatchNumbers = append(historicalBatchNumbers, historicalIndex)
		}
	}

	// Process historical data
	processHistoricalData(gridRows, clusters, historicalBatchNumbers, currentBatchIndex)

	// Calculate quality scores
	calculateQualityScores(gridRows, clusters, historicalBatchNumbers)

	// Calculate batch duration if we have previous batch
	var batchDuration string
	if currentBatchIndex > 0 && currentBatchIndex < len(allBatches) {
		batchDuration = "~10 seconds" // Placeholder
	} else {
		batchDuration = "N/A"
	}

	return types.GridData{
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

// validateAndLoadBatch validates the current batch and loads more chunks if needed
func validateAndLoadBatch(batchNum int) (int, error) {
	if batchNum < 0 {
		return 0, fmt.Errorf("batch number out of range")
	}

	// If batch number is beyond what we have, check if we can load more
	if batchNum >= len(allBatches) {
		fmt.Printf("validateAndLoadBatch: batch %d >= %d, attempting to load more chunks\n", batchNum, len(allBatches))
		if err := loadMoreChunks(batchNum); err != nil {
			if strings.Contains(err.Error(), "no more batches available") {
				fmt.Printf("validateAndLoadBatch: reached end of file at batch %d (total batches: %d)\n", batchNum, len(allBatches))
				return -1, fmt.Errorf("end of file reached")
			} else {
				fmt.Printf("validateAndLoadBatch: actual error loading chunks: %v\n", err)
				return 0, fmt.Errorf("error loading data: %v", err)
			}
		}
	}

	// Double-check that we now have the requested batch
	if batchNum >= len(allBatches) {
		fmt.Printf("validateAndLoadBatch: batch %d still out of range after loading chunks (total: %d)\n", batchNum, len(allBatches))
		return 0, fmt.Errorf("batch number out of range")
	}

	return batchNum, nil
}

// findBatchWithData finds a batch that has data, auto-advancing if necessary
func findBatchWithData(batchNum int) (int, error) {
	originalBatch := batchNum
	hasData := hasDataForBatch(batchNum, config.MinClusterSize)
	fmt.Printf("findBatchWithData: batch %d hasDataForBatch returned %v\n", batchNum, hasData)

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
					return -1, fmt.Errorf("end of file reached")
				} else {
					fmt.Printf("No batches loaded at all - input file may be empty or invalid\n")
					return 0, fmt.Errorf("no batches with data found")
				}
			}
		}
	}

	return batchNum, nil
}

// buildGridResponse builds and sends the grid data response
func buildGridResponse(w http.ResponseWriter, batchNum int) {
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

// buildEndOfFileResponse builds and sends an end-of-file response
func buildEndOfFileResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(types.GridData{
		CurrentBatch: -1, // Special value to indicate end of file
		Rows:         []types.GridRow{},
	})
}

func handleGridDataAPI(w http.ResponseWriter, r *http.Request) {
	// Use current batch from navigation instead of URL parameter
	currentBatch := getCurrentBatch()
	if currentBatch == nil {
		http.Error(w, "No current batch available", http.StatusNotFound)
		return
	}

	batchNum := currentBatch.Data.BatchNumber

	// Validate and load batch if needed
	validBatchNum, err := validateAndLoadBatch(batchNum)
	if err != nil {
		if strings.Contains(err.Error(), "end of file reached") {
			buildEndOfFileResponse(w)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Find a batch with data
	finalBatchNum, err := findBatchWithData(validBatchNum)
	if err != nil {
		if strings.Contains(err.Error(), "end of file reached") {
			buildEndOfFileResponse(w)
			return
		}
		if strings.Contains(err.Error(), "no batches with data found") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(types.GridData{})
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Build and send response
	buildGridResponse(w, finalBatchNum)
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

// validateAndLoadMedoidBatch validates the current batch and loads more chunks if needed
func validateAndLoadMedoidBatch(batchNum int) (int, error) {
	if batchNum < 0 {
		return 0, fmt.Errorf("batch number out of range")
	}

	// If batch number is beyond what we have, check if we can load more
	if batchNum >= len(allBatches) {
		fmt.Printf("validateAndLoadMedoidBatch: batch %d >= %d, attempting to load more chunks\n", batchNum, len(allBatches))
		if err := loadMoreChunks(batchNum); err != nil {
			if strings.Contains(err.Error(), "no more batches available") {
				fmt.Printf("validateAndLoadMedoidBatch: reached end of file at batch %d (total batches: %d)\n", batchNum, len(allBatches))
				return -1, fmt.Errorf("end of file reached")
			} else {
				fmt.Printf("validateAndLoadMedoidBatch: actual error loading chunks: %v\n", err)
				return 0, fmt.Errorf("error loading data: %v", err)
			}
		}
	}

	// Double-check that we now have the requested batch
	if batchNum >= len(allBatches) {
		fmt.Printf("validateAndLoadMedoidBatch: batch %d still out of range after loading chunks (total: %d)\n", batchNum, len(allBatches))
		return 0, fmt.Errorf("batch number out of range")
	}

	return batchNum, nil
}

// findMedoidBatchWithData finds a batch that has data, auto-advancing if necessary
func findMedoidBatchWithData(batchNum int) (int, error) {
	originalBatch := batchNum
	hasData := hasDataForBatch(batchNum, config.MinClusterSize)
	fmt.Printf("findMedoidBatchWithData: batch %d hasDataForBatch returned %v\n", batchNum, hasData)

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
					return -1, fmt.Errorf("end of file reached")
				} else {
					fmt.Printf("No batches loaded at all - input file may be empty or invalid\n")
					return 0, fmt.Errorf("no batches with data found")
				}
			}
		}
	}

	return batchNum, nil
}

// buildMedoidResponse builds and sends the medoid data response
func buildMedoidResponse(w http.ResponseWriter, batchNum int) {
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

// buildMedoidEndOfFileResponse builds and sends an end-of-file response for medoid data
func buildMedoidEndOfFileResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(types.MedoidData{
		CurrentBatch: -1, // Special value to indicate end of file
		Rows:         []types.MedoidRow{},
	})
}

func handleMedoidDataAPI(w http.ResponseWriter, r *http.Request) {
	// Use current batch from navigation instead of URL parameter
	currentBatch := getCurrentBatch()
	if currentBatch == nil {
		http.Error(w, "No current batch available", http.StatusNotFound)
		return
	}

	batchNum := currentBatch.Data.BatchNumber

	// Validate and load batch if needed
	validBatchNum, err := validateAndLoadMedoidBatch(batchNum)
	if err != nil {
		if strings.Contains(err.Error(), "end of file reached") {
			buildMedoidEndOfFileResponse(w)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Find a batch with data
	finalBatchNum, err := findMedoidBatchWithData(validBatchNum)
	if err != nil {
		if strings.Contains(err.Error(), "end of file reached") {
			buildMedoidEndOfFileResponse(w)
			return
		}
		if strings.Contains(err.Error(), "no batches with data found") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(types.MedoidData{})
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Build and send response
	buildMedoidResponse(w, finalBatchNum)
}

// buildHistoricalBatchNumbers generates the list of historical batch numbers
func buildHistoricalBatchNumbers(batchNum, historicalBatches int) []int {
	historicalBatchNumbers := make([]int, 0)
	for i := 1; i <= historicalBatches; i++ {
		histBatch := batchNum - i
		if histBatch >= 0 {
			historicalBatchNumbers = append(historicalBatchNumbers, histBatch)
		}
	}
	return historicalBatchNumbers
}

// extractMedoidClustersFromBatch extracts clusters from batch for medoid data
func extractMedoidClustersFromBatch(batch types.Batch) ([]types.Cluster, error) {
	return batch.Data.Clusters, nil
}

// processMedoidCluster processes a single cluster for medoid data
func processMedoidCluster(cluster types.Cluster, batchNum int, batchTime string, minClusterSize int, historicalBatchNumbers []int) *types.MedoidRow {
	// Skip clusters below minimum size
	if cluster.Size < minClusterSize {
		return nil
	}

	// Extract medoid text
	medoidText := ""
	if cluster.MedoidTweet != "" {
		medoidText = cluster.MedoidTweet
	} else if len(cluster.Tweets) > 0 {
		medoidText = cluster.Tweets[0].Text
	}

	// Extract busy words
	var busyWords []string
	for _, busyWord := range cluster.BusyWords {
		busyWords = append(busyWords, busyWord.Word)
	}

	// Calculate persistence data (how many batches back this cluster can be traced)
	persistenceData := make(map[string]int)
	for _, histBatch := range historicalBatchNumbers {
		if histBatch >= 0 && histBatch < len(allBatches) {
			// Simple persistence calculation - could be enhanced with LD method
			persistenceCount := calculatePersistence(cluster.ClusterID, histBatch, batchNum)
			persistenceData[fmt.Sprintf("%d", histBatch)] = persistenceCount
		}
	}

	medoidRow := &types.MedoidRow{
		BatchNumber:     batchNum,
		BatchTime:       batchTime,
		ClusterID:       cluster.ClusterID,
		ClusterSize:     cluster.Size,
		MedoidText:      medoidText,
		BusyWords:       busyWords,
		PersistenceData: persistenceData,
	}

	return medoidRow
}

// buildMedoidDataFromClusters builds the complete medoid data from clusters
func buildMedoidDataFromClusters(clusters []types.Cluster, batchNum int, batchTime string, minClusterSize int, historicalBatchNumbers []int) types.MedoidData {
	var medoidRows []types.MedoidRow

	for _, cluster := range clusters {
		medoidRow := processMedoidCluster(cluster, batchNum, batchTime, minClusterSize, historicalBatchNumbers)
		if medoidRow != nil {
			medoidRows = append(medoidRows, *medoidRow)
		}
	}

	return types.MedoidData{
		CurrentBatch:      batchNum,
		BatchTime:         batchTime,
		HistoricalBatches: historicalBatchNumbers,
		Rows:              medoidRows,
		MinClusterSize:    minClusterSize,
	}
}

func computeMedoidData(batchNum, historicalBatches, minClusterSize int) types.MedoidData {
	batch := allBatches[batchNum]

	// Build historical batch numbers
	historicalBatchNumbers := buildHistoricalBatchNumbers(batchNum, historicalBatches)

	// Extract clusters from batch
	clusters, err := extractMedoidClustersFromBatch(batch)
	if err != nil {
		return types.MedoidData{
			CurrentBatch:      batchNum,
			BatchTime:         batch.Data.BatchTime,
			HistoricalBatches: historicalBatchNumbers,
			Rows:              []types.MedoidRow{},
			MinClusterSize:    minClusterSize,
		}
	}

	// Build medoid data from clusters
	return buildMedoidDataFromClusters(clusters, batchNum, batch.Data.BatchTime, minClusterSize, historicalBatchNumbers)
}

func calculatePersistence(clusterID, histBatch, currentBatch int) int {
	// Simple persistence calculation - returns 1 if cluster exists in historical batch
	// This could be enhanced with Levenshtein distance method as mentioned in the spec

	if histBatch < 0 || histBatch >= len(allBatches) {
		return 0
	}

	histBatchData := allBatches[histBatch]
	for _, cluster := range histBatchData.Data.Clusters {
		if cluster.ClusterID == clusterID {
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

// validateAndLoadBubbleBatch validates the current batch and loads more chunks if needed
func validateAndLoadBubbleBatch(batchNum int) (int, error) {
	if batchNum < 0 {
		return 0, fmt.Errorf("batch number out of range")
	}

	// If batch number is beyond what we have, check if we can load more
	if batchNum >= len(allBatches) {
		fmt.Printf("validateAndLoadBubbleBatch: batch %d >= %d, attempting to load more chunks\n", batchNum, len(allBatches))
		if err := loadMoreChunks(batchNum); err != nil {
			if strings.Contains(err.Error(), "no more batches available") {
				fmt.Printf("validateAndLoadBubbleBatch: reached end of file at batch %d (total batches: %d)\n", batchNum, len(allBatches))
				return -1, fmt.Errorf("end of file reached")
			} else {
				fmt.Printf("validateAndLoadBubbleBatch: actual error loading chunks: %v\n", err)
				return 0, fmt.Errorf("error loading data: %v", err)
			}
		}
	}

	// Double-check that we now have the requested batch
	if batchNum >= len(allBatches) {
		fmt.Printf("validateAndLoadBubbleBatch: batch %d still out of range after loading chunks (total: %d)\n", batchNum, len(allBatches))
		return 0, fmt.Errorf("batch number out of range")
	}

	return batchNum, nil
}

// findBubbleBatchWithData finds a batch that has data, auto-advancing if necessary
func findBubbleBatchWithData(batchNum int) (int, error) {
	originalBatch := batchNum
	hasData := hasDataForBatch(batchNum, config.MinClusterSize)
	fmt.Printf("findBubbleBatchWithData: batch %d hasDataForBatch returned %v\n", batchNum, hasData)

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
				return 0, fmt.Errorf("no batches with data found")
			}
		}
	}

	return batchNum, nil
}

// buildBubbleResponse builds and sends the bubble data response
func buildBubbleResponse(w http.ResponseWriter, batchNum int) {
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

// buildBubbleEndOfFileResponse builds and sends an end-of-file response for bubble data
func buildBubbleEndOfFileResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"current_batch": -1, // Special value to indicate end of file
		"bubble_words":  []interface{}{},
	})
}

func handleBubbleDataAPI(w http.ResponseWriter, r *http.Request) {
	// Use current batch from navigation instead of URL parameter
	currentBatch := getCurrentBatch()
	if currentBatch == nil {
		http.Error(w, "No current batch available", http.StatusNotFound)
		return
	}

	batchNum := currentBatch.Data.BatchNumber

	// Validate and load batch if needed
	validBatchNum, err := validateAndLoadBubbleBatch(batchNum)
	if err != nil {
		if strings.Contains(err.Error(), "end of file reached") {
			buildBubbleEndOfFileResponse(w)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Find a batch with data
	finalBatchNum, err := findBubbleBatchWithData(validBatchNum)
	if err != nil {
		if strings.Contains(err.Error(), "no batches with data found") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{})
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Build and send response
	buildBubbleResponse(w, finalBatchNum)
}

// buildBubbleHistoricalBatchNumbers generates historical batch numbers for bubble data
func buildBubbleHistoricalBatchNumbers(batchNum, historicalBatches int) []int {
	historicalBatchNumbers := make([]int, 0)
	for i := 0; i < historicalBatches; i++ {
		histBatch := batchNum - i
		if histBatch >= 0 {
			historicalBatchNumbers = append(historicalBatchNumbers, histBatch)
		}
	}
	return historicalBatchNumbers
}

// extractBubbleWordsFromBatch extracts busy words from a single batch for bubble data
func extractBubbleWordsFromBatch(batch types.Batch, minClusterSize int) []map[string]interface{} {
	var words []map[string]interface{}
	for _, cluster := range batch.Data.Clusters {
		if cluster.Size < minClusterSize {
			continue
		}

		for _, busyWord := range cluster.BusyWords {
			words = append(words, map[string]interface{}{
				"word":            busyWord.Word,
				"frequency_class": busyWord.Class,
				"z_score":         busyWord.ZScore,
				"count":           float64(busyWord.Count),
				"mean":            busyWord.Mean,
			})
		}
	}

	return words
}

// calculateBubbleSizeFromZScore maps Z-score to bubble size with configurable range
func calculateBubbleSizeFromZScore(zScore float64, allWords []map[string]interface{}) float64 {
	// Use configuration parameters for bubble size scaling
	smallestZ := config.SmallestZ  // Z-score that maps to smallest bubble
	largestZ := config.LargestZ    // Z-score that maps to largest bubble
	k := config.BubbleSizeMultiple // Multiple: largest bubble is k times bigger than smallest

	// Calculate bubble size based on Z-score
	// Z=smallestZ → size=1, Z=largestZ → size=k
	// Z<smallestZ → size<1, Z>largestZ → size>k (unlimited scaling)

	if zScore <= smallestZ {
		// Linear scaling below smallestZ: Z=0 → size=0, Z=smallestZ → size=1
		bubbleSize := zScore / smallestZ
		return bubbleSize
	} else if zScore <= largestZ {
		// Linear scaling between smallestZ and largestZ: Z=smallestZ → size=1, Z=largestZ → size=k
		bubbleSize := 1.0 + (zScore-smallestZ)/(largestZ-smallestZ)*(k-1.0)
		return bubbleSize
	} else {
		// Unlimited scaling above largestZ: Z=largestZ → size=k, Z=∞ → size=∞
		// Use exponential scaling to handle extreme events
		excessZ := zScore - largestZ
		bubbleSize := k * (1.0 + excessZ/10.0) // Each 10 Z-score points above largestZ doubles the size
		return bubbleSize
	}
}

// calculateBubbleSizeFromCountRatio maps actual/mean count ratio to bubble size with configurable range
func calculateBubbleSizeFromCountRatio(actualCount, meanCount float64, allWords []map[string]interface{}) float64 {
	// Use configuration parameters for bubble size scaling
	smallestRatio := config.SmallestRatio // Ratio that maps to smallest bubble
	largestRatio := config.LargestRatio   // Ratio that maps to largest bubble
	k := config.BubbleRatioMultiple       // Multiple: largest bubble is k times bigger than smallest

	// Calculate ratio (actual/mean)
	ratio := actualCount / meanCount

	// Use logarithmic scaling to compress the range
	// This prevents extremely large ratios from creating massive bubbles
	logRatio := math.Log(ratio)
	logSmallest := math.Log(smallestRatio)
	logLargest := math.Log(largestRatio)

	// Calculate bubble size based on log ratio
	// logRatio=logSmallest → size=0.05, logRatio=logLargest → size=0.05*k
	// This makes smallest bubbles smaller while preserving size differences

	if logRatio <= logSmallest {
		// Linear scaling below logSmallest: logRatio=0 → size=0, logRatio=logSmallest → size=0.05
		bubbleSize := (logRatio / logSmallest) * 0.05
		return bubbleSize
	} else if logRatio <= logLargest {
		// Linear scaling between logSmallest and logLargest: logRatio=logSmallest → size=0.05, logRatio=logLargest → size=0.05*k
		bubbleSize := 0.05 + (logRatio-logSmallest)/(logLargest-logSmallest)*(0.05*(k-1.0))
		return bubbleSize
	} else {
		// Unlimited scaling above logLargest: logRatio=logLargest → size=0.05*k, logRatio=∞ → size=∞
		// Use exponential scaling to handle extreme events
		excessLog := logRatio - logLargest
		bubbleSize := 0.05*k + 0.05*k*(excessLog/2.0) // Each 2 log points above logLargest doubles the size
		return bubbleSize
	}
}

// processBubbleWordEvolution handles the complex word tracking and aging logic
func processBubbleWordEvolution(historicalBatchNumbers []int, minClusterSize int) (map[string]*types.BubbleWord, []types.MedoidRow) {
	activeWords := make(map[string]*types.BubbleWord)
	medoidClusters := make([]types.MedoidRow, 0)

	// Process batches from oldest to newest to track word evolution
	for i := len(historicalBatchNumbers) - 1; i >= 0; i-- {
		histBatch := historicalBatchNumbers[i]

		if histBatch >= len(allBatches) {
			continue
		}

		batch := allBatches[histBatch]
		words := extractBubbleWordsFromBatch(batch, minClusterSize)

		// Process each word from this batch
		for _, wordData := range words {
			word := wordData["word"].(string)
			frequencyClass := wordData["frequency_class"].(int)
			zScore := wordData["z_score"].(float64)

			// Calculate divergence score based on configured method
			var divergenceScore float64
			if config.BubbleSizeMethod == "zscore" {
				// Use Z-score based calculation
				divergenceScore = calculateBubbleSizeFromZScore(zScore, words)
			} else {
				// Use count ratio based calculation (actual/mean)
				actualCount, ok1 := wordData["count"].(float64)
				meanCount, ok2 := wordData["mean"].(float64)
				if !ok1 || !ok2 || meanCount == 0 {
					// Fallback to Z-score if count/mean data is invalid
					divergenceScore = calculateBubbleSizeFromZScore(zScore, words)
				} else {
					divergenceScore = calculateBubbleSizeFromCountRatio(actualCount, meanCount, words)
				}
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
				yPosition := float64(frequencyClass) / 24.0

				bubbleWord := &types.BubbleWord{
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

		// Collect medoid data for the side panel (only from current batch)
		if i == 0 {
			medoidClusters = buildBubbleMedoidData(batch, histBatch, minClusterSize)
		}

		// Age all existing words (move them left)
		for _, bubble := range activeWords {
			if bubble.BatchPosition < len(historicalBatchNumbers)-1 {
				bubble.BatchPosition++
			}
		}
	}

	return activeWords, medoidClusters
}

// buildBubbleMedoidData collects medoid data for the bubble side panel
func buildBubbleMedoidData(batch types.Batch, batchNum int, minClusterSize int) []types.MedoidRow {
	var medoidClusters []types.MedoidRow
	for _, cluster := range batch.Data.Clusters {
		if cluster.Size < minClusterSize {
			continue
		}

		medoidText := ""
		if cluster.MedoidTweet != "" {
			medoidText = cluster.MedoidTweet
		} else if len(cluster.Tweets) > 0 {
			medoidText = cluster.Tweets[0].Text
		}

		var busyWords []string
		for _, busyWord := range cluster.BusyWords {
			busyWords = append(busyWords, busyWord.Word)
		}

		medoidRow := types.MedoidRow{
			BatchNumber: batchNum,
			BatchTime:   batch.Data.BatchTime,
			ClusterID:   cluster.ClusterID,
			ClusterSize: cluster.Size,
			MedoidText:  medoidText,
			BusyWords:   busyWords,
		}

		medoidClusters = append(medoidClusters, medoidRow)
	}

	return medoidClusters
}

// assembleBubbleData sorts words and builds the final bubble data response
func assembleBubbleData(activeWords map[string]*types.BubbleWord, medoidClusters []types.MedoidRow, batchNum int, historicalBatchNumbers []int) map[string]interface{} {
	// Convert map to slice and filter out words that have fallen off the left edge
	bubbleWords := make([]types.BubbleWord, 0)
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

func computeBubbleData(batchNum, historicalBatches, minClusterSize int) map[string]interface{} {
	// Build historical batch numbers
	historicalBatchNumbers := buildBubbleHistoricalBatchNumbers(batchNum, historicalBatches)

	// Process word evolution across batches
	activeWords, medoidClusters := processBubbleWordEvolution(historicalBatchNumbers, minClusterSize)

	// Assemble final data
	return assembleBubbleData(activeWords, medoidClusters, batchNum, historicalBatchNumbers)
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

	// Find the specific cluster
	var targetCluster *types.Cluster
	for _, cluster := range batch.Data.Clusters {
		if cluster.ClusterID == clusterID {
			targetCluster = &cluster
			break
		}
	}

	if targetCluster == nil {
		http.Error(w, "Cluster not found", http.StatusNotFound)
		return
	}

	// Extract tweet data
	var tweets []map[string]interface{}
	for _, tweet := range targetCluster.Tweets {
		tweets = append(tweets, map[string]interface{}{
			"text": tweet.Text,
		})
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
