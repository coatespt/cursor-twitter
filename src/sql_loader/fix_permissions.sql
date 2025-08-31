-- Grant permissions to petercoates user on all pipeline tables
-- Run this as a superuser (postgres) or the table owner

-- Grant all permissions on experiment_runs table
GRANT ALL PRIVILEGES ON TABLE public.experiment_runs TO petercoates;
GRANT USAGE, SELECT ON SEQUENCE public.experiment_runs_run_id_seq TO petercoates;

-- Grant all permissions on batches table
GRANT ALL PRIVILEGES ON TABLE public.batches TO petercoates;
GRANT USAGE, SELECT ON SEQUENCE public.batches_id_seq TO petercoates;

-- Grant all permissions on clusters table
GRANT ALL PRIVILEGES ON TABLE public.clusters TO petercoates;
GRANT USAGE, SELECT ON SEQUENCE public.clusters_id_seq TO petercoates;

-- Grant all permissions on tweets table
GRANT ALL PRIVILEGES ON TABLE public.tweets TO petercoates;
GRANT USAGE, SELECT ON SEQUENCE public.tweets_id_seq TO petercoates;

-- Grant all permissions on busy_words table
GRANT ALL PRIVILEGES ON TABLE public.busy_words TO petercoates;
GRANT USAGE, SELECT ON SEQUENCE public.busy_words_id_seq TO petercoates;

-- Grant permissions on any indexes
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO petercoates;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO petercoates;

-- Verify permissions
SELECT 
    table_name,
    privilege_type,
    grantee
FROM information_schema.table_privileges 
WHERE table_schema = 'public' 
AND table_name IN ('experiment_runs', 'batches', 'clusters', 'tweets', 'busy_words')
AND grantee = 'petercoates'
ORDER BY table_name, privilege_type;
