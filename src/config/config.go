package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"cursor-twitter/src/regex"

	"gopkg.in/yaml.v3"
)

// Config struct for YAML config file (add log_dir)
type Config struct {
	Mode          string `yaml:"mode"`
	InputDir      string `yaml:"input"`
	FileSrcDir    string `yaml:"file_src_dir"` // Source directory for file input mode
	MQHost        string `yaml:"mq_host"`
	MQPort        int    `yaml:"mq_port"`
	MQQueue       string `yaml:"mq_queue"`
	WindowSize    int    `yaml:"window"`
	BatchSize     int    `yaml:"batch"`
	WindowBatches int    `yaml:"window_batches"` // Number of batches to keep in tweet window

	LogDir               string    `yaml:"log_dir"`
	LogLevel             string    `yaml:"log_level"` // DEBUG, INFO, WARN, ERROR
	FreqClasses          int       `yaml:"freq_classes"`
	BWArrayLen           int       `yaml:"bw_array_len"`
	ZScores              []float64 `yaml:"z_scores"`
	MinTokenLen          int       `yaml:"min_token_len"`
	SkipFrequencyClasses []int     `yaml:"skip_frequency_classes"`
	TokenPersistFiles    int       `yaml:"token_persist_files"`
	RebuildEveryFiles    int       `yaml:"rebuild_every_files"`
	MinCountThreshold    int       `yaml:"min_count_threshold"` // Minimum count for frequency class inclusion
	BusywordClasses      []int     `yaml:"busyword_classes"`    // Frequency classes to use for clustering

	Filter struct {
		Enabled    bool   `yaml:"enabled"`
		FilterDir  string `yaml:"filter_dir"`
		FilterFile string `yaml:"filter_file"` // Keep for backward compatibility
	} `yaml:"filter"`

	TokenFilters struct {
		Enabled                         bool    `yaml:"enabled"`
		MaxLength                       int     `yaml:"max_length"`
		MinCharacterDiversity           float64 `yaml:"min_character_diversity"`
		MinCharacterDiversityLowerLimit int     `yaml:"min_character_diversity_lower_limit"`
		MaxCharacterRepetition          float64 `yaml:"max_character_repetition"`
		MaxCaseAlternations             float64 `yaml:"max_case_alternations"`
		MaxNumberLetterMix              float64 `yaml:"max_number_letter_mix"`
		RejectHashtags                  bool    `yaml:"reject_hashtags"`
		RejectAtMentions                bool    `yaml:"reject_at_mentions"`
		RejectUrls                      bool    `yaml:"reject_urls"`
		RejectAllCapsLong               bool    `yaml:"reject_all_caps_long"`
		AllCapsLowerLimit               int     `yaml:"all_caps_lower_limit"`
		RemoveUrls                      bool    `yaml:"remove_urls"`
		ApostropheHandling              string  `yaml:"apostrophe_handling"`
	} `yaml:"token_filters"`

	Persistence struct {
		StateDir string `yaml:"state_dir"`
	} `yaml:"persistence"`

	Sender struct {
		StatusFile string `yaml:"status_file"`
	} `yaml:"sender"`

	Analysis struct {
		ClusteringWindowBatches      int     `yaml:"clustering_window_batches"`      // Number of batches of recent tweets to use for clustering
		MinBusyWordsPerTweet         int     `yaml:"min_busy_words_per_tweet"`       // Minimum number of busy words a tweet must contain to be included in clustering
		MinJaccardSimilarity         float64 `yaml:"min_jaccard_similarity"`         // Minimum Jaccard similarity threshold for creating edges between tweets
		JaccardUseBusyWordsOnly      bool    `yaml:"jaccard_use_busy_words_only"`    // If true, Jaccard similarity uses only busy words; if false, uses all tokens
		MaxTweetsToCluster           int     `yaml:"max_tweets_to_cluster"`          // Maximum number of tweets to cluster (0 = no limit)
		SuppressDuplicates           bool    `yaml:"suppress_duplicates"`            // Suppress duplicate tweets in visualization
		DuplicateSimilarityThreshold float64 `yaml:"duplicate_similarity_threshold"` // Similarity threshold for duplicates
		LanguageFilter               string  `yaml:"language_filter"`                // Language filter: "en", "es", "all", etc.
		ClusteringMethod             string  `yaml:"clustering_method"`              // Method for clustering: "graph" (only valid option)
		OutputMode                   string  `yaml:"output_mode"`                    // Output mode: "verbose" or "human"
		MinClusterSize               int     `yaml:"min_cluster_size"`               // Minimum number of tweets in a cluster for it to be included in the output
		CreateFallbackClusters       bool    `yaml:"create_fallback_clusters"`       // Create fallback clusters when no clusters found but tweets exist
		// Persistence window configuration for tracking clusters across multiple batches
		WindowBatchesPersistence      int `yaml:"window_batches_persistence"`       // M
		WindowBatchesPersistenceCheck int `yaml:"window_batches_persistence_check"` // K
		// Minimum number of shared busy words required for clusters to be considered related (for persistence tracking)
		MinSharedBusyWordsForPersistence int `yaml:"min_shared_busywords_for_persistence"` // Relationship strength threshold
		// Method for determining cluster relationships across batches: "busy_words" or "full_text"
		PersistenceClusteringMethod    string           `yaml:"persistence_clustering_method"` // Cross-batch relationship detection method
		DropExcessiveQuestions         bool             `yaml:"drop_excessive_questions"`      // Drop tweets with excessive question marks
		MaxHumanTweetsDisplayed        int              `yaml:"max_human_tweets_displayed"`    // Maximum number of tweets to display in human-readable format
		FilterRepetitivePatterns       bool             `yaml:"filter_repetitive_patterns"`    // Filter out clusters with repetitive meme-like patterns
		BannedPhrasesDir               string           `yaml:"banned_phrases_dir"`            // Path to directory containing banned phrase files
		BannedPhrasesFile              string           `yaml:"banned_phrases_file"`           // Path to file containing banned phrases (backward compatibility)
		RepetitivePatternThreshold     float64          `yaml:"repetitive_pattern_threshold"`  // Threshold for filtering repetitive clusters
		CompiledBannedPatterns         []*regexp.Regexp // Compiled regex patterns (not in yaml)
		DeduplicateByUser              bool             `yaml:"deduplicate_by_user"`               // Deduplicate tweets by user within clusters
		UseLevenshteinDeduplication    bool             `yaml:"use_levenshtein_deduplication"`     // Use distance-based deduplication
		DistanceMethod                 string           `yaml:"distance_method"`                   // "character" or "word" distance method
		NearDuplicateThreshold         float64          `yaml:"near_duplicate_threshold"`          // Normalized distance threshold
		CleanupTriggerBatchSize        int              `yaml:"cleanup_trigger_batch_size"`        // Trigger cleanup every N tweets
		CleanupMaxItems                int              `yaml:"cleanup_max_items"`                 // Process up to M items per cleanup cycle
		ClusterSortDescending          bool             `yaml:"cluster_sort_descending"`           // Sort clusters by size: true=descending (biggest first), false=ascending (biggest last)
		SuppressIndividualTweets       bool             `yaml:"suppress_individual_tweets"`        // Suppress individual tweets in output, keep only metadata and medoid
		EnableMetaClustering           bool             `yaml:"enable_meta_clustering"`            // Enable clustering of clusters into meta-clusters
		MetaClusterSimilarityThreshold float64          `yaml:"meta_cluster_similarity_threshold"` // Similarity threshold for merging clusters (0.3-0.6)
		MetaClusterMinSize             int              `yaml:"meta_cluster_min_size"`             // Minimum total tweets for a meta-cluster
		UseMedoidSimilarity            bool             `yaml:"use_medoid_similarity"`             // Enable medoid similarity in meta-clustering
		UseBusyWordSimilarity          bool             `yaml:"use_busy_word_similarity"`          // Enable busy word similarity in meta-clustering
		UseUnionApproach               bool             `yaml:"use_union_approach"`                // Use union of medoid and busy word meta-clustering
		MedoidSimilarityThreshold      float64          `yaml:"medoid_similarity_threshold"`       // Separate threshold for medoid similarity
		BusyWordSimilarityThreshold    float64          `yaml:"busy_word_similarity_threshold"`    // Separate threshold for busy word similarity
		BWQueueMax                     float64          `yaml:"bw_queue_max"`                      // Multiplier for batch size to trigger busyword queue warnings
		BWThreadSlowDelay              int              `yaml:"bw_thread_slow_delay"`              // Total sleep time in milliseconds when busyword queues are backlogged
	} `yaml:"analysis"`
}

