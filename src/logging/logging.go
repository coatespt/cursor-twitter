package logging

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cursor-twitter/src/config"
)

// InitializeLogger sets up the slog logger based on configuration
func InitializeLogger(cfg *config.Config) (*slog.Logger, *os.File, error) {
	// Set the slog level based on config
	var slogLevel slog.Level
	switch strings.ToUpper(cfg.LogLevel) {
	case "DEBUG":
		slogLevel = slog.LevelDebug // DEBUG shows all messages (DEBUG, INFO, WARN, ERROR)
	case "INFO":
		slogLevel = slog.LevelInfo // INFO shows INFO, WARN, ERROR
	case "WARN":
		slogLevel = slog.LevelWarn // WARN shows WARN, ERROR
	case "ERROR":
		slogLevel = slog.LevelError // ERROR shows only ERROR
	default:
		slogLevel = slog.LevelInfo
	}

	logger, logFile, err := SetupLogger(cfg.LogDir, slogLevel)
	if err != nil {
		return nil, nil, err
	}

	return logger, logFile, nil
}

// SetupLogger creates the log directory if needed and returns a slog.Logger that writes to a file.
func SetupLogger(logDir string, level slog.Level) (*slog.Logger, *os.File, error) {
	// No default! logDir must be set by config and checked in main()
	if logDir == "" {
		return nil, nil, fmt.Errorf("logDir must be set in config; refusing to use a default")
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, nil, err
	}

	// Create timestamped log filename for sortability
	timestamp := time.Now().Format("20060102_150405")
	logPath := filepath.Join(logDir, fmt.Sprintf("pipeline_%s.log", timestamp))

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, nil, err
	}
	logger := slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{
		Level: level,
	}))
	return logger, logFile, nil
}
