-- Add a deferred-visibility column so retry backoff is enforced by the
-- queue query itself, not by a worker holding its semaphore permit through
-- a `sleep()`. Without this, `mark_retrying` leaves the row immediately
-- pickable (status='retrying' has no delay signal), so another worker can
-- re-pick it before the intended backoff has elapsed while the original
-- worker's permit sits idle in the sleep.

ALTER TABLE classification_job_queue
    ADD COLUMN IF NOT EXISTS next_retry_at TIMESTAMPTZ;

-- Replace the status-only partial index with one that also covers the
-- next_retry_at filter added to `pick_next_job`.
DROP INDEX IF EXISTS idx_classification_queue_status;
CREATE INDEX IF NOT EXISTS idx_classification_queue_pickable
    ON classification_job_queue(created_at)
    WHERE status IN ('pending', 'retrying');
