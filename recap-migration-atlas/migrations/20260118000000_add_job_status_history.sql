-- Immutable event log for job status transitions
-- This implements Event Sourcing pattern for job status management
-- to fix race conditions and provide complete audit trail.

-- Step 1: Create the history table
CREATE TABLE IF NOT EXISTS recap_job_status_history (
    id BIGSERIAL PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES recap_jobs(job_id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    stage TEXT,
    transitioned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reason TEXT,
    actor TEXT DEFAULT 'system',

    CONSTRAINT chk_status_history_status CHECK (status IN ('pending', 'running', 'completed', 'failed'))
);

-- Step 2: Create indexes
CREATE INDEX IF NOT EXISTS idx_job_status_history_job_id
    ON recap_job_status_history(job_id);
CREATE INDEX IF NOT EXISTS idx_job_status_history_job_latest
    ON recap_job_status_history(job_id, id DESC);

-- Step 3: Backfill history from existing jobs
-- Create initial 'pending' event for each existing job based on kicked_at
INSERT INTO recap_job_status_history (job_id, status, transitioned_at, actor)
SELECT job_id, 'pending', kicked_at, 'migration_backfill'
FROM recap_jobs
WHERE NOT EXISTS (
    SELECT 1 FROM recap_job_status_history h WHERE h.job_id = recap_jobs.job_id
);

-- Create current status event based on updated_at for non-pending jobs
INSERT INTO recap_job_status_history (job_id, status, stage, transitioned_at, actor)
SELECT job_id, status, last_stage, updated_at, 'migration_backfill'
FROM recap_jobs
WHERE status != 'pending'
  AND NOT EXISTS (
    SELECT 1 FROM recap_job_status_history h
    WHERE h.job_id = recap_jobs.job_id AND h.status = recap_jobs.status
);
