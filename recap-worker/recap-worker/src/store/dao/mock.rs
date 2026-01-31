// テスト用のモックRecapDao実装
// プロダクションコードから分離して、テスト専用のモックを提供

#[cfg(test)]
use anyhow::Result;
#[cfg(test)]
use async_trait::async_trait;
#[cfg(test)]
use sqlx::PgPool;
#[cfg(test)]
use uuid::Uuid;

use super::article;
use super::compat::RecapDao;
use super::types::{JobStatus, JobStatusTransition, StatusTransitionActor};
use crate::pipeline::pulse::PulseResult;
use crate::store::models::{
    ClusterWithEvidence, DiagnosticEntry, ExtendedRecapJob, GenreEvaluationMetric,
    GenreEvaluationRun, GenreLearningRecord, GenreWithSummary, GraphEdgeRecord, JobStats,
    NewSubworkerRun, PersistedCluster, PersistedGenre, PulseGenerationRow, RawArticle, RecapJob,
    SubworkerRunStatus,
};

#[cfg(test)]
/// テスト用のモックRecapDao（DB接続なしで動作）
#[allow(dead_code)]
#[derive(Clone)]
pub(crate) struct MockRecapDao;

#[cfg(test)]
impl MockRecapDao {
    #[allow(dead_code)]
    pub(crate) fn new() -> Self {
        Self
    }
}

#[cfg(test)]
#[async_trait]
impl RecapDao for MockRecapDao {
    fn pool(&self) -> Option<&PgPool> {
        // モックではデータベース接続プールは不要
        None
    }

    // Job management
    #[allow(dead_code)]
    async fn create_job_with_lock(
        &self,
        _job_id: Uuid,
        _note: Option<&str>,
    ) -> Result<Option<Uuid>> {
        Ok(None)
    }

    #[allow(dead_code)]
    async fn job_exists(&self, _job_id: Uuid) -> Result<bool> {
        Ok(false)
    }

    async fn find_resumable_job(&self) -> Result<Option<(Uuid, JobStatus, Option<String>)>> {
        Ok(None)
    }

    async fn update_job_status(
        &self,
        _job_id: Uuid,
        _status: JobStatus,
        _last_stage: Option<&str>,
    ) -> Result<()> {
        Ok(())
    }

    async fn get_recap_jobs(
        &self,
        _window_seconds: i64,
        _limit: i64,
    ) -> Result<
        Vec<(
            Uuid,
            String,
            Option<String>,
            chrono::DateTime<chrono::Utc>,
            chrono::DateTime<chrono::Utc>,
        )>,
    > {
        Ok(vec![])
    }

    async fn delete_old_jobs(&self, _retention_days: i64) -> Result<u64> {
        Ok(0)
    }

    async fn record_status_transition(
        &self,
        _job_id: Uuid,
        _status: JobStatus,
        _stage: Option<&str>,
        _reason: Option<&str>,
        _actor: StatusTransitionActor,
    ) -> Result<i64> {
        Ok(1)
    }

    async fn update_job_status_with_history(
        &self,
        _job_id: Uuid,
        _status: JobStatus,
        _last_stage: Option<&str>,
        _reason: Option<&str>,
    ) -> Result<()> {
        Ok(())
    }

    async fn get_status_history(&self, _job_id: Uuid) -> Result<Vec<JobStatusTransition>> {
        Ok(vec![])
    }

    // Stage management
    async fn insert_stage_log(
        &self,
        _job_id: Uuid,
        _stage: &str,
        _status: &str,
        _message: Option<&str>,
    ) -> Result<()> {
        Ok(())
    }

    async fn save_stage_state(
        &self,
        _job_id: Uuid,
        _stage: &str,
        _state_data: &serde_json::Value,
    ) -> Result<()> {
        Ok(())
    }

    async fn load_stage_state(
        &self,
        _job_id: Uuid,
        _stage: &str,
    ) -> Result<Option<serde_json::Value>> {
        Ok(None)
    }

    async fn insert_failed_task(
        &self,
        _job_id: Uuid,
        _stage: &str,
        _payload: Option<&serde_json::Value>,
        _error: Option<&str>,
    ) -> Result<()> {
        Ok(())
    }

    // Article management
    async fn backup_raw_articles(&self, _job_id: Uuid, _articles: &[RawArticle]) -> Result<()> {
        Ok(())
    }

