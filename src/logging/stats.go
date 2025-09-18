package logging

import (
	"encoding/csv"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"cursor-twitter/src/config"
)

// InitializeStatsCSV initializes the stats CSV file and returns the path
func InitializeStatsCSV(logDir string) string {
	statsCSVPath := filepath.Join(logDir, "stats.csv")
	EnsureStatsCSVHeader(statsCSVPath)
	return statsCSVPath
}

// EnsureStatsCSVHeader creates the stats CSV file and writes the header if it doesn't exist.
func EnsureStatsCSVHeader(path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Printf("Failed to create stats CSV: %v", err)
			return
		}
		defer f.Close()
		writer := csv.NewWriter(f)
		writer.Write([]string{"timestamp", "total_tweets", "total_tokens", "distinct_tokens"})
		writer.Flush()
	}
}

// StartStatsPrinter launches a goroutine that prints stats every 5 seconds.
func StartStatsPrinter(printStatsFunc func()) {
	ticker := time.NewTicker(5 * time.Second)
	go func() {
		for range ticker.C {
			printStatsFunc()
		}
	}()
}

// PrintStats prints the current pipeline statistics and logs them as CSV.
// This function takes the necessary data as parameters to avoid global variable dependencies.
func PrintStats(
	totalTweets, totalTokens, distinctTokens int,
	lastStatsTime, pipelineStartTime time.Time,
	lastTweetCount int,
	inboundQueueSize int,
	freqClassQueueStats, freqClassProcessorStats map[string]int,
	freqClasses int,
	statsCSVPath string,
) {
	now := time.Now()
	timestamp := now.Format(time.RFC3339)

	// Calculate processing rate (recent period)
	timeDiff := now.Sub(lastStatsTime).Seconds()
	tweetDiff := totalTweets - lastTweetCount
	processingRate := float64(tweetDiff) / timeDiff

	// Calculate total rate for the whole run
	totalTimeDiff := now.Sub(pipelineStartTime).Seconds()
	totalRate := float64(totalTweets) / totalTimeDiff

	// Print frequency class stats (ordered from lowest to highest class number)
	slog.Debug("--- Frequency Class Stats ---")
	for i := 0; i < freqClasses; i++ {
		queueKey := fmt.Sprintf("freq_class_%d_queue_size", i)
		processorKey := fmt.Sprintf("freq_class_%d_tokens_processed", i)
		queueSize := freqClassQueueStats[queueKey]
		tokensProcessed := freqClassProcessorStats[processorKey]

		// Get distinct token count for this frequency class
		// Note: distinct token count is no longer available since we removed the old filter system
		distinctTokens := 0

		// Use lazy evaluation for expensive formatting
		classIndex := i
		slog.Debug("Frequency Class Stats", "class", classIndex, "queue", queueSize, "processed", tokensProcessed, "distinct", distinctTokens)
	}
	slog.Debug("----------------------")

	// Also log to slog
	slog.Info("Pipeline stats",
		"tweets", totalTweets,
		"tokens", totalTokens,
		"distinct", distinctTokens,
		"inbound_queue_size", inboundQueueSize,
		"total_rate_tweets_per_sec", totalRate,
		"processing_rate_tweets_per_sec", processingRate)

	// Log as CSV for machine consumption
	f, err := os.OpenFile(statsCSVPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("Failed to open stats CSV: %v", err)
		return
	}
	defer f.Close()
	writer := csv.NewWriter(f)
	writer.Write([]string{
		timestamp,
		fmt.Sprintf("%d", totalTweets),
		fmt.Sprintf("%d", totalTokens),
		fmt.Sprintf("%d", distinctTokens),
	})
	writer.Flush()
}

// PrintBatchSummary prints a summary of all busy words found in a batch
// This function is kept for compatibility but no longer outputs duplicate information
func PrintBatchSummary(classResults map[int][]string, batchNumber int64, cfg *config.Config) {
	// Note: Busy word summary is now logged in the structured format in runClusteringForBatch
	// This function is kept for compatibility but no longer outputs duplicate information
}

// GetCurrentWorkingDir returns the current working directory for debugging
func GetCurrentWorkingDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	return dir
}
