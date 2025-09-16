package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"gopkg.in/yaml.v3"

	"cursor-twitter/json_parser"
)

// PipelineConfig represents the pipeline configuration structure
type PipelineConfig struct {
	WindowSize        int       `yaml:"window_size"`
	TokenPersistFiles int       `yaml:"token_persist_files"`
	RebuildEveryFiles int       `yaml:"rebuild_every_files"`
	Batch             int       `yaml:"batch"`
	ZScores           []float64 `yaml:"z_scores"`
	FreqClasses       int       `yaml:"freq_classes"`
	BWArrayLen        int       `yaml:"bw_array_len"`
	BusyWordClasses   []int     `yaml:"busyword_classes"`
	LanguageFilter    string    `yaml:"language_filter"`
	Analysis          struct {
		MinBusyWordsPerTweet         int     `yaml:"min_busy_words_per_tweet"`
		MinJaccardSimilarity         float64 `yaml:"min_jaccard_similarity"`
		DuplicateSimilarityThreshold float64 `yaml:"duplicate_similarity_threshold"`
		UseMedoidSimilarity          bool    `yaml:"use_medoid_similarity"`
		UseBusyWordSimilarity        bool    `yaml:"use_busy_word_similarity"`
		MedoidSimilarityThreshold    float64 `yaml:"medoid_similarity_threshold"`
		BusyWordSimilarityThreshold  float64 `yaml:"busy_word_similarity_threshold"`
		MinTokenLen                  int     `yaml:"min_token_len"`
	} `yaml:"analysis"`
}

// DatabaseConfig holds the database connection settings
type DatabaseConfig struct {
	Database struct {
		Host     string `yaml:"host"`
		Port     int    `yaml:"port"`
		Name     string `yaml:"name"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
		SSLMode  string `yaml:"ssl_mode"`
	} `yaml:"database"`

	// Processing options
	MaxTweetsPerCluster int  `yaml:"max_tweets_per_cluster,omitempty"`
	ValidateData        bool `yaml:"validate_data,omitempty"`
}

// SQLLoader handles the database operations
type SQLLoader struct {
	db     *sql.DB
	config *DatabaseConfig
	runID  int // Current experiment run ID
}

// ensureExperimentRunsTable creates the experiment_runs table if it doesn't exist
func ensureExperimentRunsTable(db *sql.DB) error {
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS experiment_runs (
			run_id SERIAL PRIMARY KEY,
			run_date_time TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			run_name TEXT NOT NULL,
			window_size INTEGER,
			token_persist_files INTEGER,
			rebuild_every_files INTEGER,
			batch_size INTEGER,
			z_scores TEXT, -- Array stored as string
			freq_classes INTEGER,
			bw_array_len INTEGER,
			busyword_classes TEXT, -- Array stored as string
			min_busy_words_per_tweet INTEGER,
			min_jaccard_similarity DECIMAL(3,2),
			duplicate_similarity_threshold DECIMAL(3,2),
			language_filter VARCHAR(10),
			use_medoid_similarity BOOLEAN,
			use_busy_word_similarity BOOLEAN,
			medoid_similarity_threshold DECIMAL(3,2),
			busy_word_similarity_threshold DECIMAL(3,2),
			min_token_len INTEGER,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		);
	`

	_, err := db.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create experiment_runs table: %v", err)
	}

	fmt.Println("✅ experiment_runs table ensured")
	return nil
}

// NewSQLLoader creates a new SQL loader with database connection
func NewSQLLoader(configPath string) (*SQLLoader, error) {
	// Load database config
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %v", configPath, err)
	}

	var config DatabaseConfig
	if err := yaml.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %v", err)
	}

	// Build connection string
	connStr := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		config.Database.Host,
		config.Database.Port,
		config.Database.Name,
		config.Database.User,
		config.Database.Password,
		config.Database.SSLMode,
	)

	// Connect to database
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %v", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %v", err)
	}

	fmt.Printf("Connected to PostgreSQL database: %s@%s:%d/%s\n",
		config.Database.User,
		config.Database.Host,
		config.Database.Port,
		config.Database.Name,
	)

	// Verify which database we're actually connected to
	var currentDB string
	err = db.QueryRow("SELECT current_database()").Scan(&currentDB)
	if err != nil {
		log.Printf("Warning: Could not get current database name: %v", err)
	} else {
		fmt.Printf("Current database: %s\n", currentDB)
	}

	// Ensure experiment_runs table exists
	if err := ensureExperimentRunsTable(db); err != nil {
		return nil, fmt.Errorf("failed to ensure experiment_runs table: %v", err)
	}

	return &SQLLoader{db: db, config: &config}, nil
}

