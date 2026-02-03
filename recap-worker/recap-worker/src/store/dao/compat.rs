//! Backward compatibility layer for RecapDao
//!
//! This module provides backward compatibility with the old `RecapDao` trait
//! by defining it as a supertrait of all focused DAO traits.

use async_trait::async_trait;
use chrono::{DateTime, NaiveDate, Utc};
use serde_json::Value;
use sqlx::PgPool;
use std::collections::HashMap;
use uuid::Uuid;

use super::article::FetchedArticleData;
use super::traits::{
    ArticleDao, ConfigDao, EvaluationDao, GenreLearningDao, JobDao, JobStatusDao, MetricsDao,
    MorningDao, OutputDao, PulseDao, StageDao, SubworkerDao,
};
use super::types::{JobStatus, JobStatusTransition, StatusTransitionActor};
use crate::pipeline::pulse::PulseResult;
use crate::store::models::{
    ClusterWithEvidence, DiagnosticEntry, ExtendedRecapJob, GenreEvaluationMetric,
    GenreEvaluationRun, GenreLearningRecord, GenreWithSummary, GraphEdgeRecord, JobStats,
    NewSubworkerRun, PersistedCluster, PersistedGenre, PreprocessMetrics, PulseGenerationRow,
    RawArticle, RecapFinalSection, RecapJob, RecapOutput, SubworkerRunStatus,
};

/// RecapDao - Backward-compatible composite trait combining all focused DAO traits
///
/// This trait exists for backward compatibility. New code should prefer using
/// specific traits (JobDao, StageDao, etc.) for better modularity.
///
/// # Example (old style - still supported)
/// ```ignore
/// fn process(dao: Arc<dyn RecapDao>) { ... }
/// ```
///
/// # Example (new style - preferred)
/// ```ignore
/// fn process<D: JobDao + StageDao>(dao: Arc<D>) { ... }
/// ```
#[allow(dead_code)]
#[async_trait]
pub trait RecapDao: Send + Sync {
    /// Get the underlying connection pool (if available).
    ///
    /// # Returns
    ///
    /// - `Some(&PgPool)` - If the implementation has a direct connection pool reference
    /// - `None` - For mock implementations or blanket impl (default behavior)
    ///
    /// # Note (LSP Compliance)
    ///
    /// The blanket implementation returns `None` by default. This is intentional:
    /// - Mock DAOs for testing don't have a real database connection
    /// - Code that needs pool access should handle the `None` case gracefully
    /// - For direct pool access, use `RecapDaoImpl::new(pool).pool()` pattern
    ///
    /// Callers should provide fallback behavior when `pool()` returns `None`.
    fn pool(&self) -> Option<&PgPool>;

    // === JobDao methods ===
    async fn create_job_with_lock(
        &self,
        job_id: Uuid,
        note: Option<&str>,
    ) -> anyhow::Result<Option<Uuid>>;

    async fn create_job_with_lock_and_window(
        &self,
        job_id: Uuid,
        note: Option<&str>,
        window_days: u32,
    ) -> anyhow::Result<Option<Uuid>>;

    async fn job_exists(&self, job_id: Uuid) -> anyhow::Result<bool>;

    async fn find_resumable_job(&self)
    -> anyhow::Result<Option<(Uuid, JobStatus, Option<String>)>>;

    async fn update_job_status(
        &self,
        job_id: Uuid,
        status: JobStatus,
        last_stage: Option<&str>,
    ) -> anyhow::Result<()>;

