-- Check what's in those remaining 352 batches that don't have AI results
SELECT 
    b.batch_number,
    b.batch_time,
    COUNT(c.id) as cluster_count,
    COUNT(aar.result_id) as ai_results_count
FROM batches b
JOIN experiment_runs er ON b.run_id = er.run_id
LEFT JOIN clusters c ON b.id = c.batch_id
LEFT JOIN ai_analysis_results aar ON c.id = aar.cluster_id
WHERE er.run_name = 'sept_4_ptc'
GROUP BY b.batch_number, b.batch_time
HAVING COUNT(aar.result_id) = 0
ORDER BY b.batch_number
LIMIT 10;
