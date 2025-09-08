package main

import (
	"fmt"
	"os"
	"testing"
)

// TestConfigOverride tests the config override mechanism
func TestConfigOverride(t *testing.T) {
	// Create a temporary base config file
	baseConfig := `
mode: files
batch: 30000
rebuild_every_files: 5
freq_classes: 24
verbose: false
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
	cfg, err := loadConfigWithOverride(baseConfigPath, overrideConfigPath)
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

	if cfg.Verbose != false {
		t.Errorf("Expected Verbose to be false, got %v", cfg.Verbose)
	}

	if cfg.LogLevel != "INFO" {
		t.Errorf("Expected LogLevel to be INFO, got %s", cfg.LogLevel)
	}

	fmt.Printf("✅ Config override test passed!\n")
	fmt.Printf("   BatchSize: %d (expected 35000)\n", cfg.BatchSize)
	fmt.Printf("   RebuildEveryFiles: %d (expected 10)\n", cfg.RebuildEveryFiles)
	fmt.Printf("   FreqClasses: %d (expected 24)\n", cfg.FreqClasses)
	fmt.Printf("   Verbose: %v (expected false)\n", cfg.Verbose)
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
verbose: false
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
	cfg, err := loadConfigWithOverride(baseConfigPath, "")
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
