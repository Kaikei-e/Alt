use anyhow::{Context, Result};
use sqlx::{PgPool, Row};
use uuid::Uuid;

use super::JobStatus;
use super::types::{JobStatusTransition, StatusTransitionActor};
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
        Self::create_job_with_lock_and_window(pool, job_id, note, 7).await
    }

    /// アドバイザリロックを取得し、新しいジョブを作成する（ウィンドウ日数指定あり）。
    ///
    /// ロックが取得できない場合は、既に他のワーカーがそのジョブを実行中であることを示します。
    ///
    /// # Returns
    /// - `Ok(Some(job_id))`: ロック取得成功、ジョブ作成完了
    /// - `Ok(None)`: ロック取得失敗、他のワーカーが実行中
    /// - `Err`: データベースエラー
    #[allow(dead_code)]
    pub async fn create_job_with_lock_and_window(
        pool: &PgPool,
        job_id: Uuid,
        note: Option<&str>,
        window_days: u32,
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

        // Create job record with window_days
        sqlx::query(
            r"
            INSERT INTO recap_jobs (job_id, kicked_at, note, window_days)
            VALUES ($1, NOW(), $2, $3)
            ON CONFLICT (job_id) DO NOTHING
            ",
        )
        .bind(job_id)
        .bind(note)
        .bind(i32::try_from(window_days).unwrap_or(7))
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
    ///
    /// # Warning
    /// この関数は、ジョブが存在しない場合でも成功を返しますが、
    /// 警告ログを出力します。これは既存の動作との互換性を保つためです。
    ///
    /// # Note
    /// 新しいコードでは `update_job_status_with_history` を使用してください。
    /// この関数は後方互換性のために残されています。
    #[allow(dead_code)]
    pub async fn update_job_status(
        pool: &PgPool,
        job_id: Uuid,
        status: JobStatus,
        last_stage: Option<&str>,
    ) -> Result<()> {
        let result = sqlx::query(
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

        if result.rows_affected() == 0 {
            tracing::warn!(
                %job_id,
                ?status,
                ?last_stage,
                "update_job_status affected 0 rows - job may not exist or was deleted"
            );
        }

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

    /// ステータス遷移をイミュータブルな履歴テーブルに記録する。
    ///
    /// # Returns
    /// 作成されたレコードのID
    #[allow(dead_code)]
    pub async fn record_status_transition(
        pool: &PgPool,
        job_id: Uuid,
        status: JobStatus,
        stage: Option<&str>,
        reason: Option<&str>,
        actor: StatusTransitionActor,
    ) -> Result<i64> {
        let row = sqlx::query(
            r"
            INSERT INTO recap_job_status_history (job_id, status, stage, reason, actor)
            VALUES ($1, $2, $3, $4, $5)
            RETURNING id
            ",
        )
        .bind(job_id)
        .bind(status.as_ref())
        .bind(stage)
        .bind(reason)
        .bind(actor.as_ref())
        .fetch_one(pool)
        .await
        .context("failed to record status transition")?;

        let id: i64 = row.try_get("id").context("failed to get transition id")?;
        Ok(id)
    }

    /// ジョブのステータスを更新し、同時に履歴テーブルにも記録する。
    /// トランザクションを使用してアトミック性を保証する。
    pub async fn update_job_status_with_history(
        pool: &PgPool,
        job_id: Uuid,
        status: JobStatus,
        last_stage: Option<&str>,
        reason: Option<&str>,
    ) -> Result<()> {
        let mut tx = pool.begin().await.context("failed to begin transaction")?;

        // 1. Insert to immutable history table
        sqlx::query(
            r"
            INSERT INTO recap_job_status_history (job_id, status, stage, reason, actor)
            VALUES ($1, $2, $3, $4, 'system')
            ",
        )
        .bind(job_id)
        .bind(status.as_ref())
        .bind(last_stage)
        .bind(reason)
        .execute(&mut *tx)
        .await
        .context("failed to record status transition in history")?;

        // 2. Update denormalized status on recap_jobs (for backward compatibility)
        let result = sqlx::query(
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
        .execute(&mut *tx)
        .await
        .context("failed to update job status")?;

        if result.rows_affected() == 0 {
            tracing::warn!(
                %job_id,
                ?status,
                ?last_stage,
                "update_job_status_with_history affected 0 rows - job may not exist"
            );
        }

        tx.commit().await.context("failed to commit transaction")?;
        Ok(())
    }

    /// 指定されたジョブのステータス履歴を取得する。
    #[allow(dead_code)]
    pub async fn get_status_history(
        pool: &PgPool,
        job_id: Uuid,
    ) -> Result<Vec<JobStatusTransition>> {
        let rows = sqlx::query(
            r"
            SELECT id, job_id, status, stage, transitioned_at, reason, actor
            FROM recap_job_status_history
            WHERE job_id = $1
            ORDER BY id ASC
            ",
        )
        .bind(job_id)
        .fetch_all(pool)
        .await
        .context("failed to fetch status history")?;

        let mut history = Vec::with_capacity(rows.len());
        for row in rows {
            let status_str: String = row.try_get("status")?;
            let actor_str: String = row.try_get("actor")?;

            let status = match status_str.as_str() {
                "pending" => JobStatus::Pending,
                "running" => JobStatus::Running,
                "completed" => JobStatus::Completed,
                _ => JobStatus::Failed,
            };

            history.push(JobStatusTransition {
                id: row.try_get("id")?,
                job_id: row.try_get("job_id")?,
                status,
                stage: row.try_get("stage")?,
                transitioned_at: row.try_get("transitioned_at")?,
                reason: row.try_get("reason")?,
                actor: StatusTransitionActor::from_str(&actor_str),
            });
        }

        Ok(history)
    }
}
