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

    // Calculate sub-stage progress for evidence/dispatch stages
    let total_genres = calculate_total_genres(genre_progress.len(), config.recap_genres().len());
    let running_count = genre_progress
        .values()
        .filter(|g| g.status == GenreStatus::Running)
        .count();
    let succeeded_count = genre_progress
        .values()
        .filter(|g| g.status == GenreStatus::Succeeded)
        .count();

    let sub_stage_phase = get_sub_stage_phase(
        job.last_stage.as_deref(),
        !genre_progress.is_empty(),
        running_count > 0,
    );

    let sub_stage_progress = sub_stage_phase.map(|phase| SubStageProgress {
        phase: phase.to_string(),
        total_genres,
        // For evidence_building, we don't have per-genre progress tracking in the database,
        // so completed_genres will be 0. The actual progress is logged via OTel.
        // For clustering/summarization, we use the succeeded count.
        completed_genres: if phase == "evidence_building" {
            0
        } else {
            succeeded_count
        },
    });

    // Derive current_stage and stage_index from actual execution state.
    // The database last_stage is only updated AFTER a stage completes, so during
    // evidence/dispatch execution, we need to infer the actual current stage.
    let (current_stage, effective_stage_index) = match sub_stage_phase {
        Some("evidence_building") => (Some("evidence".to_string()), PipelineStage::Evidence.index()),
        Some("clustering" | "summarization") => {
            (Some("dispatch".to_string()), PipelineStage::Dispatch.index())
        }
        _ => (job.last_stage.clone(), stage_index),
    };

    ActiveJobInfo {
        job_id: job.job_id,
        status: job.status,
        current_stage,
        stage_index: effective_stage_index,
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

/// Calculate total genres for sub-stage progress.
/// Uses actual genre progress count when available (includes subgenres),
/// falls back to config count during evidence stage when no progress yet.
#[inline]
pub(crate) fn calculate_total_genres(genre_progress_len: usize, config_genres_len: usize) -> usize {
    if genre_progress_len == 0 {
        config_genres_len
    } else {
        genre_progress_len
    }
}

/// Determines if the evidence stage is currently running.
///
/// Evidence building is synchronous and fast, so this is unlikely to be caught
/// in progress through the dashboard API. However, we detect it by:
/// - `last_stage == "select"` → evidence stage is starting/running
///
/// Note: In practice, evidence building completes too quickly to observe.
/// Progress is primarily visible through OTel-compliant structured logs.
#[inline]
pub(crate) fn is_evidence_running(last_stage: Option<&str>) -> bool {
    last_stage == Some("select")
}

/// Gets the sub-stage phase based on current stage and progress.
///
/// Returns:
/// - "evidence_building" when evidence stage is active (and no genre progress yet)
/// - "clustering" when dispatch is active with running genres
/// - "summarization" when dispatch is active with no running genres
///
/// Note: Genre progress takes priority over last_stage for phase detection.
/// When genre_progress exists, dispatch is confirmed running even if last_stage
/// hasn't been updated yet.
#[inline]
pub(crate) fn get_sub_stage_phase(
    last_stage: Option<&str>,
    has_genre_progress: bool,
    has_running_genres: bool,
) -> Option<&'static str> {
    // Genre progress existence confirms dispatch is running,
    // regardless of last_stage value (which may not be updated yet)
    if has_genre_progress {
        if has_running_genres {
            Some("clustering")
        } else {
            Some("summarization")
        }
    } else if is_evidence_running(last_stage) {
        // Evidence building only when no genre progress exists
        Some("evidence_building")
    } else {
        None
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_is_evidence_running_when_last_stage_is_select() {
        // When last_stage is "select", evidence stage is starting/running
        assert!(is_evidence_running(Some("select")));
    }

    #[test]
    fn test_is_evidence_not_running_for_other_stages() {
        // Other stages should not trigger evidence running detection
        assert!(!is_evidence_running(Some("evidence")));
        assert!(!is_evidence_running(Some("dispatch")));
        assert!(!is_evidence_running(Some("fetch")));
        assert!(!is_evidence_running(None));
    }

    #[test]
    fn test_get_sub_stage_phase_evidence_building_only_without_genre_progress() {
        // Evidence building should only be returned when no genre progress exists
        assert_eq!(
            get_sub_stage_phase(Some("select"), false, false),
            Some("evidence_building")
        );
    }

    #[test]
    fn test_get_sub_stage_phase_dispatch_takes_priority_over_evidence() {
        // When last_stage is "select" but genre_progress exists,
        // dispatch should take priority over evidence_building
        // (This happens when last_stage hasn't been updated yet but dispatch has started)
        assert_eq!(
            get_sub_stage_phase(Some("select"), true, true),
            Some("clustering")
        );
        assert_eq!(
            get_sub_stage_phase(Some("select"), true, false),
            Some("summarization")
        );
    }

    #[test]
    fn test_get_sub_stage_phase_clustering() {
        // When dispatch is running with running genres, phase should be clustering
        assert_eq!(
            get_sub_stage_phase(Some("dispatch"), true, true),
            Some("clustering")
        );
        assert_eq!(
            get_sub_stage_phase(Some("evidence"), true, true),
            Some("clustering")
        );
    }

    #[test]
    fn test_get_sub_stage_phase_summarization() {
        // When dispatch is running with no running genres, phase should be summarization
        assert_eq!(
            get_sub_stage_phase(Some("dispatch"), true, false),
            Some("summarization")
        );
        assert_eq!(
            get_sub_stage_phase(Some("evidence"), true, false),
            Some("summarization")
        );
    }

    #[test]
    fn test_get_sub_stage_phase_none() {
        // When no sub-stage is active, phase should be None
        assert_eq!(get_sub_stage_phase(Some("fetch"), false, false), None);
        assert_eq!(get_sub_stage_phase(Some("persist"), false, false), None);
        assert_eq!(get_sub_stage_phase(Some("evidence"), false, false), None);
        assert_eq!(get_sub_stage_phase(None, false, false), None);
    }

    #[test]
    fn test_calculate_total_genres_uses_genre_progress_count_when_available() {
        // When genre_progress has entries (including subgenres),
        // total_genres should use genre_progress.len()
        // This ensures "69/69" instead of "69/30"
        assert_eq!(calculate_total_genres(69, 30), 69);
        assert_eq!(calculate_total_genres(45, 30), 45);
        assert_eq!(calculate_total_genres(30, 30), 30);
    }

    #[test]
    fn test_calculate_total_genres_falls_back_to_config_when_no_progress() {
        // When no genre progress exists (e.g., during evidence stage),
        // fall back to config count
        assert_eq!(calculate_total_genres(0, 30), 30);
    }
}