// loadPipelineConfig loads a pipeline configuration file
func loadPipelineConfig(configPath string) (*PipelineConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	var config PipelineConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %v", err)
	}

	return &config, nil
}

// loadPipelineConfigWithOverride loads a base config and merges it with an override
func loadPipelineConfigWithOverride(baseConfigPath, overrideConfigPath string) (*PipelineConfig, error) {
	// Load base config
	baseConfig, err := loadPipelineConfig(baseConfigPath)
	if err != nil {
		return nil, err
	}

	// Load override config
	overrideConfig, err := loadPipelineConfig(overrideConfigPath)
	if err != nil {
		return nil, err
	}

	// Merge override values into base config
	// This is a simple merge - override values replace base values
	if overrideConfig.WindowSize != 0 {
		baseConfig.WindowSize = overrideConfig.WindowSize
	}
	if overrideConfig.TokenPersistFiles != 0 {
		baseConfig.TokenPersistFiles = overrideConfig.TokenPersistFiles
	}
	if overrideConfig.RebuildEveryFiles != 0 {
		baseConfig.RebuildEveryFiles = overrideConfig.RebuildEveryFiles
	}
	if overrideConfig.Batch != 0 {
		baseConfig.Batch = overrideConfig.Batch
	}
	if len(overrideConfig.ZScores) > 0 {
		baseConfig.ZScores = overrideConfig.ZScores
	}
	if overrideConfig.FreqClasses != 0 {
		baseConfig.FreqClasses = overrideConfig.FreqClasses
	}
	if overrideConfig.BWArrayLen != 0 {
		baseConfig.BWArrayLen = overrideConfig.BWArrayLen
	}
	if len(overrideConfig.BusyWordClasses) > 0 {
		baseConfig.BusyWordClasses = overrideConfig.BusyWordClasses
	}
	if overrideConfig.LanguageFilter != "" {
		baseConfig.LanguageFilter = overrideConfig.LanguageFilter
	}
	if overrideConfig.Analysis.MinBusyWordsPerTweet != 0 {
		baseConfig.Analysis.MinBusyWordsPerTweet = overrideConfig.Analysis.MinBusyWordsPerTweet
	}
	if overrideConfig.Analysis.MinJaccardSimilarity != 0 {
		baseConfig.Analysis.MinJaccardSimilarity = overrideConfig.Analysis.MinJaccardSimilarity
	}
	if overrideConfig.Analysis.DuplicateSimilarityThreshold != 0 {
		baseConfig.Analysis.DuplicateSimilarityThreshold = overrideConfig.Analysis.DuplicateSimilarityThreshold
	}
	if overrideConfig.Analysis.MedoidSimilarityThreshold != 0 {
		baseConfig.Analysis.MedoidSimilarityThreshold = overrideConfig.Analysis.MedoidSimilarityThreshold
	}
	if overrideConfig.Analysis.BusyWordSimilarityThreshold != 0 {
		baseConfig.Analysis.BusyWordSimilarityThreshold = overrideConfig.Analysis.BusyWordSimilarityThreshold
	}
	if overrideConfig.Analysis.MinTokenLen != 0 {
		baseConfig.Analysis.MinTokenLen = overrideConfig.Analysis.MinTokenLen
	}
	// Boolean fields
	baseConfig.Analysis.UseMedoidSimilarity = overrideConfig.Analysis.UseMedoidSimilarity
	baseConfig.Analysis.UseBusyWordSimilarity = overrideConfig.Analysis.UseBusyWordSimilarity

	return baseConfig, nil
}

// generateUniqueRunName generates a unique run name, handling duplicates
// For resuming existing runs, it returns the original name
// For new runs, it appends a suffix if the name already exists
func (sl *SQLLoader) generateUniqueRunName(baseName string) (string, error) {
	// First, check if the exact name exists
	var count int
	err := sl.db.QueryRow(`
		SELECT COUNT(*) FROM new_experiment_runs WHERE run_name = $1
	`, baseName).Scan(&count)

	if err != nil {
		return "", fmt.Errorf("failed to check run name: %v", err)
	}

	if count == 0 {
		return baseName, nil // Name is unique, use it
	}

	// Name exists - for resuming, we want to reuse the same name
	// Only append suffix if we're explicitly creating a new run
	// For now, always reuse the existing name to enable resuming
	return baseName, nil
}

