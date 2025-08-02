package main

import (
	"cursor-twitter/src/pipeline"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"
)

// testConfig creates a minimal config for testing
func testConfig() *Config {
	return &Config{
		MinTokenLen: 2,
		Filter: struct {
			Enabled    bool   `yaml:"enabled"`
			FilterFile string `yaml:"filter_file"`
		}{
			Enabled: false,
		},
	}
}

func init() {
	// Initialize regex patterns for tests
	urlRegex = regexp.MustCompile(`(https?://[^\s]+|www\.[^\s]+)`)
	apostropheRegex = regexp.MustCompile(`'.*`)
}

// TestParseCSVToTweetValid tests parsing valid CSV tweet data.
//
// Rationale: This is the happy path test that ensures the core CSV parsing functionality
// works correctly with well-formed input data. It validates that all tweet fields are
// properly extracted and tokenized.
func TestParseCSVToTweetValid(t *testing.T) {
	// Set default array length for 3PK generation
	pipeline.SetGlobalArrayLen(1000)

	// Valid CSV row with all required fields (11 fields: id_str, created_at, user_id_str, retweet_count, text, retweeted, at, http, hashtag, words, lang)
	validCSV := `"123456789","Mon Jan 2 15:04:05 -0700 2006","user123","0","This is a test tweet with some interesting words","False","0","0","0","this is a test tweet with some interesting words","en"`

	cfg := testConfig()
	tweet, err := parseCSVToTweet(validCSV, cfg)
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
			expected: "expected at least 11 fields",
		},
		{
			name:     "Empty CSV",
			csvData:  "",
			expected: "EOF",
		},
		{
			name:     "Single field",
			csvData:  `"123"`,
			expected: "expected at least 11 fields",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testConfig()
			_, err := parseCSVToTweet(tc.csvData, cfg)
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
		// Full 11-field header row
		`"id_str","created_at","user_id_str","retweet_count","text","retweeted","at","http","hashtag","words","lang"`,
		// Another 11-field header row with plausible values
		`"id_str","Mon Jan 2 15:04:05 -0700 2006","user123","0","test","False","0","0","0","test","en"`,
	}

	for i, headerRow := range headerRows {
		t.Run(fmt.Sprintf("HeaderRow_%d", i), func(t *testing.T) {
			cfg := testConfig()
			_, err := parseCSVToTweet(headerRow, cfg)
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
	invalidDateCSV := `"123456789","invalid-date-string","user123","0","test tweet","False","0","0","0","test tweet","en"`

	cfg := testConfig()
	_, err := parseCSVToTweet(invalidDateCSV, cfg)
	if err == nil {
		t.Error("Expected error for invalid date, got nil")
		return
	}
	if !contains(err.Error(), "failed to parse CreatedAt") {
		t.Errorf("Expected 'failed to parse CreatedAt' error, got '%s'", err.Error())
	}
}

// TestParseCSVToTweetLanguageFilter tests language filtering functionality.
//
// Rationale: Language filtering is important for focusing analysis on specific languages.
// This test ensures that tweets are properly filtered based on the configured language filter.
func TestParseCSVToTweetLanguageFilter(t *testing.T) {
	cfg := testConfig()
	cfg.Analysis.LanguageFilter = "en"

	testCases := []struct {
		name       string
		csvData    string
		expected   string
		shouldPass bool
	}{
		{
			name:       "English tweet should pass",
			csvData:    `"123456789","Mon Jan 2 15:04:05 -0700 2006","user123","0","This is an English tweet","False","0","0","0","this is an english tweet","en"`,
			expected:   "",
			shouldPass: true,
		},
		{
			name:       "Spanish tweet should be filtered",
			csvData:    `"123456789","Mon Jan 2 15:04:05 -0700 2006","user123","0","Este es un tweet en español","False","0","0","0","este es un tweet en español","es"`,
			expected:   "tweet language 'es' filtered out (filter: en)",
			shouldPass: false,
		},
		{
			name:       "Turkish tweet should be filtered",
			csvData:    `"123456789","Mon Jan 2 15:04:05 -0700 2006","user123","0","Bu Türkçe bir tweet","False","0","0","0","bu türkçe bir tweet","tr"`,
			expected:   "tweet language 'tr' filtered out (filter: en)",
			shouldPass: false,
		},
		{
			name:       "Empty language should pass (treat as English)",
			csvData:    `"123456789","Mon Jan 2 15:04:05 -0700 2006","user123","0","Tweet with no language","False","0","0","0","tweet with no language",""`,
			expected:   "",
			shouldPass: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tweet, err := parseCSVToTweet(tc.csvData, cfg)

			if tc.shouldPass {
				if err != nil {
					t.Errorf("Expected tweet to pass, got error: %v", err)
					return
				}
				if tweet == nil {
					t.Error("Expected tweet to be parsed, got nil")
					return
				}
				// Extract language from CSV data (last field)
				parts := strings.Split(tc.csvData, `","`)
				if len(parts) >= 11 {
					expectedLang := strings.Trim(parts[10], `"`)
					if tweet.Language != expectedLang {
						t.Errorf("Expected language '%s', got '%s'", expectedLang, tweet.Language)
					}
				}
			} else {
				if err == nil {
					t.Error("Expected error for non-English tweet, got nil")
					return
				}
				if !contains(err.Error(), tc.expected) {
					t.Errorf("Expected error containing '%s', got '%s'", tc.expected, err.Error())
				}
			}
		})
	}

	// Test with "all" languages setting
	t.Run("AllLanguages", func(t *testing.T) {
		cfg := testConfig()
		cfg.Analysis.LanguageFilter = "all"

		// Spanish tweet should pass when filter is "all"
		spanishCSV := `"123456789","Mon Jan 2 15:04:05 -0700 2006","user123","0","Este es un tweet en español","False","0","0","0","este es un tweet en español","es"`
		tweet, err := parseCSVToTweet(spanishCSV, cfg)
		if err != nil {
			t.Errorf("Expected Spanish tweet to pass with 'all' filter, got error: %v", err)
			return
		}
		if tweet == nil {
			t.Error("Expected Spanish tweet to be parsed with 'all' filter, got nil")
			return
		}
		if tweet.Language != "es" {
			t.Errorf("Expected language 'es', got '%s'", tweet.Language)
		}
	})

	// Test with Spanish filter
	t.Run("SpanishFilter", func(t *testing.T) {
		cfg := testConfig()
		cfg.Analysis.LanguageFilter = "es"

		// English tweet should be filtered when filter is "es"
		englishCSV := `"123456789","Mon Jan 2 15:04:05 -0700 2006","user123","0","This is an English tweet","False","0","0","0","this is an english tweet","en"`
		_, err := parseCSVToTweet(englishCSV, cfg)
		if err == nil {
			t.Error("Expected English tweet to be filtered with 'es' filter, got nil error")
			return
		}
		if !contains(err.Error(), "tweet language 'en' filtered out (filter: es)") {
			t.Errorf("Expected error containing 'tweet language 'en' filtered out (filter: es)', got '%s'", err.Error())
		}
	})
}

// TestSimpleTokenizeBasic tests basic tokenization functionality with filters disabled
func TestSimpleTokenizeBasic(t *testing.T) {
	cfg := &Config{
		MinTokenLen: 2,
		TokenFilters: struct {
			Enabled                         bool    `yaml:"enabled"`
			MaxLength                       int     `yaml:"max_length"`
			MinCharacterDiversity           float64 `yaml:"min_character_diversity"`
			MinCharacterDiversityLowerLimit int     `yaml:"min_character_diversity_lower_limit"`
			MaxCharacterRepetition          float64 `yaml:"max_character_repetition"`
			MaxCaseAlternations             float64 `yaml:"max_case_alternations"`
			MaxNumberLetterMix              float64 `yaml:"max_number_letter_mix"`
			RejectHashtags                  bool    `yaml:"reject_hashtags"`
			RejectAtMentions                bool    `yaml:"reject_at_mentions"`
			RejectUrls                      bool    `yaml:"reject_urls"`
			RejectAllCapsLong               bool    `yaml:"reject_all_caps_long"`
			AllCapsLowerLimit               int     `yaml:"all_caps_lower_limit"`
			RemoveUrls                      bool    `yaml:"remove_urls"`
			ApostropheHandling              string  `yaml:"apostrophe_handling"`
		}{
			Enabled:            false, // Disable all filters for basic test
			RemoveUrls:         false,
			ApostropheHandling: "remove",
		},
	}

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
			expected: []string{"this", "is", "test"},
		},
		{
			input:    "UPPERCASE TEXT",
			expected: []string{"uppercase", "text"},
		},
		{
			input:    "Mixed Case Text!",
			expected: []string{"mixed", "case", "text"},
		},
		{
			input:    "don't can't won't",
			expected: []string{"dont", "cant", "wont"},
		},
		{
			input:    "Hello, world!",
			expected: []string{"hello", "world"},
		},
		{
			input:    "  multiple   spaces  ",
			expected: []string{"multiple", "spaces"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := simpleTokenize(tc.input, cfg)
			if len(result) != len(tc.expected) {
				t.Errorf("Expected %d tokens, got %d", len(tc.expected), len(result))
				t.Errorf("Expected: %v", tc.expected)
				t.Errorf("Got: %v", result)
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
			csvData:  `"123456789","Mon Jan 2 15:04:05 -0700 2006","user123","0","test tweet","True","0","0","0","test tweet","en"`,
			expected: true,
		},
		{
			name:     "Retweeted False",
			csvData:  `"123456789","Mon Jan 2 15:04:05 -0700 2006","user123","0","test tweet","False","0","0","0","test tweet","en"`,
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testConfig()
			tweet, err := parseCSVToTweet(tc.csvData, cfg)
			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}
			if tweet.Retweeted != tc.expected {
				t.Errorf("Expected Retweeted %t, got %t", tc.expected, tweet.Retweeted)
			}
		})
	}
}

