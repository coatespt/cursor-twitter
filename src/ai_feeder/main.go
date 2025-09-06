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
	"path/filepath"
	"strings"
	"sync"
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
		BatchSize           int    `yaml:"batch_size"`             // Clusters to process in parallel
		MaxRetries          int    `yaml:"max_retries"`            // Max retries per request
		RetryDelay          int    `yaml:"retry_delay"`            // Seconds between retries
		PromptTemplate      string `yaml:"prompt_template"`        // Path to prompt template file
		AnalysisType        string `yaml:"analysis_type"`          // Type of analysis to perform
		SessionName         string `yaml:"session_name"`           // Name for this analysis session
		RunID               int    `yaml:"run_id"`                 // Experiment run ID to analyze
		MaxClusters         int    `yaml:"max_clusters"`           // Max clusters to process (0 = all)
		StartFromCluster    int    `yaml:"start_from_cluster"`     // Start from specific cluster ID
		MaxTweetsPerCluster int    `yaml:"max_tweets_per_cluster"` // Max tweets to include in prompt
	} `yaml:"processing"`
	Logging struct {
		LogFile       string `yaml:"log_file"`       // Path to log file
		LogLevel      string `yaml:"log_level"`      // Log level: debug, info, warn, error
		ConsoleOutput bool   `yaml:"console_output"` // Also output to console
	} `yaml:"logging"`
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
	ID             int      `json:"id"` // Surrogate key from clusters table
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
	db           *sql.DB
	config       *AIConfig
	client       *http.Client
	clusterCount int // Track clusters processed for connection reset
	logger       *log.Logger
	logFile      *os.File
}

// setupLogging initializes logging based on configuration
func setupLogging(config *AIConfig) (*log.Logger, *os.File, error) {
	var writers []io.Writer

	// Add console output if enabled
	if config.Logging.ConsoleOutput {
		writers = append(writers, os.Stdout)
	}

	// Add file output if log file is specified
	if config.Logging.LogFile != "" {
		// Create log directory if it doesn't exist
		logDir := filepath.Dir(config.Logging.LogFile)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return nil, nil, fmt.Errorf("failed to create log directory: %v", err)
		}

		// Open log file for writing (append mode)
		logFile, err := os.OpenFile(config.Logging.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to open log file: %v", err)
		}

		writers = append(writers, logFile)

		// Create multi-writer logger
		multiWriter := io.MultiWriter(writers...)
		logger := log.New(multiWriter, "", log.LstdFlags)

		return logger, logFile, nil
	}

	// If no file logging, just use console
	if len(writers) == 0 {
		writers = append(writers, os.Stdout)
	}

	multiWriter := io.MultiWriter(writers...)
	logger := log.New(multiWriter, "", log.LstdFlags)

	return logger, nil, nil
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

	// Setup logging
	logger, logFile, err := setupLogging(&config)
	if err != nil {
		return nil, fmt.Errorf("failed to setup logging: %v", err)
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
		db:      db,
		config:  &config,
		client:  client,
		logger:  logger,
		logFile: logFile,
	}, nil
}

// Close closes the database connection and log file
func (af *AIFeeder) Close() error {
	if af.logFile != nil {
		af.logFile.Close()
	}
	return af.db.Close()
}

// CreateAnalysisSession creates a new AI analysis session
func (af *AIFeeder) CreateAnalysisSession(runName string) (int, int, error) {
	// Look up run_id from run_name
	var runID int
	err := af.db.QueryRow(`
		SELECT run_id FROM experiment_runs WHERE run_name = $1
	`, runName).Scan(&runID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to find experiment run '%s': %v", runName, err)
	}

	// Read prompt template
	promptTemplate, err := os.ReadFile(af.config.Processing.PromptTemplate)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read prompt template: %v", err)
	}

	// Count total clusters for this run
	var totalClusters int
	err = af.db.QueryRow(`
		SELECT COUNT(*) FROM clusters c
		JOIN batches b ON c.batch_id = b.id
		WHERE b.run_id = $1
	`, runID).Scan(&totalClusters)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to count clusters: %v", err)
	}

	// Insert session
	var sessionID int
	err = af.db.QueryRow(`
		INSERT INTO ai_analysis_sessions (
			run_id, session_name, ai_model, ai_endpoint, prompt_template,
			analysis_type, total_clusters
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING session_id
	`, runID, af.config.Processing.SessionName,
		af.config.AI.Model, af.config.AI.Endpoint, string(promptTemplate),
		af.config.Processing.AnalysisType, totalClusters).Scan(&sessionID)

	if err != nil {
		return 0, 0, fmt.Errorf("failed to create analysis session: %v", err)
	}

	af.logger.Printf("Created AI analysis session %d for run '%s' (run_id %d, %d clusters)",
		sessionID, runName, runID, totalClusters)

	return sessionID, runID, nil
}

