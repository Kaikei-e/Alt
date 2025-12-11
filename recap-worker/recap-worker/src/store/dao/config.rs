use anyhow::{Context, Result};
use serde_json::Value;
use sqlx::types::Json;
use sqlx::{PgPool, Row};

pub(crate) struct RecapDao;

impl RecapDao {
    /// 最新のrecap-worker設定を取得する（insert-onlyパターン、最新のものを取得）
    pub async fn get_latest_worker_config(
        pool: &PgPool,
        config_type: &str,
    ) -> Result<Option<Value>> {
        let row = sqlx::query(
            r"
            SELECT config_payload, metadata
            FROM recap_worker_config
            WHERE config_type = $1
            ORDER BY created_at DESC
            LIMIT 1
            ",
        )
        .bind(config_type)
        .fetch_optional(pool)
        .await
        .context("failed to query latest worker config")?;

        if let Some(row) = row {
            let payload: Value = row.try_get("config_payload")?;
            Ok(Some(payload))
        } else {
            Ok(None)
        }
    }

    /// recap-worker設定を保存する（insert-only）
    pub async fn insert_worker_config(
        pool: &PgPool,
        config_type: &str,
        config_payload: &Value,
        source: &str,
        metadata: Option<&Value>,
    ) -> Result<()> {
        sqlx::query(
            r"
            INSERT INTO recap_worker_config (config_type, config_payload, source, metadata)
            VALUES ($1, $2, $3, $4)
            ",
        )
        .bind(config_type)
        .bind(Json(config_payload))
        .bind(source)
        .bind(metadata.map(Json))
        .execute(pool)
        .await
        .context("failed to insert worker config")?;

        Ok(())
    }
}
