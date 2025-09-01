-- Fix batches table to add missing run_id column and link existing data
-- Run this script to make existing batches work with the new experiment_runs table

-- Add the missing run_id column
ALTER TABLE batches ADD COLUMN IF NOT EXISTS run_id INTEGER;

-- Update all existing batches to reference experiment run ID 1 (the first one we created)
UPDATE batches SET run_id = 1 WHERE run_id IS NULL;

-- Make run_id NOT NULL after updating
ALTER TABLE batches ALTER COLUMN run_id SET NOT NULL;

-- Add the foreign key constraint (using proper syntax)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints 
        WHERE constraint_name = 'fk_batches_run_id'
    ) THEN
        ALTER TABLE batches ADD CONSTRAINT fk_batches_run_id 
            FOREIGN KEY (run_id) REFERENCES experiment_runs(run_id) ON DELETE CASCADE;
    END IF;
END $$;

-- Verify the fix
SELECT 
    'Total batches' as description, 
    COUNT(*) as count 
FROM batches
UNION ALL
SELECT 
    'Batches with run_id = 1' as description, 
    COUNT(*) as count 
FROM batches WHERE run_id = 1
UNION ALL
SELECT 
    'Batches with NULL run_id' as description, 
    COUNT(*) as count 
FROM batches WHERE run_id IS NULL;

-- Show the updated table structure
\d batches
