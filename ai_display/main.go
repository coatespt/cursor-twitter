package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Server struct {
		Port int    `yaml:"port"`
		Host string `yaml:"host"`
	} `yaml:"server"`
	Database struct {
		Host     string `yaml:"host"`
		Port     int    `yaml:"port"`
		Name     string `yaml:"name"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
		SSLMode  string `yaml:"sslmode"`
	} `yaml:"database"`
	Display struct {
		BatchWindowSize    int    `yaml:"batch_window_size"`
		DefaultViewingMode string `yaml:"default_viewing_mode"`
		AutoAdvance        bool   `yaml:"auto_advance"`
	} `yaml:"display"`
	ClusterEvolution struct {
		DefaultBatchesBack      int `yaml:"default_batches_back"`
		DefaultMinMatchingWords int `yaml:"default_min_matching_words"`
	} `yaml:"cluster_evolution"`
}

// AnalysisResult represents a single AI analysis result
type AnalysisResult struct {
	ResultID         int            `json:"result_id"`
	SessionID        int            `json:"session_id"`
	ClusterID        int            `json:"cluster_id"`
	PromptText       string         `json:"prompt_text"`
	ResponseText     string         `json:"response_text"`
	ResponseMetadata sql.NullString `json:"response_metadata"`
	AnalysisMetadata sql.NullString `json:"analysis_metadata"`
	CreatedAt        time.Time      `json:"created_at"`
	ProcessingTimeMs int            `json:"processing_time_ms"`
	BatchNumber      int            `json:"batch_number"`
	BatchTime        time.Time      `json:"batch_time"`
	ClusterNumber    int            `json:"cluster_number"`
}

// ClusterEvolutionResult represents a cluster evolution analysis result
type ClusterEvolutionResult struct {
	Type                 string    `json:"type"`
	ClusterID            int       `json:"cluster_id"`
	BatchID              int       `json:"batch_id"`
	BatchNumber          int       `json:"batch_number"`
	BatchTime            time.Time `json:"batch_time"`
	ClusterNumber        int       `json:"cluster_number"`
	Size                 int       `json:"size"`
	BusyWords            []string  `json:"busy_words"`
	BatchesBack          int       `json:"batches_back"`
	AISummary            string    `json:"ai_summary"`
	MedoidTweet          string    `json:"medoid_tweet"`
	SimilarClustersCount int       `json:"similar_clusters_count"`
}

// ClusterInfo represents basic cluster information for selection
type ClusterInfo struct {
	ClusterID     int       `json:"cluster_id"`
	BatchID       int       `json:"batch_id"`
	BatchNumber   int       `json:"batch_number"`
	BatchTime     time.Time `json:"batch_time"`
	ClusterNumber int       `json:"cluster_number"`
	Size          int       `json:"size"`
	BusyWords     []string  `json:"busy_words"`
	AISummary     string    `json:"ai_summary"`
	HistoryDepth  int       `json:"history_depth"`
}

// DatasetInfo represents information about the dataset
type DatasetInfo struct {
	TotalBatches    int       `json:"total_batches"`
	TotalClusters   int       `json:"total_clusters"`
	OldestBatchTime time.Time `json:"oldest_batch_time"`
	NewestBatchTime time.Time `json:"newest_batch_time"`
	BatchSize       int       `json:"batch_size"`
}

// Server represents the web server
type Server struct {
	db     *sql.DB
	config *Config
	tmpl   *template.Template
}

// NewServer creates a new server instance
func NewServer(configPath string) (*Server, error) {
	// Load configuration
	config, err := loadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %v", err)
	}

	// Connect to database
	db, err := connectDB(config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %v", err)
	}

	// Load templates
	tmpl, err := template.ParseGlob("templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %v", err)
	}

	return &Server{
		db:     db,
		config: config,
		tmpl:   tmpl,
	}, nil
}

// loadConfig loads configuration from file
func loadConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// connectDB connects to the PostgreSQL database
func connectDB(config *Config) (*sql.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		config.Database.Host, config.Database.Port, config.Database.Name,
		config.Database.User, config.Database.Password, config.Database.SSLMode)

	return sql.Open("postgres", dsn)
}

// handleIndex handles the main page
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Get dataset information
	datasetInfo, err := s.getDatasetInfo()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get dataset info: %v", err), http.StatusInternalServerError)
		return
	}

	// Get experiment runs
	experimentRuns, err := s.getExperimentRuns()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get experiment runs: %v", err), http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"DatasetInfo":    datasetInfo,
		"ExperimentRuns": experimentRuns,
		"Config":         s.config,
	}

	if err := s.tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, fmt.Sprintf("Failed to execute template: %v", err), http.StatusInternalServerError)
	}
}

// handleGetBatches handles API request to get batches
func (s *Server) handleGetBatches(w http.ResponseWriter, r *http.Request) {
	startBatchStr := r.URL.Query().Get("start_batch")
	limitStr := r.URL.Query().Get("limit")
	runIDStr := r.URL.Query().Get("run_id")

	startBatch := 0
	if startBatchStr != "" {
		if val, err := strconv.Atoi(startBatchStr); err == nil {
			startBatch = val
		}
	}

	limit := s.config.Display.BatchWindowSize
	if limitStr != "" {
		if val, err := strconv.Atoi(limitStr); err == nil {
			limit = val
		}
	}

	runID := 1 // Default to first run
	if runIDStr != "" {
		if val, err := strconv.Atoi(runIDStr); err == nil {
			runID = val
		}
	}

	results, err := s.getAnalysisResults(startBatch, limit, runID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get analysis results: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// handleGetExperimentRuns handles API request to get all experiment runs
func (s *Server) handleGetExperimentRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := s.getExperimentRuns()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get experiment runs: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runs)
}

// handleGetClustersForBatch handles API request to get clusters for a specific batch
func (s *Server) handleGetClustersForBatch(w http.ResponseWriter, r *http.Request) {
	batchNumberStr := r.URL.Query().Get("batch_number")
	runIDStr := r.URL.Query().Get("run_id")

	if batchNumberStr == "" || runIDStr == "" {
		http.Error(w, "batch_number and run_id are required", http.StatusBadRequest)
		return
	}

	batchNumber, err := strconv.Atoi(batchNumberStr)
	if err != nil {
		http.Error(w, "Invalid batch_number", http.StatusBadRequest)
		return
	}

	runID, err := strconv.Atoi(runIDStr)
	if err != nil {
		http.Error(w, "Invalid run_id", http.StatusBadRequest)
		return
	}

	clusters, err := s.getClustersForBatch(batchNumber, runID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get clusters: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(clusters)
}

// handleGetBatchesWithClusters handles API request to get batches that have clusters
func (s *Server) handleGetBatchesWithClusters(w http.ResponseWriter, r *http.Request) {
	runIDStr := r.URL.Query().Get("run_id")

	if runIDStr == "" {
		http.Error(w, "run_id is required", http.StatusBadRequest)
		return
	}

	runID, err := strconv.Atoi(runIDStr)
	if err != nil {
		http.Error(w, "Invalid run_id", http.StatusBadRequest)
		return
	}

	batches, err := s.getBatchesWithClusters(runID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get batches: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(batches)
}

// handleGetBatchesWithAIAnalysisForEvolution handles API request to get batches that have clusters with AI analysis for evolution mode
func (s *Server) handleGetBatchesWithAIAnalysisForEvolution(w http.ResponseWriter, r *http.Request) {
	runIDStr := r.URL.Query().Get("run_id")

	if runIDStr == "" {
		http.Error(w, "run_id is required", http.StatusBadRequest)
		return
	}

	runID, err := strconv.Atoi(runIDStr)
	if err != nil {
		http.Error(w, "Invalid run_id", http.StatusBadRequest)
		return
	}

	batches, err := s.getBatchesWithAIAnalysisForEvolution(runID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get batches with AI analysis: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(batches)
}

// handleGetClusterEvolution handles API request to perform cluster evolution analysis
func (s *Server) handleGetClusterEvolution(w http.ResponseWriter, r *http.Request) {
	clusterIDStr := r.URL.Query().Get("cluster_id")
	batchNumberStr := r.URL.Query().Get("batch_number")
	runIDStr := r.URL.Query().Get("run_id")
	batchesBackStr := r.URL.Query().Get("batches_back")
	minMatchingWordsStr := r.URL.Query().Get("min_matching_words")

	if clusterIDStr == "" {
		http.Error(w, "cluster_id is required", http.StatusBadRequest)
		return
	}

	if batchNumberStr == "" {
		http.Error(w, "batch_number is required", http.StatusBadRequest)
		return
	}

	clusterID, err := strconv.Atoi(clusterIDStr)
	if err != nil {
		http.Error(w, "Invalid cluster_id", http.StatusBadRequest)
		return
	}

	batchNumber, err := strconv.Atoi(batchNumberStr)
	if err != nil {
		http.Error(w, "Invalid batch_number", http.StatusBadRequest)
		return
	}

	runID := 1 // Default to first run
	if runIDStr != "" {
		if val, err := strconv.Atoi(runIDStr); err == nil {
			runID = val
		}
	}

	batchesBack := 20 // Default value
	if batchesBackStr != "" {
		if val, err := strconv.Atoi(batchesBackStr); err == nil {
			batchesBack = val
		}
	}

	minMatchingWords := 2 // Default value
	if minMatchingWordsStr != "" {
		if val, err := strconv.Atoi(minMatchingWordsStr); err == nil {
			minMatchingWords = val
		}
	}

	results, err := s.getClusterEvolution(clusterID, batchNumber, runID, batchesBack, minMatchingWords)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get cluster evolution: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// getDatasetInfo gets information about the dataset
func (s *Server) getDatasetInfo() (*DatasetInfo, error) {
	query := `
		SELECT 
			COUNT(DISTINCT b.batch_number) as total_batches,
			COUNT(DISTINCT aar.cluster_id) as total_clusters,
			MIN(b.batch_time) as oldest_batch_time,
			MAX(b.batch_time) as newest_batch_time,
			AVG(b.total_tweets)::integer as batch_size
		FROM ai_analysis_results aar
		JOIN new_clusters c ON aar.cluster_id = c.id
		JOIN new_batches b ON c.batch_id = b.id
	`

	var info DatasetInfo
	err := s.db.QueryRow(query).Scan(
		&info.TotalBatches,
		&info.TotalClusters,
		&info.OldestBatchTime,
		&info.NewestBatchTime,
		&info.BatchSize,
	)

	if err != nil {
		return nil, err
	}

	return &info, nil
}

// getAnalysisResults gets analysis results for a range of batches
func (s *Server) getAnalysisResults(startBatch, limit, runID int) ([]AnalysisResult, error) {
	query := `
		SELECT 
			COALESCE(aar.result_id, 0) as result_id,
			COALESCE(aar.session_id, 0) as session_id,
			c.id as cluster_id,
			COALESCE(aar.prompt_text, '') as prompt_text,
			COALESCE(aar.response_text, '') as response_text,
			COALESCE(aar.response_metadata::text, '') as response_metadata,
			COALESCE(aar.analysis_metadata::text, '') as analysis_metadata,
			COALESCE(aar.created_at, NOW()) as created_at,
			COALESCE(aar.processing_time_ms, 0) as processing_time_ms,
			b.batch_number,
			b.batch_time,
			c.cluster_id as cluster_number
		FROM new_batches b
		JOIN new_clusters c ON b.id = c.batch_id
		LEFT JOIN ai_analysis_results aar ON c.id = aar.cluster_id
		WHERE b.batch_number >= $1 AND b.run_id = $3
		ORDER BY b.batch_number, c.cluster_id
		LIMIT $2
	`

	rows, err := s.db.Query(query, startBatch, limit*10, runID) // Estimate 10 clusters per batch
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []AnalysisResult
	for rows.Next() {
		var result AnalysisResult
		err := rows.Scan(
			&result.ResultID,
			&result.SessionID,
			&result.ClusterID,
			&result.PromptText,
			&result.ResponseText,
			&result.ResponseMetadata,
			&result.AnalysisMetadata,
			&result.CreatedAt,
			&result.ProcessingTimeMs,
			&result.BatchNumber,
			&result.BatchTime,
			&result.ClusterNumber,
		)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, nil
}

// getExperimentRuns gets all available experiment runs
func (s *Server) getExperimentRuns() ([]map[string]interface{}, error) {
	query := `
		SELECT 
			er.run_id,
			er.run_name,
			er.run_date_time,
			er.window_size,
			er.batch_size,
			er.freq_classes,
			er.min_jaccard_similarity,
			COUNT(DISTINCT aar.result_id) as ai_analysis_count
		FROM new_experiment_runs er
		LEFT JOIN ai_analysis_results aar ON er.run_id = (
			SELECT b.run_id 
			FROM new_clusters c 
			JOIN new_batches b ON c.batch_id = b.id 
			WHERE c.id = aar.cluster_id
		)
		GROUP BY er.run_id, er.run_name, er.run_date_time, er.window_size, er.batch_size, er.freq_classes, er.min_jaccard_similarity
		ORDER BY er.run_id DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []map[string]interface{}
	for rows.Next() {
		var runID int
		var runName, runDateTime string
		var windowSize, batchSize, freqClasses sql.NullInt32
		var minJaccard sql.NullFloat64
		var aiAnalysisCount int

		err := rows.Scan(
			&runID,
			&runName,
			&runDateTime,
			&windowSize,
			&batchSize,
			&freqClasses,
			&minJaccard,
			&aiAnalysisCount,
		)
		if err != nil {
			return nil, err
		}

		run := map[string]interface{}{
			"run_id":                 runID,
			"run_name":               runName,
			"run_date_time":          runDateTime,
			"window_size":            windowSize.Int32,
			"batch_size":             batchSize.Int32,
			"freq_classes":           freqClasses.Int32,
			"min_jaccard_similarity": minJaccard.Float64,
			"ai_analysis_count":      aiAnalysisCount,
		}
		runs = append(runs, run)
	}

	return runs, nil
}

