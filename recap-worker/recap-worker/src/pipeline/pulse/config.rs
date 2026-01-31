//! Configuration for Evening Pulse v4.0.
//!
//! This module provides configuration loading and feature flag management
//! for the Pulse pipeline, following the established `RefineRollout` pattern.

use std::env;
use uuid::Uuid;

use super::types::{PulseVersion, RoleWeights};
use crate::config::FeatureToggle;

/// Configuration for the Evening Pulse generation.
#[derive(Debug, Clone)]
pub struct PulseConfig {
    /// Pulse version to use.
    pub version: PulseVersion,
    /// Whether Pulse is enabled globally.
    pub enabled: FeatureToggle,
    /// Maximum number of topics to generate (typically 3).
    pub max_topics: usize,
    /// Quality evaluation configuration.
    pub quality: PulseQualityConfig,
    /// Syndication removal configuration.
    pub syndication: PulseSyndicationConfig,
    /// Selection configuration.
    pub selection: PulseSelectionConfig,
}

impl PulseConfig {
    /// Load configuration from environment variables.
    #[must_use]
    pub fn from_env() -> Self {
        let version = env::var("PULSE_VERSION")
            .ok()
            .and_then(|v| v.parse().ok())
            .unwrap_or(PulseVersion::V4);

        let enabled = parse_bool_env("PULSE_ENABLED", true);

        let max_topics = env::var("PULSE_MAX_TOPICS")
            .ok()
            .and_then(|v| v.parse().ok())
            .unwrap_or(3);

        Self {
            version,
            enabled: if enabled {
                FeatureToggle::Enabled
            } else {
                FeatureToggle::Disabled
            },
            max_topics,
            quality: PulseQualityConfig::from_env(),
            syndication: PulseSyndicationConfig::from_env(),
            selection: PulseSelectionConfig::from_env(),
        }
    }

    /// Check if Pulse is enabled.
    #[must_use]
    pub fn is_enabled(&self) -> bool {
        self.enabled.is_enabled()
    }
}

impl Default for PulseConfig {
    fn default() -> Self {
        Self {
            version: PulseVersion::V4,
            enabled: FeatureToggle::Disabled,
            max_topics: 3,
            quality: PulseQualityConfig::default(),
            syndication: PulseSyndicationConfig::default(),
            selection: PulseSelectionConfig::default(),
        }
    }
}

/// Configuration for quality evaluation thresholds.
#[derive(Debug, Clone)]
pub struct PulseQualityConfig {
    /// Minimum cohesion score for OK tier (default: 0.3).
    pub cohesion_threshold: f32,
    /// Maximum ambiguity score for OK tier (default: 0.5).
    pub ambiguity_threshold: f32,
    /// Minimum entity consistency for OK tier (default: 0.4).
    pub entity_consistency_threshold: f32,
    /// Threshold for embedding similarity in ambiguity calculation (default: 0.5).
    pub embedding_similarity_threshold: f32,
}

impl PulseQualityConfig {
    /// Load quality configuration from environment variables.
    #[must_use]
    pub fn from_env() -> Self {
        Self {
            cohesion_threshold: parse_f32_env("PULSE_COHESION_THRESHOLD", 0.3),
            ambiguity_threshold: parse_f32_env("PULSE_AMBIGUITY_THRESHOLD", 0.5),
            entity_consistency_threshold: parse_f32_env("PULSE_ENTITY_CONSISTENCY_THRESHOLD", 0.4),
            embedding_similarity_threshold: parse_f32_env(
                "PULSE_EMBEDDING_SIMILARITY_THRESHOLD",
                0.5,
            ),
        }
    }
}

impl Default for PulseQualityConfig {
    fn default() -> Self {
        Self {
            cohesion_threshold: 0.3,
            ambiguity_threshold: 0.5,
            entity_consistency_threshold: 0.4,
            embedding_similarity_threshold: 0.5,
        }
    }
}

/// Configuration for syndication removal.
#[derive(Debug, Clone)]
pub struct PulseSyndicationConfig {
    /// Enable canonical URL matching (Stage 1).
    pub canonical_enabled: FeatureToggle,
    /// Enable wire source detection (Stage 2).
    pub wire_enabled: FeatureToggle,
    /// Enable title similarity detection (Stage 3).
    pub title_enabled: FeatureToggle,
    /// Threshold for title similarity (default: 0.85).
    pub title_threshold: f32,
}

