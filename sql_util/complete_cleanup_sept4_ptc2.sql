-- Complete cleanup of sept_4_ptc 2 run
-- First, let's see exactly what we're dealing with
SELECT 
    er.run_name,
    COUNT(DISTINCT b.batch_number) as total_batches,
    COUNT(DISTINCT c.id) as total_clusters,
    COUNT(DISTINCT aar.result_id) as ai_results,
    COUNT(DISTINCT aar.session_id) as sessions
FROM experiment_runs er
LEFT JOIN batches b ON er.run_id = b.run_id
LEFT JOIN clusters c ON b.id = c.batch_id
LEFT JOIN ai_analysis_results aar ON c.id = aar.cluster_id
WHERE er.run_name = 'sept_4_ptc 2'
GROUP BY er.run_id, er.run_name;

-- Delete ALL AI analysis results for sept_4_ptc 2
DELETE FROM ai_analysis_results 
WHERE cluster_id IN (
    SELECT c.id 
    FROM clusters c
    JOIN batches b ON c.batch_id = b.id
    JOIN experiment_runs er ON b.run_id = er.run_id
    WHERE er.run_name = 'sept_4_ptc 2'
);

-- Delete ALL AI analysis sessions for sept_4_ptc 2
DELETE FROM ai_analysis_sessions 
WHERE session_id IN (
    SELECT DISTINCT aar.session_id
    FROM ai_analysis_results aar
    JOIN clusters c ON aar.cluster_id = c.id
    JOIN batches b ON c.batch_id = b.id
    JOIN experiment_runs er ON b.run_id = er.run_id
    WHERE er.run_name = 'sept_4_ptc 2'
);

-- Delete ALL clusters for sept_4_ptc 2
DELETE FROM clusters 
WHERE batch_id IN (
    SELECT b.id 
    FROM batches b
    JOIN experiment_runs er ON b.run_id = er.run_id
    WHERE er.run_name = 'sept_4_ptc 2'
);

-- Delete ALL batches for sept_4_ptc 2
DELETE FROM batches 
WHERE run_id IN (
    SELECT run_id 
    FROM experiment_runs 
    WHERE run_name = 'sept_4_ptc 2'
);

-- Delete the experiment run record itself
DELETE FROM experiment_runs 
WHERE run_name = 'sept_4_ptc 2';

-- Verify cleanup
SELECT 
    run_id,
    run_name,
    run_date_time
FROM experiment_runs
ORDER BY run_id;
