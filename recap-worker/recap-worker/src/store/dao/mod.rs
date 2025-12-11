use sqlx::PgPool;

pub mod article;
pub mod config;
pub mod evaluation;
pub mod genre_learning;
pub mod job;
pub mod metrics;
pub mod morning;
pub mod output;
pub mod stage;
pub mod subworker;

use super::models::{
    ClusterWithEvidence, DiagnosticEntry, GenreEvaluationMetric, GenreEvaluationRun,
    GenreLearningRecord, GenreWithSummary, GraphEdgeRecord, NewSubworkerRun, PersistedCluster,
    PersistedGenre, RawArticle, RecapJob, SubworkerRunStatus,
};

#[derive(Debug, Clone, PartialEq, sqlx::Type)]
#[sqlx(type_name = "text", rename_all = "lowercase")]
pub enum JobStatus {
    Pending,
    Running,
    Completed,
    Failed,
}

impl AsRef<str> for JobStatus {
    fn as_ref(&self) -> &str {
        match self {
            JobStatus::Pending => "pending",
            JobStatus::Running => "running",
            JobStatus::Completed => "completed",
            JobStatus::Failed => "failed",
        }
    }
}

#[derive(Debug, Clone)]
pub(crate) struct RecapDao {
    pool: PgPool,
}

impl RecapDao {
    pub(crate) fn new(pool: PgPool) -> Self {
        Self { pool }
    }

    pub(crate) fn pool(&self) -> &PgPool {
        &self.pool
    }
}

// Re-export all methods from submodules
impl RecapDao {
    // Job management
    #[allow(dead_code)]
    pub async fn create_job_with_lock(
        &self,
        job_id: uuid::Uuid,
        note: Option<&str>,
    ) -> anyhow::Result<Option<uuid::Uuid>> {
        job::RecapDao::create_job_with_lock(&self.pool, job_id, note).await
    }

    #[allow(dead_code)]
    pub async fn job_exists(&self, job_id: uuid::Uuid) -> anyhow::Result<bool> {
        job::RecapDao::job_exists(&self.pool, job_id).await
    }

    pub async fn find_resumable_job(
        &self,
    ) -> anyhow::Result<Option<(uuid::Uuid, JobStatus, Option<String>)>> {
        job::RecapDao::find_resumable_job(&self.pool).await
    }

    pub async fn update_job_status(
        &self,
        job_id: uuid::Uuid,
        status: JobStatus,
        last_stage: Option<&str>,
    ) -> anyhow::Result<()> {
        job::RecapDao::update_job_status(&self.pool, job_id, status, last_stage).await
    }

    pub async fn get_recap_jobs(
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
        job::RecapDao::get_recap_jobs(&self.pool, window_seconds, limit).await
    }

    // Stage management
    pub async fn insert_stage_log(
        &self,
        job_id: uuid::Uuid,
        stage: &str,
        status: &str,
        message: Option<&str>,
    ) -> anyhow::Result<()> {
        stage::RecapDao::insert_stage_log(&self.pool, job_id, stage, status, message).await
    }

    pub async fn save_stage_state(
        &self,
        job_id: uuid::Uuid,
        stage: &str,
        state_data: &serde_json::Value,
    ) -> anyhow::Result<()> {
        stage::RecapDao::save_stage_state(&self.pool, job_id, stage, state_data).await
    }

    pub async fn load_stage_state(
        &self,
        job_id: uuid::Uuid,
        stage: &str,
    ) -> anyhow::Result<Option<serde_json::Value>> {
        stage::RecapDao::load_stage_state(&self.pool, job_id, stage).await
    }

    pub async fn insert_failed_task(
        &self,
        job_id: uuid::Uuid,
        stage: &str,
        payload: Option<&serde_json::Value>,
        error: Option<&str>,
    ) -> anyhow::Result<()> {
        stage::RecapDao::insert_failed_task(&self.pool, job_id, stage, payload, error).await
    }

    // Article management
    pub async fn backup_raw_articles(
        &self,
        job_id: uuid::Uuid,
        articles: &[RawArticle],
    ) -> anyhow::Result<()> {
        article::RecapDao::backup_raw_articles(&self.pool, job_id, articles).await
    }

    pub async fn get_article_metadata(
        &self,
        job_id: uuid::Uuid,
        article_ids: &[String],
    ) -> anyhow::Result<
        std::collections::HashMap<String, (Option<chrono::DateTime<chrono::Utc>>, Option<String>)>,
    > {
        article::RecapDao::get_article_metadata(&self.pool, job_id, article_ids).await
    }

