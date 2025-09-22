-- Create tables for Twitter Pipeline Database (NEW SCHEMA)
-- This replaces the old create_tables.sql with the new table names
-- Run this to recreate the database from scratch

-- Experiment runs table (one row per experimental run)
CREATE TABLE IF NOT EXISTS new_experiment_runs (
    run_id SERIAL PRIMARY KEY,
    run_name TEXT NOT NULL UNIQUE,
    run_date_time TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    window_size INTEGER,
    batch_size INTEGER,
    freq_classes INTEGER,
    min_jaccard_similarity DECIMAL(3,2),
    bw_array_len INTEGER,
    z_scores TEXT, -- Array stored as string
    min_busy_words_per_tweet INTEGER,
    duplicate_similarity_threshold DECIMAL(3,2),
    language_filter VARCHAR(10),
    use_medoid_similarity BOOLEAN,
    use_busy_word_similarity BOOLEAN,
    medoid_similarity_threshold DECIMAL(3,2),
    busy_word_similarity_threshold DECIMAL(3,2),
    min_token_len INTEGER
);

-- Batch metadata table (one row per batch)
CREATE TABLE IF NOT EXISTS new_batches (
    id SERIAL PRIMARY KEY,
    run_id INTEGER NOT NULL REFERENCES new_experiment_runs(run_id) ON DELETE CASCADE,
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
CREATE TABLE IF NOT EXISTS new_clusters (
    id SERIAL PRIMARY KEY,
    batch_id INTEGER NOT NULL REFERENCES new_batches(id) ON DELETE CASCADE,
    cluster_id INTEGER NOT NULL,
    size INTEGER NOT NULL,
    quality_score DECIMAL(5,4),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Ensure unique cluster within batch
    UNIQUE(batch_id, cluster_id)
);

-- Individual tweets table (one row per tweet)
CREATE TABLE IF NOT EXISTS new_tweets (
    id SERIAL PRIMARY KEY,
    cluster_id INTEGER NOT NULL REFERENCES new_clusters(id) ON DELETE CASCADE,
    tweet_id_str TEXT NOT NULL, -- Original tweet ID from Twitter
    unix_timestamp BIGINT NOT NULL, -- Unix timestamp
    created_at_tweet TIMESTAMP WITH TIME ZONE, -- Tweet creation time
    user_id_str TEXT, -- User ID from Twitter
    tweet_text TEXT NOT NULL, -- Tweet content
    retweeted BOOLEAN DEFAULT FALSE,
    retweet_count INTEGER DEFAULT 0,
    lang VARCHAR(10), -- Language code
    batch_id INTEGER, -- Original batch ID from pipeline
    tweet_order INTEGER NOT NULL, -- Order within cluster
    is_medoid BOOLEAN DEFAULT FALSE, -- Marks this tweet as the cluster medoid
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Ensure unique tweet within cluster
    UNIQUE(cluster_id, tweet_order)
);

-- Ensure only one medoid per cluster
CREATE UNIQUE INDEX IF NOT EXISTS idx_new_tweets_one_medoid_per_cluster ON new_tweets(cluster_id) WHERE is_medoid = TRUE;

-- Tweet clusters junction table (many-to-many relationship)
CREATE TABLE IF NOT EXISTS new_tweet_clusters (
    id SERIAL PRIMARY KEY,
    tweet_id INTEGER NOT NULL REFERENCES new_tweets(id) ON DELETE CASCADE,
    cluster_id INTEGER NOT NULL REFERENCES new_clusters(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Ensure unique tweet-cluster combination
    UNIQUE(tweet_id, cluster_id)
);

-- Busy words table (one row per busy word per cluster)
CREATE TABLE IF NOT EXISTS new_busy_words (
    id SERIAL PRIMARY KEY,
    cluster_id INTEGER NOT NULL REFERENCES new_clusters(id) ON DELETE CASCADE,
    word TEXT NOT NULL,
    word_order INTEGER NOT NULL, -- Order within cluster
    frequency_class INTEGER NOT NULL, -- Frequency class of this word in this batch
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    
    -- Ensure unique word within cluster
    UNIQUE(cluster_id, word_order)
);

-- AI Analysis tables
CREATE TABLE IF NOT EXISTS ai_analysis_sessions (
    session_id SERIAL PRIMARY KEY,
    run_id INTEGER NOT NULL REFERENCES new_experiment_runs(run_id) ON DELETE CASCADE,
    session_name TEXT NOT NULL,
    ai_model VARCHAR(100) NOT NULL, -- e.g., "llama3.1:8b", "gpt-4"
    ai_endpoint VARCHAR(255) NOT NULL, -- e.g., "http://localhost:11434/api/generate"
    prompt_template TEXT NOT NULL, -- The template used for generating prompts
    analysis_type VARCHAR(50) NOT NULL, -- e.g., "cluster_summary", "trend_analysis", "anomaly_detection"
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE,
    status VARCHAR(20) DEFAULT 'running', -- 'running', 'completed', 'failed', 'paused'
    total_clusters INTEGER DEFAULT 0,
    processed_clusters INTEGER DEFAULT 0,
    failed_clusters INTEGER DEFAULT 0,
    
    UNIQUE(run_id, session_name)
);

CREATE TABLE IF NOT EXISTS ai_analysis_results (
    result_id SERIAL PRIMARY KEY,
    session_id INTEGER NOT NULL REFERENCES ai_analysis_sessions(session_id) ON DELETE CASCADE,
    cluster_id INTEGER NOT NULL REFERENCES new_clusters(id) ON DELETE CASCADE,
    prompt_text TEXT NOT NULL, -- The actual prompt sent to AI
    response_text TEXT NOT NULL, -- The AI's response
    response_metadata JSONB, -- Additional metadata (tokens used, timing, etc.)
    analysis_metadata JSONB, -- Extracted structured data from response
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    processing_time_ms INTEGER, -- Time taken for this analysis
    
    UNIQUE(session_id, cluster_id)
);

CREATE TABLE IF NOT EXISTS ai_insights (
    insight_id SERIAL PRIMARY KEY,
    result_id INTEGER NOT NULL REFERENCES ai_analysis_results(result_id) ON DELETE CASCADE,
    insight_type VARCHAR(50) NOT NULL, -- e.g., "topic", "sentiment", "key_entities", "trend"
    insight_value TEXT NOT NULL,
    confidence_score DECIMAL(3,2), -- AI confidence in this insight (0.0-1.0)
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Performance indexes
CREATE INDEX IF NOT EXISTS idx_new_batches_run_id ON new_batches(run_id);
CREATE INDEX IF NOT EXISTS idx_new_batches_batch_number ON new_batches(batch_number);
CREATE INDEX IF NOT EXISTS idx_new_batches_batch_time ON new_batches(batch_time);
CREATE INDEX IF NOT EXISTS idx_new_clusters_batch_id ON new_clusters(batch_id);
CREATE INDEX IF NOT EXISTS idx_new_clusters_cluster_id ON new_clusters(cluster_id);
CREATE INDEX IF NOT EXISTS idx_new_tweets_cluster_id ON new_tweets(cluster_id);
CREATE INDEX IF NOT EXISTS idx_new_tweet_clusters_tweet_id ON new_tweet_clusters(tweet_id);
CREATE INDEX IF NOT EXISTS idx_new_tweet_clusters_cluster_id ON new_tweet_clusters(cluster_id);
CREATE INDEX IF NOT EXISTS idx_new_busy_words_cluster_id ON new_busy_words(cluster_id);
CREATE INDEX IF NOT EXISTS idx_new_busy_words_word ON new_busy_words(word);
CREATE INDEX IF NOT EXISTS idx_new_busy_words_frequency_class ON new_busy_words(frequency_class);
CREATE INDEX IF NOT EXISTS idx_ai_analysis_sessions_run_id ON ai_analysis_sessions(run_id);
CREATE INDEX IF NOT EXISTS idx_ai_analysis_results_session_id ON ai_analysis_results(session_id);
CREATE INDEX IF NOT EXISTS idx_ai_analysis_results_cluster_id ON ai_analysis_results(cluster_id);
CREATE INDEX IF NOT EXISTS idx_ai_insights_session_id ON ai_insights(session_id);

-- Comments for documentation
COMMENT ON TABLE new_experiment_runs IS 'Experimental run configuration and metadata';
COMMENT ON TABLE new_batches IS 'Metadata for each batch processed by the pipeline';
COMMENT ON TABLE new_clusters IS 'Cluster information extracted from each batch';
COMMENT ON TABLE new_tweets IS 'Individual tweets within each cluster';
COMMENT ON TABLE new_tweet_clusters IS 'Many-to-many relationship between tweets and clusters';
COMMENT ON TABLE new_busy_words IS 'Busy words identified in each cluster with their frequency classes';
COMMENT ON TABLE ai_analysis_sessions IS 'Tracks AI analysis sessions for experiment runs';
COMMENT ON TABLE ai_analysis_results IS 'Individual AI analysis requests and responses for clusters';
COMMENT ON TABLE ai_insights IS 'Structured insights extracted from AI analysis responses';

COMMENT ON COLUMN ai_analysis_sessions.ai_model IS 'The AI model used for analysis (e.g., llama3.1:8b)';
COMMENT ON COLUMN ai_analysis_sessions.prompt_template IS 'Template used to generate prompts for clusters';
COMMENT ON COLUMN ai_analysis_results.response_metadata IS 'JSON metadata from AI response (tokens, timing, etc.)';
COMMENT ON COLUMN ai_analysis_results.analysis_metadata IS 'Structured data extracted from AI response';
COMMENT ON COLUMN ai_insights.confidence_score IS 'AI confidence in this insight (0.0-1.0)';

COMMENT ON COLUMN new_tweets.is_medoid IS 'Marks this tweet as the cluster medoid';
COMMENT ON COLUMN new_clusters.quality_score IS 'Computed quality score for the cluster';
COMMENT ON COLUMN new_busy_words.word IS 'Busy word identified in the cluster';
COMMENT ON COLUMN new_busy_words.frequency_class IS 'Frequency class of this word in this batch';