impl PulseSyndicationConfig {
    /// Load syndication configuration from environment variables.
    #[must_use]
    pub fn from_env() -> Self {
        Self {
            canonical_enabled: if parse_bool_env("PULSE_SYNDICATION_CANONICAL_ENABLED", true) {
                FeatureToggle::Enabled
            } else {
                FeatureToggle::Disabled
            },
            wire_enabled: if parse_bool_env("PULSE_SYNDICATION_WIRE_ENABLED", true) {
                FeatureToggle::Enabled
            } else {
                FeatureToggle::Disabled
            },
            title_enabled: if parse_bool_env("PULSE_SYNDICATION_TITLE_ENABLED", false) {
                FeatureToggle::Enabled
            } else {
                FeatureToggle::Disabled
            },
            title_threshold: parse_f32_env("PULSE_SYNDICATION_TITLE_THRESHOLD", 0.85),
        }
    }

    /// Check if canonical URL matching is enabled.
    #[must_use]
    pub fn is_canonical_enabled(&self) -> bool {
        self.canonical_enabled.is_enabled()
    }

    /// Check if wire source detection is enabled.
    #[must_use]
    pub fn is_wire_enabled(&self) -> bool {
        self.wire_enabled.is_enabled()
    }

    /// Check if title similarity detection is enabled.
    #[must_use]
    pub fn is_title_enabled(&self) -> bool {
        self.title_enabled.is_enabled()
    }
}

impl Default for PulseSyndicationConfig {
    fn default() -> Self {
        Self {
            canonical_enabled: FeatureToggle::Enabled,
            wire_enabled: FeatureToggle::Enabled,
            title_enabled: FeatureToggle::Disabled, // Disabled by default for safety
            title_threshold: 0.85,
        }
    }
}

/// Configuration for topic selection.
#[derive(Debug, Clone)]
pub struct PulseSelectionConfig {
    /// Weights for NeedToKnow role.
    pub need_to_know_weights: RoleWeights,
    /// Weights for Trend role.
    pub trend_weights: RoleWeights,
    /// Weights for Serendipity role.
    pub serendipity_weights: RoleWeights,
    /// Minimum score threshold for topic selection (default: 0.3).
    pub min_score_threshold: f32,
    /// Maximum fallback level before giving up (default: 5).
    pub max_fallback_level: u8,
}

impl PulseSelectionConfig {
    /// Load selection configuration from environment variables.
    #[must_use]
    pub fn from_env() -> Self {
        Self {
            need_to_know_weights: RoleWeights::need_to_know(),
            trend_weights: RoleWeights::trend(),
            serendipity_weights: RoleWeights::serendipity(),
            min_score_threshold: parse_f32_env("PULSE_MIN_SCORE_THRESHOLD", 0.3),
            max_fallback_level: parse_u8_env("PULSE_MAX_FALLBACK_LEVEL", 5),
        }
    }
}

impl Default for PulseSelectionConfig {
    fn default() -> Self {
        Self {
            need_to_know_weights: RoleWeights::need_to_know(),
            trend_weights: RoleWeights::trend(),
            serendipity_weights: RoleWeights::serendipity(),
            min_score_threshold: 0.3,
            max_fallback_level: 5,
        }
    }
}

/// Rollout gate for Pulse v4, following the `RefineRollout` pattern from `genre.rs`.
///
/// Uses job_id to deterministically decide if a job should use the new version.
#[derive(Debug, Clone)]
pub struct PulseRollout {
    /// Percentage of jobs to use the new version (0-100).
    percentage: u8,
    /// Target version for enabled jobs.
    version: PulseVersion,
}

impl PulseRollout {
    /// Create a new rollout gate.
    ///
    /// # Arguments
    ///
    /// * `percentage` - Percentage of jobs to enable (0-100).
    /// * `version` - Version to use for enabled jobs.
    #[must_use]
    pub fn new(percentage: u8, version: PulseVersion) -> Self {
        Self {
            percentage: percentage.min(100),
            version,
        }
    }

    /// Load rollout configuration from environment variables.
    #[must_use]
    pub fn from_env() -> Self {
        let percentage = env::var("PULSE_ROLLOUT_PERCENT")
            .ok()
            .and_then(|v| v.parse().ok())
            .unwrap_or(0)
            .min(100);

        let version = env::var("PULSE_VERSION")
            .ok()
            .and_then(|v| v.parse().ok())
            .unwrap_or(PulseVersion::V4);

        Self { percentage, version }
    }

