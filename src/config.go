package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"
)

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
				patterns, err = loadBannedPhrasesFromDirectory(cfg.Analysis.BannedPhrasesDir)
			} else if cfg.Analysis.BannedPhrasesFile != "" {
				// Fall back to single file (backward compatibility)
				patterns, err = loadBannedPhrases(cfg.Analysis.BannedPhrasesFile)
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

// loadAndValidateConfigWithPathResolution loads config and resolves all relative paths
func loadAndValidateConfigWithPathResolution(configPath string) (*Config, error) {
	// Convert config path to absolute path
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for config: %v", err)
	}

	// Load the config first
	cfg, err := loadAndValidateConfig(configPath)
	if err != nil {
		return nil, err
	}

	// Resolve all relative paths using absolute config path
	if err := resolvePathsInConfig(absConfigPath, cfg); err != nil {
		return nil, fmt.Errorf("failed to resolve paths in config: %v", err)
	}

	return cfg, nil
}

// loadConfigWithOverride loads a base config and optionally applies an override config
func loadConfigWithOverride(baseConfigPath, overrideConfigPath string) (*Config, error) {
	// Load the base config
	cfg, err := loadAndValidateConfigWithPathResolution(baseConfigPath)
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
		case "verbose":
			if b, ok := value.(bool); ok {
				cfg.Verbose = b
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
			// Add more cases as needed for other fields
		}
	}

	return nil
}

// loadConfigWithoutValidation loads a config file without validation
// Used for override files that may be incomplete
func loadConfigWithoutValidation(configPath string) (*Config, error) {
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
	// Note: For boolean fields, we can't distinguish between "not set" and "false" in YAML
	// So we only override if the override value is true (since false is the default)
	if override.Verbose {
		base.Verbose = override.Verbose
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
	if override.Analysis.AnalyticsBatchLagThreshold != 0 {
		base.Analysis.AnalyticsBatchLagThreshold = override.Analysis.AnalyticsBatchLagThreshold
	}
	if override.Analysis.BWThreadSlowDelay != 0 {
		base.Analysis.BWThreadSlowDelay = override.Analysis.BWThreadSlowDelay
	}
	if override.Analysis.AnalyticsLagSlowDelay != 0 {
		base.Analysis.AnalyticsLagSlowDelay = override.Analysis.AnalyticsLagSlowDelay
	}
}
