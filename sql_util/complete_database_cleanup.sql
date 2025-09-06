-- Complete database cleanup - remove all data and start fresh
-- WARNING: This will delete ALL data in the database

-- Delete all AI analysis results
DELETE FROM ai_analysis_results;

-- Delete all AI analysis sessions
DELETE FROM ai_analysis_sessions;

-- Delete all clusters
DELETE FROM clusters;

-- Delete all batches
DELETE FROM batches;

-- Delete all experiment runs
DELETE FROM experiment_runs;

-- Verify cleanup
SELECT 
    'AI analysis results' as table_name,
    COUNT(*) as remaining_count
FROM ai_analysis_results

UNION ALL

SELECT 
    'AI analysis sessions' as table_name,
    COUNT(*) as remaining_count
FROM ai_analysis_sessions

UNION ALL

SELECT 
    'Clusters' as table_name,
    COUNT(*) as remaining_count
FROM clusters

UNION ALL

SELECT 
    'Batches' as table_name,
    COUNT(*) as remaining_count
FROM batches

UNION ALL

SELECT 
    'Experiment runs' as table_name,
    COUNT(*) as remaining_count
FROM experiment_runs;
