package main

import (
	"cursor-twitter/src/tweets"
	"testing"
)

func TestWordDistance(t *testing.T) {
	tests := []struct {
		name     string
		tweet1   string
		tweet2   string
		expected int
	}{
		{
			name:     "identical tweets",
			tweet1:   "hello world",
			tweet2:   "hello world",
			expected: 0,
		},
		{
			name:     "one word different",
			tweet1:   "hello world",
			tweet2:   "hello there",
			expected: 1,
		},
		{
			name:     "completely different",
			tweet1:   "hello world",
			tweet2:   "goodbye universe",
			expected: 2,
		},
		{
			name:     "insertion",
			tweet1:   "hello world",
			tweet2:   "hello beautiful world",
			expected: 1,
		},
		{
			name:     "deletion",
			tweet1:   "hello beautiful world",
			tweet2:   "hello world",
			expected: 1,
		},
		{
			name:     "substitution",
			tweet1:   "hello world",
			tweet2:   "hello earth",
			expected: 1,
		},
		{
			name:     "multiple changes",
			tweet1:   "the quick brown fox",
			tweet2:   "a fast red dog",
			expected: 4,
		},
		{
			name:     "empty strings",
			tweet1:   "",
			tweet2:   "",
			expected: 0,
		},
		{
			name:     "one empty",
			tweet1:   "hello",
			tweet2:   "",
			expected: 1,
		},
		{
			name:     "case insensitive",
			tweet1:   "Hello World",
			tweet2:   "hello world",
			expected: 0,
		},
		{
			name:     "extra whitespace",
			tweet1:   "hello  world",
			tweet2:   "hello world",
			expected: 0,
		},
		{
			name:     "real tweet example - URL difference",
			tweet1:   "Check out this amazing product https://example.com/product1",
			tweet2:   "Check out this amazing product https://example.com/product2",
			expected: 1,
		},
		{
			name:     "real tweet example - number difference",
			tweet1:   "Breaking news: 50 people injured in accident",
			tweet2:   "Breaking news: 100 people injured in accident",
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wordDistance(tt.tweet1, tt.tweet2)
			if result != tt.expected {
				t.Errorf("wordDistance(%q, %q) = %d, expected %d", tt.tweet1, tt.tweet2, result, tt.expected)
			}
		})
	}
}

func TestNormalizedWordDistance(t *testing.T) {
	tests := []struct {
		name     string
		tweet1   string
		tweet2   string
		expected float64
	}{
		{
			name:     "identical tweets",
			tweet1:   "hello world",
			tweet2:   "hello world",
			expected: 0.0,
		},
		{
			name:     "one word different in two word tweet",
			tweet1:   "hello world",
			tweet2:   "hello there",
			expected: 0.5, // 1 edit / 2 max words
		},
		{
			name:     "one word different in longer tweet",
			tweet1:   "the quick brown fox jumps",
			tweet2:   "the quick brown dog jumps",
			expected: 0.2, // 1 edit / 5 max words
		},
		{
			name:     "completely different same length",
			tweet1:   "hello world",
			tweet2:   "goodbye earth",
			expected: 1.0, // 2 edits / 2 max words
		},
		{
			name:     "empty strings",
			tweet1:   "",
			tweet2:   "",
			expected: 0.0,
		},
		{
			name:     "one empty",
			tweet1:   "hello",
			tweet2:   "",
			expected: 1.0, // 1 edit / 1 max word
		},
		{
			name:     "insertion in short tweet",
			tweet1:   "hello",
			tweet2:   "hello world",
			expected: 0.5, // 1 edit / 2 max words
		},
		{
			name:     "real tweet example - minor variation",
			tweet1:   "Just had the best coffee ever at this new cafe downtown",
			tweet2:   "Just had the best coffee ever at this new shop downtown",
			expected: 0.091, // 1 edit / 11 max words (cafe vs shop)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizedWordDistance(tt.tweet1, tt.tweet2)
			// Use tolerance for floating point comparison
			tolerance := 0.001
			if result < tt.expected-tolerance || result > tt.expected+tolerance {
				t.Errorf("normalizedWordDistance(%q, %q) = %.3f, expected %.3f", tt.tweet1, tt.tweet2, result, tt.expected)
			}
		})
	}
}