// CreateExperimentRun creates a new experiment run record and returns the run_id
// If the run already exists, it returns the existing run_id for resuming
func (sl *SQLLoader) CreateExperimentRun(runName, pipelineConfigPath, overrideConfigPath string) (int, error) {
	// Generate unique run name
	uniqueRunName, err := sl.generateUniqueRunName(runName)
	if err != nil {
		return 0, fmt.Errorf("failed to generate unique run name: %v", err)
	}

	fmt.Printf("Using run name: %s\n", uniqueRunName)

	// Check if run already exists
	var existingRunID int
	err = sl.db.QueryRow(`
		SELECT run_id FROM new_experiment_runs WHERE run_name = $1
	`, uniqueRunName).Scan(&existingRunID)

	if err == nil {
		// Run exists, return existing run_id for resuming
		fmt.Printf("Resuming existing run with ID: %d\n", existingRunID)
		return existingRunID, nil
	} else if err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed to check if run exists: %v", err)
	}

	// Run doesn't exist, create new one
	fmt.Printf("Creating new run...\n")

	// Load pipeline configuration
	pipelineConfig, err := loadPipelineConfig(pipelineConfigPath)
	if err != nil {
		return 0, fmt.Errorf("failed to load pipeline config: %v", err)
	}

	// Apply override config if provided
	if overrideConfigPath != "" {
		overrideConfig, err := loadPipelineConfig(overrideConfigPath)
		if err != nil {
			return 0, fmt.Errorf("failed to load override config: %v", err)
		}
		// Apply overrides (this is a simple merge - in practice you might want more sophisticated merging)
		pipelineConfig = overrideConfig
	}

	// Convert arrays to strings
	zScoresStr := fmt.Sprintf("%v", pipelineConfig.ZScores)

	// Insert experiment run with all configuration details
	var runID int
	err = sl.db.QueryRow(`
		INSERT INTO new_experiment_runs (
			run_name, window_size, batch_size, freq_classes, min_jaccard_similarity,
			bw_array_len, z_scores, min_busy_words_per_tweet, duplicate_similarity_threshold,
			language_filter, use_medoid_similarity, use_busy_word_similarity,
			medoid_similarity_threshold, busy_word_similarity_threshold, min_token_len
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING run_id
	`, uniqueRunName, pipelineConfig.WindowSize, pipelineConfig.Batch, pipelineConfig.FreqClasses,
		pipelineConfig.Analysis.MinJaccardSimilarity, pipelineConfig.BWArrayLen, zScoresStr,
		pipelineConfig.Analysis.MinBusyWordsPerTweet, pipelineConfig.Analysis.DuplicateSimilarityThreshold,
		pipelineConfig.LanguageFilter, pipelineConfig.Analysis.UseMedoidSimilarity,
		pipelineConfig.Analysis.UseBusyWordSimilarity, pipelineConfig.Analysis.MedoidSimilarityThreshold,
		pipelineConfig.Analysis.BusyWordSimilarityThreshold, pipelineConfig.Analysis.MinTokenLen).Scan(&runID)

	if err != nil {
		return 0, fmt.Errorf("failed to insert experiment run: %v", err)
	}

	fmt.Printf("Created new run with ID: %d\n", runID)
	return runID, nil
}

// Close closes the database connection
func (sl *SQLLoader) Close() error {
	if sl.db != nil {
		return sl.db.Close()
	}
	return nil
}

