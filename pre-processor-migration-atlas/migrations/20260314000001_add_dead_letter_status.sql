-- Migration: add dead_letter status to summarize_job_queue
-- Created: 2026-03-14
-- Description: Adds 'dead_letter' to the status CHECK constraint for non-retryable job failures
--   Fixes SQLSTATE 23514 errors when pre-processor tries to transition jobs to dead_letter

-- Drop the existing CHECK constraint and recreate with dead_letter included
ALTER TABLE summarize_job_queue
    DROP CONSTRAINT IF EXISTS summarize_job_queue_status_check;

ALTER TABLE summarize_job_queue
    ADD CONSTRAINT summarize_job_queue_status_check
    CHECK (status IN ('pending', 'running', 'completed', 'failed', 'dead_letter'));

-- Update the column comment to reflect the new status
COMMENT ON COLUMN summarize_job_queue.status IS 'Job status: pending, running, completed, failed, dead_letter';