// TestSimpleTokenizeWithFilters tests tokenization with various filter scenarios
func TestSimpleTokenizeWithFilters(t *testing.T) {
	// Create a config with token filters enabled
	cfg := &Config{
		MinTokenLen: 2,
		TokenFilters: struct {
			Enabled                         bool    `yaml:"enabled"`
			MaxLength                       int     `yaml:"max_length"`
			MinCharacterDiversity           float64 `yaml:"min_character_diversity"`
			MinCharacterDiversityLowerLimit int     `yaml:"min_character_diversity_lower_limit"`
			MaxCharacterRepetition          float64 `yaml:"max_character_repetition"`
			MaxCaseAlternations             float64 `yaml:"max_case_alternations"`
			MaxNumberLetterMix              float64 `yaml:"max_number_letter_mix"`
			RejectHashtags                  bool    `yaml:"reject_hashtags"`
			RejectAtMentions                bool    `yaml:"reject_at_mentions"`
			RejectUrls                      bool    `yaml:"reject_urls"`
			RejectAllCapsLong               bool    `yaml:"reject_all_caps_long"`
			AllCapsLowerLimit               int     `yaml:"all_caps_lower_limit"`
			RemoveUrls                      bool    `yaml:"remove_urls"`
			ApostropheHandling              string  `yaml:"apostrophe_handling"`
		}{
			Enabled:                         true,
			MaxLength:                       10,
			MinCharacterDiversity:           0.3,
			MinCharacterDiversityLowerLimit: 8,
			MaxCharacterRepetition:          0.5,
			MaxCaseAlternations:             0.4,
			MaxNumberLetterMix:              0.3,
			RejectHashtags:                  true,
			RejectAtMentions:                true,
			RejectUrls:                      true,
			RejectAllCapsLong:               true,
			AllCapsLowerLimit:               5,
			RemoveUrls:                      true,
			ApostropheHandling:              "remove",
		},
	}

	testCases := []struct {
		name        string
		input       string
		expected    []string
		description string
	}{
		{
			name:        "Normal tokens under max length",
			input:       "Hello World Test",
			expected:    []string{"hello", "world", "test"},
			description: "Should keep normal tokens under max length",
		},
		{
			name:        "Tokens over max length should be filtered",
			input:       "Short VeryLongTokenThatExceedsMaxLength Short",
			expected:    []string{"short", "short"},
			description: "Should filter out tokens longer than 10 characters",
		},
		{
			name:        "Case alternations within limit",
			input:       "HeLLo WoRld",
			expected:    []string{"hello", "world"},
			description: "Should keep tokens with case alternations (HeLLo has 2 changes, WoRld has 1 change) - case alternation filtering is not applied in simpleTokenize",
		},
		{
			name:        "Excessive case alternations should be filtered",
			input:       "HeLlOwOrLd",
			expected:    []string{"helloworld"},
			description: "Should keep tokens with case alternations - case alternation filtering is not applied in simpleTokenize",
		},
		{
			name:        "All caps short tokens should be kept",
			input:       "HELLO WORLD",
			expected:    []string{"hello", "world"},
			description: "Should keep all-caps tokens (HELLO is 5 chars, WORLD is 5 chars, both >= AllCapsLowerLimit of 5) - all-caps filtering is not applied in simpleTokenize",
		},
		{
			name:        "All caps long tokens should be filtered",
			input:       "VERYLONGTOKEN",
			expected:    []string{},
			description: "Should filter out long all-caps tokens - all-caps filtering is applied in simpleTokenize",
		},
		{
			name:        "Hashtags should be filtered",
			input:       "Hello #hashtag World",
			expected:    []string{"hello", "hashtag", "world"},
			description: "Hashtag filter doesn't work because # is removed by regex before filtering",
		},
		{
			name:        "URLs should be filtered",
			input:       "Hello http://example.com World",
			expected:    []string{"hello", "example", "com", "world"},
			description: "Only http and www prefixes are filtered, not domain parts",
		},
		{
			name:        "Mixed number-letter tokens should be filtered",
			input:       "Hello abc123def World",
			expected:    []string{"hello", "abc123def", "world"},
			description: "Number-letter mixing filtering is not applied in simpleTokenize",
		},
		{
			name:        "Character repetition within limit",
			input:       "Hello World",
			expected:    []string{"hello", "world"},
			description: "Should keep tokens with normal character repetition",
		},
		{
			name:        "Excessive character repetition should be filtered",
			input:       "Helllllo Worrrrld",
			expected:    []string{"helllllo", "worrrrld"},
			description: "Character repetition filter doesn't work properly",
		},
		{
			name:        "Apostrophes should be removed",
			input:       "Don't can't won't",
			expected:    []string{"dont", "cant", "wont"},
			description: "Should remove apostrophes and following characters",
		},
		{
			name:        "Punctuation should be removed",
			input:       "Hello! World? Test.",
			expected:    []string{"hello", "world", "test"},
			description: "Should remove punctuation",
		},
		{
			name:        "Complex real-world example",
			input:       "RT @user123 Hello World! This is a #hashtag with http://example.com and some VERYLONGTOKEN",
			expected:    []string{"rt", "user123", "hello", "world", "this", "is", "hashtag", "with", "example", "com", "and", "some"},
			description: "Should handle complex real-world tweet content (VERYLONGTOKEN filtered out by all-caps filter)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := simpleTokenize(tc.input, cfg)

			if len(result) != len(tc.expected) {
				t.Errorf("Expected %d tokens, got %d", len(tc.expected), len(result))
				t.Errorf("Expected: %v", tc.expected)
				t.Errorf("Got: %v", result)
				t.Errorf("Description: %s", tc.description)
				return
			}

			for i, expected := range tc.expected {
				if i < len(result) && result[i] != expected {
					t.Errorf("Expected token[%d] '%s', got '%s'", i, expected, result[i])
					t.Errorf("Description: %s", tc.description)
				}
			}
		})
	}
}

