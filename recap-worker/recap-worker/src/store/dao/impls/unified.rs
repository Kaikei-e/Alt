//! Unified DAO implementation
//!
//! This module provides a single implementation that combines all DAO traits.
//! It maintains backward compatibility while supporting the new focused trait system.

use anyhow::Result;
use async_trait::async_trait;
use chrono::{DateTime, Utc};
use serde_json::Value;
use sqlx::PgPool;
use std::collections::HashMap;
use uuid::Uuid;

use chrono::NaiveDate;

use crate::pipeline::pulse::PulseResult;
use crate::store::dao::article::FetchedArticleData;
use crate::store::dao::traits::{
    ArticleDao, ConfigDao, EvaluationDao, GenreLearningDao, JobDao, JobStatusDao, MetricsDao,
    MorningDao, OutputDao, PulseDao, StageDao, SubworkerDao,
};
use crate::store::dao::types::{JobStatus, JobStatusTransition, StatusTransitionActor};
use crate::store::models::{
    ClusterWithEvidence, DiagnosticEntry, ExtendedRecapJob, GenreEvaluationMetric,
    GenreEvaluationRun, GenreLearningRecord, GenreWithSummary, GraphEdgeRecord, JobStats,
    NewSubworkerRun, PersistedCluster, PersistedGenre, PreprocessMetrics, PulseGenerationRow,
    RawArticle, RecapFinalSection, RecapJob, RecapOutput, SubworkerRunStatus,
};

/// Unified DAO implementation that combines all DAO traits
#[derive(Debug, Clone)]
pub struct UnifiedDao {
    pool: PgPool,
}

impl UnifiedDao {
    /// Create a new UnifiedDao with the given connection pool
    pub fn new(pool: PgPool) -> Self {
        Self { pool }
    }

    /// Get a reference to the underlying connection pool
    #[allow(dead_code)]
    pub fn pool(&self) -> &PgPool {
        &self.pool
    }
}

// JobDao implementation
#[async_trait]
impl JobDao for UnifiedDao {
    async fn create_job_with_lock(
        &self,
        job_id: Uuid,
        note: Option<&str>,
    ) -> Result<Option<Uuid>> {
        crate::store::dao::job::RecapDao::create_job_with_lock(&self.pool, job_id, note).await
    }

    async fn job_exists(&self, job_id: Uuid) -> Result<bool> {
        crate::store::dao::job::RecapDao::job_exists(&self.pool, job_id).await
    }

    async fn find_resumable_job(&self) -> Result<Option<(Uuid, JobStatus, Option<String>)>> {
        crate::store::dao::job::RecapDao::find_resumable_job(&self.pool).await
    }

    async fn update_job_status(
        &self,
        job_id: Uuid,
        status: JobStatus,
        last_stage: Option<&str>,
    ) -> Result<()> {
        crate::store::dao::job::RecapDao::update_job_status(&self.pool, job_id, status, last_stage)
            .await
    }

    async fn get_recap_jobs(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> Result<Vec<(Uuid, String, Option<String>, DateTime<Utc>, DateTime<Utc>)>> {
        crate::store::dao::job::RecapDao::get_recap_jobs(&self.pool, window_seconds, limit).await
    }

    async fn delete_old_jobs(&self, retention_days: i64) -> Result<u64> {
        crate::store::dao::job::RecapDao::delete_old_jobs(&self.pool, retention_days).await
    }

    async fn record_status_transition(
        &self,
        job_id: Uuid,
        status: JobStatus,
        stage: Option<&str>,
        reason: Option<&str>,
        actor: StatusTransitionActor,
    ) -> Result<i64> {
        crate::store::dao::job::RecapDao::record_status_transition(
            &self.pool, job_id, status, stage, reason, actor,
        )
        .await
    }

    async fn update_job_status_with_history(
        &self,
        job_id: Uuid,
        status: JobStatus,
        last_stage: Option<&str>,
        reason: Option<&str>,
    ) -> Result<()> {
        crate::store::dao::job::RecapDao::update_job_status_with_history(
            &self.pool, job_id, status, last_stage, reason,
        )
        .await
    }

    async fn get_status_history(&self, job_id: Uuid) -> Result<Vec<JobStatusTransition>> {
        crate::store::dao::job::RecapDao::get_status_history(&self.pool, job_id).await
    }
}

// StageDao implementation
#[async_trait]
impl StageDao for UnifiedDao {
    async fn insert_stage_log(
        &self,
        job_id: Uuid,
        stage: &str,
        status: &str,
        message: Option<&str>,
    ) -> Result<()> {
        crate::store::dao::stage::RecapDao::insert_stage_log(
            &self.pool, job_id, stage, status, message,
        )
        .await
    }

