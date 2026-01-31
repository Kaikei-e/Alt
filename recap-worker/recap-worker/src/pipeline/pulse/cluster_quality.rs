//! Cluster quality evaluation for Evening Pulse.
//!
//! This module provides quality evaluation for clusters using three metrics:
//! - **Cohesion**: Title similarity (Jaccard)
//! - **Ambiguity**: Embedding dissimilarity ratio
//! - **Entity Consistency**: Most frequent entity occurrence rate
//!
//! These metrics are combined to produce a three-tier quality diagnosis (Ok, Caution, Ng).

use std::collections::{HashMap, HashSet};

use async_trait::async_trait;

use super::config::PulseQualityConfig;
use super::types::{ClusterQualityMetrics, QualityTier};
use crate::pipeline::embedding::cosine_similarity;

/// Article with entities for quality evaluation.
#[derive(Debug, Clone)]
pub struct ArticleEntities {
    /// Article identifier.
    pub id: String,
    /// Article title.
    pub title: Option<String>,
    /// Extracted entities (normalized).
    pub entities: Vec<String>,
}

/// Trait for evaluating cluster quality.
#[async_trait]
pub trait ClusterQualityEvaluator: Send + Sync {
    /// Evaluate the quality of a cluster.
    ///
    /// # Arguments
    ///
    /// * `titles` - Article titles in the cluster.
    /// * `embeddings` - Optional article embeddings for ambiguity calculation.
    /// * `articles` - Articles with extracted entities.
    async fn evaluate(
        &self,
        titles: &[String],
        embeddings: Option<&[Vec<f32>]>,
        articles: &[ArticleEntities],
    ) -> ClusterQualityMetrics;
}

/// Default implementation of cluster quality evaluation.
pub struct DefaultClusterQualityEvaluator {
    config: PulseQualityConfig,
}

impl DefaultClusterQualityEvaluator {
    /// Create a new evaluator with the given configuration.
    #[must_use]
    pub fn new(config: PulseQualityConfig) -> Self {
        Self { config }
    }
}

impl Default for DefaultClusterQualityEvaluator {
    fn default() -> Self {
        Self::new(PulseQualityConfig::default())
    }
}

#[async_trait]
impl ClusterQualityEvaluator for DefaultClusterQualityEvaluator {
    async fn evaluate(
        &self,
        titles: &[String],
        embeddings: Option<&[Vec<f32>]>,
        articles: &[ArticleEntities],
    ) -> ClusterQualityMetrics {
        let cohesion = compute_cohesion(titles);
        let ambiguity = embeddings
            .map_or(0.0, |e| compute_ambiguity(e, self.config.embedding_similarity_threshold));
        let entity_consistency = compute_entity_consistency(articles);

        let tier = diagnose_quality(cohesion, ambiguity, entity_consistency, &self.config);

        ClusterQualityMetrics {
            cohesion,
            ambiguity,
            entity_consistency,
            tier,
        }
    }
}

/// Compute cohesion score using Jaccard similarity of titles.
///
/// Cohesion measures how similar article titles are within a cluster.
/// Higher cohesion indicates a more focused topic.
///
/// # Arguments
///
/// * `titles` - Article titles in the cluster.
///
/// # Returns
///
/// Average Jaccard similarity between all pairs of titles (0.0 - 1.0).
#[must_use]
pub fn compute_cohesion(titles: &[String]) -> f32 {
    if titles.len() < 2 {
        return 1.0;
    }

    let mut total_similarity = 0.0;
    let mut pair_count = 0;

    for i in 0..titles.len() {
        for j in (i + 1)..titles.len() {
            let sim = jaccard_similarity(&titles[i], &titles[j]);
            total_similarity += sim;
            pair_count += 1;
        }
    }

    if pair_count == 0 {
        1.0
    } else {
        total_similarity / pair_count as f32
    }
}

