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
	tmpl, err := template.ParseGlob("ai_display/templates/*.html")
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
			"run_id":                runID,
			"run_name":              runName,
			"run_date_time":         runDateTime,
			"window_size":           windowSize.Int32,
			"batch_size":            batchSize.Int32,
			"freq_classes":          freqClasses.Int32,
			"min_jaccard_similarity": minJaccard.Float64,
		}
		runs = append(runs, run)
	}

	return runs, nil
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

	// Serve static files
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("ai_display/static"))))

	addr := fmt.Sprintf("%s:%d", server.config.Server.Host, server.config.Server.Port)
	log.Printf("Starting AI Display server on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
