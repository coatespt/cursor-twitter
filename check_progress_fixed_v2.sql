-- Fixed progress query without ORDER BY issue
SELECT 
    r.run_name,
    COUNT(DISTINCT b.id) as batches_processed,
    COUNT(c.id) as clusters_created,
    MAX(b.batch_number) as latest_batch_number,
    r.created_at
FROM experiment_runs r
LEFT JOIN batches b ON r.run_id = b.run_id
LEFT JOIN clusters c ON b.id = c.batch_id
WHERE r.run_name LIKE '%batch_30000_rb10_w_log_cluster%'
GROUP BY r.run_id, r.run_name, r.created_at
ORDER BY r.created_at DESC;
