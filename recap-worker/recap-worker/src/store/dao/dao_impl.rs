/// RecapDaoImpl - RecapDaoトレイトの実装
use async_trait::async_trait;
use sqlx::PgPool;

use super::article;
use super::dao_trait::RecapDao;
use super::types::JobStatus;
use crate::store::models::{
    ClusterWithEvidence, DiagnosticEntry, GenreEvaluationMetric, GenreEvaluationRun,
    GenreLearningRecord, GenreWithSummary, GraphEdgeRecord, NewSubworkerRun, PersistedCluster,
    PersistedGenre, RawArticle, RecapJob, SubworkerRunStatus,
};

#[derive(Debug, Clone)]
pub struct RecapDaoImpl {
    pool: PgPool,
}

impl RecapDaoImpl {
    pub fn new(pool: PgPool) -> Self {
        Self { pool }
    }
}

#[async_trait]
impl RecapDao for RecapDaoImpl {
    fn pool(&self) -> Option<&PgPool> {
        Some(&self.pool)
    }

    // Job management
    #[allow(dead_code)]
    async fn create_job_with_lock(
        &self,
        job_id: uuid::Uuid,
        note: Option<&str>,
    ) -> anyhow::Result<Option<uuid::Uuid>> {
        super::job::RecapDao::create_job_with_lock(&self.pool, job_id, note).await
    }

    #[allow(dead_code)]
    async fn job_exists(&self, job_id: uuid::Uuid) -> anyhow::Result<bool> {
        super::job::RecapDao::job_exists(&self.pool, job_id).await
    }

    async fn find_resumable_job(
        &self,
    ) -> anyhow::Result<Option<(uuid::Uuid, JobStatus, Option<String>)>> {
        super::job::RecapDao::find_resumable_job(&self.pool).await
    }

    async fn update_job_status(
        &self,
        job_id: uuid::Uuid,
        status: JobStatus,
        last_stage: Option<&str>,
    ) -> anyhow::Result<()> {
        super::job::RecapDao::update_job_status(&self.pool, job_id, status, last_stage).await
    }

