-- First, let's see what runs we have and how many AI results each has
SELECT 
    er.run_name,
    COUNT(DISTINCT aar.result_id) as ai_results_count,
    COUNT(DISTINCT aar.cluster_id) as clusters_with_ai,
    COUNT(DISTINCT aar.session_id) as sessions_count
FROM experiment_runs er
LEFT JOIN batches b ON er.run_id = b.run_id
LEFT JOIN clusters c ON b.id = c.batch_id
LEFT JOIN ai_analysis_results aar ON c.id = aar.cluster_id
GROUP BY er.run_id, er.run_name
ORDER BY er.run_id;

-- Show what we're about to delete (AI results NOT from sept_4_ptc)
SELECT 
    er.run_name,
    COUNT(DISTINCT aar.result_id) as ai_results_to_delete,
    COUNT(DISTINCT aar.cluster_id) as clusters_affected,
    COUNT(DISTINCT aar.session_id) as sessions_to_delete
FROM experiment_runs er
JOIN batches b ON er.run_id = b.run_id
JOIN clusters c ON b.id = c.batch_id
JOIN ai_analysis_results aar ON c.id = aar.cluster_id
WHERE er.run_name != 'sept_4_ptc'
GROUP BY er.run_id, er.run_name;

-- Delete AI analysis results for runs other than sept_4_ptc
DELETE FROM ai_analysis_results 
WHERE cluster_id IN (
    SELECT c.id 
    FROM clusters c
    JOIN batches b ON c.batch_id = b.id
    JOIN experiment_runs er ON b.run_id = er.run_id
    WHERE er.run_name != 'sept_4_ptc'
);

-- Delete AI analysis sessions for runs other than sept_4_ptc
DELETE FROM ai_analysis_sessions 
WHERE session_id IN (
    SELECT DISTINCT aar.session_id
    FROM ai_analysis_results aar
    JOIN clusters c ON aar.cluster_id = c.id
    JOIN batches b ON c.batch_id = b.id
    JOIN experiment_runs er ON b.run_id = er.run_id
    WHERE er.run_name != 'sept_4_ptc'
);

-- Show final counts after cleanup
SELECT 
    er.run_name,
    COUNT(DISTINCT aar.result_id) as ai_results_remaining,
    COUNT(DISTINCT aar.cluster_id) as clusters_with_ai,
    COUNT(DISTINCT aar.session_id) as sessions_remaining
FROM experiment_runs er
LEFT JOIN batches b ON er.run_id = b.run_id
LEFT JOIN clusters c ON b.id = c.batch_id
LEFT JOIN ai_analysis_results aar ON c.id = aar.cluster_id
GROUP BY er.run_id, er.run_name
ORDER BY er.run_id;
