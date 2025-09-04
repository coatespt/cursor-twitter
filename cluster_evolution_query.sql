-- Cluster Evolution Query with AI Analysis Results
-- This query tracks how a cluster evolves over time by finding similar clusters
-- based on shared busy words across batches, and shows the AI analysis summaries.

-- Parameters to adjust:
-- 1. TARGET_CLUSTER_ID: The cluster ID to analyze (currently set to 1020)
-- 2. MIN_MATCHING_WORDS: Minimum number of busy words that must match (currently set to 2)
-- 3. MAX_BATCHES_BACK: How many batches back to look (currently set to 20)

WITH target_cluster AS (
    SELECT 
        c.batch_id, 
        c.id as cluster_id, 
        c.cluster_id as cluster_number, 
        c.size, 
        b.batch_number, 
        b.batch_time 
    FROM clusters c 
    JOIN batches b ON c.batch_id = b.id 
    WHERE c.id = 1020  -- CHANGE THIS: Set to the cluster ID you want to analyze
),
target_busy_words AS (
    SELECT word, frequency_class, word_order 
    FROM busy_words 
    WHERE cluster_id = 1020  -- CHANGE THIS: Same as above
    ORDER BY word_order
),
batch_matches AS (
    SELECT 
        c.batch_id, 
        c.id as cluster_id, 
        c.cluster_id as cluster_number, 
        c.size, 
        COUNT(bw.word) as matching_words,
        ARRAY_AGG(bw.word ORDER BY bw.word_order) as matching_words_list,
        ARRAY_AGG(bw.frequency_class ORDER BY bw.word_order) as matching_freq_classes,
        (SELECT batch_id FROM target_cluster) - c.batch_id as batches_back
    FROM clusters c 
    JOIN busy_words bw ON c.id = bw.cluster_id 
    WHERE bw.word IN (SELECT word FROM target_busy_words) 
        AND c.batch_id < (SELECT batch_id FROM target_cluster)
        AND c.batch_id >= (SELECT batch_id FROM target_cluster) - 20  -- CHANGE THIS: How many batches back to look
    GROUP BY c.batch_id, c.id, c.cluster_id, c.size 
    HAVING COUNT(bw.word) >= 2  -- CHANGE THIS: Minimum number of matching words
    ORDER BY c.batch_id DESC, matching_words DESC
),
all_clusters_with_ai AS (
    SELECT 
        'TARGET CLUSTER' as type,
        tc.batch_id,
        tc.cluster_id,
        tc.cluster_number,
        tc.size,
        tc.batch_number,
        tc.batch_time,
        ARRAY(SELECT word FROM target_busy_words ORDER BY word_order) as busy_words,
        0 as batches_back,
        COALESCE(ar.response_text, 'No AI analysis available') as ai_summary
    FROM target_cluster tc
    LEFT JOIN ai_analysis_results ar ON tc.cluster_id = ar.cluster_id

    UNION ALL

    SELECT 
        'MATCHING CLUSTERS' as type,
        bm.batch_id,
        bm.cluster_id,
        bm.cluster_number,
        bm.size,
        b.batch_number,
        b.batch_time,
        bm.matching_words_list as busy_words,
        bm.batches_back,
        COALESCE(ar.response_text, 'No AI analysis available') as ai_summary
    FROM batch_matches bm 
    JOIN batches b ON bm.batch_id = b.id 
    LEFT JOIN ai_analysis_results ar ON bm.cluster_id = ar.cluster_id
)
-- Display AI summaries first for easy scanning, then full details
SELECT 
    type,
    batch_number,
    batches_back,
    LEFT(ai_summary, 300) as ai_summary_preview,
    busy_words,
    size,
    cluster_number
FROM all_clusters_with_ai
ORDER BY batch_id DESC, type DESC;

-- ============================================================================
-- OPTIONAL: For detailed reading, uncomment the query below to see full AI text
-- ============================================================================

/*
-- Full AI Analysis Text (for detailed reading)
SELECT 
    type,
    batch_number,
    batches_back,
    ai_summary as full_ai_analysis,
    busy_words,
    size,
    cluster_number
FROM all_clusters_with_ai
ORDER BY batch_id DESC, type DESC;
*/

