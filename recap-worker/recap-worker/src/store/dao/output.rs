use anyhow::{Context, Result};
use chrono::{DateTime, Duration, Utc};
use serde_json::Value;
use sqlx::types::Json;
use sqlx::{PgPool, Row};
use std::collections::HashMap;
use uuid::Uuid;

use super::super::models::{ClusterEvidence, ClusterWithEvidence, GenreWithSummary, RecapJob};

pub(crate) struct RecapDao;

impl RecapDao {
    /// 最終セクションを保存する。
    #[allow(dead_code)]
    pub async fn save_final_section(
        pool: &PgPool,
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
        .fetch_one(pool)
        .await
        .context("failed to insert final section")?;

        Ok(row.get("id"))
    }

    /// 生成済みリキャップ出力を保存する。
    #[allow(dead_code)]
    pub async fn upsert_recap_output(
        pool: &PgPool,
        output: &crate::store::models::RecapOutput,
    ) -> Result<()> {
        sqlx::query(
            r"
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
            ",
        )
        .bind(output.job_id)
        .bind(&output.genre)
        .bind(&output.response_id)
        .bind(&output.title_ja)
        .bind(&output.summary_ja)
        .bind(Json(output.bullets_ja.clone()))
        .bind(Json(output.body_json.clone()))
        .execute(pool)
        .await
        .context("failed to upsert recap_outputs record")?;

        Ok(())
    }

    /// 指定されたjob_idとgenreのrecap_outputからbody_jsonを取得する。
    pub(crate) async fn get_recap_output_body_json(
        pool: &PgPool,
        job_id: Uuid,
        genre: &str,
    ) -> Result<Option<Value>> {
        let row = sqlx::query(
            r"
            SELECT body_json FROM recap_outputs
            WHERE job_id = $1 AND genre = $2
            ",
        )
        .bind(job_id)
        .bind(genre)
        .fetch_optional(pool)
        .await
        .context("failed to fetch recap output body_json")?;

        if let Some(row) = row {
            let body_json: Json<Value> = row.try_get("body_json")?;
            Ok(Some(body_json.0))
        } else {
            Ok(None)
        }
    }

