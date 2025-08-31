-- Twitter Pipeline Database Schema
-- This schema captures all fields from the JSON output of the main pipeline

-- Enable UUID extension for unique identifiers
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Experiment runs table (one row per experimental run)
CREATE TABLE experiment_runs (
    run_id SERIAL PRIMARY KEY,
    run_date_time TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    run_name TEXT NOT NULL,
    window INTEGER,
    token_persist_files INTEGER,
    rebuild_every_files INTEGER,
    batch INTEGER,
    z_scores TEXT, -- Array stored as string
    freq_classes INTEGER,
    bw_array_len INTEGER,
    busyword_classes TEXT, -- Array stored as string
    min_busy_words_per_tweet INTEGER,
    min_jaccard_similarity DECIMAL(3,2),
    duplicate_similarity_threshold DECIMAL(3,2),
    language_filter VARCHAR(10),
    use_medoid_similarity BOOLEAN,
    use_busy_word_similarity BOOLEAN,
    medoid_similarity_threshold DECIMAL(3,2),
    busy_word_similarity_threshold DECIMAL(3,2),
    min_token_len INTEGER,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Batch metadata table (one row per batch)
CREATE TABLE batches (
    id SERIAL PRIMARY KEY,
    run_id INTEGER NOT NULL REFERENCES experiment_runs(run_id) ON DELETE CASCADE,
    batch_number INTEGER NOT NULL,
    batch_time TIMESTAMP WITH TIME ZONE NOT NULL,
    method VARCHAR(50) NOT NULL,
    total_tweets INTEGER NOT NULL,
    total_clusters INTEGER NOT NULL,
    clusters_above_min_size INTEGER NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Ensure unique batch numbers within a run
    UNIQUE(run_id, batch_number)
);

-- Cluster metadata table (one row per cluster)
CREATE TABLE clusters (
    id SERIAL PRIMARY KEY,
    batch_id INTEGER NOT NULL REFERENCES batches(id) ON DELETE CASCADE,
    cluster_id INTEGER NOT NULL,
    size INTEGER NOT NULL,
    quality_score DECIMAL(5,4),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Ensure unique cluster within batch
    UNIQUE(batch_id, cluster_id)
);

-- Individual tweets table (one row per tweet)
CREATE TABLE tweets (
    id SERIAL PRIMARY KEY,
    cluster_id INTEGER NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    tweet_text TEXT NOT NULL,
    tweet_order INTEGER NOT NULL, -- Order within cluster
    is_medoid BOOLEAN DEFAULT FALSE, -- Marks this tweet as the cluster medoid
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Ensure unique tweet within cluster
    UNIQUE(cluster_id, tweet_order)
);

-- Ensure only one medoid per cluster
CREATE UNIQUE INDEX idx_tweets_one_medoid_per_cluster ON tweets(cluster_id) WHERE is_medoid = TRUE;

-- Busy words table (one row per busy word per cluster)
CREATE TABLE busy_words (
    id SERIAL PRIMARY KEY,
    cluster_id INTEGER NOT NULL REFERENCES clusters(id) ON DELETE CASCADE,
    word TEXT NOT NULL,
    word_order INTEGER NOT NULL, -- Order within cluster
    frequency_class INTEGER NOT NULL, -- Frequency class of this word in this batch
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Ensure unique word within cluster
    UNIQUE(cluster_id, word_order)
);

-- Indexes for performance
CREATE INDEX idx_batches_batch_number ON batches(batch_number);
CREATE INDEX idx_batches_batch_time ON batches(batch_time);
CREATE INDEX idx_clusters_batch_id ON clusters(batch_id);
CREATE INDEX idx_clusters_cluster_id ON clusters(cluster_id);
CREATE INDEX idx_tweets_cluster_id ON tweets(cluster_id);
CREATE INDEX idx_busy_words_cluster_id ON busy_words(cluster_id);
CREATE INDEX idx_busy_words_word ON busy_words(word);
CREATE INDEX idx_busy_words_frequency_class ON busy_words(frequency_class);

-- Views for easier querying

-- View: Complete cluster information with tweet and word counts
CREATE VIEW cluster_summary AS
SELECT 
    b.batch_number,
    b.batch_time,
    c.cluster_id,
    c.size,
    c.quality_score,
    COUNT(DISTINCT t.id) as tweet_count,
    COUNT(DISTINCT bw.id) as busy_word_count,
    (SELECT array_agg(word ORDER BY word_order) FROM busy_words bw2 WHERE bw2.cluster_id = c.id) as busy_words,
    (SELECT array_agg(tweet_text ORDER BY tweet_order) FROM tweets t2 WHERE t2.cluster_id = c.id) as tweet_texts,
    (SELECT t2.tweet_text FROM tweets t2 WHERE t2.cluster_id = c.id AND t2.is_medoid = TRUE LIMIT 1) as medoid_tweet
FROM batches b
JOIN clusters c ON b.id = c.batch_id
LEFT JOIN tweets t ON c.id = t.cluster_id
LEFT JOIN busy_words bw ON c.id = bw.cluster_id
GROUP BY b.id, b.batch_number, b.batch_time, c.id, c.cluster_id, c.size, c.quality_score
ORDER BY b.batch_number, c.cluster_id;

-- View: Batch statistics
CREATE VIEW batch_stats AS
SELECT 
    b.batch_number,
    b.batch_time,
    b.total_tweets,
    b.total_clusters,
    b.clusters_above_min_size,
    COUNT(c.id) as actual_clusters,
    AVG(c.size) as avg_cluster_size,
    MAX(c.size) as max_cluster_size,
    MIN(c.size) as min_cluster_size,
    AVG(c.quality_score) as avg_quality_score
FROM batches b
LEFT JOIN clusters c ON b.id = c.batch_id
GROUP BY b.id, b.batch_number, b.batch_time, b.total_tweets, b.total_clusters, b.clusters_above_min_size
ORDER BY b.batch_number;

-- View: Most common busy words across all batches
CREATE VIEW common_busy_words AS
SELECT 
    bw.word,
    COUNT(DISTINCT c.id) as cluster_count,
    COUNT(DISTINCT b.id) as batch_count,
    AVG(c.size) as avg_cluster_size,
    MAX(c.size) as max_cluster_size
FROM busy_words bw
JOIN clusters c ON bw.cluster_id = c.id
JOIN batches b ON c.batch_id = b.id
GROUP BY bw.word
ORDER BY cluster_count DESC, batch_count DESC;

-- View: Frequency class evolution for words over time
CREATE VIEW word_frequency_evolution AS
SELECT 
    bw.word,
    b.batch_number,
    b.batch_time,
    bw.frequency_class,
    c.cluster_id,
    c.size as cluster_size,
    LAG(bw.frequency_class) OVER (PARTITION BY bw.word ORDER BY b.batch_time) as prev_frequency_class,
    bw.frequency_class - LAG(bw.frequency_class) OVER (PARTITION BY bw.word ORDER BY b.batch_time) as frequency_class_change
FROM busy_words bw
JOIN clusters c ON bw.cluster_id = c.id
JOIN batches b ON c.batch_id = b.id
ORDER BY bw.word, b.batch_time;

-- View: Words with significant frequency class changes
CREATE VIEW significant_frequency_changes AS
SELECT 
    word,
    batch_number,
    batch_time,
    frequency_class,
    prev_frequency_class,
    frequency_class_change,
    CASE 
        WHEN ABS(frequency_class_change) >= 5 THEN 'Major Change'
        WHEN ABS(frequency_class_change) >= 2 THEN 'Moderate Change'
        ELSE 'Minor Change'
    END as change_magnitude
FROM word_frequency_evolution
WHERE frequency_class_change IS NOT NULL
ORDER BY ABS(frequency_class_change) DESC, batch_time;

-- Comments for documentation
COMMENT ON TABLE batches IS 'Metadata for each batch processed by the pipeline';
COMMENT ON TABLE clusters IS 'Cluster information extracted from each batch';
COMMENT ON TABLE tweets IS 'Individual tweets within each cluster';
COMMENT ON TABLE busy_words IS 'Busy words identified in each cluster with their frequency classes (normalized to avoid arrays)';

COMMENT ON COLUMN tweets.is_medoid IS 'Marks this tweet as the cluster medoid';
COMMENT ON COLUMN clusters.quality_score IS 'Computed quality score for the cluster';
COMMENT ON COLUMN busy_words.word IS 'Busy word identified in the cluster';
COMMENT ON COLUMN busy_words.frequency_class IS 'Frequency class of this word in this batch';