    // Genre learning
    pub async fn load_tag_label_graph(
        &self,
        window_label: &str,
    ) -> anyhow::Result<Vec<GraphEdgeRecord>> {
        genre_learning::RecapDao::load_tag_label_graph(&self.pool, window_label).await
    }

    pub async fn upsert_genre_learning_record(
        &self,
        record: &GenreLearningRecord,
    ) -> anyhow::Result<()> {
        genre_learning::RecapDao::upsert_genre_learning_record(&self.pool, record).await
    }

    pub async fn upsert_genre_learning_records_bulk(
        &self,
        records: &[GenreLearningRecord],
    ) -> anyhow::Result<()> {
        genre_learning::RecapDao::upsert_genre_learning_records_bulk(&self.pool, records).await
    }

    // Config
    pub async fn get_latest_worker_config(
        &self,
        config_type: &str,
    ) -> anyhow::Result<Option<serde_json::Value>> {
        config::RecapDao::get_latest_worker_config(&self.pool, config_type).await
    }

    pub async fn insert_worker_config(
        &self,
        config_type: &str,
        config_payload: &serde_json::Value,
        source: &str,
        metadata: Option<&serde_json::Value>,
    ) -> anyhow::Result<()> {
        config::RecapDao::insert_worker_config(
            &self.pool,
            config_type,
            config_payload,
            source,
            metadata,
        )
        .await
    }

    // Metrics
    pub async fn save_preprocess_metrics(
        &self,
        metrics: &crate::store::models::PreprocessMetrics,
    ) -> anyhow::Result<()> {
        metrics::RecapDao::save_preprocess_metrics(&self.pool, metrics).await
    }

    pub async fn save_system_metrics(
        &self,
        job_id: uuid::Uuid,
        metric_type: &str,
        metrics: &serde_json::Value,
    ) -> anyhow::Result<()> {
        metrics::RecapDao::save_system_metrics(&self.pool, job_id, metric_type, metrics).await
    }

    pub async fn get_system_metrics(
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
        metrics::RecapDao::get_system_metrics(&self.pool, metric_type, window_seconds, limit).await
    }

    pub async fn get_recent_activity(
        &self,
        window_seconds: i64,
        limit: i64,
    ) -> anyhow::Result<Vec<(Option<uuid::Uuid>, String, chrono::DateTime<chrono::Utc>)>> {
        metrics::RecapDao::get_recent_activity(&self.pool, window_seconds, limit).await
    }

    pub async fn get_log_errors(
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
        metrics::RecapDao::get_log_errors(&self.pool, window_seconds, limit).await
    }

    pub async fn get_admin_jobs(
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
        metrics::RecapDao::get_admin_jobs(&self.pool, window_seconds, limit).await
    }

    // Output
    #[allow(dead_code)]
    pub async fn save_final_section(
        &self,
        section: &crate::store::models::RecapFinalSection,
    ) -> anyhow::Result<i64> {
        output::RecapDao::save_final_section(&self.pool, section).await
    }

    #[allow(dead_code)]
    pub async fn upsert_recap_output(
        &self,
        output: &crate::store::models::RecapOutput,
    ) -> anyhow::Result<()> {
        output::RecapDao::upsert_recap_output(&self.pool, output).await
    }

    pub(crate) async fn get_recap_output_body_json(
        &self,
        job_id: uuid::Uuid,
        genre: &str,
    ) -> anyhow::Result<Option<serde_json::Value>> {
        output::RecapDao::get_recap_output_body_json(&self.pool, job_id, genre).await
    }

    pub(crate) async fn get_latest_completed_job(
        &self,
        window_days: i32,
    ) -> anyhow::Result<Option<RecapJob>> {
        output::RecapDao::get_latest_completed_job(&self.pool, window_days).await
    }

    pub(crate) async fn get_genres_by_job(
        &self,
        job_id: uuid::Uuid,
    ) -> anyhow::Result<Vec<GenreWithSummary>> {
        output::RecapDao::get_genres_by_job(&self.pool, job_id).await
    }

    pub(crate) async fn get_clusters_by_job(
        &self,
        job_id: uuid::Uuid,
    ) -> anyhow::Result<std::collections::HashMap<String, Vec<ClusterWithEvidence>>> {
        output::RecapDao::get_clusters_by_job(&self.pool, job_id).await
    }

