-- =====================================================
-- SQL Queries to Explore New Schema Data in pgAdmin
-- =====================================================

-- 1. Overview of all experiment runs
SELECT 
    run_id,
    run_name,
    run_date_time,
    created_at
FROM new_experiment_runs
ORDER BY run_id DESC;

-- 2. Batch summary for a specific run
SELECT 
    b.id as batch_id,
    b.batch_number,
    b.batch_time,
    b.method,
    b.total_tweets,
    b.total_clusters,
    b.clusters_above_min_size,
    b.created_at
FROM new_batches b
JOIN new_experiment_runs r ON b.run_id = r.run_id
WHERE r.run_name = 'DB_SCHEMA_TEST'  -- Change this to your run name
ORDER BY b.batch_number;

-- 3. Cluster details for a specific batch
SELECT 
    c.cluster_id,
    c.batch_id,
    c.size,
    c.quality_score,
    c.created_at,
    b.batch_number
FROM new_clusters c
JOIN new_batches b ON c.batch_id = b.id
JOIN new_experiment_runs r ON b.run_id = r.run_id
WHERE r.run_name = 'DB_SCHEMA_TEST'  -- Change this to your run name
  AND b.batch_number = 0  -- Change this to specific batch number
ORDER BY c.cluster_id;

-- 4. Tweet distribution across clusters
SELECT 
    c.cluster_id,
    c.size as cluster_size,
    COUNT(tc.tweet_id) as actual_tweet_count,
    COUNT(CASE WHEN tc.is_medoid THEN 1 END) as medoid_count
FROM new_clusters c
JOIN new_batches b ON c.batch_id = b.id
JOIN new_experiment_runs r ON b.run_id = r.run_id
LEFT JOIN new_tweet_clusters tc ON c.cluster_id = tc.cluster_id
WHERE r.run_name = 'DB_SCHEMA_TEST'  -- Change this to your run name
  AND b.batch_number = 0  -- Change this to specific batch number
GROUP BY c.cluster_id, c.size
ORDER BY c.cluster_id;

-- 5. Sample tweets from a specific cluster
SELECT 
    t.tweet_id,
    t.tweet_text,
    tc.tweet_order,
    tc.is_medoid,
    c.cluster_id,
    b.batch_number
FROM new_tweets t
JOIN new_tweet_clusters tc ON t.tweet_id = tc.tweet_id
JOIN new_clusters c ON tc.cluster_id = c.cluster_id
JOIN new_batches b ON c.batch_id = b.id
JOIN new_experiment_runs r ON b.run_id = r.run_id
WHERE r.run_name = 'DB_SCHEMA_TEST'  -- Change this to your run name
  AND b.batch_number = 0  -- Change this to specific batch number
  AND c.cluster_id = 1  -- Change this to specific cluster ID
ORDER BY tc.tweet_order;

-- 6. Busy words for a specific cluster
SELECT 
    bw.id,
    bw.word,
    bw.word_order,
    bw.frequency_class,
    c.cluster_id,
    b.batch_number
FROM new_busy_words bw
JOIN new_clusters c ON bw.cluster_id = c.cluster_id
JOIN new_batches b ON c.batch_id = b.id
JOIN new_experiment_runs r ON b.run_id = r.run_id
WHERE r.run_name = 'DB_SCHEMA_TEST'  -- Change this to your run name
  AND b.batch_number = 0  -- Change this to specific batch number
  AND c.cluster_id = 1  -- Change this to specific cluster ID
ORDER BY bw.word_order;

-- 7. Overall statistics for a run
SELECT 
    r.run_name,
    COUNT(DISTINCT b.id) as total_batches,
    COUNT(DISTINCT c.cluster_id) as total_clusters,
    COUNT(DISTINCT t.tweet_id) as total_tweets,
    COUNT(DISTINCT bw.id) as total_busy_words,
    SUM(c.size) as total_tweet_instances,
    AVG(c.size) as avg_cluster_size,
    MIN(b.batch_time) as first_batch,
    MAX(b.batch_time) as last_batch
FROM new_experiment_runs r
LEFT JOIN new_batches b ON r.run_id = b.run_id
LEFT JOIN new_clusters c ON b.id = c.batch_id
LEFT JOIN new_tweet_clusters tc ON c.cluster_id = tc.cluster_id
LEFT JOIN new_tweets t ON tc.tweet_id = t.tweet_id
LEFT JOIN new_busy_words bw ON c.cluster_id = bw.cluster_id
WHERE r.run_name = 'DB_SCHEMA_TEST'  -- Change this to your run name
GROUP BY r.run_id, r.run_name;

-- 8. Find tweets that appear in multiple clusters (if any)
SELECT 
    t.tweet_id,
    t.tweet_text,
    COUNT(tc.cluster_id) as cluster_count,
    ARRAY_AGG(tc.cluster_id ORDER BY tc.cluster_id) as cluster_ids
FROM new_tweets t
JOIN new_tweet_clusters tc ON t.tweet_id = tc.tweet_id
JOIN new_clusters c ON tc.cluster_id = c.cluster_id
JOIN new_batches b ON c.batch_id = b.id
JOIN new_experiment_runs r ON b.run_id = r.run_id
WHERE r.run_name = 'DB_SCHEMA_TEST'  -- Change this to your run name
GROUP BY t.tweet_id, t.tweet_text
HAVING COUNT(tc.cluster_id) > 1
ORDER BY cluster_count DESC, t.tweet_id;

-- 9. Cluster size distribution
SELECT 
    size_range,
    COUNT(*) as cluster_count,
    SUM(size) as total_tweets_in_range
FROM (
    SELECT 
        CASE 
            WHEN c.size = 1 THEN '1'
            WHEN c.size BETWEEN 2 AND 5 THEN '2-5'
            WHEN c.size BETWEEN 6 AND 10 THEN '6-10'
            WHEN c.size BETWEEN 11 AND 20 THEN '11-20'
            WHEN c.size BETWEEN 21 AND 50 THEN '21-50'
            ELSE '50+'
        END as size_range,
        c.size
    FROM new_clusters c
    JOIN new_batches b ON c.batch_id = b.id
    JOIN new_experiment_runs r ON b.run_id = r.run_id
    WHERE r.run_name = 'DB_SCHEMA_TEST'  -- Change this to your run name
) size_groups
GROUP BY size_range
ORDER BY 
    CASE size_range
        WHEN '1' THEN 1
        WHEN '2-5' THEN 2
        WHEN '6-10' THEN 3
        WHEN '11-20' THEN 4
        WHEN '21-50' THEN 5
        WHEN '50+' THEN 6
    END;

-- 10. Most common busy words across all clusters
SELECT 
    bw.word,
    COUNT(*) as cluster_count,
    AVG(bw.frequency_class) as avg_frequency_class,
    MIN(bw.frequency_class) as min_frequency_class,
    MAX(bw.frequency_class) as max_frequency_class
FROM new_busy_words bw
JOIN new_clusters c ON bw.cluster_id = c.cluster_id
JOIN new_batches b ON c.batch_id = b.id
JOIN new_experiment_runs r ON b.run_id = r.run_id
WHERE r.run_name = 'DB_SCHEMA_TEST'  -- Change this to your run name
GROUP BY bw.word
ORDER BY cluster_count DESC, bw.word
LIMIT 20;

-- =====================================================
-- Quick Setup Instructions:
-- =====================================================
-- 1. Copy and paste these queries into pgAdmin
-- 2. Replace 'DB_SCHEMA_TEST' with your actual run name
-- 3. Adjust batch numbers and cluster IDs as needed
-- 4. Run individual queries to explore different aspects of your data
-- =====================================================
