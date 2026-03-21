\echo 'Audit stale pending summarize jobs (pending rows whose summaries already exist)'

SELECT COUNT(*) AS stale_pending_count
FROM summarize_job_queue q
WHERE q.status = 'pending'
  AND EXISTS (
    SELECT 1
    FROM article_summaries s
    WHERE s.article_id = q.article_id
  );

\echo 'Sample stale rows'

SELECT q.article_id, q.created_at, q.retry_count
FROM summarize_job_queue q
WHERE q.status = 'pending'
  AND EXISTS (
    SELECT 1
    FROM article_summaries s
    WHERE s.article_id = q.article_id
  )
ORDER BY q.created_at ASC
LIMIT 50;

\echo 'Delete stale pending rows inside a transaction after reviewing the audit above'
BEGIN;

WITH deleted AS (
  DELETE FROM summarize_job_queue
  WHERE id IN (
    SELECT candidate.id
    FROM summarize_job_queue candidate
    WHERE candidate.status = 'pending'
      AND EXISTS (
        SELECT 1
        FROM article_summaries s
        WHERE s.article_id = candidate.article_id
      )
    ORDER BY candidate.created_at ASC
    FOR UPDATE SKIP LOCKED
  )
  RETURNING article_id
)
SELECT COUNT(*) AS deleted_stale_pending_count
FROM deleted;

COMMIT;
