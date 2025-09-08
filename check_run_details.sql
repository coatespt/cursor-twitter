SELECT run_id, run_name, created_at, batch_count, cluster_count FROM experiment_runs WHERE run_name LIKE '%batch_30000_rb10_w_log_cluster%' ORDER BY created_at DESC;
