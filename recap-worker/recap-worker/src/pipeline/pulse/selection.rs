//! Role-based topic selection for Evening Pulse.
//!
//! This module implements the selection algorithm that chooses up to 3 topics
//! from evaluated clusters, assigning each a distinct role:
//!
//! 1. **NeedToKnow** - High-impact news (prioritizes impact score)
//! 2. **Trend** - Trending topics (prioritizes burst score)
//! 3. **Serendipity** - Unexpected discoveries (prioritizes novelty score)
//!
//! ## Selection Algorithm
//!
//! Topics are selected in order: NeedToKnow → Trend → Serendipity.
//! Each role uses different weight configurations to score candidates.
//! Once a cluster is selected, it cannot be selected again for other roles.
//!
//! ## Graceful Degradation
//!
//! When there aren't enough high-quality clusters, the selection process
//! degrades gracefully through multiple levels, relaxing constraints
//! to provide at least some output.

use std::collections::HashSet;

use async_trait::async_trait;

use super::config::PulseSelectionConfig;
use super::types::{
    ClusterWithMetrics, PulseTopic, QualityTier, RoleWeights, ScoreBreakdown, SelectionTrace,
    TopicRole,
};

/// Trait for topic selection.
#[async_trait]
pub trait TopicSelector: Send + Sync {
    /// Select topics from the given clusters.
    ///
    /// # Arguments
    ///
    /// * `clusters` - Evaluated clusters with quality metrics and scores.
    /// * `max_topics` - Maximum number of topics to select (typically 3).
    ///
    /// # Returns
    ///
    /// Selected topics with roles and rationales.
    async fn select(
        &self,
        clusters: &[ClusterWithMetrics],
        max_topics: usize,
    ) -> SelectionResult;
}

/// Result of topic selection.
#[derive(Debug, Clone)]
pub struct SelectionResult {
    /// Selected topics.
    pub topics: Vec<PulseTopic>,
    /// Selection trace for debugging.
    pub trace: SelectionTrace,
    /// Fallback level used (0 = normal, higher = more degradation).
    pub fallback_level: u8,
}

impl SelectionResult {
    /// Check if the selection was successful (at least one topic).
    #[must_use]
    pub fn is_success(&self) -> bool {
        !self.topics.is_empty()
    }

    /// Check if all requested topics were filled.
    #[must_use]
    pub fn is_complete(&self, max_topics: usize) -> bool {
        self.topics.len() >= max_topics
    }
}

/// Default implementation of topic selection.
pub struct DefaultTopicSelector {
    config: PulseSelectionConfig,
}

impl DefaultTopicSelector {
    /// Create a new selector with the given configuration.
    #[must_use]
    pub fn new(config: PulseSelectionConfig) -> Self {
        Self { config }
    }
}

impl Default for DefaultTopicSelector {
    fn default() -> Self {
        Self::new(PulseSelectionConfig::default())
    }
}

#[async_trait]
impl TopicSelector for DefaultTopicSelector {
    async fn select(
        &self,
        clusters: &[ClusterWithMetrics],
        max_topics: usize,
    ) -> SelectionResult {
        select_topics_with_fallback(clusters, max_topics, &self.config)
    }
}

/// Select topics with graceful degradation.
fn select_topics_with_fallback(
    clusters: &[ClusterWithMetrics],
    max_topics: usize,
    config: &PulseSelectionConfig,
) -> SelectionResult {
    // Level 0: Normal selection (OK tier only)
    let result = select_topics(clusters, max_topics, config, QualityTier::Ok);
    if result.topics.len() >= max_topics {
        return result;
    }

    // Level 1: Include Caution tier
    let result = select_topics(clusters, max_topics, config, QualityTier::Caution);
    if result.topics.len() >= max_topics {
        return SelectionResult {
            fallback_level: 1,
            ..result
        };
    }

    // Level 2: Include all tiers (even NG with lower priority)
    let result = select_topics(clusters, max_topics, config, QualityTier::Ng);
    if result.topics.len() >= 2 {
        return SelectionResult {
            fallback_level: 2,
            ..result
        };
    }

    // Level 3-5: Reduce topic count targets
    // Level 3: 2 topics
    if result.topics.len() >= 2 {
        return SelectionResult {
            fallback_level: 3,
            ..result
        };
    }

    // Level 4: 1 topic (Quiet Day Mode)
    if !result.topics.is_empty() {
        return SelectionResult {
            fallback_level: 4,
            ..result
        };
    }

    // Level 5-6: No topics available
    SelectionResult {
        topics: Vec::new(),
        trace: SelectionTrace::default(),
        fallback_level: if clusters.is_empty() { 6 } else { 5 },
    }
}