// GetClustersForAnalysis retrieves clusters to analyze
func (af *AIFeeder) GetClustersForAnalysis(sessionID int, runID int) ([]ClusterData, error) {
	query := `
		SELECT 
			c.id, c.cluster_id, b.batch_number, b.batch_time, c.size, c.quality_score,
			(SELECT array_agg(tweet_text ORDER BY tweet_order) FROM tweets WHERE cluster_id = c.id) as tweets,
			(SELECT tweet_text FROM tweets WHERE cluster_id = c.id AND is_medoid = true LIMIT 1) as medoid_tweet,
			(SELECT array_agg(word ORDER BY word_order) FROM busy_words WHERE cluster_id = c.id) as busy_words,
			(SELECT frequency_class FROM busy_words WHERE cluster_id = c.id LIMIT 1) as frequency_class
		FROM clusters c
		JOIN batches b ON c.batch_id = b.id
		WHERE b.run_id = $1
		AND c.id >= $2
		AND c.id NOT IN (
			SELECT DISTINCT cluster_id 
			FROM ai_analysis_results 
		)
		ORDER BY c.id
	`

	// Add limit if specified
	if af.config.Processing.MaxClusters > 0 {
		query += fmt.Sprintf(" LIMIT %d", af.config.Processing.MaxClusters)
	}

	rows, err := af.db.Query(query, runID, af.config.Processing.StartFromCluster)
	if err != nil {
		return nil, fmt.Errorf("failed to query clusters: %v", err)
	}
	defer rows.Close()

	var clusters []ClusterData
	for rows.Next() {
		var cluster ClusterData
		var tweetsStr, busyWordsStr sql.NullString
		var medoidTweet sql.NullString

		var frequencyClass sql.NullInt32
		err := rows.Scan(
			&cluster.ID, &cluster.ClusterID, &cluster.BatchNumber, &cluster.BatchTime,
			&cluster.Size, &cluster.QualityScore, &tweetsStr, &medoidTweet, &busyWordsStr, &frequencyClass,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan cluster: %v", err)
		}

		if frequencyClass.Valid {
			cluster.FrequencyClass = int(frequencyClass.Int32)
		} else {
			cluster.FrequencyClass = 0 // Default value for NULL
		}
		if err != nil {
			return nil, fmt.Errorf("failed to scan cluster: %v", err)
		}

		// Parse arrays from strings
		if tweetsStr.Valid {
			// Remove {} and split by comma
			tweetsStr := strings.Trim(tweetsStr.String, "{}")
			if tweetsStr != "" {
				cluster.Tweets = strings.Split(tweetsStr, ",")
			}
		}
		if busyWordsStr.Valid {
			// Remove {} and split by comma
			busyWordsStr := strings.Trim(busyWordsStr.String, "{}")
			if busyWordsStr != "" {
				cluster.BusyWords = strings.Split(busyWordsStr, ",")
			}
		}
		if medoidTweet.Valid {
			cluster.MedoidTweet = medoidTweet.String
		}

		clusters = append(clusters, cluster)
	}

	return clusters, nil
}

