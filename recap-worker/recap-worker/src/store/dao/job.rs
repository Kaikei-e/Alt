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

    /// 再開可能なジョブ (`pending` / `running` のままプロセスが落ちたジョブ) を探す。
    ///
    /// 起動時にだけ呼ばれる前提で、過去にクラッシュした自プロセスの中断 Job
    /// を 1 件だけ返す。**`failed` は対象外** (一度失敗したものは自動再開せず、
    /// 必要なら次の自動 batch / 手動再 kick で取り直す)。
    ///
    /// `max_age_hours` より古い `kicked_at` は **再開対象から外す**。Recap Job
    /// が数時間掛かることを許容するため、デフォルトは 12h を想定。これより
    /// 古いものはデータ窓が動いてしまい再開しても意味が薄い。
    pub async fn find_resumable_job(
        pool: &PgPool,
        max_age_hours: i64,
    ) -> Result<Option<(Uuid, JobStatus, Option<String>, u32)>> {
        // make_interval() の hours 引数は INT。i32 で bind する。
        // 過去 i32::MAX 時間 (≈ 245k 年) を超える設定は現実的に出ない
        // ので、ここでは飽和キャストで安全側に倒す。
        let max_age_hours_i32 = i32::try_from(max_age_hours).unwrap_or(i32::MAX);
        let row = sqlx::query(
            r"
            SELECT job_id, status, last_stage, window_days
            FROM recap_jobs
            WHERE status IN ('pending', 'running')
              AND kicked_at > NOW() - make_interval(hours => $1)
            ORDER BY kicked_at DESC
            LIMIT 1
            ",
        )
        .bind(max_age_hours_i32)
        .fetch_optional(pool)
        .await
        .context("failed to find resumable job")?;

        if let Some(row) = row {
            let job_id: Uuid = row.try_get("job_id")?;
            let status_str: String = row.try_get("status")?;
            let last_stage: Option<String> = row.try_get("last_stage")?;
            let window_days: Option<i32> = row.try_get("window_days")?;
            let window_days = window_days.unwrap_or(7).cast_unsigned();

            let status = match status_str.as_str() {
                "pending" => JobStatus::Pending,
                "running" => JobStatus::Running,
                "completed" => JobStatus::Completed,
                _ => JobStatus::Failed, // Default fallback
            };

            Ok(Some((job_id, status, last_stage, window_days)))
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
    /// 放置された `pending` / `running` ジョブを `failed` に確定させる。
    /// `kicked_at` が `max_age_hours` より古いものを対象に、status と
    /// `error_message` を一括更新し、変更件数を返す。
    ///
    /// 起動直後と一定間隔で叩くことを想定。これがないと、過去のクラッシュで
    /// `pending` / `running` のまま残った行が dashboard に居座り続け、
    /// `find_resumable_job` も同じ行を毎回掴んで失敗を繰り返す原因になる。
    /// Boot 時の Job 衛生: `pending` / `running` のまま残っている全行を
    /// `failed` に確定させる。**プロセスが起動し直したということは前プロセス
    /// は死んでいる** ので、in-flight だった Job は定義上全て orphan。
    /// `keep_job_id` を指定するとそのジョブだけは sweep 対象から外す
    /// (resume 候補として残しておくため)。
    pub async fn mark_abandoned_jobs(
        pool: &PgPool,
        keep_job_id: Option<Uuid>,
    ) -> Result<u64> {
        let result = match keep_job_id {
            Some(keep) => {
                sqlx::query(
                    r"
                    UPDATE recap_jobs
                    SET status = 'failed',
                        last_stage = COALESCE(last_stage, 'abandoned_at_startup'),
                        updated_at = NOW()
                    WHERE status IN ('pending', 'running')
                      AND job_id <> $1
                    ",
                )
                .bind(keep)
                .execute(pool)
                .await
            }
            None => {
                sqlx::query(
                    r"
                    UPDATE recap_jobs
                    SET status = 'failed',
                        last_stage = COALESCE(last_stage, 'abandoned_at_startup'),
                        updated_at = NOW()
                    WHERE status IN ('pending', 'running')
                    ",
                )
                .execute(pool)
                .await
            }
        }
        .context("failed to mark abandoned recap jobs")?;

        Ok(result.rows_affected())
    }

    pub async fn delete_old_jobs(pool: &PgPool, retention_days: i64) -> Result<u64> {
        // Bug fix (2026-04-13): make_interval(days => $1) requires INT,
        // but the original implementation bound $1 as f64, which Postgres
        // rejects with `function make_interval(days => double precision)
        // does not exist`. Cast safely to i32 (≈ 5.8M years headroom).
        let retention_days_i32 = i32::try_from(retention_days).unwrap_or(i32::MAX);
        let result = sqlx::query(
            r"
            DELETE FROM recap_jobs
            WHERE kicked_at < NOW() - make_interval(days => $1)
            ",
        )
        .bind(retention_days_i32)
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
