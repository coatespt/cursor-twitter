-- Check for foreign key constraint issues
-- Look for AI analysis results that reference non-existent clusters

SELECT 
    'AI results with invalid cluster references' as description,
    COUNT(*) as count
FROM ai_analysis_results aar
LEFT JOIN clusters c ON aar.cluster_id = c.id
WHERE c.id IS NULL

UNION ALL

-- Check for clusters that reference non-existent batches
SELECT 
    'Clusters with invalid batch references' as description,
    COUNT(*) as count
FROM clusters c
LEFT JOIN batches b ON c.batch_id = b.id
WHERE b.id IS NULL

UNION ALL

-- Check for batches that reference non-existent runs
SELECT 
    'Batches with invalid run references' as description,
    COUNT(*) as count
FROM batches b
LEFT JOIN experiment_runs er ON b.run_id = er.run_id
WHERE er.run_id IS NULL;

-- Show some examples of the foreign key violations
SELECT 
    aar.result_id,
    aar.cluster_id,
    aar.created_at
FROM ai_analysis_results aar
LEFT JOIN clusters c ON aar.cluster_id = c.id
WHERE c.id IS NULL
LIMIT 10;
