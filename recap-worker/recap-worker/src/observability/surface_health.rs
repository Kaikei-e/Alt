use anyhow::Result;
use chrono::{DateTime, Utc};
use sqlx::PgPool;

use crate::observability::metrics::Metrics;
use crate::store::dao::surface::SurfaceDaoOps;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SurfaceHealthTarget {
    ThreeDayRecap,
    TopicCards,
    EveningPulse,
    MorningLetter,
}

impl SurfaceHealthTarget {
    pub const ALL: [Self; 4] = [
        Self::ThreeDayRecap,
        Self::TopicCards,
        Self::EveningPulse,
        Self::MorningLetter,
    ];

    #[must_use]
    pub const fn as_str(self) -> &'static str {
        match self {
            Self::ThreeDayRecap => "three_day_recap",
            Self::TopicCards => "topic_cards",
            Self::EveningPulse => "evening_pulse",
            Self::MorningLetter => "morning_letter",
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct SurfaceHealthSeed {
    pub last_success_at: Option<DateTime<Utc>>,
    pub consecutive_failures: u64,
}

pub fn seed_surface(metrics: &Metrics, surface: SurfaceHealthTarget, seed: SurfaceHealthSeed) {
    let label = [surface.as_str()];
    let ts = seed.last_success_at.map_or(0.0, |dt| dt.timestamp() as f64);
    metrics
        .surface_last_success_timestamp_seconds
        .with_label_values(&label)
        .set(ts);
    metrics
        .surface_consecutive_failures
        .with_label_values(&label)
        .set(seed.consecutive_failures as f64);
}

pub fn record_success(
    metrics: &Metrics,
    surface: SurfaceHealthTarget,
    completed_at: DateTime<Utc>,
) {
    let label = [surface.as_str()];
    metrics
        .surface_last_success_timestamp_seconds
        .with_label_values(&label)
        .set(completed_at.timestamp() as f64);
    metrics
        .surface_consecutive_failures
        .with_label_values(&label)
        .set(0.0);
}

pub fn record_failure(metrics: &Metrics, surface: SurfaceHealthTarget) {
    let label = [surface.as_str()];
    metrics
        .surface_consecutive_failures
        .with_label_values(&label)
        .inc();
}

pub async fn seed_all_surfaces(pool: &PgPool, metrics: &Metrics) -> Result<()> {
    let seeds = SurfaceDaoOps::get_all_surface_health_seeds(pool).await?;
    for (surface, seed) in seeds {
        seed_surface(metrics, surface, seed);
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::TimeZone;
    use prometheus::Registry;
    use std::sync::Arc;

    #[test]
    fn seed_and_record_transitions_update_both_gauges_for_surface() {
        let registry = Arc::new(Registry::new());
        let metrics = Metrics::new(registry).expect("metrics");

        seed_surface(
            &metrics,
            SurfaceHealthTarget::ThreeDayRecap,
            SurfaceHealthSeed {
                last_success_at: None,
                consecutive_failures: 3,
            },
        );
        assert_eq!(
            metrics
                .surface_last_success_timestamp_seconds
                .with_label_values(&["three_day_recap"])
                .get(),
            0.0
        );
        assert_eq!(
            metrics
                .surface_consecutive_failures
                .with_label_values(&["three_day_recap"])
                .get(),
            3.0
        );

        let completed_at = Utc.with_ymd_and_hms(2026, 11, 9, 12, 0, 0).unwrap();
        record_success(&metrics, SurfaceHealthTarget::ThreeDayRecap, completed_at);
        assert_eq!(
            metrics
                .surface_last_success_timestamp_seconds
                .with_label_values(&["three_day_recap"])
                .get(),
            completed_at.timestamp() as f64
        );
        assert_eq!(
            metrics
                .surface_consecutive_failures
                .with_label_values(&["three_day_recap"])
                .get(),
            0.0
        );

        record_failure(&metrics, SurfaceHealthTarget::ThreeDayRecap);
        record_failure(&metrics, SurfaceHealthTarget::ThreeDayRecap);
        assert_eq!(
            metrics
                .surface_last_success_timestamp_seconds
                .with_label_values(&["three_day_recap"])
                .get(),
            completed_at.timestamp() as f64
        );
        assert_eq!(
            metrics
                .surface_consecutive_failures
                .with_label_values(&["three_day_recap"])
                .get(),
            2.0
        );
    }
}
