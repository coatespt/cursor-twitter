SELECT COUNT(*) as cluster_count FROM clusters WHERE run_id = (SELECT run_id FROM experiment_runs WHERE run_name LIKE '%batch_30000_rb10_w_log_cluster%' ORDER BY created_at DESC LIMIT 1);
