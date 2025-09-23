/*
 * Cursor swears that no Interfaces may ever be used in connection with any data type defined in this directory and it will enforce this in any file using these definitions.
 */

package types

// BusyWord represents a single busy word with its statistical data
type BusyWord struct {
	Word   string  `json:"word"`
	Class  int     `json:"class"`
	ZScore float64 `json:"z_score"`
	Count  int     `json:"count"`
	Mean   float64 `json:"mean"`
}

// Tweet represents a single tweet
type Tweet struct {
	Text     string `json:"text"`
	IsMedoid bool   `json:"is_medoid"`
}

// Cluster represents a single cluster
type Cluster struct {
	ClusterID       int            `json:"cluster_id"`
	Size            int            `json:"size"`
	BusyWords       []BusyWord     `json:"busy_words"`
	MedoidTweet     string         `json:"medoid_tweet"`
	Tweets          []Tweet        `json:"tweets"`
	BusyWordClasses map[string]int `json:"busy_word_classes"`
}

// Batch represents a single batch from the JSON data
type Batch struct {
	Type string `json:"type"`
	Data struct {
		BatchNumber          int       `json:"batch_number"`
		BatchTime            string    `json:"batch_time"`
		Method               string    `json:"method"`
		TotalTweets          int       `json:"total_tweets"`
		TotalClusters        int       `json:"total_clusters"`
		ClustersAboveMinSize int       `json:"clusters_above_min_size"`
		Clusters             []Cluster `json:"clusters"`
	} `json:"data"`
}

// Config holds the server configuration
type Config struct {
	InputFile           string  `yaml:"input_file"`
	BatchSize           int     `yaml:"batch_size"`
	HistoricalBatches   int     `yaml:"historical_batches"`
	MinClusterSize      int     `yaml:"min_cluster_size"`
	RecurrenceThreshold float64 `yaml:"recurrence_threshold"`
	RecurrenceStrategy  string  `yaml:"recurrence_strategy"`
	BubbleBatches       int     `yaml:"bubble_batches"`
	BubbleColor         string  `yaml:"bubble_color"`
	BubbleSizeMethod    string  `yaml:"bubble_size_method"` // Options: "zscore" or "count_ratio"
	// Bubble size scaling parameters based on Z-scores
	SmallestZ          float64 `yaml:"smallest_z"`           // Z-score that maps to smallest bubble (default: 4.0)
	LargestZ           float64 `yaml:"largest_z"`            // Z-score that maps to largest bubble (default: 20.0)
	BubbleSizeMultiple float64 `yaml:"bubble_size_multiple"` // Multiple: largest bubble is k times bigger than smallest (default: 2.0)
	// Bubble size scaling parameters based on count ratios (actual/mean)
	SmallestRatio       float64 `yaml:"smallest_ratio"`        // Ratio that maps to smallest bubble (default: 1.0)
	LargestRatio        float64 `yaml:"largest_ratio"`         // Ratio that maps to largest bubble (default: 20.0)
	BubbleRatioMultiple float64 `yaml:"bubble_ratio_multiple"` // Multiple: largest bubble is k times bigger than smallest (default: 2.0)
}

// PageData holds data for the HTML template
type PageData struct {
	CurrentBatch int    `json:"current_batch"`
	Batch        Batch  `json:"batch"`
	HasNext      bool   `json:"has_next"`
	HasPrev      bool   `json:"has_prev"`
	BatchInfo    string `json:"batch_info"`
}

// ChartData represents the data structure for the chart
type ChartData struct {
	Clusters []ClusterData `json:"clusters"`
}

// ClusterData represents a single cluster for the chart
type ClusterData struct {
	ClusterID   int      `json:"cluster_id"`
	Size        int      `json:"size"`
	BusyWords   []string `json:"busy_words"`
	WordCounts  []int    `json:"word_counts"`
	TotalTweets int      `json:"total_tweets"`
}

// GridRow represents a single row in the busy word grid
type GridRow struct {
	Word           string            `json:"word"`
	ClusterID      int               `json:"cluster_id"`
	ClusterSize    int               `json:"cluster_size"`
	QualityScore   float64           `json:"quality_score"`
	HistoricalData map[string]string `json:"historical_data"`
	RecurrenceData map[string]bool   `json:"recurrence_data"`
}

// TweetData represents tweet information for display
type TweetData struct {
	Text     string `json:"text"`
	IsMedoid bool   `json:"is_medoid"`
}

// ClusterTweetData represents tweet data for a cluster
type ClusterTweetData struct {
	ClusterID int         `json:"cluster_id"`
	Size      int         `json:"size"`
	BusyWords []string    `json:"busy_words"`
	Tweets    []TweetData `json:"tweets"`
}

// GridData represents the complete grid for display
type GridData struct {
	CurrentBatch      int                `json:"current_batch"`
	BatchTime         string             `json:"batch_time"`
	BatchDuration     string             `json:"batch_duration"`
	HistoricalBatches []int              `json:"historical_batches"`
	Rows              []GridRow          `json:"rows"`
	MinClusterSize    int                `json:"min_cluster_size"`
	TweetData         []ClusterTweetData `json:"tweet_data"`
}

// MedoidRow represents a single row in the medoid list
type MedoidRow struct {
	BatchNumber     int            `json:"batch_number"`
	BatchTime       string         `json:"batch_time"`
	ClusterID       int            `json:"cluster_id"`
	ClusterSize     int            `json:"cluster_size"`
	MedoidText      string         `json:"medoid_text"`
	BusyWords       []string       `json:"busy_words"`
	PersistenceData map[string]int `json:"persistence_data"`
}

// MedoidData represents the complete medoid list for display
type MedoidData struct {
	CurrentBatch      int         `json:"current_batch"`
	BatchTime         string      `json:"batch_time"`
	BatchDuration     string      `json:"batch_duration"`
	HistoricalBatches []int       `json:"historical_batches"`
	Rows              []MedoidRow `json:"rows"`
	MinClusterSize    int         `json:"min_cluster_size"`
}

// BubbleWord represents a single busyword in the bubble graph
type BubbleWord struct {
	Word            string  `json:"word"`
	FrequencyClass  int     `json:"frequency_class"`
	DivergenceScore float64 `json:"divergence_score"`
	BatchPosition   int     `json:"batch_position"`
	ColorIntensity  float64 `json:"color_intensity"`
	BubbleSize      float64 `json:"bubble_size"`
	YPosition       float64 `json:"y_position"`
}

// BubbleData represents the complete bubble graph data
type BubbleData struct {
	CurrentBatch   int          `json:"current_batch"`
	BatchTime      string       `json:"batch_time"`
	BatchInfo      string       `json:"batch_info"`
	BubbleWords    []BubbleWord `json:"bubble_words"`
	MedoidClusters []MedoidRow  `json:"medoid_clusters"`
	NumBatches     int          `json:"num_batches"`
}