    async fn save_stage_state(
        &self,
        job_id: Uuid,
        stage: &str,
        state_data: &Value,
    ) -> Result<()> {
        crate::store::dao::stage::RecapDao::save_stage_state(&self.pool, job_id, stage, state_data)
            .await
    }

    async fn load_stage_state(&self, job_id: Uuid, stage: &str) -> Result<Option<Value>> {
        crate::store::dao::stage::RecapDao::load_stage_state(&self.pool, job_id, stage).await
    }

    async fn insert_failed_task(
        &self,
        job_id: Uuid,
        stage: &str,
        payload: Option<&Value>,
        error: Option<&str>,
    ) -> Result<()> {
        crate::store::dao::stage::RecapDao::insert_failed_task(
            &self.pool, job_id, stage, payload, error,
        )
        .await
    }
}

// ArticleDao implementation
#[async_trait]
impl ArticleDao for UnifiedDao {
    async fn backup_raw_articles(&self, job_id: Uuid, articles: &[RawArticle]) -> Result<()> {
        crate::store::dao::article::RecapDao::backup_raw_articles(&self.pool, job_id, articles)
            .await
    }

    async fn get_article_metadata(
        &self,
        job_id: Uuid,
        article_ids: &[String],
    ) -> Result<HashMap<String, (Option<DateTime<Utc>>, Option<String>)>> {
        crate::store::dao::article::RecapDao::get_article_metadata(&self.pool, job_id, article_ids)
            .await
    }

    async fn get_articles_by_ids(
        &self,
        job_id: Uuid,
        article_ids: &[String],
    ) -> Result<Vec<FetchedArticleData>> {
        crate::store::dao::article::RecapDao::get_articles_by_ids(&self.pool, job_id, article_ids)
            .await
    }
}

// GenreLearningDao implementation
#[async_trait]
impl GenreLearningDao for UnifiedDao {
    async fn load_tag_label_graph(&self, window_label: &str) -> Result<Vec<GraphEdgeRecord>> {
        crate::store::dao::genre_learning::RecapDao::load_tag_label_graph(&self.pool, window_label)
            .await
    }

    async fn upsert_genre_learning_record(&self, record: &GenreLearningRecord) -> Result<()> {
        crate::store::dao::genre_learning::RecapDao::upsert_genre_learning_record(
            &self.pool, record,
        )
        .await
    }

    async fn upsert_genre_learning_records_bulk(
        &self,
        records: &[GenreLearningRecord],
    ) -> Result<()> {
        crate::store::dao::genre_learning::RecapDao::upsert_genre_learning_records_bulk(
            &self.pool, records,
        )
        .await
    }
}

// ConfigDao implementation
#[async_trait]
impl ConfigDao for UnifiedDao {
    async fn get_latest_worker_config(&self, config_type: &str) -> Result<Option<Value>> {
        crate::store::dao::config::RecapDao::get_latest_worker_config(&self.pool, config_type).await
    }

    async fn insert_worker_config(
        &self,
        config_type: &str,
        config_payload: &Value,
        source: &str,
        metadata: Option<&Value>,
    ) -> Result<()> {
        crate::store::dao::config::RecapDao::insert_worker_config(
            &self.pool,
            config_type,
            config_payload,
            source,
            metadata,
        )
        .await
    }
}

// MetricsDao implementation
#[async_trait]
impl MetricsDao for UnifiedDao {
    async fn save_preprocess_metrics(&self, metrics: &PreprocessMetrics) -> Result<()> {
        crate::store::dao::metrics::RecapDao::save_preprocess_metrics(&self.pool, metrics).await
    }

    async fn save_system_metrics(
        &self,
        job_id: Uuid,
        metric_type: &str,
        metrics: &Value,
    ) -> Result<()> {
        crate::store::dao::metrics::RecapDao::save_system_metrics(
            &self.pool, job_id, metric_type, metrics,
        )
        .await
    }

    async fn get_system_metrics(
        &self,
        metric_type: Option<&str>,
        window_seconds: i64,
        limit: i64,
    ) -> Result<Vec<(Option<Uuid>, DateTime<Utc>, Value)>> {
        crate::store::dao::metrics::RecapDao::get_system_metrics(
            &self.pool,
            metric_type,
            window_seconds,
            limit,
        )
        .await
    }

