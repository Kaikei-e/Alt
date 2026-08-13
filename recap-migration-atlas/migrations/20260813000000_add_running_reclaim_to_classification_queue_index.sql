-- `pick_next_job` now claims stale 'running' rows (rows whose started_at lease
-- elapsed because the worker holding them died) from the same statement that
-- claims pending work. The pickable partial index has to cover 'running' as
-- well, otherwise it no longer matches the pick predicate at all and every
-- pick degrades to a sequential scan of the whole queue.

DROP INDEX IF EXISTS idx_classification_queue_pickable;
CREATE INDEX IF NOT EXISTS idx_classification_queue_pickable
    ON classification_job_queue(created_at)
    WHERE status IN ('pending', 'retrying', 'running');
