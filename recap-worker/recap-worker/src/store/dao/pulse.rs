//! Pulse DAO SQL implementations
//!
//! This module provides database operations for Evening Pulse data,
//! including retrieving and saving pulse generation results.

use anyhow::{Context, Result};
use chrono::NaiveDate;
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
                result_payload
            FROM pulse_generations
            WHERE target_date = $1 AND status = 'succeeded'
            ORDER BY finished_at DESC NULLS LAST
            LIMIT 1
            ",
        )
        .bind(date)
        .fetch_optional(pool)
        .await
        .context("failed to fetch pulse by date")?;

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
                result_payload
            FROM pulse_generations
            WHERE status = 'succeeded'
            ORDER BY finished_at DESC NULLS LAST
            LIMIT 1
            ",
        )
        .fetch_optional(pool)
        .await
        .context("failed to fetch latest pulse")?;

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
    ) -> Result<i64> {
        let version = result.version.to_string();
        let status = if result.is_success() {
            "succeeded"
        } else {
            "failed"
        };
        let topics_count = result.topic_count() as i32;
        let result_payload =
            serde_json::to_value(result).context("failed to serialize pulse result")?;

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
            RETURNING id
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
        .context("failed to insert pulse generation")?;

        let id: i64 = row.try_get("id")?;
        Ok(id)
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
    })
}
