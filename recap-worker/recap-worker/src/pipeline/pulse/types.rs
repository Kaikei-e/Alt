//! Core type definitions for Evening Pulse v4.0.
//!
//! This module defines the fundamental types used across the Pulse pipeline,
//! including quality tiers, topic roles, and generation results.

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use uuid::Uuid;

/// Evening Pulse version for A/B testing and gradual rollout.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum PulseVersion {
    /// Legacy version (existing implementation).
    V2,
    /// Current stable version.
    V3,
    /// New implementation with quality evaluation and role-based selection.
    #[default]
    V4,
}

impl std::fmt::Display for PulseVersion {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            PulseVersion::V2 => write!(f, "v2"),
            PulseVersion::V3 => write!(f, "v3"),
            PulseVersion::V4 => write!(f, "v4"),
        }
    }
}

impl std::str::FromStr for PulseVersion {
    type Err = String;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s.to_lowercase().as_str() {
            "v2" => Ok(PulseVersion::V2),
            "v3" => Ok(PulseVersion::V3),
            "v4" => Ok(PulseVersion::V4),
            _ => Err(format!("unknown pulse version: {s}")),
        }
    }
}

/// Quality diagnosis tier for cluster evaluation.
///
/// Three-tier system based on cohesion, ambiguity, and entity consistency metrics.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum QualityTier {
    /// High quality - passes all thresholds.
    Ok,
    /// Marginal quality - near threshold boundaries, requires caution.
    #[default]
    Caution,
    /// Low quality - fails multiple thresholds, should be excluded.
    Ng,
}

impl std::fmt::Display for QualityTier {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            QualityTier::Ok => write!(f, "ok"),
            QualityTier::Caution => write!(f, "caution"),
            QualityTier::Ng => write!(f, "ng"),
        }
    }
}

/// Topic role for role-based selection.
///
/// Each Evening Pulse contains up to 3 topics, one per role:
/// 1. `NeedToKnow` - High-impact, breaking news
/// 2. `Trend` - Rising popularity, trending topics
/// 3. `Serendipity` - Unexpected discovery, unique perspectives
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum TopicRole {
    /// High-impact news that users need to know about.
    /// Prioritizes impact score in selection.
    NeedToKnow,
    /// Trending topics with rising popularity.
    /// Prioritizes burst score in selection.
    Trend,
    /// Unexpected discoveries with unique perspectives.
    /// Prioritizes novelty score in selection.
    Serendipity,
}

impl std::fmt::Display for TopicRole {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            TopicRole::NeedToKnow => write!(f, "need_to_know"),
            TopicRole::Trend => write!(f, "trend"),
            TopicRole::Serendipity => write!(f, "serendipity"),
        }
    }
}

/// Cluster quality metrics for evaluation.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ClusterQualityMetrics {
    /// Title cohesion - average Jaccard similarity between article titles.
    /// Range: 0.0 (no overlap) to 1.0 (identical titles).
    pub cohesion: f32,

    /// Ambiguity - ratio of article pairs with low embedding similarity.
    /// Range: 0.0 (all similar) to 1.0 (all dissimilar).
    pub ambiguity: f32,

    /// Entity consistency - ratio of articles containing the most frequent entity.
    /// Range: 0.0 (no common entities) to 1.0 (all share same entity).
    pub entity_consistency: f32,

    /// Diagnosed quality tier based on the above metrics.
    pub tier: QualityTier,
}

impl Default for ClusterQualityMetrics {
    fn default() -> Self {
        Self {
            cohesion: 0.0,
            ambiguity: 1.0,
            entity_consistency: 0.0,
            tier: QualityTier::Ng,
        }
    }
}

/// Role-based weights for scoring clusters.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct RoleWeights {
    /// Weight for impact score (0.0 - 1.0).
    pub impact: f32,
    /// Weight for burst score (0.0 - 1.0).
    pub burst: f32,
    /// Weight for novelty score (0.0 - 1.0).
    pub novelty: f32,
    /// Weight for recency score (0.0 - 1.0).
    pub recency: f32,
}

impl RoleWeights {
    /// Create weights optimized for NeedToKnow role.
    /// Prioritizes impact (0.50) with lower weights on other factors.
    #[must_use]
    pub fn need_to_know() -> Self {
        Self {
            impact: 0.50,
            burst: 0.15,
            novelty: 0.10,
            recency: 0.25,
        }
    }