    // Subworker
    #[allow(dead_code)]
    pub(crate) async fn insert_subworker_run(&self, run: &NewSubworkerRun) -> anyhow::Result<i64> {
        subworker::RecapDao::insert_subworker_run(&self.pool, run).await
    }

    #[allow(dead_code)]
    pub(crate) async fn mark_subworker_run_success(
        &self,
        run_id: i64,
        cluster_count: i32,
        response_payload: &serde_json::Value,
    ) -> anyhow::Result<()> {
        subworker::RecapDao::mark_subworker_run_success(
            &self.pool,
            run_id,
            cluster_count,
            response_payload,
        )
        .await
    }

    #[allow(dead_code)]
    pub(crate) async fn mark_subworker_run_failure(
        &self,
        run_id: i64,
        status: SubworkerRunStatus,
        error_message: &str,
    ) -> anyhow::Result<()> {
        subworker::RecapDao::mark_subworker_run_failure(&self.pool, run_id, status, error_message)
            .await
    }

    #[allow(dead_code)]
    pub(crate) async fn insert_clusters(
        &self,
        run_id: i64,
        clusters: &[PersistedCluster],
    ) -> anyhow::Result<()> {
        subworker::RecapDao::insert_clusters(&self.pool, run_id, clusters).await
    }

    #[allow(dead_code)]
    pub(crate) async fn upsert_diagnostics(
        &self,
        run_id: i64,
        diagnostics: &[DiagnosticEntry],
    ) -> anyhow::Result<()> {
        subworker::RecapDao::upsert_diagnostics(&self.pool, run_id, diagnostics).await
    }

    #[allow(dead_code)]
    pub(crate) async fn upsert_genre(&self, genre: &PersistedGenre) -> anyhow::Result<()> {
        subworker::RecapDao::upsert_genre(&self.pool, genre).await
    }

    // Evaluation
    pub async fn save_genre_evaluation(
        &self,
        run: &GenreEvaluationRun,
        metrics: &[GenreEvaluationMetric],
    ) -> anyhow::Result<()> {
        evaluation::RecapDao::save_genre_evaluation(&self.pool, run, metrics).await
    }

    #[allow(clippy::too_many_lines)]
    pub async fn get_genre_evaluation(
        &self,
        run_id: uuid::Uuid,
    ) -> anyhow::Result<Option<(GenreEvaluationRun, Vec<GenreEvaluationMetric>)>> {
        evaluation::RecapDao::get_genre_evaluation(&self.pool, run_id).await
    }

    pub async fn get_latest_genre_evaluation(
        &self,
    ) -> anyhow::Result<Option<(GenreEvaluationRun, Vec<GenreEvaluationMetric>)>> {
        evaluation::RecapDao::get_latest_genre_evaluation(&self.pool).await
    }

    // Morning
    pub async fn save_morning_article_groups(
        &self,
        groups: &[(uuid::Uuid, uuid::Uuid, bool)],
    ) -> anyhow::Result<()> {
        morning::RecapDao::save_morning_article_groups(&self.pool, groups).await
    }

