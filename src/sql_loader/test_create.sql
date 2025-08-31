-- Simple test to create one table in PostgreSQL
-- This will help us debug the issue

-- First, let's see what schemas exist
SELECT schema_name FROM information_schema.schemata WHERE schema_name NOT IN ('information_schema', 'pg_catalog');

-- Create a simple test table
CREATE TABLE IF NOT EXISTS public.test_batches (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);

-- Insert a test row
INSERT INTO public.test_batches (name) VALUES ('test');

-- Check if it was created
SELECT COUNT(*) FROM public.test_batches;

-- Show all tables in public schema
SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE';
