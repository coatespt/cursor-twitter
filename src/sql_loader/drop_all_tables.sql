-- Drop all tables in the correct order (respecting foreign key constraints)
DROP TABLE IF EXISTS new_tweet_clusters CASCADE;
DROP TABLE IF EXISTS new_busy_words CASCADE;
DROP TABLE IF EXISTS new_tweets CASCADE;
DROP TABLE IF EXISTS new_clusters CASCADE;
DROP TABLE IF EXISTS new_batches CASCADE;
DROP TABLE IF EXISTS new_experiment_runs CASCADE;
DROP TABLE IF EXISTS ai_analysis_results CASCADE;
DROP TABLE IF EXISTS ai_insights CASCADE;
DROP TABLE IF EXISTS ai_analysis_sessions CASCADE;
