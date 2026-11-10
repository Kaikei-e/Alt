//! Pulse DAO SQL implementations
//!
//! This module provides database operations for Evening Pulse data,
//! including retrieving and saving pulse generation results.

use crate::error::{RecapError, Result};
use chrono::{DateTime, NaiveDate, Utc};
use serde_json::Value;
use sqlx::types::Json;
use sqlx::{PgPool, Row};

use crate::pipeline::pulse::PulseResult;
use crate::store::models::PulseGenerationRow;

pub(crate) struct RecapDao;

impl RecapDao {
    /// Get the pulse generation for a specific date.
    ///
    /// Returns the most recent successful pulse generation for the given date.
    pub(crate) async fn get_pulse_by_date(
        pool: &PgPool,
        date: NaiveDate,
    ) -> Result<Option<PulseGenerationRow>> {
        let row = sqlx::query(
            r"
            SELECT
                job_id,
                target_date,
                version,
                result_payload,
                COALESCE(finished_at, started_at) AS shown_at
            FROM pulse_generations
            WHERE target_date = $1 AND status = 'succeeded'
            ORDER BY COALESCE(finished_at, started_at) DESC, id DESC
            LIMIT 1
            ",
        )
        .bind(date)
        .fetch_optional(pool)
        .await
        .map_err(|e| RecapError::Db(format!("failed to fetch pulse by date: {e}")))?;

        match row {
            Some(row) => Ok(Some(map_row_to_pulse_generation(row)?)),
            None => Ok(None),
        }
    }

    /// Get the latest successful pulse generation.
    ///
    /// Returns the most recent pulse generation regardless of date.
    pub(crate) async fn get_latest_pulse(pool: &PgPool) -> Result<Option<PulseGenerationRow>> {
        let row = sqlx::query(
            r"
            SELECT
                job_id,
                target_date,
                version,
                result_payload,
                COALESCE(finished_at, started_at) AS shown_at
            FROM pulse_generations
            WHERE status = 'succeeded'
            ORDER BY COALESCE(finished_at, started_at) DESC, id DESC
            LIMIT 1
            ",
        )
        .fetch_optional(pool)
        .await
        .map_err(|e| RecapError::Db(format!("failed to fetch latest pulse: {e}")))?;

        match row {
            Some(row) => Ok(Some(map_row_to_pulse_generation(row)?)),
            None => Ok(None),
        }
    }

    /// Save a pulse generation result.
    ///
    /// Inserts the pulse generation result into the database, including:
    /// - Generation metadata (job_id, target_date, version, status)
    /// - Full result payload as JSON
    /// - Topics count
    ///
    /// Returns the database-assigned generation ID.
    pub(crate) async fn save_pulse_generation(
        pool: &PgPool,
        result: &PulseResult,
        target_date: NaiveDate,
    ) -> Result<(i64, DateTime<Utc>)> {
        let version = result.version.to_string();
        let status = if result.is_success() {
            "succeeded"
        } else {
            "failed"
        };
        let topics_count = result.topic_count() as i32;
        let result_payload = serde_json::to_value(result)
            .map_err(|e| RecapError::Db(format!("failed to serialize pulse result: {e}")))?;

        let row = sqlx::query(
            r"
            INSERT INTO pulse_generations (
                job_id,
                target_date,
                version,
                status,
                topics_count,
                started_at,
                finished_at,
                config_snapshot,
                result_payload
            ) VALUES (
                $1,
                $2,
                $3,
                $4,
                $5,
                $6,
                NOW(),
                '{}'::JSONB,
                $7
            )
            RETURNING id, finished_at
            ",
        )
        .bind(result.job_id)
        .bind(target_date)
        .bind(version)
        .bind(status)
        .bind(topics_count)
        .bind(result.generated_at)
        .bind(Json(result_payload))
        .fetch_one(pool)
        .await
        .map_err(|e| RecapError::Db(format!("failed to insert pulse generation: {e}")))?;

        let id: i64 = row.try_get("id")?;
        let finished_at: DateTime<Utc> = row.try_get("finished_at")?;
        Ok((id, finished_at))
    }
}