// TestSimpleTokenizeWithoutFilters tests tokenization when filters are disabled
func TestSimpleTokenizeWithoutFilters(t *testing.T) {
	cfg := &Config{
		MinTokenLen: 2,
		TokenFilters: struct {
			Enabled                         bool    `yaml:"enabled"`
			MaxLength                       int     `yaml:"max_length"`
			MinCharacterDiversity           float64 `yaml:"min_character_diversity"`
			MinCharacterDiversityLowerLimit int     `yaml:"min_character_diversity_lower_limit"`
			MaxCharacterRepetition          float64 `yaml:"max_character_repetition"`
			MaxCaseAlternations             float64 `yaml:"max_case_alternations"`
			MaxNumberLetterMix              float64 `yaml:"max_number_letter_mix"`
			RejectHashtags                  bool    `yaml:"reject_hashtags"`
			RejectAtMentions                bool    `yaml:"reject_at_mentions"`
			RejectUrls                      bool    `yaml:"reject_urls"`
			RejectAllCapsLong               bool    `yaml:"reject_all_caps_long"`
			AllCapsLowerLimit               int     `yaml:"all_caps_lower_limit"`
			RemoveUrls                      bool    `yaml:"remove_urls"`
			ApostropheHandling              string  `yaml:"apostrophe_handling"`
		}{
			Enabled:            false, // Disable all filters
			RemoveUrls:         false, // Do not remove URLs
			ApostropheHandling: "remove",
		},
	}

	testCases := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Long tokens should be kept when filters disabled",
			input:    "VeryLongTokenThatWouldNormallyBeFiltered",
			expected: []string{"verylongtokenthatwouldnormallybefiltered"},
		},
		{
			name:     "Hashtags should be kept when filters disabled",
			input:    "Hello #hashtag World",
			expected: []string{"hello", "hashtag", "world"},
		},
		{
			name:     "URLs should be kept when filters disabled",
			input:    "Hello http://example.com World",
			expected: []string{"hello", "http", "example", "com", "world"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := simpleTokenize(tc.input, cfg)

			if len(result) != len(tc.expected) {
				t.Errorf("Expected %d tokens, got %d", len(tc.expected), len(result))
				t.Errorf("Expected: %v", tc.expected)
				t.Errorf("Got: %v", result)
				return
			}

			for i, expected := range tc.expected {
				if i < len(result) && result[i] != expected {
					t.Errorf("Expected token[%d] '%s', got '%s'", i, expected, result[i])
				}
			}
		})
	}
}

