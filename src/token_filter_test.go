package main

import (
	"testing"
)

// testTokenFilterConfig creates a test configuration with token filters enabled
func testTokenFilterConfig() *Config {
	return &Config{
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
			RejectUrls                      bool    `yaml:"reject_urls"`
			RejectAllCapsLong               bool    `yaml:"reject_all_caps_long"`
			AllCapsLowerLimit               int     `yaml:"all_caps_lower_limit"`
		}{
			Enabled:                         true,
			MaxLength:                       20,
			MinCharacterDiversity:           0.3,
			MinCharacterDiversityLowerLimit: 10,
			MaxCharacterRepetition:          0.6,
			MaxCaseAlternations:             0.5,
			MaxNumberLetterMix:              0.3,
			RejectHashtags:                  true,
			RejectUrls:                      true,
			RejectAllCapsLong:               true,
			AllCapsLowerLimit:               10,
		},
	}
}

// TestShouldFilterTokenMaxLength tests the maximum length filter
func TestShouldFilterTokenMaxLength(t *testing.T) {
	cfg := testTokenFilterConfig()
	cfg.TokenFilters.MaxLength = 10

	testCases := []struct {
		token        string
		shouldFilter bool
	}{
		{"short", false},
		{"mediumlength", true},  // 12 chars > 10
		{"verylongtoken", true}, // 13 chars > 10
		{"exactlyten", false},   // 10 chars = 10
	}

	for _, tc := range testCases {
		result := shouldFilterToken(tc.token, cfg)
		if result != tc.shouldFilter {
			t.Errorf("MaxLength filter: token '%s' (len=%d), expected filter=%t, got %t",
				tc.token, len(tc.token), tc.shouldFilter, result)
		}
	}
}

// TestShouldFilterTokenCharacterDiversity tests the character diversity filter
func TestShouldFilterTokenCharacterDiversity(t *testing.T) {
	cfg := testTokenFilterConfig()
	cfg.TokenFilters.MinCharacterDiversity = 0.3
	cfg.TokenFilters.MinCharacterDiversityLowerLimit = 10

	testCases := []struct {
		token        string
		shouldFilter bool
		reason       string
	}{
		{"short", false, "too short to apply diversity filter"},
		{"Baaaaaaaaaaaaaaaaaaaa", true, "low diversity (2/20 = 0.1)"},
		{"jajajajajajajajajajayes", true, "low diversity (4/22 = 0.18)"},
		{"normalword", false, "good diversity (8/10 = 0.8)"},
		{"diversechars", false, "good diversity (10/12 = 0.83)"},
		{"aaaaaaaaaa", true, "no diversity (1/10 = 0.1)"},
	}

	for _, tc := range testCases {
		result := shouldFilterToken(tc.token, cfg)
		if result != tc.shouldFilter {
			t.Errorf("CharacterDiversity filter: token '%s', expected filter=%t, got %t (%s)",
				tc.token, tc.shouldFilter, result, tc.reason)
		}
	}
}

// TestShouldFilterTokenCharacterRepetition tests the character repetition filter
func TestShouldFilterTokenCharacterRepetition(t *testing.T) {
	cfg := testTokenFilterConfig()
	cfg.TokenFilters.MaxCharacterRepetition = 0.6

	testCases := []struct {
		token        string
		shouldFilter bool
		reason       string
	}{
		{"normal", false, "no consecutive repeats"},
		{"hello", false, "one repeat (l), ratio 0.2"},
		{"aaaa", true, "three repeats, ratio 0.75"},
		{"hahaha", false, "three repeats, ratio 0.5 (not > 0.6)"},
		{"ababab", false, "alternating pattern, no consecutive repeats"},
		{"aaaabbbb", true, "many consecutive repeats"},
	}

	for _, tc := range testCases {
		result := shouldFilterToken(tc.token, cfg)
		if result != tc.shouldFilter {
			t.Errorf("CharacterRepetition filter: token '%s', expected filter=%t, got %t (%s)",
				tc.token, tc.shouldFilter, result, tc.reason)
		}
	}
}

