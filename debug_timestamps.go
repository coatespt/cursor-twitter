package main

import (
	"fmt"
	"time"
)

func main() {
	// Test the timestamp parsing that's used in the main program
	testCases := []string{
		"Mon Feb 11 19:59:49 -0800 2012", // PST (UTC-8)
		"Mon Feb 11 23:55:00 -0800 2012", // Whitney's death time in PST
		"Sun Feb 12 01:01:23 -0800 2012", // The medoid tweet time
	}

	fmt.Println("Testing timestamp parsing:")
	fmt.Println("Format: Mon Jan 2 15:04:05 -0700 2006")
	fmt.Println()

	for i, testCase := range testCases {
		fmt.Printf("Test case %d: %s\n", i+1, testCase)
		
		// Parse using the same format as in main.go
		parsed, err := time.Parse("Mon Jan 2 15:04:05 -0700 2006", testCase)
		if err != nil {
			fmt.Printf("  Error: %v\n", err)
			continue
		}
		
		fmt.Printf("  Parsed time: %s\n", parsed)
		fmt.Printf("  Unix timestamp: %d\n", parsed.Unix())
		fmt.Printf("  UTC formatted: %s\n", parsed.Format("2006-01-02 15:04:05 UTC"))
		fmt.Printf("  Local timezone: %s\n", parsed.Location())
		fmt.Println()
	}

	// Test Whitney's actual death time
	fmt.Println("Whitney Houston death timeline:")
	fmt.Println("  Actual death: ~3:55 PM PST on Feb 11, 2012")
	fmt.Println("  PST to UTC: 3:55 PM PST = 11:55 PM UTC (PST is UTC-8)")
	fmt.Println("  So tweets about her death should be around 23:55 UTC on Feb 11")
	fmt.Println()
	
	// Test the problematic timestamps from the output
	fmt.Println("Problematic timestamps from output:")
	fmt.Println("  first_tweet_time: 2012-02-11 19:59:49 UTC")
	fmt.Println("  medoid_tweet_text: [2012-02-12 01:01:23 UTC]")
	fmt.Println()
	fmt.Println("Analysis:")
	fmt.Println("  - First tweet at 19:59:49 UTC = 11:59:49 AM PST")
	fmt.Println("  - Medoid tweet at 01:01:23 UTC = 5:01:23 PM PST (previous day)")
	fmt.Println("  - Time difference: 5 hours 1 minute 34 seconds")
	fmt.Println("  - This suggests tweets from different time periods are being clustered together")
} 