// LoadConfig loads the YAML config file into a Config struct.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadAndValidateConfig loads and validates configuration
func LoadAndValidateConfig(path string) (*Config, error) {
	cfg, err := LoadConfig(path)
	if err != nil {
		return nil, err
	}
	if cfg.LogDir == "" {
		return nil, fmt.Errorf("ERROR: 'log_dir' must be defined in the config file and cannot be empty.")
	}

	// Note: Banned phrases loading moved to after path resolution
	// to handle relative paths correctly

	return cfg, nil
}

// resolvePathRelativeToConfig takes a path and resolves it relative to the project root
// If the path is already absolute, it returns it unchanged
// If the path is relative, it's resolved relative to the project root (where the program runs)
func resolvePathRelativeToConfig(configPath, relativePath string) string {
	// If the path is already absolute, return it unchanged
	if filepath.IsAbs(relativePath) {
		return relativePath
	}

	// Get the project root (parent of config directory)
	configDir := filepath.Dir(configPath)
	projectRoot := filepath.Dir(configDir)

	// Resolve the relative path from the project root
	resolvedPath := filepath.Join(projectRoot, relativePath)

	// Clean the path (resolve any .. or . components)
	resolvedPath = filepath.Clean(resolvedPath)

	return resolvedPath
}

// resolvePathsInConfig takes a config struct and resolves all relative paths
// relative to the config file location
func resolvePathsInConfig(configPath string, cfg *Config) error {
	// Resolve log directory
	if cfg.LogDir != "" {
		cfg.LogDir = resolvePathRelativeToConfig(configPath, cfg.LogDir)
	}

	// Resolve file source directory
	if cfg.FileSrcDir != "" {
		cfg.FileSrcDir = resolvePathRelativeToConfig(configPath, cfg.FileSrcDir)
	}

	// Resolve filter directory
	if cfg.Filter.FilterDir != "" {
		cfg.Filter.FilterDir = resolvePathRelativeToConfig(configPath, cfg.Filter.FilterDir)
	}

	// Resolve persistence state directory
	if cfg.Persistence.StateDir != "" {
		cfg.Persistence.StateDir = resolvePathRelativeToConfig(configPath, cfg.Persistence.StateDir)
	}

	// Resolve sender status file
	if cfg.Sender.StatusFile != "" {
		cfg.Sender.StatusFile = resolvePathRelativeToConfig(configPath, cfg.Sender.StatusFile)
	}

	// Resolve banned phrases directory
	if cfg.Analysis.BannedPhrasesDir != "" {
		cfg.Analysis.BannedPhrasesDir = resolvePathRelativeToConfig(configPath, cfg.Analysis.BannedPhrasesDir)

		// Load and compile banned phrases after path resolution
		if cfg.Analysis.FilterRepetitivePatterns {
			var patterns []*regexp.Regexp
			var err error

			// Try directory first (new approach)
			if cfg.Analysis.BannedPhrasesDir != "" {
				patterns, err = regex.LoadBannedPhrasesFromDirectory(cfg.Analysis.BannedPhrasesDir)
			} else if cfg.Analysis.BannedPhrasesFile != "" {
				// Fall back to single file (backward compatibility)
				patterns, err = regex.LoadBannedPhrasesFromFile(cfg.Analysis.BannedPhrasesFile)
			} else {
				return fmt.Errorf("neither banned_phrases_dir nor banned_phrases_file specified in config")
			}

			if err != nil {
				return fmt.Errorf("failed to load banned phrases: %v", err)
			}
			cfg.Analysis.CompiledBannedPatterns = patterns
		}
	}

	return nil
}

