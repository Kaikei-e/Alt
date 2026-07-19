use crate::error::{RecapError, Result};
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
        if run.genre.trim().is_empty() {
            return Err(RecapError::Db(
                "subworker run requires a non-empty genre".to_string(),
            ));
        }

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
        .map_err(|e| {
            RecapError::Db(format!("failed to insert recap_subworker_runs record: {e}"))
        })?;

        let id: i64 = row
            .try_get("id")
            .map_err(|e| RecapError::Db(format!("inserted run row missing id column: {e}")))?;
        Ok(id)
    }

    #[allow(dead_code)]
    pub(crate) async fn mark_subworker_run_success(
        pool: &PgPool,
        run_id: i64,
        cluster_count: i32,
        response_payload: &Value,
    ) -> Result<()> {
        if cluster_count < 0 {
            return Err(RecapError::Db(
                "cluster_count must be non-negative".to_string(),
            ));
        }

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
        .map_err(|e| {
            RecapError::Db(format!(
                "failed to update recap_subworker_runs with success state: {e}"
            ))
        })?;

        Ok(())
    }

    #[allow(dead_code)]
    pub(crate) async fn mark_subworker_run_failure(
        pool: &PgPool,
        run_id: i64,
        status: SubworkerRunStatus,
        error_message: &str,
    ) -> Result<()> {
        if !matches!(
            status,
            SubworkerRunStatus::Partial | SubworkerRunStatus::Failed
        ) {
            return Err(RecapError::Db(
                "failure status must be partial or failed".to_string(),
            ));
        }

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
        .map_err(|e| {
            RecapError::Db(format!(
                "failed to update recap_subworker_runs with failure state: {e}"
            ))
        })?;

        Ok(())
    }

    /// Insert clusters and their sentences using batch operations.
    ///
    /// Uses two-phase batch insert:
    /// 1. Batch upsert all clusters, returning their row IDs
    /// 2. Batch upsert all sentences with mapped cluster_row_ids
    ///
    /// This reduces N+M queries to 2 queries for better performance.
    #[allow(dead_code)]
    #[allow(clippy::too_many_lines)]
    pub(crate) async fn insert_clusters(
        pool: &PgPool,
        run_id: i64,
        clusters: &[PersistedCluster],
    ) -> Result<()> {
        if clusters.is_empty() {
            return Ok(());
        }

        let mut tx = pool.begin().await.map_err(|e| {
            RecapError::Db(format!(
                "failed to begin transaction for cluster insert: {e}"
            ))
        })?;

        // Phase 1: Batch upsert clusters using UNNEST
        let cluster_ids: Vec<i32> = clusters.iter().map(|c| c.cluster_id).collect();
        let sizes: Vec<i32> = clusters.iter().map(|c| c.size).collect();
        let labels: Vec<Option<String>> = clusters.iter().map(|c| c.label.clone()).collect();
        let top_terms: Vec<serde_json::Value> =
            clusters.iter().map(|c| c.top_terms.clone()).collect();
        let stats: Vec<serde_json::Value> = clusters.iter().map(|c| c.stats.clone()).collect();

        let cluster_rows = sqlx::query(
            r"
            INSERT INTO recap_subworker_clusters (run_id, cluster_id, size, label, top_terms, stats)
            SELECT $1, cluster_id, size, label, top_terms, stats
            FROM UNNEST($2::int[], $3::int[], $4::text[], $5::jsonb[], $6::jsonb[])
                AS t(cluster_id, size, label, top_terms, stats)
            ON CONFLICT (run_id, cluster_id) DO UPDATE SET
                size = EXCLUDED.size,
                label = EXCLUDED.label,
                top_terms = EXCLUDED.top_terms,
                stats = EXCLUDED.stats
            RETURNING id, cluster_id
            ",
        )
        .bind(run_id)
        .bind(&cluster_ids)
        .bind(&sizes)
        .bind(&labels)
        .bind(&top_terms)
        .bind(&stats)
        .fetch_all(&mut *tx)
        .await
        .map_err(|e| {
            RecapError::Db(format!(
                "failed to batch insert recap_subworker_clusters: {e}"
            ))
        })?;

        // Build cluster_id -> row_id mapping
        let cluster_id_to_row_id: std::collections::HashMap<i32, i64> = cluster_rows
            .iter()
            .map(|row| {
                let cluster_id: i32 = row.try_get("cluster_id").unwrap_or(0);
                let row_id: i64 = row.try_get("id").unwrap_or(0);
                (cluster_id, row_id)
            })
            .collect();

        // Phase 2: Batch upsert sentences using UNNEST
        let mut sentence_cluster_row_ids: Vec<i64> = Vec::new();
        let mut sentence_article_ids: Vec<String> = Vec::new();
        let mut sentence_paragraph_idxs: Vec<Option<i32>> = Vec::new();
        let mut sentence_ids: Vec<i32> = Vec::new();
        let mut sentence_texts: Vec<String> = Vec::new();
        let mut sentence_langs: Vec<String> = Vec::new();
        let mut sentence_scores: Vec<f32> = Vec::new();

        for cluster in clusters {
            let cluster_row_id = *cluster_id_to_row_id
                .get(&cluster.cluster_id)
                .ok_or_else(|| RecapError::Db("missing cluster_row_id mapping".to_string()))?;

            for sentence in &cluster.sentences {
                sentence_cluster_row_ids.push(cluster_row_id);
                sentence_article_ids.push(sentence.article_id.clone());
                sentence_paragraph_idxs.push(sentence.paragraph_idx);
                sentence_ids.push(sentence.sentence_id);
                sentence_texts.push(sentence.text.clone());
                sentence_langs.push(sentence.lang.clone());
                sentence_scores.push(sentence.score);
            }
        }

        if !sentence_cluster_row_ids.is_empty() {
            sqlx::query(
                r"
                INSERT INTO recap_subworker_sentences
                    (cluster_row_id, source_article_id, paragraph_idx, sentence_id, sentence_text, lang, score)
                SELECT cluster_row_id, source_article_id, paragraph_idx, sentence_id, sentence_text, lang, score
                FROM UNNEST($1::bigint[], $2::text[], $3::int[], $4::int[], $5::text[], $6::text[], $7::real[])
                    AS t(cluster_row_id, source_article_id, paragraph_idx, sentence_id, sentence_text, lang, score)
                ON CONFLICT (cluster_row_id, source_article_id, sentence_id) DO UPDATE
                SET sentence_text = EXCLUDED.sentence_text,
                    lang = EXCLUDED.lang,
                    score = EXCLUDED.score,
                    paragraph_idx = EXCLUDED.paragraph_idx
                ",
            )
            .bind(&sentence_cluster_row_ids)
            .bind(&sentence_article_ids)
            .bind(&sentence_paragraph_idxs)
            .bind(&sentence_ids)
            .bind(&sentence_texts)
            .bind(&sentence_langs)
            .bind(&sentence_scores)
            .execute(&mut *tx)
            .await
            .map_err(|e| RecapError::Db(format!("failed to batch insert recap_subworker_sentences: {e}")))?;
        }

        tx.commit().await.map_err(|e| {
            RecapError::Db(format!("failed to commit cluster insert transaction: {e}"))
        })?;

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

        let mut tx = pool.begin().await.map_err(|e| {
            RecapError::Db(format!(
                "failed to begin transaction for diagnostics upsert: {e}"
            ))
        })?;

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
            .map_err(|e| RecapError::Db(format!("failed to upsert diagnostics entry: {e}")))?;
        }

        tx.commit().await.map_err(|e| {
            RecapError::Db(format!("failed to commit diagnostics transaction: {e}"))
        })?;

        Ok(())
    }

    /// `executor` は `&PgPool` と `&mut Transaction<'_, Postgres>` の両方を
    /// 受け付ける — `persist_genre_output` が `upsert_recap_output` と
    /// 同一トランザクションで実行するため。
    #[allow(dead_code)]
    pub(crate) async fn upsert_genre<'e, E>(executor: E, genre: &PersistedGenre) -> Result<()>
    where
        E: sqlx::postgres::PgExecutor<'e>,
    {
        if genre.genre.trim().is_empty() {
            return Err(RecapError::Db(
                "genre payload must include a non-empty genre name".to_string(),
            ));
        }

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
        .execute(executor)
        .await
        .map_err(|e| RecapError::Db(format!("failed to upsert recap section: {e}")))?;

        Ok(())
    }
}
