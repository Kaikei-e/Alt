use anyhow::{Context, Result};
use chrono::{DateTime, Utc};
use serde_json::Value;
use sqlx::types::Json;
use sqlx::{PgPool, Row};
use uuid::Uuid;

pub(crate) struct RecapDao;

impl RecapDao {
    /// 前処理統計を保存する。
    pub async fn save_preprocess_metrics(
        pool: &PgPool,
        metrics: &crate::store::models::PreprocessMetrics,
    ) -> Result<()> {
        sqlx::query(
            r"
            INSERT INTO recap_preprocess_metrics
                (job_id, total_articles_fetched, articles_processed, articles_dropped_empty,
                 articles_html_cleaned, total_characters, avg_chars_per_article, languages_detected)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
            ON CONFLICT (job_id) DO UPDATE SET
                total_articles_fetched = EXCLUDED.total_articles_fetched,
                articles_processed = EXCLUDED.articles_processed,
                articles_dropped_empty = EXCLUDED.articles_dropped_empty,
                articles_html_cleaned = EXCLUDED.articles_html_cleaned,
                total_characters = EXCLUDED.total_characters,
                avg_chars_per_article = EXCLUDED.avg_chars_per_article,
                languages_detected = EXCLUDED.languages_detected
            ",
        )
        .bind(metrics.job_id)
        .bind(metrics.total_articles_fetched)
        .bind(metrics.articles_processed)
        .bind(metrics.articles_dropped_empty)
        .bind(metrics.articles_html_cleaned)
        .bind(metrics.total_characters)
        .bind(metrics.avg_chars_per_article)
        .bind(&metrics.languages_detected)
        .execute(pool)
        .await
        .context("failed to save preprocess metrics")?;

        Ok(())
    }

    /// システムメトリクスを保存する。
    pub async fn save_system_metrics(
        pool: &PgPool,
        job_id: Uuid,
        metric_type: &str,
        metrics: &Value,
    ) -> Result<()> {
        sqlx::query(
            r"
            INSERT INTO recap_system_metrics (job_id, metric_type, metrics, timestamp)
            VALUES ($1, $2, $3, NOW())
            ",
        )
        .bind(job_id)
        .bind(metric_type)
        .bind(Json(metrics))
        .execute(pool)
        .await
        .context("failed to insert system metrics")?;

        Ok(())
    }

    /// システムメトリクスを取得する（Dashboard用）
    pub async fn get_system_metrics(
        pool: &PgPool,
        metric_type: Option<&str>,
        window_seconds: i64,
        limit: i64,
    ) -> Result<Vec<(Option<Uuid>, DateTime<Utc>, Value)>> {
        let window_seconds = window_seconds.max(0);
        let limit = limit.clamp(1, 1000); // 最大1000件、最小1件

        let query = if let Some(metric_type) = metric_type {
            sqlx::query(
                r"
                SELECT job_id, timestamp, metrics
                FROM recap_system_metrics
                WHERE metric_type = $1
                  AND timestamp > NOW() - ($2 || ' seconds')::interval
                ORDER BY timestamp DESC
                LIMIT $3
                ",
            )
            .bind(metric_type)
            .bind(window_seconds)
            .bind(limit)
        } else {
            sqlx::query(
                r"
                SELECT job_id, timestamp, metrics
                FROM recap_system_metrics
                WHERE timestamp > NOW() - ($1 || ' seconds')::interval
                ORDER BY timestamp DESC
                LIMIT $2
                ",
            )
            .bind(window_seconds)
            .bind(limit)
        };

        let rows = query
            .fetch_all(pool)
            .await
            .context("failed to fetch system metrics")?;

        let mut results = Vec::with_capacity(rows.len());
        for row in rows {
            let job_id: Option<Uuid> = row.try_get("job_id")?;
            let timestamp: DateTime<Utc> = row.try_get("timestamp")?;
            let metrics: Json<Value> = row.try_get("metrics")?;
            results.push((job_id, timestamp, metrics.0));
        }

        Ok(results)
    }

