-- Constrain `recap_jobs.status` to the same enumerated set already enforced
-- on `recap_job_status_history` (chk_status_history_status, widened in
-- 20260512000000_add_morning_completed_status.sql). `recap_jobs.status` has
-- carried no CHECK since the column was introduced, so any typo written by
-- a future INSERT/UPDATE would silently pass through instead of failing
-- fast at the DB boundary.
--
-- Verified read-only against production recap-db before writing this
-- migration: `SELECT status, count(*) FROM recap_jobs GROUP BY status`
-- returned only 'failed' (86) and 'morning_completed' (595) — both already
-- members of the target set, so this ADD CONSTRAINT validates cleanly.
ALTER TABLE "recap_jobs" ADD CONSTRAINT "chk_recap_jobs_status"
    CHECK (status IN ('pending', 'running', 'completed', 'failed', 'morning_completed'));
