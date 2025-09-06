-- AI Analysis Progress Report for sept_4_ptc run
-- Shows batches and clusters with AI analysis vs total counts

WITH run_stats AS (
    SELECT 
        er.run_id,
        er.run_name,
        COUNT(DISTINCT b.batch_number) as batches_with_ai,
        COUNT(DISTINCT c.id) as clusters_with_ai,
        COUNT(DISTINCT aar.result_id) as total_ai_results
    FROM experiment_runs er
    JOIN batches b ON er.run_id = b.run_id
    JOIN clusters c ON b.id = c.batch_id
    JOIN ai_analysis_results aar ON c.id = aar.cluster_id
    WHERE er.run_name = 'sept_4_ptc'
    GROUP BY er.run_id, er.run_name
),
total_counts AS (
    SELECT 
        er.run_id,
        COUNT(DISTINCT b.batch_number) as total_batches,
        COUNT(DISTINCT c.id) as total_clusters
    FROM experiment_runs er
    JOIN batches b ON er.run_id = b.run_id
    LEFT JOIN clusters c ON b.id = c.batch_id
    WHERE er.run_name = 'sept_4_ptc'
    GROUP BY er.run_id
)
SELECT 
    rs.run_name,
    rs.batches_with_ai,
    rs.clusters_with_ai,
    rs.total_ai_results,
    tc.total_batches,
    tc.total_clusters,
    tc.total_batches - rs.batches_with_ai as batches_remaining,
    tc.total_clusters - rs.clusters_with_ai as clusters_remaining,
    (rs.batches_with_ai::float / tc.total_batches * 100)::numeric(5,2) as batch_completion_pct,
    (rs.clusters_with_ai::float / tc.total_clusters * 100)::numeric(5,2) as cluster_completion_pct
FROM run_stats rs
JOIN total_counts tc ON rs.run_id = tc.run_id;
