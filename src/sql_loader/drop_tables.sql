-- Drop all Twitter Pipeline tables and indexes
-- Run this to clean up before recreating

-- Drop views first (they depend on tables)
DROP VIEW IF EXISTS significant_frequency_changes CASCADE;
DROP VIEW IF EXISTS word_frequency_evolution CASCADE;
DROP VIEW IF EXISTS common_busy_words CASCADE;
DROP VIEW IF EXISTS batch_stats CASCADE;
DROP VIEW IF EXISTS cluster_summary CASCADE;

-- Drop tables (in reverse dependency order)
DROP TABLE IF EXISTS busy_words CASCADE;
DROP TABLE IF EXISTS tweets CASCADE;
DROP TABLE IF EXISTS clusters CASCADE;
DROP TABLE IF EXISTS batches CASCADE;
DROP TABLE IF EXISTS experiment_runs CASCADE;

-- Verify they're gone
SELECT 'Tables dropped successfully' as status;