// TestShouldFilterTokenCaseAlternations tests the case alternation filter
func TestShouldFilterTokenCaseAlternations(t *testing.T) {
	cfg := testTokenFilterConfig()
	cfg.TokenFilters.MaxCaseAlternations = 0.5

	testCases := []struct {
		token        string
		shouldFilter bool
		reason       string
	}{
		{"normal", false, "no case changes"},
		{"Hello", false, "one case change, ratio 0.2"},
		{"tHiSiS", true, "five case changes, ratio 0.83"},
		{"MiXeDcAsE", true, "eight case changes, ratio 0.89"},
		{"UPPERCASE", false, "no case changes"},
		{"lowercase", false, "no case changes"},
		{"AbCdEf", true, "five case changes, ratio 0.83"},
	}

	for _, tc := range testCases {
		result := shouldFilterToken(tc.token, cfg)
		if result != tc.shouldFilter {
			t.Errorf("CaseAlternations filter: token '%s', expected filter=%t, got %t (%s)",
				tc.token, tc.shouldFilter, result, tc.reason)
		}
	}
}

// TestShouldFilterTokenNumberLetterMix tests the number-letter mixing filter
func TestShouldFilterTokenNumberLetterMix(t *testing.T) {
	cfg := testTokenFilterConfig()
	cfg.TokenFilters.MaxNumberLetterMix = 0.3

	testCases := []struct {
		token        string
		shouldFilter bool
		reason       string
	}{
		{"normal", false, "no digits"},
		{"word123", true, "3/7 = 0.43 > 0.3"},
		{"abc123def456", true, "6/12 = 0.5 > 0.3"},
		{"123456789", true, "9/9 = 1.0 > 0.3"},
		{"abc123", true, "3/6 = 0.5 > 0.3"},
		{"a1b2c3d4e5", true, "5/10 = 0.5 > 0.3"},
		{"word", false, "no digits"},
	}

	for _, tc := range testCases {
		result := shouldFilterToken(tc.token, cfg)
		if result != tc.shouldFilter {
			t.Errorf("NumberLetterMix filter: token '%s', expected filter=%t, got %t (%s)",
				tc.token, tc.shouldFilter, result, tc.reason)
		}
	}
}

// TestShouldFilterTokenHashtags tests the hashtag rejection filter
func TestShouldFilterTokenHashtags(t *testing.T) {
	cfg := testTokenFilterConfig()
	cfg.TokenFilters.RejectHashtags = true

	testCases := []struct {
		token        string
		shouldFilter bool
	}{
		{"normal", false},
		{"#hashtag", true},
		{"#test", true},
		{"word#suffix", false}, // hashtag not at start
		{"#", true},
		{"#123", true},
	}

	for _, tc := range testCases {
		result := shouldFilterToken(tc.token, cfg)
		if result != tc.shouldFilter {
			t.Errorf("Hashtag filter: token '%s', expected filter=%t, got %t",
				tc.token, tc.shouldFilter, result)
		}
	}
}

// TestShouldFilterTokenUrls tests the URL rejection filter
func TestShouldFilterTokenUrls(t *testing.T) {
	cfg := testTokenFilterConfig()
	cfg.TokenFilters.RejectUrls = true

	testCases := []struct {
		token        string
		shouldFilter bool
	}{
		{"normal", false},
		{"http://example.com", true},
		{"https://test.com", true},
		{"www.example.com", true},
		{"wordhttp", false}, // http not at start
		{"http", true},
		{"www", true},
	}

	for _, tc := range testCases {
		result := shouldFilterToken(tc.token, cfg)
		if result != tc.shouldFilter {
			t.Errorf("URL filter: token '%s', expected filter=%t, got %t",
				tc.token, tc.shouldFilter, result)
		}
	}
}

