package main

import (
	"fmt"
	"os"
	"testing"

	"cursor-twitter/src/config"
)

// TestConfigOverride tests the config override mechanism
func TestConfigOverride(t *testing.T) {
	// Create a temporary base config file
	baseConfig := `
mode: files
batch: 30000
rebuild_every_files: 5
freq_classes: 24
log_level: INFO
`

	// Create a temporary override config file
	overrideConfig := `
batch: 35000
rebuild_every_files: 10
`

	// Write base config
	baseConfigPath := "test_base_config.yaml"
	err := os.WriteFile(baseConfigPath, []byte(baseConfig), 0644)
	if err != nil {
		t.Fatalf("Failed to write base config: %v", err)
	}
	defer os.Remove(baseConfigPath)

	// Write override config
	overrideConfigPath := "test_override_config.yaml"
	err = os.WriteFile(overrideConfigPath, []byte(overrideConfig), 0644)
	if err != nil {
		t.Fatalf("Failed to write override config: %v", err)
	}
	defer os.Remove(overrideConfigPath)

	// Test the override mechanism
	cfg, err := config.LoadConfigWithOverride(baseConfigPath, overrideConfigPath)
	if err != nil {
		t.Fatalf("Failed to load config with override: %v", err)
	}

	// Verify that overrides were applied
	if cfg.BatchSize != 35000 {
		t.Errorf("Expected BatchSize to be 35000, got %d", cfg.BatchSize)
	}

	if cfg.RebuildEveryFiles != 10 {
		t.Errorf("Expected RebuildEveryFiles to be 10, got %d", cfg.RebuildEveryFiles)
	}

	// Verify that non-overridden values remain from base config
	if cfg.FreqClasses != 24 {
		t.Errorf("Expected FreqClasses to be 24, got %d", cfg.FreqClasses)
	}

	if cfg.LogLevel != "INFO" {
		t.Errorf("Expected LogLevel to be INFO, got %s", cfg.LogLevel)
	}

	fmt.Printf("✅ Config override test passed!\n")
	fmt.Printf("   BatchSize: %d (expected 35000)\n", cfg.BatchSize)
	fmt.Printf("   RebuildEveryFiles: %d (expected 10)\n", cfg.RebuildEveryFiles)
	fmt.Printf("   FreqClasses: %d (expected 24)\n", cfg.FreqClasses)
	fmt.Printf("   LogLevel: %s (expected INFO)\n", cfg.LogLevel)
}

// TestConfigOverrideNoOverride tests config loading without override
func TestConfigOverrideNoOverride(t *testing.T) {
	// Create a temporary base config file
	baseConfig := `
mode: files
batch: 30000
rebuild_every_files: 5
freq_classes: 24
log_level: INFO
`

	// Write base config
	baseConfigPath := "test_base_config_no_override.yaml"
	err := os.WriteFile(baseConfigPath, []byte(baseConfig), 0644)
	if err != nil {
		t.Fatalf("Failed to write base config: %v", err)
	}
	defer os.Remove(baseConfigPath)

	// Test loading without override
	cfg, err := config.LoadConfigWithOverride(baseConfigPath, "")
	if err != nil {
		t.Fatalf("Failed to load config without override: %v", err)
	}

	// Verify that base config values are used
	if cfg.BatchSize != 30000 {
		t.Errorf("Expected BatchSize to be 30000, got %d", cfg.BatchSize)
	}

	if cfg.RebuildEveryFiles != 5 {
		t.Errorf("Expected RebuildEveryFiles to be 5, got %d", cfg.RebuildEveryFiles)
	}

	fmt.Printf("✅ Config no-override test passed!\n")
	fmt.Printf("   BatchSize: %d (expected 30000)\n", cfg.BatchSize)
	fmt.Printf("   RebuildEveryFiles: %d (expected 5)\n", cfg.RebuildEveryFiles)
}