    async fn get_article_metadata(
        &self,
        _job_id: Uuid,
        _article_ids: &[String],
    ) -> Result<
        std::collections::HashMap<String, (Option<chrono::DateTime<chrono::Utc>>, Option<String>)>,
    > {
        Ok(std::collections::HashMap::new())
    }

    async fn get_articles_by_ids(
        &self,
        _job_id: Uuid,
        _article_ids: &[String],
    ) -> Result<Vec<article::FetchedArticleData>> {
        Ok(vec![])
    }

    // Genre learning
    async fn load_tag_label_graph(&self, _window_label: &str) -> Result<Vec<GraphEdgeRecord>> {
        Ok(vec![])
    }

    async fn upsert_genre_learning_record(&self, _record: &GenreLearningRecord) -> Result<()> {
        Ok(())
    }

    async fn upsert_genre_learning_records_bulk(
        &self,
        _records: &[GenreLearningRecord],
    ) -> Result<()> {
        Ok(())
    }

    // Config
    async fn get_latest_worker_config(
        &self,
        _config_type: &str,
    ) -> Result<Option<serde_json::Value>> {
        Ok(None)
    }

    async fn insert_worker_config(
        &self,
        _config_type: &str,
        _config_payload: &serde_json::Value,
        _source: &str,
        _metadata: Option<&serde_json::Value>,
    ) -> Result<()> {
        Ok(())
    }

    // Metrics
    async fn save_preprocess_metrics(
        &self,
        _metrics: &crate::store::models::PreprocessMetrics,
    ) -> Result<()> {
        Ok(())
    }

    async fn save_system_metrics(
        &self,
        _job_id: Uuid,
        _metric_type: &str,
        _metrics: &serde_json::Value,
    ) -> Result<()> {
        Ok(())
    }

    async fn get_system_metrics(
        &self,
        _metric_type: Option<&str>,
        _window_seconds: i64,
        _limit: i64,
    ) -> Result<
        Vec<(
            Option<Uuid>,
            chrono::DateTime<chrono::Utc>,
            serde_json::Value,
        )>,
    > {
        Ok(vec![])
    }

    async fn get_recent_activity(
        &self,
        _window_seconds: i64,
        _limit: i64,
    ) -> Result<Vec<(Option<Uuid>, String, chrono::DateTime<chrono::Utc>)>> {
        Ok(vec![])
    }

    async fn get_log_errors(
        &self,
        _window_seconds: i64,
        _limit: i64,
    ) -> Result<
        Vec<(
            chrono::DateTime<chrono::Utc>,
            String,
            Option<String>,
            Option<String>,
            Option<String>,
        )>,
    > {
        Ok(vec![])
    }

    async fn get_admin_jobs(
        &self,
        _window_seconds: i64,
        _limit: i64,
    ) -> Result<
        Vec<(
            Uuid,
            String,
            String,
            chrono::DateTime<chrono::Utc>,
            Option<chrono::DateTime<chrono::Utc>>,
            Option<serde_json::Value>,
            Option<serde_json::Value>,
            Option<String>,
        )>,
    > {
        Ok(vec![])
    }

    // Output
    #[allow(dead_code)]
    async fn save_final_section(
        &self,
        _section: &crate::store::models::RecapFinalSection,
    ) -> Result<i64> {
        Ok(0)
    }

    #[allow(dead_code)]
    async fn upsert_recap_output(&self, _output: &crate::store::models::RecapOutput) -> Result<()> {
        Ok(())
    }

    async fn get_recap_output_body_json(
        &self,
        _job_id: Uuid,
        _genre: &str,
    ) -> Result<Option<serde_json::Value>> {
        Ok(None)
    }

    async fn get_latest_completed_job(&self, _window_days: i32) -> Result<Option<RecapJob>> {
        Ok(None)
    }

    async fn get_genres_by_job(&self, _job_id: Uuid) -> Result<Vec<GenreWithSummary>> {
        Ok(vec![])
    }

    async fn get_clusters_by_job(
        &self,
        _job_id: Uuid,
    ) -> Result<std::collections::HashMap<String, Vec<ClusterWithEvidence>>> {
        Ok(std::collections::HashMap::new())
    }

    // Subworker
    #[allow(dead_code)]
    async fn insert_subworker_run(&self, _run: &NewSubworkerRun) -> Result<i64> {
        Ok(0)
    }