    /// Create weights optimized for Trend role.
    /// Prioritizes burst (0.50) to capture trending topics.
    #[must_use]
    pub fn trend() -> Self {
        Self {
            impact: 0.20,
            burst: 0.50,
            novelty: 0.10,
            recency: 0.20,
        }
    }

    /// Create weights optimized for Serendipity role.
    /// Prioritizes novelty (0.50) for unique discoveries.
    #[must_use]
    pub fn serendipity() -> Self {
        Self {
            impact: 0.15,
            burst: 0.15,
            novelty: 0.50,
            recency: 0.20,
        }
    }

    /// Validate that weights sum to approximately 1.0.
    #[must_use]
    pub fn is_valid(&self) -> bool {
        let sum = self.impact + self.burst + self.novelty + self.recency;
        (sum - 1.0).abs() < 0.01
    }
}

impl Default for RoleWeights {
    fn default() -> Self {
        Self {
            impact: 0.25,
            burst: 0.25,
            novelty: 0.25,
            recency: 0.25,
        }
    }
}

/// Cluster with computed metrics for selection.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ClusterWithMetrics {
    /// Cluster identifier from the clustering system.
    pub cluster_id: i64,
    /// Article IDs in this cluster.
    pub article_ids: Vec<String>,
    /// Cluster label or title.
    pub label: Option<String>,
    /// Quality metrics for this cluster.
    pub quality_metrics: ClusterQualityMetrics,
    /// Impact score - measures importance/significance (0.0 - 1.0).
    pub impact_score: f32,
    /// Burst score - measures recent activity spike (0.0 - 1.0).
    pub burst_score: f32,
    /// Novelty score - measures uniqueness (0.0 - 1.0).
    pub novelty_score: f32,
    /// Recency score - measures freshness (0.0 - 1.0).
    pub recency_score: f32,
    /// Top entities extracted from the cluster.
    pub top_entities: Vec<String>,
    /// Syndication status after deduplication.
    pub syndication_status: Option<SyndicationStatus>,
    /// Representative articles (top 3 headlines with sources).
    #[serde(default)]
    pub representative_articles: Vec<RepresentativeArticle>,
    /// Unique source names in this cluster.
    #[serde(default)]
    pub source_names: Vec<String>,
    /// Genre classification (e.g., "AI", "Business").
    #[serde(default)]
    pub genre: Option<String>,
}

impl Default for ClusterWithMetrics {
    fn default() -> Self {
        Self {
            cluster_id: 0,
            article_ids: Vec::new(),
            label: None,
            quality_metrics: ClusterQualityMetrics::default(),
            impact_score: 0.0,
            burst_score: 0.0,
            novelty_score: 0.0,
            recency_score: 0.0,
            top_entities: Vec::new(),
            syndication_status: None,
            representative_articles: Vec::new(),
            source_names: Vec::new(),
            genre: None,
        }
    }
}

/// Syndication status indicating how an article was detected as syndicated content.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum SyndicationStatus {
    /// Original content (not syndicated).
    Original,
    /// Matched via canonical URL.
    CanonicalMatch,
    /// Identified as wire service content (Reuters, AP, etc.).
    WireSource,
    /// Detected via title similarity.
    TitleSimilar,
}

impl std::fmt::Display for SyndicationStatus {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            SyndicationStatus::Original => write!(f, "original"),
            SyndicationStatus::CanonicalMatch => write!(f, "canonical_match"),
            SyndicationStatus::WireSource => write!(f, "wire_source"),
            SyndicationStatus::TitleSimilar => write!(f, "title_similar"),
        }
    }
}

/// Representative article for display in topic cards.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct RepresentativeArticle {
    /// Article ID.
    pub article_id: String,
    /// Article title/headline.
    pub title: String,
    /// Source URL.
    pub source_url: String,
    /// Source name (e.g., "Reuters", "BBC").
    pub source_name: String,
    /// Published timestamp (RFC3339).
    pub published_at: String,
}

