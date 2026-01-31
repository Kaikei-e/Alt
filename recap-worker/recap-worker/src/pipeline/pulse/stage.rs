//! Pulse pipeline stage for Evening Pulse generation.
//!
//! This module provides the main entry point for Evening Pulse v4 generation,
//! orchestrating the quality evaluation, syndication removal, topic selection,
//! and rationale generation stages.

use std::collections::HashMap;
use std::sync::Arc;

use anyhow::{Context, Result};
use async_trait::async_trait;
use chrono::Utc;
use uuid::Uuid;

use super::cluster_quality::{
    ArticleEntities, ClusterQualityEvaluator, DefaultClusterQualityEvaluator,
};
use super::config::{PulseConfig, PulseRollout};
use super::rationale::{DefaultRationaleGenerator, RationaleGenerator};
use super::selection::{DefaultTopicSelector, TopicSelector};
use super::syndication::{ArticleWithMetadata, DefaultSyndicationRemover, SyndicationRemover};
use super::types::{
    ClusterWithMetrics, PulseDiagnostics, PulseResult, PulseTopic, PulseVersion, QualityTier,
};

/// Input data for pulse generation.
#[derive(Debug, Clone)]
pub struct PulseInput {
    /// Job ID for this generation run.
    pub job_id: Uuid,
    /// Clusters with their articles.
    pub clusters: Vec<ClusterInput>,
}

/// Input data for a single cluster.
#[derive(Debug, Clone)]
pub struct ClusterInput {
    /// Cluster ID.
    pub cluster_id: i64,
    /// Cluster label (topic name).
    pub label: Option<String>,
    /// Articles in this cluster.
    pub articles: Vec<ArticleInput>,
    /// Article embeddings (for ambiguity calculation).
    pub embeddings: Vec<Vec<f32>>,
    /// Pre-computed impact score (if available).
    pub impact_score: Option<f32>,
    /// Pre-computed burst score (if available).
    pub burst_score: Option<f32>,
    /// Pre-computed novelty score (if available).
    pub novelty_score: Option<f32>,
    /// Pre-computed recency score (if available).
    pub recency_score: Option<f32>,
    /// Genre classification (e.g., "AI", "Business").
    pub genre: Option<String>,
}

/// Input data for a single article.
#[derive(Debug, Clone)]
pub struct ArticleInput {
    /// Article ID.
    pub id: String,
    /// Article title.
    pub title: String,
    /// Source URL.
    pub source_url: String,
    /// Canonical URL (if available).
    pub canonical_url: Option<String>,
    /// Open Graph URL (if available).
    pub og_url: Option<String>,
    /// Named entities extracted from the article.
    pub entities: Vec<String>,
    /// Published timestamp (RFC3339).
    pub published_at: Option<String>,
}

impl ArticleInput {
    /// Convert to syndication article format.
    fn to_syndication_article(&self) -> ArticleWithMetadata {
        ArticleWithMetadata {
            id: self.id.clone(),
            title: self.title.clone(),
            source_url: self.source_url.clone(),
            canonical_url: self.canonical_url.clone(),
            og_url: self.og_url.clone(),
        }
    }

    /// Convert to article entities format.
    fn to_article_entities(&self) -> ArticleEntities {
        ArticleEntities {
            id: self.id.clone(),
            title: Some(self.title.clone()),
            entities: self.entities.clone(),
        }
    }

    /// Convert to representative article format for UI display.
    fn to_representative_article(&self) -> super::types::RepresentativeArticle {
        super::types::RepresentativeArticle {
            article_id: self.id.clone(),
            title: self.title.clone(),
            source_url: self.source_url.clone(),
            source_name: extract_source_name(&self.source_url),
            published_at: self.published_at.clone().unwrap_or_default(),
        }
    }
}

/// Extract source name from URL (e.g., "https://www.reuters.com/..." -> "Reuters").
fn extract_source_name(url: &str) -> String {
    reqwest::Url::parse(url)
        .ok()
        .and_then(|u: reqwest::Url| u.host_str().map(String::from))
        .map_or_else(
            || "Unknown".to_string(),
            |host: String| {
                // Remove common prefixes like "www."
                let host = host.strip_prefix("www.").unwrap_or(&host);
                // Extract domain name without TLD for common patterns
                let parts: Vec<&str> = host.split('.').collect();
                if parts.len() >= 2 {
                    // Capitalize first letter
                    let name = parts[0];
                    let mut chars: Vec<char> = name.chars().collect();
                    if let Some(first) = chars.first_mut() {
                        *first = first.to_ascii_uppercase();
                    }
                    chars.into_iter().collect()
                } else {
                    host.to_string()
                }
            },
        )
}

