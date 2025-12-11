use anyhow::{Context, Result};
use chrono::DateTime;
use sqlx::{PgPool, Row};
use std::collections::HashMap;
use uuid::Uuid;

use super::super::models::RawArticle;

pub(crate) struct RecapDao;

impl RecapDao {
    /// Raw記事をバックアップテーブルに保存する。
    pub async fn backup_raw_articles(
        pool: &PgPool,
        job_id: Uuid,
        articles: &[RawArticle],
    ) -> Result<()> {
        if articles.is_empty() {
            return Ok(());
        }

        let mut tx = pool.begin().await.context("failed to begin transaction")?;

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

    /// 記事IDのリストからメタデータ（published_at, source_url）を取得する。
    pub async fn get_article_metadata(
        pool: &PgPool,
        job_id: Uuid,
        article_ids: &[String],
    ) -> Result<HashMap<String, (Option<DateTime<chrono::Utc>>, Option<String>)>> {
        if article_ids.is_empty() {
            return Ok(HashMap::new());
        }

        let rows = sqlx::query(
            r"
            SELECT article_id, published_at, source_url
            FROM recap_job_articles
            WHERE job_id = $1 AND article_id = ANY($2)
            ",
        )
        .bind(job_id)
        .bind(article_ids)
        .fetch_all(pool)
        .await
        .context("failed to fetch article metadata")?;

        let mut metadata = HashMap::new();
        for row in rows {
            let article_id: String = row.try_get("article_id")?;
            let published_at: Option<DateTime<chrono::Utc>> = row.try_get("published_at")?;
            let source_url: Option<String> = row.try_get("source_url")?;
            metadata.insert(article_id, (published_at, source_url));
        }

        Ok(metadata)
    }
}