// TestShouldFilterTokenAllCapsLong tests the all-caps long filter
func TestShouldFilterTokenAllCapsLong(t *testing.T) {
	cfg := testTokenFilterConfig()
	cfg.TokenFilters.RejectAllCapsLong = true
	cfg.TokenFilters.AllCapsLowerLimit = 10

	testCases := []struct {
		token        string
		shouldFilter bool
		reason       string
	}{
		{"normal", false, "not all caps"},
		{"SHORT", false, "all caps but too short"},
		{"THISISPROBABLYGARBAGE", true, "all caps and long"},
		{"MixedCase", false, "mixed case"},
		{"UPPERCASE", false, "all caps but exactly 10 chars"},
		{"VERYLONGUPPERCASE", true, "all caps and long"},
		{"lowercase", false, "lowercase"},
	}

	for _, tc := range testCases {
		result := shouldFilterToken(tc.token, cfg)
		if result != tc.shouldFilter {
			t.Errorf("AllCapsLong filter: token '%s', expected filter=%t, got %t (%s)",
				tc.token, tc.shouldFilter, result, tc.reason)
		}
	}
}

// TestShouldFilterTokenDisabled tests that filters are disabled when TokenFilters.Enabled is false
func TestShouldFilterTokenDisabled(t *testing.T) {
	cfg := testTokenFilterConfig()
	cfg.TokenFilters.Enabled = false

	// These should all pass when filters are disabled
	testTokens := []string{
		"Baaaaaaaaaaaaaaaaaaaa",
		"tHiSiSpRoBaBlYgArBaGe",
		"abc123def456",
		"#hashtag",
		"http://example.com",
		"THISISPROBABLYGARBAGE",
	}

	for _, token := range testTokens {
		result := shouldFilterToken(token, cfg)
		if result {
			t.Errorf("Token filter disabled but '%s' was filtered", token)
		}
	}
}

// TestSimpleTokenizeWithFilters tests that the tokenization function applies filters correctly
func TestSimpleTokenizeWithFilters(t *testing.T) {
	cfg := testTokenFilterConfig()

	// Test text with various garbage tokens
	text := "normal Baaaaaaaaaaaaaaaaaaaa tHiSiSpRoBaBlYgArBaGe abc123def456 #hashtag http://example.com THISISPROBABLYGARBAGE good"

	tokens := simpleTokenize(text, cfg)

	// After tokenization, the text becomes lowercase and punctuation is removed
	// So "#hashtag" becomes "hashtag" and passes through
	expected := []string{"normal", "hashtag", "good"}

	if len(tokens) != len(expected) {
		t.Errorf("Expected %d tokens, got %d: %v", len(expected), len(tokens), tokens)
		return
	}

	for i, expectedToken := range expected {
		if tokens[i] != expectedToken {
			t.Errorf("Expected token[%d] '%s', got '%s'", i, expectedToken, tokens[i])
		}
	}
}

// TestSimpleTokenizeWithoutFilters tests that tokenization works normally when filters are disabled
func TestSimpleTokenizeWithoutFilters(t *testing.T) {
	cfg := testTokenFilterConfig()
	cfg.TokenFilters.Enabled = false

	text := "normal Baaaaaaaaaaaaaaaaaaaa tHiSiSpRoBaBlYgArBaGe abc123def456 #hashtag http://example.com THISISPROBABLYGARBAGE good"

	tokens := simpleTokenize(text, cfg)

	// Should include all tokens when filters are disabled
	expected := []string{"normal", "baaaaaaaaaaaaaaaaaaaa", "thisisprobablygarbage", "abc123def456", "hashtag", "httpexamplecom", "thisisprobablygarbage", "good"}

	if len(tokens) != len(expected) {
		t.Errorf("Expected %d tokens, got %d: %v", len(expected), len(tokens), tokens)
		return
	}

	for i, expectedToken := range expected {
		if tokens[i] != expectedToken {
			t.Errorf("Expected token[%d] '%s', got '%s'", i, expectedToken, tokens[i])
		}
	}
}