// InsertBatch inserts a batch and returns its database ID
func (sl *SQLLoader) InsertBatch(batch json_parser.Batch) (int, error) {
	// Parse batch time - handle both with and without UTC suffix
	var batchTime time.Time
	var err error
	if strings.HasSuffix(batch.Data.BatchTime, " UTC") {
		batchTime, err = time.Parse("2006-01-02 15:04:05 MST", batch.Data.BatchTime)
	} else {
		batchTime, err = time.Parse("2006-01-02 15:04:05", batch.Data.BatchTime)
	}
	if err != nil {
		return 0, fmt.Errorf("failed to parse batch time %s: %v", batch.Data.BatchTime, err)
	}

	// Check if batch already exists
	var batchID int
	err = sl.db.QueryRow(`
		SELECT id FROM new_batches WHERE run_id = $1 AND batch_number = $2
	`, sl.runID, batch.Data.BatchNumber).Scan(&batchID)

	if err == sql.ErrNoRows {
		// Batch doesn't exist, insert it
		err = sl.db.QueryRow(`
			INSERT INTO new_batches (run_id, batch_number, batch_time, method, total_tweets, total_clusters, clusters_above_min_size)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id
		`, sl.runID, batch.Data.BatchNumber, batchTime, batch.Data.Method, batch.Data.TotalTweets,
			batch.Data.TotalClusters, batch.Data.ClustersAboveMinSize).Scan(&batchID)

		if err != nil {
			return 0, fmt.Errorf("failed to insert batch %d: %v", batch.Data.BatchNumber, err)
		}
	} else if err != nil {
		return 0, fmt.Errorf("failed to check batch %d: %v", batch.Data.BatchNumber, err)
	} else {
		// Batch already exists, skip it
		fmt.Printf("Batch %d already exists, skipping\n", batch.Data.BatchNumber)
		return batchID, nil
	}

	return batchID, nil
}

// InsertCluster inserts a cluster and returns its database ID
func (sl *SQLLoader) InsertCluster(batchID int, cluster json_parser.Cluster) (int, error) {
	var clusterID int
	err := sl.db.QueryRow(`
		INSERT INTO new_clusters (batch_id, size, quality_score)
		VALUES ($1, $2, $3)
		RETURNING cluster_id
	`, batchID, cluster.Size, cluster.QualityScore).Scan(&clusterID)

	if err != nil {
		return 0, fmt.Errorf("failed to insert cluster %d: %v", cluster.ClusterID, err)
	}

	return clusterID, nil
}

// InsertTweets inserts all tweets for a cluster using the new schema
func (sl *SQLLoader) InsertTweets(clusterID int, cluster json_parser.Cluster) error {
	medoidText := cluster.GetMedoidText()

	// Limit number of tweets if configured
	tweetCount := len(cluster.TweetTexts)
	if sl.config.MaxTweetsPerCluster > 0 && tweetCount > sl.config.MaxTweetsPerCluster {
		fmt.Printf("    Limiting cluster %d from %d to %d tweets\n", cluster.ClusterID, tweetCount, sl.config.MaxTweetsPerCluster)
		tweetCount = sl.config.MaxTweetsPerCluster
	}

	for i := 0; i < tweetCount; i++ {
		tweetText := cluster.TweetTexts[i]
		isMedoid := (tweetText == medoidText)

		// First, insert the tweet into the tweets table
		var tweetID int
		err := sl.db.QueryRow(`
			INSERT INTO new_tweets (tweet_text)
			VALUES ($1)
			RETURNING tweet_id
		`, tweetText).Scan(&tweetID)

		if err != nil {
			return fmt.Errorf("failed to insert tweet %d: %v", i+1, err)
		}

		// Then, link the tweet to the cluster
		_, err = sl.db.Exec(`
			INSERT INTO new_tweet_clusters (tweet_id, cluster_id, tweet_order, is_medoid)
			VALUES ($1, $2, $3, $4)
		`, tweetID, clusterID, i+1, isMedoid)

		if err != nil {
			return fmt.Errorf("failed to link tweet %d to cluster %d: %v", tweetID, cluster.ClusterID, err)
		}
	}

	return nil
}

// InsertBusyWords inserts all busy words for a cluster
func (sl *SQLLoader) InsertBusyWords(clusterID int, cluster json_parser.Cluster) error {
	for i, word := range cluster.BusyWords {
		// Get frequency class from busy_word_classes map, default to 12 if not found
		frequencyClass := 12
		if cluster.BusyWordClasses != nil {
			if freqClass, exists := cluster.BusyWordClasses[word]; exists {
				if freqFloat, ok := freqClass.(float64); ok {
					frequencyClass = int(freqFloat)
				}
			}
		}

		_, err := sl.db.Exec(`
			INSERT INTO new_busy_words (cluster_id, word, word_order, frequency_class)
			VALUES ($1, $2, $3, $4)
		`, clusterID, word, i+1, frequencyClass)

		if err != nil {
			return fmt.Errorf("failed to insert busy word %s in cluster %d: %v", word, cluster.ClusterID, err)
		}
	}

	return nil
}

