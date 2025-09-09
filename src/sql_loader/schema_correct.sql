-- Correct Twitter Pipeline Database Schema
-- Uses composite primary keys for proper uniqueness within context

-- Experiment runs table (one row per experimental run)
CREATE TABLE experiment_runs (
    run_id SERIAL PRIMARY KEY,
    run_date_time TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    run_name TEXT NOT NULL UNIQUE,
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
-- PRIMARY KEY is (run_id, batch_number) - batch_number is unique within a run
CREATE TABLE batches (
    run_id INTEGER NOT NULL REFERENCES experiment_runs(run_id) ON DELETE CASCADE,
    batch_number INTEGER NOT NULL,
    batch_time TIMESTAMP WITH TIME ZONE NOT NULL,
    method VARCHAR(50) NOT NULL,
    total_tweets INTEGER NOT NULL,
    total_clusters INTEGER NOT NULL,
    clusters_above_min_size INTEGER NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Composite primary key: batch_number is unique within a run
    PRIMARY KEY (run_id, batch_number)
);

-- Cluster metadata table (one row per cluster)
-- PRIMARY KEY is (run_id, batch_number, cluster_id) - cluster_id is unique within a batch
CREATE TABLE clusters (
    run_id INTEGER NOT NULL,
    batch_number INTEGER NOT NULL,
    cluster_id INTEGER NOT NULL,
    size INTEGER NOT NULL,
    quality_score DECIMAL(5,4),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Foreign key to batches table
    FOREIGN KEY (run_id, batch_number) REFERENCES batches(run_id, batch_number) ON DELETE CASCADE,
    
    -- Composite primary key: cluster_id is unique within a batch
    PRIMARY KEY (run_id, batch_number, cluster_id)
);

-- Individual tweets table (one row per tweet)
-- Uses globally unique ID since tweets can theoretically be in multiple clusters
CREATE TABLE tweets (
    id SERIAL PRIMARY KEY,
    run_id INTEGER NOT NULL,
    batch_number INTEGER NOT NULL,
    cluster_id INTEGER NOT NULL,
    tweet_text TEXT NOT NULL,
    tweet_order INTEGER NOT NULL, -- Order within cluster
    is_medoid BOOLEAN DEFAULT FALSE, -- Marks this tweet as the cluster medoid
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Foreign key to clusters table
    FOREIGN KEY (run_id, batch_number, cluster_id) REFERENCES clusters(run_id, batch_number, cluster_id) ON DELETE CASCADE,
    
    -- Ensure unique tweet within cluster
    UNIQUE (run_id, batch_number, cluster_id, tweet_order)
);

-- Ensure only one medoid per cluster
CREATE UNIQUE INDEX idx_tweets_one_medoid_per_cluster ON tweets(run_id, batch_number, cluster_id) WHERE is_medoid = TRUE;

-- Busy words table (one row per busy word per cluster)
-- Used by AI display system for analysis
CREATE TABLE busy_words (
    id SERIAL PRIMARY KEY,
    run_id INTEGER NOT NULL,
    batch_number INTEGER NOT NULL,
    cluster_id INTEGER NOT NULL,
    word TEXT NOT NULL,
    word_order INTEGER NOT NULL, -- Order within cluster
    frequency_class INTEGER NOT NULL, -- Frequency class of this word in this batch
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Foreign key to clusters table
    FOREIGN KEY (run_id, batch_number, cluster_id) REFERENCES clusters(run_id, batch_number, cluster_id) ON DELETE CASCADE,
    
    -- Ensure unique word within cluster
    UNIQUE (run_id, batch_number, cluster_id, word_order)
);

-- AI Analysis Results table
CREATE TABLE ai_analysis_results (
    id SERIAL PRIMARY KEY,
    run_id INTEGER NOT NULL,
    batch_number INTEGER NOT NULL,
    cluster_id INTEGER NOT NULL,
    analysis_type VARCHAR(50) NOT NULL,
    result_text TEXT,
    confidence_score DECIMAL(3,2),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Foreign key to clusters table
    FOREIGN KEY (run_id, batch_number, cluster_id) REFERENCES clusters(run_id, batch_number, cluster_id) ON DELETE CASCADE
);

-- AI Analysis Sessions table
CREATE TABLE ai_analysis_sessions (
    id SERIAL PRIMARY KEY,
    session_name VARCHAR(100) NOT NULL,
    run_id INTEGER NOT NULL,
    start_time TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    end_time TIMESTAMP WITH TIME ZONE,
    status VARCHAR(20) DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Foreign key to experiment_runs table
    FOREIGN KEY (run_id) REFERENCES experiment_runs(run_id) ON DELETE CASCADE
);

-- AI Insights table
CREATE TABLE ai_insights (
    id SERIAL PRIMARY KEY,
    result_id INTEGER NOT NULL,
    insight_type VARCHAR(50) NOT NULL,
    insight_text TEXT NOT NULL,
    confidence_score DECIMAL(3,2),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Foreign key to ai_analysis_results table
    FOREIGN KEY (result_id) REFERENCES ai_analysis_results(id) ON DELETE CASCADE
);

-- Indexes for performance
CREATE INDEX idx_batches_batch_time ON batches(batch_time);
CREATE INDEX idx_clusters_size ON clusters(size);
CREATE INDEX idx_tweets_cluster ON tweets(run_id, batch_number, cluster_id);
CREATE INDEX idx_busy_words_word ON busy_words(word);
CREATE INDEX idx_busy_words_frequency_class ON busy_words(frequency_class);
CREATE INDEX idx_ai_analysis_results_type ON ai_analysis_results(analysis_type);
