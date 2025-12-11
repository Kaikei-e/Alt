use anyhow::{Context, Result};
use serde_json::Value;
use sqlx::types::Json;
use sqlx::{PgPool, Row};
use uuid::Uuid;

pub(crate) struct RecapDao;

impl RecapDao {
    /// ステージの実行ログを記録する。
    pub async fn insert_stage_log(
        pool: &PgPool,
        job_id: Uuid,
        stage: &str,
        status: &str,
        message: Option<&str>,
    ) -> Result<()> {
        sqlx::query(
            r"
            INSERT INTO recap_job_stage_logs (job_id, stage, status, message)
            VALUES ($1, $2, $3, $4)
            ",
        )
        .bind(job_id)
        .bind(stage)
        .bind(status)
        .bind(message)
        .execute(pool)
        .await
        .context("failed to insert stage log")?;

        Ok(())
    }

    /// ステージの状態（チェックポイント）を保存する。
    pub async fn save_stage_state(
        pool: &PgPool,
        job_id: Uuid,
        stage: &str,
        state_data: &Value,
    ) -> Result<()> {
        sqlx::query(
            r"
            INSERT INTO recap_stage_state (job_id, stage, state)
            VALUES ($1, $2, $3)
            ON CONFLICT (job_id, stage) DO UPDATE SET
                state = EXCLUDED.state,
                created_at = NOW()
            ",
        )
        .bind(job_id)
        .bind(stage)
        .bind(Json(state_data))
        .execute(pool)
        .await
        .context("failed to save stage state")?;

        Ok(())
    }

    /// ステージの状態（チェックポイント）を読み込む。
    pub async fn load_stage_state(
        pool: &PgPool,
        job_id: Uuid,
        stage: &str,
    ) -> Result<Option<Value>> {
        let row = sqlx::query(
            r"
            SELECT state FROM recap_stage_state
            WHERE job_id = $1 AND stage = $2
            ",
        )
        .bind(job_id)
        .bind(stage)
        .fetch_optional(pool)
        .await
        .context("failed to load stage state")?;

        if let Some(row) = row {
            let state_json: Json<Value> = row.try_get("state")?;
            Ok(Some(state_json.0))
        } else {
            Ok(None)
        }
    }

    /// 失敗したタスクを記録する。
    pub async fn insert_failed_task(
        pool: &PgPool,
        job_id: Uuid,
        stage: &str,
        payload: Option<&Value>,
        error: Option<&str>,
    ) -> Result<()> {
        sqlx::query(
            r"
            INSERT INTO recap_failed_tasks (job_id, stage, payload, error)
            VALUES ($1, $2, $3, $4)
            ",
        )
        .bind(job_id)
        .bind(stage)
        .bind(payload.map(Json))
        .bind(error)
        .execute(pool)
        .await
        .context("failed to insert failed task")?;

        Ok(())
    }
}
