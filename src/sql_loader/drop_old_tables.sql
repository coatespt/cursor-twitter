-- Drop old tables that are no longer used
-- Run this to clean up the old schema tables

-- Drop old tables in dependency order (children first)
DROP TABLE IF EXISTS busy_words CASCADE;
DROP TABLE IF EXISTS tweets CASCADE;
DROP TABLE IF EXISTS clusters CASCADE;
DROP TABLE IF EXISTS batches CASCADE;
DROP TABLE IF EXISTS experiment_runs CASCADE;

-- Drop any old indexes that might exist
DROP INDEX IF EXISTS idx_batches_batch_number;
DROP INDEX IF EXISTS idx_batches_batch_time;
DROP INDEX IF EXISTS idx_clusters_batch_id;
DROP INDEX IF EXISTS idx_clusters_cluster_id;
DROP INDEX IF EXISTS idx_tweets_cluster_id;
DROP INDEX IF EXISTS idx_busy_words_cluster_id;
DROP INDEX IF EXISTS idx_busy_words_word;
DROP INDEX IF EXISTS idx_busy_words_frequency_class;
DROP INDEX IF EXISTS idx_tweets_one_medoid_per_cluster;

-- Comments
COMMENT ON SCHEMA public IS 'Old tables have been removed. Use new_* tables instead.';
