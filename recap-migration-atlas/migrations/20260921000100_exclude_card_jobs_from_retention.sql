-- Exclude recap jobs associated with card snapshots, candidates, cards, job stats,
-- or evaluation windows from daily retention cleanup.
-- Cards and evaluation rows are INSERT-only golden data; their FKs have no CASCADE
-- so the delete must skip them across all 5 tables.
-- The job timestamp column is kicked_at.
-- Unrelated cleanups (log_errors, classification_job_queue, admin_jobs) run first.

CREATE OR REPLACE PROCEDURE cleanup_old_recap_data()
LANGUAGE plpgsql AS $$
DECLARE
    batch_size INTEGER := 1000;
    deleted_count INTEGER;
    retention_days INTEGER;
    cutoff_date TIMESTAMPTZ;
BEGIN
    -- 保持期間を取得
    SELECT r.retention_days INTO retention_days
    FROM recap_retention_config r
    ORDER BY id DESC LIMIT 1;

    cutoff_date := NOW() - (retention_days || ' days')::INTERVAL;

    -- 1. 古いエラーログを削除
    LOOP
        DELETE FROM log_errors
        WHERE id IN (
            SELECT id FROM log_errors
            WHERE timestamp < cutoff_date
            LIMIT batch_size
            FOR UPDATE SKIP LOCKED
        );

        GET DIAGNOSTICS deleted_count = ROW_COUNT;
        COMMIT;

        EXIT WHEN deleted_count < batch_size;
        PERFORM pg_sleep(0.1);
    END LOOP;

    -- 2. 完了した分類ジョブキューを削除
    LOOP
        DELETE FROM classification_job_queue
        WHERE id IN (
            SELECT id FROM classification_job_queue
            WHERE status IN ('completed', 'failed')
              AND created_at < cutoff_date
            LIMIT batch_size
            FOR UPDATE SKIP LOCKED
        );

        GET DIAGNOSTICS deleted_count = ROW_COUNT;
        COMMIT;

        EXIT WHEN deleted_count < batch_size;
        PERFORM pg_sleep(0.1);
    END LOOP;

    -- 3. 完了した管理ジョブを削除
    LOOP
        DELETE FROM admin_jobs
        WHERE job_id IN (
            SELECT job_id FROM admin_jobs
            WHERE status IN ('completed', 'failed', 'cancelled')
              AND started_at < cutoff_date
            LIMIT batch_size
            FOR UPDATE SKIP LOCKED
        );

        GET DIAGNOSTICS deleted_count = ROW_COUNT;
        COMMIT;

        EXIT WHEN deleted_count < batch_size;
        PERFORM pg_sleep(0.1);
    END LOOP;

    -- 4. 完了/失敗した古いジョブを削除（カスケード削除される）
    LOOP
        DELETE FROM recap_jobs
        WHERE id IN (
            SELECT id FROM recap_jobs
            WHERE status IN ('completed', 'failed')
              AND kicked_at < cutoff_date
              AND NOT EXISTS (
                  SELECT 1 FROM recap_card_snapshots s
                  WHERE s.job_id = recap_jobs.job_id
              )
              AND NOT EXISTS (
                  SELECT 1 FROM recap_card_candidates c
                  WHERE c.job_id = recap_jobs.job_id
              )
              AND NOT EXISTS (
                  SELECT 1 FROM recap_cards rc
                  WHERE rc.job_id = recap_jobs.job_id
              )
              AND NOT EXISTS (
                  SELECT 1 FROM recap_card_job_stats js
                  WHERE js.job_id = recap_jobs.job_id
              )
              AND NOT EXISTS (
                  SELECT 1 FROM recap_eval_windows ew
                  WHERE ew.snapshot_job_id = recap_jobs.job_id
              )
            LIMIT batch_size
            FOR UPDATE SKIP LOCKED
        );

        GET DIAGNOSTICS deleted_count = ROW_COUNT;
        COMMIT;

        EXIT WHEN deleted_count < batch_size;
        PERFORM pg_sleep(0.1);
    END LOOP;

    RAISE NOTICE 'Data retention cleanup completed at %', NOW();
END;
$$;