func TestRemoveNearDuplicates(t *testing.T) {
	// Create test tweets
	testTweets := []*tweets.Tweet{
		{Text: "Check out this amazing product https://example.com/product1"},
		{Text: "Check out this amazing product https://example.com/product2"},
		{Text: "Check out this amazing product https://example.com/product3"},
		{Text: "Completely different tweet about something else"},
		{Text: "Check out this amazing product https://example.com/product4"},
	}

	tests := []struct {
		name            string
		tweets          []*tweets.Tweet
		threshold       float64
		distanceMethod  string
		expectedCount   int
		expectedRemoved int
	}{
		{
			name:            "strict threshold removes near duplicates",
			tweets:          testTweets,
			threshold:       0.1, // Very strict - should remove similar tweets
			distanceMethod:  "word",
			expectedCount:   2, // Should keep medoid + completely different tweet
			expectedRemoved: 3, // Should remove 3 similar tweets
		},
		{
			name:            "loose threshold keeps more tweets",
			tweets:          testTweets,
			threshold:       0.5, // Loose - should keep more tweets
			distanceMethod:  "word",
			expectedCount:   4, // Should keep most tweets
			expectedRemoved: 1, // Should remove only 1
		},
		{
			name:            "single tweet",
			tweets:          []*tweets.Tweet{{Text: "single tweet"}},
			threshold:       0.1,
			distanceMethod:  "word",
			expectedCount:   1,
			expectedRemoved: 0,
		},
		{
			name:            "empty tweet list",
			tweets:          []*tweets.Tweet{},
			threshold:       0.1,
			distanceMethod:  "word",
			expectedCount:   0,
			expectedRemoved: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, removed := removeNearDuplicates(tt.tweets, tt.threshold, tt.distanceMethod)

			if len(result) != tt.expectedCount {
				t.Errorf("Expected %d tweets after deduplication, got %d", tt.expectedCount, len(result))
			}

			if removed != tt.expectedRemoved {
				t.Errorf("Expected %d tweets removed, got %d", tt.expectedRemoved, removed)
			}

			// Verify that the medoid (first tweet) is always kept
			if len(result) > 0 && len(tt.tweets) > 0 {
				// The medoid should be one of the original tweets
				found := false
				for _, originalTweet := range tt.tweets {
					if result[0].Text == originalTweet.Text {
						found = true
						break
					}
				}
				if !found {
					t.Error("Medoid tweet not found in original tweets")
				}
			}
		})
	}
}

func TestWordDistanceEdgeCases(t *testing.T) {
	t.Run("very long tweets", func(t *testing.T) {
		tweet1 := "this is a very long tweet with many words that should test the algorithm's performance and correctness when dealing with longer text content"
		tweet2 := "this is a very long tweet with many words that should test the algorithm's performance and correctness when dealing with longer text content"

		result := wordDistance(tweet1, tweet2)
		if result != 0 {
			t.Errorf("Expected 0 for identical long tweets, got %d", result)
		}
	})

	t.Run("tweets with special characters", func(t *testing.T) {
		tweet1 := "Hello @user! Check out #hashtag and $symbol"
		tweet2 := "Hello @user! Check out #hashtag and $symbol"

		result := wordDistance(tweet1, tweet2)
		if result != 0 {
			t.Errorf("Expected 0 for identical tweets with special chars, got %d", result)
		}
	})

	t.Run("tweets with URLs", func(t *testing.T) {
		tweet1 := "Visit https://example.com for more info"
		tweet2 := "Visit https://different.com for more info"

		result := wordDistance(tweet1, tweet2)
		if result != 1 {
			t.Errorf("Expected 1 for tweets with different URLs, got %d", result)
		}
	})
}

func BenchmarkWordDistance(b *testing.B) {
	tweet1 := "This is a benchmark test tweet with multiple words to measure performance"
	tweet2 := "This is a benchmark test tweet with multiple words to measure speed"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wordDistance(tweet1, tweet2)
	}
}

func BenchmarkNormalizedWordDistance(b *testing.B) {
	tweet1 := "This is a benchmark test tweet with multiple words to measure performance"
	tweet2 := "This is a benchmark test tweet with multiple words to measure speed"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		normalizedWordDistance(tweet1, tweet2)
	}
}
