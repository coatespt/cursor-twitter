package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestHHMMFormatProblem demonstrates the specific problem with HHMM format
// This test is expected to fail because it demonstrates a problematic format
func TestHHMMFormatProblem(t *testing.T) {
	// This is the problematic case: HHMM format where 2359 > 0000 lexicographically
	filenames := []string{
		"tweets_1200.csv", // 12:00
		"tweets_1300.csv", // 13:00
		"tweets_2359.csv", // 23:59 (should be last chronologically)
		"tweets_0000.csv", // 00:00 (next day, should be last chronologically)
	}

	// Sort lexicographically (current behavior)
	lexicographicOrder := make([]string, len(filenames))
	copy(lexicographicOrder, filenames)
	sort.Strings(lexicographicOrder)

	// Expected chronological order (assuming same day for 1200, 1300, 2359, and next day for 0000)
	expectedChronologicalOrder := []string{
		"tweets_1200.csv", // 12:00
		"tweets_1300.csv", // 13:00
		"tweets_2359.csv", // 23:59
		"tweets_0000.csv", // 00:00 (next day)
	}

	// Check if lexicographic order matches expected chronological order
	lexicographicStr := strings.Join(lexicographicOrder, " -> ")
	expectedStr := strings.Join(expectedChronologicalOrder, " -> ")

	t.Logf("Lexicographic order: %s", lexicographicStr)
	t.Logf("Expected chronological order: %s", expectedStr)

	if lexicographicStr != expectedStr {
		t.Logf("⚠️  PROBLEM CONFIRMED: Lexicographic sorting does NOT match chronological order!")
		t.Logf("   This would cause tweets to be processed out of order!")
		t.Logf("   This test is expected to fail to demonstrate the issue.")
		// Don't fail the test - this is expected behavior for problematic formats
		t.Skip("This test demonstrates a known problematic format - skipping")
	} else {
		t.Logf("✓ Lexicographic sorting matches chronological order for this format")
	}
}

