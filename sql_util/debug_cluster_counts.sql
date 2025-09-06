-- Debug the cluster counts to understand what's happening

-- Total clusters in the database
SELECT 
    'Total clusters in DB' as description,
    COUNT(*) as count
FROM clusters c
JOIN batches b ON c.batch_id = b.id
JOIN experiment_runs er ON b.run_id = er.run_id
WHERE er.run_name = 'sept_4_ptc'

UNION ALL

-- Clusters with AI analysis
SELECT 
    'Clusters with AI analysis' as description,
    COUNT(DISTINCT c.id) as count
FROM clusters c
JOIN batches b ON c.batch_id = b.id
JOIN experiment_runs er ON b.run_id = er.run_id
JOIN ai_analysis_results aar ON c.id = aar.cluster_id
WHERE er.run_name = 'sept_4_ptc'

UNION ALL

-- Total AI results
SELECT 
    'Total AI results' as description,
    COUNT(*) as count
FROM ai_analysis_results aar
JOIN clusters c ON aar.cluster_id = c.id
JOIN batches b ON c.batch_id = b.id
JOIN experiment_runs er ON b.run_id = er.run_id
WHERE er.run_name = 'sept_4_ptc'

UNION ALL

-- Batches with clusters
SELECT 
    'Batches with clusters' as description,
    COUNT(DISTINCT b.batch_number) as count
FROM batches b
JOIN experiment_runs er ON b.run_id = er.run_id
JOIN clusters c ON b.id = c.batch_id
WHERE er.run_name = 'sept_4_ptc'

UNION ALL

-- Total batches
SELECT 
    'Total batches' as description,
    COUNT(DISTINCT b.batch_number) as count
FROM batches b
JOIN experiment_runs er ON b.run_id = er.run_id
WHERE er.run_name = 'sept_4_ptc';
