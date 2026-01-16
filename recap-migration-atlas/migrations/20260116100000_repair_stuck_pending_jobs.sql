-- Repair stuck jobs that have status='pending' but last_stage='persist'
-- This is a data repair migration to fix inconsistent job states.
--
-- Root cause: update_job_status was not checking rows_affected, so if a job
-- was deleted between save_stage_state and update_job_status, the stage state
-- was saved but the job status was not updated.
--
-- Fix: Jobs with output in recap_outputs are marked as 'completed'.
--      Jobs without output are marked as 'failed'.

-- First, check the current state (for logging/audit purposes)
-- SELECT job_id, status, last_stage, kicked_at
-- FROM recap_jobs
-- WHERE status = 'pending' AND last_stage = 'persist';

-- Mark jobs with existing output as completed
UPDATE recap_jobs rj
SET status = 'completed',
    updated_at = NOW()
WHERE status = 'pending'
  AND last_stage = 'persist'
  AND EXISTS (
    SELECT 1 FROM recap_outputs ro
    WHERE ro.job_id = rj.job_id
  );

-- Mark jobs without output as failed
UPDATE recap_jobs
SET status = 'failed',
    updated_at = NOW()
WHERE status = 'pending'
  AND last_stage = 'persist'
  AND NOT EXISTS (
    SELECT 1 FROM recap_outputs ro
    WHERE ro.job_id = recap_jobs.job_id
  );

-- Also fix any jobs that are stuck in 'pending' state with other last_stage values
-- These are less common but could also indicate incomplete processing
UPDATE recap_jobs
SET status = 'failed',
    updated_at = NOW()
WHERE status = 'pending'
  AND last_stage IS NOT NULL
  AND last_stage != 'persist'
  AND updated_at < NOW() - INTERVAL '1 hour';
