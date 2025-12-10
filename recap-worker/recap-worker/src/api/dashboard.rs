use axum::{
    Json,
    extract::{Query, State},
    http::StatusCode,
};
use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use uuid::Uuid;

use crate::app::AppState;

#[derive(Debug, Deserialize)]
pub(crate) struct MetricsQuery {
    #[serde(rename = "type")]
    metric_type: Option<String>,
    window: Option<i64>,
    limit: Option<i64>,
}

#[derive(Debug, Serialize)]
pub(crate) struct SystemMetric {
    job_id: Option<Uuid>,
    timestamp: DateTime<Utc>,
    metrics: Value,
}

#[derive(Debug, Serialize)]
pub(crate) struct RecentActivity {
    job_id: Option<Uuid>,
    metric_type: String,
    timestamp: DateTime<Utc>,
}

#[derive(Debug, Serialize)]
pub(crate) struct LogError {
    timestamp: DateTime<Utc>,
    error_type: String,
    error_message: Option<String>,
    raw_line: Option<String>,
    service: Option<String>,
}

#[derive(Debug, Serialize)]
pub(crate) struct AdminJob {
    job_id: Uuid,
    kind: String,
    status: String,
    started_at: DateTime<Utc>,
    finished_at: Option<DateTime<Utc>>,
    payload: Option<Value>,
    result: Option<Value>,
    error: Option<String>,
}

#[derive(Debug, Deserialize)]
pub(crate) struct WindowQuery {
    window: Option<i64>,
    limit: Option<i64>,
}

/// システムメトリクスを取得する
pub(crate) async fn get_metrics(
    State(state): State<AppState>,
    Query(params): Query<MetricsQuery>,
) -> Result<Json<Vec<SystemMetric>>, (StatusCode, String)> {
    let window_seconds = params.window.unwrap_or(14400); // デフォルト4時間
    let limit = params.limit.unwrap_or(500);

    let metrics = state
        .dao()
        .get_system_metrics(params.metric_type.as_deref(), window_seconds, limit)
        .await
        .map_err(|e| {
            tracing::error!(error = %e, "failed to fetch system metrics");
            (StatusCode::INTERNAL_SERVER_ERROR, e.to_string())
        })?;

    let result: Vec<SystemMetric> = metrics
        .into_iter()
        .map(|(job_id, timestamp, metrics)| SystemMetric {
            job_id,
            timestamp,
            metrics,
        })
        .collect();

    Ok(Json(result))
}

/// 最近のアクティビティを取得する
pub(crate) async fn get_overview(
    State(state): State<AppState>,
    Query(params): Query<WindowQuery>,
) -> Result<Json<Vec<RecentActivity>>, (StatusCode, String)> {
    let window_seconds = params.window.unwrap_or(14400); // デフォルト4時間
    let limit = params.limit.unwrap_or(200);

    let activities = state
        .dao()
        .get_recent_activity(window_seconds, limit)
        .await
        .map_err(|e| {
            tracing::error!(error = %e, "failed to fetch recent activity");
            (StatusCode::INTERNAL_SERVER_ERROR, e.to_string())
        })?;

    let result: Vec<RecentActivity> = activities
        .into_iter()
        .map(|(job_id, metric_type, timestamp)| RecentActivity {
            job_id,
            metric_type,
            timestamp,
        })
        .collect();

    Ok(Json(result))
}

/// エラーログを取得する
pub(crate) async fn get_logs(
    State(state): State<AppState>,
    Query(params): Query<WindowQuery>,
) -> Result<Json<Vec<LogError>>, (StatusCode, String)> {
    let window_seconds = params.window.unwrap_or(14400); // デフォルト4時間
    let limit = params.limit.unwrap_or(2000);

    let logs = state
        .dao()
        .get_log_errors(window_seconds, limit)
        .await
        .map_err(|e| {
            tracing::error!(error = %e, "failed to fetch log errors");
            (StatusCode::INTERNAL_SERVER_ERROR, e.to_string())
        })?;

    let result: Vec<LogError> = logs
        .into_iter()
        .map(
            |(timestamp, error_type, error_message, raw_line, service)| LogError {
                timestamp,
                error_type,
                error_message,
                raw_line,
                service,
            },
        )
        .collect();

    Ok(Json(result))
}

/// 管理ジョブを取得する
pub(crate) async fn get_jobs(
    State(state): State<AppState>,
    Query(params): Query<WindowQuery>,
) -> Result<Json<Vec<AdminJob>>, (StatusCode, String)> {
    let window_seconds = params.window.unwrap_or(14400); // デフォルト4時間
    let limit = params.limit.unwrap_or(200);

    let jobs = state
        .dao()
        .get_admin_jobs(window_seconds, limit)
        .await
        .map_err(|e| {
            tracing::error!(error = %e, "failed to fetch admin jobs");
            (StatusCode::INTERNAL_SERVER_ERROR, e.to_string())
        })?;

    let result: Vec<AdminJob> = jobs
        .into_iter()
        .map(
            |(job_id, kind, status, started_at, finished_at, payload, result, error)| AdminJob {
                job_id,
                kind,
                status,
                started_at,
                finished_at,
                payload,
                result,
                error,
            },
        )
        .collect();

    Ok(Json(result))
}
