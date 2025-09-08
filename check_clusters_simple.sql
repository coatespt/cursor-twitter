-- Simple cluster count (alternative approach)
SELECT COUNT(*) as cluster_count 
FROM clusters 
WHERE batch_id IN (
    SELECT id FROM batches 
    WHERE run_id = (
        SELECT run_id FROM experiment_runs 
        WHERE run_name LIKE '%batch_30000_rb10_w_log_cluster%' 
        ORDER BY created_at DESC LIMIT 1
    )
);