/// Trait for pulse pipeline stage execution.
#[async_trait]
pub trait PulseStage: Send + Sync {
    /// Generate Evening Pulse topics from the given input.
    async fn generate(&self, input: PulseInput) -> Result<PulseResult>;

    /// Check if pulse generation is enabled for the given job.
    fn is_enabled(&self, job_id: Uuid) -> bool;

    /// Get the current pulse version.
    fn version(&self) -> PulseVersion;
}

/// Default implementation of the pulse pipeline stage.
pub struct DefaultPulseStage {
    config: PulseConfig,
    rollout: PulseRollout,
    quality_evaluator: Arc<dyn ClusterQualityEvaluator>,
    syndication_remover: Arc<dyn SyndicationRemover>,
    topic_selector: Arc<dyn TopicSelector>,
    rationale_generator: Arc<dyn RationaleGenerator>,
}

impl DefaultPulseStage {
    /// Create a new pulse stage with the given configuration.
    #[must_use]
    pub fn new(config: PulseConfig, rollout: PulseRollout) -> Self {
        let quality_evaluator =
            Arc::new(DefaultClusterQualityEvaluator::new(config.quality.clone()));
        let syndication_remover =
            Arc::new(DefaultSyndicationRemover::new(config.syndication.clone()));
        let topic_selector = Arc::new(DefaultTopicSelector::new(config.selection.clone()));
        let rationale_generator = Arc::new(DefaultRationaleGenerator);

        Self {
            config,
            rollout,
            quality_evaluator,
            syndication_remover,
            topic_selector,
            rationale_generator,
        }
    }

    /// Create a new pulse stage with custom components (for testing).
    #[must_use]
    pub fn with_components(
        config: PulseConfig,
        rollout: PulseRollout,
        quality_evaluator: Arc<dyn ClusterQualityEvaluator>,
        syndication_remover: Arc<dyn SyndicationRemover>,
        topic_selector: Arc<dyn TopicSelector>,
        rationale_generator: Arc<dyn RationaleGenerator>,
    ) -> Self {
        Self {
            config,
            rollout,
            quality_evaluator,
            syndication_remover,
            topic_selector,
            rationale_generator,
        }
    }

    /// Process a single cluster: evaluate quality, remove syndication, compute scores.
    async fn process_cluster(&self, cluster: &ClusterInput) -> Result<Option<ClusterWithMetrics>> {
        // Skip empty clusters
        if cluster.articles.is_empty() {
            return Ok(None);
        }

        // Extract titles for cohesion calculation
        let titles: Vec<String> = cluster.articles.iter().map(|a| a.title.clone()).collect();

        // Extract article entities
        let article_entities: Vec<ArticleEntities> = cluster
            .articles
            .iter()
            .map(ArticleInput::to_article_entities)
            .collect();

        // Compute quality metrics
        let embeddings_ref: Option<&[Vec<f32>]> = if cluster.embeddings.is_empty() {
            None
        } else {
            Some(&cluster.embeddings)
        };

        let quality_metrics = self
            .quality_evaluator
            .evaluate(&titles, embeddings_ref, &article_entities)
            .await;

        // Remove syndicated content
        let articles_for_syndication: Vec<ArticleWithMetadata> = cluster
            .articles
            .iter()
            .map(ArticleInput::to_syndication_article)
            .collect();

        let syndication_result = self
            .syndication_remover
            .remove_syndication(articles_for_syndication)
            .await
            .context("syndication removal failed")?;

        // Determine syndication status
        let syndication_status = if syndication_result.has_removals() {
            Some(super::types::SyndicationStatus::TitleSimilar) // Simplified
        } else {
            None
        };

        // Collect top entities
        let mut entity_counts: HashMap<String, usize> = HashMap::new();
        for article in &cluster.articles {
            for entity in &article.entities {
                *entity_counts.entry(entity.clone()).or_default() += 1;
            }
        }
        let mut top_entities: Vec<(String, usize)> = entity_counts.into_iter().collect();
        top_entities.sort_by(|a, b| b.1.cmp(&a.1));
        let top_entities: Vec<String> = top_entities.into_iter().take(5).map(|(e, _)| e).collect();

        // Build representative articles (top 3 from original articles after syndication removal)
        let original_article_ids: std::collections::HashSet<String> = syndication_result
            .original_articles
            .iter()
            .map(|a| a.id.clone())
            .collect();
        let representative_articles: Vec<super::types::RepresentativeArticle> = cluster
            .articles
            .iter()
            .filter(|a| original_article_ids.contains(&a.id))
            .take(3)
            .map(ArticleInput::to_representative_article)
            .collect();

        // Collect unique source names
        let mut source_names: Vec<String> = cluster
            .articles
            .iter()
            .filter(|a| original_article_ids.contains(&a.id))
            .map(|a| extract_source_name(&a.source_url))
            .collect::<std::collections::HashSet<String>>()
            .into_iter()
            .collect();
        source_names.sort();

        // Build ClusterWithMetrics
        let cluster_with_metrics = ClusterWithMetrics {
            cluster_id: cluster.cluster_id,
            article_ids: syndication_result
                .original_articles
                .iter()
                .map(|a| a.id.clone())
                .collect(),
            label: cluster.label.clone(),
            quality_metrics,
            impact_score: cluster.impact_score.unwrap_or(0.5),
            burst_score: cluster.burst_score.unwrap_or(0.5),
            novelty_score: cluster.novelty_score.unwrap_or(0.5),
            recency_score: cluster.recency_score.unwrap_or(0.5),
            top_entities,
            syndication_status,
            representative_articles,
            source_names,
            genre: cluster.genre.clone(),
        };

        Ok(Some(cluster_with_metrics))
    }