    /// Get the latest completed recap job for a given window
    pub(crate) async fn get_latest_completed_job(
        pool: &PgPool,
        window_days: i32,
    ) -> Result<Option<RecapJob>> {
        let row = sqlx::query(
            r"
            SELECT job_id, MAX(created_at) AS created_at
            FROM recap_outputs
            GROUP BY job_id
            ORDER BY MAX(created_at) DESC
            LIMIT 1
            ",
        )
        .fetch_optional(pool)
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
                    r"
                    SELECT COUNT(*) AS article_count
                    FROM recap_job_articles
                    WHERE job_id = $1
                    ",
                )
                .bind(job_id)
                .fetch_one(pool)
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
    pub(crate) async fn get_genres_by_job(
        pool: &PgPool,
        job_id: Uuid,
    ) -> Result<Vec<GenreWithSummary>> {
        let rows = sqlx::query(
            r"
            SELECT genre AS genre_name, summary_ja
            FROM recap_outputs
            WHERE job_id = $1
            ORDER BY genre
            ",
        )
        .bind(job_id)
        .fetch_all(pool)
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

    /// Load all clusters grouped by genre in a single query.
    pub(crate) async fn get_clusters_by_job(
        pool: &PgPool,
        job_id: Uuid,
    ) -> Result<HashMap<String, Vec<ClusterWithEvidence>>> {
        let rows = sqlx::query(
            r"
            WITH latest_runs AS (
                SELECT id, genre
                FROM (
                    SELECT id,
                           genre,
                           ROW_NUMBER() OVER (PARTITION BY genre ORDER BY started_at DESC) AS rn
                    FROM recap_subworker_runs
                    WHERE job_id = $1 AND status = 'succeeded'
                ) ranked
                WHERE rn = 1
            )
            SELECT
                lr.genre,
                c.id AS cluster_row_id,
                c.cluster_id,
                c.top_terms,
                ce.article_id AS evidence_article_id,
                ce.title AS evidence_title,
                ce.source_url AS evidence_source_url,
                ce.published_at AS evidence_published_at,
                ce.lang AS evidence_lang,
                ce.rank AS evidence_rank,
                ra.title AS article_title,
                ra.source_url AS article_source_url,
                ra.published_at AS article_published_at,
                ra.lang_hint AS article_lang_hint
            FROM latest_runs lr
            JOIN recap_subworker_clusters c ON c.run_id = lr.id
            LEFT JOIN recap_cluster_evidence ce ON ce.cluster_row_id = c.id
            LEFT JOIN recap_job_articles ra
                ON ra.job_id = $1 AND ra.article_id = ce.article_id
            ORDER BY lr.genre, c.cluster_id, ce.rank NULLS LAST
            ",
        )
        .bind(job_id)
        .fetch_all(pool)
        .await
        .context("failed to fetch cluster bundle")?;

        let mut clusters_by_row = process_cluster_rows(rows)?;

        let missing: Vec<i64> = clusters_by_row
            .iter()
            .filter(|(_, entry)| entry.1.evidence.is_empty())
            .map(|(cluster_row_id, _)| *cluster_row_id)
            .collect();

        if !missing.is_empty() {
            let fallback = fetch_evidence_from_sentences(pool, job_id, &missing).await?;

            for (cluster_row_id, evidence) in fallback {
                if let Some(entry) = clusters_by_row.get_mut(&cluster_row_id) {
                    entry.1.evidence = evidence;
                }
            }
        }

        let mut genre_map: HashMap<String, Vec<ClusterWithEvidence>> = HashMap::new();
        for (_, (genre, cluster)) in clusters_by_row {
            genre_map.entry(genre).or_default().push(cluster);
        }

        for clusters in genre_map.values_mut() {
            clusters.sort_by_key(|c| c.cluster_id);
        }

        Ok(genre_map)
    }
}

fn process_cluster_rows(
    rows: Vec<sqlx::postgres::PgRow>,
) -> Result<HashMap<i64, (String, ClusterWithEvidence)>> {
    let mut clusters_by_row: HashMap<i64, (String, ClusterWithEvidence)> = HashMap::new();

    for row in rows {
        let genre: String = row.try_get("genre")?;
        let cluster_row_id: i64 = row.try_get("cluster_row_id")?;
        let cluster_id: i32 = row.try_get("cluster_id")?;
        let top_terms_json: Json<Value> = row.try_get("top_terms")?;
        let top_terms: Option<Vec<String>> = serde_json::from_value(top_terms_json.0).ok();

        let entry = clusters_by_row.entry(cluster_row_id).or_insert_with(|| {
            (
                genre.clone(),
                ClusterWithEvidence {
                    cluster_id,
                    top_terms: top_terms.clone(),
                    evidence: Vec::new(),
                },
            )
        });

        if let Some(article_id) = row.try_get::<Option<String>, _>("evidence_article_id")? {
            let title = row
                .try_get::<Option<String>, _>("evidence_title")?
                .or_else(|| {
                    row.try_get::<Option<String>, _>("article_title")
                        .ok()
                        .flatten()
                })
                .unwrap_or_default();
            let source_url = row
                .try_get::<Option<String>, _>("evidence_source_url")?
                .or_else(|| {
                    row.try_get::<Option<String>, _>("article_source_url")
                        .ok()
                        .flatten()
                })
                .unwrap_or_default();
            let published_at = row
                .try_get::<Option<DateTime<Utc>>, _>("evidence_published_at")?
                .or_else(|| {
                    row.try_get::<Option<DateTime<Utc>>, _>("article_published_at")
                        .ok()
                        .flatten()
                })
                .unwrap_or_else(|| Utc::now());
            let lang = row
                .try_get::<Option<String>, _>("evidence_lang")?
                .or_else(|| {
                    row.try_get::<Option<String>, _>("article_lang_hint")
                        .ok()
                        .flatten()
                });

            entry.1.evidence.push(ClusterEvidence {
                article_id,
                title,
                source_url,
                published_at,
                lang,
            });
        }
    }

    Ok(clusters_by_row)
}

async fn fetch_evidence_from_sentences(
    pool: &PgPool,
    job_id: Uuid,
    cluster_row_ids: &[i64],
) -> Result<HashMap<i64, Vec<ClusterEvidence>>> {
    if cluster_row_ids.is_empty() {
        return Ok(HashMap::new());
    }

    let rows = sqlx::query(
        r"
        WITH ranked AS (
            SELECT
                s.cluster_row_id,
                s.source_article_id,
                MAX(ra.title) AS title,
                MAX(ra.source_url) AS source_url,
                MAX(ra.published_at) AS published_at,
                MAX(ra.lang_hint) AS lang_hint,
                ROW_NUMBER() OVER (
                    PARTITION BY s.cluster_row_id
                    ORDER BY MAX(ra.published_at) DESC NULLS LAST, s.source_article_id
                ) AS rn
            FROM recap_subworker_sentences s
            LEFT JOIN recap_job_articles ra
                ON ra.job_id = $1 AND ra.article_id = s.source_article_id
            WHERE s.cluster_row_id = ANY($2)
            GROUP BY s.cluster_row_id, s.source_article_id
        )
        SELECT *
        FROM ranked
        WHERE rn <= 10
        ORDER BY cluster_row_id, rn
        ",
    )
    .bind(job_id)
    .bind(cluster_row_ids)
    .fetch_all(pool)
    .await
    .context("failed to fetch fallback evidence")?;

    let mut grouped: HashMap<i64, Vec<ClusterEvidence>> = HashMap::new();
    for row in rows {
        let cluster_row_id: i64 = row.try_get("cluster_row_id")?;
        let title = row
            .try_get::<Option<String>, _>("title")
            .unwrap_or(None)
            .unwrap_or_default();
        let source_url = row
            .try_get::<Option<String>, _>("source_url")
            .unwrap_or(None)
            .unwrap_or_default();
        let published_at = row
            .try_get::<Option<DateTime<Utc>>, _>("published_at")
            .unwrap_or(None)
            .unwrap_or_else(|| Utc::now());
        let lang = row
            .try_get::<Option<String>, _>("lang_hint")
            .unwrap_or(None);

        grouped
            .entry(cluster_row_id)
            .or_default()
            .push(ClusterEvidence {
                article_id: row.try_get::<String, _>("source_article_id")?,
                title,
                source_url,
                published_at,
                lang,
            });
    }

    Ok(grouped)
}
