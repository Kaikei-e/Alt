use anyhow::{Context, Result, ensure};
use chrono::Duration;
use serde_json::Value;
use sqlx::types::{
    Json,
    chrono::{DateTime, Utc},
};
use sqlx::{PgPool, Row};
use std::convert::TryFrom;
use uuid::Uuid;

use super::models::{
    ClusterEvidence, ClusterWithEvidence, DiagnosticEntry, GenreWithSummary, NewSubworkerRun,
    PersistedCluster, PersistedGenre, RawArticle, RecapJob, SubworkerRunStatus,
};
use crate::util::idempotency::try_acquire_job_lock;

#[derive(Debug, Clone)]
pub(crate) struct RecapDao {
    pool: PgPool,
}

impl RecapDao {
    pub(crate) fn new(pool: PgPool) -> Self {
        Self { pool }
    }

    /// アドバイザリロックを取得し、新しいジョブを作成する。
    ///
    /// ロックが取得できない場合は、既に他のワーカーがそのジョブを実行中であることを示します。
    ///
    /// # Returns
    /// - `Ok(Some(job_id))`: ロック取得成功、ジョブ作成完了
    /// - `Ok(None)`: ロック取得失敗、他のワーカーが実行中
    /// - `Err`: データベースエラー
    #[allow(dead_code)]
    pub async fn create_job_with_lock(
        &self,
        job_id: Uuid,
        note: Option<&str>,
    ) -> Result<Option<Uuid>> {
        let mut tx = self
            .pool
            .begin()
            .await
            .context("failed to begin transaction")?;

        // Try to acquire advisory lock
        let lock_acquired = try_acquire_job_lock(&mut *tx, job_id)
            .await
            .context("failed to acquire advisory lock")?;

        if !lock_acquired {
            // Another worker is already processing this job
            tx.rollback()
                .await
                .context("failed to rollback transaction")?;
            return Ok(None);
        }

        // Create job record
        sqlx::query(
            r"
            INSERT INTO recap_jobs (job_id, kicked_at, note)
            VALUES ($1, NOW(), $2)
            ON CONFLICT (job_id) DO NOTHING
            ",
        )
        .bind(job_id)
        .bind(note)
        .execute(&mut *tx)
        .await
        .context("failed to insert recap_jobs record")?;

        tx.commit().await.context("failed to commit transaction")?;

        Ok(Some(job_id))
    }

    /// 指定されたjob_idのジョブが存在するかチェックする。
    #[allow(dead_code)]
    pub async fn job_exists(&self, job_id: Uuid) -> Result<bool> {
        let row =
            sqlx::query("SELECT EXISTS(SELECT 1 FROM recap_jobs WHERE job_id = $1) as exists")
                .bind(job_id)
                .fetch_one(&self.pool)
                .await
                .context("failed to check job existence")?;

        let exists: bool = row
            .try_get("exists")
            .context("failed to get exists result")?;
        Ok(exists)
    }

    /// Raw記事をバックアップテーブルに保存する。
    pub async fn backup_raw_articles(&self, job_id: Uuid, articles: &[RawArticle]) -> Result<()> {
        if articles.is_empty() {
            return Ok(());
        }

        let mut tx = self
            .pool
            .begin()
            .await
            .context("failed to begin transaction")?;

        for article in articles {
            sqlx::query(
                r"
                INSERT INTO recap_job_articles
                    (job_id, article_id, title, fulltext_html, published_at, source_url, lang_hint, normalized_hash)
                VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
                ON CONFLICT (job_id, article_id) DO NOTHING
                ",
            )
            .bind(job_id)
            .bind(&article.article_id)
            .bind(&article.title)
            .bind(&article.fulltext_html)
            .bind(article.published_at)
            .bind(&article.source_url)
            .bind(&article.lang_hint)
            .bind(&article.normalized_hash)
            .execute(&mut *tx)
            .await
            .context("failed to insert raw article")?;
        }

        tx.commit().await.context("failed to commit raw articles")?;

        Ok(())
    }