/// Select topics from clusters with a minimum quality tier requirement.
fn select_topics(
    clusters: &[ClusterWithMetrics],
    max_topics: usize,
    config: &PulseSelectionConfig,
    min_tier: QualityTier,
) -> SelectionResult {
    let mut selected = Vec::with_capacity(max_topics.min(3));
    let mut used_cluster_ids: HashSet<i64> = HashSet::new();
    let mut trace = SelectionTrace::default();

    // Filter clusters by minimum quality tier
    let eligible_clusters: Vec<&ClusterWithMetrics> = clusters
        .iter()
        .filter(|c| tier_meets_minimum(c.quality_metrics.tier, min_tier))
        .collect();

    // Count NG clusters for trace
    trace.excluded_ng_clusters = clusters
        .iter()
        .filter(|c| c.quality_metrics.tier == QualityTier::Ng)
        .count();

    // Role priority order
    let roles = [TopicRole::NeedToKnow, TopicRole::Trend, TopicRole::Serendipity];

    for role in roles {
        if selected.len() >= max_topics {
            break;
        }

        let weights = get_weights_for_role(role, config);

        // Find best cluster for this role
        let candidate = eligible_clusters
            .iter()
            .filter(|c| !used_cluster_ids.contains(&c.cluster_id))
            .filter(|c| {
                let score = score_cluster_for_role(c, weights);
                score >= config.min_score_threshold
            })
            .max_by(|a, b| {
                let score_a = score_cluster_for_role(a, weights);
                let score_b = score_cluster_for_role(b, weights);
                score_a
                    .partial_cmp(&score_b)
                    .unwrap_or(std::cmp::Ordering::Equal)
            });

        // Update trace
        let candidates_for_role = eligible_clusters
            .iter()
            .filter(|c| !used_cluster_ids.contains(&c.cluster_id))
            .count();

        match role {
            TopicRole::NeedToKnow => trace.need_to_know_candidates = candidates_for_role,
            TopicRole::Trend => trace.trend_candidates = candidates_for_role,
            TopicRole::Serendipity => trace.serendipity_candidates = candidates_for_role,
        }

        if let Some(cluster) = candidate {
            let score = score_cluster_for_role(cluster, weights);
            let breakdown = ScoreBreakdown {
                impact_score: cluster.impact_score * weights.impact,
                burst_score: cluster.burst_score * weights.burst,
                novelty_score: cluster.novelty_score * weights.novelty,
                recency_score: cluster.recency_score * weights.recency,
            };

            selected.push(PulseTopic {
                cluster_id: cluster.cluster_id,
                role,
                score,
                rationale: String::new(), // Filled in rationale generation
                articles: cluster.article_ids.clone(),
                quality_metrics: cluster.quality_metrics.clone(),
                score_breakdown: breakdown,
            });

            used_cluster_ids.insert(cluster.cluster_id);
        }
    }

    trace.excluded_duplicates = used_cluster_ids.len().saturating_sub(selected.len());

    SelectionResult {
        topics: selected,
        trace,
        fallback_level: 0,
    }
}

/// Calculate score for a cluster given a role's weights.
#[must_use]
pub fn score_cluster_for_role(cluster: &ClusterWithMetrics, weights: &RoleWeights) -> f32 {
    cluster.impact_score * weights.impact
        + cluster.burst_score * weights.burst
        + cluster.novelty_score * weights.novelty
        + cluster.recency_score * weights.recency
}

/// Get the weights configuration for a role.
fn get_weights_for_role(role: TopicRole, config: &PulseSelectionConfig) -> &RoleWeights {
    match role {
        TopicRole::NeedToKnow => &config.need_to_know_weights,
        TopicRole::Trend => &config.trend_weights,
        TopicRole::Serendipity => &config.serendipity_weights,
    }
}

/// Check if a tier meets the minimum requirement.
fn tier_meets_minimum(tier: QualityTier, min: QualityTier) -> bool {
    match min {
        QualityTier::Ok => matches!(tier, QualityTier::Ok),
        QualityTier::Caution => matches!(tier, QualityTier::Ok | QualityTier::Caution),
        QualityTier::Ng => true, // All tiers meet NG minimum
    }
}

#[cfg(test)]
mod tests {
    use super::super::types::ClusterQualityMetrics;
    use super::*;