// ProcessBatch processes a single batch and inserts all its data
func (sl *SQLLoader) ProcessBatch(batch json_parser.Batch) error {
	// Insert batch (InsertBatch handles duplicate checking)
	batchID, err := sl.InsertBatch(batch)
	if err != nil {
		return err
	}

	// Parse clusters from batch
	clusters, err := json_parser.ParseClusters(batch)
	if err != nil {
		return fmt.Errorf("failed to parse clusters for batch %d: %v", batch.Data.BatchNumber, err)
	}

	// Log batches with no clusters (expected when fallback clusters are disabled)
	if len(clusters) == 0 && batch.Data.TotalTweets > 0 {
		fmt.Printf("  INFO: Batch %d has 0 clusters but %d total tweets (fallback clusters disabled)\n",
			batch.Data.BatchNumber, batch.Data.TotalTweets)
	}

	// Process each cluster
	for _, cluster := range clusters {
		// Validate cluster data if enabled
		if sl.config.ValidateData {
			if len(cluster.BusyWords) == 0 {
				fmt.Printf("  WARNING: Cluster %d in batch %d has no busy words!\n", cluster.ClusterID, batch.Data.BatchNumber)
			}
			if len(cluster.TweetTexts) == 0 {
				fmt.Printf("  WARNING: Cluster %d in batch %d has no tweets!\n", cluster.ClusterID, batch.Data.BatchNumber)
			}
		}

		// In the new schema, we always insert new clusters since each gets a unique auto-incrementing cluster_id
		// We don't need to check for existing clusters by the old cluster_id from JSON
		clusterID, err := sl.InsertCluster(batchID, cluster)
		if err != nil {
			return fmt.Errorf("failed to insert cluster %d: %v", cluster.ClusterID, err)
		}

		// Insert tweets
		if err := sl.InsertTweets(clusterID, cluster); err != nil {
			return err
		}

		// Insert busy words
		if err := sl.InsertBusyWords(clusterID, cluster); err != nil {
			return err
		}
	}

	// Calculate total busy words more safely
	totalBusyWords := 0
	for _, cluster := range clusters {
		totalBusyWords += len(cluster.BusyWords)
	}

	fmt.Printf("Processed batch %d: %d clusters, %d tweets, %d busy words\n",
		batch.Data.BatchNumber,
		len(clusters),
		batch.Data.TotalTweets,
		totalBusyWords,
	)

	return nil
}

// LoadJSONFile loads and processes an entire JSON file
func (sl *SQLLoader) LoadJSONFile(jsonFilePath string) error {
	// Create parser
	parser, err := json_parser.NewParser(jsonFilePath)
	if err != nil {
		return fmt.Errorf("failed to create parser: %v", err)
	}
	defer parser.Close()

	// Get file size for progress reporting
	fileInfo, err := os.Stat(jsonFilePath)
	if err != nil {
		return fmt.Errorf("failed to get file info: %v", err)
	}
	totalSize := fileInfo.Size()

	fmt.Printf("Loading JSON file: %s (%.2f MB)\n", jsonFilePath, float64(totalSize)/(1024*1024))

	// Check what the last processed batch was for this run
	var lastProcessedBatch int
	err = sl.db.QueryRow(`
		SELECT COALESCE(MAX(batch_number), -1) 
		FROM new_batches 
		WHERE run_id = $1
	`, sl.runID).Scan(&lastProcessedBatch)

	if err != nil {
		return fmt.Errorf("failed to check last processed batch: %v", err)
	}

	if lastProcessedBatch >= 0 {
		fmt.Printf("Resuming from batch %d (last processed batch for run_id %d)\n", lastProcessedBatch+1, sl.runID)
	} else {
		fmt.Printf("Starting from beginning (no batches found for run_id %d)\n", sl.runID)
	}

	// Process batches in chunks
	totalBatches := 0
	skippedBatches := 0
	startTime := time.Now()

	for {
		batches, err := parser.LoadNextChunkContinuous()
		if err != nil {
			return fmt.Errorf("failed to load chunk: %v", err)
		}

		// If no batches returned, we've reached the end of the file
		if len(batches) == 0 {
			fmt.Printf("Reached end of file, processing complete.\n")
			break
		}

		// Process each batch in this chunk
		for _, batch := range batches {
			// Skip batches that have already been processed
			if batch.Data.BatchNumber <= lastProcessedBatch {
				skippedBatches++
				if skippedBatches%100 == 0 {
					fmt.Printf("Skipped %d already processed batches\n", skippedBatches)
				}
				continue
			}

			if err := sl.ProcessBatch(batch); err != nil {
				return fmt.Errorf("failed to process batch %d: %v", batch.Data.BatchNumber, err)
			}
			totalBatches++
		}

		// Progress update every 100 batches
		if totalBatches%100 == 0 {
			elapsed := time.Since(startTime)
			fmt.Printf("Processed %d new batches in %v (skipped %d already processed)\n", totalBatches, elapsed, skippedBatches)
		}
	}

	// Note: This function now runs continuously and never exits
	// It will keep waiting for new data from the main pipeline
	return nil
}

