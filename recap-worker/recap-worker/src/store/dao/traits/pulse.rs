//! Pulse DAO trait for Evening Pulse data access
//!
//! This module defines the trait for retrieving and saving Evening Pulse generation results.

use std::future::Future;

use anyhow::Result;
use chrono::NaiveDate;

use crate::pipeline::pulse::PulseResult;
use crate::store::models::PulseGenerationRow;

/// Data access trait for Evening Pulse operations.
///
/// Provides methods to retrieve pulse generation results,
/// enabling the `/v1/pulse/latest` API endpoint.
pub trait PulseDao: Send + Sync {
    /// Get the pulse generation for a specific date.
    ///
    /// Returns the most recent successful pulse generation for the given date.
    fn get_pulse_by_date(
        &self,
        date: NaiveDate,
    ) -> impl Future<Output = Result<Option<PulseGenerationRow>>> + Send;

    /// Get the latest successful pulse generation.
    ///
    /// Returns the most recent pulse generation regardless of date.
    fn get_latest_pulse(&self) -> impl Future<Output = Result<Option<PulseGenerationRow>>> + Send;

    /// Save a pulse generation result.
    ///
    /// Inserts the pulse generation result into the database, including:
    /// - Generation metadata (job_id, target_date, version, status)
    /// - Full result payload as JSON
    /// - Topics count
    ///
    /// Returns the database-assigned generation ID.
    fn save_pulse_generation(
        &self,
        result: &PulseResult,
        target_date: NaiveDate,
    ) -> impl Future<Output = Result<i64>> + Send;
}
