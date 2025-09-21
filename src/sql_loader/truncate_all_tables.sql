-- Truncate all tables in the correct order (respecting foreign key constraints)
-- This will clear all data but keep the table structure

-- Truncate tables that reference other tables first
TRUNCATE TABLE new_tweet_clusters CASCADE;
TRUNCATE TABLE new_busy_words CASCADE;
TRUNCATE TABLE new_tweets CASCADE;
TRUNCATE TABLE new_clusters CASCADE;
TRUNCATE TABLE new_batches CASCADE;

-- Truncate AI analysis tables
TRUNCATE TABLE ai_analysis_results CASCADE;
TRUNCATE TABLE ai_insights CASCADE;
TRUNCATE TABLE ai_analysis_sessions CASCADE;

-- Truncate experiment runs last (referenced by others)
TRUNCATE TABLE new_experiment_runs CASCADE;

-- Reset sequences to start from 1
ALTER SEQUENCE new_experiment_runs_run_id_seq RESTART WITH 1;
ALTER SEQUENCE new_batches_id_seq RESTART WITH 1;
ALTER SEQUENCE new_clusters_id_seq RESTART WITH 1;
ALTER SEQUENCE new_tweets_id_seq RESTART WITH 1;
ALTER SEQUENCE new_tweet_clusters_id_seq RESTART WITH 1;
ALTER SEQUENCE new_busy_words_id_seq RESTART WITH 1;
ALTER SEQUENCE ai_analysis_sessions_session_id_seq RESTART WITH 1;
ALTER SEQUENCE ai_analysis_results_result_id_seq RESTART WITH 1;
ALTER SEQUENCE ai_insights_insight_id_seq RESTART WITH 1;
