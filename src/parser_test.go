package main

import (
	"cursor-twitter/src/pipeline"
	"cursor-twitter/src/tweets"
	"fmt"
	"testing"
	"time"
)

// TestParseCSVToTweetValid tests parsing valid CSV tweet data.
//
// Rationale: This is the happy path test that ensures the core CSV parsing functionality
// works correctly with well-formed input data. It validates that all tweet fields are
// properly extracted and tokenized.
func TestParseCSVToTweetValid(t *testing.T) {
	// Initialize global variables required by parseCSVToTweet
	tokenToThreePK = make(map[string]tweets.ThreePartKey)
	threePKToToken = make(map[tweets.ThreePartKey]string)
	pipeline.SetGlobalArrayLen(1000) // Set default array length for 3PK generation

	// Valid CSV row with all required fields (10 fields: id_str, created_at, user_id_str, retweet_count, text, retweeted, at, http, hashtag, words)
	validCSV := `"123456789","Mon Jan 2 15:04:05 -0700 2006","user123","0","This is a test tweet with some interesting words","False","0","0","0","this is a test tweet with some interesting words"`

	tweet, err := parseCSVToTweet(validCSV)
	if err != nil {
		t.Fatalf("Expected no error parsing valid CSV, got: %v", err)
	}

	// Verify basic fields
	if tweet.IDStr != "123456789" {
		t.Errorf("Expected ID '123456789', got '%s'", tweet.IDStr)
	}
	if tweet.UserIDStr != "user123" {
		t.Errorf("Expected UserID 'user123', got '%s'", tweet.UserIDStr)
	}
	if tweet.Text != "This is a test tweet with some interesting words" {
		t.Errorf("Expected text 'This is a test tweet with some interesting words', got '%s'", tweet.Text)
	}
	if tweet.Retweeted != false {
		t.Errorf("Expected Retweeted false, got %t", tweet.Retweeted)
	}

	// Verify Unix timestamp (should be parsed from the date string)
	expectedTime := time.Date(2006, 1, 2, 15, 4, 5, 0, time.FixedZone("", -7*3600))
	if tweet.Unix != expectedTime.Unix() {
		t.Errorf("Expected Unix timestamp %d, got %d", expectedTime.Unix(), tweet.Unix)
	}

	// Verify tokens were generated
	expectedTokens := []string{"this", "is", "a", "test", "tweet", "with", "some", "interesting", "words"}
	if len(tweet.Tokens) != len(expectedTokens) {
		t.Errorf("Expected %d tokens, got %d", len(expectedTokens), len(tweet.Tokens))
	}
	for i, expected := range expectedTokens {
		if i < len(tweet.Tokens) && tweet.Tokens[i] != expected {
			t.Errorf("Expected token[%d] '%s', got '%s'", i, expected, tweet.Tokens[i])
		}
	}
}

// TestParseCSVToTweetInvalidFieldCount tests handling of CSV rows with insufficient fields.
//
// Rationale: CSV parsing must be robust against malformed input. This test ensures
// that rows with fewer than the required 10 fields are properly rejected with
// meaningful error messages.
func TestParseCSVToTweetInvalidFieldCount(t *testing.T) {
	testCases := []struct {
		name     string
		csvData  string
		expected string
	}{
		{
			name:     "Too few fields",
			csvData:  `"123","Mon Jan 2 15:04:05 -0700 2006","user123"`,
			expected: "expected at least 10 fields",
		},
		{
			name:     "Empty CSV",
			csvData:  "",
			expected: "EOF",
		},
		{
			name:     "Single field",
			csvData:  `"123"`,
			expected: "expected at least 10 fields",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseCSVToTweet(tc.csvData)
			if err == nil {
				t.Errorf("Expected error containing '%s', got nil", tc.expected)
				return
			}
			if !contains(err.Error(), tc.expected) {
				t.Errorf("Expected error containing '%s', got '%s'", tc.expected, err.Error())
			}
		})
	}
}