    #[allow(dead_code)]
    async fn mark_subworker_run_success(
        &self,
        _run_id: i64,
        _cluster_count: i32,
        _response_payload: &serde_json::Value,
    ) -> Result<()> {
        Ok(())
    }

    #[allow(dead_code)]
    async fn mark_subworker_run_failure(
        &self,
        _run_id: i64,
        _status: SubworkerRunStatus,
        _error_message: &str,
    ) -> Result<()> {
        Ok(())
    }

    #[allow(dead_code)]
    async fn insert_clusters(&self, _run_id: i64, _clusters: &[PersistedCluster]) -> Result<()> {
        Ok(())
    }

    #[allow(dead_code)]
    async fn upsert_diagnostics(
        &self,
        _run_id: i64,
        _diagnostics: &[DiagnosticEntry],
    ) -> Result<()> {
        Ok(())
    }

    #[allow(dead_code)]
    async fn upsert_genre(&self, _genre: &PersistedGenre) -> Result<()> {
        Ok(())
    }

    // Evaluation
    async fn save_genre_evaluation(
        &self,
        _run: &GenreEvaluationRun,
        _metrics: &[GenreEvaluationMetric],
    ) -> Result<()> {
        Ok(())
    }

    #[allow(clippy::too_many_lines)]
    async fn get_genre_evaluation(
        &self,
        _run_id: Uuid,
    ) -> Result<Option<(GenreEvaluationRun, Vec<GenreEvaluationMetric>)>> {
        Ok(None)
    }

    async fn get_latest_genre_evaluation(
        &self,
    ) -> Result<Option<(GenreEvaluationRun, Vec<GenreEvaluationMetric>)>> {
        Ok(None)
    }

    // Morning
    async fn save_morning_article_groups(&self, _groups: &[(Uuid, Uuid, bool)]) -> Result<()> {
        Ok(())
    }

    async fn get_morning_article_groups(
        &self,
        _since: chrono::DateTime<chrono::Utc>,
    ) -> Result<Vec<(Uuid, Uuid, bool, chrono::DateTime<chrono::Utc>)>> {
        Ok(vec![])
    }

    // Job Status Dashboard
    async fn get_extended_jobs(
        &self,
        _window_seconds: i64,
        _limit: i64,
    ) -> Result<Vec<ExtendedRecapJob>> {
        Ok(vec![])
    }

    async fn get_user_jobs(
        &self,
        _user_id: Uuid,
        _window_seconds: i64,
        _limit: i64,
    ) -> Result<Vec<ExtendedRecapJob>> {
        Ok(vec![])
    }

    async fn get_running_job(&self) -> Result<Option<ExtendedRecapJob>> {
        Ok(None)
    }

    async fn get_job_stats(&self) -> Result<JobStats> {
        Ok(JobStats {
            success_rate_24h: 0.0,
            avg_duration_secs: None,
            total_jobs_24h: 0,
            running_jobs: 0,
            failed_jobs_24h: 0,
        })
    }

    async fn get_user_article_count_for_job(
        &self,
        _job_id: Uuid,
        _user_id: Uuid,
    ) -> Result<i32> {
        Ok(0)
    }

    async fn get_total_article_count_for_job(&self, _job_id: Uuid) -> Result<i32> {
        Ok(0)
    }

    async fn get_genre_progress(
        &self,
        _job_id: Uuid,
    ) -> Result<Vec<(String, String, Option<i32>)>> {
        Ok(vec![])
    }

    async fn get_completed_stages(&self, _job_id: Uuid) -> Result<Vec<String>> {
        Ok(vec![])
    }

    async fn create_user_triggered_job(
        &self,
        _job_id: Uuid,
        _user_id: Uuid,
        _note: Option<&str>,
    ) -> Result<()> {
        Ok(())
    }

    async fn get_user_jobs_count(
        &self,
        _user_id: Uuid,
        _window_seconds: i64,
    ) -> Result<i32> {
        Ok(0)
    }

    // Pulse
    async fn get_pulse_by_date(
        &self,
        _date: chrono::NaiveDate,
    ) -> Result<Option<PulseGenerationRow>> {
        Ok(None)
    }

    async fn get_latest_pulse(&self) -> Result<Option<PulseGenerationRow>> {
        Ok(None)
    }

    async fn save_pulse_generation(
        &self,
        _result: &PulseResult,
        _target_date: chrono::NaiveDate,
    ) -> Result<i64> {
        Ok(1) // Return mock generation ID
    }
}
