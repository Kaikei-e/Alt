//! DAO integration tests
//!
//! These tests require a DATABASE_URL environment variable to run.

#![allow(dead_code)]

use super::JobStatus;
use super::compat::RecapDao;
use super::impls::UnifiedDao;
use crate::store::models::{
    CoarseCandidateRecord, DiagnosticEntry, GenreEvaluationMetric, GenreEvaluationRun,
    GenreLearningRecord, LearningTimestamps, NewSubworkerRun, PersistedCluster, PersistedGenre,
    PersistedSentence, RecapOutput, RefineDecisionRecord, TagProfileRecord, TagSignalRecord,
};
use chrono::Utc;
use serde_json::Value;
use sqlx::postgres::PgPoolOptions;
use sqlx::{Executor, PgPool, Row};
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

async fn setup_job_tables(pool: &PgPool) -> anyhow::Result<()> {
    pool.execute(
        r"
        CREATE TABLE IF NOT EXISTS recap_jobs (
            job_id UUID PRIMARY KEY,
            status TEXT NOT NULL DEFAULT 'pending',
            last_stage TEXT,
            kicked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            note TEXT
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
    let dao = UnifiedDao::new(pool.clone());
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
    let dao = UnifiedDao::new(pool.clone());
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
    let dao = UnifiedDao::new(pool.clone());

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
    let dao = UnifiedDao::new(pool.clone());

    let job_id = Uuid::new_v4();
    let run = NewSubworkerRun::new(job_id, "ai", serde_json::json!({"articles": 5}));
    let run_id = dao.insert_subworker_run(&run).await?;

    dao.mark_subworker_run_success(run_id, 3, &serde_json::json!({"summary": "done"}))
        .await?;

    let row = sqlx::query(
        "SELECT status, cluster_count, response_payload FROM recap_subworker_runs WHERE id = $1",
    )
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
    let dao = UnifiedDao::new(pool.clone());

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
    let dao = UnifiedDao::new(pool.clone());

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

    let row = sqlx::query(
        "SELECT value FROM recap_subworker_diagnostics WHERE run_id = $1 AND metric = 'embed_seconds'",
    )
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
    let dao = UnifiedDao::new(pool.clone());

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
    let dao = UnifiedDao::new(pool.clone());

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

#[tokio::test]
async fn update_job_status_warns_on_nonexistent_job() -> anyhow::Result<()> {
    let Ok(database_url) = std::env::var("DATABASE_URL") else {
        return Ok(());
    };
    let pool = PgPoolOptions::new()
        .max_connections(1)
        .connect(&database_url)
        .await?;
    setup_job_tables(&pool).await?;
    let dao = UnifiedDao::new(pool.clone());

    let nonexistent_job_id = Uuid::new_v4();

    // Should succeed (returns Ok) but log a warning
    let result = dao
        .update_job_status(nonexistent_job_id, JobStatus::Running, Some("test_stage"))
        .await;

    assert!(
        result.is_ok(),
        "update_job_status should return Ok even for non-existent job"
    );

    // Verify no job was created
    let row = sqlx::query("SELECT COUNT(*) as count FROM recap_jobs WHERE job_id = $1")
        .bind(nonexistent_job_id)
        .fetch_one(&pool)
        .await?;
    let count: i64 = row.get("count");
    assert_eq!(count, 0, "No job should be created for non-existent job_id");

    Ok(())
}

#[tokio::test]
async fn update_job_status_updates_existing_job() -> anyhow::Result<()> {
    let Ok(database_url) = std::env::var("DATABASE_URL") else {
        return Ok(());
    };
    let pool = PgPoolOptions::new()
        .max_connections(1)
        .connect(&database_url)
        .await?;
    setup_job_tables(&pool).await?;
    let dao = UnifiedDao::new(pool.clone());

    let job_id = Uuid::new_v4();

    // Insert a job first
    sqlx::query(
        r"
        INSERT INTO recap_jobs (job_id, status, kicked_at, updated_at)
        VALUES ($1, 'pending', NOW(), NOW())
        ON CONFLICT (job_id) DO NOTHING
        ",
    )
    .bind(job_id)
    .execute(&pool)
    .await?;

    // Update job status
    let result = dao
        .update_job_status(job_id, JobStatus::Running, Some("fetch"))
        .await;

    assert!(result.is_ok(), "update_job_status should succeed");

    // Verify the update
    let row = sqlx::query("SELECT status, last_stage FROM recap_jobs WHERE job_id = $1")
        .bind(job_id)
        .fetch_one(&pool)
        .await?;
    let status: String = row.get("status");
    let last_stage: Option<String> = row.get("last_stage");

    assert_eq!(status, "running");
    assert_eq!(last_stage, Some("fetch".to_string()));

    Ok(())
}

// ============================================================================
// Job Status History Tests (Immutable Event Sourcing)
// ============================================================================

async fn setup_status_history_tables(pool: &PgPool) -> anyhow::Result<()> {
    pool.execute(
        r"
        CREATE TABLE IF NOT EXISTS recap_job_status_history (
            id BIGSERIAL PRIMARY KEY,
            job_id UUID NOT NULL REFERENCES recap_jobs(job_id) ON DELETE CASCADE,
            status TEXT NOT NULL,
            stage TEXT,
            transitioned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            reason TEXT,
            actor TEXT DEFAULT 'system',
            CONSTRAINT chk_status_history CHECK (status IN ('pending', 'running', 'completed', 'failed'))
        );
        CREATE INDEX IF NOT EXISTS idx_job_status_history_job_id ON recap_job_status_history(job_id);
        CREATE INDEX IF NOT EXISTS idx_job_status_history_job_latest ON recap_job_status_history(job_id, id DESC);
        ",
    )
    .await?;
    Ok(())
}

#[tokio::test]
async fn record_status_transition_creates_immutable_record() -> anyhow::Result<()> {
    let Ok(database_url) = std::env::var("DATABASE_URL") else {
        return Ok(());
    };
    let pool = PgPoolOptions::new()
        .max_connections(1)
        .connect(&database_url)
        .await?;
    setup_job_tables(&pool).await?;
    setup_status_history_tables(&pool).await?;
    let dao = UnifiedDao::new(pool.clone());

    let job_id = Uuid::new_v4();

    // Insert a job first
    sqlx::query(
        r"INSERT INTO recap_jobs (job_id, status, kicked_at, updated_at)
          VALUES ($1, 'pending', NOW(), NOW())",
    )
    .bind(job_id)
    .execute(&pool)
    .await?;

    // Record a status transition
    let transition_id = dao
        .record_status_transition(
            job_id,
            JobStatus::Running,
            Some("fetch"),
            None,
            super::types::StatusTransitionActor::System,
        )
        .await?;

    assert!(transition_id > 0, "Should return a valid transition ID");

    // Verify the record was created
    let row = sqlx::query(
        "SELECT job_id, status, stage, actor FROM recap_job_status_history WHERE id = $1",
    )
    .bind(transition_id)
    .fetch_one(&pool)
    .await?;

    let stored_job_id: Uuid = row.get("job_id");
    let stored_status: String = row.get("status");
    let stored_stage: Option<String> = row.get("stage");
    let stored_actor: String = row.get("actor");

    assert_eq!(stored_job_id, job_id);
    assert_eq!(stored_status, "running");
    assert_eq!(stored_stage, Some("fetch".to_string()));
    assert_eq!(stored_actor, "system");

    Ok(())
}

#[tokio::test]
async fn status_history_preserves_all_transitions() -> anyhow::Result<()> {
    let Ok(database_url) = std::env::var("DATABASE_URL") else {
        return Ok(());
    };
    let pool = PgPoolOptions::new()
        .max_connections(1)
        .connect(&database_url)
        .await?;
    setup_job_tables(&pool).await?;
    setup_status_history_tables(&pool).await?;
    let dao = UnifiedDao::new(pool.clone());

    let job_id = Uuid::new_v4();

    // Insert a job
    sqlx::query(
        r"INSERT INTO recap_jobs (job_id, status, kicked_at, updated_at)
          VALUES ($1, 'pending', NOW(), NOW())",
    )
    .bind(job_id)
    .execute(&pool)
    .await?;

    // Record multiple transitions
    dao.record_status_transition(
        job_id,
        JobStatus::Running,
        Some("fetch"),
        None,
        super::types::StatusTransitionActor::System,
    )
    .await?;

    dao.record_status_transition(
        job_id,
        JobStatus::Running,
        Some("preprocess"),
        None,
        super::types::StatusTransitionActor::System,
    )
    .await?;

    dao.record_status_transition(
        job_id,
        JobStatus::Failed,
        Some("genre"),
        Some("timeout error"),
        super::types::StatusTransitionActor::System,
    )
    .await?;

    // Get all transitions
    let history = dao.get_status_history(job_id).await?;

    assert_eq!(history.len(), 3, "Should have 3 transitions recorded");
    assert_eq!(history[0].status, JobStatus::Running);
    assert_eq!(history[0].stage.as_deref(), Some("fetch"));
    assert_eq!(history[1].status, JobStatus::Running);
    assert_eq!(history[1].stage.as_deref(), Some("preprocess"));
    assert_eq!(history[2].status, JobStatus::Failed);
    assert_eq!(history[2].stage.as_deref(), Some("genre"));
    assert_eq!(history[2].reason.as_deref(), Some("timeout error"));

    Ok(())
}

#[tokio::test]
async fn update_job_status_with_history_is_atomic() -> anyhow::Result<()> {
    let Ok(database_url) = std::env::var("DATABASE_URL") else {
        return Ok(());
    };
    let pool = PgPoolOptions::new()
        .max_connections(1)
        .connect(&database_url)
        .await?;
    setup_job_tables(&pool).await?;
    setup_status_history_tables(&pool).await?;
    let dao = UnifiedDao::new(pool.clone());

    let job_id = Uuid::new_v4();

    // Insert a job
    sqlx::query(
        r"INSERT INTO recap_jobs (job_id, status, kicked_at, updated_at)
          VALUES ($1, 'pending', NOW(), NOW())",
    )
    .bind(job_id)
    .execute(&pool)
    .await?;

    // Update status with history
    dao.update_job_status_with_history(job_id, JobStatus::Completed, Some("persist"), None)
        .await?;

    // Verify recap_jobs was updated
    let job_row = sqlx::query("SELECT status, last_stage FROM recap_jobs WHERE job_id = $1")
        .bind(job_id)
        .fetch_one(&pool)
        .await?;
    let status: String = job_row.get("status");
    let last_stage: Option<String> = job_row.get("last_stage");

    assert_eq!(status, "completed");
    assert_eq!(last_stage, Some("persist".to_string()));

    // Verify history was also recorded
    let history = dao.get_status_history(job_id).await?;
    assert!(
        !history.is_empty(),
        "History should have at least one record"
    );
    assert_eq!(history.last().unwrap().status, JobStatus::Completed);
    assert_eq!(history.last().unwrap().stage.as_deref(), Some("persist"));

    Ok(())
}

#[tokio::test]
async fn update_job_status_with_history_records_error_reason() -> anyhow::Result<()> {
    let Ok(database_url) = std::env::var("DATABASE_URL") else {
        return Ok(());
    };
    let pool = PgPoolOptions::new()
        .max_connections(1)
        .connect(&database_url)
        .await?;
    setup_job_tables(&pool).await?;
    setup_status_history_tables(&pool).await?;
    let dao = UnifiedDao::new(pool.clone());

    let job_id = Uuid::new_v4();

    // Insert a job
    sqlx::query(
        r"INSERT INTO recap_jobs (job_id, status, kicked_at, updated_at)
          VALUES ($1, 'pending', NOW(), NOW())",
    )
    .bind(job_id)
    .execute(&pool)
    .await?;

    // Update status with error reason
    let error_reason = "Connection timeout: failed to connect to subworker";
    dao.update_job_status_with_history(
        job_id,
        JobStatus::Failed,
        Some("dispatch"),
        Some(error_reason),
    )
    .await?;

    // Verify history contains the error reason
    let history = dao.get_status_history(job_id).await?;
    assert_eq!(history.len(), 1);
    assert_eq!(history[0].status, JobStatus::Failed);
    assert_eq!(history[0].reason.as_deref(), Some(error_reason));

    Ok(())
}
