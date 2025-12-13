/// RecapDaoトレイト - データアクセス層の抽象化
use async_trait::async_trait;
use sqlx::PgPool;

use super::article;
use super::types::JobStatus;
use crate::store::models::{
    ClusterWithEvidence, DiagnosticEntry, GenreEvaluationMetric, GenreEvaluationRun,
    GenreLearningRecord, GenreWithSummary, GraphEdgeRecord, NewSubworkerRun, PersistedCluster,
    PersistedGenre, RawArticle, RecapJob, SubworkerRunStatus,
};

#[async_trait]
pub trait RecapDao: Send + Sync {
    /// データベース接続プールへの参照を返す（実装によってはNoneを返すことも可能）
    fn pool(&self) -> Option<&PgPool>;

    // Job management
    async fn create_job_with_lock(
        &self,
        job_id: uuid::Uuid,
        note: Option<&str>,
    ) -> anyhow::Result<Option<uuid::Uuid>>;

    #[allow(dead_code)]
    async fn job_exists(&self, job_id: uuid::Uuid) -> anyhow::Result<bool>;

    async fn find_resumable_job(
        &self,
    ) -> anyhow::Result<Option<(uuid::Uuid, JobStatus, Option<String>)>>;

    async fn update_job_status(
        &self,
        job_id: uuid::Uuid,
        status: JobStatus,
        last_stage: Option<&str>,
    ) -> anyhow::Result<()>;

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
    >;

    async fn delete_old_jobs(&self, retention_days: i64) -> anyhow::Result<u64>;

    // Stage management
    async fn insert_stage_log(
        &self,
        job_id: uuid::Uuid,
        stage: &str,
        status: &str,
        message: Option<&str>,
    ) -> anyhow::Result<()>;

    async fn save_stage_state(
        &self,
        job_id: uuid::Uuid,
        stage: &str,
        state_data: &serde_json::Value,
    ) -> anyhow::Result<()>;

    async fn load_stage_state(
        &self,
        job_id: uuid::Uuid,
        stage: &str,
    ) -> anyhow::Result<Option<serde_json::Value>>;

    async fn insert_failed_task(
        &self,
        job_id: uuid::Uuid,
        stage: &str,
        payload: Option<&serde_json::Value>,
        error: Option<&str>,
    ) -> anyhow::Result<()>;

    // Article management
    async fn backup_raw_articles(
        &self,
        job_id: uuid::Uuid,
        articles: &[RawArticle],
    ) -> anyhow::Result<()>;

    async fn get_article_metadata(
        &self,
        job_id: uuid::Uuid,
        article_ids: &[String],
    ) -> anyhow::Result<
        std::collections::HashMap<String, (Option<chrono::DateTime<chrono::Utc>>, Option<String>)>,
    >;

    async fn get_articles_by_ids(
        &self,
        job_id: uuid::Uuid,
        article_ids: &[String],
    ) -> anyhow::Result<Vec<article::FetchedArticleData>>;

    // Genre learning
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

    // Config
    async fn get_latest_worker_config(
        &self,
        config_type: &str,
    ) -> anyhow::Result<Option<serde_json::Value>>;

    async fn insert_worker_config(
        &self,
        config_type: &str,
        config_payload: &serde_json::Value,
        source: &str,
        metadata: Option<&serde_json::Value>,
    ) -> anyhow::Result<()>;

    // Metrics
    async fn save_preprocess_metrics(
        &self,
        metrics: &crate::store::models::PreprocessMetrics,
    ) -> anyhow::Result<()>;

    async fn save_system_metrics(
        &self,
        job_id: uuid::Uuid,
        metric_type: &str,
        metrics: &serde_json::Value,
    ) -> anyhow::Result<()>;

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
    >;

    async fn get_recent_activity(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> anyhow::Result<Vec<(Option<uuid::Uuid>, String, chrono::DateTime<chrono::Utc>)>>;

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
    >;

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
    >;

    // Output
    #[allow(dead_code)]
    async fn save_final_section(
        &self,
        section: &crate::store::models::RecapFinalSection,
    ) -> anyhow::Result<i64>;

    async fn upsert_recap_output(
        &self,
        output: &crate::store::models::RecapOutput,
    ) -> anyhow::Result<()>;

    async fn get_recap_output_body_json(
        &self,
        job_id: uuid::Uuid,
        genre: &str,
    ) -> anyhow::Result<Option<serde_json::Value>>;

    async fn get_latest_completed_job(&self, window_days: i32) -> anyhow::Result<Option<RecapJob>>;

    async fn get_genres_by_job(&self, job_id: uuid::Uuid) -> anyhow::Result<Vec<GenreWithSummary>>;

    async fn get_clusters_by_job(
        &self,
        job_id: uuid::Uuid,
    ) -> anyhow::Result<std::collections::HashMap<String, Vec<ClusterWithEvidence>>>;

    // Subworker
    async fn insert_subworker_run(&self, run: &NewSubworkerRun) -> anyhow::Result<i64>;

    async fn mark_subworker_run_success(
        &self,
        run_id: i64,
        cluster_count: i32,
        response_payload: &serde_json::Value,
    ) -> anyhow::Result<()>;

    #[allow(dead_code)]
    async fn mark_subworker_run_failure(
        &self,
        run_id: i64,
        status: SubworkerRunStatus,
        error_message: &str,
    ) -> anyhow::Result<()>;

    #[allow(dead_code)]
    async fn insert_clusters(
        &self,
        run_id: i64,
        clusters: &[PersistedCluster],
    ) -> anyhow::Result<()>;

    #[allow(dead_code)]
    async fn upsert_diagnostics(
        &self,
        run_id: i64,
        diagnostics: &[DiagnosticEntry],
    ) -> anyhow::Result<()>;

    async fn upsert_genre(&self, genre: &PersistedGenre) -> anyhow::Result<()>;

    // Evaluation
    async fn save_genre_evaluation(
        &self,
        run: &GenreEvaluationRun,
        metrics: &[GenreEvaluationMetric],
    ) -> anyhow::Result<()>;

    async fn get_genre_evaluation(
        &self,
        run_id: uuid::Uuid,
    ) -> anyhow::Result<Option<(GenreEvaluationRun, Vec<GenreEvaluationMetric>)>>;

    async fn get_latest_genre_evaluation(
        &self,
    ) -> anyhow::Result<Option<(GenreEvaluationRun, Vec<GenreEvaluationMetric>)>>;

    // Morning
    async fn save_morning_article_groups(
        &self,
        groups: &[(uuid::Uuid, uuid::Uuid, bool)],
    ) -> anyhow::Result<()>;

    async fn get_morning_article_groups(
        &self,
        since: chrono::DateTime<chrono::Utc>,
    ) -> anyhow::Result<Vec<(uuid::Uuid, uuid::Uuid, bool, chrono::DateTime<chrono::Utc>)>>;
}
