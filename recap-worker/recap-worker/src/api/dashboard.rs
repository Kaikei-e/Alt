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

#[derive(Debug, Serialize)]
pub(crate) struct RecapJob {
    job_id: Uuid,
    status: String,
    last_stage: Option<String>,
    kicked_at: DateTime<Utc>,
    updated_at: DateTime<Utc>,
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

/// Recapジョブを取得する
pub(crate) async fn get_recap_jobs(
    State(state): State<AppState>,
    Query(params): Query<WindowQuery>,
) -> Result<Json<Vec<RecapJob>>, (StatusCode, String)> {
    let window_seconds = params.window.unwrap_or(14400); // デフォルト4時間
    let limit = params.limit.unwrap_or(200);

    let jobs = state
        .dao()
        .get_recap_jobs(window_seconds, limit)
        .await
        .map_err(|e| {
            tracing::error!(error = %e, "failed to fetch recap jobs");
            (StatusCode::INTERNAL_SERVER_ERROR, e.to_string())
        })?;

    let result: Vec<RecapJob> = jobs
        .into_iter()
        .map(
            |(job_id, status, last_stage, kicked_at, updated_at)| RecapJob {
                job_id,
                status,
                last_stage,
                kicked_at,
                updated_at,
            },
        )
        .collect();

    Ok(Json(result))
}

// ============================================================================
// Job Progress Dashboard Endpoints
// ============================================================================

use crate::config::Config;
use crate::store::dao::{GenreStatus, PipelineStage, RecapDao};
use crate::store::models::{
    ActiveJobInfo, ExtendedRecapJob, GenreProgressInfo, JobProgressEvent, JobStats,
    RecentJobSummary, StatusTransitionResponse, SubStageProgress, UserJobContext,
};
use std::collections::HashMap;
use std::sync::Arc;

#[derive(Debug, Deserialize)]
pub(crate) struct JobProgressQuery {
    user_id: Option<Uuid>,
    window: Option<i64>,
    limit: Option<i64>,
}

/// Build ActiveJobInfo from a running job
async fn build_active_job_info(
    dao: &Arc<dyn RecapDao>,
    config: &Config,
    job: ExtendedRecapJob,
    user_id: Option<Uuid>,
) -> ActiveJobInfo {
    let completed_stages = dao.get_completed_stages(job.job_id).await.unwrap_or_default();
    let genre_progress_raw = dao.get_genre_progress(job.job_id).await.unwrap_or_default();
    let total_articles = dao.get_total_article_count_for_job(job.job_id).await.ok();

    // Build genre progress map
    let mut genre_progress: HashMap<String, GenreProgressInfo> = HashMap::new();
    for (genre, status_str, cluster_count) in genre_progress_raw {
        let status = match status_str.as_str() {
            "running" => GenreStatus::Running,
            "succeeded" => GenreStatus::Succeeded,
            "failed" => GenreStatus::Failed,
            _ => GenreStatus::Pending,
        };
        genre_progress.insert(
            genre,
            GenreProgressInfo {
                status,
                cluster_count,
                article_count: None,
            },
        );
    }

    // Calculate current stage index
    let stage_index = job
        .last_stage
        .as_ref()
        .and_then(|s| PipelineStage::from_str(s))
        .map_or(0, PipelineStage::index);

    // Get user article count if user_id provided
    let user_article_count = if let Some(uid) = user_id {
        dao.get_user_article_count_for_job(job.job_id, uid).await.ok()
    } else {
        None
    };

    // Calculate sub-stage progress for dispatch stage
    let dispatch_running = is_dispatch_running(job.last_stage.as_deref(), !genre_progress.is_empty());

    let sub_stage_progress = if dispatch_running {
        let total_genres = config.recap_genres().len();
        let running_count = genre_progress
            .values()
            .filter(|g| g.status == GenreStatus::Running)
            .count();
        let succeeded_count = genre_progress
            .values()
            .filter(|g| g.status == GenreStatus::Succeeded)
            .count();

        // Determine phase:
        // - If any genre is "running" → clustering phase
        // - If genres have succeeded but dispatch not complete → summarization phase
        let phase = if running_count > 0 {
            "clustering".to_string()
        } else {
            "summarization".to_string()
        };

        Some(SubStageProgress {
            phase,
            total_genres,
            completed_genres: succeeded_count,
        })
    } else {
        None
    };

    ActiveJobInfo {
        job_id: job.job_id,
        status: job.status,
        current_stage: job.last_stage.clone(),
        stage_index,
        stages_completed: completed_stages,
        genre_progress,
        total_articles,
        user_article_count,
        kicked_at: job.kicked_at,
        trigger_source: job.trigger_source,
        sub_stage_progress,
    }
}

/// Enrich recent jobs with status history
async fn enrich_with_status_history(dao: &Arc<dyn RecapDao>, jobs: &mut Vec<RecentJobSummary>) {
    for job in jobs {
        if let Ok(history) = dao.get_status_history(job.job_id).await {
            job.status_history = history
                .into_iter()
                .map(|t| StatusTransitionResponse {
                    id: t.id,
                    status: t.status,
                    stage: t.stage,
                    transitioned_at: t.transitioned_at,
                    reason: t.reason,
                    actor: t.actor.as_ref().to_string(),
                })
                .collect();
        }
    }
}

/// Get comprehensive job progress for dashboard
pub(crate) async fn get_job_progress(
    State(state): State<AppState>,
    Query(params): Query<JobProgressQuery>,
) -> Result<Json<JobProgressEvent>, (StatusCode, String)> {
    let window_seconds = params.window.unwrap_or(86400); // デフォルト24時間
    let limit = params.limit.unwrap_or(50);
    let dao = state.dao();

    // Get running job (if any)
    let running_job = dao.get_running_job().await.map_err(|e| {
        tracing::error!(error = %e, "failed to fetch running job");
        (StatusCode::INTERNAL_SERVER_ERROR, e.to_string())
    })?;

    // Build active job info if there's a running job
    let active_job = if let Some(job) = running_job {
        Some(build_active_job_info(&dao, state.config(), job, params.user_id).await)
    } else {
        None
    };

    // Get recent jobs
    let recent_jobs_data = if let Some(user_id) = params.user_id {
        dao.get_user_jobs(user_id, window_seconds, limit).await
    } else {
        dao.get_extended_jobs(window_seconds, limit).await
    }
    .map_err(|e| {
        tracing::error!(error = %e, "failed to fetch recent jobs");
        (StatusCode::INTERNAL_SERVER_ERROR, e.to_string())
    })?;

    let mut recent_jobs: Vec<RecentJobSummary> =
        recent_jobs_data.into_iter().map(|j| j.to_summary()).collect();

    // Fetch status history for each recent job
    enrich_with_status_history(&dao, &mut recent_jobs).await;

    // Get job stats
    let job_stats = dao.get_job_stats().await.map_err(|e| {
        tracing::error!(error = %e, "failed to fetch job stats");
        (StatusCode::INTERNAL_SERVER_ERROR, e.to_string())
    })?;

    // Build user context if user_id provided
    let user_context = if let Some(user_id) = params.user_id {
        let user_jobs_count = dao
            .get_user_jobs_count(user_id, window_seconds)
            .await
            .unwrap_or(0);

        Some(UserJobContext {
            user_article_count: 0, // TODO: Calculate from favorite_feeds
            user_jobs_count,
            user_feed_ids: Vec::new(), // TODO: Fetch from alt-backend
        })
    } else {
        None
    };

    Ok(Json(JobProgressEvent {
        active_job,
        recent_jobs,
        stats: job_stats,
        user_context,
    }))
}

/// Get job statistics
pub(crate) async fn get_job_stats(
    State(state): State<AppState>,
) -> Result<Json<JobStats>, (StatusCode, String)> {
    let job_stats = state.dao().get_job_stats().await.map_err(|e| {
        tracing::error!(error = %e, "failed to fetch job stats");
        (StatusCode::INTERNAL_SERVER_ERROR, e.to_string())
    })?;

    Ok(Json(job_stats))
}

// ============================================================================
// Helper Functions for Testing
// ============================================================================

/// Determines if the dispatch stage is currently running.
///
/// This logic is extracted for testability. The `last_stage` is only updated
/// AFTER a stage completes, so during dispatch execution it may still show
/// "evidence". We detect dispatch is running by checking if genre_progress
/// has data (which is populated during dispatch).
#[inline]
pub(crate) fn is_dispatch_running(last_stage: Option<&str>, has_genre_progress: bool) -> bool {
    last_stage == Some("dispatch") || (last_stage == Some("evidence") && has_genre_progress)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_is_dispatch_running_when_last_stage_is_dispatch() {
        // When last_stage is "dispatch", dispatch is running regardless of genre_progress
        assert!(is_dispatch_running(Some("dispatch"), false));
        assert!(is_dispatch_running(Some("dispatch"), true));
    }

    #[test]
    fn test_is_dispatch_running_when_last_stage_is_evidence_with_genre_progress() {
        // When last_stage is "evidence" but genre_progress exists,
        // dispatch is actually running (last_stage not yet updated)
        assert!(is_dispatch_running(Some("evidence"), true));
    }

    #[test]
    fn test_is_dispatch_not_running_when_last_stage_is_evidence_without_genre_progress() {
        // When last_stage is "evidence" and no genre_progress,
        // dispatch hasn't started yet
        assert!(!is_dispatch_running(Some("evidence"), false));
    }

    #[test]
    fn test_is_dispatch_not_running_for_other_stages() {
        // Other stages should not trigger dispatch running detection
        assert!(!is_dispatch_running(Some("fetch"), false));
        assert!(!is_dispatch_running(Some("fetch"), true));
        assert!(!is_dispatch_running(Some("preprocess"), false));
        assert!(!is_dispatch_running(Some("persist"), false));
        assert!(!is_dispatch_running(None, false));
        assert!(!is_dispatch_running(None, true));
    }
}
