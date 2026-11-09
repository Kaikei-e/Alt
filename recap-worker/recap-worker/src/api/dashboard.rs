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

const MAX_DASHBOARD_LIMIT: i64 = 200;
const MAX_DASHBOARD_WINDOW_SECONDS: i64 = 90 * 86_400;

#[inline]
pub(crate) fn clamp_limit(limit: Option<i64>, default: i64) -> i64 {
    limit.unwrap_or(default).clamp(1, MAX_DASHBOARD_LIMIT)
}

#[inline]
pub(crate) fn clamp_window(window: Option<i64>, default: i64) -> i64 {
    window
        .unwrap_or(default)
        .clamp(1, MAX_DASHBOARD_WINDOW_SECONDS)
}

/// システムメトリクスを取得する
pub(crate) async fn get_metrics(
    State(state): State<AppState>,
    Query(params): Query<MetricsQuery>,
) -> Result<Json<Vec<SystemMetric>>, (StatusCode, String)> {
    let window_seconds = clamp_window(params.window, 14400); // デフォルト4時間
    let limit = clamp_limit(params.limit, 200);

    let metrics = state
        .dao()
        .get_system_metrics(params.metric_type.as_deref(), window_seconds, limit)
        .await
        .map_err(|e| {
            tracing::error!(error = %e, "failed to fetch system metrics");
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                "failed to fetch system metrics".to_string(),
            )
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
    let window_seconds = clamp_window(params.window, 14400); // デフォルト4時間
    let limit = clamp_limit(params.limit, 200);

    let activities = state
        .dao()
        .get_recent_activity(window_seconds, limit)
        .await
        .map_err(|e| {
            tracing::error!(error = %e, "failed to fetch recent activity");
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                "failed to fetch recent activity".to_string(),
            )
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
    let window_seconds = clamp_window(params.window, 14400); // デフォルト4時間
    let limit = clamp_limit(params.limit, 200);

    let logs = state
        .dao()
        .get_log_errors(window_seconds, limit)
        .await
        .map_err(|e| {
            tracing::error!(error = %e, "failed to fetch log errors");
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                "failed to fetch log errors".to_string(),
            )
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
    let window_seconds = clamp_window(params.window, 14400); // デフォルト4時間
    let limit = clamp_limit(params.limit, 200);

    let jobs = state
        .dao()
        .get_admin_jobs(window_seconds, limit)
        .await
        .map_err(|e| {
            tracing::error!(error = %e, "failed to fetch admin jobs");
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                "failed to fetch admin jobs".to_string(),
            )
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
    let window_seconds = clamp_window(params.window, 14400); // デフォルト4時間
    let limit = clamp_limit(params.limit, 200);

    let jobs = state
        .dao()
        .get_recap_jobs(window_seconds, limit)
        .await
        .map_err(|e| {
            tracing::error!(error = %e, "failed to fetch recap jobs");
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                "failed to fetch recap jobs".to_string(),
            )
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
    let completed_stages = dao
        .get_completed_stages(job.job_id)
        .await
        .unwrap_or_default();
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
        dao.get_user_article_count_for_job(job.job_id, uid)
            .await
            .ok()
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
        Some("evidence_building") => (
            Some("evidence".to_string()),
            PipelineStage::Evidence.index(),
        ),
        Some("clustering" | "summarization") => (
            Some("dispatch".to_string()),
            PipelineStage::Dispatch.index(),
        ),
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
    let window_seconds = clamp_window(params.window, 86400); // デフォルト24時間
    let limit = clamp_limit(params.limit, 50);
    let dao = state.dao();

    // Get running job (if any)
    let running_job = dao.get_running_job().await.map_err(|e| {
        tracing::error!(error = %e, "failed to fetch running job");
        (
            StatusCode::INTERNAL_SERVER_ERROR,
            "failed to fetch running job".to_string(),
        )
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
        (
            StatusCode::INTERNAL_SERVER_ERROR,
            "failed to fetch recent jobs".to_string(),
        )
    })?;

    let mut recent_jobs: Vec<RecentJobSummary> = recent_jobs_data
        .into_iter()
        .map(|j| j.to_summary())
        .collect();

    // Fetch status history for each recent job
    enrich_with_status_history(&dao, &mut recent_jobs).await;

    // Get job stats
    let job_stats = dao.get_job_stats().await.map_err(|e| {
        tracing::error!(error = %e, "failed to fetch job stats");
        (
            StatusCode::INTERNAL_SERVER_ERROR,
            "failed to fetch job stats".to_string(),
        )
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
        (
            StatusCode::INTERNAL_SERVER_ERROR,
            "failed to fetch job stats".to_string(),
        )
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

    #[test]
    fn test_clamp_limit_enforces_bounds() {
        assert_eq!(clamp_limit(None, 50), 50);
        assert_eq!(clamp_limit(Some(0), 50), 1);
        assert_eq!(clamp_limit(Some(-100), 50), 1);
        assert_eq!(clamp_limit(Some(100), 50), 100);
        assert_eq!(clamp_limit(Some(10_000), 50), 200);
    }

    #[test]
    fn test_clamp_window_enforces_bounds() {
        assert_eq!(clamp_window(None, 14_400), 14_400);
        assert_eq!(clamp_window(Some(0), 14_400), 1);
        assert_eq!(clamp_window(Some(-100), 14_400), 1);
        assert_eq!(clamp_window(Some(3_600), 14_400), 3_600);
        assert_eq!(
            clamp_window(Some(i64::MAX), 14_400),
            MAX_DASHBOARD_WINDOW_SECONDS
        );
    }

    #[derive(Default)]
    struct WindowRecordingDao {
        inner: crate::store::dao::mock::MockRecapDao,
        recorded_windows: std::sync::Mutex<Vec<i64>>,
    }

    impl WindowRecordingDao {
        fn recorded_windows(&self) -> Vec<i64> {
            self.recorded_windows
                .lock()
                .expect("recorded_windows mutex")
                .clone()
        }
    }

    #[async_trait::async_trait]
    impl RecapDao for WindowRecordingDao {
        fn pool(&self) -> Option<&sqlx::PgPool> {
            self.inner.pool()
        }

        async fn create_job_with_lock(
            &self,
            job_id: Uuid,
            note: Option<&str>,
        ) -> crate::error::Result<Option<Uuid>> {
            self.inner.create_job_with_lock(job_id, note).await
        }

        async fn create_job_with_lock_and_window(
            &self,
            job_id: Uuid,
            note: Option<&str>,
            window_days: u32,
            trigger_source: &str,
        ) -> crate::error::Result<Option<Uuid>> {
            self.inner
                .create_job_with_lock_and_window(job_id, note, window_days, trigger_source)
                .await
        }

        async fn job_exists(&self, job_id: Uuid) -> crate::error::Result<bool> {
            self.inner.job_exists(job_id).await
        }

        async fn find_resumable_job(
            &self,
            max_age_hours: i64,
        ) -> crate::error::Result<Option<(Uuid, crate::store::dao::JobStatus, Option<String>, u32)>>
        {
            self.inner.find_resumable_job(max_age_hours).await
        }

        async fn find_most_recent_failed_job(
            &self,
        ) -> crate::error::Result<Option<(Uuid, crate::store::dao::JobStatus, Option<String>, u32)>>
        {
            self.inner.find_most_recent_failed_job().await
        }

        async fn mark_abandoned_jobs(
            &self,
            keep_job_id: Option<Uuid>,
        ) -> crate::error::Result<u64> {
            self.inner.mark_abandoned_jobs(keep_job_id).await
        }

        async fn find_running_cards_job(&self) -> crate::error::Result<Option<Uuid>> {
            self.inner.find_running_cards_job().await
        }

        async fn update_job_status(
            &self,
            job_id: Uuid,
            status: crate::store::dao::JobStatus,
            last_stage: Option<&str>,
        ) -> crate::error::Result<()> {
            self.inner
                .update_job_status(job_id, status, last_stage)
                .await
        }

        async fn get_recap_jobs(
            &self,
            window_seconds: i64,
            limit: i64,
        ) -> crate::error::Result<Vec<(Uuid, String, Option<String>, DateTime<Utc>, DateTime<Utc>)>>
        {
            self.recorded_windows
                .lock()
                .expect("recorded_windows mutex")
                .push(window_seconds);
            if !(1..=MAX_DASHBOARD_WINDOW_SECONDS).contains(&window_seconds) {
                return Err(crate::error::RecapError::Db(
                    "interval out of range".to_string(),
                ));
            }
            self.inner.get_recap_jobs(window_seconds, limit).await
        }

        async fn delete_old_jobs(&self, retention_days: i64) -> crate::error::Result<u64> {
            self.inner.delete_old_jobs(retention_days).await
        }

        async fn record_status_transition(
            &self,
            job_id: Uuid,
            status: crate::store::dao::JobStatus,
            stage: Option<&str>,
            reason: Option<&str>,
            actor: crate::store::dao::types::StatusTransitionActor,
        ) -> crate::error::Result<i64> {
            self.inner
                .record_status_transition(job_id, status, stage, reason, actor)
                .await
        }

        async fn update_job_status_with_history(
            &self,
            job_id: Uuid,
            status: crate::store::dao::JobStatus,
            last_stage: Option<&str>,
            reason: Option<&str>,
        ) -> crate::error::Result<()> {
            self.inner
                .update_job_status_with_history(job_id, status, last_stage, reason)
                .await
        }

        async fn get_status_history(
            &self,
            job_id: Uuid,
        ) -> crate::error::Result<Vec<crate::store::dao::types::JobStatusTransition>> {
            self.inner.get_status_history(job_id).await
        }

        async fn insert_stage_log(
            &self,
            job_id: Uuid,
            stage: &str,
            status: &str,
            message: Option<&str>,
        ) -> crate::error::Result<()> {
            self.inner
                .insert_stage_log(job_id, stage, status, message)
                .await
        }

        async fn save_stage_state(
            &self,
            job_id: Uuid,
            stage: &str,
            state_data: &Value,
        ) -> crate::error::Result<()> {
            self.inner.save_stage_state(job_id, stage, state_data).await
        }

        async fn load_stage_state(
            &self,
            job_id: Uuid,
            stage: &str,
        ) -> crate::error::Result<Option<Value>> {
            self.inner.load_stage_state(job_id, stage).await
        }

        async fn insert_failed_task(
            &self,
            job_id: Uuid,
            stage: &str,
            payload: Option<&Value>,
            error: Option<&str>,
        ) -> crate::error::Result<()> {
            self.inner
                .insert_failed_task(job_id, stage, payload, error)
                .await
        }

        async fn backup_raw_articles(
            &self,
            job_id: Uuid,
            articles: &[crate::store::models::RawArticle],
        ) -> crate::error::Result<()> {
            self.inner.backup_raw_articles(job_id, articles).await
        }

        async fn get_article_metadata(
            &self,
            job_id: Uuid,
            article_ids: &[String],
        ) -> crate::error::Result<HashMap<String, (Option<DateTime<Utc>>, Option<String>)>>
        {
            self.inner.get_article_metadata(job_id, article_ids).await
        }

        async fn get_articles_by_ids(
            &self,
            job_id: Uuid,
            article_ids: &[String],
        ) -> crate::error::Result<Vec<crate::store::dao::article::FetchedArticleData>> {
            self.inner.get_articles_by_ids(job_id, article_ids).await
        }

        async fn load_tag_label_graph(
            &self,
            window_label: &str,
        ) -> crate::error::Result<Vec<crate::store::models::GraphEdgeRecord>> {
            self.inner.load_tag_label_graph(window_label).await
        }

        async fn upsert_genre_learning_record(
            &self,
            record: &crate::store::models::GenreLearningRecord,
        ) -> crate::error::Result<()> {
            self.inner.upsert_genre_learning_record(record).await
        }

        async fn upsert_genre_learning_records_bulk(
            &self,
            records: &[crate::store::models::GenreLearningRecord],
        ) -> crate::error::Result<()> {
            self.inner.upsert_genre_learning_records_bulk(records).await
        }

        async fn get_latest_worker_config(
            &self,
            config_type: &str,
        ) -> crate::error::Result<Option<Value>> {
            self.inner.get_latest_worker_config(config_type).await
        }

        async fn insert_worker_config(
            &self,
            config_type: &str,
            config_payload: &Value,
            source: &str,
            metadata: Option<&Value>,
        ) -> crate::error::Result<()> {
            self.inner
                .insert_worker_config(config_type, config_payload, source, metadata)
                .await
        }

        async fn save_preprocess_metrics(
            &self,
            metrics: &crate::store::models::PreprocessMetrics,
        ) -> crate::error::Result<()> {
            self.inner.save_preprocess_metrics(metrics).await
        }

        async fn save_system_metrics(
            &self,
            job_id: Uuid,
            metric_type: &str,
            metrics: &Value,
        ) -> crate::error::Result<()> {
            self.inner
                .save_system_metrics(job_id, metric_type, metrics)
                .await
        }

        async fn get_system_metrics(
            &self,
            metric_type: Option<&str>,
            window_seconds: i64,
            limit: i64,
        ) -> crate::error::Result<Vec<(Option<Uuid>, DateTime<Utc>, Value)>> {
            self.inner
                .get_system_metrics(metric_type, window_seconds, limit)
                .await
        }

        async fn get_recent_activity(
            &self,
            window_seconds: i64,
            limit: i64,
        ) -> crate::error::Result<Vec<(Option<Uuid>, String, DateTime<Utc>)>> {
            self.inner.get_recent_activity(window_seconds, limit).await
        }

        async fn get_log_errors(
            &self,
            window_seconds: i64,
            limit: i64,
        ) -> crate::error::Result<
            Vec<(
                DateTime<Utc>,
                String,
                Option<String>,
                Option<String>,
                Option<String>,
            )>,
        > {
            self.inner.get_log_errors(window_seconds, limit).await
        }

        async fn get_admin_jobs(
            &self,
            window_seconds: i64,
            limit: i64,
        ) -> crate::error::Result<
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
            self.inner.get_admin_jobs(window_seconds, limit).await
        }

        async fn save_final_section(
            &self,
            section: &crate::store::models::RecapFinalSection,
        ) -> crate::error::Result<i64> {
            self.inner.save_final_section(section).await
        }

        async fn upsert_recap_output(
            &self,
            output: &crate::store::models::RecapOutput,
        ) -> crate::error::Result<()> {
            self.inner.upsert_recap_output(output).await
        }

        async fn persist_genre_output(
            &self,
            output: &crate::store::models::RecapOutput,
            genre: &crate::store::models::PersistedGenre,
        ) -> crate::error::Result<()> {
            self.inner.persist_genre_output(output, genre).await
        }

        async fn get_sentence_ids_by_run(
            &self,
            run_id: i64,
        ) -> crate::error::Result<HashMap<String, Vec<i64>>> {
            self.inner.get_sentence_ids_by_run(run_id).await
        }

        async fn get_recap_output_body_json(
            &self,
            job_id: Uuid,
            genre: &str,
        ) -> crate::error::Result<Option<Value>> {
            self.inner.get_recap_output_body_json(job_id, genre).await
        }

        async fn get_latest_completed_job(
            &self,
            window_days: i32,
        ) -> crate::error::Result<Option<crate::store::models::RecapJob>> {
            self.inner.get_latest_completed_job(window_days).await
        }

        async fn get_genres_by_job(
            &self,
            job_id: Uuid,
        ) -> crate::error::Result<Vec<crate::store::models::GenreWithSummary>> {
            self.inner.get_genres_by_job(job_id).await
        }

        async fn get_clusters_by_job(
            &self,
            job_id: Uuid,
        ) -> crate::error::Result<HashMap<String, Vec<crate::store::models::ClusterWithEvidence>>>
        {
            self.inner.get_clusters_by_job(job_id).await
        }

        async fn search_recaps_by_term(
            &self,
            term: &str,
            limit: i32,
        ) -> crate::error::Result<Vec<crate::store::models::RecapSearchHit>> {
            self.inner.search_recaps_by_term(term, limit).await
        }

        async fn insert_subworker_run(
            &self,
            run: &crate::store::models::NewSubworkerRun,
        ) -> crate::error::Result<i64> {
            self.inner.insert_subworker_run(run).await
        }

        async fn mark_subworker_run_success(
            &self,
            run_id: i64,
            cluster_count: i32,
            response_payload: &Value,
        ) -> crate::error::Result<()> {
            self.inner
                .mark_subworker_run_success(run_id, cluster_count, response_payload)
                .await
        }

        async fn mark_subworker_run_failure(
            &self,
            run_id: i64,
            status: crate::store::models::SubworkerRunStatus,
            error_message: &str,
        ) -> crate::error::Result<()> {
            self.inner
                .mark_subworker_run_failure(run_id, status, error_message)
                .await
        }

        async fn insert_clusters(
            &self,
            run_id: i64,
            clusters: &[crate::store::models::PersistedCluster],
        ) -> crate::error::Result<()> {
            self.inner.insert_clusters(run_id, clusters).await
        }

        async fn upsert_diagnostics(
            &self,
            run_id: i64,
            diagnostics: &[crate::store::models::DiagnosticEntry],
        ) -> crate::error::Result<()> {
            self.inner.upsert_diagnostics(run_id, diagnostics).await
        }

        async fn upsert_genre(
            &self,
            genre: &crate::store::models::PersistedGenre,
        ) -> crate::error::Result<()> {
            self.inner.upsert_genre(genre).await
        }

        async fn save_genre_evaluation(
            &self,
            run: &crate::store::models::GenreEvaluationRun,
            metrics: &[crate::store::models::GenreEvaluationMetric],
        ) -> crate::error::Result<()> {
            self.inner.save_genre_evaluation(run, metrics).await
        }

        async fn get_genre_evaluation(
            &self,
            run_id: Uuid,
        ) -> crate::error::Result<
            Option<(
                crate::store::models::GenreEvaluationRun,
                Vec<crate::store::models::GenreEvaluationMetric>,
            )>,
        > {
            self.inner.get_genre_evaluation(run_id).await
        }

        async fn get_latest_genre_evaluation(
            &self,
        ) -> crate::error::Result<
            Option<(
                crate::store::models::GenreEvaluationRun,
                Vec<crate::store::models::GenreEvaluationMetric>,
            )>,
        > {
            self.inner.get_latest_genre_evaluation().await
        }

        async fn save_morning_article_groups(
            &self,
            groups: &[(Uuid, Uuid, bool)],
        ) -> crate::error::Result<()> {
            self.inner.save_morning_article_groups(groups).await
        }

        async fn get_morning_article_groups(
            &self,
            since: DateTime<Utc>,
        ) -> crate::error::Result<Vec<(Uuid, Uuid, bool, DateTime<Utc>)>> {
            self.inner.get_morning_article_groups(since).await
        }

        async fn save_morning_letter(
            &self,
            letter: &crate::store::models::MorningLetter,
        ) -> crate::error::Result<Uuid> {
            self.inner.save_morning_letter(letter).await
        }

        async fn save_morning_letter_sources(
            &self,
            sources: &[crate::store::models::MorningLetterSource],
        ) -> crate::error::Result<()> {
            self.inner.save_morning_letter_sources(sources).await
        }

        async fn get_morning_letter_by_date(
            &self,
            date: chrono::NaiveDate,
        ) -> crate::error::Result<Option<crate::store::models::MorningLetter>> {
            self.inner.get_morning_letter_by_date(date).await
        }

        async fn get_latest_morning_letter(
            &self,
        ) -> crate::error::Result<Option<crate::store::models::MorningLetter>> {
            self.inner.get_latest_morning_letter().await
        }

        async fn get_morning_letter_sources(
            &self,
            letter_id: Uuid,
        ) -> crate::error::Result<Vec<crate::store::models::MorningLetterSource>> {
            self.inner.get_morning_letter_sources(letter_id).await
        }

        async fn get_previous_morning_letter(
            &self,
            edition_timezone: &str,
            before: chrono::NaiveDate,
        ) -> crate::error::Result<Option<crate::store::models::MorningLetter>> {
            self.inner
                .get_previous_morning_letter(edition_timezone, before)
                .await
        }

        async fn get_extended_jobs(
            &self,
            window_seconds: i64,
            limit: i64,
        ) -> crate::error::Result<Vec<ExtendedRecapJob>> {
            self.inner.get_extended_jobs(window_seconds, limit).await
        }

        async fn get_user_jobs(
            &self,
            user_id: Uuid,
            window_seconds: i64,
            limit: i64,
        ) -> crate::error::Result<Vec<ExtendedRecapJob>> {
            self.inner
                .get_user_jobs(user_id, window_seconds, limit)
                .await
        }

        async fn get_running_job(&self) -> crate::error::Result<Option<ExtendedRecapJob>> {
            self.inner.get_running_job().await
        }

        async fn get_job_stats(&self) -> crate::error::Result<JobStats> {
            self.inner.get_job_stats().await
        }

        async fn get_user_article_count_for_job(
            &self,
            job_id: Uuid,
            user_id: Uuid,
        ) -> crate::error::Result<i32> {
            self.inner
                .get_user_article_count_for_job(job_id, user_id)
                .await
        }

        async fn get_total_article_count_for_job(&self, job_id: Uuid) -> crate::error::Result<i32> {
            self.inner.get_total_article_count_for_job(job_id).await
        }

        async fn get_genre_progress(
            &self,
            job_id: Uuid,
        ) -> crate::error::Result<Vec<(String, String, Option<i32>)>> {
            self.inner.get_genre_progress(job_id).await
        }

        async fn get_completed_stages(&self, job_id: Uuid) -> crate::error::Result<Vec<String>> {
            self.inner.get_completed_stages(job_id).await
        }

        async fn create_user_triggered_job(
            &self,
            job_id: Uuid,
            user_id: Uuid,
            note: Option<&str>,
        ) -> crate::error::Result<()> {
            self.inner
                .create_user_triggered_job(job_id, user_id, note)
                .await
        }

        async fn get_user_jobs_count(
            &self,
            user_id: Uuid,
            window_seconds: i64,
        ) -> crate::error::Result<i32> {
            self.inner
                .get_user_jobs_count(user_id, window_seconds)
                .await
        }

        async fn get_pulse_by_date(
            &self,
            date: chrono::NaiveDate,
        ) -> crate::error::Result<Option<crate::store::models::PulseGenerationRow>> {
            self.inner.get_pulse_by_date(date).await
        }

        async fn get_latest_pulse(
            &self,
        ) -> crate::error::Result<Option<crate::store::models::PulseGenerationRow>> {
            self.inner.get_latest_pulse().await
        }

        async fn save_pulse_generation(
            &self,
            result: &crate::pipeline::pulse::PulseResult,
            target_date: chrono::NaiveDate,
        ) -> crate::error::Result<(i64, chrono::DateTime<chrono::Utc>)> {
            self.inner.save_pulse_generation(result, target_date).await
        }
    }

    #[tokio::test]
    #[allow(clippy::await_holding_lock)]
    async fn get_recap_jobs_clamps_huge_window_instead_of_returning_500() {
        use crate::{
            app::{ComponentRegistry, build_router},
            config::ENV_MUTEX,
        };
        use axum::{body::Body, http::Request};
        use tower::ServiceExt;

        let _lock = ENV_MUTEX
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        let registry = temp_env::async_with_vars(
            [
                (
                    "RECAP_DB_DSN",
                    Some("postgres://recap:recap@localhost:15432/recap"),
                ),
                ("NEWS_CREATOR_BASE_URL", Some("http://localhost:18001/")),
                ("SUBWORKER_BASE_URL", Some("http://localhost:18002/")),
                ("ALT_BACKEND_BASE_URL", Some("http://localhost:19000/")),
                ("RECAP_KNOWLEDGE_EMIT", Some("false")),
                ("RECAP_ADMIN_AUTH", Some("disabled")),
                ("RECAP_EVAL_LISTENER", Some("disabled")),
                ("RECAP_CARDS_JOB", Some("disabled")),
                ("RECAP_GENRES", Some("ai,space")),
                ("RECAP_PRE_REFRESH_GRAPH_ENABLED", Some("false")),
                (
                    "HUGGING_FACE_TOKEN_PATH",
                    Some("/tmp/test-token-which-does-not-exist"),
                ),
                ("TOKEN_COUNTER_ALLOW_DUMMY_FALLBACK", Some("true")),
            ],
            async {
                let config = Config::from_env().expect("config loads");
                ComponentRegistry::build(config)
                    .await
                    .expect("registry builds")
            },
        )
        .await;

        let dao = Arc::new(WindowRecordingDao::default());
        let registry = registry.with_recap_dao(dao.clone());
        let app = build_router(registry);

        let uri = format!("/v1/dashboard/recap_jobs?window={}", i64::MAX);
        let request = Request::get(uri)
            .body(Body::empty())
            .expect("request builds");
        let response = app.oneshot(request).await.expect("request succeeds");

        assert_eq!(response.status(), StatusCode::OK);
        assert_eq!(dao.recorded_windows(), vec![MAX_DASHBOARD_WINDOW_SECONDS]);
    }
}
