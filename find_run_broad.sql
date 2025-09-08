SELECT run_id, run_name, created_at FROM experiment_runs WHERE run_name LIKE '%30000%' AND run_name LIKE '%rb10%' ORDER BY created_at DESC;