-- ============================================================================
-- MULTI-CLUSTER ANALYSIS: Analyze N preceding clusters from a starting ID
-- ============================================================================

/*
-- Parameters for multi-cluster analysis:
-- 1. STARTING_CLUSTER_ID: The highest cluster ID to start from (currently set to 1020)
-- 2. NUM_PRECEDING_CLUSTERS: How many preceding clusters to analyze (currently set to 5)
-- 3. MIN_MATCHING_WORDS: Minimum busy words that must match (currently set to 2)
-- 4. MAX_BATCHES_BACK: How many batches back to look for each cluster (currently set to 10)

WITH cluster_sequence AS (
    SELECT 
        c.id as cluster_id,
        c.batch_id,
        c.cluster_id as cluster_number,
        c.size,
        b.batch_number,
        b.batch_time,
        ROW_NUMBER() OVER (ORDER BY c.id DESC) as sequence_rank
    FROM clusters c
    JOIN batches b ON c.batch_id = b.id
    WHERE c.id <= 1020  -- CHANGE THIS: Set to the starting cluster ID
    ORDER BY c.id DESC
    LIMIT 5  -- CHANGE THIS: Set to number of preceding clusters to analyze
),
cluster_busy_words AS (
    SELECT 
        cs.cluster_id,
        cs.batch_id,
        cs.batch_number,
        cs.sequence_rank,
        cs.size,
        ARRAY_AGG(bw.word ORDER BY bw.word_order) as busy_words,
        ARRAY_AGG(bw.frequency_class ORDER BY bw.word_order) as freq_classes
    FROM cluster_sequence cs
    JOIN busy_words bw ON cs.cluster_id = bw.cluster_id
    GROUP BY cs.cluster_id, cs.batch_id, cs.batch_number, cs.sequence_rank, cs.size
),
cluster_evolution AS (
    SELECT 
        cbw.cluster_id,
        cbw.batch_id,
        cbw.batch_number,
        cbw.sequence_rank,
        cbw.busy_words,
        cbw.freq_classes,
        cbw.size,
        COALESCE(ar.response_text, 'No AI analysis available') as ai_summary,
        -- Find clusters from earlier batches with similar busy words
        (SELECT COUNT(DISTINCT c2.id)
         FROM clusters c2
         JOIN busy_words bw2 ON c2.id = bw2.cluster_id
         WHERE bw2.word = ANY(cbw.busy_words)
           AND c2.batch_id < cbw.batch_id
           AND c2.batch_id >= cbw.batch_id - 10  -- CHANGE THIS: Max batches back to look
           AND c2.id != cbw.cluster_id
        ) as similar_clusters_count
    FROM cluster_busy_words cbw
    LEFT JOIN ai_analysis_results ar ON cbw.cluster_id = ar.cluster_id
)
SELECT 
    sequence_rank,
    cluster_id,
    batch_number,
    busy_words,
    freq_classes,
    LEFT(ai_summary, 200) as ai_summary_preview,
    similar_clusters_count,
    size
FROM cluster_evolution
ORDER BY sequence_rank;

-- This query shows:
-- 1. A sequence of N clusters starting from the specified ID
-- 2. Their busy words and frequency classes
-- 3. AI analysis summaries (truncated for easy scanning)
-- 4. Count of similar clusters found in earlier batches
-- 5. How topics evolve across the cluster sequence
*/

-- Example usage:
-- 1. Change TARGET_CLUSTER_ID to the cluster you want to analyze
-- 2. Adjust MIN_MATCHING_WORDS (in the HAVING clause) to control similarity threshold
-- 3. Adjust MAX_BATCHES_BACK to control how far back in time to look
-- 4. The results show:
--    - AI summaries (truncated to 300 chars) for easy scanning
--    - Busy words, cluster size, and cluster number
--    - Evolution of the topic over time with AI insights
-- 5. Uncomment the second query to see full AI analysis text
-- 6. Uncomment the third query for multi-cluster analysis across N preceding clusters
