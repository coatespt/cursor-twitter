-- DEPRECATED: This file creates the old table names (batches, clusters, tweets, busy_words)
-- Use create_new_tables.sql instead for the current schema with new_* table names
-- This file is kept for reference only

-- Experiment runs table (one row per experimental run)
CREATE TABLE IF NOT EXISTS experiment_runs (
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
CREATE TABLE IF NOT EXISTS batches (
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
CREATE TABLE IF NOT EXISTS clusters (
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
CREATE TABLE IF NOT EXISTS tweets (
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
CREATE UNIQUE INDEX IF NOT EXISTS idx_tweets_one_medoid_per_cluster ON tweets(cluster_id) WHERE is_medoid = TRUE;

-- Busy words table (one row per busy word per cluster)
CREATE TABLE IF NOT EXISTS busy_words (
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
CREATE INDEX IF NOT EXISTS idx_batches_batch_number ON batches(batch_number);
CREATE INDEX IF NOT EXISTS idx_batches_batch_time ON batches(batch_time);
CREATE INDEX IF NOT EXISTS idx_clusters_batch_id ON clusters(batch_id);
CREATE INDEX IF NOT EXISTS idx_clusters_cluster_id ON clusters(cluster_id);
CREATE INDEX IF NOT EXISTS idx_tweets_cluster_id ON tweets(cluster_id);
CREATE INDEX IF NOT EXISTS idx_busy_words_cluster_id ON busy_words(cluster_id);
CREATE INDEX IF NOT EXISTS idx_busy_words_word ON busy_words(word);
CREATE INDEX IF NOT EXISTS idx_busy_words_frequency_class ON busy_words(frequency_class);

-- Comments for documentation
COMMENT ON TABLE batches IS 'Metadata for each batch processed by the pipeline';
COMMENT ON TABLE clusters IS 'Cluster information extracted from each batch';
COMMENT ON TABLE tweets IS 'Individual tweets within each cluster';
COMMENT ON TABLE busy_words IS 'Busy words identified in each cluster with their frequency classes';

COMMENT ON COLUMN tweets.is_medoid IS 'Marks this tweet as the cluster medoid';
COMMENT ON COLUMN clusters.quality_score IS 'Computed quality score for the cluster';
COMMENT ON COLUMN busy_words.word IS 'Busy word identified in the cluster';
COMMENT ON COLUMN busy_words.frequency_class IS 'Frequency class of this word in this batch';
