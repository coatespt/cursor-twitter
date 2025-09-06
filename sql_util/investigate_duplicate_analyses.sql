-- Investigate why there are multiple AI analysis results per cluster
-- Look at the actual data to understand the duplication

SELECT 
    c.id as cluster_id,
    c.cluster_id as cluster_number,
    b.batch_number,
    COUNT(aar.result_id) as analysis_count,
    ARRAY_AGG(aar.result_id ORDER BY aar.created_at) as result_ids,
    ARRAY_AGG(aar.session_id ORDER BY aar.created_at) as session_ids,
    ARRAY_AGG(aar.created_at ORDER BY aar.created_at) as created_times
FROM clusters c
JOIN batches b ON c.batch_id = b.id
JOIN experiment_runs er ON b.run_id = er.run_id
JOIN ai_analysis_results aar ON c.id = aar.cluster_id
WHERE er.run_name = 'sept_4_ptc'
  AND c.id IN (
    SELECT c2.id 
    FROM clusters c2
    JOIN batches b2 ON c2.batch_id = b2.id
    JOIN experiment_runs er2 ON b2.run_id = er2.run_id
    JOIN ai_analysis_results aar2 ON c2.id = aar2.cluster_id
    WHERE er2.run_name = 'sept_4_ptc'
    GROUP BY c2.id
    HAVING COUNT(aar2.result_id) > 1
  )
GROUP BY c.id, c.cluster_id, b.batch_number
ORDER BY analysis_count DESC, c.id
LIMIT 10;
