-- recap-db 初回データクリーンアップスクリプト
--
-- 使用方法:
--   docker compose exec recap-db psql -U $RECAP_DB_USER -d $RECAP_DB_NAME -f /path/to/recap-db-initial-cleanup.sql
--
-- 注意: このスクリプトは大量のデータを削除するため、実行前にバックアップを取ることを推奨します。

-- 保持期間（日数）を設定
\set retention_days 7

-- カットオフ日時を計算
\echo 'Starting initial cleanup...'
\echo 'Retention period: 7 days'

-- 現在の状況を確認
SELECT 'recap_jobs' AS table_name, COUNT(*) AS total_rows,
       COUNT(*) FILTER (WHERE created_at < NOW() - INTERVAL '7 days' AND status IN ('completed', 'failed')) AS rows_to_delete
FROM recap_jobs
UNION ALL
SELECT 'log_errors', COUNT(*),
       COUNT(*) FILTER (WHERE timestamp < NOW() - INTERVAL '7 days')
FROM log_errors
UNION ALL
SELECT 'classification_job_queue', COUNT(*),
       COUNT(*) FILTER (WHERE created_at < NOW() - INTERVAL '7 days' AND status IN ('completed', 'failed'))
FROM classification_job_queue
UNION ALL
SELECT 'admin_jobs', COUNT(*),
       COUNT(*) FILTER (WHERE created_at < NOW() - INTERVAL '7 days' AND status IN ('completed', 'failed', 'cancelled'))
FROM admin_jobs;

\echo ''
\echo 'Press Ctrl+C within 10 seconds to cancel, or wait to continue...'
SELECT pg_sleep(10);

-- バッチ削除を実行
DO $$
DECLARE
    batch_size INTEGER := 1000;
    deleted_count INTEGER;
    total_deleted INTEGER := 0;
    cutoff_date TIMESTAMPTZ := NOW() - INTERVAL '7 days';
BEGIN
    RAISE NOTICE 'Cutoff date: %', cutoff_date;

    -- 1. 完了/失敗した古いジョブを削除（カスケード削除される）
    RAISE NOTICE 'Cleaning up recap_jobs...';
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
        total_deleted := total_deleted + deleted_count;

        IF deleted_count > 0 THEN
            RAISE NOTICE 'Deleted % recap_jobs (total: %)', deleted_count, total_deleted;
        END IF;

        EXIT WHEN deleted_count < batch_size;
        PERFORM pg_sleep(0.1);
    END LOOP;

    RAISE NOTICE 'recap_jobs cleanup completed. Total deleted: %', total_deleted;
    total_deleted := 0;

    -- 2. 古いエラーログを削除
    RAISE NOTICE 'Cleaning up log_errors...';
    LOOP
        DELETE FROM log_errors
        WHERE id IN (
            SELECT id FROM log_errors
            WHERE timestamp < cutoff_date
            LIMIT batch_size
            FOR UPDATE SKIP LOCKED
        );

        GET DIAGNOSTICS deleted_count = ROW_COUNT;
        total_deleted := total_deleted + deleted_count;

        IF deleted_count > 0 THEN
            RAISE NOTICE 'Deleted % log_errors (total: %)', deleted_count, total_deleted;
        END IF;

        EXIT WHEN deleted_count < batch_size;
        PERFORM pg_sleep(0.1);
    END LOOP;

    RAISE NOTICE 'log_errors cleanup completed. Total deleted: %', total_deleted;
    total_deleted := 0;

    -- 3. 完了した分類ジョブキューを削除
    RAISE NOTICE 'Cleaning up classification_job_queue...';
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
        total_deleted := total_deleted + deleted_count;

        IF deleted_count > 0 THEN
            RAISE NOTICE 'Deleted % classification_job_queue (total: %)', deleted_count, total_deleted;
        END IF;

        EXIT WHEN deleted_count < batch_size;
        PERFORM pg_sleep(0.1);
    END LOOP;

    RAISE NOTICE 'classification_job_queue cleanup completed. Total deleted: %', total_deleted;
    total_deleted := 0;

    -- 4. 完了した管理ジョブを削除
    RAISE NOTICE 'Cleaning up admin_jobs...';
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
        total_deleted := total_deleted + deleted_count;

        IF deleted_count > 0 THEN
            RAISE NOTICE 'Deleted % admin_jobs (total: %)', deleted_count, total_deleted;
        END IF;

        EXIT WHEN deleted_count < batch_size;
        PERFORM pg_sleep(0.1);
    END LOOP;

    RAISE NOTICE 'admin_jobs cleanup completed. Total deleted: %', total_deleted;

    RAISE NOTICE 'Initial cleanup completed at %', NOW();
END;
$$;

-- クリーンアップ後の状況を確認
\echo ''
\echo 'Cleanup completed. Current status:'
SELECT 'recap_jobs' AS table_name, COUNT(*) AS total_rows FROM recap_jobs
UNION ALL
SELECT 'recap_job_articles', COUNT(*) FROM recap_job_articles
UNION ALL
SELECT 'recap_subworker_runs', COUNT(*) FROM recap_subworker_runs
UNION ALL
SELECT 'log_errors', COUNT(*) FROM log_errors
UNION ALL
SELECT 'classification_job_queue', COUNT(*) FROM classification_job_queue
UNION ALL
SELECT 'admin_jobs', COUNT(*) FROM admin_jobs;

-- VACUUM ANALYZE を推奨
\echo ''
\echo 'Recommended: Run VACUUM ANALYZE to reclaim disk space:'
\echo '  VACUUM ANALYZE recap_jobs;'
\echo '  VACUUM ANALYZE recap_job_articles;'
\echo '  VACUUM ANALYZE recap_subworker_runs;'
\echo '  VACUUM ANALYZE log_errors;'
\echo '  VACUUM ANALYZE classification_job_queue;'
\echo '  VACUUM ANALYZE admin_jobs;'