    async fn get_recap_jobs(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> anyhow::Result<
        Vec<(
            uuid::Uuid,
            String,
            Option<String>,
            chrono::DateTime<chrono::Utc>,
            chrono::DateTime<chrono::Utc>,
        )>,
    > {
        super::job::RecapDao::get_recap_jobs(&self.pool, window_seconds, limit).await
    }

    async fn delete_old_jobs(&self, retention_days: i64) -> anyhow::Result<u64> {
        super::job::RecapDao::delete_old_jobs(&self.pool, retention_days).await
    }

    // Stage management
    async fn insert_stage_log(
        &self,
        job_id: uuid::Uuid,
        stage: &str,
        status: &str,
        message: Option<&str>,
    ) -> anyhow::Result<()> {
        super::stage::RecapDao::insert_stage_log(&self.pool, job_id, stage, status, message).await
    }

    async fn save_stage_state(
        &self,
        job_id: uuid::Uuid,
        stage: &str,
        state_data: &serde_json::Value,
    ) -> anyhow::Result<()> {
        super::stage::RecapDao::save_stage_state(&self.pool, job_id, stage, state_data).await
    }

    async fn load_stage_state(
        &self,
        job_id: uuid::Uuid,
        stage: &str,
    ) -> anyhow::Result<Option<serde_json::Value>> {
        super::stage::RecapDao::load_stage_state(&self.pool, job_id, stage).await
    }

    async fn insert_failed_task(
        &self,
        job_id: uuid::Uuid,
        stage: &str,
        payload: Option<&serde_json::Value>,
        error: Option<&str>,
    ) -> anyhow::Result<()> {
        super::stage::RecapDao::insert_failed_task(&self.pool, job_id, stage, payload, error).await
    }

    // Article management
    async fn backup_raw_articles(
        &self,
        job_id: uuid::Uuid,
        articles: &[RawArticle],
    ) -> anyhow::Result<()> {
        super::article::RecapDao::backup_raw_articles(&self.pool, job_id, articles).await
    }

    async fn get_article_metadata(
        &self,
        job_id: uuid::Uuid,
        article_ids: &[String],
    ) -> anyhow::Result<
        std::collections::HashMap<String, (Option<chrono::DateTime<chrono::Utc>>, Option<String>)>,
    > {
        super::article::RecapDao::get_article_metadata(&self.pool, job_id, article_ids).await
    }

    async fn get_articles_by_ids(
        &self,
        job_id: uuid::Uuid,
        article_ids: &[String],
    ) -> anyhow::Result<Vec<article::FetchedArticleData>> {
        super::article::RecapDao::get_articles_by_ids(&self.pool, job_id, article_ids).await
    }

    // Genre learning
    async fn load_tag_label_graph(
        &self,
        window_label: &str,
    ) -> anyhow::Result<Vec<GraphEdgeRecord>> {
        super::genre_learning::RecapDao::load_tag_label_graph(&self.pool, window_label).await
    }

    async fn upsert_genre_learning_record(
        &self,
        record: &GenreLearningRecord,
    ) -> anyhow::Result<()> {
        super::genre_learning::RecapDao::upsert_genre_learning_record(&self.pool, record).await
    }

    async fn upsert_genre_learning_records_bulk(
        &self,
        records: &[GenreLearningRecord],
    ) -> anyhow::Result<()> {
        super::genre_learning::RecapDao::upsert_genre_learning_records_bulk(&self.pool, records)
            .await
    }

    // Config
    async fn get_latest_worker_config(
        &self,
        config_type: &str,
    ) -> anyhow::Result<Option<serde_json::Value>> {
        super::config::RecapDao::get_latest_worker_config(&self.pool, config_type).await
    }

    async fn insert_worker_config(
        &self,
        config_type: &str,
        config_payload: &serde_json::Value,
        source: &str,
        metadata: Option<&serde_json::Value>,
    ) -> anyhow::Result<()> {
        super::config::RecapDao::insert_worker_config(
            &self.pool,
            config_type,
            config_payload,
            source,
            metadata,
        )
        .await
    }

    // Metrics
    async fn save_preprocess_metrics(
        &self,
        metrics: &crate::store::models::PreprocessMetrics,
    ) -> anyhow::Result<()> {
        super::metrics::RecapDao::save_preprocess_metrics(&self.pool, metrics).await
    }

    async fn save_system_metrics(
        &self,
        job_id: uuid::Uuid,
        metric_type: &str,
        metrics: &serde_json::Value,
    ) -> anyhow::Result<()> {
        super::metrics::RecapDao::save_system_metrics(&self.pool, job_id, metric_type, metrics)
            .await
    }

    async fn get_system_metrics(
        &self,
        metric_type: Option<&str>,
        window_seconds: i64,
        limit: i64,
    ) -> anyhow::Result<
        Vec<(
            Option<uuid::Uuid>,
            chrono::DateTime<chrono::Utc>,
            serde_json::Value,
        )>,
    > {
        super::metrics::RecapDao::get_system_metrics(&self.pool, metric_type, window_seconds, limit)
            .await
    }

    async fn get_recent_activity(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> anyhow::Result<Vec<(Option<uuid::Uuid>, String, chrono::DateTime<chrono::Utc>)>> {
        super::metrics::RecapDao::get_recent_activity(&self.pool, window_seconds, limit).await
    }

    async fn get_log_errors(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> anyhow::Result<
        Vec<(
            chrono::DateTime<chrono::Utc>,
            String,
            Option<String>,
            Option<String>,
            Option<String>,
        )>,
    > {
        super::metrics::RecapDao::get_log_errors(&self.pool, window_seconds, limit).await
    }

    async fn get_admin_jobs(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> anyhow::Result<
        Vec<(
            uuid::Uuid,
            String,
            String,
            chrono::DateTime<chrono::Utc>,
            Option<chrono::DateTime<chrono::Utc>>,
            Option<serde_json::Value>,
            Option<serde_json::Value>,
            Option<String>,
        )>,
    > {
        super::metrics::RecapDao::get_admin_jobs(&self.pool, window_seconds, limit).await
    }

    // Output
    #[allow(dead_code)]
    async fn save_final_section(
        &self,
        section: &crate::store::models::RecapFinalSection,
    ) -> anyhow::Result<i64> {
        super::output::RecapDao::save_final_section(&self.pool, section).await
    }

    #[allow(dead_code)]
    async fn upsert_recap_output(
        &self,
        output: &crate::store::models::RecapOutput,
    ) -> anyhow::Result<()> {
        super::output::RecapDao::upsert_recap_output(&self.pool, output).await
    }

    async fn get_recap_output_body_json(
        &self,
        job_id: uuid::Uuid,
        genre: &str,
    ) -> anyhow::Result<Option<serde_json::Value>> {
        super::output::RecapDao::get_recap_output_body_json(&self.pool, job_id, genre).await
    }

    async fn get_latest_completed_job(&self, window_days: i32) -> anyhow::Result<Option<RecapJob>> {
        super::output::RecapDao::get_latest_completed_job(&self.pool, window_days).await
    }

    async fn get_genres_by_job(&self, job_id: uuid::Uuid) -> anyhow::Result<Vec<GenreWithSummary>> {
        super::output::RecapDao::get_genres_by_job(&self.pool, job_id).await
    }

    async fn get_clusters_by_job(
        &self,
        job_id: uuid::Uuid,
    ) -> anyhow::Result<std::collections::HashMap<String, Vec<ClusterWithEvidence>>> {
        super::output::RecapDao::get_clusters_by_job(&self.pool, job_id).await
    }

    // Subworker
    #[allow(dead_code)]
    async fn insert_subworker_run(&self, run: &NewSubworkerRun) -> anyhow::Result<i64> {
        super::subworker::RecapDao::insert_subworker_run(&self.pool, run).await
    }

    #[allow(dead_code)]
    async fn mark_subworker_run_success(
        &self,
        run_id: i64,
        cluster_count: i32,
        response_payload: &serde_json::Value,
    ) -> anyhow::Result<()> {
        super::subworker::RecapDao::mark_subworker_run_success(
            &self.pool,
            run_id,
            cluster_count,
            response_payload,
        )
        .await
    }

    #[allow(dead_code)]
    async fn mark_subworker_run_failure(
        &self,
        run_id: i64,
        status: SubworkerRunStatus,
        error_message: &str,
    ) -> anyhow::Result<()> {
        super::subworker::RecapDao::mark_subworker_run_failure(
            &self.pool,
            run_id,
            status,
            error_message,
        )
        .await
    }

    #[allow(dead_code)]
    async fn insert_clusters(
        &self,
        run_id: i64,
        clusters: &[PersistedCluster],
    ) -> anyhow::Result<()> {
        super::subworker::RecapDao::insert_clusters(&self.pool, run_id, clusters).await
    }

    #[allow(dead_code)]
    async fn upsert_diagnostics(
        &self,
        run_id: i64,
        diagnostics: &[DiagnosticEntry],
    ) -> anyhow::Result<()> {
        super::subworker::RecapDao::upsert_diagnostics(&self.pool, run_id, diagnostics).await
    }

    #[allow(dead_code)]
    async fn upsert_genre(&self, genre: &PersistedGenre) -> anyhow::Result<()> {
        super::subworker::RecapDao::upsert_genre(&self.pool, genre).await
    }

    // Evaluation
    async fn save_genre_evaluation(
        &self,
        run: &GenreEvaluationRun,
        metrics: &[GenreEvaluationMetric],
    ) -> anyhow::Result<()> {
        super::evaluation::RecapDao::save_genre_evaluation(&self.pool, run, metrics).await
    }

    #[allow(clippy::too_many_lines)]
    async fn get_genre_evaluation(
        &self,
        run_id: uuid::Uuid,
    ) -> anyhow::Result<Option<(GenreEvaluationRun, Vec<GenreEvaluationMetric>)>> {
        super::evaluation::RecapDao::get_genre_evaluation(&self.pool, run_id).await
    }

    async fn get_latest_genre_evaluation(
        &self,
    ) -> anyhow::Result<Option<(GenreEvaluationRun, Vec<GenreEvaluationMetric>)>> {
        super::evaluation::RecapDao::get_latest_genre_evaluation(&self.pool).await
    }

    // Morning
    async fn save_morning_article_groups(
        &self,
        groups: &[(uuid::Uuid, uuid::Uuid, bool)],
    ) -> anyhow::Result<()> {
        super::morning::RecapDao::save_morning_article_groups(&self.pool, groups).await
    }

    async fn get_morning_article_groups(
        &self,
        since: chrono::DateTime<chrono::Utc>,
    ) -> anyhow::Result<Vec<(uuid::Uuid, uuid::Uuid, bool, chrono::DateTime<chrono::Utc>)>> {
        super::morning::RecapDao::get_morning_article_groups(&self.pool, since).await
    }
}
