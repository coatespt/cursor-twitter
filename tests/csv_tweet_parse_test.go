package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
)

type Tweet struct {
	IDStr        string
	Unix         int64
	UserIDStr    string
	Text         string
	Tokens       []string
	Retweeted    bool
	RetweetCount int
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: go run csv_tweet_parse_test.go <csv_file>")
		os.Exit(1)
	}
	csvFile := os.Args[1]
	f, err := os.Open(csvFile)
	if err != nil {
		fmt.Printf("Failed to open CSV file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.FieldsPerRecord = -1 // allow variable number of fields

	totalRows := 0
	successful := 0
	errors := 0

	// Read header
	head, err := reader.Read()
	if err != nil {
		fmt.Printf("Failed to read header: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Header: %v\n", head)

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("Error reading row: %v\n", err)
			errors++
			continue
		}
		totalRows++
		if len(row) < 10 {
			fmt.Printf("Row %d: too few fields (%d)\n", totalRows, len(row))
			errors++
			continue
		}
		tweet := Tweet{}
		tweet.IDStr = row[0]
		// No unix field in CSV, set to 0
		tweet.UserIDStr = row[2]
		tweet.Text = row[4]
		// Tokens field not present in CSV, leave empty
		// Retweeted
		retweeted, err := strconv.ParseBool(row[5])
		if err != nil {
			fmt.Printf("Row %d: retweeted parse error: %v\n", totalRows, err)
			errors++
			continue
		}
		tweet.Retweeted = retweeted
		// RetweetCount
		retweetCount, err := strconv.Atoi(row[3])
		if err != nil {
			fmt.Printf("Row %d: retweet_count parse error: %v\n", totalRows, err)
			errors++
			continue
		}
		tweet.RetweetCount = retweetCount
		successful++
	}

	fmt.Printf("\nSummary:\n")
	fmt.Printf("  Total rows read: %d\n", totalRows)
	fmt.Printf("  Successfully parsed tweets: %d\n", successful)
	fmt.Printf("  Errors: %d\n", errors)
}