    pub async fn get_morning_article_groups(
        &self,
        since: chrono::DateTime<chrono::Utc>,
    ) -> anyhow::Result<Vec<(uuid::Uuid, uuid::Uuid, bool, chrono::DateTime<chrono::Utc>)>> {
        morning::RecapDao::get_morning_article_groups(&self.pool, since).await
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::store::models::{
        CoarseCandidateRecord, LearningTimestamps, PersistedSentence, RecapOutput,
        RefineDecisionRecord, TagProfileRecord, TagSignalRecord,
    };
    use chrono::Utc;
    use serde_json::Value;
    use sqlx::{Executor, Row, postgres::PgPoolOptions};
    use uuid::Uuid;

    async fn setup_subworker_tables(pool: &PgPool) -> anyhow::Result<()> {
        pool.execute(
            r"
            CREATE TABLE IF NOT EXISTS recap_subworker_runs (
                id BIGSERIAL PRIMARY KEY,
                job_id UUID NOT NULL,
                genre TEXT NOT NULL,
                status TEXT NOT NULL,
                cluster_count INT NOT NULL DEFAULT 0,
                started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                finished_at TIMESTAMPTZ,
                request_payload JSONB NOT NULL DEFAULT '{}'::JSONB,
                response_payload JSONB,
                error_message TEXT
            );

            CREATE TABLE IF NOT EXISTS recap_subworker_clusters (
                id BIGSERIAL PRIMARY KEY,
                run_id BIGINT NOT NULL REFERENCES recap_subworker_runs(id) ON DELETE CASCADE,
                cluster_id INT NOT NULL,
                size INT NOT NULL,
                label TEXT,
                top_terms JSONB NOT NULL,
                stats JSONB NOT NULL,
                UNIQUE (run_id, cluster_id)
            );

            CREATE TABLE IF NOT EXISTS recap_subworker_sentences (
                id BIGSERIAL PRIMARY KEY,
                cluster_row_id BIGINT NOT NULL REFERENCES recap_subworker_clusters(id) ON DELETE CASCADE,
                source_article_id TEXT NOT NULL,
                paragraph_idx INT,
                sentence_id INT NOT NULL,
                sentence_text TEXT NOT NULL,
                lang TEXT NOT NULL DEFAULT 'unknown',
                score REAL NOT NULL,
                UNIQUE (cluster_row_id, source_article_id, sentence_id)
            );

            CREATE TABLE IF NOT EXISTS recap_subworker_diagnostics (
                run_id BIGINT NOT NULL REFERENCES recap_subworker_runs(id) ON DELETE CASCADE,
                metric TEXT NOT NULL,
                value JSONB NOT NULL,
                PRIMARY KEY (run_id, metric)
            );
            ",
        )
        .await?;
        Ok(())
    }

    async fn setup_learning_tables(pool: &PgPool) -> anyhow::Result<()> {
        pool.execute(
            r"
            CREATE TABLE IF NOT EXISTS recap_genre_learning_results (
                job_id UUID NOT NULL,
                article_id TEXT NOT NULL,
                coarse_candidates JSONB NOT NULL,
                refine_decision JSONB NOT NULL,
                tag_profile JSONB NOT NULL,
                graph_context JSONB NOT NULL DEFAULT '[]'::JSONB,
                feedback JSONB,
                telemetry JSONB,
                timestamps JSONB NOT NULL,
                created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                PRIMARY KEY (job_id, article_id)
            );

            CREATE TABLE IF NOT EXISTS tag_label_graph (
                window_label TEXT NOT NULL,
                genre TEXT NOT NULL,
                tag TEXT NOT NULL,
                weight REAL NOT NULL,
                sample_size INTEGER NOT NULL DEFAULT 0,
                last_observed_at TIMESTAMPTZ,
                updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                PRIMARY KEY (window_label, genre, tag)
            );
            ",
        )
        .await?;
        Ok(())
    }

    async fn setup_recap_tables(pool: &PgPool) -> anyhow::Result<()> {
        pool.execute(
            r"
            CREATE TABLE IF NOT EXISTS recap_sections (
                job_id UUID NOT NULL,
                genre TEXT NOT NULL,
                response_id TEXT,
                PRIMARY KEY (job_id, genre)
            );

            CREATE TABLE IF NOT EXISTS recap_outputs (
                job_id UUID NOT NULL,
                genre TEXT NOT NULL,
                response_id TEXT NOT NULL,
                title_ja TEXT NOT NULL,
                summary_ja TEXT NOT NULL,
                bullets_ja JSONB NOT NULL,
                body_json JSONB NOT NULL,
                created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                PRIMARY KEY (job_id, genre)
            );
            ",
        )
        .await?;
        Ok(())
    }

    async fn setup_evaluation_tables(pool: &PgPool) -> anyhow::Result<()> {
        pool.execute(
            r"
            CREATE TABLE IF NOT EXISTS recap_genre_evaluation_runs (
                run_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                dataset_path TEXT NOT NULL,
                total_items INTEGER NOT NULL,
                macro_precision DOUBLE PRECISION NOT NULL,
                macro_recall DOUBLE PRECISION NOT NULL,
                macro_f1 DOUBLE PRECISION NOT NULL,
                summary_tp INTEGER NOT NULL,
                summary_fp INTEGER NOT NULL,
                summary_fn INTEGER NOT NULL,
                created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
            );

            CREATE TABLE IF NOT EXISTS recap_genre_evaluation_metrics (
                run_id UUID NOT NULL REFERENCES recap_genre_evaluation_runs(run_id) ON DELETE CASCADE,
                genre TEXT NOT NULL,
                tp INTEGER NOT NULL,
                fp INTEGER NOT NULL,
                fn_count INTEGER NOT NULL,
                precision DOUBLE PRECISION NOT NULL,
                recall DOUBLE PRECISION NOT NULL,
                f1_score DOUBLE PRECISION NOT NULL,
                PRIMARY KEY (run_id, genre)
            );
            ",
        )
        .await?;
        Ok(())
    }

    async fn setup_schema(pool: &PgPool) -> anyhow::Result<()> {
        setup_subworker_tables(pool).await?;
        setup_learning_tables(pool).await?;
        setup_recap_tables(pool).await?;
        setup_evaluation_tables(pool).await?;
        Ok(())
    }

    #[tokio::test]
    async fn upsert_genre_inserts() -> anyhow::Result<()> {
        let Ok(database_url) = std::env::var("DATABASE_URL") else {
            return Ok(());
        };
        let pool = PgPoolOptions::new()
            .max_connections(1)
            .connect(&database_url)
            .await?;
        setup_schema(&pool).await?;
        let dao = RecapDao::new(pool.clone());
        let record = PersistedGenre::new(Uuid::new_v4(), "ai");

        dao.upsert_genre(&record).await?;

        let row =
            sqlx::query(r"SELECT job_id, genre, response_id FROM recap_sections WHERE job_id = $1")
                .bind(record.job_id)
                .fetch_one(&pool)
                .await?;

        let job_id: Uuid = row.get("job_id");
        let genre: String = row.get("genre");
        let response_id: Option<String> = row.get("response_id");

        assert_eq!(job_id, record.job_id);
        assert_eq!(genre, record.genre);
        assert!(response_id.is_none());
        Ok(())
    }

    #[tokio::test]
    async fn upsert_genre_updates_response() -> anyhow::Result<()> {
        let Ok(database_url) = std::env::var("DATABASE_URL") else {
            return Ok(());
        };
        let pool = PgPoolOptions::new()
            .max_connections(1)
            .connect(&database_url)
            .await?;
        setup_schema(&pool).await?;
        let dao = RecapDao::new(pool.clone());
        let job_id = Uuid::new_v4();
        let base = PersistedGenre::new(job_id, "science");

        dao.upsert_genre(&base).await?;

        let updated = base.with_response_id(Some("resp-1".to_string()));
        dao.upsert_genre(&updated).await?;

        let row =
            sqlx::query(r"SELECT response_id FROM recap_sections WHERE job_id = $1 AND genre = $2")
                .bind(job_id)
                .bind(&updated.genre)
                .fetch_one(&pool)
                .await?;

        let response_id: Option<String> = row.get("response_id");
        assert_eq!(response_id.as_deref(), Some("resp-1"));
        Ok(())
    }

    #[tokio::test]
    async fn upsert_recap_output_inserts() -> anyhow::Result<()> {
        let Ok(database_url) = std::env::var("DATABASE_URL") else {
            return Ok(());
        };
        let pool = PgPoolOptions::new()
            .max_connections(1)
            .connect(&database_url)
            .await?;
        setup_schema(&pool).await?;
        let dao = RecapDao::new(pool.clone());

        let job_id = Uuid::new_v4();
        let output = RecapOutput::new(
            job_id,
            "science",
            "resp-123",
            "サマリータイトル",
            "箇条書き1\n箇条書き2",
            serde_json::json!([
                { "text": "箇条書き1", "sources": [] },
                { "text": "箇条書き2", "sources": [] }
            ]),
            serde_json::json!({
                "title": "サマリータイトル",
                "bullets": ["箇条書き1", "箇条書き2"],
                "language": "ja"
            }),
        );

        dao.upsert_recap_output(&output).await?;

        let row = sqlx::query(
            "SELECT response_id, title_ja, summary_ja, bullets_ja, body_json \
             FROM recap_outputs WHERE job_id = $1 AND genre = $2",
        )
        .bind(job_id)
        .bind("science")
        .fetch_one(&pool)
        .await?;

        let response_id: String = row.get("response_id");
        let title: String = row.get("title_ja");
        let summary: String = row.get("summary_ja");
        let bullets: Value = row.get("bullets_ja");
        let body: Value = row.get("body_json");

        assert_eq!(response_id, "resp-123");
        assert_eq!(title, "サマリータイトル");
        assert_eq!(summary, "箇条書き1\n箇条書き2");
        assert_eq!(bullets["0"]["text"], "箇条書き1");
        assert_eq!(body["title"], "サマリータイトル");
        Ok(())
    }

    #[tokio::test]
    async fn subworker_run_lifecycle() -> anyhow::Result<()> {
        let Ok(database_url) = std::env::var("DATABASE_URL") else {
            return Ok(());
        };
        let pool = PgPoolOptions::new()
            .max_connections(1)
            .connect(&database_url)
            .await?;
        setup_schema(&pool).await?;
        let dao = RecapDao::new(pool.clone());

        let job_id = Uuid::new_v4();
        let run = NewSubworkerRun::new(job_id, "ai", serde_json::json!({"articles": 5}));
        let run_id = dao.insert_subworker_run(&run).await?;

        dao.mark_subworker_run_success(run_id, 3, &serde_json::json!({"summary": "done"}))
            .await?;

        let row = sqlx::query("SELECT status, cluster_count, response_payload FROM recap_subworker_runs WHERE id = $1")
            .bind(run_id)
            .fetch_one(&pool)
            .await?;

        let status: String = row.get("status");
        let cluster_count: i32 = row.get("cluster_count");
        let response: Value = row.get::<Value, _>("response_payload");

        assert_eq!(status, "succeeded");
        assert_eq!(cluster_count, 3);
        assert_eq!(response["summary"], "done");

        Ok(())
    }

    #[tokio::test]
    async fn insert_clusters_with_sentences() -> anyhow::Result<()> {
        let Ok(database_url) = std::env::var("DATABASE_URL") else {
            return Ok(());
        };
        let pool = PgPoolOptions::new()
            .max_connections(1)
            .connect(&database_url)
            .await?;
        setup_schema(&pool).await?;
        let dao = RecapDao::new(pool.clone());

        let run_id = dao
            .insert_subworker_run(&NewSubworkerRun::new(
                Uuid::new_v4(),
                "security",
                serde_json::json!({}),
            ))
            .await?;

        let clusters = vec![PersistedCluster::new(
            0,
            2,
            Some("GPU不足".to_string()),
            serde_json::json!(["gpu", "supply"]),
            serde_json::json!({"avg_sim": 0.73}),
            vec![
                PersistedSentence::new("art_1", 1, "Sentence A", "ja", Some(0), 0.9),
                PersistedSentence::new("art_2", 2, "Sentence B", "ja", Some(1), 0.8),
            ],
        )];

        dao.insert_clusters(run_id, &clusters).await?;

        let row = sqlx::query("SELECT COUNT(*) as count FROM recap_subworker_sentences")
            .fetch_one(&pool)
            .await?;
        let count: i64 = row.get("count");
        assert_eq!(count, 2);

        Ok(())
    }

    #[tokio::test]
    async fn diagnostics_upsert() -> anyhow::Result<()> {
        let Ok(database_url) = std::env::var("DATABASE_URL") else {
            return Ok(());
        };
        let pool = PgPoolOptions::new()
            .max_connections(1)
            .connect(&database_url)
            .await?;
        setup_schema(&pool).await?;
        let dao = RecapDao::new(pool.clone());

        let run_id = dao
            .insert_subworker_run(&NewSubworkerRun::new(
                Uuid::new_v4(),
                "business",
                serde_json::json!({}),
            ))
            .await?;

        dao.upsert_diagnostics(
            run_id,
            &[DiagnosticEntry::new(
                "embed_seconds",
                serde_json::json!(1.23),
            )],
        )
        .await?;

        dao.upsert_diagnostics(
            run_id,
            &[DiagnosticEntry::new(
                "embed_seconds",
                serde_json::json!(2.34),
            )],
        )
        .await?;

        let row = sqlx::query("SELECT value FROM recap_subworker_diagnostics WHERE run_id = $1 AND metric = 'embed_seconds'")
            .bind(run_id)
            .fetch_one(&pool)
            .await?;
        let value: Value = row.get("value");
        assert_eq!(value, serde_json::json!(2.34));

        Ok(())
    }

    #[tokio::test]
    async fn upsert_genre_learning_record_inserts() -> anyhow::Result<()> {
        let Ok(database_url) = std::env::var("DATABASE_URL") else {
            return Ok(());
        };
        let pool = PgPoolOptions::new()
            .max_connections(1)
            .connect(&database_url)
            .await?;
        setup_schema(&pool).await?;
        let dao = RecapDao::new(pool.clone());

        let job_id = Uuid::new_v4();
        let now = Utc::now();
        let record = GenreLearningRecord::new(
            job_id,
            "article-1",
            vec![CoarseCandidateRecord {
                genre: "tech".to_string(),
                score: 0.9,
                keyword_support: 5,
                classifier_confidence: 0.88,
                tag_overlap_count: Some(1),
                graph_boost: Some(0.1),
                llm_confidence: None,
            }],
            RefineDecisionRecord {
                final_genre: "tech".to_string(),
                confidence: 0.9,
                strategy: "tag_consistency".to_string(),
                llm_trace_id: None,
                notes: None,
            },
            TagProfileRecord {
                top_tags: vec![TagSignalRecord {
                    label: "AI".to_string(),
                    confidence: 0.8,
                    source: Some("tag-generator".to_string()),
                    source_ts: Some(now),
                }],
                entropy: 0.0,
            },
            LearningTimestamps::new(now, now),
        );

        dao.upsert_genre_learning_record(&record).await?;

        let row = sqlx::query(
            r"SELECT job_id, article_id FROM recap_genre_learning_results WHERE job_id = $1",
        )
        .bind(job_id)
        .fetch_one(&pool)
        .await?;

        let stored_job_id: Uuid = row.get("job_id");
        let stored_article_id: String = row.get("article_id");

        assert_eq!(stored_job_id, job_id);
        assert_eq!(stored_article_id, "article-1");
        Ok(())
    }

    #[tokio::test]
    async fn save_genre_evaluation_inserts() -> anyhow::Result<()> {
        let Ok(database_url) = std::env::var("DATABASE_URL") else {
            return Ok(());
        };
        let pool = PgPoolOptions::new()
            .max_connections(1)
            .connect(&database_url)
            .await?;
        setup_schema(&pool).await?;
        let dao = RecapDao::new(pool.clone());

        use crate::store::models::{GenreEvaluationMetric, GenreEvaluationRun};

        let run = GenreEvaluationRun::new(
            "/app/data/golden_classification.json",
            16,
            0.526_315_789_473_684_2,
            0.416_666_666_666_666_63,
            0.465_116_279_069_767_4,
            13,
            35,
            19,
        );

        let metrics = vec![
            GenreEvaluationMetric::new("ai", 2, 0, 0, 1.0, 1.0, 1.0),
            GenreEvaluationMetric::new("business", 1, 1, 2, 0.5, 0.333_333_333_333_333_3, 0.4),
        ];

        dao.save_genre_evaluation(&run, &metrics).await?;

        // Verify run was inserted
        let row = sqlx::query(
            r"SELECT run_id, dataset_path, total_items, macro_precision FROM recap_genre_evaluation_runs WHERE run_id = $1",
        )
        .bind(run.run_id)
        .fetch_one(&pool)
        .await?;

        let stored_run_id: Uuid = row.get("run_id");
        let stored_path: String = row.get("dataset_path");
        let stored_total: i32 = row.get("total_items");
        let stored_macro_precision: f64 = row.get("macro_precision");

        assert_eq!(stored_run_id, run.run_id);
        assert_eq!(stored_path, "/app/data/golden_classification.json");
        assert_eq!(stored_total, 16);
        assert!((stored_macro_precision - 0.526_315_789_473_684_2).abs() < 0.0001);

        // Verify metrics were inserted
        let metric_rows = sqlx::query(
            r"SELECT genre, tp, fp, fn_count, precision, recall, f1_score FROM recap_genre_evaluation_metrics WHERE run_id = $1 ORDER BY genre",
        )
        .bind(run.run_id)
        .fetch_all(&pool)
        .await?;

        assert_eq!(metric_rows.len(), 2);

        let ai_row = &metric_rows[0];
        assert_eq!(ai_row.get::<String, _>("genre"), "ai");
        assert_eq!(ai_row.get::<i32, _>("tp"), 2);
        assert_eq!(ai_row.get::<i32, _>("fp"), 0);
        assert_eq!(ai_row.get::<i32, _>("fn_count"), 0);

        let business_row = &metric_rows[1];
        assert_eq!(business_row.get::<String, _>("genre"), "business");
        assert_eq!(business_row.get::<i32, _>("tp"), 1);
        assert_eq!(business_row.get::<i32, _>("fp"), 1);
        assert_eq!(business_row.get::<i32, _>("fn_count"), 2);

        Ok(())
    }
}
