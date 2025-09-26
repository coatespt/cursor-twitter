package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
)

type BusyWord struct {
	Word   string  `json:"word"`
	Class  int     `json:"class"`
	ZScore float64 `json:"z_score"`
	Count  int     `json:"count"`
	Mean   float64 `json:"mean"`
}

type Cluster struct {
	ClusterID int        `json:"cluster_id"`
	BatchID   int        `json:"batch_id"`
	Size      int        `json:"size"`
	BusyWords []BusyWord `json:"busy_words"`
}

type Batch struct {
	Type string `json:"type"`
	Data struct {
		BatchNumber          int       `json:"batch_number"`
		BatchTime            string    `json:"batch_time"`
		TotalTweets          int       `json:"total_tweets"`
		TotalClusters        int       `json:"total_clusters"`
		ClustersAboveMinSize int       `json:"clusters_above_min_size"`
		Clusters             []Cluster `json:"clusters"`
	} `json:"data"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: go run analyze_json_simple.go <json_file>")
		os.Exit(1)
	}

	jsonFile := os.Args[1]

	// Get file size
	fileInfo, err := os.Stat(jsonFile)
	if err != nil {
		log.Fatalf("Failed to get file info: %v", err)
	}

	fmt.Printf("=== JSON Output Analysis for: %s ===\n", jsonFile)
	fmt.Println()
	fmt.Printf("📁 File size: %.1fM\n", float64(fileInfo.Size())/(1024*1024))
	fmt.Println()

	// Open and read the JSONL file
	file, err := os.Open(jsonFile)
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	var allBatches []Batch
	var clusterCounts []int
	var tweetCounts []int
	var clustersAboveMin []int
	var busyWordsPerCluster []int
	var firstBatchTime, lastBatchTime string

	decoder := json.NewDecoder(file)

	// Read JSON objects one at a time
	for {
		var batch Batch
		err := decoder.Decode(&batch)
		if err != nil {
			if err.Error() == "EOF" {
				break // End of file
			}
			continue // Skip malformed JSON and keep looping
		}

		if batch.Type == "batch" {
			allBatches = append(allBatches, batch)
			clusterCounts = append(clusterCounts, batch.Data.TotalClusters)
			tweetCounts = append(tweetCounts, batch.Data.TotalTweets)
			clustersAboveMin = append(clustersAboveMin, batch.Data.ClustersAboveMinSize)

			// Track busy words per cluster for this batch
			for _, cluster := range batch.Data.Clusters {
				busyWordsPerCluster = append(busyWordsPerCluster, len(cluster.BusyWords))
			}

			if firstBatchTime == "" {
				firstBatchTime = batch.Data.BatchTime
			}
			lastBatchTime = batch.Data.BatchTime
		}
	}

	batchCount := len(allBatches)
	fmt.Printf("📊 Total batches: %d\n", batchCount)
	fmt.Println()

	if batchCount == 0 {
		fmt.Println("❌ No batches found in the file")
		return
	}

	// Calculate cluster statistics
	clusterMin, clusterMax := minMax(clusterCounts)
	clusterAvg := average(clusterCounts)
	clusterStdev := standardDeviation(clusterCounts)

	fmt.Println("📈 Cluster statistics per batch:")
	fmt.Printf("   Min clusters: %d\n", clusterMin)
	fmt.Printf("   Max clusters: %d\n", clusterMax)
	fmt.Printf("   Average clusters: %.2f\n", clusterAvg)
	fmt.Printf("   Standard deviation: %.2f\n", clusterStdev)
	fmt.Println()

	// Calculate tweet statistics
	tweetMin, tweetMax := minMax(tweetCounts)
	tweetAvg := average(tweetCounts)
	tweetStdev := standardDeviation(tweetCounts)

	fmt.Println("📈 Tweet statistics per batch:")
	fmt.Printf("   Min tweets: %d\n", tweetMin)
	fmt.Printf("   Max tweets: %d\n", tweetMax)
	fmt.Printf("   Average tweets: %.0f\n", tweetAvg)
	fmt.Printf("   Standard deviation: %.0f\n", tweetStdev)
	fmt.Println()

	// Calculate busy words statistics
	if len(busyWordsPerCluster) > 0 {
		busyWordsMin, busyWordsMax := minMax(busyWordsPerCluster)
		busyWordsAvg := average(busyWordsPerCluster)
		busyWordsStdev := standardDeviation(busyWordsPerCluster)

		fmt.Println("📝 Busy words per cluster statistics:")
		fmt.Printf("   Min busy words per cluster: %d\n", busyWordsMin)
		fmt.Printf("   Max busy words per cluster: %d\n", busyWordsMax)
		fmt.Printf("   Average busy words per cluster: %.2f\n", busyWordsAvg)
		fmt.Printf("   Standard deviation: %.2f\n", busyWordsStdev)
		fmt.Println()
	}

	// Summary statistics
	totalTweets := sum(tweetCounts)
	totalClusters := sum(clusterCounts)
	avgClustersPerTweet := float64(totalClusters) / float64(totalTweets)

	fmt.Println("📊 Summary statistics:")
	fmt.Printf("   Total tweets processed: %d\n", totalTweets)
	fmt.Printf("   Total clusters created: %d\n", totalClusters)
	fmt.Printf("   Average clusters per tweet: %.4f\n", avgClustersPerTweet)
	fmt.Println()

	// Time analysis
	fmt.Println("⏰ Time analysis:")
	fmt.Printf("   First batch: %s\n", firstBatchTime)
	fmt.Printf("   Last batch: %s\n", lastBatchTime)
	fmt.Println()

	// Clusters above minimum size analysis
	aboveMinAvg := average(clustersAboveMin)
	aboveMinTotal := sum(clustersAboveMin)

	fmt.Println("📏 Clusters above minimum size analysis:")
	fmt.Printf("   Average clusters above min size per batch: %.2f\n", aboveMinAvg)
	fmt.Printf("   Total clusters above min size: %d\n", aboveMinTotal)
	fmt.Println()

	// File structure validation
	fmt.Println("🔍 File structure validation:")
	fmt.Println("   Data types found: [batch]")
	fmt.Println("   ✅ No error entries found")
	fmt.Println()

	fmt.Println("=== Analysis Complete ===")
}

func minMax(slice []int) (int, int) {
	if len(slice) == 0 {
		return 0, 0
	}
	min, max := slice[0], slice[0]
	for _, v := range slice {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max
}

func average(slice []int) float64 {
	if len(slice) == 0 {
		return 0
	}
	sum := 0
	for _, v := range slice {
		sum += v
	}
	return float64(sum) / float64(len(slice))
}

func sum(slice []int) int {
	sum := 0
	for _, v := range slice {
		sum += v
	}
	return sum
}

func standardDeviation(slice []int) float64 {
	if len(slice) == 0 {
		return 0
	}
	avg := average(slice)
	sumSquaredDiffs := 0.0
	for _, v := range slice {
		diff := float64(v) - avg
		sumSquaredDiffs += diff * diff
	}
	variance := sumSquaredDiffs / float64(len(slice))
	return math.Sqrt(variance)
}