// LoadAndValidateConfigWithPathResolution loads config and resolves all relative paths
func LoadAndValidateConfigWithPathResolution(configPath string) (*Config, error) {
	// Convert config path to absolute path
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for config: %v", err)
	}

	// Load the config first
	cfg, err := LoadAndValidateConfig(configPath)
	if err != nil {
		return nil, err
	}

	// Resolve all relative paths using absolute config path
	if err := resolvePathsInConfig(absConfigPath, cfg); err != nil {
		return nil, fmt.Errorf("failed to resolve paths in config: %v", err)
	}

	return cfg, nil
}

// LoadConfigWithOverride loads a base config and optionally applies an override config
func LoadConfigWithOverride(baseConfigPath, overrideConfigPath string) (*Config, error) {
	// Load the base config
	cfg, err := LoadAndValidateConfigWithPathResolution(baseConfigPath)
	if err != nil {
		return nil, err
	}

	// If no override specified, return the base config
	if overrideConfigPath == "" {
		return cfg, nil
	}

	// Apply override values directly from the YAML file
	if err := applyOverrideFromYAML(cfg, overrideConfigPath); err != nil {
		return nil, fmt.Errorf("failed to apply override config: %v", err)
	}

	return cfg, nil
}

// applyOverrideFromYAML applies only the values that are present in the override YAML file
func applyOverrideFromYAML(cfg *Config, overrideConfigPath string) error {
	// Read the override config file
	data, err := os.ReadFile(overrideConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read override config file: %v", err)
	}

	// Parse YAML into a map to see what keys are actually present
	var overrideMap map[string]interface{}
	if err := yaml.Unmarshal(data, &overrideMap); err != nil {
		return fmt.Errorf("failed to parse override YAML: %v", err)
	}

	// Apply only the values that are present in the override file
	for key, value := range overrideMap {
		switch key {
		case "mode":
			if str, ok := value.(string); ok {
				cfg.Mode = str
			}
		case "batch":
			if num, ok := value.(int); ok {
				cfg.BatchSize = num
			}
		case "window":
			if num, ok := value.(int); ok {
				cfg.WindowSize = num
			}
		case "log_level":
			if str, ok := value.(string); ok {
				cfg.LogLevel = str
			}
		case "log_dir":
			if str, ok := value.(string); ok {
				cfg.LogDir = str
			}
		case "freq_classes":
			if num, ok := value.(int); ok {
				cfg.FreqClasses = num
			}
		case "bw_array_len":
			if num, ok := value.(int); ok {
				cfg.BWArrayLen = num
			}
		case "token_persist_files":
			if num, ok := value.(int); ok {
				cfg.TokenPersistFiles = num
			}
		case "rebuild_every_files":
			if num, ok := value.(int); ok {
				cfg.RebuildEveryFiles = num
			}
		case "window_batches":
			if num, ok := value.(int); ok {
				cfg.WindowBatches = num
			}
		case "min_count_threshold":
			if num, ok := value.(int); ok {
				cfg.MinCountThreshold = num
			}
		case "z_scores":
			if scores, ok := value.([]interface{}); ok {
				var floatScores []float64
				for _, score := range scores {
					if f, ok := score.(float64); ok {
						floatScores = append(floatScores, f)
					}
				}
				cfg.ZScores = floatScores
			}
		case "skip_frequency_classes":
			if classes, ok := value.([]interface{}); ok {
				var intClasses []int
				for _, class := range classes {
					if i, ok := class.(int); ok {
						intClasses = append(intClasses, i)
					}
				}
				cfg.SkipFrequencyClasses = intClasses
			}
		case "busyword_classes":
			if classes, ok := value.([]interface{}); ok {
				var intClasses []int
				for _, class := range classes {
					if i, ok := class.(int); ok {
						intClasses = append(intClasses, i)
					}
				}
				cfg.BusywordClasses = intClasses
			}
		case "analysis":
			if analysisMap, ok := value.(map[interface{}]interface{}); ok {
				// Handle analysis section overrides
				if minClusterSize, ok := analysisMap["min_cluster_size"].(int); ok {
					cfg.Analysis.MinClusterSize = minClusterSize
				}
				if jaccardUseBusyWordsOnly, ok := analysisMap["jaccard_use_busy_words_only"].(bool); ok {
					cfg.Analysis.JaccardUseBusyWordsOnly = jaccardUseBusyWordsOnly
				}
				if minJaccardSimilarity, ok := analysisMap["min_jaccard_similarity"].(float64); ok {
					cfg.Analysis.MinJaccardSimilarity = minJaccardSimilarity
				}
				if useLevenshteinDeduplication, ok := analysisMap["use_levenshtein_deduplication"].(bool); ok {
					cfg.Analysis.UseLevenshteinDeduplication = useLevenshteinDeduplication
				}
				if filterRepetitivePatterns, ok := analysisMap["filter_repetitive_patterns"].(bool); ok {
					cfg.Analysis.FilterRepetitivePatterns = filterRepetitivePatterns
				}
				if deduplicateByUser, ok := analysisMap["deduplicate_by_user"].(bool); ok {
					cfg.Analysis.DeduplicateByUser = deduplicateByUser
				}
				if createFallbackClusters, ok := analysisMap["create_fallback_clusters"].(bool); ok {
					cfg.Analysis.CreateFallbackClusters = createFallbackClusters
				}
				if maxTweetsToCluster, ok := analysisMap["max_tweets_to_cluster"].(int); ok {
					cfg.Analysis.MaxTweetsToCluster = maxTweetsToCluster
				}
			}
			// Add more cases as needed for other fields
		}
	}

	return nil
}