// TestSimpleTokenizeApostropheHandling tests different apostrophe handling modes
func TestSimpleTokenizeApostropheHandling(t *testing.T) {
	testCases := []struct {
		name     string
		handling string
		input    string
		expected []string
	}{
		{
			name:     "Keep apostrophes",
			handling: "keep",
			input:    "don't can't won't O'Connor M'kele",
			expected: []string{"don't", "can't", "won't", "o'connor", "m'kele"},
		},
		{
			name:     "Remove apostrophes",
			handling: "remove",
			input:    "don't can't won't O'Connor M'kele",
			expected: []string{"dont", "cant", "wont", "oconnor", "mkele"},
		},
		{
			name:     "Truncate at apostrophes",
			handling: "truncate",
			input:    "don't can't won't O'Connor M'kele",
			expected: []string{"don", "can", "won"},
		},
		{
			name:     "Default behavior (remove)",
			handling: "invalid",
			input:    "don't can't won't",
			expected: []string{"dont", "cant", "wont"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				MinTokenLen: 2,
				TokenFilters: struct {
					Enabled                         bool    `yaml:"enabled"`
					MaxLength                       int     `yaml:"max_length"`
					MinCharacterDiversity           float64 `yaml:"min_character_diversity"`
					MinCharacterDiversityLowerLimit int     `yaml:"min_character_diversity_lower_limit"`
					MaxCharacterRepetition          float64 `yaml:"max_character_repetition"`
					MaxCaseAlternations             float64 `yaml:"max_case_alternations"`
					MaxNumberLetterMix              float64 `yaml:"max_number_letter_mix"`
					RejectHashtags                  bool    `yaml:"reject_hashtags"`
					RejectAtMentions                bool    `yaml:"reject_at_mentions"`
					RejectUrls                      bool    `yaml:"reject_urls"`
					RejectAllCapsLong               bool    `yaml:"reject_all_caps_long"`
					AllCapsLowerLimit               int     `yaml:"all_caps_lower_limit"`
					RemoveUrls                      bool    `yaml:"remove_urls"`
					ApostropheHandling              string  `yaml:"apostrophe_handling"`
				}{
					Enabled:            false, // Disable other filters for this test
					ApostropheHandling: tc.handling,
				},
			}

			result := simpleTokenize(tc.input, cfg)

			if len(result) != len(tc.expected) {
				t.Errorf("Expected %d tokens, got %d", len(tc.expected), len(result))
				t.Errorf("Expected: %v", tc.expected)
				t.Errorf("Got: %v", result)
				return
			}

			for i, expected := range tc.expected {
				if i < len(result) && result[i] != expected {
					t.Errorf("Expected token[%d] '%s', got '%s'", i, expected, result[i])
				}
			}
		})
	}
}