    /// 最近のアクティビティを取得する（Dashboard用）
    pub async fn get_recent_activity(
        pool: &PgPool,
        window_seconds: i64,
        limit: i64,
    ) -> Result<Vec<(Option<Uuid>, String, DateTime<Utc>)>> {
        let window_seconds = window_seconds.max(0);
        let limit = limit.clamp(1, 500); // 最大500件、最小1件

        let rows = sqlx::query(
            r"
            SELECT job_id, metric_type, timestamp
            FROM recap_system_metrics
            WHERE timestamp > NOW() - ($1 || ' seconds')::interval
            ORDER BY timestamp DESC
            LIMIT $2
            ",
        )
        .bind(window_seconds)
        .bind(limit)
        .fetch_all(pool)
        .await
        .context("failed to fetch recent activity")?;

        let mut results = Vec::with_capacity(rows.len());
        for row in rows {
            let job_id: Option<Uuid> = row.try_get("job_id")?;
            let metric_type: String = row.try_get("metric_type")?;
            let timestamp: DateTime<Utc> = row.try_get("timestamp")?;
            results.push((job_id, metric_type, timestamp));
        }

        Ok(results)
    }

    /// エラーログを取得する（Dashboard用）
    pub async fn get_log_errors(
        pool: &PgPool,
        window_seconds: i64,
        limit: i64,
    ) -> Result<
        Vec<(
            DateTime<Utc>,
            String,
            Option<String>,
            Option<String>,
            Option<String>,
        )>,
    > {
        let window_seconds = window_seconds.max(0);
        let limit = limit.clamp(1, 2000); // 最大2000件、最小1件

        let rows = sqlx::query(
            r"
            SELECT timestamp, error_type, error_message, raw_line, service
            FROM log_errors
            WHERE timestamp > NOW() - ($1 || ' seconds')::interval
            ORDER BY timestamp DESC
            LIMIT $2
            ",
        )
        .bind(window_seconds)
        .bind(limit)
        .fetch_all(pool)
        .await
        .context("failed to fetch log errors")?;

        let mut results = Vec::with_capacity(rows.len());
        for row in rows {
            let timestamp: DateTime<Utc> = row.try_get("timestamp")?;
            let error_type: String = row.try_get("error_type")?;
            let error_message: Option<String> = row.try_get("error_message")?;
            let raw_line: Option<String> = row.try_get("raw_line")?;
            let service: Option<String> = row.try_get("service")?;
            results.push((timestamp, error_type, error_message, raw_line, service));
        }

        Ok(results)
    }

    /// 管理ジョブを取得する（Dashboard用）
    pub async fn get_admin_jobs(
        pool: &PgPool,
        window_seconds: i64,
        limit: i64,
    ) -> Result<
        Vec<(
            Uuid,
            String,
            String,
            DateTime<Utc>,
            Option<DateTime<Utc>>,
            Option<Value>,
            Option<Value>,
            Option<String>,
        )>,
    > {
        let window_seconds = window_seconds.max(0);
        let limit = limit.clamp(1, 200); // 最大200件、最小1件

        let rows = sqlx::query(
            r"
            SELECT job_id, kind, status, started_at, finished_at, payload, result, error
            FROM admin_jobs
            WHERE started_at > NOW() - ($1 || ' seconds')::interval
            ORDER BY started_at DESC
            LIMIT $2
            ",
        )
        .bind(window_seconds)
        .bind(limit)
        .fetch_all(pool)
        .await
        .context("failed to fetch admin jobs")?;

        let mut results = Vec::with_capacity(rows.len());
        for row in rows {
            let job_id: Uuid = row.try_get("job_id")?;
            let kind: String = row.try_get("kind")?;
            let status: String = row.try_get("status")?;
            let started_at: DateTime<Utc> = row.try_get("started_at")?;
            let finished_at: Option<DateTime<Utc>> = row.try_get("finished_at")?;
            let payload: Option<Json<Value>> = row.try_get("payload")?;
            let result: Option<Json<Value>> = row.try_get("result")?;
            let error: Option<String> = row.try_get("error")?;
            results.push((
                job_id,
                kind,
                status,
                started_at,
                finished_at,
                payload.map(|j| j.0),
                result.map(|j| j.0),
                error,
            ));
        }

        Ok(results)
    }
}
