-- Add target_date column to pulse_generations for existing deployments
-- This is a no-op for new deployments where the column already exists in base migration.

-- Add target_date column if missing
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'pulse_generations'
        AND column_name = 'target_date'
    ) THEN
        ALTER TABLE pulse_generations
        ADD COLUMN target_date DATE NOT NULL DEFAULT CURRENT_DATE;
    END IF;
END $$;

-- Add index if missing
CREATE INDEX IF NOT EXISTS idx_pulse_generations_target_date
    ON pulse_generations (target_date DESC);

-- Drop and recreate view to include target_date column
DROP VIEW IF EXISTS pulse_latest_generations;

CREATE VIEW pulse_latest_generations AS
SELECT DISTINCT ON (job_id)
    id,
    job_id,
    target_date,
    version,
    status,
    topics_count,
    started_at,
    finished_at,
    config_snapshot,
    result_payload,
    error_message
FROM pulse_generations
ORDER BY job_id, started_at DESC;

COMMENT ON VIEW pulse_latest_generations IS 'Latest pulse generation for each job';