/// Selected topic with rationale for Evening Pulse.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct PulseTopic {
    /// Cluster ID from which this topic was selected.
    pub cluster_id: i64,
    /// Role assigned to this topic (NeedToKnow, Trend, or Serendipity).
    pub role: TopicRole,
    /// Final computed score for this topic.
    pub score: f32,
    /// Human-readable rationale explaining why this topic was selected.
    pub rationale: String,
    /// Article IDs included in this topic.
    pub articles: Vec<String>,
    /// Quality metrics for the source cluster.
    pub quality_metrics: ClusterQualityMetrics,
    /// Individual score components for transparency.
    pub score_breakdown: ScoreBreakdown,
    /// Representative articles (top 3 headlines with sources).
    #[serde(default)]
    pub representative_articles: Vec<RepresentativeArticle>,
    /// Top entities extracted from the cluster.
    #[serde(default)]
    pub top_entities: Vec<String>,
    /// Unique source names in this topic.
    #[serde(default)]
    pub source_names: Vec<String>,
    /// Genre classification (e.g., "AI", "Business").
    #[serde(default)]
    pub genre: Option<String>,
}

/// Breakdown of individual score components for transparency.
#[derive(Debug, Clone, PartialEq, Default, Serialize, Deserialize)]
pub struct ScoreBreakdown {
    /// Impact score contribution.
    pub impact_score: f32,
    /// Burst score contribution.
    pub burst_score: f32,
    /// Novelty score contribution.
    pub novelty_score: f32,
    /// Recency score contribution.
    pub recency_score: f32,
}

/// Evening Pulse generation result.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct PulseResult {
    /// Job ID for this generation run.
    pub job_id: Uuid,
    /// Version of the pulse algorithm used.
    pub version: PulseVersion,
    /// Selected topics (up to 3).
    pub topics: Vec<PulseTopic>,
    /// Generation timestamp.
    pub generated_at: DateTime<Utc>,
    /// Diagnostics for debugging and monitoring.
    pub diagnostics: PulseDiagnostics,
}

impl PulseResult {
    /// Create a new pulse result.
    #[must_use]
    pub fn new(job_id: Uuid, version: PulseVersion, topics: Vec<PulseTopic>) -> Self {
        Self {
            job_id,
            version,
            topics,
            generated_at: Utc::now(),
            diagnostics: PulseDiagnostics::default(),
        }
    }

    /// Check if this result represents a successful generation.
    #[must_use]
    pub fn is_success(&self) -> bool {
        !self.topics.is_empty()
    }

    /// Get the number of topics generated.
    #[must_use]
    pub fn topic_count(&self) -> usize {
        self.topics.len()
    }
}

/// Diagnostics for pulse generation debugging and monitoring.
#[derive(Debug, Clone, PartialEq, Default, Serialize, Deserialize)]
pub struct PulseDiagnostics {
    /// Number of articles removed due to syndication detection.
    pub syndication_removed: usize,
    /// Total number of clusters evaluated.
    pub clusters_evaluated: usize,
    /// Distribution of quality tiers across evaluated clusters.
    pub quality_tier_distribution: HashMap<String, usize>,
    /// Selection trace showing the decision process.
    pub selection_trace: SelectionTrace,
    /// Fallback level used (0 = normal, higher = more degradation).
    pub fallback_level: u8,
    /// Processing duration in milliseconds.
    pub duration_ms: u64,
}

/// Trace of the selection process for debugging.
#[derive(Debug, Clone, PartialEq, Default, Serialize, Deserialize)]
pub struct SelectionTrace {
    /// Candidates considered for NeedToKnow role.
    pub need_to_know_candidates: usize,
    /// Candidates considered for Trend role.
    pub trend_candidates: usize,
    /// Candidates considered for Serendipity role.
    pub serendipity_candidates: usize,
    /// Clusters excluded due to NG quality tier.
    pub excluded_ng_clusters: usize,
    /// Clusters excluded due to duplicate selection prevention.
    pub excluded_duplicates: usize,
}

/// Generation status for database persistence.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum GenerationStatus {
    /// Generation is in progress.
    Running,
    /// Generation completed successfully.
    Succeeded,
    /// Generation failed.
    Failed,
}

impl std::fmt::Display for GenerationStatus {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            GenerationStatus::Running => write!(f, "running"),
            GenerationStatus::Succeeded => write!(f, "succeeded"),
            GenerationStatus::Failed => write!(f, "failed"),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn pulse_version_display_and_parse() {
        assert_eq!(PulseVersion::V2.to_string(), "v2");
        assert_eq!(PulseVersion::V3.to_string(), "v3");
        assert_eq!(PulseVersion::V4.to_string(), "v4");

        assert_eq!("v2".parse::<PulseVersion>().unwrap(), PulseVersion::V2);
        assert_eq!("V3".parse::<PulseVersion>().unwrap(), PulseVersion::V3);
        assert_eq!("v4".parse::<PulseVersion>().unwrap(), PulseVersion::V4);
        assert!("v5".parse::<PulseVersion>().is_err());
    }

