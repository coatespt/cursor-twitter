-- Clear all Twitter Pipeline data from the database
-- This script removes all data but preserves the schema structure

-- Disable foreign key checks temporarily to allow deletion in any order
SET session_replication_role = replica;

-- Clear all data tables (in dependency order)
TRUNCATE TABLE busy_words CASCADE;
TRUNCATE TABLE tweets CASCADE;
TRUNCATE TABLE clusters CASCADE;
TRUNCATE TABLE batches CASCADE;
TRUNCATE TABLE experiment_runs CASCADE;

-- Re-enable foreign key checks
SET session_replication_role = DEFAULT;

-- Reset sequences to start from 1
ALTER SEQUENCE experiment_runs_run_id_seq RESTART WITH 1;
ALTER SEQUENCE batches_id_seq RESTART WITH 1;
ALTER SEQUENCE clusters_id_seq RESTART WITH 1;
ALTER SEQUENCE tweets_id_seq RESTART WITH 1;
ALTER SEQUENCE busy_words_id_seq RESTART WITH 1;

-- Verify tables are empty
SELECT 'experiment_runs' as table_name, COUNT(*) as row_count FROM experiment_runs
UNION ALL
SELECT 'batches', COUNT(*) FROM batches
UNION ALL
SELECT 'clusters', COUNT(*) FROM clusters
UNION ALL
SELECT 'tweets', COUNT(*) FROM tweets
UNION ALL
SELECT 'busy_words', COUNT(*) FROM busy_words
ORDER BY table_name;

-- Display summary
SELECT 'Database cleared successfully' as status;