// getClustersForBatch gets all clusters for a specific batch
func (s *Server) getClustersForBatch(batchNumber, runID int) ([]ClusterInfo, error) {
	query := `
		SELECT 
			c.cluster_id,
			c.batch_id,
			b.batch_number,
			b.batch_time,
			c.cluster_id as cluster_number,
			c.size,
			COALESCE(ARRAY_AGG(bw.word ORDER BY bw.word_order) FILTER (WHERE bw.word IS NOT NULL), ARRAY[]::text[]) as busy_words,
			COALESCE(ar.response_text, 'No AI analysis available') as ai_summary,
			0 as history_depth
		FROM new_clusters c
		JOIN new_batches b ON c.batch_id = b.id
		LEFT JOIN new_busy_words bw ON c.id = bw.cluster_id
		LEFT JOIN ai_analysis_results ar ON c.id = ar.cluster_id
		WHERE b.batch_number = $1 AND b.run_id = $2
		GROUP BY c.cluster_id, c.batch_id, b.batch_number, b.batch_time, c.size, ar.response_text
		ORDER BY c.cluster_id
	`

	fmt.Printf("DEBUG: getClustersForBatch called with batchNumber=%d, runID=%d\n", batchNumber, runID)
	rows, err := s.db.Query(query, batchNumber, runID)
	if err != nil {
		fmt.Printf("DEBUG: Query error: %v\n", err)
		return nil, err
	}
	defer rows.Close()

	var clusters []ClusterInfo
	for rows.Next() {
		var cluster ClusterInfo
		var busyWordsStr string
		err := rows.Scan(
			&cluster.ClusterID,
			&cluster.BatchID,
			&cluster.BatchNumber,
			&cluster.BatchTime,
			&cluster.ClusterNumber,
			&cluster.Size,
			&busyWordsStr,
			&cluster.AISummary,
			&cluster.HistoryDepth,
		)
		if err != nil {
			return nil, err
		}

		// Parse busy words array (PostgreSQL array format: {word1,word2})
		if busyWordsStr != "{}" && busyWordsStr != "" {
			// Simple parsing of PostgreSQL array format
			words := parsePostgreSQLArray(busyWordsStr)
			cluster.BusyWords = words
		}

		clusters = append(clusters, cluster)
	}

	fmt.Printf("DEBUG: Found %d clusters for batch %d\n", len(clusters), batchNumber)
	// Return empty array instead of nil if no clusters found
	return clusters, nil
}