    /// 前処理統計を保存する。
    pub async fn save_preprocess_metrics(
        &self,
        metrics: &crate::store::models::PreprocessMetrics,
    ) -> Result<()> {
        sqlx::query(
            r"
            INSERT INTO recap_preprocess_metrics
                (job_id, total_articles_fetched, articles_processed, articles_dropped_empty,
                 articles_html_cleaned, total_characters, avg_chars_per_article, languages_detected)
            VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
            ON CONFLICT (job_id) DO UPDATE SET
                total_articles_fetched = EXCLUDED.total_articles_fetched,
                articles_processed = EXCLUDED.articles_processed,
                articles_dropped_empty = EXCLUDED.articles_dropped_empty,
                articles_html_cleaned = EXCLUDED.articles_html_cleaned,
                total_characters = EXCLUDED.total_characters,
                avg_chars_per_article = EXCLUDED.avg_chars_per_article,
                languages_detected = EXCLUDED.languages_detected
            ",
        )
        .bind(metrics.job_id)
        .bind(metrics.total_articles_fetched)
        .bind(metrics.articles_processed)
        .bind(metrics.articles_dropped_empty)
        .bind(metrics.articles_html_cleaned)
        .bind(metrics.total_characters)
        .bind(metrics.avg_chars_per_article)
        .bind(&metrics.languages_detected)
        .execute(&self.pool)
        .await
        .context("failed to save preprocess metrics")?;

        Ok(())
    }

    /// 最終セクションを保存する。
    #[allow(dead_code)]
    pub async fn save_final_section(
        &self,
        section: &crate::store::models::RecapFinalSection,
    ) -> Result<i64> {
        let bullets_json =
            serde_json::to_value(&section.bullets_ja).context("failed to serialize bullets")?;

        let row = sqlx::query(
            r"
            INSERT INTO recap_final_sections
                (job_id, genre, title_ja, bullets_ja, model_name)
            VALUES ($1, $2, $3, $4, $5)
            ON CONFLICT (job_id, genre) DO UPDATE SET
                title_ja = EXCLUDED.title_ja,
                bullets_ja = EXCLUDED.bullets_ja,
                model_name = EXCLUDED.model_name,
                updated_at = NOW()
            RETURNING id
            ",
        )
        .bind(section.job_id)
        .bind(&section.genre)
        .bind(&section.title_ja)
        .bind(bullets_json)
        .bind(&section.model_name)
        .fetch_one(&self.pool)
        .await
        .context("failed to insert final section")?;

        Ok(row.get("id"))
    }

    /// 生成済みリキャップ出力を保存する。
    #[allow(dead_code)]
    pub async fn upsert_recap_output(
        &self,
        output: &crate::store::models::RecapOutput,
    ) -> Result<()> {
        sqlx::query(
            r#"
            INSERT INTO recap_outputs
                (job_id, genre, response_id, title_ja, summary_ja, bullets_ja, body_json)
            VALUES ($1, $2, $3, $4, $5, $6, $7)
            ON CONFLICT (job_id, genre) DO UPDATE SET
                response_id = EXCLUDED.response_id,
                title_ja = EXCLUDED.title_ja,
                summary_ja = EXCLUDED.summary_ja,
                bullets_ja = EXCLUDED.bullets_ja,
                body_json = EXCLUDED.body_json,
                updated_at = NOW()
            "#,
        )
        .bind(output.job_id)
        .bind(&output.genre)
        .bind(&output.response_id)
        .bind(&output.title_ja)
        .bind(&output.summary_ja)
        .bind(Json(output.bullets_ja.clone()))
        .bind(Json(output.body_json.clone()))
        .execute(&self.pool)
        .await
        .context("failed to upsert recap_outputs record")?;

        Ok(())
    }

    #[allow(dead_code)]
    pub(crate) async fn insert_subworker_run(&self, run: &NewSubworkerRun) -> Result<i64> {
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
        .fetch_one(&self.pool)
        .await
        .context("failed to insert recap_subworker_runs record")?;

        let id: i64 = row
            .try_get("id")
            .context("inserted run row missing id column")?;
        Ok(id)
    }

