-- Check the most recent AI analysis results to see if progress is being made
SELECT 
    aar.result_id,
    aar.created_at,
    c.cluster_id,
    b.batch_number,
    b.batch_time
FROM ai_analysis_results aar
JOIN clusters c ON aar.cluster_id = c.id
JOIN batches b ON c.batch_id = b.id
JOIN experiment_runs er ON b.run_id = er.run_id
WHERE er.run_name = 'sept_4_ptc'
ORDER BY aar.created_at DESC
LIMIT 10;

-- Check if there are any new results in the last few minutes
SELECT 
    COUNT(*) as recent_results,
    MAX(aar.created_at) as latest_result
FROM ai_analysis_results aar
JOIN clusters c ON aar.cluster_id = c.id
JOIN batches b ON c.batch_id = b.id
JOIN experiment_runs er ON b.run_id = er.run_id
WHERE er.run_name = 'sept_4_ptc'
  AND aar.created_at > NOW() - INTERVAL '5 minutes';