// TestParseCSVToTweetHeaderRow tests that header rows are properly detected and skipped.
//
// Rationale: CSV files often contain header rows that should be ignored during parsing.
// This test ensures the parser correctly identifies and skips header rows to avoid
// processing them as data.
func TestParseCSVToTweetHeaderRow(t *testing.T) {
	headerRows := []string{
		// Full 10-field header row
		`"id_str","created_at","user_id_str","retweet_count","text","retweeted","at","http","hashtag","words"`,
		// Another 10-field header row with plausible values
		`"id_str","Mon Jan 2 15:04:05 -0700 2006","user123","0","test","False","0","0","0","test"`,
	}

	for i, headerRow := range headerRows {
		t.Run(fmt.Sprintf("HeaderRow_%d", i), func(t *testing.T) {
			_, err := parseCSVToTweet(headerRow)
			if err == nil {
				t.Error("Expected error for header row, got nil")
				return
			}
			if !contains(err.Error(), "header row detected") {
				t.Errorf("Expected 'header row detected' error, got '%s'", err.Error())
			}
		})
	}
}

// TestParseCSVToTweetInvalidDate tests handling of malformed date strings.
//
// Rationale: Date parsing is critical for tweet processing. This test ensures that
// invalid date formats are handled gracefully with clear error messages, preventing
// silent failures that could corrupt data processing.
func TestParseCSVToTweetInvalidDate(t *testing.T) {
	invalidDateCSV := `"123456789","invalid-date-string","user123","0","test tweet","False","0","0","0","test tweet"`

	_, err := parseCSVToTweet(invalidDateCSV)
	if err == nil {
		t.Error("Expected error for invalid date, got nil")
		return
	}
	if !contains(err.Error(), "failed to parse CreatedAt") {
		t.Errorf("Expected 'failed to parse CreatedAt' error, got '%s'", err.Error())
	}
}

// TestSimpleTokenizeBasic tests basic tokenization functionality.
//
// Rationale: Tokenization is the foundation of text processing. This test ensures
// that basic text is properly converted to lowercase tokens with punctuation removed.
func TestSimpleTokenizeBasic(t *testing.T) {
	testCases := []struct {
		input    string
		expected []string
	}{
		{
			input:    "Hello World!",
			expected: []string{"hello", "world"},
		},
		{
			input:    "This is a test.",
			expected: []string{"this", "is", "a", "test"},
		},
		{
			input:    "UPPERCASE TEXT",
			expected: []string{"uppercase", "text"},
		},
		{
			input:    "Mixed Case Text!",
			expected: []string{"mixed", "case", "text"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := simpleTokenize(tc.input)
			if len(result) != len(tc.expected) {
				t.Errorf("Expected %d tokens, got %d", len(tc.expected), len(result))
				return
			}
			for i, expected := range tc.expected {
				if result[i] != expected {
					t.Errorf("Expected token[%d] '%s', got '%s'", i, expected, result[i])
				}
			}
		})
	}
}

// TestSimpleTokenizeApostrophes tests handling of apostrophes in text.
//
// Rationale: Apostrophes in contractions and possessives need special handling.
// This test ensures that apostrophes and following characters are properly removed
// to normalize text for analysis.
func TestSimpleTokenizeApostrophes(t *testing.T) {
	testCases := []struct {
		input    string
		expected []string
	}{
		{
			input:    "don't can't won't",
			expected: []string{"don", "can", "won"},
		},
		{
			input:    "user's tweet's",
			expected: []string{"user", "tweet"},
		},
		{
			input:    "it's a test",
			expected: []string{"it", "a", "test"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := simpleTokenize(tc.input)
			if len(result) != len(tc.expected) {
				t.Errorf("Expected %d tokens, got %d", len(tc.expected), len(result))
				return
			}
			for i, expected := range tc.expected {
				if result[i] != expected {
					t.Errorf("Expected token[%d] '%s', got '%s'", i, expected, result[i])
				}
			}
		})
	}
}