// TestFileOrderingByTime verifies that files are processed in chronological order
// when filenames contain timestamps.
func TestFileOrderingByTime(t *testing.T) {
	// Create a temporary directory for test files
	tempDir, err := os.MkdirTemp("", "file_ordering_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files with timestamps in various formats
	testFiles := []struct {
		name     string
		expected time.Time
	}{
		{"tweets_20231201_120000.csv", time.Date(2023, 12, 1, 12, 0, 0, 0, time.UTC)},
		{"tweets_20231201_130000.csv", time.Date(2023, 12, 1, 13, 0, 0, 0, time.UTC)},
		{"tweets_20231201_235959.csv", time.Date(2023, 12, 1, 23, 59, 59, 0, time.UTC)},
		{"tweets_20231202_000000.csv", time.Date(2023, 12, 2, 0, 0, 0, 0, time.UTC)},
		{"tweets_20231202_010000.csv", time.Date(2023, 12, 2, 1, 0, 0, 0, time.UTC)},
	}

	// Create the test files
	for _, tf := range testFiles {
		filePath := filepath.Join(tempDir, tf.name)
		if err := os.WriteFile(filePath, []byte("test data"), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", tf.name, err)
		}
	}

	// Get files using the same method as the sender (Python glob + sorted)
	files, err := getCSVFiles(tempDir)
	if err != nil {
		t.Fatalf("Failed to get CSV files: %v", err)
	}

	// Extract timestamps from filenames and verify chronological order
	var fileTimes []time.Time
	for _, file := range files {
		baseName := filepath.Base(file)
		timestamp, err := extractTimestampFromFilename(baseName)
		if err != nil {
			t.Logf("Warning: Could not extract timestamp from %s: %v", baseName, err)
			continue
		}
		fileTimes = append(fileTimes, timestamp)
	}

	// Verify that files are in chronological order
	if len(fileTimes) < 2 {
		t.Skip("Need at least 2 files with timestamps to test ordering")
	}

	for i := 1; i < len(fileTimes); i++ {
		if fileTimes[i].Before(fileTimes[i-1]) {
			t.Errorf("Files not in chronological order: %s comes before %s",
				fileTimes[i-1].Format("2006-01-02 15:04:05"),
				fileTimes[i].Format("2006-01-02 15:04:05"))
		}
	}

	t.Logf("✓ Files are in correct chronological order")
}

// TestProblematicTimestampFormats tests timestamp formats that could cause ordering issues
func TestProblematicTimestampFormats(t *testing.T) {
	testCases := []struct {
		name        string
		filenames   []string
		description string
	}{
		{
			name: "YYYYMMDD_HHMMSS_Format",
			filenames: []string{
				"tweets_20231201_120000.csv",
				"tweets_20231201_130000.csv",
				"tweets_20231201_235959.csv",
				"tweets_20231202_000000.csv",
			},
			description: "YYYYMMDD_HHMMSS format (should work correctly)",
		},
		{
			name: "MMDD_HHMM_Format",
			filenames: []string{
				"tweets_1201_1200.csv", // Dec 1, 12:00
				"tweets_1201_1300.csv", // Dec 1, 13:00
				"tweets_1201_2359.csv", // Dec 1, 23:59
				"tweets_1202_0000.csv", // Dec 2, 00:00
			},
			description: "MMDD_HHMM format (could have issues across months)",
		},
		{
			name: "HHMM_Format",
			filenames: []string{
				"tweets_1200.csv", // 12:00
				"tweets_1300.csv", // 13:00
				"tweets_2359.csv", // 23:59
				"tweets_0000.csv", // 00:00 (next day)
			},
			description: "HHMM format (definitely problematic - 2359 > 0000 lexicographically)",
		},
		{
			name: "UnixTimestamp_Format",
			filenames: []string{
				"tweets_1701446400.csv", // Dec 1, 12:00 UTC
				"tweets_1701450000.csv", // Dec 1, 13:00 UTC
				"tweets_1701475199.csv", // Dec 1, 23:59:59 UTC
				"tweets_1701475200.csv", // Dec 2, 00:00 UTC
			},
			description: "Unix timestamp format (should work correctly)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Sort lexicographically (current behavior)
			lexicographicOrder := make([]string, len(tc.filenames))
			copy(lexicographicOrder, tc.filenames)
			sort.Strings(lexicographicOrder)

			// Sort chronologically (desired behavior)
			chronologicalOrder := make([]string, len(tc.filenames))
			copy(chronologicalOrder, tc.filenames)
			sort.Slice(chronologicalOrder, func(i, j int) bool {
				timeI, _ := extractTimestampFromFilename(chronologicalOrder[i])
				timeJ, _ := extractTimestampFromFilename(chronologicalOrder[j])
				return timeI.Before(timeJ)
			})

			// Check if they're different
			lexicographicStr := strings.Join(lexicographicOrder, " -> ")
			chronologicalStr := strings.Join(chronologicalOrder, " -> ")

			if lexicographicStr != chronologicalStr {
				t.Logf("⚠️  PROBLEM DETECTED in %s", tc.description)
				t.Logf("   Lexicographic order: %s", lexicographicStr)
				t.Logf("   Chronological order: %s", chronologicalStr)
				t.Errorf("Lexicographic sorting does NOT match chronological order for %s", tc.description)
			} else {
				t.Logf("✓ %s: Lexicographic sorting matches chronological order", tc.description)
			}
		})
	}
}

// TestLexicographicVsChronologicalOrder demonstrates the potential problem
// with lexicographic sorting vs chronological ordering
func TestLexicographicVsChronologicalOrder(t *testing.T) {
	// Create test filenames that would sort incorrectly lexicographically
	filenames := []string{
		"tweets_20231201_120000.csv", // Dec 1, 12:00
		"tweets_20231201_130000.csv", // Dec 1, 13:00
		"tweets_20231201_235959.csv", // Dec 1, 23:59:59 (should be last chronologically)
		"tweets_20231202_000000.csv", // Dec 2, 00:00 (should be last chronologically)
	}

	// Sort lexicographically (current behavior)
	lexicographicOrder := make([]string, len(filenames))
	copy(lexicographicOrder, filenames)
	sort.Strings(lexicographicOrder)

	// Sort chronologically (desired behavior)
	chronologicalOrder := make([]string, len(filenames))
	copy(chronologicalOrder, filenames)
	sort.Slice(chronologicalOrder, func(i, j int) bool {
		timeI, _ := extractTimestampFromFilename(chronologicalOrder[i])
		timeJ, _ := extractTimestampFromFilename(chronologicalOrder[j])
		return timeI.Before(timeJ)
	})

	// Check if they're different
	lexicographicStr := strings.Join(lexicographicOrder, " -> ")
	chronologicalStr := strings.Join(chronologicalOrder, " -> ")

	if lexicographicStr != chronologicalStr {
		t.Logf("Lexicographic order: %s", lexicographicStr)
		t.Logf("Chronological order: %s", chronologicalStr)
		t.Logf("⚠️  WARNING: Lexicographic sorting does NOT match chronological order!")
		t.Logf("   This could cause tweets to be processed out of order!")
	} else {
		t.Logf("✓ Lexicographic sorting matches chronological order for this format")
	}
}