// getBatchesWithClusters gets all batches that have clusters for a specific run
func (s *Server) getBatchesWithClusters(runID int) ([]map[string]interface{}, error) {
	query := `
		SELECT DISTINCT
			b.batch_number,
			b.batch_time,
			COUNT(c.cluster_id) as cluster_count
		FROM new_batches b
		JOIN new_clusters c ON b.id = c.batch_id
		WHERE b.run_id = $1
		GROUP BY b.batch_number, b.batch_time
		HAVING COUNT(c.cluster_id) > 0
		ORDER BY b.batch_number ASC
	`

	rows, err := s.db.Query(query, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var batches []map[string]interface{}
	for rows.Next() {
		var batchNumber int
		var batchTime time.Time
		var clusterCount int

		err := rows.Scan(&batchNumber, &batchTime, &clusterCount)
		if err != nil {
			return nil, err
		}

		batch := map[string]interface{}{
			"batch_number":  batchNumber,
			"batch_time":    batchTime,
			"cluster_count": clusterCount,
		}
		batches = append(batches, batch)
	}

	return batches, nil
}

// getBatchesWithAIAnalysisForEvolution gets only batches that have clusters with AI analysis for evolution mode
func (s *Server) getBatchesWithAIAnalysisForEvolution(runID int) ([]map[string]interface{}, error) {
	// First, let's check if there are any AI analysis results for this run_id
	checkQuery := `
		SELECT COUNT(*) 
		FROM ai_analysis_results aar
		JOIN new_clusters c ON aar.cluster_id = c.id
		JOIN new_batches b ON c.batch_id = b.id
		WHERE b.run_id = $1
	`
	var aiResultCount int
	err := s.db.QueryRow(checkQuery, runID).Scan(&aiResultCount)
	if err != nil {
		return nil, fmt.Errorf("failed to check AI analysis results: %v", err)
	}

	// Log the count for debugging
	fmt.Printf("DEBUG: Found %d AI analysis results for run_id %d\n", aiResultCount, runID)

	if aiResultCount == 0 {
		return []map[string]interface{}{}, nil
	}

	// Simplified query - just get batches with AI analysis, skip complex persistence calculation (optimized)
	query := `
		SELECT DISTINCT
			b.batch_number,
			b.batch_time,
			COUNT(DISTINCT c.cluster_id) as cluster_count,
			0 as persistent_count
		FROM new_batches b
		JOIN new_clusters c ON b.id = c.batch_id
		JOIN ai_analysis_results aar ON c.id = aar.cluster_id
		WHERE b.run_id = $1
		GROUP BY b.batch_number, b.batch_time
		ORDER BY b.batch_number ASC
	`

	rows, err := s.db.Query(query, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var batches []map[string]interface{}
	for rows.Next() {
		var batchNumber int
		var batchTime time.Time
		var clusterCount int
		var persistentCount int

		err := rows.Scan(&batchNumber, &batchTime, &clusterCount, &persistentCount)
		if err != nil {
			return nil, err
		}

		batch := map[string]interface{}{
			"batch_number":     batchNumber,
			"batch_time":       batchTime,
			"cluster_count":    clusterCount,
			"persistent_count": persistentCount,
		}
		batches = append(batches, batch)

		// Debug logging for first few batches
		if len(batches) <= 5 {
			fmt.Printf("DEBUG: Batch %d: %d clusters, %d persistent\n", batchNumber, clusterCount, persistentCount)
		}
	}

	fmt.Printf("DEBUG: Returning %d batches with AI analysis for run_id %d\n", len(batches), runID)
	return batches, nil
}

// handleBatchHasPersistentClusters checks if a batch has clusters with history depth > 0
func (s *Server) handleBatchHasPersistentClusters(w http.ResponseWriter, r *http.Request) {
	batchNumberStr := r.URL.Query().Get("batch_number")
	runIDStr := r.URL.Query().Get("run_id")

	if batchNumberStr == "" || runIDStr == "" {
		http.Error(w, "batch_number and run_id are required", http.StatusBadRequest)
		return
	}

	batchNumber, err := strconv.Atoi(batchNumberStr)
	if err != nil {
		http.Error(w, "Invalid batch_number", http.StatusBadRequest)
		return
	}

	runID, err := strconv.Atoi(runIDStr)
	if err != nil {
		http.Error(w, "Invalid run_id", http.StatusBadRequest)
		return
	}

	hasPersistent, err := s.batchHasPersistentClusters(runID, batchNumber)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to check persistent clusters: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"has_persistent": hasPersistent})
}