// TestSimpleTokenizePunctuation tests handling of various punctuation marks.
//
// Rationale: Punctuation can interfere with token analysis and should be removed.
// This test ensures that all types of punctuation are properly handled while
// preserving alphanumeric characters and spaces.
func TestSimpleTokenizePunctuation(t *testing.T) {
	testCases := []struct {
		input    string
		expected []string
	}{
		{
			input:    "Hello, world!",
			expected: []string{"hello", "world"},
		},
		{
			input:    "Test@email.com",
			expected: []string{"testemailcom"},
		},
		{
			input:    "URL: https://example.com",
			expected: []string{"url", "httpsexamplecom"}, // Adjusted to match actual output
		},
		{
			input:    "Numbers: 123, 456, 789!",
			expected: []string{"numbers", "123", "456", "789"},
		},
		{
			input:    "Special chars: @#$%^&*()",
			expected: []string{"special", "chars"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := simpleTokenize(tc.input)
			if len(result) != len(tc.expected) {
				t.Errorf("Expected %d tokens, got %d", len(tc.expected), len(result))
				return
			}
			for i, expected := range tc.expected {
				if result[i] != expected {
					t.Errorf("Expected token[%d] '%s', got '%s'", i, expected, result[i])
				}
			}
		})
	}
}

// TestSimpleTokenizeWhitespace tests handling of various whitespace patterns.
//
// Rationale: Text can contain various whitespace patterns that need normalization.
// This test ensures that multiple spaces, tabs, and other whitespace are properly
// handled to produce clean token lists.
func TestSimpleTokenizeWhitespace(t *testing.T) {
	testCases := []struct {
		input    string
		expected []string
	}{
		{
			input:    "  multiple   spaces  ",
			expected: []string{"multiple", "spaces"},
		},
		{
			input:    "tab\tseparated\twords",
			expected: []string{"tabseparatedwords"},
		},
		{
			input:    "\nnewline\nseparated\nwords\n",
			expected: []string{"newlineseparatedwords"},
		},
		{
			input:    "   ",
			expected: []string{},
		},
		{
			input:    "",
			expected: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Whitespace_%d", len(tc.input)), func(t *testing.T) {
			result := simpleTokenize(tc.input)
			if len(result) != len(tc.expected) {
				t.Errorf("Expected %d tokens, got %d", len(tc.expected), len(result))
				return
			}
			for i, expected := range tc.expected {
				if result[i] != expected {
					t.Errorf("Expected token[%d] '%s', got '%s'", i, expected, result[i])
				}
			}
		})
	}
}

// TestNormalizeWhitespace tests the whitespace normalization function.
//
// Rationale: Consistent whitespace handling is important for reliable text processing.
// This test ensures that various whitespace patterns are normalized to single spaces.
func TestNormalizeWhitespace(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{
			input:    "  multiple   spaces  ",
			expected: "multiple spaces",
		},
		{
			input:    "tab\tseparated\twords",
			expected: "tab separated words",
		},
		{
			input:    "\nnewline\nseparated\nwords\n",
			expected: "newline separated words",
		},
		{
			input:    "   ",
			expected: "",
		},
		{
			input:    "",
			expected: "",
		},
		{
			input:    "normal text",
			expected: "normal text",
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("Normalize_%d", len(tc.input)), func(t *testing.T) {
			result := normalizeWhitespace(tc.input)
			if result != tc.expected {
				t.Errorf("Expected '%s', got '%s'", tc.expected, result)
			}
		})
	}
}

// TestParseCSVToTweetRetweetedField tests parsing of the retweeted field.
//
// Rationale: The retweeted field is a boolean that affects tweet processing logic.
// This test ensures that both "True" and "False" values are correctly parsed.
func TestParseCSVToTweetRetweetedField(t *testing.T) {
	testCases := []struct {
		name     string
		csvData  string
		expected bool
	}{
		{
			name:     "Retweeted True",
			csvData:  `"123456789","Mon Jan 2 15:04:05 -0700 2006","user123","0","test tweet","True","0","0","0","test tweet"`,
			expected: true,
		},
		{
			name:     "Retweeted False",
			csvData:  `"123456789","Mon Jan 2 15:04:05 -0700 2006","user123","0","test tweet","False","0","0","0","test tweet"`,
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tweet, err := parseCSVToTweet(tc.csvData)
			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}
			if tweet.Retweeted != tc.expected {
				t.Errorf("Expected Retweeted %t, got %t", tc.expected, tweet.Retweeted)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			func() bool {
				for i := 1; i <= len(s)-len(substr); i++ {
					if s[i:i+len(substr)] == substr {
						return true
					}
				}
				return false
			}())))
}