// LoadConfigWithoutValidation loads a config file without validation
// Used for override files that may be incomplete
func LoadConfigWithoutValidation(configPath string) (*Config, error) {
	// Read the config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	// Parse YAML
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %v", err)
	}

	return &cfg, nil
}

// mergeConfigs merges override values into the base config
// Only non-zero/non-empty values from override are applied
func mergeConfigs(base, override *Config) {
	// Merge basic fields
	if override.Mode != "" {
		base.Mode = override.Mode
	}
	if override.MQHost != "" {
		base.MQHost = override.MQHost
	}
	if override.MQPort != 0 {
		base.MQPort = override.MQPort
	}
	if override.MQQueue != "" {
		base.MQQueue = override.MQQueue
	}
	if override.FileSrcDir != "" {
		base.FileSrcDir = override.FileSrcDir
	}
	if override.WindowSize != 0 {
		base.WindowSize = override.WindowSize
	}
	if override.TokenPersistFiles != 0 {
		base.TokenPersistFiles = override.TokenPersistFiles
	}
	if override.RebuildEveryFiles != 0 {
		base.RebuildEveryFiles = override.RebuildEveryFiles
	}
	if override.BatchSize != 0 {
		base.BatchSize = override.BatchSize
	}
	if override.WindowBatches != 0 {
		base.WindowBatches = override.WindowBatches
	}
	if override.LogDir != "" {
		base.LogDir = override.LogDir
	}
	if override.LogLevel != "" {
		base.LogLevel = override.LogLevel
	}
	if len(override.ZScores) > 0 {
		base.ZScores = override.ZScores
	}
	if override.FreqClasses != 0 {
		base.FreqClasses = override.FreqClasses
	}
	if override.BWArrayLen != 0 {
		base.BWArrayLen = override.BWArrayLen
	}
	if len(override.SkipFrequencyClasses) > 0 {
		base.SkipFrequencyClasses = override.SkipFrequencyClasses
	}
	if len(override.BusywordClasses) > 0 {
		base.BusywordClasses = override.BusywordClasses
	}
	if override.MinCountThreshold != 0 {
		base.MinCountThreshold = override.MinCountThreshold
	}

	// Merge filter settings
	if override.Filter.Enabled {
		base.Filter.Enabled = override.Filter.Enabled
	}
	if override.Filter.FilterDir != "" {
		base.Filter.FilterDir = override.Filter.FilterDir
	}

	// Merge persistence settings
	if override.Persistence.StateDir != "" {
		base.Persistence.StateDir = override.Persistence.StateDir
	}

	// Merge sender settings
	if override.Sender.StatusFile != "" {
		base.Sender.StatusFile = override.Sender.StatusFile
	}

	// Merge analysis settings
	if override.Analysis.ClusteringWindowBatches != 0 {
		base.Analysis.ClusteringWindowBatches = override.Analysis.ClusteringWindowBatches
	}
	if override.Analysis.MinBusyWordsPerTweet != 0 {
		base.Analysis.MinBusyWordsPerTweet = override.Analysis.MinBusyWordsPerTweet
	}
	if override.Analysis.MinJaccardSimilarity != 0 {
		base.Analysis.MinJaccardSimilarity = override.Analysis.MinJaccardSimilarity
	}
	if override.Analysis.MinClusterSize != 0 {
		base.Analysis.MinClusterSize = override.Analysis.MinClusterSize
	}
	if override.Analysis.JaccardUseBusyWordsOnly {
		base.Analysis.JaccardUseBusyWordsOnly = override.Analysis.JaccardUseBusyWordsOnly
	}
	if override.Analysis.MaxTweetsToCluster != 0 {
		base.Analysis.MaxTweetsToCluster = override.Analysis.MaxTweetsToCluster
	}
	if override.Analysis.SuppressDuplicates {
		base.Analysis.SuppressDuplicates = override.Analysis.SuppressDuplicates
	}
	if override.Analysis.DuplicateSimilarityThreshold != 0 {
		base.Analysis.DuplicateSimilarityThreshold = override.Analysis.DuplicateSimilarityThreshold
	}
	if override.Analysis.LanguageFilter != "" {
		base.Analysis.LanguageFilter = override.Analysis.LanguageFilter
	}
	if override.Analysis.WindowBatchesPersistence != 0 {
		base.Analysis.WindowBatchesPersistence = override.Analysis.WindowBatchesPersistence
	}
	if override.Analysis.WindowBatchesPersistenceCheck != 0 {
		base.Analysis.WindowBatchesPersistenceCheck = override.Analysis.WindowBatchesPersistenceCheck
	}
	if override.Analysis.MinSharedBusyWordsForPersistence != 0 {
		base.Analysis.MinSharedBusyWordsForPersistence = override.Analysis.MinSharedBusyWordsForPersistence
	}
	if override.Analysis.PersistenceClusteringMethod != "" {
		base.Analysis.PersistenceClusteringMethod = override.Analysis.PersistenceClusteringMethod
	}
	if override.Analysis.MaxHumanTweetsDisplayed != 0 {
		base.Analysis.MaxHumanTweetsDisplayed = override.Analysis.MaxHumanTweetsDisplayed
	}
	if override.Analysis.FilterRepetitivePatterns {
		base.Analysis.FilterRepetitivePatterns = override.Analysis.FilterRepetitivePatterns
	}
	if override.Analysis.BannedPhrasesDir != "" {
		base.Analysis.BannedPhrasesDir = override.Analysis.BannedPhrasesDir
	}
	if override.Analysis.RepetitivePatternThreshold != 0 {
		base.Analysis.RepetitivePatternThreshold = override.Analysis.RepetitivePatternThreshold
	}
	if override.Analysis.DeduplicateByUser {
		base.Analysis.DeduplicateByUser = override.Analysis.DeduplicateByUser
	}
	if override.Analysis.UseLevenshteinDeduplication {
		base.Analysis.UseLevenshteinDeduplication = override.Analysis.UseLevenshteinDeduplication
	}
	if override.Analysis.DistanceMethod != "" {
		base.Analysis.DistanceMethod = override.Analysis.DistanceMethod
	}
	if override.Analysis.NearDuplicateThreshold != 0 {
		base.Analysis.NearDuplicateThreshold = override.Analysis.NearDuplicateThreshold
	}
	if override.Analysis.CleanupTriggerBatchSize != 0 {
		base.Analysis.CleanupTriggerBatchSize = override.Analysis.CleanupTriggerBatchSize
	}
	if override.Analysis.CleanupMaxItems != 0 {
		base.Analysis.CleanupMaxItems = override.Analysis.CleanupMaxItems
	}
	if override.Analysis.ClusterSortDescending {
		base.Analysis.ClusterSortDescending = override.Analysis.ClusterSortDescending
	}
	if override.Analysis.SuppressIndividualTweets != base.Analysis.SuppressIndividualTweets {
		base.Analysis.SuppressIndividualTweets = override.Analysis.SuppressIndividualTweets
	}
	if override.Analysis.EnableMetaClustering != base.Analysis.EnableMetaClustering {
		base.Analysis.EnableMetaClustering = override.Analysis.EnableMetaClustering
	}
	if override.Analysis.MetaClusterSimilarityThreshold != 0 {
		base.Analysis.MetaClusterSimilarityThreshold = override.Analysis.MetaClusterSimilarityThreshold
	}
	if override.Analysis.MetaClusterMinSize != 0 {
		base.Analysis.MetaClusterMinSize = override.Analysis.MetaClusterMinSize
	}
	if override.Analysis.UseMedoidSimilarity != base.Analysis.UseMedoidSimilarity {
		base.Analysis.UseMedoidSimilarity = override.Analysis.UseMedoidSimilarity
	}
	if override.Analysis.UseBusyWordSimilarity != base.Analysis.UseBusyWordSimilarity {
		base.Analysis.UseBusyWordSimilarity = override.Analysis.UseBusyWordSimilarity
	}
	if override.Analysis.UseUnionApproach != base.Analysis.UseUnionApproach {
		base.Analysis.UseUnionApproach = override.Analysis.UseUnionApproach
	}
	if override.Analysis.MedoidSimilarityThreshold != 0 {
		base.Analysis.MedoidSimilarityThreshold = override.Analysis.MedoidSimilarityThreshold
	}
	if override.Analysis.BusyWordSimilarityThreshold != 0 {
		base.Analysis.BusyWordSimilarityThreshold = override.Analysis.BusyWordSimilarityThreshold
	}
	if override.Analysis.BWQueueMax != 0 {
		base.Analysis.BWQueueMax = override.Analysis.BWQueueMax
	}
	if override.Analysis.BWThreadSlowDelay != 0 {
		base.Analysis.BWThreadSlowDelay = override.Analysis.BWThreadSlowDelay
	}
}
