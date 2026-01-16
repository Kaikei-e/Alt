use anyhow::{Context, Result};
use sqlx::{PgPool, Row};
use uuid::Uuid;

use super::JobStatus;
use crate::util::idempotency::try_acquire_job_lock;

pub(crate) struct RecapDao;

impl RecapDao {
    /// アドバイザリロックを取得し、新しいジョブを作成する。
    ///
    /// ロックが取得できない場合は、既に他のワーカーがそのジョブを実行中であることを示します。
    ///
    /// # Returns
    /// - `Ok(Some(job_id))`: ロック取得成功、ジョブ作成完了
    /// - `Ok(None)`: ロック取得失敗、他のワーカーが実行中
    /// - `Err`: データベースエラー
    #[allow(dead_code)]
    pub async fn create_job_with_lock(
        pool: &PgPool,
        job_id: Uuid,
        note: Option<&str>,
    ) -> Result<Option<Uuid>> {
        let mut tx = pool.begin().await.context("failed to begin transaction")?;

        // Try to acquire advisory lock
        let lock_acquired = try_acquire_job_lock(&mut tx, job_id)
            .await
            .context("failed to acquire advisory lock")?;

        if !lock_acquired {
            // Another worker is already processing this job
            tx.rollback()
                .await
                .context("failed to rollback transaction")?;
            return Ok(None);
        }

        // Create job record
        sqlx::query(
            r"
            INSERT INTO recap_jobs (job_id, kicked_at, note)
            VALUES ($1, NOW(), $2)
            ON CONFLICT (job_id) DO NOTHING
            ",
        )
        .bind(job_id)
        .bind(note)
        .execute(&mut *tx)
        .await
        .context("failed to insert recap_jobs record")?;

        tx.commit().await.context("failed to commit transaction")?;

        Ok(Some(job_id))
    }

    /// 指定されたjob_idのジョブが存在するかチェックする。
    #[allow(dead_code)]
    pub async fn job_exists(pool: &PgPool, job_id: Uuid) -> Result<bool> {
        let row =
            sqlx::query("SELECT EXISTS(SELECT 1 FROM recap_jobs WHERE job_id = $1) as exists")
                .bind(job_id)
                .fetch_one(pool)
                .await
                .context("failed to check job existence")?;

        let exists: bool = row
            .try_get("exists")
            .context("failed to get exists result")?;
        Ok(exists)
    }

    /// 再開可能なジョブ（failedまたはrunningのまま放置されたジョブ）を探す。
    ///
    /// ここでは簡易的に「最新の非完了ジョブ」を返す実装とします。
    pub async fn find_resumable_job(
        pool: &PgPool,
    ) -> Result<Option<(Uuid, JobStatus, Option<String>)>> {
        let row = sqlx::query(
            r"
            SELECT job_id, status, last_stage
            FROM recap_jobs
            WHERE status IN ('pending', 'running', 'failed')
            ORDER BY kicked_at DESC
            LIMIT 1
            ",
        )
        .fetch_optional(pool)
        .await
        .context("failed to find resumable job")?;

        if let Some(row) = row {
            let job_id: Uuid = row.try_get("job_id")?;
            let status_str: String = row.try_get("status")?;
            let last_stage: Option<String> = row.try_get("last_stage")?;

            let status = match status_str.as_str() {
                "pending" => JobStatus::Pending,
                "running" => JobStatus::Running,
                "completed" => JobStatus::Completed,
                _ => JobStatus::Failed, // Default fallback
            };

            Ok(Some((job_id, status, last_stage)))
        } else {
            Ok(None)
        }
    }

    /// ジョブのステータスと最終ステージを更新する。
    pub async fn update_job_status(
        pool: &PgPool,
        job_id: Uuid,
        status: JobStatus,
        last_stage: Option<&str>,
    ) -> Result<()> {
        sqlx::query(
            r"
            UPDATE recap_jobs
            SET status = $2,
            last_stage = COALESCE($3, last_stage),
            updated_at = NOW()
            WHERE job_id = $1
            ",
        )
        .bind(job_id)
        .bind(status.as_ref())
        .bind(last_stage)
        .execute(pool)
        .await
        .context("failed to update job status")?;

        Ok(())
    }

    /// ダッシュボード用に全ジョブを取得する。
    pub async fn get_recap_jobs(
        pool: &PgPool,
        window_seconds: i64,
        limit: i64,
    ) -> Result<
        Vec<(
            Uuid,
            String,
            Option<String>,
            chrono::DateTime<chrono::Utc>,
            chrono::DateTime<chrono::Utc>,
        )>,
    > {
        let rows = sqlx::query(
            r"
            SELECT job_id, status, last_stage, kicked_at, updated_at
            FROM recap_jobs
            WHERE kicked_at > NOW() - make_interval(secs => $1)
            ORDER BY kicked_at DESC
            LIMIT $2
            ",
        )
        .bind(window_seconds as f64)
        .bind(limit)
        .fetch_all(pool)
        .await
        .context("failed to fetch recap jobs")?;

        let mut results = Vec::new();
        for row in rows {
            let job_id: Uuid = row.try_get("job_id")?;
            let status_str: String = row.try_get("status")?;
            let last_stage: Option<String> = row.try_get("last_stage")?;
            let kicked_at: chrono::DateTime<chrono::Utc> = row.try_get("kicked_at")?;
            let updated_at: chrono::DateTime<chrono::Utc> = row.try_get("updated_at")?;
            results.push((job_id, status_str, last_stage, kicked_at, updated_at));
        }

        Ok(results)
    }

    /// 指定された保持期間より古いジョブを削除する。
    ///
    /// CASCADEにより、関連するrecap_job_articles、recap_stage_state等も自動削除される。
    ///
    /// # Arguments
    /// * `pool` - データベース接続プール
    /// * `retention_days` - 保持期間（日数）。この日数より古いジョブが削除対象となる
    ///
    /// # Returns
    /// 削除されたジョブの件数
    pub async fn delete_old_jobs(pool: &PgPool, retention_days: i64) -> Result<u64> {
        let result = sqlx::query(
            r"
            DELETE FROM recap_jobs
            WHERE kicked_at < NOW() - make_interval(days => $1)
            ",
        )
        .bind(retention_days as f64)
        .execute(pool)
        .await
        .context("failed to delete old jobs")?;

        Ok(result.rows_affected())
    }
}