    fn cluster(
        id: i64,
        tier: QualityTier,
        impact: f32,
        burst: f32,
        novelty: f32,
        recency: f32,
    ) -> ClusterWithMetrics {
        ClusterWithMetrics {
            cluster_id: id,
            article_ids: vec![format!("art-{id}")],
            label: Some(format!("Cluster {id}")),
            quality_metrics: ClusterQualityMetrics {
                cohesion: 0.5,
                ambiguity: 0.3,
                entity_consistency: 0.6,
                tier,
            },
            impact_score: impact,
            burst_score: burst,
            novelty_score: novelty,
            recency_score: recency,
            top_entities: vec![],
            syndication_status: None,
        }
    }

    #[test]
    fn test_score_cluster_for_role() {
        let c = cluster(1, QualityTier::Ok, 0.8, 0.6, 0.4, 0.7);

        let need_to_know_weights = RoleWeights::need_to_know();
        let score = score_cluster_for_role(&c, &need_to_know_weights);
        // 0.8*0.5 + 0.6*0.15 + 0.4*0.1 + 0.7*0.25 = 0.4 + 0.09 + 0.04 + 0.175 = 0.705
        assert!((score - 0.705).abs() < 0.01);

        let trend_weights = RoleWeights::trend();
        let score = score_cluster_for_role(&c, &trend_weights);
        // 0.8*0.2 + 0.6*0.5 + 0.4*0.1 + 0.7*0.2 = 0.16 + 0.3 + 0.04 + 0.14 = 0.64
        assert!((score - 0.64).abs() < 0.01);

        let serendipity_weights = RoleWeights::serendipity();
        let score = score_cluster_for_role(&c, &serendipity_weights);
        // 0.8*0.15 + 0.6*0.15 + 0.4*0.5 + 0.7*0.2 = 0.12 + 0.09 + 0.2 + 0.14 = 0.55
        assert!((score - 0.55).abs() < 0.01);
    }

    #[test]
    fn test_tier_meets_minimum() {
        // Ok minimum
        assert!(tier_meets_minimum(QualityTier::Ok, QualityTier::Ok));
        assert!(!tier_meets_minimum(QualityTier::Caution, QualityTier::Ok));
        assert!(!tier_meets_minimum(QualityTier::Ng, QualityTier::Ok));

        // Caution minimum
        assert!(tier_meets_minimum(QualityTier::Ok, QualityTier::Caution));
        assert!(tier_meets_minimum(QualityTier::Caution, QualityTier::Caution));
        assert!(!tier_meets_minimum(QualityTier::Ng, QualityTier::Caution));

        // Ng minimum (accepts all)
        assert!(tier_meets_minimum(QualityTier::Ok, QualityTier::Ng));
        assert!(tier_meets_minimum(QualityTier::Caution, QualityTier::Ng));
        assert!(tier_meets_minimum(QualityTier::Ng, QualityTier::Ng));
    }

    #[test]
    fn test_select_topics_basic() {
        let config = PulseSelectionConfig::default();
        let clusters = vec![
            cluster(1, QualityTier::Ok, 0.9, 0.3, 0.2, 0.5), // High impact
            cluster(2, QualityTier::Ok, 0.3, 0.9, 0.2, 0.5), // High burst
            cluster(3, QualityTier::Ok, 0.3, 0.3, 0.9, 0.5), // High novelty
        ];

        let result = select_topics(&clusters, 3, &config, QualityTier::Ok);

        assert_eq!(result.topics.len(), 3);
        assert_eq!(result.topics[0].role, TopicRole::NeedToKnow);
        assert_eq!(result.topics[1].role, TopicRole::Trend);
        assert_eq!(result.topics[2].role, TopicRole::Serendipity);

        // Check cluster assignments match expected scores
        assert_eq!(result.topics[0].cluster_id, 1); // Highest impact
        assert_eq!(result.topics[1].cluster_id, 2); // Highest burst
        assert_eq!(result.topics[2].cluster_id, 3); // Highest novelty
    }

    #[test]
    fn test_select_topics_no_duplicate_clusters() {
        let config = PulseSelectionConfig::default();
        // Single cluster that scores well for all roles
        let clusters = vec![cluster(1, QualityTier::Ok, 0.9, 0.9, 0.9, 0.9)];

        let result = select_topics(&clusters, 3, &config, QualityTier::Ok);

        // Should only select once
        assert_eq!(result.topics.len(), 1);
        assert_eq!(result.topics[0].cluster_id, 1);
    }

    #[test]
    fn test_select_topics_excludes_ng() {
        let config = PulseSelectionConfig::default();
        let clusters = vec![
            cluster(1, QualityTier::Ng, 0.9, 0.9, 0.9, 0.9),
            cluster(2, QualityTier::Ok, 0.5, 0.5, 0.5, 0.5),
        ];

        let result = select_topics(&clusters, 3, &config, QualityTier::Ok);

        // Should only select the OK cluster
        assert_eq!(result.topics.len(), 1);
        assert_eq!(result.topics[0].cluster_id, 2);
        assert_eq!(result.trace.excluded_ng_clusters, 1);
    }