    #[test]
    fn quality_tier_display() {
        assert_eq!(QualityTier::Ok.to_string(), "ok");
        assert_eq!(QualityTier::Caution.to_string(), "caution");
        assert_eq!(QualityTier::Ng.to_string(), "ng");
    }

    #[test]
    fn topic_role_display() {
        assert_eq!(TopicRole::NeedToKnow.to_string(), "need_to_know");
        assert_eq!(TopicRole::Trend.to_string(), "trend");
        assert_eq!(TopicRole::Serendipity.to_string(), "serendipity");
    }

    #[test]
    fn role_weights_validation() {
        let default_weights = RoleWeights::default();
        assert!(default_weights.is_valid());

        let need_to_know = RoleWeights::need_to_know();
        assert!(need_to_know.is_valid());

        let trend = RoleWeights::trend();
        assert!(trend.is_valid());

        let serendipity = RoleWeights::serendipity();
        assert!(serendipity.is_valid());

        let invalid = RoleWeights {
            impact: 0.5,
            burst: 0.5,
            novelty: 0.5,
            recency: 0.5,
        };
        assert!(!invalid.is_valid());
    }

    #[test]
    fn pulse_result_success_check() {
        let job_id = Uuid::new_v4();
        let result = PulseResult::new(job_id, PulseVersion::V4, vec![]);
        assert!(!result.is_success());
        assert_eq!(result.topic_count(), 0);

        let topic = PulseTopic {
            cluster_id: 1,
            role: TopicRole::NeedToKnow,
            score: 0.8,
            rationale: "Test".to_string(),
            articles: vec!["art-1".to_string()],
            quality_metrics: ClusterQualityMetrics::default(),
            score_breakdown: ScoreBreakdown::default(),
            representative_articles: Vec::new(),
            top_entities: Vec::new(),
            source_names: Vec::new(),
            genre: None,
        };
        let result_with_topic = PulseResult::new(job_id, PulseVersion::V4, vec![topic]);
        assert!(result_with_topic.is_success());
        assert_eq!(result_with_topic.topic_count(), 1);
    }

    #[test]
    fn syndication_status_display() {
        assert_eq!(SyndicationStatus::Original.to_string(), "original");
        assert_eq!(
            SyndicationStatus::CanonicalMatch.to_string(),
            "canonical_match"
        );
        assert_eq!(SyndicationStatus::WireSource.to_string(), "wire_source");
        assert_eq!(SyndicationStatus::TitleSimilar.to_string(), "title_similar");
    }

    #[test]
    fn generation_status_display() {
        assert_eq!(GenerationStatus::Running.to_string(), "running");
        assert_eq!(GenerationStatus::Succeeded.to_string(), "succeeded");
        assert_eq!(GenerationStatus::Failed.to_string(), "failed");
    }

    #[test]
    fn serde_roundtrip() {
        let topic = PulseTopic {
            cluster_id: 42,
            role: TopicRole::Trend,
            score: 0.75,
            rationale: "Trending topic".to_string(),
            articles: vec!["a1".to_string(), "a2".to_string()],
            quality_metrics: ClusterQualityMetrics {
                cohesion: 0.8,
                ambiguity: 0.2,
                entity_consistency: 0.9,
                tier: QualityTier::Ok,
            },
            score_breakdown: ScoreBreakdown {
                impact_score: 0.2,
                burst_score: 0.4,
                novelty_score: 0.1,
                recency_score: 0.05,
            },
            representative_articles: vec![RepresentativeArticle {
                article_id: "a1".to_string(),
                title: "Test Article".to_string(),
                source_url: "https://example.com/a1".to_string(),
                source_name: "Example".to_string(),
                published_at: "2026-01-31T12:00:00Z".to_string(),
            }],
            top_entities: vec!["Entity1".to_string()],
            source_names: vec!["Example".to_string()],
            genre: Some("Tech".to_string()),
        };

        let json = serde_json::to_string(&topic).unwrap();
        let parsed: PulseTopic = serde_json::from_str(&json).unwrap();
        assert_eq!(topic, parsed);
    }
}