// SendToAI sends a prompt to the AI service and returns the response
func (af *AIFeeder) SendToAI(prompt string) (*OllamaResponse, error) {
	// Reset connection every 10 clusters to clear context
	af.clusterCount++
	if af.clusterCount%10 == 0 {
		timeout := time.Duration(af.config.AI.Timeout) * time.Second
		af.client = &http.Client{
			Timeout: timeout,
		}
		af.logger.Printf("Reset HTTP connection after %d clusters", af.clusterCount)
	}

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
			af.logger.Printf("Retrying AI request (attempt %d/%d)", attempt+1, af.config.Processing.MaxRetries+1)
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

// cleanAIResponse removes verbose preamble and fluff from AI responses
func cleanAIResponse(response string) string {
	// Remove common verbose introductions
	patterns := []string{
		"I'd be happy to analyze",
		"I'll analyze this Twitter cluster",
		"Let me analyze this cluster",
		"Here's my analysis",
		"Based on the provided data",
		"Looking at this Twitter cluster",
		"After examining the tweets",
	}

	cleaned := response
	for _, pattern := range patterns {
		if idx := strings.Index(strings.ToLower(cleaned), strings.ToLower(pattern)); idx != -1 {
			// Find the next line break or end of sentence
			if newlineIdx := strings.Index(cleaned[idx:], "\n"); newlineIdx != -1 {
				cleaned = cleaned[:idx] + cleaned[idx+newlineIdx+1:]
			} else if periodIdx := strings.Index(cleaned[idx:], "."); periodIdx != -1 {
				cleaned = cleaned[:idx] + cleaned[idx+periodIdx+1:]
			}
		}
	}

	// Clean up whitespace more aggressively
	// Split into lines, trim each line, and rejoin
	lines := strings.Split(cleaned, "\n")
	var cleanedLines []string
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine != "" {
			cleanedLines = append(cleanedLines, trimmedLine)
		}
	}
	cleaned = strings.Join(cleanedLines, "\n")

	// Ensure it starts with the structured format
	if !strings.HasPrefix(cleaned, "**Main Topic:**") {
		// Try to find where the structured format starts
		if idx := strings.Index(cleaned, "**Main Topic:**"); idx != -1 {
			cleaned = cleaned[idx:]
		}
	}

	// Final trim to remove any remaining leading/trailing whitespace
	cleaned = strings.TrimSpace(cleaned)

	return cleaned
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

	// Clean the response to remove verbose preamble
	cleanedResponse := cleanAIResponse(response.Response)

	// Insert result
	_, err = af.db.Exec(`
		INSERT INTO ai_analysis_results (
			session_id, cluster_id, prompt_text, response_text, response_metadata, processing_time_ms
		) VALUES ($1, $2, $3, $4, $5, $6)
	`, sessionID, clusterID, prompt, cleanedResponse, responseMetadataJSON, int(processingTime.Milliseconds()))

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
	// Limit tweets to max_tweets_per_cluster
	maxTweets := af.config.Processing.MaxTweetsPerCluster
	if maxTweets > 0 && len(cluster.Tweets) > maxTweets {
		// Keep medoid tweet and sample from others
		limitedTweets := make([]string, 0, maxTweets)
		limitedTweets = append(limitedTweets, cluster.Tweets[0]) // Keep first tweet (likely medoid)

		// Add sample tweets from the rest
		step := len(cluster.Tweets) / (maxTweets - 1)
		if step < 1 {
			step = 1
		}
		for i := 1; i < len(cluster.Tweets) && len(limitedTweets) < maxTweets; i += step {
			limitedTweets = append(limitedTweets, cluster.Tweets[i])
		}
		cluster.Tweets = limitedTweets
	}

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
	if err := af.StoreAnalysisResult(sessionID, cluster.ID, prompt, response, processingTime); err != nil {
		return fmt.Errorf("failed to store analysis result: %v", err)
	}

	af.logger.Printf("Processed cluster %d (batch %d): %d tweets, %d busy words, %dms",
		cluster.ClusterID, cluster.BatchNumber, len(cluster.Tweets), len(cluster.BusyWords), int(processingTime.Milliseconds()))

	return nil
}