    /// Check if the rollout allows this job to use the new version.
    ///
    /// Uses a deterministic bucket based on job_id for consistent behavior.
    #[must_use]
    pub fn allows(&self, job_id: Uuid) -> bool {
        if self.percentage == 0 {
            return false;
        }
        if self.percentage >= 100 {
            return true;
        }
        // Deterministic bucket based on job_id
        let bucket = (job_id.as_u128() % 100) as u8;
        bucket < self.percentage
    }

    /// Get the target version for enabled jobs.
    #[must_use]
    pub fn version(&self) -> PulseVersion {
        self.version
    }

    /// Get the rollout percentage.
    #[must_use]
    pub fn percentage(&self) -> u8 {
        self.percentage
    }
}

impl Default for PulseRollout {
    fn default() -> Self {
        Self {
            percentage: 0,
            version: PulseVersion::V4,
        }
    }
}

// Helper functions for environment variable parsing

fn parse_bool_env(name: &str, default: bool) -> bool {
    env::var(name)
        .ok()
        .map_or(default, |v| matches!(v.to_lowercase().as_str(), "true" | "1" | "yes" | "on"))
}

fn parse_f32_env(name: &str, default: f32) -> f32 {
    env::var(name)
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(default)
}

fn parse_u8_env(name: &str, default: u8) -> u8 {
    env::var(name)
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(default)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn pulse_config_defaults() {
        let config = PulseConfig::default();
        assert_eq!(config.version, PulseVersion::V4);
        assert!(!config.is_enabled());
        assert_eq!(config.max_topics, 3);
    }

    #[test]
    fn pulse_quality_config_defaults() {
        let config = PulseQualityConfig::default();
        assert!((config.cohesion_threshold - 0.3).abs() < f32::EPSILON);
        assert!((config.ambiguity_threshold - 0.5).abs() < f32::EPSILON);
        assert!((config.entity_consistency_threshold - 0.4).abs() < f32::EPSILON);
    }

    #[test]
    fn pulse_syndication_config_defaults() {
        let config = PulseSyndicationConfig::default();
        assert!(config.is_canonical_enabled());
        assert!(config.is_wire_enabled());
        assert!(!config.is_title_enabled()); // Disabled by default
        assert!((config.title_threshold - 0.85).abs() < f32::EPSILON);
    }

    #[test]
    fn pulse_selection_config_defaults() {
        let config = PulseSelectionConfig::default();
        assert!(config.need_to_know_weights.is_valid());
        assert!(config.trend_weights.is_valid());
        assert!(config.serendipity_weights.is_valid());
        assert!((config.min_score_threshold - 0.3).abs() < f32::EPSILON);
    }

    #[test]
    fn pulse_rollout_allows_0_percent() {
        let rollout = PulseRollout::new(0, PulseVersion::V4);
        // Should never allow at 0%
        for _ in 0..100 {
            let job_id = Uuid::new_v4();
            assert!(!rollout.allows(job_id));
        }
    }

    #[test]
    fn pulse_rollout_allows_100_percent() {
        let rollout = PulseRollout::new(100, PulseVersion::V4);
        // Should always allow at 100%
        for _ in 0..100 {
            let job_id = Uuid::new_v4();
            assert!(rollout.allows(job_id));
        }
    }

    #[test]
    fn pulse_rollout_deterministic() {
        let rollout = PulseRollout::new(50, PulseVersion::V4);
        let job_id = Uuid::parse_str("550e8400-e29b-41d4-a716-446655440000").unwrap();

        // Same job_id should always give same result
        let result1 = rollout.allows(job_id);
        let result2 = rollout.allows(job_id);
        assert_eq!(result1, result2);
    }

    #[test]
    fn pulse_rollout_distribution() {
        let rollout = PulseRollout::new(50, PulseVersion::V4);
        let mut allowed_count = 0;
        let total: u64 = 1000;

        for i in 0u64..total {
            // Create deterministic UUIDs
            let job_id = Uuid::from_u128(u128::from(i));
            if rollout.allows(job_id) {
                allowed_count += 1;
            }
        }

        // Should be roughly 50% (within 10% margin)
        let ratio = f64::from(allowed_count) / total as f64;
        assert!(
            (0.4..=0.6).contains(&ratio),
            "Expected ~50% allowed, got {}%",
            ratio * 100.0
        );
    }

    #[test]
    fn pulse_rollout_caps_at_100() {
        let rollout = PulseRollout::new(150, PulseVersion::V4);
        assert_eq!(rollout.percentage(), 100);
    }
}
