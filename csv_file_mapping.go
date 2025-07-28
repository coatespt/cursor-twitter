package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CSVFile represents a CSV file with its timestamp information
type CSVFile struct {
	Filename  string
	StartTime int64
	EndTime   int64
	StartDate time.Time
	EndDate   time.Time
}

func main() {
	dir := flag.String("dir", "", "Directory containing CSV files")
	flag.Parse()

	if *dir == "" {
		fmt.Println("Usage: ./csv_file_mapping -dir <directory>")
		fmt.Println("Example: ./csv_file_mapping -dir /path/to/csv > mapping.csv")
		os.Exit(1)
	}

	// Get all CSV files in the directory
	files, err := getCSVFiles(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading directory '%s': %v\n", *dir, err)
		os.Exit(1)
	}

	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "No CSV files found in directory: %s\n", *dir)
		os.Exit(1)
	}

	// Sort files by start time
	sort.Slice(files, func(i, j int) bool {
		return files[i].StartTime < files[j].StartTime
	})

	// Output header
	fmt.Println("start_datetime,end_datetime,filename")

	// Output each file's mapping
	for _, file := range files {
		startStr := file.StartDate.Format("2006-01-02 15:04:05")
		endStr := file.EndDate.Format("2006-01-02 15:04:05")
		fmt.Printf("%s,%s,%s\n", startStr, endStr, file.Filename)
	}
}

// getCSVFiles reads all CSV files in the directory and parses their timestamps
func getCSVFiles(dir string) ([]CSVFile, error) {
	var files []CSVFile

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".csv") {
			startTime, endTime, err := parseFilenameTimestamps(entry.Name())
			if err != nil {
				// Skip files that don't match the expected pattern
				continue
			}

			file := CSVFile{
				Filename:  entry.Name(),
				StartTime: startTime,
				EndTime:   endTime,
				StartDate: time.Unix(startTime/1000, 0), // Convert milliseconds to seconds
				EndDate:   time.Unix(endTime/1000, 0),
			}
			files = append(files, file)
		}
	}

	return files, nil
}

// parseFilenameTimestamps extracts start and end timestamps from filename
// Expected format: gnip.csv_1327784181418_1327784481418.csv
func parseFilenameTimestamps(filename string) (int64, int64, error) {
	// Remove .csv extension
	name := strings.TrimSuffix(filename, ".csv")

	// Split by underscores
	parts := strings.Split(name, "_")
	if len(parts) < 3 {
		return 0, 0, fmt.Errorf("filename doesn't match expected pattern")
	}

	// Get the last two parts as timestamps
	startStr := parts[len(parts)-2]
	endStr := parts[len(parts)-1]

	startTime, err := strconv.ParseInt(startStr, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid start timestamp: %s", startStr)
	}

	endTime, err := strconv.ParseInt(endStr, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid end timestamp: %s", endStr)
	}

	return startTime, endTime, nil
}
