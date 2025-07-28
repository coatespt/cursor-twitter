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
	datetime := flag.String("datetime", "", "Target datetime (format: 2012-02-14 19:35:55)")
	n := flag.Int("n", 1, "Number of files to go back (default: 1)")
	flag.Parse()

	if *dir == "" || *datetime == "" {
		fmt.Println("Usage: ./find_csv_file -dir <directory> -datetime \"2012-02-14 19:35:55\" -n <number>")
		fmt.Println("Example: ./find_csv_file -dir /path/to/csv -datetime \"2012-02-14 19:35:55\" -n 3")
		os.Exit(1)
	}

	// Parse the target datetime
	targetTime, err := time.Parse("2006-01-02 15:04:05", *datetime)
	if err != nil {
		fmt.Printf("Error parsing datetime '%s': %v\n", *datetime, err)
		fmt.Println("Expected format: 2012-02-14 19:35:55")
		os.Exit(1)
	}

	// Get all CSV files in the directory
	csvFiles, err := getCSVFiles(*dir)
	if err != nil {
		fmt.Printf("Error reading directory '%s': %v\n", *dir, err)
		os.Exit(1)
	}

	if len(csvFiles) == 0 {
		fmt.Printf("No CSV files found in directory: %s\n", *dir)
		os.Exit(1)
	}

	// Find the file that brackets the target datetime
	bracketingFile, found := findBracketingFile(csvFiles, targetTime)
	if !found {
		fmt.Printf("No file brackets the datetime %s\n", *datetime)
		os.Exit(1)
	}

	// Find the Nth file before the bracketing file
	resultFile, found := findNthFileBefore(csvFiles, bracketingFile, *n)
	if !found {
		fmt.Printf("No file found %d positions before the bracketing file\n", *n)
		os.Exit(1)
	}

	fmt.Println(resultFile.Filename)
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

	// Sort files by start time
	sort.Slice(files, func(i, j int) bool {
		return files[i].StartTime < files[j].StartTime
	})

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

// findBracketingFile finds the file that contains the target datetime
func findBracketingFile(files []CSVFile, targetTime time.Time) (CSVFile, bool) {
	targetUnix := targetTime.Unix() * 1000 // Convert to milliseconds

	for _, file := range files {
		if targetUnix >= file.StartTime && targetUnix <= file.EndTime {
			return file, true
		}
	}

	return CSVFile{}, false
}

// findNthFileBefore finds the Nth file before the given file
func findNthFileBefore(files []CSVFile, targetFile CSVFile, n int) (CSVFile, bool) {
	// Find the index of the target file
	targetIndex := -1
	for i, file := range files {
		if file.Filename == targetFile.Filename {
			targetIndex = i
			break
		}
	}

	if targetIndex == -1 {
		return CSVFile{}, false
	}

	// Calculate the index of the file we want (N positions back)
	desiredIndex := targetIndex - n

	// If we can't go back N positions, return the earliest file
	if desiredIndex < 0 {
		desiredIndex = 0
	}

	return files[desiredIndex], true
}
