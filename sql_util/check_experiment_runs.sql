-- Check what experiment runs exist
SELECT 
    run_id,
    run_name,
    run_date_time,
    window_size,
    batch_size
FROM experiment_runs
ORDER BY run_id;

-- Check if sept_4_ptc 2 has any data at all
SELECT 
    er.run_name,
    COUNT(DISTINCT b.batch_number) as total_batches,
    COUNT(DISTINCT c.id) as total_clusters,
    COUNT(DISTINCT aar.result_id) as ai_results
FROM experiment_runs er
LEFT JOIN batches b ON er.run_id = b.run_id
LEFT JOIN clusters c ON b.id = c.batch_id
LEFT JOIN ai_analysis_results aar ON c.id = aar.cluster_id
WHERE er.run_name = 'sept_4_ptc 2'
GROUP BY er.run_id, er.run_name;