    async fn get_recap_jobs(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> anyhow::Result<Vec<(Uuid, String, Option<String>, DateTime<Utc>, DateTime<Utc>)>>;

    async fn delete_old_jobs(&self, retention_days: i64) -> anyhow::Result<u64>;

    /// ステータス遷移をイミュータブルな履歴テーブルに記録する
    async fn record_status_transition(
        &self,
        job_id: Uuid,
        status: JobStatus,
        stage: Option<&str>,
        reason: Option<&str>,
        actor: StatusTransitionActor,
    ) -> anyhow::Result<i64>;

    /// ジョブのステータスを更新し、同時に履歴テーブルにも記録する（アトミック）
    async fn update_job_status_with_history(
        &self,
        job_id: Uuid,
        status: JobStatus,
        last_stage: Option<&str>,
        reason: Option<&str>,
    ) -> anyhow::Result<()>;

    /// 指定されたジョブのステータス履歴を取得する
    async fn get_status_history(&self, job_id: Uuid) -> anyhow::Result<Vec<JobStatusTransition>>;

    // === StageDao methods ===
    async fn insert_stage_log(
        &self,
        job_id: Uuid,
        stage: &str,
        status: &str,
        message: Option<&str>,
    ) -> anyhow::Result<()>;

    async fn save_stage_state(
        &self,
        job_id: Uuid,
        stage: &str,
        state_data: &Value,
    ) -> anyhow::Result<()>;

    async fn load_stage_state(&self, job_id: Uuid, stage: &str) -> anyhow::Result<Option<Value>>;

    async fn insert_failed_task(
        &self,
        job_id: Uuid,
        stage: &str,
        payload: Option<&Value>,
        error: Option<&str>,
    ) -> anyhow::Result<()>;

    // === ArticleDao methods ===
    async fn backup_raw_articles(
        &self,
        job_id: Uuid,
        articles: &[RawArticle],
    ) -> anyhow::Result<()>;

    async fn get_article_metadata(
        &self,
        job_id: Uuid,
        article_ids: &[String],
    ) -> anyhow::Result<HashMap<String, (Option<DateTime<Utc>>, Option<String>)>>;

    async fn get_articles_by_ids(
        &self,
        job_id: Uuid,
        article_ids: &[String],
    ) -> anyhow::Result<Vec<FetchedArticleData>>;

    // === GenreLearningDao methods ===
    async fn load_tag_label_graph(
        &self,
        window_label: &str,
    ) -> anyhow::Result<Vec<GraphEdgeRecord>>;

    async fn upsert_genre_learning_record(
        &self,
        record: &GenreLearningRecord,
    ) -> anyhow::Result<()>;

    async fn upsert_genre_learning_records_bulk(
        &self,
        records: &[GenreLearningRecord],
    ) -> anyhow::Result<()>;

    // === ConfigDao methods ===
    async fn get_latest_worker_config(&self, config_type: &str) -> anyhow::Result<Option<Value>>;

    async fn insert_worker_config(
        &self,
        config_type: &str,
        config_payload: &Value,
        source: &str,
        metadata: Option<&Value>,
    ) -> anyhow::Result<()>;

    // === MetricsDao methods ===
    async fn save_preprocess_metrics(&self, metrics: &PreprocessMetrics) -> anyhow::Result<()>;

    async fn save_system_metrics(
        &self,
        job_id: Uuid,
        metric_type: &str,
        metrics: &Value,
    ) -> anyhow::Result<()>;

    async fn get_system_metrics(
        &self,
        metric_type: Option<&str>,
        window_seconds: i64,
        limit: i64,
    ) -> anyhow::Result<Vec<(Option<Uuid>, DateTime<Utc>, Value)>>;

    async fn get_recent_activity(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> anyhow::Result<Vec<(Option<Uuid>, String, DateTime<Utc>)>>;

    async fn get_log_errors(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> anyhow::Result<
        Vec<(
            DateTime<Utc>,
            String,
            Option<String>,
            Option<String>,
            Option<String>,
        )>,
    >;

    async fn get_admin_jobs(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> anyhow::Result<
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

    // === OutputDao methods ===
    async fn save_final_section(&self, section: &RecapFinalSection) -> anyhow::Result<i64>;

    async fn upsert_recap_output(&self, output: &RecapOutput) -> anyhow::Result<()>;

    async fn get_recap_output_body_json(
        &self,
        job_id: Uuid,
        genre: &str,
    ) -> anyhow::Result<Option<Value>>;

    async fn get_latest_completed_job(&self, window_days: i32) -> anyhow::Result<Option<RecapJob>>;

    async fn get_genres_by_job(&self, job_id: Uuid) -> anyhow::Result<Vec<GenreWithSummary>>;

    async fn get_clusters_by_job(
        &self,
        job_id: Uuid,
    ) -> anyhow::Result<HashMap<String, Vec<ClusterWithEvidence>>>;

    // === SubworkerDao methods ===
    async fn insert_subworker_run(&self, run: &NewSubworkerRun) -> anyhow::Result<i64>;

    async fn mark_subworker_run_success(
        &self,
        run_id: i64,
        cluster_count: i32,
        response_payload: &Value,
    ) -> anyhow::Result<()>;

    async fn mark_subworker_run_failure(
        &self,
        run_id: i64,
        status: SubworkerRunStatus,
        error_message: &str,
    ) -> anyhow::Result<()>;

    async fn insert_clusters(
        &self,
        run_id: i64,
        clusters: &[PersistedCluster],
    ) -> anyhow::Result<()>;

    async fn upsert_diagnostics(
        &self,
        run_id: i64,
        diagnostics: &[DiagnosticEntry],
    ) -> anyhow::Result<()>;

    async fn upsert_genre(&self, genre: &PersistedGenre) -> anyhow::Result<()>;

    // === EvaluationDao methods ===
    async fn save_genre_evaluation(
        &self,
        run: &GenreEvaluationRun,
        metrics: &[GenreEvaluationMetric],
    ) -> anyhow::Result<()>;

    async fn get_genre_evaluation(
        &self,
        run_id: Uuid,
    ) -> anyhow::Result<Option<(GenreEvaluationRun, Vec<GenreEvaluationMetric>)>>;

    async fn get_latest_genre_evaluation(
        &self,
    ) -> anyhow::Result<Option<(GenreEvaluationRun, Vec<GenreEvaluationMetric>)>>;

    // === MorningDao methods ===
    async fn save_morning_article_groups(
        &self,
        groups: &[(Uuid, Uuid, bool)],
    ) -> anyhow::Result<()>;

    async fn get_morning_article_groups(
        &self,
        since: DateTime<Utc>,
    ) -> anyhow::Result<Vec<(Uuid, Uuid, bool, DateTime<Utc>)>>;

    // === JobStatusDao methods ===
    async fn get_extended_jobs(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> anyhow::Result<Vec<ExtendedRecapJob>>;

    async fn get_user_jobs(
        &self,
        user_id: Uuid,
        window_seconds: i64,
        limit: i64,
    ) -> anyhow::Result<Vec<ExtendedRecapJob>>;

    async fn get_running_job(&self) -> anyhow::Result<Option<ExtendedRecapJob>>;

    async fn get_job_stats(&self) -> anyhow::Result<JobStats>;

    async fn get_user_article_count_for_job(
        &self,
        job_id: Uuid,
        user_id: Uuid,
    ) -> anyhow::Result<i32>;

    async fn get_total_article_count_for_job(&self, job_id: Uuid) -> anyhow::Result<i32>;

    async fn get_genre_progress(
        &self,
        job_id: Uuid,
    ) -> anyhow::Result<Vec<(String, String, Option<i32>)>>;

    async fn get_completed_stages(&self, job_id: Uuid) -> anyhow::Result<Vec<String>>;

    async fn create_user_triggered_job(
        &self,
        job_id: Uuid,
        user_id: Uuid,
        note: Option<&str>,
    ) -> anyhow::Result<()>;

    async fn get_user_jobs_count(&self, user_id: Uuid, window_seconds: i64) -> anyhow::Result<i32>;

    // === PulseDao methods ===
    async fn get_pulse_by_date(
        &self,
        date: NaiveDate,
    ) -> anyhow::Result<Option<PulseGenerationRow>>;

    async fn get_latest_pulse(&self) -> anyhow::Result<Option<PulseGenerationRow>>;

    async fn save_pulse_generation(
        &self,
        result: &PulseResult,
        target_date: NaiveDate,
    ) -> anyhow::Result<i64>;
}

// Blanket implementation: any type implementing all focused traits also implements RecapDao
#[async_trait]
impl<T> RecapDao for T
where
    T: JobDao
        + StageDao
        + ArticleDao
        + GenreLearningDao
        + ConfigDao
        + MetricsDao
        + OutputDao
        + SubworkerDao
        + EvaluationDao
        + MorningDao
        + JobStatusDao
        + PulseDao
        + Send
        + Sync,
{
    fn pool(&self) -> Option<&PgPool> {
        // Default implementation returns None.
        // This is intentional for the blanket impl - see trait documentation.
        // For direct pool access, instantiate RecapDaoImpl directly.
        None
    }

    async fn create_job_with_lock(
        &self,
        job_id: Uuid,
        note: Option<&str>,
    ) -> anyhow::Result<Option<Uuid>> {
        JobDao::create_job_with_lock(self, job_id, note).await
    }

    async fn create_job_with_lock_and_window(
        &self,
        job_id: Uuid,
        note: Option<&str>,
        window_days: u32,
    ) -> anyhow::Result<Option<Uuid>> {
        JobDao::create_job_with_lock_and_window(self, job_id, note, window_days).await
    }

    async fn job_exists(&self, job_id: Uuid) -> anyhow::Result<bool> {
        JobDao::job_exists(self, job_id).await
    }

    async fn find_resumable_job(
        &self,
    ) -> anyhow::Result<Option<(Uuid, JobStatus, Option<String>)>> {
        JobDao::find_resumable_job(self).await
    }

    async fn update_job_status(
        &self,
        job_id: Uuid,
        status: JobStatus,
        last_stage: Option<&str>,
    ) -> anyhow::Result<()> {
        JobDao::update_job_status(self, job_id, status, last_stage).await
    }

    async fn get_recap_jobs(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> anyhow::Result<Vec<(Uuid, String, Option<String>, DateTime<Utc>, DateTime<Utc>)>> {
        JobDao::get_recap_jobs(self, window_seconds, limit).await
    }

    async fn delete_old_jobs(&self, retention_days: i64) -> anyhow::Result<u64> {
        JobDao::delete_old_jobs(self, retention_days).await
    }

    async fn record_status_transition(
        &self,
        job_id: Uuid,
        status: JobStatus,
        stage: Option<&str>,
        reason: Option<&str>,
        actor: StatusTransitionActor,
    ) -> anyhow::Result<i64> {
        JobDao::record_status_transition(self, job_id, status, stage, reason, actor).await
    }

    async fn update_job_status_with_history(
        &self,
        job_id: Uuid,
        status: JobStatus,
        last_stage: Option<&str>,
        reason: Option<&str>,
    ) -> anyhow::Result<()> {
        JobDao::update_job_status_with_history(self, job_id, status, last_stage, reason).await
    }

    async fn get_status_history(&self, job_id: Uuid) -> anyhow::Result<Vec<JobStatusTransition>> {
        JobDao::get_status_history(self, job_id).await
    }

    async fn insert_stage_log(
        &self,
        job_id: Uuid,
        stage: &str,
        status: &str,
        message: Option<&str>,
    ) -> anyhow::Result<()> {
        StageDao::insert_stage_log(self, job_id, stage, status, message).await
    }

    async fn save_stage_state(
        &self,
        job_id: Uuid,
        stage: &str,
        state_data: &Value,
    ) -> anyhow::Result<()> {
        StageDao::save_stage_state(self, job_id, stage, state_data).await
    }

    async fn load_stage_state(&self, job_id: Uuid, stage: &str) -> anyhow::Result<Option<Value>> {
        StageDao::load_stage_state(self, job_id, stage).await
    }

    async fn insert_failed_task(
        &self,
        job_id: Uuid,
        stage: &str,
        payload: Option<&Value>,
        error: Option<&str>,
    ) -> anyhow::Result<()> {
        StageDao::insert_failed_task(self, job_id, stage, payload, error).await
    }

    async fn backup_raw_articles(
        &self,
        job_id: Uuid,
        articles: &[RawArticle],
    ) -> anyhow::Result<()> {
        ArticleDao::backup_raw_articles(self, job_id, articles).await
    }

    async fn get_article_metadata(
        &self,
        job_id: Uuid,
        article_ids: &[String],
    ) -> anyhow::Result<HashMap<String, (Option<DateTime<Utc>>, Option<String>)>> {
        ArticleDao::get_article_metadata(self, job_id, article_ids).await
    }

    async fn get_articles_by_ids(
        &self,
        job_id: Uuid,
        article_ids: &[String],
    ) -> anyhow::Result<Vec<FetchedArticleData>> {
        ArticleDao::get_articles_by_ids(self, job_id, article_ids).await
    }

    async fn load_tag_label_graph(
        &self,
        window_label: &str,
    ) -> anyhow::Result<Vec<GraphEdgeRecord>> {
        GenreLearningDao::load_tag_label_graph(self, window_label).await
    }

    async fn upsert_genre_learning_record(
        &self,
        record: &GenreLearningRecord,
    ) -> anyhow::Result<()> {
        GenreLearningDao::upsert_genre_learning_record(self, record).await
    }

    async fn upsert_genre_learning_records_bulk(
        &self,
        records: &[GenreLearningRecord],
    ) -> anyhow::Result<()> {
        GenreLearningDao::upsert_genre_learning_records_bulk(self, records).await
    }

    async fn get_latest_worker_config(&self, config_type: &str) -> anyhow::Result<Option<Value>> {
        ConfigDao::get_latest_worker_config(self, config_type).await
    }

    async fn insert_worker_config(
        &self,
        config_type: &str,
        config_payload: &Value,
        source: &str,
        metadata: Option<&Value>,
    ) -> anyhow::Result<()> {
        ConfigDao::insert_worker_config(self, config_type, config_payload, source, metadata).await
    }

    async fn save_preprocess_metrics(&self, metrics: &PreprocessMetrics) -> anyhow::Result<()> {
        MetricsDao::save_preprocess_metrics(self, metrics).await
    }

    async fn save_system_metrics(
        &self,
        job_id: Uuid,
        metric_type: &str,
        metrics: &Value,
    ) -> anyhow::Result<()> {
        MetricsDao::save_system_metrics(self, job_id, metric_type, metrics).await
    }

    async fn get_system_metrics(
        &self,
        metric_type: Option<&str>,
        window_seconds: i64,
        limit: i64,
    ) -> anyhow::Result<Vec<(Option<Uuid>, DateTime<Utc>, Value)>> {
        MetricsDao::get_system_metrics(self, metric_type, window_seconds, limit).await
    }

    async fn get_recent_activity(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> anyhow::Result<Vec<(Option<Uuid>, String, DateTime<Utc>)>> {
        MetricsDao::get_recent_activity(self, window_seconds, limit).await
    }

    async fn get_log_errors(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> anyhow::Result<
        Vec<(
            DateTime<Utc>,
            String,
            Option<String>,
            Option<String>,
            Option<String>,
        )>,
    > {
        MetricsDao::get_log_errors(self, window_seconds, limit).await
    }

    async fn get_admin_jobs(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> anyhow::Result<
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
        MetricsDao::get_admin_jobs(self, window_seconds, limit).await
    }

    async fn save_final_section(&self, section: &RecapFinalSection) -> anyhow::Result<i64> {
        OutputDao::save_final_section(self, section).await
    }

    async fn upsert_recap_output(&self, output: &RecapOutput) -> anyhow::Result<()> {
        OutputDao::upsert_recap_output(self, output).await
    }

    async fn get_recap_output_body_json(
        &self,
        job_id: Uuid,
        genre: &str,
    ) -> anyhow::Result<Option<Value>> {
        OutputDao::get_recap_output_body_json(self, job_id, genre).await
    }

    async fn get_latest_completed_job(&self, window_days: i32) -> anyhow::Result<Option<RecapJob>> {
        OutputDao::get_latest_completed_job(self, window_days).await
    }

    async fn get_genres_by_job(&self, job_id: Uuid) -> anyhow::Result<Vec<GenreWithSummary>> {
        OutputDao::get_genres_by_job(self, job_id).await
    }

    async fn get_clusters_by_job(
        &self,
        job_id: Uuid,
    ) -> anyhow::Result<HashMap<String, Vec<ClusterWithEvidence>>> {
        OutputDao::get_clusters_by_job(self, job_id).await
    }

    async fn insert_subworker_run(&self, run: &NewSubworkerRun) -> anyhow::Result<i64> {
        SubworkerDao::insert_subworker_run(self, run).await
    }

    async fn mark_subworker_run_success(
        &self,
        run_id: i64,
        cluster_count: i32,
        response_payload: &Value,
    ) -> anyhow::Result<()> {
        SubworkerDao::mark_subworker_run_success(self, run_id, cluster_count, response_payload)
            .await
    }

    async fn mark_subworker_run_failure(
        &self,
        run_id: i64,
        status: SubworkerRunStatus,
        error_message: &str,
    ) -> anyhow::Result<()> {
        SubworkerDao::mark_subworker_run_failure(self, run_id, status, error_message).await
    }

    async fn insert_clusters(
        &self,
        run_id: i64,
        clusters: &[PersistedCluster],
    ) -> anyhow::Result<()> {
        SubworkerDao::insert_clusters(self, run_id, clusters).await
    }

    async fn upsert_diagnostics(
        &self,
        run_id: i64,
        diagnostics: &[DiagnosticEntry],
    ) -> anyhow::Result<()> {
        SubworkerDao::upsert_diagnostics(self, run_id, diagnostics).await
    }

    async fn upsert_genre(&self, genre: &PersistedGenre) -> anyhow::Result<()> {
        SubworkerDao::upsert_genre(self, genre).await
    }

    async fn save_genre_evaluation(
        &self,
        run: &GenreEvaluationRun,
        metrics: &[GenreEvaluationMetric],
    ) -> anyhow::Result<()> {
        EvaluationDao::save_genre_evaluation(self, run, metrics).await
    }

    async fn get_genre_evaluation(
        &self,
        run_id: Uuid,
    ) -> anyhow::Result<Option<(GenreEvaluationRun, Vec<GenreEvaluationMetric>)>> {
        EvaluationDao::get_genre_evaluation(self, run_id).await
    }

    async fn get_latest_genre_evaluation(
        &self,
    ) -> anyhow::Result<Option<(GenreEvaluationRun, Vec<GenreEvaluationMetric>)>> {
        EvaluationDao::get_latest_genre_evaluation(self).await
    }

    async fn save_morning_article_groups(
        &self,
        groups: &[(Uuid, Uuid, bool)],
    ) -> anyhow::Result<()> {
        MorningDao::save_morning_article_groups(self, groups).await
    }

    async fn get_morning_article_groups(
        &self,
        since: DateTime<Utc>,
    ) -> anyhow::Result<Vec<(Uuid, Uuid, bool, DateTime<Utc>)>> {
        MorningDao::get_morning_article_groups(self, since).await
    }

    async fn get_extended_jobs(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> anyhow::Result<Vec<ExtendedRecapJob>> {
        JobStatusDao::get_extended_jobs(self, window_seconds, limit).await
    }

    async fn get_user_jobs(
        &self,
        user_id: Uuid,
        window_seconds: i64,
        limit: i64,
    ) -> anyhow::Result<Vec<ExtendedRecapJob>> {
        JobStatusDao::get_user_jobs(self, user_id, window_seconds, limit).await
    }

    async fn get_running_job(&self) -> anyhow::Result<Option<ExtendedRecapJob>> {
        JobStatusDao::get_running_job(self).await
    }

    async fn get_job_stats(&self) -> anyhow::Result<JobStats> {
        JobStatusDao::get_job_stats(self).await
    }

    async fn get_user_article_count_for_job(
        &self,
        job_id: Uuid,
        user_id: Uuid,
    ) -> anyhow::Result<i32> {
        JobStatusDao::get_user_article_count_for_job(self, job_id, user_id).await
    }

    async fn get_total_article_count_for_job(&self, job_id: Uuid) -> anyhow::Result<i32> {
        JobStatusDao::get_total_article_count_for_job(self, job_id).await
    }

    async fn get_genre_progress(
        &self,
        job_id: Uuid,
    ) -> anyhow::Result<Vec<(String, String, Option<i32>)>> {
        JobStatusDao::get_genre_progress(self, job_id).await
    }

    async fn get_completed_stages(&self, job_id: Uuid) -> anyhow::Result<Vec<String>> {
        JobStatusDao::get_completed_stages(self, job_id).await
    }

    async fn create_user_triggered_job(
        &self,
        job_id: Uuid,
        user_id: Uuid,
        note: Option<&str>,
    ) -> anyhow::Result<()> {
        JobStatusDao::create_user_triggered_job(self, job_id, user_id, note).await
    }

    async fn get_user_jobs_count(&self, user_id: Uuid, window_seconds: i64) -> anyhow::Result<i32> {
        JobStatusDao::get_user_jobs_count(self, user_id, window_seconds).await
    }

    async fn get_pulse_by_date(
        &self,
        date: NaiveDate,
    ) -> anyhow::Result<Option<PulseGenerationRow>> {
        PulseDao::get_pulse_by_date(self, date).await
    }

    async fn get_latest_pulse(&self) -> anyhow::Result<Option<PulseGenerationRow>> {
        PulseDao::get_latest_pulse(self).await
    }

    async fn save_pulse_generation(
        &self,
        result: &PulseResult,
        target_date: NaiveDate,
    ) -> anyhow::Result<i64> {
        PulseDao::save_pulse_generation(self, result, target_date).await
    }
}