func main() {
	if len(os.Args) < 3 {
		fmt.Printf("Usage: %s <run_name> <config.yaml>\n", os.Args[0])
		fmt.Printf("  run_name: Name of the experiment run to analyze (e.g., \"sept_4_ptc\")\n")
		fmt.Printf("  config.yaml: AI feeder configuration file\n")
		fmt.Printf("Example: %s \"sept_4_ptc\" ../../config/ai_feeder.yaml\n", os.Args[0])
		os.Exit(1)
	}

	runName := os.Args[1]
	configPath := os.Args[2]

	// Create AI feeder
	feeder, err := NewAIFeeder(configPath)
	if err != nil {
		log.Fatalf("Failed to create AI feeder: %v", err)
	}
	defer feeder.Close()

	// Create analysis session
	sessionID, runID, err := feeder.CreateAnalysisSession(runName)
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
	clusters, err := feeder.GetClustersForAnalysis(sessionID, runID)
	if err != nil {
		log.Fatalf("Failed to get clusters: %v", err)
	}

	fmt.Printf("Starting analysis of %d clusters...\n", len(clusters))

	// Process clusters with concurrent workers
	processed := 0
	failed := 0
	startTime := time.Now()

	// Use mutex for thread-safe counters
	var mu sync.Mutex
	updateCounters := func(success bool) {
		mu.Lock()
		defer mu.Unlock()
		if success {
			processed++
		} else {
			failed++
		}
	}

	// Worker function
	worker := func(clusterChan <-chan ClusterData, wg *sync.WaitGroup) {
		defer wg.Done()
		for cluster := range clusterChan {
			clusterStartTime := time.Now()

			if err := feeder.ProcessCluster(sessionID, cluster, promptTemplate); err != nil {
				fmt.Printf("Failed to process cluster %d: %v\n", cluster.ClusterID, err)
				updateCounters(false)
			} else {
				clusterTime := time.Since(clusterStartTime)
				updateCounters(true)

				// Show individual cluster timing
				fmt.Printf("Processed cluster %d (batch %d): %d tweets, %d busy words, %v\n",
					cluster.ClusterID, cluster.BatchNumber, len(cluster.Tweets), len(cluster.BusyWords), clusterTime)
			}
		}
	}

	// Create worker pool
	numWorkers := 1 // Reduced to 1 worker to avoid overwhelming Ollama
	clusterChan := make(chan ClusterData, numWorkers)
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go worker(clusterChan, &wg)
	}

	// Send clusters to workers
	go func() {
		for _, cluster := range clusters {
			clusterChan <- cluster
		}
		close(clusterChan)
	}()

	// Progress monitoring goroutine
	go func() {
		ticker := time.NewTicker(30 * time.Second) // Update every 30 seconds
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				mu.Lock()
				currentProcessed := processed
				currentFailed := failed
				mu.Unlock()

				if currentProcessed > 0 {
					elapsed := time.Since(startTime)
					avgTime := elapsed / time.Duration(currentProcessed)
					estimatedTotal := avgTime * time.Duration(len(clusters))
					estimatedRemaining := estimatedTotal - elapsed

					feeder.UpdateSessionProgress(sessionID, currentProcessed, currentFailed)
					fmt.Printf("=== PERFORMANCE TIMING STATS ===\n")
					fmt.Printf("Progress: %d/%d processed, %d failed (%.1f%%)\n",
						currentProcessed, len(clusters), currentFailed, float64(currentProcessed)/float64(len(clusters))*100)
					fmt.Printf("Total elapsed time: %v\n", elapsed)
					fmt.Printf("Average time per cluster: %v\n", avgTime)
					fmt.Printf("Estimated total time: %v\n", estimatedTotal)
					fmt.Printf("Estimated remaining: %v\n", estimatedRemaining)
					fmt.Printf("Processing rate: %.2f clusters/hour\n",
						float64(currentProcessed)/elapsed.Hours())
					fmt.Printf("Concurrent workers: %d\n", numWorkers)
					fmt.Printf("Current throughput: %.2f clusters/minute\n",
						float64(currentProcessed)/elapsed.Minutes())
					fmt.Printf("================================\n")
				}
			}
		}
	}()

	// Wait for all workers to complete
	wg.Wait()

	// Complete session
	if err := feeder.CompleteSession(sessionID); err != nil {
		log.Printf("Failed to complete session: %v", err)
	}

	fmt.Printf("Analysis completed: %d processed, %d failed\n", processed, failed)
}