fn map_row_to_pulse_generation(row: sqlx::postgres::PgRow) -> Result<PulseGenerationRow> {
    let result_payload: Option<Value> = row
        .try_get::<Option<Json<Value>>, _>("result_payload")?
        .map(|j| j.0);

    Ok(PulseGenerationRow {
        job_id: row.try_get("job_id")?,
        target_date: row.try_get("target_date")?,
        version: row.try_get("version")?,
        result_payload,
        finished_at: row.try_get("shown_at")?,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use sqlx::postgres::PgPoolOptions;
    use uuid::Uuid;

    #[tokio::test]
    async fn get_pulse_by_date_and_latest_select_coalesce_finished_at_started_at()
    -> anyhow::Result<()> {
        let Ok(database_url) = std::env::var("DATABASE_URL") else {
            return Ok(());
        };

        let pool = PgPoolOptions::new()
            .max_connections(1)
            .connect(&database_url)
            .await?;

        sqlx::query(
            r"
            CREATE TEMP TABLE pulse_generations (
                id BIGSERIAL PRIMARY KEY,
                job_id UUID NOT NULL,
                target_date DATE NOT NULL,
                version TEXT NOT NULL,
                status TEXT NOT NULL,
                started_at TIMESTAMPTZ NOT NULL,
                finished_at TIMESTAMPTZ,
                result_payload JSONB
            )
            ",
        )
        .execute(&pool)
        .await?;

        let shared_job_id = Uuid::new_v4();
        let tie_winner_job_id = Uuid::new_v4();
        let day1 = NaiveDate::from_ymd_opt(2026, 11, 8).unwrap();
        let day2 = NaiveDate::from_ymd_opt(2026, 11, 9).unwrap();
        let day1_started =
            DateTime::parse_from_rfc3339("2026-11-08T18:00:00Z")?.with_timezone(&Utc);
        let day2_started =
            DateTime::parse_from_rfc3339("2026-11-09T18:00:00Z")?.with_timezone(&Utc);
        let day2_finished =
            DateTime::parse_from_rfc3339("2026-11-09T18:02:00Z")?.with_timezone(&Utc);

        sqlx::query(
            r"
            INSERT INTO pulse_generations (
                job_id, target_date, version, status, started_at, finished_at, result_payload
            ) VALUES
                ($1, $2, 'v4', 'succeeded', $3, NULL, '{}'::JSONB),
                ($1, $4, 'v4', 'succeeded', $5, $6, '{}'::JSONB),
                ($7, $4, 'v4', 'succeeded', $5, $6, '{}'::JSONB)
            ",
        )
        .bind(shared_job_id)
        .bind(day1)
        .bind(day1_started)
        .bind(day2)
        .bind(day2_started)
        .bind(day2_finished)
        .bind(tie_winner_job_id)
        .execute(&pool)
        .await?;

        let day1_row = RecapDao::get_pulse_by_date(&pool, day1)
            .await?
            .expect("day1 pulse row");
        assert_eq!(day1_row.target_date, day1);
        assert_eq!(day1_row.finished_at, day1_started);

        let day2_row = RecapDao::get_pulse_by_date(&pool, day2)
            .await?
            .expect("day2 pulse row");
        assert_eq!(day2_row.job_id, tie_winner_job_id);
        assert_eq!(day2_row.finished_at, day2_finished);

        let latest_row = RecapDao::get_latest_pulse(&pool)
            .await?
            .expect("latest pulse row");
        assert_eq!(latest_row.job_id, tie_winner_job_id);
        assert_eq!(latest_row.target_date, day2);
        assert_eq!(latest_row.finished_at, day2_finished);

        Ok(())
    }
}