    #[test]
    fn test_select_topics_fallback_to_caution() {
        let config = PulseSelectionConfig::default();
        let clusters = vec![
            cluster(1, QualityTier::Caution, 0.9, 0.3, 0.2, 0.5),
            cluster(2, QualityTier::Caution, 0.3, 0.9, 0.2, 0.5),
        ];

        // With OK minimum, no topics
        let result = select_topics(&clusters, 3, &config, QualityTier::Ok);
        assert_eq!(result.topics.len(), 0);

        // With Caution minimum, both selected
        let result = select_topics(&clusters, 3, &config, QualityTier::Caution);
        assert_eq!(result.topics.len(), 2);
    }

    #[tokio::test]
    async fn test_default_selector() {
        let selector = DefaultTopicSelector::default();
        let clusters = vec![
            cluster(1, QualityTier::Ok, 0.9, 0.3, 0.2, 0.5),
            cluster(2, QualityTier::Ok, 0.3, 0.9, 0.2, 0.5),
            cluster(3, QualityTier::Ok, 0.3, 0.3, 0.9, 0.5),
        ];

        let result = selector.select(&clusters, 3).await;

        assert!(result.is_success());
        assert!(result.is_complete(3));
        assert_eq!(result.fallback_level, 0);
    }

    #[tokio::test]
    async fn test_selector_with_fallback() {
        let selector = DefaultTopicSelector::default();
        // Only Caution clusters
        let clusters = vec![
            cluster(1, QualityTier::Caution, 0.9, 0.3, 0.2, 0.5),
            cluster(2, QualityTier::Caution, 0.3, 0.9, 0.2, 0.5),
            cluster(3, QualityTier::Caution, 0.3, 0.3, 0.9, 0.5),
        ];

        let result = selector.select(&clusters, 3).await;

        assert!(result.is_success());
        assert!(result.is_complete(3));
        assert_eq!(result.fallback_level, 1); // Fell back to Caution tier
    }

    #[tokio::test]
    async fn test_selector_empty_clusters() {
        let selector = DefaultTopicSelector::default();
        let clusters: Vec<ClusterWithMetrics> = vec![];

        let result = selector.select(&clusters, 3).await;

        assert!(!result.is_success());
        assert_eq!(result.fallback_level, 6); // Maximum fallback
    }

    #[test]
    fn test_selection_trace() {
        let config = PulseSelectionConfig::default();
        let clusters = vec![
            cluster(1, QualityTier::Ok, 0.9, 0.3, 0.2, 0.5),
            cluster(2, QualityTier::Ok, 0.3, 0.9, 0.2, 0.5),
            cluster(3, QualityTier::Ng, 0.5, 0.5, 0.5, 0.5), // NG cluster
        ];

        let result = select_topics(&clusters, 3, &config, QualityTier::Ok);

        assert_eq!(result.trace.need_to_know_candidates, 2);
        assert_eq!(result.trace.trend_candidates, 1); // One already used
        assert_eq!(result.trace.excluded_ng_clusters, 1);
    }

    #[test]
    fn test_min_score_threshold() {
        let config = PulseSelectionConfig {
            min_score_threshold: 0.9, // Very high threshold
            ..Default::default()
        };

        let clusters = vec![cluster(1, QualityTier::Ok, 0.5, 0.5, 0.5, 0.5)];

        let result = select_topics(&clusters, 3, &config, QualityTier::Ok);

        // Score is 0.5 which is below 0.9 threshold
        assert_eq!(result.topics.len(), 0);
    }

    #[test]
    fn test_score_breakdown() {
        let config = PulseSelectionConfig::default();
        let clusters = vec![cluster(1, QualityTier::Ok, 0.8, 0.6, 0.4, 0.7)];

        let result = select_topics(&clusters, 1, &config, QualityTier::Ok);

        assert_eq!(result.topics.len(), 1);
        let breakdown = &result.topics[0].score_breakdown;

        // NeedToKnow weights: impact=0.5, burst=0.15, novelty=0.1, recency=0.25
        assert!((breakdown.impact_score - 0.4).abs() < 0.01); // 0.8 * 0.5
        assert!((breakdown.burst_score - 0.09).abs() < 0.01); // 0.6 * 0.15
        assert!((breakdown.novelty_score - 0.04).abs() < 0.01); // 0.4 * 0.1
        assert!((breakdown.recency_score - 0.175).abs() < 0.01); // 0.7 * 0.25
    }
}
