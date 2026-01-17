-- recap-worker/recap-db/init/006_sub_stage_progress.sql
-- Add stage column to recap_subworker_runs for evidence/dispatch differentiation.
-- This enables future tracking of evidence stage progress alongside dispatch progress.

-- Add stage column with default 'dispatch' for backward compatibility
ALTER TABLE recap_subworker_runs
ADD COLUMN IF NOT EXISTS stage TEXT NOT NULL DEFAULT 'dispatch';

-- Add check constraint for valid stage values
ALTER TABLE recap_subworker_runs
DROP CONSTRAINT IF EXISTS recap_subworker_runs_stage_check;

ALTER TABLE recap_subworker_runs
ADD CONSTRAINT recap_subworker_runs_stage_check
CHECK (stage IN ('evidence', 'dispatch'));

-- Create index for filtering by stage
CREATE INDEX IF NOT EXISTS idx_recap_subworker_runs_stage
    ON recap_subworker_runs (stage);

-- Create composite index for job_id + stage queries
CREATE INDEX IF NOT EXISTS idx_recap_subworker_runs_job_stage
    ON recap_subworker_runs (job_id, stage);

COMMENT ON COLUMN recap_subworker_runs.stage IS 'Pipeline stage: evidence (corpus building) or dispatch (clustering/summarization)';
