package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// resolvePathRelativeToConfig takes a path and resolves it relative to the config file location
// If the path is already absolute, it returns it unchanged
// If the path is relative, it's resolved relative to the config file's directory
func resolvePathRelativeToConfig(configPath, relativePath string) string {
	// If the path is already absolute, return it unchanged
	if filepath.IsAbs(relativePath) {
		return relativePath
	}

	// Get the directory where the config file is located
	configDir := filepath.Dir(configPath)

	// Resolve the relative path from the config directory
	resolvedPath := filepath.Join(configDir, relativePath)

	// Clean the path (resolve any .. or . components)
	resolvedPath = filepath.Clean(resolvedPath)

	return resolvedPath
}

// loadConfigWithPathResolution loads config and resolves relative paths
func loadConfigWithPathResolution(configPath string) error {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %v", err)
	}

	if err := yaml.Unmarshal(content, &config); err != nil {
		return fmt.Errorf("failed to parse config: %v", err)
	}

	// Resolve relative paths in the config
	if config.InputFile != "" {
		config.InputFile = resolvePathRelativeToConfig(configPath, config.InputFile)
	}

	return nil
}
