-- Add window_days column to recap_jobs table for 3-day/7-day recap differentiation
ALTER TABLE recap_jobs
ADD COLUMN IF NOT EXISTS window_days INTEGER NOT NULL DEFAULT 7;

-- Add index for efficient filtering by window_days
CREATE INDEX IF NOT EXISTS idx_recap_jobs_window_days ON recap_jobs(window_days);

-- Add composite index for filtering completed jobs by window_days
CREATE INDEX IF NOT EXISTS idx_recap_jobs_window_days_status ON recap_jobs(window_days, status) WHERE status = 'completed';

-- Add window_days to recap_outputs for more direct querying
ALTER TABLE recap_outputs
ADD COLUMN IF NOT EXISTS window_days INTEGER NOT NULL DEFAULT 7;

-- Add index on recap_outputs for window_days filtering
CREATE INDEX IF NOT EXISTS idx_recap_outputs_window_days ON recap_outputs(window_days);
