//! Evening Pulse v4.0 Pipeline Module.
//!
//! This module implements the Evening Pulse feature, a daily digest that delivers
//! 3 high-quality news topics as an "entrance" to 7 Days Recap, helping users
//! understand the day's news in 60 seconds.
//!
//! ## Architecture
//!
//! The Pulse pipeline consists of several stages:
//!
//! 1. **Quality Evaluation** (`cluster_quality.rs`) - Evaluates cluster quality using
//!    cohesion, ambiguity, and entity consistency metrics.
//!
//! 2. **Syndication Removal** (`syndication.rs`) - Removes duplicate/syndicated content
//!    using canonical URL matching, wire source detection, and title similarity.
//!
//! 3. **Role-Based Selection** (`selection.rs`) - Selects up to 3 topics with diverse
//!    roles: NeedToKnow, Trend, and Serendipity.
//!
//! 4. **Rationale Generation** (`rationale.rs`) - Generates human-readable explanations
//!    for why each topic was selected.
//!
//! ## Usage
//!
//! ```rust,ignore
//! use recap_worker::pipeline::pulse::{PulseConfig, PulseRollout, PulseStage};
//!
//! let config = PulseConfig::from_env();
//! let rollout = PulseRollout::from_env();
//!
//! if rollout.allows(job_id) {
//!     let result = pulse_stage.generate(&job, clusters).await?;
//! }
//! ```
//!
//! ## Feature Flags
//!
//! - `PULSE_ENABLED` - Global enable/disable
//! - `PULSE_ROLLOUT_PERCENT` - Percentage rollout (0-100)
//! - `PULSE_VERSION` - Target version (v2, v3, v4)
//!
//! ## Database Tables
//!
//! - `pulse_generations` - Generation run logs
//! - `pulse_cluster_diagnostics` - Per-cluster quality metrics
//! - `pulse_selection_log` - Selection decision logs

pub mod cluster_quality;
pub mod config;
pub mod rationale;
pub mod selection;
pub mod stage;
pub mod syndication;
pub mod types;

// Re-export commonly used types
pub use config::{
    PulseConfig, PulseQualityConfig, PulseRollout, PulseSelectionConfig, PulseSyndicationConfig,
};
pub use types::{
    ClusterQualityMetrics, ClusterWithMetrics, GenerationStatus, PulseDiagnostics, PulseResult,
    PulseTopic, PulseVersion, QualityTier, RoleWeights, ScoreBreakdown, SelectionTrace,
    SyndicationStatus, TopicRole,
};

// Re-export stage components
pub use cluster_quality::{ClusterQualityEvaluator, DefaultClusterQualityEvaluator};
pub use rationale::RationaleGenerator;
pub use selection::{DefaultTopicSelector, TopicSelector};
pub use stage::{ArticleInput, ClusterInput, DefaultPulseStage, PulseInput, PulseStage};
pub use syndication::{DefaultSyndicationRemover, SyndicationRemover, SyndicationResult};
