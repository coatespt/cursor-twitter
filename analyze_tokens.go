package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Simple tokenizer: splits on non-word characters, lowercases, removes empty tokens
func simpleTokenize(text string) []string {
	// Remove punctuation, split on whitespace
	re := regexp.MustCompile(`\W+`)
	tokens := re.Split(strings.ToLower(text), -1)
	var out []string
	for _, t := range tokens {
		t = strings.TrimSpace(t)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// Reads all CSV files in a directory (non-recursive)
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
	// Sort for reproducibility
	sort.Strings(files)
	return files, nil
}

func main() {
	inputDir := flag.String("input", "data", "Directory containing tweet CSV files")
	interval := flag.Int("interval", 10000, "Interval for reporting stats (default 10000)")
	filterTokens := flag.Bool("filter-tokens", true, "Filter out URLs, mentions, hashtags, and short tokens (default true)")
	flag.Parse()

	files, err := getCSVFiles(*inputDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input directory: %v\n", err)
		os.Exit(1)
	}
	if len(files) == 0 {
		fmt.Fprintf(os.Stderr, "No CSV files found in directory: %s\n", *inputDir)
		os.Exit(1)
	}

	tokenSet := make(map[string]struct{})
	tweetsRead := 0
	startTime := time.Now()
	fmt.Printf("TweetsRead\tDistinctTokens\n")

	// For graphing
	var xVals []int
	var yVals []int

	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening file %s: %v\n", file, err)
			continue
		}
		reader := csv.NewReader(bufio.NewReader(f))
		reader.FieldsPerRecord = -1 // allow variable columns
		reader.LazyQuotes = true    // handle bare quotes in tweet text
		firstRow := true
		textCol := -1
		for {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading CSV: %v\n", err)
				break
			}
			if firstRow {
				// Find the text column (case-insensitive match for "text")
				for i, col := range record {
					if strings.ToLower(col) == "text" {
						textCol = i
						break
					}
				}
				firstRow = false
				continue
			}
			if textCol == -1 || textCol >= len(record) {
				continue // skip if no text column
			}
			text := record[textCol]
			tokens := simpleTokenize(text)
			for _, tok := range tokens {
				if *filterTokens {
					if len(tok) < 2 {
						continue
					}
					if strings.HasPrefix(tok, "http") {
						continue
					}
					if strings.HasPrefix(tok, "@") {
						continue
					}
					if strings.HasPrefix(tok, "#") {
						continue
					}
				}
				tokenSet[tok] = struct{}{}
			}
			tweetsRead++
			if tweetsRead%(*interval) == 0 {
				elapsed := time.Since(startTime).Seconds()
				tps := float64(tweetsRead) / elapsed
				progress := fmt.Sprintf("Processed %d tweets (distinct: %d) [%.0f tweets/sec]", tweetsRead, len(tokenSet), tps)
				fmt.Printf("\r%-80s", progress)
				xVals = append(xVals, tweetsRead)
				yVals = append(yVals, len(tokenSet))
			}
		}
		f.Close()
	}
	// Final output
	fmt.Printf("\rProcessed %d tweets (distinct: %d)\n", tweetsRead, len(tokenSet))
	xVals = append(xVals, tweetsRead)
	yVals = append(yVals, len(tokenSet))

	// ASCII Art Graph
	printASCIIGraph(xVals, yVals, 60, 20)
}

// printASCIIGraph prints a simple ASCII line graph for the data
func printASCIIGraph(xVals, yVals []int, width, height int) {
	if len(xVals) < 2 {
		fmt.Println("Not enough data for graph.")
		return
	}
	// Find min/max
	xMin, xMax := xVals[0], xVals[len(xVals)-1]
	yMin, yMax := yVals[0], yVals[0]
	for _, y := range yVals {
		if y < yMin {
			yMin = y
		}
		if y > yMax {
			yMax = y
		}
	}
	if yMax == yMin {
		yMax = yMin + 1 // avoid div by zero
	}
	// Prepare grid
	grid := make([][]rune, height)
	for i := range grid {
		grid[i] = make([]rune, width)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}
	// Map data points to grid
	for i := 0; i < len(xVals); i++ {
		col := int(float64(i) / float64(len(xVals)-1) * float64(width-1))
		row := int(float64(yVals[i]-yMin) / float64(yMax-yMin) * float64(height-1))
		row = height - 1 - row // invert Y axis
		if col >= 0 && col < width && row >= 0 && row < height {
			grid[row][col] = '*'
		}
	}
	// Prepare Y axis labels
	yLabels := make([]string, height)
	yLabels[0] = fmt.Sprintf("%d", yMax)
	yLabels[height/2] = fmt.Sprintf("%d", (yMax+yMin)/2)
	yLabels[height-1] = fmt.Sprintf("%d", yMin)
	for i := 1; i < height-1; i++ {
		if yLabels[i] == "" {
			yLabels[i] = " "
		}
	}
	// Print graph with Y axis labels
	fmt.Println("\nASCII Graph: Distinct Tokens vs. Tweets Read")
	for i := 0; i < height; i++ {
		fmt.Printf("%8s |", yLabels[i])
		for j := 0; j < width; j++ {
			fmt.Print(string(grid[i][j]))
		}
		fmt.Println()
	}
	// X axis
	fmt.Printf("%8s +%s\n", "", strings.Repeat("-", width))
	// X axis labels: min, mid, max
	xMid := (xMin + xMax) / 2
	label := fmt.Sprintf("%d", xMin)
	labelMid := fmt.Sprintf("%d", xMid)
	labelMax := fmt.Sprintf("%d", xMax)
	// Place min at start, mid at center, max at end
	labelLine := make([]rune, width)
	for i := range labelLine {
		labelLine[i] = ' '
	}
	copy(labelLine[0:], []rune(label))
	copy(labelLine[width/2-len(labelMid)/2:], []rune(labelMid))
	copy(labelLine[width-len(labelMax):], []rune(labelMax))
	fmt.Printf("         %s\n", string(labelLine))
	fmt.Printf("Y: %d to %d\n", yMin, yMax)
}
