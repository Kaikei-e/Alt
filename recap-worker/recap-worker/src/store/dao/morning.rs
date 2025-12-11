use anyhow::{Context, Result};
use chrono::{DateTime, Utc};
use sqlx::{PgPool, Row};
use uuid::Uuid;

pub(crate) struct RecapDao;

impl RecapDao {
    /// Morning Letter: Overnight Updatesのグループを保存する。
    pub async fn save_morning_article_groups(
        pool: &PgPool,
        groups: &[(Uuid, Uuid, bool)],
    ) -> Result<()> {
        if groups.is_empty() {
            return Ok(());
        }

        let mut tx = pool
            .begin()
            .await
            .context("failed to begin transaction for morning groups")?;

        for (group_id, article_id, is_primary) in groups {
            sqlx::query(
                r"
                INSERT INTO morning_article_groups (group_id, article_id, is_primary)
                VALUES ($1, $2, $3)
                ON CONFLICT (group_id, article_id) DO NOTHING
                ",
            )
            .bind(group_id)
            .bind(article_id)
            .bind(is_primary)
            .execute(&mut *tx)
            .await
            .context("failed to insert morning article group")?;
        }

        tx.commit()
            .await
            .context("failed to commit morning groups")?;

        Ok(())
    }

    /// Morning Letter: 指定された日時以降に作成された記事グループを取得する。
    pub async fn get_morning_article_groups(
        pool: &PgPool,
        since: DateTime<Utc>,
    ) -> Result<Vec<(Uuid, Uuid, bool, DateTime<Utc>)>> {
        let rows = sqlx::query(
            r"
            SELECT group_id, article_id, is_primary, created_at
            FROM morning_article_groups
            WHERE created_at > $1
            ORDER BY created_at ASC
            ",
        )
        .bind(since)
        .fetch_all(pool)
        .await
        .context("failed to fetch morning article groups")?;

        let mut results = Vec::with_capacity(rows.len());
        for row in rows {
            results.push((
                row.try_get("group_id")?,
                row.try_get("article_id")?,
                row.try_get("is_primary")?,
                row.try_get("created_at")?,
            ));
        }

        Ok(results)
    }
}
