-- Very simple test with explicit commit
BEGIN;

CREATE TABLE public.simple_test (
    id INTEGER PRIMARY KEY,
    name TEXT
);

INSERT INTO public.simple_test VALUES (1, 'test');

COMMIT;

-- Now check if it exists
SELECT 'Table created' as status, COUNT(*) as row_count FROM public.simple_test;

-- List all tables
SELECT table_name FROM information_schema.tables 
WHERE table_schema = 'public' 
AND table_type = 'BASE TABLE'
ORDER BY table_name;
