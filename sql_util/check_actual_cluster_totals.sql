-- Check the actual total number of clusters from the JSON pipeline
-- vs clusters with AI analysis

-- Total clusters inserted from JSON (all clusters in the database)
SELECT 
    'Total clusters from JSON pipeline' as description,
    COUNT(*) as count
FROM clusters c
JOIN batches b ON c.batch_id = b.id
JOIN experiment_runs er ON b.run_id = er.run_id
WHERE er.run_name = 'sept_4_ptc'

UNION ALL

-- Clusters with AI analysis results
SELECT 
    'Clusters with AI analysis' as description,
    COUNT(DISTINCT c.id) as count
FROM clusters c
JOIN batches b ON c.batch_id = b.id
JOIN experiment_runs er ON b.run_id = er.run_id
JOIN ai_analysis_results aar ON c.id = aar.cluster_id
WHERE er.run_name = 'sept_4_ptc'

UNION ALL

-- Total batches with clusters
SELECT 
    'Total batches with clusters' as description,
    COUNT(DISTINCT b.batch_number) as count
FROM batches b
JOIN experiment_runs er ON b.run_id = er.run_id
JOIN clusters c ON b.id = c.batch_id
WHERE er.run_name = 'sept_4_ptc'

UNION ALL

-- Total batches (including empty ones)
SELECT 
    'Total batches (including empty)' as description,
    COUNT(DISTINCT b.batch_number) as count
FROM batches b
JOIN experiment_runs er ON b.run_id = er.run_id
WHERE er.run_name = 'sept_4_ptc';

-- Show some sample data to understand the scale
SELECT 
    b.batch_number,
    COUNT(c.id) as cluster_count
FROM batches b
JOIN experiment_runs er ON b.run_id = er.run_id
LEFT JOIN clusters c ON b.id = c.batch_id
WHERE er.run_name = 'sept_4_ptc'
GROUP BY b.batch_number
ORDER BY b.batch_number DESC
LIMIT 10;