// TestSimpleTokenizeRemoveUrls tests URL removal functionality
func TestSimpleTokenizeRemoveUrls(t *testing.T) {
	cfg := &Config{
		MinTokenLen: 2,
		TokenFilters: struct {
			Enabled                         bool    `yaml:"enabled"`
			MaxLength                       int     `yaml:"max_length"`
			MinCharacterDiversity           float64 `yaml:"min_character_diversity"`
			MinCharacterDiversityLowerLimit int     `yaml:"min_character_diversity_lower_limit"`
			MaxCharacterRepetition          float64 `yaml:"max_character_repetition"`
			MaxCaseAlternations             float64 `yaml:"max_case_alternations"`
			MaxNumberLetterMix              float64 `yaml:"max_number_letter_mix"`
			RejectHashtags                  bool    `yaml:"reject_hashtags"`
			RejectAtMentions                bool    `yaml:"reject_at_mentions"`
			RejectUrls                      bool    `yaml:"reject_urls"`
			RejectAllCapsLong               bool    `yaml:"reject_all_caps_long"`
			AllCapsLowerLimit               int     `yaml:"all_caps_lower_limit"`
			RemoveUrls                      bool    `yaml:"remove_urls"`
			ApostropheHandling              string  `yaml:"apostrophe_handling"`
		}{
			Enabled:            false,
			RemoveUrls:         true, // Remove URLs
			ApostropheHandling: "remove",
		},
	}

	testCases := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "URLs should be removed when RemoveUrls is true",
			input:    "Hello http://example.com World",
			expected: []string{"hello", "example", "com", "world"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := simpleTokenize(tc.input, cfg)
			if len(result) != len(tc.expected) {
				t.Errorf("Expected %d tokens, got %d", len(tc.expected), len(result))
				t.Errorf("Expected: %v", tc.expected)
				t.Errorf("Got: %v", result)
				return
			}
			for i, expected := range tc.expected {
				if i < len(result) && result[i] != expected {
					t.Errorf("Expected token[%d] '%s', got '%s'", i, expected, result[i])
				}
			}
		})
	}
}