func main() {
	if len(os.Args) < 4 {
		fmt.Printf("Usage: %s <run_name> <database-config.yaml> <pipeline-config.yaml> [override-config.yaml] [json-file]\n", os.Args[0])
		fmt.Printf("  run_name: Name for this experiment run (e.g., \"sept_4_ptc\", \"Test Run\")\n")
		fmt.Printf("  database-config.yaml: Database connection settings\n")
		fmt.Printf("  pipeline-config.yaml: Pipeline processing configuration\n")
		fmt.Printf("  override-config.yaml: Optional config overrides\n")
		fmt.Printf("  json-file: Optional specific JSON file to load (defaults to pipeline output)\n")
		fmt.Printf("\nExamples:\n")
		fmt.Printf("  %s \"sept_4_ptc\" ../../config/database.yaml ../../config/config.yaml\n", os.Args[0])
		fmt.Printf("  %s \"High Freq Test\" ../../config/database.yaml ../../config/config.yaml ../../config/experiments/high_freq.yaml ../../august_12_clusters.json\n", os.Args[0])
		os.Exit(1)
	}

	runName := os.Args[1]
	dbConfigPath := os.Args[2]
	pipelineConfigPath := os.Args[3]

	// If no run name provided, generate a default one
	if runName == "" || runName == "default" {
		runName = "Run"
	}

	var overrideConfigPath, jsonFilePath string
	if len(os.Args) > 4 {
		if strings.HasSuffix(os.Args[4], ".json") {
			jsonFilePath = os.Args[4]
		} else {
			overrideConfigPath = os.Args[4]
			if len(os.Args) > 5 {
				jsonFilePath = os.Args[5]
			}
		}
	}

	// Validate file paths
	if _, err := os.Stat(dbConfigPath); os.IsNotExist(err) {
		log.Fatalf("Database config file not found: %s", dbConfigPath)
	}
	if _, err := os.Stat(pipelineConfigPath); os.IsNotExist(err) {
		log.Fatalf("Pipeline config file not found: %s", pipelineConfigPath)
	}
	if overrideConfigPath != "" {
		if _, err := os.Stat(overrideConfigPath); os.IsNotExist(err) {
			log.Fatalf("Override config file not found: %s", overrideConfigPath)
		}
	}
	if jsonFilePath != "" {
		if _, err := os.Stat(jsonFilePath); os.IsNotExist(err) {
			log.Fatalf("JSON file not found: %s", jsonFilePath)
		}
	}

	// Create SQL loader
	loader, err := NewSQLLoader(dbConfigPath)
	if err != nil {
		log.Fatalf("Failed to create SQL loader: %v", err)
	}
	defer loader.Close()

	// Create experiment run record
	fmt.Printf("Creating experiment run: %s\n", runName)
	runID, err := loader.CreateExperimentRun(runName, pipelineConfigPath, overrideConfigPath)
	if err != nil {
		log.Fatalf("Failed to create experiment run: %v", err)
	}
	fmt.Printf("Created experiment run with ID: %d\n", runID)

	// Store run_id in loader for use in batch insertions
	loader.runID = runID

	// Test database connection with test table
	fmt.Println("Testing database connection...")
	var count int
	err = loader.db.QueryRow("SELECT COUNT(*) FROM new_experiment_runs").Scan(&count)
	if err != nil {
		log.Fatalf("Failed to query new_experiment_runs: %v", err)
	}
	fmt.Printf("Test table has %d rows\n", count)

	// Check if any pipeline data exists
	var batchCount int
	err = loader.db.QueryRow("SELECT COUNT(*) FROM new_batches").Scan(&batchCount)
	if err != nil {
		fmt.Printf("Error checking batches: %v\n", err)
	} else {
		fmt.Printf("Pipeline data: %d batches\n", batchCount)
	}

	// Test creating a table from Go
	fmt.Println("\nTesting table creation from Go:")
	_, err = loader.db.Exec(`
		CREATE TABLE IF NOT EXISTS public.go_test_table (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL
		)
	`)
	if err != nil {
		fmt.Printf("Error creating table: %v\n", err)
	} else {
		fmt.Println("Successfully created go_test_table")

		// Insert a test row
		_, err = loader.db.Exec("INSERT INTO public.go_test_table (name) VALUES ($1)", "test from go")
		if err != nil {
			fmt.Printf("Error inserting row: %v\n", err)
		} else {
			fmt.Println("Successfully inserted test row")
		}
	}

	// Check for expected tables using pg_tables (more direct)
	fmt.Println("\nChecking for expected tables (using pg_tables):")
	expectedTables := []string{"batches", "clusters", "tweets", "busy_words"}
	for _, tableName := range expectedTables {
		var tableExists bool
		err = loader.db.QueryRow(`
			SELECT EXISTS (
				SELECT 1 FROM pg_tables 
				WHERE schemaname = 'public' 
				AND tablename = $1
			)
		`, tableName).Scan(&tableExists)

		if err != nil {
			fmt.Printf("  ❌ Error checking %s: %v\n", tableName, err)
		} else if tableExists {
			fmt.Printf("  ✅ %s\n", tableName)
		} else {
			fmt.Printf("  ❌ %s (MISSING)\n", tableName)
		}
	}

	// Check table ownership and permissions
	fmt.Println("\nTable ownership and permissions:")
	rows, err := loader.db.Query(`
		SELECT 
			tablename,
			tableowner,
			hasindexes,
			hasrules,
			hastriggers
		FROM pg_tables 
		WHERE schemaname = 'public'
		ORDER BY tablename
	`)
	if err != nil {
		fmt.Printf("Error querying pg_tables: %v\n", err)
	} else {
		defer rows.Close()
		for rows.Next() {
			var tableName, owner string
			var hasIndexes, hasRules, hasTriggers bool
			if err := rows.Scan(&tableName, &owner, &hasIndexes, &hasRules, &hasTriggers); err != nil {
				fmt.Printf("Error scanning row: %v\n", err)
			} else {
				fmt.Printf("  %s (owner: %s, indexes: %t)\n", tableName, owner, hasIndexes)
			}
		}
	}

	// Check current user and permissions
	fmt.Println("\nChecking user and permissions:")
	var currentUser string
	err = loader.db.QueryRow("SELECT current_user").Scan(&currentUser)
	if err != nil {
		fmt.Printf("Error getting current user: %v\n", err)
	} else {
		fmt.Printf("Current user: %s\n", currentUser)
	}

	var searchPath string
	err = loader.db.QueryRow("SHOW search_path").Scan(&searchPath)
	if err != nil {
		fmt.Printf("Error getting search path: %v\n", err)
	} else {
		fmt.Printf("Search path: %s\n", searchPath)
	}

	// Check all tables in all schemas
	fmt.Println("\nAll tables in database:")
	allRows, err := loader.db.Query(`
		SELECT table_schema, table_name, table_type
		FROM information_schema.tables 
		WHERE table_schema NOT IN ('information_schema', 'pg_catalog')
		ORDER BY table_schema, table_name
	`)
	if err != nil {
		fmt.Printf("Error querying tables: %v\n", err)
	} else {
		defer allRows.Close()
		for allRows.Next() {
			var schema, name, tableType string
			if err := allRows.Scan(&schema, &name, &tableType); err != nil {
				fmt.Printf("Error scanning row: %v\n", err)
			} else {
				fmt.Printf("  %s.%s (%s)\n", schema, name, tableType)
			}
		}
	}

	// If JSON file is provided, load it
	if jsonFilePath != "" {
		fmt.Printf("\nLoading JSON file: %s\n", jsonFilePath)
		if err := loader.LoadJSONFile(jsonFilePath); err != nil {
			log.Fatalf("Failed to load JSON file: %v", err)
		}
		fmt.Println("SQL loading completed successfully!")
	} else {
		fmt.Println("\nDatabase connection test completed successfully!")
		fmt.Println("To load a JSON file, provide it as an argument.")
	}
}
