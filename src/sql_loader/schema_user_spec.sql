-- Database Schema Based on User Specifications
-- From SESSION_NOTES.md lines 22-40

-- Experiment runs table (one row per experimental run)
CREATE TABLE experiment_runs (
    run_id SERIAL PRIMARY KEY,  -- Globally unique key
    run_name TEXT NOT NULL UNIQUE,  -- Name like "DB_CLEAR_TEST" or "PTC_window_size_test"
    run_date_time TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    -- Could add other run metadata here
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Batch metadata table (one row per batch)
CREATE TABLE batches (
    id SERIAL PRIMARY KEY,  -- Meaningless globally unique key
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
    cluster_id SERIAL PRIMARY KEY,  -- Globally unique cluster_id
    batch_id INTEGER NOT NULL REFERENCES batches(id) ON DELETE CASCADE,
    size INTEGER NOT NULL,
    quality_score DECIMAL(5,4),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Ensure unique cluster within batch (using batch_id + some ordering)
    UNIQUE(batch_id, cluster_id)
);

-- Individual tweets table (one row per tweet)
CREATE TABLE tweets (
    tweet_id SERIAL PRIMARY KEY,  -- Globally unique meaningless tweet_id
    tweet_text TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Linkage table: tweets can be part of multiple clusters
CREATE TABLE tweet_clusters (
    tweet_id INTEGER NOT NULL REFERENCES tweets(tweet_id) ON DELETE CASCADE,
    cluster_id INTEGER NOT NULL REFERENCES clusters(cluster_id) ON DELETE CASCADE,
    tweet_order INTEGER NOT NULL, -- Order within cluster
    is_medoid BOOLEAN DEFAULT FALSE, -- Marks this tweet as the cluster medoid
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Primary key is the combination
    PRIMARY KEY (tweet_id, cluster_id),
    
    -- Ensure unique tweet order within cluster
    UNIQUE (cluster_id, tweet_order)
);

-- Ensure only one medoid per cluster
CREATE UNIQUE INDEX idx_tweet_clusters_one_medoid_per_cluster ON tweet_clusters(cluster_id) WHERE is_medoid = TRUE;

-- Busy words table (one row per busy word per cluster)
CREATE TABLE busy_words (
    id SERIAL PRIMARY KEY,  -- Globally unique meaningless key
    cluster_id INTEGER NOT NULL REFERENCES clusters(cluster_id) ON DELETE CASCADE,
    word TEXT NOT NULL,
    word_order INTEGER NOT NULL, -- Order within cluster
    frequency_class INTEGER NOT NULL, -- Frequency class of this word in this batch
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Ensure unique word within cluster
    UNIQUE (cluster_id, word_order)
);

-- AI Analysis Results table
CREATE TABLE ai_analysis_results (
    id SERIAL PRIMARY KEY,  -- Globally unique meaningless key
    cluster_id INTEGER NOT NULL REFERENCES clusters(cluster_id) ON DELETE CASCADE,
    analysis_type VARCHAR(50) NOT NULL,
    result_text TEXT,
    confidence_score DECIMAL(3,2),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- AI Analysis Sessions table
CREATE TABLE ai_analysis_sessions (
    id SERIAL PRIMARY KEY,  -- Globally unique meaningless key
    session_name VARCHAR(100) NOT NULL,
    run_id INTEGER NOT NULL REFERENCES experiment_runs(run_id) ON DELETE CASCADE,
    start_time TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    end_time TIMESTAMP WITH TIME ZONE,
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- AI Insights table
CREATE TABLE ai_insights (
    id SERIAL PRIMARY KEY,  -- Globally unique meaningless key
    result_id INTEGER NOT NULL REFERENCES ai_analysis_results(id) ON DELETE CASCADE,
    insight_type VARCHAR(50) NOT NULL,
    insight_text TEXT NOT NULL,
    confidence_score DECIMAL(3,2),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for performance
CREATE INDEX idx_batches_run_id ON batches(run_id);
CREATE INDEX idx_batches_batch_time ON batches(batch_time);
CREATE INDEX idx_clusters_batch_id ON clusters(batch_id);
CREATE INDEX idx_tweet_clusters_cluster_id ON tweet_clusters(cluster_id);
CREATE INDEX idx_tweet_clusters_tweet_id ON tweet_clusters(tweet_id);
CREATE INDEX idx_busy_words_cluster_id ON busy_words(cluster_id);
CREATE INDEX idx_busy_words_word ON busy_words(word);
CREATE INDEX idx_busy_words_frequency_class ON busy_words(frequency_class);
CREATE INDEX idx_ai_analysis_results_cluster_id ON ai_analysis_results(cluster_id);
CREATE INDEX idx_ai_analysis_results_type ON ai_analysis_results(analysis_type);
