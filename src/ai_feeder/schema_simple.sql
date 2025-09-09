-- Simplified AI Analysis Database Schema for Testing
-- This version doesn't depend on experiment_runs table

-- AI analysis sessions (simplified for testing)
CREATE TABLE IF NOT EXISTS ai_analysis_sessions (
    session_id SERIAL PRIMARY KEY,
    run_id INTEGER DEFAULT 1, -- Default to 1 for testing
    session_name TEXT NOT NULL,
    ai_model VARCHAR(100) NOT NULL, -- e.g., "llama3:latest"
    ai_endpoint VARCHAR(255) NOT NULL, -- e.g., "http://192.168.1.76:11434/api/generate"
    prompt_template TEXT NOT NULL, -- The template used for generating prompts
    analysis_type VARCHAR(50) NOT NULL, -- e.g., "cluster_summary", "trend_analysis", "anomaly_detection"
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE,
    status VARCHAR(20) DEFAULT 'running', -- 'running', 'completed', 'failed', 'paused'
    total_clusters INTEGER DEFAULT 0,
    processed_clusters INTEGER DEFAULT 0,
    failed_clusters INTEGER DEFAULT 0
);

-- Individual AI analysis requests and responses
CREATE TABLE IF NOT EXISTS ai_analysis_results (
    result_id SERIAL PRIMARY KEY,
    session_id INTEGER NOT NULL REFERENCES ai_analysis_sessions(session_id) ON DELETE CASCADE,
    cluster_id INTEGER NOT NULL REFERENCES new_clusters(cluster_id) ON DELETE CASCADE,
    prompt_text TEXT NOT NULL, -- The actual prompt sent to AI
    response_text TEXT NOT NULL, -- The AI's response
    response_metadata JSONB, -- Additional metadata (tokens used, timing, etc.)
    analysis_metadata JSONB, -- Extracted structured data from response
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    processing_time_ms INTEGER, -- Time taken for this analysis
    
    UNIQUE(session_id, cluster_id)
);

-- Extracted insights from AI analysis (structured data)
CREATE TABLE IF NOT EXISTS ai_insights (
    insight_id SERIAL PRIMARY KEY,
    result_id INTEGER NOT NULL REFERENCES ai_analysis_results(result_id) ON DELETE CASCADE,
    insight_type VARCHAR(50) NOT NULL, -- e.g., "topic", "sentiment", "key_entities", "trend"
    insight_value TEXT NOT NULL,
    confidence_score DECIMAL(3,2), -- AI confidence in this insight (0.0-1.0)
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_ai_sessions_run_id ON ai_analysis_sessions(run_id);
CREATE INDEX IF NOT EXISTS idx_ai_sessions_status ON ai_analysis_sessions(status);
CREATE INDEX IF NOT EXISTS idx_ai_results_session_id ON ai_analysis_results(session_id);
CREATE INDEX IF NOT EXISTS idx_ai_results_cluster_id ON ai_analysis_results(cluster_id);
CREATE INDEX IF NOT EXISTS idx_ai_insights_result_id ON ai_insights(result_id);
CREATE INDEX IF NOT EXISTS idx_ai_insights_type ON ai_insights(insight_type);

-- Comments for documentation
COMMENT ON TABLE ai_analysis_sessions IS 'Tracks AI analysis sessions for experiment runs';
COMMENT ON TABLE ai_analysis_results IS 'Individual AI analysis requests and responses for clusters';
COMMENT ON TABLE ai_insights IS 'Structured insights extracted from AI analysis responses';
