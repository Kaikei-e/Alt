-- pg_cron拡張の有効化
CREATE EXTENSION IF NOT EXISTS pg_cron;

-- 保持期間設定テーブル
CREATE TABLE IF NOT EXISTS recap_retention_config (
    id SERIAL PRIMARY KEY,
    retention_days INTEGER NOT NULL DEFAULT 7,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

INSERT INTO recap_retention_config (retention_days) VALUES (7)
ON CONFLICT DO NOTHING;

-- バッチ削除プロシージャ
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

    -- 1. 完了/失敗した古いジョブを削除（カスケード削除される）
    LOOP
        DELETE FROM recap_jobs
        WHERE id IN (
            SELECT id FROM recap_jobs
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

    -- 2. 古いエラーログを削除
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

    -- 3. 完了した分類ジョブキューを削除
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

    -- 4. 完了した管理ジョブを削除
    LOOP
        DELETE FROM admin_jobs
        WHERE id IN (
            SELECT id FROM admin_jobs
            WHERE status IN ('completed', 'failed', 'cancelled')
              AND created_at < cutoff_date
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

-- pg_cronジョブをスケジュール（毎日 03:00 JST = 18:00 UTC）
SELECT cron.schedule(
    'recap-data-cleanup',
    '0 18 * * *',
    'CALL cleanup_old_recap_data()'
);
