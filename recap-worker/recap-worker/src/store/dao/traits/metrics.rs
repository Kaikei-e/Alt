//! MetricsDao trait - Metrics and telemetry operations

use anyhow::Result;
use async_trait::async_trait;
use chrono::{DateTime, Utc};
use serde_json::Value;
use uuid::Uuid;

use crate::store::models::PreprocessMetrics;

/// MetricsDao - メトリクス・テレメトリのためのデータアクセス層
#[allow(dead_code)]
#[async_trait]
pub trait MetricsDao: Send + Sync {
    /// 前処理メトリクスを保存する
    async fn save_preprocess_metrics(&self, metrics: &PreprocessMetrics) -> Result<()>;

    /// システムメトリクスを保存する
    async fn save_system_metrics(
        &self,
        job_id: Uuid,
        metric_type: &str,
        metrics: &Value,
    ) -> Result<()>;

    /// システムメトリクスを取得する
    async fn get_system_metrics(
        &self,
        metric_type: Option<&str>,
        window_seconds: i64,
        limit: i64,
    ) -> Result<Vec<(Option<Uuid>, DateTime<Utc>, Value)>>;

    /// 最近のアクティビティを取得する
    async fn get_recent_activity(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> Result<Vec<(Option<Uuid>, String, DateTime<Utc>)>>;

    /// ログエラーを取得する
    async fn get_log_errors(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> Result<Vec<(DateTime<Utc>, String, Option<String>, Option<String>, Option<String>)>>;

    /// 管理用ジョブ一覧を取得する
    async fn get_admin_jobs(
        &self,
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
    >;
}
