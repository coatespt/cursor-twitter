-- Show how many AI analysis results each cluster has for sept_4_ptc run only
-- This explains why total_ai_results > clusters_with_ai

SELECT 
    analysis_count,
    COUNT(*) as clusters_with_this_count,
    analysis_count * COUNT(*) as total_results_for_this_count
FROM (
    SELECT 
        c.id as cluster_id,
        COUNT(aar.result_id) as analysis_count
    FROM clusters c
    JOIN batches b ON c.batch_id = b.id
    JOIN experiment_runs er ON b.run_id = er.run_id
    LEFT JOIN ai_analysis_results aar ON c.id = aar.cluster_id
    WHERE er.run_name = 'sept_4_ptc'
    GROUP BY c.id
    HAVING COUNT(aar.result_id) > 0  -- Only clusters that have AI analysis
) cluster_counts
GROUP BY analysis_count
ORDER BY analysis_count;
