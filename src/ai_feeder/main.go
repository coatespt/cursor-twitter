package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"text/template"
	"time"

	_ "github.com/lib/pq"
	"gopkg.in/yaml.v3"
)

// AIConfig holds the AI service configuration
type AIConfig struct {
	AI struct {
		Model    string `yaml:"model"`    // e.g., "llama3.1:8b"
		Endpoint string `yaml:"endpoint"` // e.g., "http://localhost:11434/api/generate"
		Timeout  int    `yaml:"timeout"`  // seconds
	} `yaml:"ai"`

	Database struct {
		Host     string `yaml:"host"`
		Port     int    `yaml:"port"`
		Name     string `yaml:"name"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
		SSLMode  string `yaml:"ssl_mode"`
	} `yaml:"database"`

	Processing struct {
		BatchSize        int    `yaml:"batch_size"`         // Clusters to process in parallel
		MaxRetries       int    `yaml:"max_retries"`        // Max retries per request
		RetryDelay       int    `yaml:"retry_delay"`        // Seconds between retries
		PromptTemplate   string `yaml:"prompt_template"`    // Path to prompt template file
		AnalysisType     string `yaml:"analysis_type"`      // Type of analysis to perform
		SessionName      string `yaml:"session_name"`       // Name for this analysis session
		RunID            int    `yaml:"run_id"`             // Experiment run ID to analyze
		MaxClusters      int    `yaml:"max_clusters"`       // Max clusters to process (0 = all)
		StartFromCluster int    `yaml:"start_from_cluster"` // Start from specific cluster ID
	} `yaml:"processing"`
}

// OllamaRequest represents the request to Ollama API
type OllamaRequest struct {
	Model   string `json:"model"`
	Prompt  string `json:"prompt"`
	Stream  bool   `json:"stream"`
	Options struct {
		Temperature float64 `json:"temperature"`
		TopP        float64 `json:"top_p"`
	} `json:"options"`
}

// OllamaResponse represents the response from Ollama API
type OllamaResponse struct {
	Model              string `json:"model"`
	CreatedAt          string `json:"created_at"`
	Response           string `json:"response"`
	Done               bool   `json:"done"`
	Context            []int  `json:"context"`
	TotalDuration      int64  `json:"total_duration"`
	LoadDuration       int64  `json:"load_duration"`
	PromptEvalCount    int    `json:"prompt_eval_count"`
	PromptEvalDuration int64  `json:"prompt_eval_duration"`
	EvalCount          int    `json:"eval_count"`
	EvalDuration       int64  `json:"eval_duration"`
}

// ClusterData represents the data extracted from database for AI analysis
type ClusterData struct {
	ClusterID      int      `json:"cluster_id"`
	BatchNumber    int      `json:"batch_number"`
	BatchTime      string   `json:"batch_time"`
	Size           int      `json:"size"`
	QualityScore   float64  `json:"quality_score"`
	Tweets         []string `json:"tweets"`
	MedoidTweet    string   `json:"medoid_tweet"`
	BusyWords      []string `json:"busy_words"`
	FrequencyClass int      `json:"frequency_class"`
}

// AIFeeder handles the AI analysis pipeline
type AIFeeder struct {
	db     *sql.DB
	config *AIConfig
	client *http.Client
}

// NewAIFeeder creates a new AI feeder with database connection and HTTP client
func NewAIFeeder(configPath string) (*AIFeeder, error) {
	// Load configuration
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	var config AIConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %v", err)
	}

	// Connect to database
	dsn := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		config.Database.Host, config.Database.Port, config.Database.Name,
		config.Database.User, config.Database.Password, config.Database.SSLMode)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %v", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %v", err)
	}

	// Create HTTP client with timeout
	timeout := time.Duration(config.AI.Timeout) * time.Second
	client := &http.Client{
		Timeout: timeout,
	}

	return &AIFeeder{
		db:     db,
		config: &config,
		client: client,
	}, nil
}

// Close closes the database connection
func (af *AIFeeder) Close() error {
	return af.db.Close()
}

// CreateAnalysisSession creates a new AI analysis session
func (af *AIFeeder) CreateAnalysisSession() (int, error) {
	// Read prompt template
	promptTemplate, err := os.ReadFile(af.config.Processing.PromptTemplate)
	if err != nil {
		return 0, fmt.Errorf("failed to read prompt template: %v", err)
	}

	// Count total clusters for this run
	var totalClusters int
	err = af.db.QueryRow(`
		SELECT COUNT(*) FROM clusters c
		JOIN batches b ON c.batch_id = b.id
		WHERE b.run_id = $1
	`, af.config.Processing.RunID).Scan(&totalClusters)
	if err != nil {
		return 0, fmt.Errorf("failed to count clusters: %v", err)
	}

	// Insert session
	var sessionID int
	err = af.db.QueryRow(`
		INSERT INTO ai_analysis_sessions (
			run_id, session_name, ai_model, ai_endpoint, prompt_template,
			analysis_type, total_clusters
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING session_id
	`, af.config.Processing.RunID, af.config.Processing.SessionName,
		af.config.AI.Model, af.config.AI.Endpoint, string(promptTemplate),
		af.config.Processing.AnalysisType, totalClusters).Scan(&sessionID)

	if err != nil {
		return 0, fmt.Errorf("failed to create analysis session: %v", err)
	}

	fmt.Printf("Created AI analysis session %d for run %d (%d clusters)\n",
		sessionID, af.config.Processing.RunID, totalClusters)

	return sessionID, nil
}

// GetClustersForAnalysis retrieves clusters to analyze
func (af *AIFeeder) GetClustersForAnalysis(sessionID int) ([]ClusterData, error) {
	query := `
		SELECT 
			c.id, c.cluster_id, b.batch_number, b.batch_time, c.size, c.quality_score,
			array_agg(t.tweet_text ORDER BY t.tweet_order) as tweets,
			(SELECT tweet_text FROM tweets WHERE cluster_id = c.id AND is_medoid = true LIMIT 1) as medoid_tweet,
			array_agg(DISTINCT bw.word ORDER BY bw.word_order) as busy_words,
			(SELECT frequency_class FROM busy_words WHERE cluster_id = c.id LIMIT 1) as frequency_class
		FROM clusters c
		JOIN batches b ON c.batch_id = b.id
		LEFT JOIN tweets t ON c.id = t.cluster_id
		LEFT JOIN busy_words bw ON c.id = bw.cluster_id
		WHERE b.run_id = $1
		AND c.id >= $2
		GROUP BY c.id, c.cluster_id, b.batch_number, b.batch_time, c.size, c.quality_score
		ORDER BY c.id
	`

	// Add limit if specified
	if af.config.Processing.MaxClusters > 0 {
		query += fmt.Sprintf(" LIMIT %d", af.config.Processing.MaxClusters)
	}

	rows, err := af.db.Query(query, af.config.Processing.RunID, af.config.Processing.StartFromCluster)
	if err != nil {
		return nil, fmt.Errorf("failed to query clusters: %v", err)
	}
	defer rows.Close()

	var clusters []ClusterData
	for rows.Next() {
		var cluster ClusterData
		var tweets, busyWords []string
		var medoidTweet sql.NullString

		err := rows.Scan(
			&cluster.ClusterID, &cluster.BatchNumber, &cluster.BatchTime,
			&cluster.Size, &cluster.QualityScore, &tweets, &medoidTweet, &busyWords, &cluster.FrequencyClass,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan cluster: %v", err)
		}

		cluster.Tweets = tweets
		if medoidTweet.Valid {
			cluster.MedoidTweet = medoidTweet.String
		}
		cluster.BusyWords = busyWords

		clusters = append(clusters, cluster)
	}

	return clusters, nil
}

// SendToAI sends a prompt to the AI service and returns the response
func (af *AIFeeder) SendToAI(prompt string) (*OllamaResponse, error) {
	request := OllamaRequest{
		Model:  af.config.AI.Model,
		Prompt: prompt,
		Stream: false,
	}
	request.Options.Temperature = 0.7
	request.Options.TopP = 0.9

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	// Send request with retries
	var response *OllamaResponse
	for attempt := 0; attempt <= af.config.Processing.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(af.config.Processing.RetryDelay) * time.Second)
			fmt.Printf("Retrying AI request (attempt %d/%d)\n", attempt+1, af.config.Processing.MaxRetries+1)
		}

		resp, err := af.client.Post(af.config.AI.Endpoint, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			if attempt == af.config.Processing.MaxRetries {
				return nil, fmt.Errorf("failed to send request after %d attempts: %v", af.config.Processing.MaxRetries+1, err)
			}
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			if attempt == af.config.Processing.MaxRetries {
				return nil, fmt.Errorf("AI service returned status %d: %s", resp.StatusCode, string(body))
			}
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			if attempt == af.config.Processing.MaxRetries {
				return nil, fmt.Errorf("failed to read response body: %v", err)
			}
			continue
		}

		response = &OllamaResponse{}
		if err := json.Unmarshal(body, response); err != nil {
			if attempt == af.config.Processing.MaxRetries {
				return nil, fmt.Errorf("failed to unmarshal response: %v", err)
			}
			continue
		}

		break // Success
	}

	return response, nil
}

// StoreAnalysisResult stores the AI analysis result in the database
func (af *AIFeeder) StoreAnalysisResult(sessionID int, clusterID int, prompt string, response *OllamaResponse, processingTime time.Duration) error {
	// Convert response to metadata
	responseMetadata := map[string]interface{}{
		"model":                response.Model,
		"total_duration":       response.TotalDuration,
		"load_duration":        response.LoadDuration,
		"prompt_eval_count":    response.PromptEvalCount,
		"prompt_eval_duration": response.PromptEvalDuration,
		"eval_count":           response.EvalCount,
		"eval_duration":        response.EvalDuration,
	}

	responseMetadataJSON, err := json.Marshal(responseMetadata)
	if err != nil {
		return fmt.Errorf("failed to marshal response metadata: %v", err)
	}

	// Insert result
	_, err = af.db.Exec(`
		INSERT INTO ai_analysis_results (
			session_id, cluster_id, prompt_text, response_text, response_metadata, processing_time_ms
		) VALUES ($1, $2, $3, $4, $5, $6)
	`, sessionID, clusterID, prompt, response.Response, responseMetadataJSON, int(processingTime.Milliseconds()))

	if err != nil {
		return fmt.Errorf("failed to insert analysis result: %v", err)
	}

	return nil
}

// UpdateSessionProgress updates the session progress
func (af *AIFeeder) UpdateSessionProgress(sessionID int, processed, failed int) error {
	_, err := af.db.Exec(`
		UPDATE ai_analysis_sessions 
		SET processed_clusters = $2, failed_clusters = $3
		WHERE session_id = $1
	`, sessionID, processed, failed)
	return err
}

// CompleteSession marks the session as completed
func (af *AIFeeder) CompleteSession(sessionID int) error {
	_, err := af.db.Exec(`
		UPDATE ai_analysis_sessions 
		SET status = 'completed', completed_at = NOW()
		WHERE session_id = $1
	`, sessionID)
	return err
}

// ProcessCluster analyzes a single cluster
func (af *AIFeeder) ProcessCluster(sessionID int, cluster ClusterData, promptTemplate *template.Template) error {
	// Generate prompt from template
	var promptBuffer bytes.Buffer
	if err := promptTemplate.Execute(&promptBuffer, cluster); err != nil {
		return fmt.Errorf("failed to execute prompt template: %v", err)
	}
	prompt := promptBuffer.String()

	// Send to AI
	startTime := time.Now()
	response, err := af.SendToAI(prompt)
	if err != nil {
		return fmt.Errorf("failed to get AI response: %v", err)
	}
	processingTime := time.Since(startTime)

	// Store result
	if err := af.StoreAnalysisResult(sessionID, cluster.ClusterID, prompt, response, processingTime); err != nil {
		return fmt.Errorf("failed to store analysis result: %v", err)
	}

	fmt.Printf("Processed cluster %d (batch %d): %d tweets, %d busy words, %dms\n",
		cluster.ClusterID, cluster.BatchNumber, len(cluster.Tweets), len(cluster.BusyWords), int(processingTime.Milliseconds()))

	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Usage: %s <config.yaml>\n", os.Args[0])
		fmt.Printf("Example: %s ../../config/ai_feeder.yaml\n", os.Args[0])
		os.Exit(1)
	}

	configPath := os.Args[1]

	// Create AI feeder
	feeder, err := NewAIFeeder(configPath)
	if err != nil {
		log.Fatalf("Failed to create AI feeder: %v", err)
	}
	defer feeder.Close()

	// Create analysis session
	sessionID, err := feeder.CreateAnalysisSession()
	if err != nil {
		log.Fatalf("Failed to create analysis session: %v", err)
	}

	// Load prompt template
	promptTemplateData, err := os.ReadFile(feeder.config.Processing.PromptTemplate)
	if err != nil {
		log.Fatalf("Failed to read prompt template: %v", err)
	}

	promptTemplate, err := template.New("prompt").Parse(string(promptTemplateData))
	if err != nil {
		log.Fatalf("Failed to parse prompt template: %v", err)
	}

	// Get clusters to analyze
	clusters, err := feeder.GetClustersForAnalysis(sessionID)
	if err != nil {
		log.Fatalf("Failed to get clusters: %v", err)
	}

	fmt.Printf("Starting analysis of %d clusters...\n", len(clusters))

	// Process clusters
	processed := 0
	failed := 0
	for i, cluster := range clusters {
		if err := feeder.ProcessCluster(sessionID, cluster, promptTemplate); err != nil {
			fmt.Printf("Failed to process cluster %d: %v\n", cluster.ClusterID, err)
			failed++
		} else {
			processed++
		}

		// Update progress every 10 clusters
		if (i+1)%10 == 0 {
			feeder.UpdateSessionProgress(sessionID, processed, failed)
			fmt.Printf("Progress: %d/%d processed, %d failed\n", processed, len(clusters), failed)
		}
	}

	// Complete session
	if err := feeder.CompleteSession(sessionID); err != nil {
		log.Printf("Failed to complete session: %v", err)
	}

	fmt.Printf("Analysis completed: %d processed, %d failed\n", processed, failed)
}