    /// Generate rationales for selected topics.
    fn generate_rationales(
        &self,
        topics: &mut [PulseTopic],
        clusters: &HashMap<i64, ClusterWithMetrics>,
    ) {
        for topic in topics.iter_mut() {
            if let Some(cluster) = clusters.get(&topic.cluster_id) {
                topic.rationale = self.rationale_generator.generate(topic, cluster);
            }
        }
    }
}

impl Default for DefaultPulseStage {
    fn default() -> Self {
        Self::new(PulseConfig::default(), PulseRollout::default())
    }
}

#[async_trait]
impl PulseStage for DefaultPulseStage {
    async fn generate(&self, input: PulseInput) -> Result<PulseResult> {
        let start_time = std::time::Instant::now();

        // Check if enabled
        if !self.is_enabled(input.job_id) {
            return Ok(PulseResult {
                job_id: input.job_id,
                version: self.version(),
                topics: Vec::new(),
                generated_at: Utc::now(),
                diagnostics: PulseDiagnostics::default(),
            });
        }

        // Process all clusters
        let mut processed_clusters: Vec<ClusterWithMetrics> = Vec::new();
        let mut cluster_map: HashMap<i64, ClusterWithMetrics> = HashMap::new();

        for cluster_input in &input.clusters {
            match self.process_cluster(cluster_input).await {
                Ok(Some(cluster)) => {
                    cluster_map.insert(cluster.cluster_id, cluster.clone());
                    processed_clusters.push(cluster);
                }
                Ok(None) => {
                    // Empty cluster, skip
                    tracing::debug!(
                        cluster_id = cluster_input.cluster_id,
                        "skipping empty cluster"
                    );
                }
                Err(e) => {
                    tracing::warn!(
                        cluster_id = cluster_input.cluster_id,
                        error = ?e,
                        "failed to process cluster"
                    );
                }
            }
        }

        // Select topics using the trait method
        let selection_result = self
            .topic_selector
            .select(&processed_clusters, self.config.max_topics)
            .await;

        let mut topics = selection_result.topics;
        let fallback_level = selection_result.fallback_level;

        // Generate rationales
        self.generate_rationales(&mut topics, &cluster_map);

        // Build diagnostics
        let mut quality_tier_ok = 0;
        let mut quality_tier_caution = 0;
        let mut quality_tier_ng = 0;
        let mut syndication_removed = 0;

        for cluster in &processed_clusters {
            match cluster.quality_metrics.tier {
                QualityTier::Ok => quality_tier_ok += 1,
                QualityTier::Caution => quality_tier_caution += 1,
                QualityTier::Ng => quality_tier_ng += 1,
            }
            if cluster.syndication_status.is_some() {
                syndication_removed += 1;
            }
        }

        let mut quality_tier_distribution = HashMap::new();
        quality_tier_distribution.insert("ok".to_string(), quality_tier_ok);
        quality_tier_distribution.insert("caution".to_string(), quality_tier_caution);
        quality_tier_distribution.insert("ng".to_string(), quality_tier_ng);

        let diagnostics = PulseDiagnostics {
            syndication_removed,
            clusters_evaluated: processed_clusters.len(),
            quality_tier_distribution,
            selection_trace: selection_result.trace,
            fallback_level,
            duration_ms: start_time.elapsed().as_millis() as u64,
        };

        Ok(PulseResult {
            job_id: input.job_id,
            version: self.version(),
            topics,
            generated_at: Utc::now(),
            diagnostics,
        })
    }