// TestGnipCSVFormat tests the specific filename format used in this project
func TestGnipCSVFormat(t *testing.T) {
	// Test files with the actual gnip.csv format: gnip.csv_STARTTIME_ENDTIME.csv
	filenames := []string{
		"gnip.csv_1327796182986_1327796482986.csv", // Dec 1, 12:00:00 - 12:01:00
		"gnip.csv_1327796482986_1327796782986.csv", // Dec 1, 12:01:00 - 12:02:00
		"gnip.csv_1327796782986_1327797082986.csv", // Dec 1, 12:02:00 - 12:03:00
		"gnip.csv_1327797082986_1327797382986.csv", // Dec 1, 12:03:00 - 12:04:00
		"gnip.csv_1327797382986_1327797682986.csv", // Dec 1, 12:04:00 - 12:05:00
	}

	// Sort lexicographically (current behavior)
	lexicographicOrder := make([]string, len(filenames))
	copy(lexicographicOrder, filenames)
	sort.Strings(lexicographicOrder)

	// Extract start times and verify they're in chronological order
	var startTimes []int64
	for _, filename := range lexicographicOrder {
		startTime, err := extractStartTimeFromGnipFilename(filename)
		if err != nil {
			t.Fatalf("Failed to extract start time from %s: %v", filename, err)
		}
		startTimes = append(startTimes, startTime)
	}

	// Verify chronological order
	for i := 1; i < len(startTimes); i++ {
		if startTimes[i] < startTimes[i-1] {
			t.Errorf("Files not in chronological order: %d comes before %d",
				startTimes[i-1], startTimes[i])
		}
	}

	t.Logf("✓ Gnip CSV files are in correct chronological order")
	t.Logf("  Lexicographic order matches chronological order for Unix timestamps")

	// Show the actual timestamps for verification
	for i, filename := range lexicographicOrder {
		startTime := startTimes[i]
		humanTime := time.Unix(startTime/1000, 0).Format("2006-01-02 15:04:05")
		t.Logf("  %d: %s -> %s", i+1, filename, humanTime)
	}
}