/// Compute ambiguity score using embedding similarity.
///
/// Ambiguity measures the ratio of article pairs that have low semantic similarity.
/// Higher ambiguity indicates a less coherent cluster.
///
/// # Arguments
///
/// * `embeddings` - Article embeddings.
/// * `threshold` - Similarity threshold below which pairs are considered dissimilar.
///
/// # Returns
///
/// Ratio of low-similarity pairs to total pairs (0.0 - 1.0).
#[must_use]
pub fn compute_ambiguity(embeddings: &[Vec<f32>], threshold: f32) -> f32 {
    if embeddings.len() < 2 {
        return 0.0;
    }

    let mut low_sim_pairs = 0;
    let mut total_pairs = 0;

    for i in 0..embeddings.len() {
        for j in (i + 1)..embeddings.len() {
            let sim = cosine_similarity(&embeddings[i], &embeddings[j]);
            if sim < threshold {
                low_sim_pairs += 1;
            }
            total_pairs += 1;
        }
    }

    if total_pairs == 0 {
        0.0
    } else {
        low_sim_pairs as f32 / total_pairs as f32
    }
}

/// Compute entity consistency score.
///
/// Entity consistency measures the ratio of articles that contain the most
/// frequent entity. Higher consistency indicates a more focused topic.
///
/// # Arguments
///
/// * `articles` - Articles with extracted entities.
///
/// # Returns
///
/// Ratio of articles containing the most frequent entity (0.0 - 1.0).
#[must_use]
pub fn compute_entity_consistency(articles: &[ArticleEntities]) -> f32 {
    if articles.is_empty() {
        return 0.0;
    }

    // Count entity occurrences across articles (not total mentions)
    let mut entity_article_count: HashMap<String, usize> = HashMap::new();

    for article in articles {
        // Use a set to count each entity only once per article
        let unique_entities: HashSet<_> = article.entities.iter().collect();
        for entity in unique_entities {
            *entity_article_count.entry(entity.clone()).or_insert(0) += 1;
        }
    }

    let max_count = entity_article_count.values().max().copied().unwrap_or(0);

    max_count as f32 / articles.len() as f32
}

/// Get the most frequent entities from articles.
///
/// # Arguments
///
/// * `articles` - Articles with extracted entities.
/// * `limit` - Maximum number of entities to return.
///
/// # Returns
///
/// List of top entities sorted by frequency.
#[must_use]
pub fn get_top_entities(articles: &[ArticleEntities], limit: usize) -> Vec<String> {
    let mut entity_counts: HashMap<String, usize> = HashMap::new();

    for article in articles {
        for entity in &article.entities {
            *entity_counts.entry(entity.clone()).or_insert(0) += 1;
        }
    }

    let mut entities: Vec<(String, usize)> = entity_counts.into_iter().collect();
    entities.sort_by(|a, b| b.1.cmp(&a.1));

    entities.into_iter().take(limit).map(|(e, _)| e).collect()
}

/// Diagnose quality tier based on metrics and configuration.
///
/// Three-tier diagnosis:
/// - **Ok**: Passes all thresholds
/// - **Caution**: Fails one threshold (borderline quality)
/// - **Ng**: Fails two or more thresholds (low quality)
///
/// # Arguments
///
/// * `cohesion` - Cohesion score (0.0 - 1.0).
/// * `ambiguity` - Ambiguity score (0.0 - 1.0).
/// * `entity_consistency` - Entity consistency score (0.0 - 1.0).
/// * `config` - Quality configuration with thresholds.
#[must_use]
pub fn diagnose_quality(
    cohesion: f32,
    ambiguity: f32,
    entity_consistency: f32,
    config: &PulseQualityConfig,
) -> QualityTier {
    let mut issues = 0;

    // Cohesion should be ABOVE threshold (higher is better)
    if cohesion < config.cohesion_threshold {
        issues += 1;
    }

    // Ambiguity should be BELOW threshold (lower is better)
    if ambiguity > config.ambiguity_threshold {
        issues += 1;
    }

    // Entity consistency should be ABOVE threshold (higher is better)
    if entity_consistency < config.entity_consistency_threshold {
        issues += 1;
    }

    match issues {
        0 => QualityTier::Ok,
        1 => QualityTier::Caution,
        _ => QualityTier::Ng,
    }
}