    async fn get_recent_activity(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> Result<Vec<(Option<Uuid>, String, DateTime<Utc>)>> {
        crate::store::dao::metrics::RecapDao::get_recent_activity(&self.pool, window_seconds, limit)
            .await
    }

    async fn get_log_errors(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> Result<Vec<(DateTime<Utc>, String, Option<String>, Option<String>, Option<String>)>> {
        crate::store::dao::metrics::RecapDao::get_log_errors(&self.pool, window_seconds, limit)
            .await
    }

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
    > {
        crate::store::dao::metrics::RecapDao::get_admin_jobs(&self.pool, window_seconds, limit)
            .await
    }
}

// OutputDao implementation
#[async_trait]
impl OutputDao for UnifiedDao {
    async fn save_final_section(&self, section: &RecapFinalSection) -> Result<i64> {
        crate::store::dao::output::RecapDao::save_final_section(&self.pool, section).await
    }

    async fn upsert_recap_output(&self, output: &RecapOutput) -> Result<()> {
        crate::store::dao::output::RecapDao::upsert_recap_output(&self.pool, output).await
    }

    async fn get_recap_output_body_json(
        &self,
        job_id: Uuid,
        genre: &str,
    ) -> Result<Option<Value>> {
        crate::store::dao::output::RecapDao::get_recap_output_body_json(&self.pool, job_id, genre)
            .await
    }

    async fn get_latest_completed_job(&self, window_days: i32) -> Result<Option<RecapJob>> {
        crate::store::dao::output::RecapDao::get_latest_completed_job(&self.pool, window_days).await
    }

    async fn get_genres_by_job(&self, job_id: Uuid) -> Result<Vec<GenreWithSummary>> {
        crate::store::dao::output::RecapDao::get_genres_by_job(&self.pool, job_id).await
    }

    async fn get_clusters_by_job(
        &self,
        job_id: Uuid,
    ) -> Result<HashMap<String, Vec<ClusterWithEvidence>>> {
        crate::store::dao::output::RecapDao::get_clusters_by_job(&self.pool, job_id).await
    }
}

// SubworkerDao implementation
#[async_trait]
impl SubworkerDao for UnifiedDao {
    async fn insert_subworker_run(&self, run: &NewSubworkerRun) -> Result<i64> {
        crate::store::dao::subworker::RecapDao::insert_subworker_run(&self.pool, run).await
    }

    async fn mark_subworker_run_success(
        &self,
        run_id: i64,
        cluster_count: i32,
        response_payload: &Value,
    ) -> Result<()> {
        crate::store::dao::subworker::RecapDao::mark_subworker_run_success(
            &self.pool,
            run_id,
            cluster_count,
            response_payload,
        )
        .await
    }

    async fn mark_subworker_run_failure(
        &self,
        run_id: i64,
        status: SubworkerRunStatus,
        error_message: &str,
    ) -> Result<()> {
        crate::store::dao::subworker::RecapDao::mark_subworker_run_failure(
            &self.pool,
            run_id,
            status,
            error_message,
        )
        .await
    }

    async fn insert_clusters(&self, run_id: i64, clusters: &[PersistedCluster]) -> Result<()> {
        crate::store::dao::subworker::RecapDao::insert_clusters(&self.pool, run_id, clusters).await
    }

    async fn upsert_diagnostics(
        &self,
        run_id: i64,
        diagnostics: &[DiagnosticEntry],
    ) -> Result<()> {
        crate::store::dao::subworker::RecapDao::upsert_diagnostics(&self.pool, run_id, diagnostics)
            .await
    }

    async fn upsert_genre(&self, genre: &PersistedGenre) -> Result<()> {
        crate::store::dao::subworker::RecapDao::upsert_genre(&self.pool, genre).await
    }
}

// EvaluationDao implementation
#[async_trait]
impl EvaluationDao for UnifiedDao {
    async fn save_genre_evaluation(
        &self,
        run: &GenreEvaluationRun,
        metrics: &[GenreEvaluationMetric],
    ) -> Result<()> {
        crate::store::dao::evaluation::RecapDao::save_genre_evaluation(&self.pool, run, metrics)
            .await
    }

    async fn get_genre_evaluation(
        &self,
        run_id: Uuid,
    ) -> Result<Option<(GenreEvaluationRun, Vec<GenreEvaluationMetric>)>> {
        crate::store::dao::evaluation::RecapDao::get_genre_evaluation(&self.pool, run_id).await
    }

    async fn get_latest_genre_evaluation(
        &self,
    ) -> Result<Option<(GenreEvaluationRun, Vec<GenreEvaluationMetric>)>> {
        crate::store::dao::evaluation::RecapDao::get_latest_genre_evaluation(&self.pool).await
    }
}

// MorningDao implementation
#[async_trait]
impl MorningDao for UnifiedDao {
    async fn save_morning_article_groups(&self, groups: &[(Uuid, Uuid, bool)]) -> Result<()> {
        crate::store::dao::morning::RecapDao::save_morning_article_groups(&self.pool, groups).await
    }

    async fn get_morning_article_groups(
        &self,
        since: DateTime<Utc>,
    ) -> Result<Vec<(Uuid, Uuid, bool, DateTime<Utc>)>> {
        crate::store::dao::morning::RecapDao::get_morning_article_groups(&self.pool, since).await
    }
}

// JobStatusDao implementation
#[async_trait]
impl JobStatusDao for UnifiedDao {
    async fn get_extended_jobs(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> Result<Vec<ExtendedRecapJob>> {
        crate::store::dao::job_status::JobStatusDao::get_extended_jobs(
            &self.pool,
            window_seconds,
            limit,
        )
        .await
    }

    async fn get_user_jobs(
        &self,
        user_id: Uuid,
        window_seconds: i64,
        limit: i64,
    ) -> Result<Vec<ExtendedRecapJob>> {
        crate::store::dao::job_status::JobStatusDao::get_user_jobs(
            &self.pool, user_id, window_seconds, limit,
        )
        .await
    }

    async fn get_running_job(&self) -> Result<Option<ExtendedRecapJob>> {
        crate::store::dao::job_status::JobStatusDao::get_running_job(&self.pool).await
    }

    async fn get_job_stats(&self) -> Result<JobStats> {
        crate::store::dao::job_status::JobStatusDao::get_job_stats(&self.pool).await
    }

    async fn get_user_article_count_for_job(&self, job_id: Uuid, user_id: Uuid) -> Result<i32> {
        crate::store::dao::job_status::JobStatusDao::get_user_article_count_for_job(
            &self.pool, job_id, user_id,
        )
        .await
    }

    async fn get_total_article_count_for_job(&self, job_id: Uuid) -> Result<i32> {
        crate::store::dao::job_status::JobStatusDao::get_total_article_count_for_job(
            &self.pool, job_id,
        )
        .await
    }

    async fn get_genre_progress(&self, job_id: Uuid) -> Result<Vec<(String, String, Option<i32>)>> {
        crate::store::dao::job_status::JobStatusDao::get_genre_progress(&self.pool, job_id).await
    }

    async fn get_completed_stages(&self, job_id: Uuid) -> Result<Vec<String>> {
        crate::store::dao::job_status::JobStatusDao::get_completed_stages(&self.pool, job_id).await
    }

    async fn create_user_triggered_job(
        &self,
        job_id: Uuid,
        user_id: Uuid,
        note: Option<&str>,
    ) -> Result<()> {
        crate::store::dao::job_status::JobStatusDao::create_user_triggered_job(
            &self.pool, job_id, user_id, note,
        )
        .await
    }

    async fn get_user_jobs_count(&self, user_id: Uuid, window_seconds: i64) -> Result<i32> {
        crate::store::dao::job_status::JobStatusDao::get_user_jobs_count(
            &self.pool, user_id, window_seconds,
        )
        .await
    }
}

// PulseDao implementation
#[async_trait]
impl PulseDao for UnifiedDao {
    async fn get_pulse_by_date(&self, date: NaiveDate) -> Result<Option<PulseGenerationRow>> {
        crate::store::dao::pulse::RecapDao::get_pulse_by_date(&self.pool, date).await
    }

    async fn get_latest_pulse(&self) -> Result<Option<PulseGenerationRow>> {
        crate::store::dao::pulse::RecapDao::get_latest_pulse(&self.pool).await
    }

    async fn save_pulse_generation(
        &self,
        result: &PulseResult,
        target_date: NaiveDate,
    ) -> Result<i64> {
        crate::store::dao::pulse::RecapDao::save_pulse_generation(&self.pool, result, target_date)
            .await
    }
}
