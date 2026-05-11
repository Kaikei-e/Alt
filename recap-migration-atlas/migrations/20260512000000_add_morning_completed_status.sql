-- Add `morning_completed` as a recognised recap job status.
--
-- Background: the morning-update daemon (recap-worker, every ~30 min) reuses
-- the `recap_jobs` row machinery of the batch pipeline — its fetch stage
-- INSERTs a row with the schema default `status='pending'` (needed for the
-- advisory lock + the raw-article-backup FK) — but the morning pipeline never
-- advanced that row, so one orphaned `pending` row leaked per tick (~48/day)
-- and only a recap-worker restart swept them to `failed`. recap-worker now
-- seals those rows as `morning_completed`, kept distinct from `completed` so
-- the dashboard can tell 30-min editorial ticks apart from real recap jobs.
-- This migration widens the history CHECK constraint to accept the new status
-- and back-fills the already-leaked orphan `pending` morning rows.

-- Step 1: allow `morning_completed` in the immutable status history log.
ALTER TABLE "recap_job_status_history" DROP CONSTRAINT "chk_status_history_status";
ALTER TABLE "recap_job_status_history" ADD CONSTRAINT "chk_status_history_status"
    CHECK (status IN ('pending', 'running', 'completed', 'failed', 'morning_completed'));

-- Step 2: back-fill orphaned morning rows that pre-date the recap-worker fix.
-- These are rows the morning fetch stage created but never sealed; the morning
-- letter itself was persisted to `morning_letters` regardless, so treat them as
-- completed. (`recap_jobs.status` carries no CHECK constraint, so no ALTER.)
UPDATE "recap_jobs"
SET status = 'morning_completed',
    last_stage = COALESCE(last_stage, 'persist'),
    updated_at = NOW()
WHERE status = 'pending'
  AND trigger_source = 'morning';

-- Step 3: record the terminal transition in the event log for the rows we just
-- sealed, unless a `morning_completed` event already exists for them.
INSERT INTO "recap_job_status_history" (job_id, status, stage, transitioned_at, reason, actor)
SELECT job_id, 'morning_completed', last_stage, NOW(),
       'backfill: morning-update row sealed by migration 20260512000000', 'migration_backfill'
FROM "recap_jobs"
WHERE status = 'morning_completed'
  AND trigger_source = 'morning'
  AND NOT EXISTS (
    SELECT 1 FROM "recap_job_status_history" h
    WHERE h.job_id = "recap_jobs".job_id AND h.status = 'morning_completed'
  );