/// Compute Jaccard similarity between two strings.
///
/// Uses word-level tokenization for comparison.
#[must_use]
pub fn jaccard_similarity(a: &str, b: &str) -> f32 {
    let words_a: HashSet<_> = tokenize(a).collect();
    let words_b: HashSet<_> = tokenize(b).collect();

    if words_a.is_empty() && words_b.is_empty() {
        return 1.0;
    }

    if words_a.is_empty() || words_b.is_empty() {
        return 0.0;
    }

    let intersection = words_a.intersection(&words_b).count();
    let union = words_a.union(&words_b).count();

    if union == 0 {
        0.0
    } else {
        intersection as f32 / union as f32
    }
}

/// Simple tokenizer that splits on whitespace and punctuation.
fn tokenize(text: &str) -> impl Iterator<Item = &str> {
    text.split(|c: char| c.is_whitespace() || c.is_ascii_punctuation())
        .filter(|s| !s.is_empty())
        .map(str::trim)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_jaccard_similarity_identical() {
        let sim = jaccard_similarity("hello world", "hello world");
        assert!((sim - 1.0).abs() < f32::EPSILON);
    }

    #[test]
    fn test_jaccard_similarity_different() {
        let sim = jaccard_similarity("hello world", "goodbye moon");
        assert!((sim - 0.0).abs() < f32::EPSILON);
    }

    #[test]
    fn test_jaccard_similarity_partial() {
        let sim = jaccard_similarity("hello world", "hello moon");
        // "hello" is common, so similarity is 1/3
        assert!((sim - 1.0 / 3.0).abs() < 0.01);
    }

    #[test]
    fn test_jaccard_similarity_empty() {
        assert!((jaccard_similarity("", "") - 1.0).abs() < f32::EPSILON);
        assert!((jaccard_similarity("hello", "") - 0.0).abs() < f32::EPSILON);
        assert!((jaccard_similarity("", "world") - 0.0).abs() < f32::EPSILON);
    }

    #[test]
    fn test_compute_cohesion_single_title() {
        let titles = vec!["Single title".to_string()];
        let cohesion = compute_cohesion(&titles);
        assert!((cohesion - 1.0).abs() < f32::EPSILON);
    }

    #[test]
    fn test_compute_cohesion_identical_titles() {
        let titles = vec![
            "Same title".to_string(),
            "Same title".to_string(),
            "Same title".to_string(),
        ];
        let cohesion = compute_cohesion(&titles);
        assert!((cohesion - 1.0).abs() < f32::EPSILON);
    }

    #[test]
    fn test_compute_cohesion_different_titles() {
        let titles = vec![
            "Apple releases new iPhone".to_string(),
            "Google announces Android update".to_string(),
            "Microsoft launches Windows feature".to_string(),
        ];
        let cohesion = compute_cohesion(&titles);
        // Should have low cohesion due to different content
        assert!(cohesion < 0.5);
    }

    #[test]
    fn test_compute_cohesion_similar_titles() {
        let titles = vec![
            "Apple releases new iPhone 15".to_string(),
            "Apple announces iPhone 15 launch".to_string(),
            "New iPhone 15 released by Apple".to_string(),
        ];
        let cohesion = compute_cohesion(&titles);
        // Should have higher cohesion due to similar content
        assert!(cohesion > 0.2);
    }

    #[test]
    fn test_compute_ambiguity_empty() {
        let embeddings: Vec<Vec<f32>> = vec![];
        let ambiguity = compute_ambiguity(&embeddings, 0.5);
        assert!((ambiguity - 0.0).abs() < f32::EPSILON);
    }

    #[test]
    fn test_compute_ambiguity_single() {
        let embeddings = vec![vec![1.0, 0.0, 0.0]];
        let ambiguity = compute_ambiguity(&embeddings, 0.5);
        assert!((ambiguity - 0.0).abs() < f32::EPSILON);
    }

    #[test]
    fn test_compute_ambiguity_identical() {
        let embeddings = vec![
            vec![1.0, 0.0, 0.0],
            vec![1.0, 0.0, 0.0],
            vec![1.0, 0.0, 0.0],
        ];
        let ambiguity = compute_ambiguity(&embeddings, 0.5);
        // All pairs have similarity 1.0, so ambiguity should be 0.0
        assert!((ambiguity - 0.0).abs() < f32::EPSILON);
    }

    #[test]
    fn test_compute_ambiguity_orthogonal() {
        let embeddings = vec![
            vec![1.0, 0.0, 0.0],
            vec![0.0, 1.0, 0.0],
            vec![0.0, 0.0, 1.0],
        ];
        let ambiguity = compute_ambiguity(&embeddings, 0.5);
        // All pairs are orthogonal (similarity 0.0), so ambiguity should be 1.0
        assert!((ambiguity - 1.0).abs() < f32::EPSILON);
    }

    #[test]
    fn test_compute_entity_consistency_empty() {
        let articles: Vec<ArticleEntities> = vec![];
        let consistency = compute_entity_consistency(&articles);
        assert!((consistency - 0.0).abs() < f32::EPSILON);
    }

    #[test]
    fn test_compute_entity_consistency_all_same() {
        let articles = vec![
            ArticleEntities {
                id: "1".to_string(),
                title: None,
                entities: vec!["Apple".to_string()],
            },
            ArticleEntities {
                id: "2".to_string(),
                title: None,
                entities: vec!["Apple".to_string()],
            },
            ArticleEntities {
                id: "3".to_string(),
                title: None,
                entities: vec!["Apple".to_string()],
            },
        ];
        let consistency = compute_entity_consistency(&articles);
        assert!((consistency - 1.0).abs() < f32::EPSILON);
    }

    #[test]
    fn test_compute_entity_consistency_all_different() {
        let articles = vec![
            ArticleEntities {
                id: "1".to_string(),
                title: None,
                entities: vec!["Apple".to_string()],
            },
            ArticleEntities {
                id: "2".to_string(),
                title: None,
                entities: vec!["Google".to_string()],
            },
            ArticleEntities {
                id: "3".to_string(),
                title: None,
                entities: vec!["Microsoft".to_string()],
            },
        ];
        let consistency = compute_entity_consistency(&articles);
        // Each entity appears in 1/3 of articles
        assert!((consistency - 1.0 / 3.0).abs() < f32::EPSILON);
    }

    #[test]
    fn test_compute_entity_consistency_partial() {
        let articles = vec![
            ArticleEntities {
                id: "1".to_string(),
                title: None,
                entities: vec!["Apple".to_string(), "iPhone".to_string()],
            },
            ArticleEntities {
                id: "2".to_string(),
                title: None,
                entities: vec!["Apple".to_string()],
            },
            ArticleEntities {
                id: "3".to_string(),
                title: None,
                entities: vec!["Google".to_string()],
            },
        ];
        let consistency = compute_entity_consistency(&articles);
        // Apple appears in 2/3 of articles
        assert!((consistency - 2.0 / 3.0).abs() < f32::EPSILON);
    }

    #[test]
    fn test_get_top_entities() {
        let articles = vec![
            ArticleEntities {
                id: "1".to_string(),
                title: None,
                entities: vec![
                    "Apple".to_string(),
                    "iPhone".to_string(),
                    "Tim Cook".to_string(),
                ],
            },
            ArticleEntities {
                id: "2".to_string(),
                title: None,
                entities: vec!["Apple".to_string(), "iPhone".to_string()],
            },
            ArticleEntities {
                id: "3".to_string(),
                title: None,
                entities: vec!["Apple".to_string()],
            },
        ];

        let top = get_top_entities(&articles, 2);
        assert_eq!(top.len(), 2);
        assert_eq!(top[0], "Apple");
        assert_eq!(top[1], "iPhone");
    }

    #[test]
    fn test_diagnose_quality_ok() {
        let config = PulseQualityConfig {
            cohesion_threshold: 0.3,
            ambiguity_threshold: 0.5,
            entity_consistency_threshold: 0.4,
            embedding_similarity_threshold: 0.5,
        };

        let tier = diagnose_quality(0.8, 0.2, 0.9, &config);
        assert_eq!(tier, QualityTier::Ok);
    }

    #[test]
    fn test_diagnose_quality_caution() {
        let config = PulseQualityConfig {
            cohesion_threshold: 0.3,
            ambiguity_threshold: 0.5,
            entity_consistency_threshold: 0.4,
            embedding_similarity_threshold: 0.5,
        };

        // Fails cohesion only
        let tier = diagnose_quality(0.2, 0.2, 0.9, &config);
        assert_eq!(tier, QualityTier::Caution);

        // Fails ambiguity only
        let tier = diagnose_quality(0.8, 0.6, 0.9, &config);
        assert_eq!(tier, QualityTier::Caution);

        // Fails entity consistency only
        let tier = diagnose_quality(0.8, 0.2, 0.3, &config);
        assert_eq!(tier, QualityTier::Caution);
    }

    #[test]
    fn test_diagnose_quality_ng() {
        let config = PulseQualityConfig {
            cohesion_threshold: 0.3,
            ambiguity_threshold: 0.5,
            entity_consistency_threshold: 0.4,
            embedding_similarity_threshold: 0.5,
        };

        // Fails all three
        let tier = diagnose_quality(0.1, 0.9, 0.1, &config);
        assert_eq!(tier, QualityTier::Ng);

        // Fails two
        let tier = diagnose_quality(0.1, 0.9, 0.9, &config);
        assert_eq!(tier, QualityTier::Ng);
    }

    #[test]
    fn test_diagnose_quality_boundary_values() {
        let config = PulseQualityConfig {
            cohesion_threshold: 0.3,
            ambiguity_threshold: 0.5,
            entity_consistency_threshold: 0.4,
            embedding_similarity_threshold: 0.5,
        };

        // Exactly at thresholds (should fail as < and > are used)
        let tier = diagnose_quality(0.3, 0.5, 0.4, &config);
        // cohesion: 0.3 < 0.3 = false, ambiguity: 0.5 > 0.5 = false, entity: 0.4 < 0.4 = false
        assert_eq!(tier, QualityTier::Ok);

        // Just below/above thresholds (should fail all)
        let tier = diagnose_quality(0.29, 0.51, 0.39, &config);
        assert_eq!(tier, QualityTier::Ng);
    }

    #[tokio::test]
    async fn test_default_evaluator() {
        let evaluator = DefaultClusterQualityEvaluator::default();

        let titles = vec![
            "Apple releases iPhone 15".to_string(),
            "Apple announces iPhone 15 launch".to_string(),
        ];

        let articles = vec![
            ArticleEntities {
                id: "1".to_string(),
                title: Some("Apple releases iPhone 15".to_string()),
                entities: vec!["Apple".to_string(), "iPhone 15".to_string()],
            },
            ArticleEntities {
                id: "2".to_string(),
                title: Some("Apple announces iPhone 15 launch".to_string()),
                entities: vec!["Apple".to_string(), "iPhone 15".to_string()],
            },
        ];

        let metrics = evaluator.evaluate(&titles, None, &articles).await;

        // Should have good cohesion due to similar titles
        assert!(metrics.cohesion > 0.2);
        // No embeddings provided, ambiguity should be 0.0
        assert!((metrics.ambiguity - 0.0).abs() < f32::EPSILON);
        // Perfect entity consistency
        assert!((metrics.entity_consistency - 1.0).abs() < f32::EPSILON);
    }
}
