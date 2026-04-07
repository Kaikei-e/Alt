use anyhow::{Context, Result};
use chrono::{DateTime, NaiveDate, Utc};
use serde_json::Value;
use sqlx::types::Json;
use sqlx::{PgPool, Row};
use uuid::Uuid;

use crate::store::models::{MorningLetter, MorningLetterSource};

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

    /// Morning Letter を保存する (UPSERT on target_date + edition_timezone)。
    pub(crate) async fn save_morning_letter(pool: &PgPool, letter: &MorningLetter) -> Result<()> {
        sqlx::query(
            r"
            INSERT INTO morning_letters (
                id, target_date, edition_timezone, source_recap_job_id,
                is_degraded, schema_version, generation_revision,
                result_jsonb, model, generation_metadata_jsonb
            ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
            ON CONFLICT (target_date, edition_timezone) DO UPDATE SET
                result_jsonb = EXCLUDED.result_jsonb,
                is_degraded = EXCLUDED.is_degraded,
                model = EXCLUDED.model,
                generation_metadata_jsonb = EXCLUDED.generation_metadata_jsonb,
                generation_revision = morning_letters.generation_revision + 1
            ",
        )
        .bind(letter.id)
        .bind(letter.target_date)
        .bind(&letter.edition_timezone)
        .bind(letter.source_recap_job_id)
        .bind(letter.is_degraded)
        .bind(letter.schema_version)
        .bind(letter.generation_revision)
        .bind(Json(&letter.result_jsonb))
        .bind(&letter.model)
        .bind(Json(&letter.generation_metadata_jsonb))
        .execute(pool)
        .await
        .context("failed to upsert morning letter")?;

        Ok(())
    }

    /// Morning Letter のソース (provenance) を保存する。
    /// 既存のソースを削除してから一括 INSERT する。
    pub(crate) async fn save_morning_letter_sources(
        pool: &PgPool,
        sources: &[MorningLetterSource],
    ) -> Result<()> {
        if sources.is_empty() {
            return Ok(());
        }

        let letter_id = sources[0].letter_id;

        let mut tx = pool
            .begin()
            .await
            .context("failed to begin transaction for morning letter sources")?;

        sqlx::query("DELETE FROM morning_letter_sources WHERE letter_id = $1")
            .bind(letter_id)
            .execute(&mut *tx)
            .await
            .context("failed to delete existing morning letter sources")?;

        for source in sources {
            sqlx::query(
                r"
                INSERT INTO morning_letter_sources (
                    letter_id, section_key, article_id, source_type, position
                ) VALUES ($1, $2, $3, $4, $5)
                ",
            )
            .bind(source.letter_id)
            .bind(&source.section_key)
            .bind(source.article_id)
            .bind(&source.source_type)
            .bind(source.position)
            .execute(&mut *tx)
            .await
            .context("failed to insert morning letter source")?;
        }

        tx.commit()
            .await
            .context("failed to commit morning letter sources")?;

        Ok(())
    }

    /// 指定日の Morning Letter を取得する。
    pub(crate) async fn get_morning_letter_by_date(
        pool: &PgPool,
        date: NaiveDate,
    ) -> Result<Option<MorningLetter>> {
        let row = sqlx::query(
            r"
            SELECT
                id, target_date, edition_timezone, source_recap_job_id,
                is_degraded, schema_version, generation_revision,
                result_jsonb, model, generation_metadata_jsonb, created_at
            FROM morning_letters
            WHERE target_date = $1
            LIMIT 1
            ",
        )
        .bind(date)
        .fetch_optional(pool)
        .await
        .context("failed to fetch morning letter by date")?;

        match row {
            Some(row) => Ok(Some(map_row_to_morning_letter(row)?)),
            None => Ok(None),
        }
    }

    /// Morning Letter のソース (provenance) を取得する。
    pub(crate) async fn get_morning_letter_sources(
        pool: &PgPool,
        letter_id: Uuid,
    ) -> Result<Vec<MorningLetterSource>> {
        let rows = sqlx::query(
            r"
            SELECT letter_id, section_key, article_id, source_type, position
            FROM morning_letter_sources
            WHERE letter_id = $1
            ORDER BY section_key, position
            ",
        )
        .bind(letter_id)
        .fetch_all(pool)
        .await
        .context("failed to fetch morning letter sources")?;

        let mut sources = Vec::with_capacity(rows.len());
        for row in rows {
            sources.push(MorningLetterSource {
                letter_id: row.try_get("letter_id")?,
                section_key: row.try_get("section_key")?,
                article_id: row.try_get("article_id")?,
                source_type: row.try_get("source_type")?,
                position: row.try_get("position")?,
            });
        }
        Ok(sources)
    }

    /// 最新の Morning Letter を取得する。
    pub(crate) async fn get_latest_morning_letter(pool: &PgPool) -> Result<Option<MorningLetter>> {
        let row = sqlx::query(
            r"
            SELECT
                id, target_date, edition_timezone, source_recap_job_id,
                is_degraded, schema_version, generation_revision,
                result_jsonb, model, generation_metadata_jsonb, created_at
            FROM morning_letters
            ORDER BY target_date DESC
            LIMIT 1
            ",
        )
        .fetch_optional(pool)
        .await
        .context("failed to fetch latest morning letter")?;

        match row {
            Some(row) => Ok(Some(map_row_to_morning_letter(row)?)),
            None => Ok(None),
        }
    }
}

fn map_row_to_morning_letter(row: sqlx::postgres::PgRow) -> Result<MorningLetter> {
    let result_jsonb: Value = row.try_get::<Json<Value>, _>("result_jsonb").map(|j| j.0)?;
    let generation_metadata_jsonb: Value = row
        .try_get::<Json<Value>, _>("generation_metadata_jsonb")
        .map(|j| j.0)?;

    Ok(MorningLetter {
        id: row.try_get("id")?,
        target_date: row.try_get("target_date")?,
        edition_timezone: row.try_get("edition_timezone")?,
        source_recap_job_id: row.try_get("source_recap_job_id")?,
        is_degraded: row.try_get("is_degraded")?,
        schema_version: row.try_get("schema_version")?,
        generation_revision: row.try_get("generation_revision")?,
        result_jsonb,
        model: row.try_get("model")?,
        generation_metadata_jsonb,
        created_at: row.try_get("created_at")?,
    })
}
