use anyhow::{Context, Result, ensure};
use serde_json::Value;
use sqlx::types::Json;
use sqlx::{PgPool, Row};

use super::super::models::{
    DiagnosticEntry, NewSubworkerRun, PersistedCluster, PersistedGenre, SubworkerRunStatus,
};

pub(crate) struct RecapDao;

impl RecapDao {
    #[allow(dead_code)]
    pub(crate) async fn insert_subworker_run(pool: &PgPool, run: &NewSubworkerRun) -> Result<i64> {
        ensure!(
            !run.genre.trim().is_empty(),
            "subworker run requires a non-empty genre"
        );

        let row = sqlx::query(
            r"
            INSERT INTO recap_subworker_runs (job_id, genre, status, request_payload)
            VALUES ($1, $2, $3, $4)
            RETURNING id
            ",
        )
        .bind(run.job_id)
        .bind(&run.genre)
        .bind(run.status.as_str())
        .bind(Json(run.request_payload.clone()))
        .fetch_one(pool)
        .await
        .context("failed to insert recap_subworker_runs record")?;

        let id: i64 = row
            .try_get("id")
            .context("inserted run row missing id column")?;
        Ok(id)
    }

    #[allow(dead_code)]
    pub(crate) async fn mark_subworker_run_success(
        pool: &PgPool,
        run_id: i64,
        cluster_count: i32,
        response_payload: &Value,
    ) -> Result<()> {
        ensure!(cluster_count >= 0, "cluster_count must be non-negative");

        sqlx::query(
            r"
            UPDATE recap_subworker_runs
            SET status = 'succeeded',
                cluster_count = $2,
                finished_at = NOW(),
                response_payload = $3,
                error_message = NULL
            WHERE id = $1
            ",
        )
        .bind(run_id)
        .bind(cluster_count)
        .bind(Json(response_payload.clone()))
        .execute(pool)
        .await
        .context("failed to update recap_subworker_runs with success state")?;

        Ok(())
    }

    #[allow(dead_code)]
    pub(crate) async fn mark_subworker_run_failure(
        pool: &PgPool,
        run_id: i64,
        status: SubworkerRunStatus,
        error_message: &str,
    ) -> Result<()> {
        ensure!(
            matches!(
                status,
                SubworkerRunStatus::Partial | SubworkerRunStatus::Failed
            ),
            "failure status must be partial or failed"
        );

        sqlx::query(
            r"
            UPDATE recap_subworker_runs
            SET status = $2,
                finished_at = NOW(),
                error_message = $3
            WHERE id = $1
            ",
        )
        .bind(run_id)
        .bind(status.as_str())
        .bind(error_message)
        .execute(pool)
        .await
        .context("failed to update recap_subworker_runs with failure state")?;

        Ok(())
    }

    #[allow(dead_code)]
    pub(crate) async fn insert_clusters(
        pool: &PgPool,
        run_id: i64,
        clusters: &[PersistedCluster],
    ) -> Result<()> {
        if clusters.is_empty() {
            return Ok(());
        }

        let mut tx = pool
            .begin()
            .await
            .context("failed to begin transaction for cluster insert")?;

        for cluster in clusters {
            let row = sqlx::query(
                r"
                INSERT INTO recap_subworker_clusters
                    (run_id, cluster_id, size, label, top_terms, stats)
                VALUES ($1, $2, $3, $4, $5, $6)
                ON CONFLICT (run_id, cluster_id) DO UPDATE SET
                    size = EXCLUDED.size,
                    label = EXCLUDED.label,
                    top_terms = EXCLUDED.top_terms,
                    stats = EXCLUDED.stats
                RETURNING id
                ",
            )
            .bind(run_id)
            .bind(cluster.cluster_id)
            .bind(cluster.size)
            .bind(&cluster.label)
            .bind(Json(cluster.top_terms.clone()))
            .bind(Json(cluster.stats.clone()))
            .fetch_one(&mut *tx)
            .await
            .context("failed to insert recap_subworker_cluster")?;

            let cluster_row_id: i64 = row
                .try_get("id")
                .context("cluster insert missing id column")?;

            for sentence in &cluster.sentences {
                sqlx::query(
                    r"
                    INSERT INTO recap_subworker_sentences
                        (cluster_row_id, source_article_id, paragraph_idx, sentence_id, sentence_text, lang, score)
                    VALUES ($1, $2, $3, $4, $5, $6, $7)
                    ON CONFLICT (cluster_row_id, source_article_id, sentence_id) DO UPDATE
                    SET sentence_text = EXCLUDED.sentence_text,
                        lang = EXCLUDED.lang,
                        score = EXCLUDED.score,
                        paragraph_idx = EXCLUDED.paragraph_idx
                    ",
                )
                .bind(cluster_row_id)
                .bind(&sentence.article_id)
                .bind(sentence.paragraph_idx)
                .bind(sentence.sentence_id)
                .bind(&sentence.text)
                .bind(&sentence.lang)
                .bind(sentence.score)
                .execute(&mut *tx)
                .await
                .context("failed to insert recap_subworker_sentence")?;
            }
        }

        tx.commit()
            .await
            .context("failed to commit cluster insert transaction")?;

        Ok(())
    }

    #[allow(dead_code)]
    pub(crate) async fn upsert_diagnostics(
        pool: &PgPool,
        run_id: i64,
        diagnostics: &[DiagnosticEntry],
    ) -> Result<()> {
        if diagnostics.is_empty() {
            return Ok(());
        }

        let mut tx = pool
            .begin()
            .await
            .context("failed to begin transaction for diagnostics upsert")?;

        for entry in diagnostics {
            sqlx::query(
                r"
                INSERT INTO recap_subworker_diagnostics (run_id, metric, value)
                VALUES ($1, $2, $3)
                ON CONFLICT (run_id, metric)
                DO UPDATE SET value = EXCLUDED.value
                ",
            )
            .bind(run_id)
            .bind(&entry.key)
            .bind(Json(entry.value.clone()))
            .execute(&mut *tx)
            .await
            .context("failed to upsert diagnostics entry")?;
        }

        tx.commit()
            .await
            .context("failed to commit diagnostics transaction")?;

        Ok(())
    }

    #[allow(dead_code)]
    pub(crate) async fn upsert_genre(pool: &PgPool, genre: &PersistedGenre) -> Result<()> {
        ensure!(
            !genre.genre.trim().is_empty(),
            "genre payload must include a non-empty genre name"
        );

        sqlx::query(
            r"
            INSERT INTO recap_sections (job_id, genre, response_id)
            VALUES ($1, $2, $3)
            ON CONFLICT (job_id, genre)
            DO UPDATE SET response_id = EXCLUDED.response_id
            ",
        )
        .bind(genre.job_id)
        .bind(&genre.genre)
        .bind(&genre.response_id)
        .execute(pool)
        .await
        .context("failed to upsert recap section")?;

        Ok(())
    }
}
