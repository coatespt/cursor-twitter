-- Fixed cluster count query
SELECT COUNT(*) as cluster_count 
FROM clusters c
JOIN batches b ON c.batch_id = b.id
JOIN experiment_runs r ON b.run_id = r.run_id
WHERE r.run_name LIKE '%batch_30000_rb10_w_log_cluster%'
ORDER BY r.created_at DESC;
