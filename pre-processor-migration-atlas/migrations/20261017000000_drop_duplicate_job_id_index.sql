-- Drop duplicate non-unique index on summarize_job_queue.job_id.
-- The unique constraint summarize_job_queue_job_id_key already maintains a B-tree index on (job_id).
DROP INDEX IF EXISTS idx_summarize_job_queue_job_id;
