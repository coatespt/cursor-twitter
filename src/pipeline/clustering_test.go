package pipeline

import (
	"cursor-twitter/src/tweets"
	"testing"
)

func TestOptimizedTweetClusterer(t *testing.T) {
	// Create test tweets
	testTweets := []*tweets.Tweet{
		{
			IDStr:  "1",
			Text:   "breaking news earthquake california",
			Tokens: []string{"breaking", "news", "earthquake", "california"},
		},
		{
			IDStr:  "2",
			Text:   "major earthquake hits california coast",
			Tokens: []string{"major", "earthquake", "hits", "california", "coast"},
		},
		{
			IDStr:  "3",
			Text:   "california earthquake damage assessment",
			Tokens: []string{"california", "earthquake", "damage", "assessment"},
		},
		{
			IDStr:  "4",
			Text:   "weather forecast sunny day",
			Tokens: []string{"weather", "forecast", "sunny", "day"},
		},
		{
			IDStr:  "5",
			Text:   "sunny weather perfect for beach",
			Tokens: []string{"sunny", "weather", "perfect", "for", "beach"},
		},
	}

	// Create busy words (earthquake-related and weather-related)
	busyWords := map[string]bool{
		"earthquake": true,
		"california": true,
		"weather":    true,
		"sunny":      true,
	}

	// Create clusterer
	clusterer := NewOptimizedTweetClusterer(0.1, 1000, false)

	// Perform clustering
	result := clusterer.ClusterTweets(testTweets, busyWords, 1)

	// Verify results
	if result.Stats.TotalTweets != 5 {
		t.Errorf("Expected 5 total tweets, got %d", result.Stats.TotalTweets)
	}

	if result.Stats.TweetsWithWords != 5 {
		t.Errorf("Expected 5 tweets with busy words, got %d", result.Stats.TweetsWithWords)
	}

	if len(result.Clusters) == 0 {
		t.Error("Expected at least one cluster, got none")
	}

	// Check that we have at least one edge
	if result.Stats.TotalEdges == 0 {
		t.Error("Expected at least one edge, got none")
	}

	t.Logf("Clustering successful: %d clusters, %d edges, density %.4f",
		len(result.Clusters), result.Stats.TotalEdges, result.Stats.GraphDensity)
}

func TestJaccardSimilarity(t *testing.T) {
	clusterer := NewOptimizedTweetClusterer(0.1, 1000, false)

	tweet1 := &tweets.Tweet{
		IDStr:  "1",
		Text:   "earthquake california breaking",
		Tokens: []string{"earthquake", "california", "breaking"},
	}

	tweet2 := &tweets.Tweet{
		IDStr:  "2",
		Text:   "california earthquake major",
		Tokens: []string{"california", "earthquake", "major"},
	}

	busyWords := map[string]bool{
		"earthquake": true,
		"california": true,
		"breaking":   true,
		"major":      true,
	}
	similarity := clusterer.calculateJaccardSimilarity(tweet1, tweet2, busyWords)

	// Should have 2 shared words out of 4 unique words total
	expected := 2.0 / 4.0 // 0.5
	if similarity != expected {
		t.Errorf("Expected Jaccard similarity %.2f, got %.2f", expected, similarity)
	}

	t.Logf("Jaccard similarity: %.2f", similarity)
}