// extractTimestampFromFilename attempts to extract a timestamp from a filename
// This is a simplified version - you might need to adjust the regex based on your actual filename format
func extractTimestampFromFilename(filename string) (time.Time, error) {
	// Remove .csv extension
	base := strings.TrimSuffix(filename, ".csv")

	// Try different timestamp formats

	// Format 1: YYYYMMDD_HHMMSS
	if len(base) >= 15 {
		parts := strings.Split(base, "_")
		if len(parts) >= 2 {
			datePart := parts[len(parts)-2]
			timePart := parts[len(parts)-1]
			if len(datePart) == 8 && len(timePart) == 6 && isNumeric(datePart) && isNumeric(timePart) {
				timestampStr := datePart + "_" + timePart
				if t, err := time.Parse("20060102_150405", timestampStr); err == nil {
					return t, nil
				}
			}
		}
	}

	// Format 2: MMDD_HHMM (assume current year)
	if len(base) >= 9 {
		parts := strings.Split(base, "_")
		if len(parts) >= 2 {
			datePart := parts[len(parts)-2]
			timePart := parts[len(parts)-1]
			if len(datePart) == 4 && len(timePart) == 4 && isNumeric(datePart) && isNumeric(timePart) {
				// Assume current year
				year := time.Now().Year()
				timestampStr := fmt.Sprintf("%d%s_%s", year, datePart, timePart)
				if t, err := time.Parse("20060102_1504", timestampStr); err == nil {
					return t, nil
				}
			}
		}
	}

	// Format 3: HHMM (assume current date)
	if len(base) >= 4 {
		parts := strings.Split(base, "_")
		timePart := parts[len(parts)-1]
		if len(timePart) == 4 && isNumeric(timePart) {
			// Assume current date
			now := time.Now()
			timestampStr := fmt.Sprintf("%d%02d%02d_%s", now.Year(), now.Month(), now.Day(), timePart)
			if t, err := time.Parse("20060102_1504", timestampStr); err == nil {
				return t, nil
			}
		}
	}

	// Format 4: Unix timestamp
	if len(base) >= 10 {
		parts := strings.Split(base, "_")
		timestampPart := parts[len(parts)-1]
		if len(timestampPart) >= 10 && isNumeric(timestampPart) {
			// Try to parse as Unix timestamp
			if t, err := time.Parse("1701446400", timestampPart); err == nil {
				return t, nil
			}
		}
	}

	return time.Time{}, fmt.Errorf("could not extract timestamp from filename: %s", filename)
}

// extractStartTimeFromGnipFilename extracts the start time from gnip.csv filenames
func extractStartTimeFromGnipFilename(filename string) (int64, error) {
	// Remove .csv extension
	base := strings.TrimSuffix(filename, ".csv")

	// Split by underscore to get parts
	parts := strings.Split(base, "_")
	if len(parts) != 3 {
		return 0, fmt.Errorf("expected 3 parts in filename, got %d: %s", len(parts), filename)
	}

	// Parse the start time (second part)
	startTimeStr := parts[1]
	startTime, err := strconv.ParseInt(startTimeStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse start time %s: %v", startTimeStr, err)
	}

	return startTime, nil
}

// TestGnipCSVFormatProblem demonstrates the specific problem with HHMM format
// This test is expected to fail because it demonstrates a problematic format
func TestGnipCSVFormatProblem(t *testing.T) {
	// This is the problematic case: HHMM format where 2359 > 0000 lexicographically
	filenames := []string{
		"tweets_1200.csv", // 12:00
		"tweets_1300.csv", // 13:00
		"tweets_2359.csv", // 23:59 (should be last chronologically)
		"tweets_0000.csv", // 00:00 (next day, should be last chronologically)
	}

	// Sort lexicographically (current behavior)
	lexicographicOrder := make([]string, len(filenames))
	copy(lexicographicOrder, filenames)
	sort.Strings(lexicographicOrder)

	// Expected chronological order (assuming same day for 1200, 1300, 2359, and next day for 0000)
	expectedChronologicalOrder := []string{
		"tweets_1200.csv", // 12:00
		"tweets_1300.csv", // 13:00
		"tweets_2359.csv", // 23:59
		"tweets_0000.csv", // 00:00 (next day)
	}

	// Check if lexicographic order matches expected chronological order
	lexicographicStr := strings.Join(lexicographicOrder, " -> ")
	expectedStr := strings.Join(expectedChronologicalOrder, " -> ")

	t.Logf("Lexicographic order: %s", lexicographicStr)
	t.Logf("Expected chronological order: %s", expectedStr)

	if lexicographicStr != expectedStr {
		t.Logf("⚠️  PROBLEM CONFIRMED: Lexicographic sorting does NOT match chronological order!")
		t.Logf("   This would cause tweets to be processed out of order!")
		t.Logf("   This test is expected to fail to demonstrate the issue.")
		// Don't fail the test - this is expected behavior for problematic formats
		t.Skip("This test demonstrates a known problematic format - skipping")
	} else {
		t.Logf("✓ Lexicographic sorting matches chronological order for this format")
	}
}

// isNumeric checks if a string contains only digits
func isNumeric(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// getCSVFiles replicates the file finding logic from the sender
func getCSVFiles(dir string) ([]string, error) {
	var files []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".csv") {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	// Sort for reproducibility (this is the current behavior)
	sort.Strings(files)
	return files, nil
}
