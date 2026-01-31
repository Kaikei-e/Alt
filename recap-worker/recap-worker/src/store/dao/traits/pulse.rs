//! Pulse DAO trait for Evening Pulse data access
//!
//! This module defines the trait for retrieving and saving Evening Pulse generation results.

use anyhow::Result;
use async_trait::async_trait;
use chrono::NaiveDate;

use crate::pipeline::pulse::PulseResult;
use crate::store::models::PulseGenerationRow;

/// Data access trait for Evening Pulse operations.
///
/// Provides methods to retrieve pulse generation results,
/// enabling the `/v1/pulse/latest` API endpoint.
#[async_trait]
pub trait PulseDao: Send + Sync {
    /// Get the pulse generation for a specific date.
    ///
    /// Returns the most recent successful pulse generation for the given date.
    async fn get_pulse_by_date(&self, date: NaiveDate) -> Result<Option<PulseGenerationRow>>;

    /// Get the latest successful pulse generation.
    ///
    /// Returns the most recent pulse generation regardless of date.
    async fn get_latest_pulse(&self) -> Result<Option<PulseGenerationRow>>;

    /// Save a pulse generation result.
    ///
    /// Inserts the pulse generation result into the database, including:
    /// - Generation metadata (job_id, target_date, version, status)
    /// - Full result payload as JSON
    /// - Topics count
    ///
    /// Returns the database-assigned generation ID.
    async fn save_pulse_generation(
        &self,
        result: &PulseResult,
        target_date: NaiveDate,
    ) -> Result<i64>;
}

#[cfg(test)]
mod tests {
    use super::*;

    // Trait definition test - ensure trait is object-safe
    fn _assert_object_safe(_: &dyn PulseDao) {}
}