// TestConfigOverrideAnalysisSection tests the analysis section override
func TestConfigOverrideAnalysisSection(t *testing.T) {
	// Create a temporary base config file with analysis section
	baseConfig := `
mode: files
batch: 30000
log_level: DEBUG
analysis:
  min_cluster_size: 2
  min_jaccard_similarity: 0.2
  jaccard_use_busy_words_only: false
  use_levenshtein_deduplication: true
  filter_repetitive_patterns: true
  deduplicate_by_user: true
  create_fallback_clusters: false
  max_tweets_to_cluster: 1000
  max_human_tweets_displayed: 10
`

	// Create a temporary override config file with analysis section
	overrideConfig := `
batch: 35000
log_level: INFO
analysis:
  min_cluster_size: 5
  min_jaccard_similarity: 0.1
  jaccard_use_busy_words_only: true
  use_levenshtein_deduplication: false
  filter_repetitive_patterns: false
  deduplicate_by_user: false
  create_fallback_clusters: true
  max_tweets_to_cluster: 2000
  max_tweets_displayed: 15
`

	// Write base config
	baseConfigPath := "test_base_analysis_config.yaml"
	err := os.WriteFile(baseConfigPath, []byte(baseConfig), 0644)
	if err != nil {
		t.Fatalf("Failed to write base config: %v", err)
	}
	defer os.Remove(baseConfigPath)

	// Write override config
	overrideConfigPath := "test_override_analysis_config.yaml"
	err = os.WriteFile(overrideConfigPath, []byte(overrideConfig), 0644)
	if err != nil {
		t.Fatalf("Failed to write override config: %v", err)
	}
	defer os.Remove(overrideConfigPath)

	// Test the override mechanism
	cfg, err := config.LoadConfigWithOverride(baseConfigPath, overrideConfigPath)
	if err != nil {
		t.Fatalf("Failed to load config with override: %v", err)
	}

	// Verify that analysis section overrides were applied
	if cfg.Analysis.MinClusterSize != 5 {
		t.Errorf("Expected MinClusterSize to be 5, got %d", cfg.Analysis.MinClusterSize)
	}

	if cfg.Analysis.MinJaccardSimilarity != 0.1 {
		t.Errorf("Expected MinJaccardSimilarity to be 0.1, got %f", cfg.Analysis.MinJaccardSimilarity)
	}

	if cfg.Analysis.JaccardUseBusyWordsOnly != true {
		t.Errorf("Expected JaccardUseBusyWordsOnly to be true, got %t", cfg.Analysis.JaccardUseBusyWordsOnly)
	}

	if cfg.Analysis.UseLevenshteinDeduplication != false {
		t.Errorf("Expected UseLevenshteinDeduplication to be false, got %t", cfg.Analysis.UseLevenshteinDeduplication)
	}

	if cfg.Analysis.FilterRepetitivePatterns != false {
		t.Errorf("Expected FilterRepetitivePatterns to be false, got %t", cfg.Analysis.FilterRepetitivePatterns)
	}

	if cfg.Analysis.DeduplicateByUser != false {
		t.Errorf("Expected DeduplicateByUser to be false, got %t", cfg.Analysis.DeduplicateByUser)
	}

	if cfg.Analysis.CreateFallbackClusters != true {
		t.Errorf("Expected CreateFallbackClusters to be true, got %t", cfg.Analysis.CreateFallbackClusters)
	}

	if cfg.Analysis.MaxTweetsToCluster != 2000 {
		t.Errorf("Expected MaxTweetsToCluster to be 2000, got %d", cfg.Analysis.MaxTweetsToCluster)
	}

	if cfg.Analysis.MaxTweetsDisplayed != 15 {
		t.Errorf("Expected MaxTweetsDisplayed to be 15, got %d", cfg.Analysis.MaxTweetsDisplayed)
	}

	// Verify that non-analysis overrides were also applied
	if cfg.BatchSize != 35000 {
		t.Errorf("Expected BatchSize to be 35000, got %d", cfg.BatchSize)
	}

	if cfg.LogLevel != "INFO" {
		t.Errorf("Expected LogLevel to be INFO, got %s", cfg.LogLevel)
	}

	fmt.Printf("✅ Analysis section override test passed!\n")
	fmt.Printf("   MinClusterSize: %d (expected 5)\n", cfg.Analysis.MinClusterSize)
	fmt.Printf("   MinJaccardSimilarity: %f (expected 0.1)\n", cfg.Analysis.MinJaccardSimilarity)
	fmt.Printf("   JaccardUseBusyWordsOnly: %t (expected true)\n", cfg.Analysis.JaccardUseBusyWordsOnly)
	fmt.Printf("   CreateFallbackClusters: %t (expected true)\n", cfg.Analysis.CreateFallbackClusters)
	fmt.Printf("   MaxTweetsDisplayed: %d (expected 15)\n", cfg.Analysis.MaxTweetsDisplayed)
}
