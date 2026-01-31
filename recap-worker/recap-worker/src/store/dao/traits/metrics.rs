//! MetricsDao trait - Metrics and telemetry operations

use std::future::Future;

use anyhow::Result;
use chrono::{DateTime, Utc};
use serde_json::Value;
use uuid::Uuid;

use crate::store::models::PreprocessMetrics;

/// MetricsDao - メトリクス・テレメトリのためのデータアクセス層
#[allow(dead_code, clippy::type_complexity)]
pub trait MetricsDao: Send + Sync {
    /// 前処理メトリクスを保存する
    fn save_preprocess_metrics(
        &self,
        metrics: &PreprocessMetrics,
    ) -> impl Future<Output = Result<()>> + Send;

    /// システムメトリクスを保存する
    fn save_system_metrics(
        &self,
        job_id: Uuid,
        metric_type: &str,
        metrics: &Value,
    ) -> impl Future<Output = Result<()>> + Send;

    /// システムメトリクスを取得する
    fn get_system_metrics(
        &self,
        metric_type: Option<&str>,
        window_seconds: i64,
        limit: i64,
    ) -> impl Future<Output = Result<Vec<(Option<Uuid>, DateTime<Utc>, Value)>>> + Send;

    /// 最近のアクティビティを取得する
    fn get_recent_activity(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> impl Future<Output = Result<Vec<(Option<Uuid>, String, DateTime<Utc>)>>> + Send;

    /// ログエラーを取得する
    fn get_log_errors(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> impl Future<
        Output = Result<
            Vec<(
                DateTime<Utc>,
                String,
                Option<String>,
                Option<String>,
                Option<String>,
            )>,
        >,
    > + Send;

    /// 管理用ジョブ一覧を取得する
    fn get_admin_jobs(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> impl Future<
        Output = Result<
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
        >,
    > + Send;
}