    #[allow(dead_code)]
    pub(crate) async fn mark_subworker_run_success(
        &self,
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
        .execute(&self.pool)
        .await
        .context("failed to update recap_subworker_runs with success state")?;

        Ok(())
    }

    #[allow(dead_code)]
    pub(crate) async fn mark_subworker_run_failure(
        &self,
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
        .execute(&self.pool)
        .await
        .context("failed to update recap_subworker_runs with failure state")?;

        Ok(())
    }

    #[allow(dead_code)]
    pub(crate) async fn insert_clusters(
        &self,
        run_id: i64,
        clusters: &[PersistedCluster],
    ) -> Result<()> {
        if clusters.is_empty() {
            return Ok(());
        }

        let mut tx = self
            .pool
            .begin()
            .await
            .context("failed to begin transaction for cluster insert")?;

        for cluster in clusters {
            let row = sqlx::query(
                r"
                INSERT INTO recap_subworker_clusters
                    (run_id, cluster_id, size, label, top_terms, stats)
                VALUES ($1, $2, $3, $4, $5, $6)
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
        &self,
        run_id: i64,
        diagnostics: &[DiagnosticEntry],
    ) -> Result<()> {
        if diagnostics.is_empty() {
            return Ok(());
        }

        let mut tx = self
            .pool
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
    pub(crate) async fn upsert_genre(&self, genre: &PersistedGenre) -> Result<()> {
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
        .execute(&self.pool)
        .await
        .context("failed to upsert recap section")?;

        Ok(())
    }

    /// Get the latest completed recap job for a given window
    pub(crate) async fn get_latest_completed_job(
        &self,
        window_days: i32,
    ) -> Result<Option<RecapJob>> {
        let row = sqlx::query(
            r#"
            SELECT job_id, MAX(created_at) AS created_at
            FROM recap_outputs
            GROUP BY job_id
            ORDER BY MAX(created_at) DESC
            LIMIT 1
            "#,
        )
        .fetch_optional(&self.pool)
        .await
        .context("failed to fetch latest completed job")?;

        match row {
            Some(row) => {
                let job_id: Uuid = row.try_get("job_id")?;
                let created_at: DateTime<Utc> = row.try_get("created_at")?;

                let window_duration = Duration::days(i64::from(window_days));
                let window_end = created_at;
                let window_start = window_end - window_duration;

                let total_articles = match sqlx::query(
                    r#"
                    SELECT COUNT(*) AS article_count
                    FROM recap_job_articles
                    WHERE job_id = $1
                    "#,
                )
                .bind(job_id)
                .fetch_one(&self.pool)
                .await
                {
                    Ok(article_row) => match article_row.try_get::<i64, _>("article_count") {
                        Ok(article_count) => i32::try_from(article_count).unwrap_or(i32::MAX),
                        Err(get_err) => {
                            tracing::warn!(
                                "Failed to read article count for job {}: {}. Falling back to 0.",
                                job_id,
                                get_err
                            );
                            0
                        }
                    },
                    Err(err) => {
                        tracing::warn!(
                            "Failed to count recap job articles for job {}: {}. Falling back to 0.",
                            job_id,
                            err
                        );
                        0
                    }
                };

                Ok(Some(RecapJob {
                    job_id,
                    started_at: created_at,
                    window_start,
                    window_end,
                    total_articles: Some(total_articles),
                }))
            }
            None => Ok(None),
        }
    }

    /// Get all genres for a job with their summaries
    pub(crate) async fn get_genres_by_job(&self, job_id: Uuid) -> Result<Vec<GenreWithSummary>> {
        let rows = sqlx::query(
            r#"
            SELECT genre AS genre_name, summary_ja
            FROM recap_outputs
            WHERE job_id = $1
            ORDER BY genre
            "#,
        )
        .bind(job_id)
        .fetch_all(&self.pool)
        .await
        .context("failed to fetch genres by job")?;

        let mut genres = Vec::new();
        for row in rows {
            genres.push(GenreWithSummary {
                genre_name: row.try_get("genre_name")?,
                summary_ja: row.try_get("summary_ja").ok(),
            });
        }

        Ok(genres)
    }

    /// Get all clusters for a genre with evidence
    pub(crate) async fn get_clusters_by_genre(
        &self,
        job_id: Uuid,
        genre_name: &str,
    ) -> Result<Vec<ClusterWithEvidence>> {
        // First get the run_id for this genre
        let run_row = sqlx::query(
            r"
            SELECT id
            FROM recap_subworker_runs
            WHERE job_id = $1 AND genre = $2 AND status = 'succeeded'
            ORDER BY started_at DESC
            LIMIT 1
            ",
        )
        .bind(job_id)
        .bind(genre_name)
        .fetch_optional(&self.pool)
        .await
        .context("failed to fetch subworker run")?;

        let run_id: i64 = match run_row {
            Some(row) => row.try_get("id")?,
            None => return Ok(Vec::new()),
        };

        // Get clusters
        let cluster_rows = sqlx::query(
            r#"
            SELECT id, cluster_id, top_terms
            FROM recap_subworker_clusters
            WHERE run_id = $1
            ORDER BY cluster_id
            "#,
        )
        .bind(run_id)
        .fetch_all(&self.pool)
        .await
        .context("failed to fetch clusters")?;

        let mut clusters = Vec::new();
        for cluster_row in cluster_rows {
            let cluster_row_id: i64 = cluster_row.try_get("id")?;
            let cluster_id: i32 = cluster_row.try_get("cluster_id")?;
            let top_terms_json: Json<Value> = cluster_row.try_get("top_terms")?;
            let top_terms: Option<Vec<String>> = serde_json::from_value(top_terms_json.0).ok();

            // Get evidence (sentences) for this cluster
            let evidence_rows = sqlx::query(
                r#"
                SELECT
                    s.source_article_id,
                    MAX(ra.title) AS title,
                    MAX(ra.source_url) AS source_url,
                    MAX(ra.published_at) AS published_at,
                    MAX(ra.lang_hint) AS lang_hint
                FROM recap_subworker_sentences s
                LEFT JOIN recap_job_articles ra
                    ON ra.job_id = $2 AND ra.article_id = s.source_article_id
                WHERE s.cluster_row_id = $1
                GROUP BY s.source_article_id
                ORDER BY MAX(ra.published_at) DESC NULLS LAST, s.source_article_id
                LIMIT 10
                "#,
            )
            .bind(cluster_row_id)
            .bind(job_id)
            .fetch_all(&self.pool)
            .await
            .context("failed to fetch evidence")?;

            let mut evidence = Vec::new();
            for ev_row in evidence_rows {
                let title = ev_row
                    .try_get::<Option<String>, _>("title")
                    .unwrap_or(None)
                    .unwrap_or_default();
                let source_url = ev_row
                    .try_get::<Option<String>, _>("source_url")
                    .unwrap_or(None)
                    .unwrap_or_default();
                let published_at = ev_row
                    .try_get::<Option<DateTime<Utc>>, _>("published_at")
                    .unwrap_or(None)
                    .unwrap_or_else(|| Utc::now());
                let lang = ev_row
                    .try_get::<Option<String>, _>("lang_hint")
                    .unwrap_or(None);

                evidence.push(ClusterEvidence {
                    article_id: ev_row.try_get::<String, _>("source_article_id")?,
                    title,
                    source_url,
                    published_at,
                    lang,
                });
            }

            clusters.push(ClusterWithEvidence {
                cluster_id,
                top_terms,
                evidence,
            });
        }

        Ok(clusters)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::store::models::{PersistedSentence, RecapOutput};
    use serde_json::Value;
    use sqlx::{Executor, Row, postgres::PgPoolOptions};
    use uuid::Uuid;

    async fn setup_schema(pool: &PgPool) -> Result<()> {
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

    #[tokio::test]
    async fn upsert_genre_inserts() -> Result<()> {
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
    async fn upsert_genre_updates_response() -> Result<()> {
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
    async fn upsert_recap_output_inserts() -> Result<()> {
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
    async fn subworker_run_lifecycle() -> Result<()> {
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
    async fn insert_clusters_with_sentences() -> Result<()> {
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
    async fn diagnostics_upsert() -> Result<()> {
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
}
