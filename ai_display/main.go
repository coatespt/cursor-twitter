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

// handleGetClusterEvolution handles API request to perform cluster evolution analysis
func (s *Server) handleGetClusterEvolution(w http.ResponseWriter, r *http.Request) {
	clusterIDStr := r.URL.Query().Get("cluster_id")
	batchesBackStr := r.URL.Query().Get("batches_back")
	minMatchingWordsStr := r.URL.Query().Get("min_matching_words")

	if clusterIDStr == "" {
		http.Error(w, "cluster_id is required", http.StatusBadRequest)
		return
	}

	clusterID, err := strconv.Atoi(clusterIDStr)
	if err != nil {
		http.Error(w, "Invalid cluster_id", http.StatusBadRequest)
		return
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

	results, err := s.getClusterEvolution(clusterID, batchesBack, minMatchingWords)
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
		JOIN clusters c ON aar.cluster_id = c.id
		JOIN batches b ON c.batch_id = b.id
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
			aar.result_id,
			aar.session_id,
			aar.cluster_id,
			aar.prompt_text,
			aar.response_text,
			aar.response_metadata,
			aar.analysis_metadata,
			aar.created_at,
			aar.processing_time_ms,
			b.batch_number,
			b.batch_time,
			c.cluster_id as cluster_number
		FROM ai_analysis_results aar
		JOIN clusters c ON aar.cluster_id = c.id
		JOIN batches b ON c.batch_id = b.id
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
			run_id,
			run_name,
			run_date_time,
			window_size,
			batch_size,
			freq_classes,
			min_jaccard_similarity
		FROM experiment_runs
		ORDER BY run_id DESC
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

		err := rows.Scan(
			&runID,
			&runName,
			&runDateTime,
			&windowSize,
			&batchSize,
			&freqClasses,
			&minJaccard,
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
		}
		runs = append(runs, run)
	}

	return runs, nil
}

// getClustersForBatch gets all clusters for a specific batch
func (s *Server) getClustersForBatch(batchNumber, runID int) ([]ClusterInfo, error) {
	query := `
		SELECT 
			c.id as cluster_id,
			c.batch_id,
			b.batch_number,
			b.batch_time,
			c.cluster_id as cluster_number,
			c.size,
			ARRAY_AGG(bw.word ORDER BY bw.word_order) as busy_words,
			COALESCE(ar.response_text, 'No AI analysis available') as ai_summary
		FROM clusters c
		JOIN batches b ON c.batch_id = b.id
		LEFT JOIN busy_words bw ON c.id = bw.cluster_id
		LEFT JOIN ai_analysis_results ar ON c.id = ar.cluster_id
		WHERE b.batch_number = $1 AND b.run_id = $2
		GROUP BY c.id, c.batch_id, b.batch_number, b.batch_time, c.cluster_id, c.size, ar.response_text
		ORDER BY c.cluster_id
	`

	rows, err := s.db.Query(query, batchNumber, runID)
	if err != nil {
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

	// Return empty array instead of nil if no clusters found
	return clusters, nil
}

// getBatchesWithClusters gets only batches that have clusters for a specific run
func (s *Server) getBatchesWithClusters(runID int) ([]map[string]interface{}, error) {
	query := `
		SELECT DISTINCT
			b.batch_number,
			b.batch_time,
			COUNT(c.id) as cluster_count
		FROM batches b
		JOIN clusters c ON b.id = c.batch_id
		WHERE b.run_id = $1
		GROUP BY b.batch_number, b.batch_time
		HAVING COUNT(c.id) > 0
		ORDER BY b.batch_number DESC
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

// getClusterEvolution performs cluster evolution analysis
func (s *Server) getClusterEvolution(clusterID, batchesBack, minMatchingWords int) ([]ClusterEvolutionResult, error) {
	query := `
		WITH target_cluster AS (
			SELECT 
				c.batch_id, 
				c.id as cluster_id, 
				c.cluster_id as cluster_number, 
				c.size, 
				b.batch_number, 
				b.batch_time 
			FROM clusters c 
			JOIN batches b ON c.batch_id = b.id 
			WHERE c.id = $1
		),
		target_busy_words AS (
			SELECT word, frequency_class, word_order 
			FROM busy_words 
			WHERE cluster_id = $1
			ORDER BY word_order
		),
		batch_matches AS (
			SELECT 
				c.batch_id, 
				c.id as cluster_id, 
				c.cluster_id as cluster_number, 
				c.size, 
				COUNT(bw.word) as matching_words,
				ARRAY_AGG(bw.word ORDER BY bw.word_order) as matching_words_list,
				(SELECT batch_id FROM target_cluster) - c.batch_id as batches_back
			FROM clusters c 
			JOIN busy_words bw ON c.id = bw.cluster_id 
			WHERE bw.word IN (SELECT word FROM target_busy_words) 
				AND c.batch_id < (SELECT batch_id FROM target_cluster)
				AND c.batch_id >= (SELECT batch_id FROM target_cluster) - $2
			GROUP BY c.batch_id, c.id, c.cluster_id, c.size 
			HAVING COUNT(bw.word) >= $3
			ORDER BY c.batch_id DESC, matching_words DESC
		),
		all_clusters_with_ai AS (
			SELECT 
				'TARGET CLUSTER' as type,
				tc.batch_id,
				tc.cluster_id,
				tc.cluster_number,
				tc.size,
				tc.batch_number,
				tc.batch_time,
				ARRAY(SELECT word FROM target_busy_words ORDER BY word_order) as busy_words,
				0 as batches_back,
				COALESCE(ar.response_text, 'No AI analysis available') as ai_summary
			FROM target_cluster tc
			LEFT JOIN ai_analysis_results ar ON tc.cluster_id = ar.cluster_id

			UNION ALL

			SELECT 
				'MATCHING CLUSTERS' as type,
				bm.batch_id,
				bm.cluster_id,
				bm.cluster_number,
				bm.size,
				b.batch_number,
				b.batch_time,
				bm.matching_words_list as busy_words,
				bm.batches_back,
				COALESCE(ar.response_text, 'No AI analysis available') as ai_summary
			FROM batch_matches bm 
			JOIN batches b ON bm.batch_id = b.id 
			LEFT JOIN ai_analysis_results ar ON bm.cluster_id = ar.cluster_id
		)
		SELECT 
			type,
			cluster_id,
			batch_id,
			batch_number,
			batch_time,
			cluster_number,
			size,
			busy_words,
			batches_back,
			ai_summary
		FROM all_clusters_with_ai
		ORDER BY batch_id DESC, type DESC
	`

	rows, err := s.db.Query(query, clusterID, batchesBack, minMatchingWords)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ClusterEvolutionResult
	for rows.Next() {
		var result ClusterEvolutionResult
		var busyWordsStr string
		err := rows.Scan(
			&result.Type,
			&result.ClusterID,
			&result.BatchID,
			&result.BatchNumber,
			&result.BatchTime,
			&result.ClusterNumber,
			&result.Size,
			&busyWordsStr,
			&result.BatchesBack,
			&result.AISummary,
		)
		if err != nil {
			return nil, err
		}

		// Parse busy words array
		if busyWordsStr != "{}" && busyWordsStr != "" {
			result.BusyWords = parsePostgreSQLArray(busyWordsStr)
		}

		results = append(results, result)
	}

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

	// Serve static files
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	addr := fmt.Sprintf("%s:%d", server.config.Server.Host, server.config.Server.Port)
	log.Printf("Starting AI Display server on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