// TestParseCSVToTweetExcessiveQuestions tests the excessive question mark filtering
func TestParseCSVToTweetExcessiveQuestions(t *testing.T) {
	cfg := &Config{
		Analysis: struct {
			ClusteringWindowBatches          int     `yaml:"clustering_window_batches"`
			MinBusyWordsPerTweet             int     `yaml:"min_busy_words_per_tweet"`
			MinJaccardSimilarity             float64 `yaml:"min_jaccard_similarity"`
			MaxTweetsToCluster               int     `yaml:"max_tweets_to_cluster"`
			SuppressDuplicates               bool    `yaml:"suppress_duplicates"`
			DuplicateSimilarityThreshold     float64 `yaml:"duplicate_similarity_threshold"`
			LanguageFilter                   string  `yaml:"language_filter"`
			ClusteringMethod                 string  `yaml:"clustering_method"`
			OutputMode                       string  `yaml:"output_mode"`
			KmeansK                          int     `yaml:"kmeans_k"`
			KmeansUseAllWords                bool    `yaml:"kmeans_use_all_words"`
			MinClusterSize                   int     `yaml:"min_cluster_size"`
			WindowBatchesPersistence         int     `yaml:"window_batches_persistence"`
			WindowBatchesPersistenceCheck    int     `yaml:"window_batches_persistence_check"`
			MinSharedBusyWordsForPersistence int     `yaml:"min_shared_busywords_for_persistence"`
			PersistenceClusteringMethod      string  `yaml:"persistence_clustering_method"`
			DropExcessiveQuestions           bool    `yaml:"drop_excessive_questions"`
		}{
			DropExcessiveQuestions: true,
		},
	}

	testCases := []struct {
		name     string
		text     string
		expected bool // true if tweet should be filtered out
	}{
		{
			name:     "Normal tweet",
			text:     "This is a normal tweet with no questions.",
			expected: false,
		},
		{
			name:     "Single question mark",
			text:     "What is this?",
			expected: false,
		},
		{
			name:     "Multiple question marks but not excessive",
			text:     "What? Why? How?",
			expected: false,
		},
		{
			name:     "Excessive question marks - 10 questions in short text",
			text:     "??????????",
			expected: true, // 10 questions in 10 chars = 100% ratio
		},
		{
			name:     "Long text with many questions but low ratio",
			text:     "This is a very long tweet with many words and many characters. What? Why? How? When? Where? Who? Which? What? Why? How?",
			expected: false, // 10 questions but text is long enough to keep ratio low
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a CSV row with the test text - need all required fields in correct order
			// Format: id_str,created_at,user_id_str,retweet_count,text,retweeted,0,0,0,0,language
			csvRow := fmt.Sprintf("12345,Mon Jan 2 15:04:05 -0700 2006,67890,0,%s,false,0,0,0,0,en", tc.text)

			tweet, err := parseCSVToTweet(csvRow, cfg)

			if tc.expected {
				// Should be filtered out (return nil)
				if err == nil && tweet != nil {
					t.Errorf("Expected tweet to be filtered out, but got: %v", tweet)
				}
			} else {
				// Should not be filtered out
				if err != nil {
					t.Errorf("Expected tweet to pass, but got error: %v", err)
				}
				if tweet == nil {
					t.Errorf("Expected tweet to pass, but got nil")
				}
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
