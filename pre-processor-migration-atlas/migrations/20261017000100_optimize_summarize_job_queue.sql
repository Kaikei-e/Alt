-- Drop 100% redundant index on job_id (summarize_job_queue_job_id_key from UNIQUE constraint already indexes job_id)
DROP INDEX IF EXISTS idx_summarize_job_queue_job_id;

-- Prune terminal jobs (completed, failed) older than 7 days to reclaim heap & index bloat
DELETE FROM summarize_job_queue
WHERE status IN ('completed', 'failed')
  AND COALESCE(completed_at, created_at) < NOW() - INTERVAL '7 days';

-- Clear stored summary text on completed jobs older than the 24h HasRecentSuccessfulJob window
UPDATE summarize_job_queue
SET summary = NULL
WHERE status = 'completed'
  AND summary IS NOT NULL
  AND COALESCE(completed_at, created_at) < NOW() - INTERVAL '24 hours';

-- Add composite covering index for per-article queue guard lookups (HasRecentSuccessfulJob, HasInFlightJob, HasDeadLetterJob, HasRecentFailedJob)
CREATE INDEX IF NOT EXISTS idx_summarize_job_queue_article_status_time
    ON summarize_job_queue (article_id, status, completed_at DESC, created_at DESC);