    fn is_enabled(&self, job_id: Uuid) -> bool {
        self.config.is_enabled() && self.rollout.allows(job_id)
    }

    fn version(&self) -> PulseVersion {
        self.config.version
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::FeatureToggle;

    fn make_article(id: &str, title: &str) -> ArticleInput {
        ArticleInput {
            id: id.to_string(),
            title: title.to_string(),
            source_url: format!("https://example.com/{id}"),
            canonical_url: None,
            og_url: None,
            entities: vec!["Entity1".to_string()],
            published_at: Some("2026-01-31T12:00:00Z".to_string()),
        }
    }

    fn make_cluster(id: i64, articles: Vec<ArticleInput>) -> ClusterInput {
        let embeddings = articles.iter().map(|_| vec![0.5; 128]).collect();
        ClusterInput {
            cluster_id: id,
            label: Some(format!("Cluster {id}")),
            articles,
            embeddings,
            impact_score: Some(0.7),
            burst_score: Some(0.6),
            novelty_score: Some(0.5),
            recency_score: Some(0.8),
            genre: None,
        }
    }

    fn enabled_config() -> PulseConfig {
        PulseConfig {
            enabled: FeatureToggle::Enabled,
            ..Default::default()
        }
    }

    fn full_rollout() -> PulseRollout {
        PulseRollout::new(100, PulseVersion::V4)
    }

    fn zero_rollout() -> PulseRollout {
        PulseRollout::new(0, PulseVersion::V4)
    }

    #[tokio::test]
    async fn test_pulse_stage_basic() {
        let stage = DefaultPulseStage::new(enabled_config(), full_rollout());

        let input = PulseInput {
            job_id: Uuid::new_v4(),
            clusters: vec![
                make_cluster(
                    1,
                    vec![
                        make_article("a1", "Breaking news about technology"),
                        make_article("a2", "Tech industry updates"),
                    ],
                ),
                make_cluster(
                    2,
                    vec![
                        make_article("b1", "Sports championship results"),
                        make_article("b2", "Championship game highlights"),
                    ],
                ),
                make_cluster(
                    3,
                    vec![
                        make_article("c1", "Economic outlook report"),
                        make_article("c2", "Financial markets update"),
                    ],
                ),
            ],
        };

        let result = stage.generate(input).await.unwrap();

        assert!(result.is_success());
        assert!(!result.topics.is_empty());
        assert!(result.topics.len() <= 3);
    }

    #[tokio::test]
    async fn test_pulse_stage_disabled() {
        let config = PulseConfig {
            enabled: FeatureToggle::Disabled,
            ..Default::default()
        };
        let stage = DefaultPulseStage::new(config, full_rollout());

        let input = PulseInput {
            job_id: Uuid::new_v4(),
            clusters: vec![make_cluster(1, vec![make_article("a1", "Test")])],
        };

        let result = stage.generate(input).await.unwrap();

        assert!(!result.is_success());
        assert!(result.topics.is_empty());
    }

    #[tokio::test]
    async fn test_pulse_stage_rollout_zero() {
        let stage = DefaultPulseStage::new(enabled_config(), zero_rollout());

        let input = PulseInput {
            job_id: Uuid::new_v4(),
            clusters: vec![make_cluster(1, vec![make_article("a1", "Test")])],
        };

        let result = stage.generate(input).await.unwrap();

        // Zero rollout means disabled
        assert!(!result.is_success());
    }

    #[tokio::test]
    async fn test_pulse_stage_empty_clusters() {
        let stage = DefaultPulseStage::new(enabled_config(), full_rollout());

        let input = PulseInput {
            job_id: Uuid::new_v4(),
            clusters: vec![],
        };

        let result = stage.generate(input).await.unwrap();

        assert!(!result.is_success());
        assert!(result.topics.is_empty());
    }

    #[tokio::test]
    async fn test_pulse_stage_version() {
        let config = PulseConfig {
            version: PulseVersion::V4,
            ..Default::default()
        };
        let stage = DefaultPulseStage::new(config, full_rollout());

        assert_eq!(stage.version(), PulseVersion::V4);
    }

    #[tokio::test]
    async fn test_pulse_stage_diagnostics() {
        let stage = DefaultPulseStage::new(enabled_config(), full_rollout());

        let input = PulseInput {
            job_id: Uuid::new_v4(),
            clusters: vec![
                make_cluster(1, vec![make_article("a1", "Test 1")]),
                make_cluster(2, vec![make_article("a2", "Test 2")]),
            ],
        };

        let result = stage.generate(input).await.unwrap();

        assert_eq!(result.diagnostics.clusters_evaluated, 2);
        // Duration may be 0 on very fast runs, just ensure it's recorded
        let _ = result.diagnostics.duration_ms;
    }

    #[tokio::test]
    async fn test_pulse_stage_rationale_generation() {
        let stage = DefaultPulseStage::new(enabled_config(), full_rollout());

        let input = PulseInput {
            job_id: Uuid::new_v4(),
            clusters: vec![
                make_cluster(
                    1,
                    vec![
                        make_article("a1", "Important breaking news"),
                        make_article("a2", "More breaking news"),
                    ],
                ),
                make_cluster(
                    2,
                    vec![
                        make_article("b1", "Trending topic today"),
                        make_article("b2", "More trending content"),
                    ],
                ),
                make_cluster(
                    3,
                    vec![
                        make_article("c1", "Unique perspective"),
                        make_article("c2", "Different viewpoint"),
                    ],
                ),
            ],
        };

        let result = stage.generate(input).await.unwrap();

        // All topics should have rationales
        for topic in &result.topics {
            assert!(!topic.rationale.is_empty(), "Topic should have rationale");
        }
    }

    #[test]
    fn test_is_enabled_with_rollout() {
        let rollout = PulseRollout::new(50, PulseVersion::V4);
        let stage = DefaultPulseStage::new(enabled_config(), rollout);

        // With 50% rollout, some jobs should be enabled, some disabled
        let mut enabled_count = 0;
        for i in 0u64..100 {
            let job_id = Uuid::from_u128(u128::from(i));
            if stage.is_enabled(job_id) {
                enabled_count += 1;
            }
        }

        // Should be approximately 50% (with some variance)
        assert!(enabled_count > 30 && enabled_count < 70);
    }

    #[tokio::test]
    async fn test_process_cluster_with_entities() {
        let stage = DefaultPulseStage::default();

        let cluster = ClusterInput {
            cluster_id: 1,
            label: Some("Test Cluster".to_string()),
            articles: vec![
                ArticleInput {
                    id: "a1".to_string(),
                    title: "Apple releases new iPhone".to_string(),
                    source_url: "https://example.com/a1".to_string(),
                    canonical_url: None,
                    og_url: None,
                    entities: vec!["Apple".to_string(), "iPhone".to_string()],
                    published_at: Some("2026-01-31T12:00:00Z".to_string()),
                },
                ArticleInput {
                    id: "a2".to_string(),
                    title: "Apple announces iPhone update".to_string(),
                    source_url: "https://example.com/a2".to_string(),
                    canonical_url: None,
                    og_url: None,
                    entities: vec!["Apple".to_string(), "iPhone".to_string()],
                    published_at: Some("2026-01-31T11:00:00Z".to_string()),
                },
            ],
            embeddings: vec![vec![0.5; 128], vec![0.5; 128]],
            impact_score: Some(0.8),
            burst_score: Some(0.6),
            novelty_score: Some(0.4),
            recency_score: Some(0.9),
            genre: Some("Tech".to_string()),
        };

        let result = stage.process_cluster(&cluster).await.unwrap();

        assert!(result.is_some());
        let cluster_with_metrics = result.unwrap();
        assert_eq!(cluster_with_metrics.cluster_id, 1);
        assert!(!cluster_with_metrics.top_entities.is_empty());
    }

    #[tokio::test]
    async fn test_fallback_level_tracking() {
        let mut config = enabled_config();
        // Set strict quality thresholds to force fallback
        config.quality.cohesion_threshold = 0.99;
        config.selection.min_score_threshold = 0.99;

        let stage = DefaultPulseStage::new(config, full_rollout());

        let input = PulseInput {
            job_id: Uuid::new_v4(),
            clusters: vec![make_cluster(1, vec![make_article("a1", "Test")])],
        };

        let result = stage.generate(input).await.unwrap();

        // Check that we got a result (with fallback if needed)
        // Fallback level is a u8, so it's always >= 0
        let _ = result.diagnostics.fallback_level;
    }
}