// batchHasPersistentClusters checks if a batch has any clusters with history depth > 0
func (s *Server) batchHasPersistentClusters(runID, batchNumber int) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1 FROM new_clusters c
			JOIN new_batches b ON c.batch_id = b.id
			JOIN ai_analysis_results aar ON c.id = aar.cluster_id
			WHERE b.run_id = $1 AND b.batch_number = $2
			AND EXISTS (
				SELECT 1 FROM new_busy_words bw1
				JOIN new_clusters c1 ON bw1.cluster_id = c1.id
				JOIN new_batches b1 ON c1.batch_id = b1.id
				WHERE b1.run_id = $1
				AND b1.batch_number < $2
				AND bw1.word IN (
					SELECT bw2.word FROM new_busy_words bw2
					WHERE bw2.cluster_id = c.cluster_id
				)
			)
		)
	`

	var hasPersistent bool
	err := s.db.QueryRow(query, runID, batchNumber).Scan(&hasPersistent)
	return hasPersistent, err
}

// getClusterEvolution performs cluster evolution analysis - finds matching clusters from previous batches
func (s *Server) getClusterEvolution(clusterID, batchNumber, runID, batchesBack, minMatchingWords int) ([]ClusterEvolutionResult, error) {
	fmt.Printf("DEBUG: getClusterEvolution called with clusterID=%d, batchNumber=%d, runID=%d, batchesBack=%d, minMatchingWords=%d\n", clusterID, batchNumber, runID, batchesBack, minMatchingWords)

	query := `
		WITH target_cluster AS (
			SELECT c.id, c.cluster_id, c.batch_id, b.batch_number, b.batch_time, c.size
			FROM new_clusters c 
			JOIN new_batches b ON c.batch_id = b.id 
			WHERE c.cluster_id = $1 AND b.batch_number = $2 AND b.run_id = $3
		),
		target_busy_words AS (
			SELECT ARRAY_AGG(bw.word) as words
			FROM new_busy_words bw
			WHERE bw.cluster_id = (SELECT id FROM target_cluster)
		),
		similar_clusters AS (
			SELECT 
				c.id as cluster_internal_id,
				c.batch_id, 
				c.cluster_id, 
				c.cluster_id as cluster_number, 
				c.size, 
				b.batch_number,
				b.batch_time,
				ARRAY_AGG(bw.word ORDER BY bw.word_order) as busy_words,
				COUNT(bw.word) as matching_words,
				(SELECT tc.batch_number - b.batch_number FROM target_cluster tc) as batches_back
			FROM new_clusters c
			JOIN new_batches b ON c.batch_id = b.id
			JOIN new_busy_words bw ON c.id = bw.cluster_id
			CROSS JOIN target_busy_words
			WHERE b.run_id = (SELECT b2.run_id FROM target_cluster tc2 JOIN new_batches b2 ON tc2.batch_id = b2.id)
			  AND b.batch_number < (SELECT batch_number FROM target_cluster)
			  AND b.batch_number >= (SELECT batch_number FROM target_cluster) - $4
			  AND bw.word = ANY(target_busy_words.words)
			GROUP BY c.id, c.batch_id, c.cluster_id, c.size, b.batch_number, b.batch_time
			HAVING COUNT(bw.word) >= $5
			ORDER BY matching_words DESC, b.batch_number DESC
		)
		SELECT 
			'TARGET CLUSTER' as type,
			tc.batch_id,
			tc.cluster_id,
			tc.cluster_id as cluster_number,
			tc.size,
			tc.batch_number,
			tc.batch_time,
			ARRAY(SELECT word FROM new_busy_words WHERE cluster_id = tc.id ORDER BY word_order) as busy_words,
			0 as batches_back,
			COALESCE(ar.response_text, 'No AI analysis available') as ai_summary,
			COALESCE((SELECT t.tweet_text FROM new_tweets t WHERE t.cluster_id = tc.id AND t.is_medoid = true LIMIT 1), 'No medoid tweet available') as medoid_tweet
		FROM target_cluster tc
		LEFT JOIN ai_analysis_results ar ON tc.id = ar.cluster_id

		UNION ALL

		SELECT 
			'MATCHING CLUSTERS' as type,
			sc.batch_id,
			sc.cluster_id,
			sc.cluster_number,
			sc.size,
			sc.batch_number,
			sc.batch_time,
			sc.busy_words,
			sc.batches_back,
			COALESCE(ar.response_text, 'No AI analysis available') as ai_summary,
			COALESCE((SELECT t.tweet_text FROM new_tweets t WHERE t.cluster_id = sc.cluster_internal_id AND t.is_medoid = true LIMIT 1), 'No medoid tweet available') as medoid_tweet
		FROM similar_clusters sc 
		LEFT JOIN ai_analysis_results ar ON sc.cluster_internal_id = ar.cluster_id
		ORDER BY batch_id ASC, type DESC
	`

	rows, err := s.db.Query(query, clusterID, batchNumber, runID, batchesBack, minMatchingWords)
	if err != nil {
		fmt.Printf("DEBUG: Query error: %v\n", err)
		return nil, err
	}
	defer rows.Close()

	var results []ClusterEvolutionResult
	fmt.Printf("DEBUG: Query executed successfully\n")
	for rows.Next() {
		var result ClusterEvolutionResult
		var busyWordsStr string
		err := rows.Scan(
			&result.Type,
			&result.BatchID,
			&result.ClusterID,
			&result.ClusterNumber,
			&result.Size,
			&result.BatchNumber,
			&result.BatchTime,
			&busyWordsStr,
			&result.BatchesBack,
			&result.AISummary,
			&result.MedoidTweet,
		)
		if err != nil {
			return nil, err
		}

		// Parse busy words array
		if busyWordsStr != "{}" && busyWordsStr != "" {
			result.BusyWords = parsePostgreSQLArray(busyWordsStr)
		}

		// Initialize SimilarClustersCount to 0 since it's not in the query
		result.SimilarClustersCount = 0

		results = append(results, result)
	}

	fmt.Printf("DEBUG: Found %d evolution results\n", len(results))
	return results, nil
}

// parsePostgreSQLArray parses PostgreSQL array format {word1,word2} into Go slice
func parsePostgreSQLArray(arrayStr string) []string {
	if len(arrayStr) < 2 {
		return []string{}
	}

	// Remove { and }
	content := arrayStr[1 : len(arrayStr)-1]
	if content == "" {
		return []string{}
	}

	// Split by comma
	words := strings.Split(content, ",")
	var result []string
	for _, word := range words {
		trimmed := strings.TrimSpace(word)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

// Close closes the database connection
func (s *Server) Close() error {
	return s.db.Close()
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("Usage: ai_display <config_file>")
	}

	configPath := os.Args[1]

	server, err := NewServer(configPath)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}
	defer server.Close()

	// Set up routes
	http.HandleFunc("/", server.handleIndex)
	http.HandleFunc("/api/batches", server.handleGetBatches)
	http.HandleFunc("/api/experiment-runs", server.handleGetExperimentRuns)
	http.HandleFunc("/api/clusters", server.handleGetClustersForBatch)
	http.HandleFunc("/api/cluster-evolution", server.handleGetClusterEvolution)
	http.HandleFunc("/api/batches-with-clusters", server.handleGetBatchesWithClusters)
	http.HandleFunc("/api/batches-with-ai-analysis", server.handleGetBatchesWithAIAnalysisForEvolution)
	http.HandleFunc("/api/batch-has-persistent-clusters", server.handleBatchHasPersistentClusters)

	// Serve static files
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	addr := fmt.Sprintf("%s:%d", server.config.Server.Host, server.config.Server.Port)
	log.Printf("Starting AI Display server on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
