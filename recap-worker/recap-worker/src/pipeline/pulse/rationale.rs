//! Rationale generation for Evening Pulse topics.
//!
//! This module generates human-readable explanations for why each topic
//! was selected, based on the role and scoring data.
//!
//! ## Templates
//!
//! Each role has a specific template that emphasizes the relevant metrics and entities:
//!
//! - **NeedToKnow**: "{impact_level} impact news about \"{entity}\". Covered by {sources}."
//! - **Trend**: "\"{entity}\" is {burst_level}. Coverage increasing rapidly."
//! - **Serendipity**: "{novelty_level} perspective on \"{entity}\". A unique angle not seen elsewhere."

use super::types::{ClusterWithMetrics, PulseTopic, TopicRole};

/// Trait for rationale generation.
pub trait RationaleGenerator: Send + Sync {
    /// Generate a rationale for the given topic.
    fn generate(&self, topic: &PulseTopic, cluster: &ClusterWithMetrics) -> String;
}

/// Default implementation of rationale generation.
#[derive(Default)]
pub struct DefaultRationaleGenerator;

impl RationaleGenerator for DefaultRationaleGenerator {
    fn generate(&self, topic: &PulseTopic, cluster: &ClusterWithMetrics) -> String {
        generate_rationale(topic, cluster)
    }
}

/// Generate a rationale for a topic based on its role and metrics.
///
/// Uses English templates with entity information when available.
#[must_use]
pub fn generate_rationale(topic: &PulseTopic, cluster: &ClusterWithMetrics) -> String {
    match topic.role {
        TopicRole::NeedToKnow => format_need_to_know_rationale(topic, cluster),
        TopicRole::Trend => format_trend_rationale(cluster),
        TopicRole::Serendipity => format_serendipity_rationale(cluster),
    }
}

/// Format rationale for NeedToKnow role.
///
/// Includes entity information and source count.
fn format_need_to_know_rationale(topic: &PulseTopic, cluster: &ClusterWithMetrics) -> String {
    let impact_level = classify_impact_level(cluster.impact_score);
    let article_count = topic.articles.len();
    let source_text = if article_count == 1 {
        "1 source".to_string()
    } else {
        format!("{} sources", article_count)
    };

    if let Some(entity) = cluster.top_entities.first() {
        return format!(
            "{} impact news about \"{}\". Covered by {}.",
            impact_level, entity, source_text
        );
    }

    format!("{} impact news. Covered by {}.", impact_level, source_text)
}

/// Format rationale for Trend role.
///
/// Includes entity information and burst level.
fn format_trend_rationale(cluster: &ClusterWithMetrics) -> String {
    let burst_level = classify_burst_level(cluster.burst_score);

    if let Some(entity) = cluster.top_entities.first() {
        return format!(
            "\"{}\" is {}. Coverage increasing rapidly.",
            entity,
            burst_level.to_lowercase()
        );
    }

    format!("{} topic. Coverage increasing rapidly.", burst_level)
}

/// Format rationale for Serendipity role.
///
/// Includes entity information and novelty level.
fn format_serendipity_rationale(cluster: &ClusterWithMetrics) -> String {
    let novelty_level = classify_novelty_level(cluster.novelty_score);

    if let Some(entity) = cluster.top_entities.first() {
        return format!(
            "{} perspective on \"{}\". A unique angle not seen elsewhere.",
            novelty_level, entity
        );
    }

    format!(
        "{} perspective on this topic. A unique angle not seen elsewhere.",
        novelty_level
    )
}

/// Classify impact score into a human-readable level.
fn classify_impact_level(score: f32) -> &'static str {
    if score > 0.8 {
        "Very high"
    } else if score > 0.6 {
        "High"
    } else if score > 0.4 {
        "Moderate"
    } else {
        "Notable"
    }
}

/// Classify burst score into a human-readable level.
fn classify_burst_level(score: f32) -> &'static str {
    if score > 0.8 {
        "Rapidly trending"
    } else if score > 0.6 {
        "Rising"
    } else if score > 0.4 {
        "Emerging"
    } else {
        "Quietly notable"
    }
}

/// Classify novelty score into a human-readable level.
fn classify_novelty_level(score: f32) -> &'static str {
    if score > 0.8 {
        "Highly novel"
    } else if score > 0.6 {
        "Fresh"
    } else if score > 0.4 {
        "Unique"
    } else {
        "Different"
    }
}


#[cfg(test)]
mod tests {
    use super::super::types::{ClusterQualityMetrics, QualityTier, ScoreBreakdown};
    use super::*;

    fn make_topic(role: TopicRole, articles: Vec<String>) -> PulseTopic {
        PulseTopic {
            cluster_id: 1,
            role,
            score: 0.5,
            rationale: String::new(),
            articles,
            quality_metrics: ClusterQualityMetrics::default(),
            score_breakdown: ScoreBreakdown::default(),
            representative_articles: Vec::new(),
            top_entities: Vec::new(),
            source_names: Vec::new(),
        }
    }

    fn make_cluster(impact: f32, burst: f32, novelty: f32) -> ClusterWithMetrics {
        ClusterWithMetrics {
            cluster_id: 1,
            article_ids: vec!["a1".to_string()],
            label: Some("Test".to_string()),
            quality_metrics: ClusterQualityMetrics {
                cohesion: 0.5,
                ambiguity: 0.3,
                entity_consistency: 0.6,
                tier: QualityTier::Ok,
            },
            impact_score: impact,
            burst_score: burst,
            novelty_score: novelty,
            recency_score: 0.5,
            top_entities: vec![],
            syndication_status: None,
            representative_articles: Vec::new(),
            source_names: Vec::new(),
        }
    }

    fn make_cluster_with_entities(
        impact: f32,
        burst: f32,
        novelty: f32,
        entities: Vec<&str>,
    ) -> ClusterWithMetrics {
        ClusterWithMetrics {
            cluster_id: 1,
            article_ids: vec!["a1".to_string()],
            label: Some("Test".to_string()),
            quality_metrics: ClusterQualityMetrics {
                cohesion: 0.5,
                ambiguity: 0.3,
                entity_consistency: 0.6,
                tier: QualityTier::Ok,
            },
            impact_score: impact,
            burst_score: burst,
            novelty_score: novelty,
            recency_score: 0.5,
            top_entities: entities.into_iter().map(String::from).collect(),
            syndication_status: None,
            representative_articles: Vec::new(),
            source_names: Vec::new(),
        }
    }

    #[test]
    fn test_need_to_know_rationale_high_impact() {
        let topic = make_topic(
            TopicRole::NeedToKnow,
            vec!["a1".to_string(), "a2".to_string()],
        );
        let cluster = make_cluster(0.9, 0.3, 0.3);

        let rationale = generate_rationale(&topic, &cluster);

        assert!(rationale.contains("Very high"));
        assert!(rationale.contains("2 sources"));
    }

    #[test]
    fn test_need_to_know_rationale_with_entity() {
        let topic = make_topic(
            TopicRole::NeedToKnow,
            vec!["a1".to_string(), "a2".to_string()],
        );
        let cluster = make_cluster_with_entities(0.9, 0.3, 0.3, vec!["OpenAI", "GPT-5"]);

        let rationale = generate_rationale(&topic, &cluster);

        assert!(rationale.contains("Very high"));
        assert!(rationale.contains("\"OpenAI\""));
        assert!(rationale.contains("2 sources"));
    }

    #[test]
    fn test_need_to_know_rationale_single_source() {
        let topic = make_topic(TopicRole::NeedToKnow, vec!["a1".to_string()]);
        let cluster = make_cluster(0.5, 0.3, 0.3);

        let rationale = generate_rationale(&topic, &cluster);

        assert!(rationale.contains("1 source"));
    }

    #[test]
    fn test_trend_rationale_high_burst() {
        let topic = make_topic(TopicRole::Trend, vec!["a1".to_string()]);
        let cluster = make_cluster(0.3, 0.9, 0.3);

        let rationale = generate_rationale(&topic, &cluster);

        assert!(rationale.contains("Rapidly trending"));
        assert!(rationale.contains("Coverage increasing rapidly"));
    }

    #[test]
    fn test_trend_rationale_with_entity() {
        let topic = make_topic(TopicRole::Trend, vec!["a1".to_string()]);
        let cluster = make_cluster_with_entities(0.3, 0.9, 0.3, vec!["Bitcoin"]);

        let rationale = generate_rationale(&topic, &cluster);

        assert!(rationale.contains("\"Bitcoin\""));
        assert!(rationale.contains("rapidly trending"));
        assert!(rationale.contains("Coverage increasing rapidly"));
    }

    #[test]
    fn test_serendipity_rationale_high_novelty() {
        let topic = make_topic(TopicRole::Serendipity, vec!["a1".to_string()]);
        let cluster = make_cluster(0.3, 0.3, 0.9);

        let rationale = generate_rationale(&topic, &cluster);

        assert!(rationale.contains("Highly novel"));
        assert!(rationale.contains("unique angle"));
    }

    #[test]
    fn test_serendipity_rationale_with_entity() {
        let topic = make_topic(TopicRole::Serendipity, vec!["a1".to_string()]);
        let cluster = make_cluster_with_entities(0.3, 0.3, 0.9, vec!["Quantum Computing"]);

        let rationale = generate_rationale(&topic, &cluster);

        assert!(rationale.contains("Highly novel"));
        assert!(rationale.contains("\"Quantum Computing\""));
        assert!(rationale.contains("unique angle"));
    }

    #[test]
    fn test_impact_level_classification() {
        assert_eq!(classify_impact_level(0.9), "Very high");
        assert_eq!(classify_impact_level(0.7), "High");
        assert_eq!(classify_impact_level(0.5), "Moderate");
        assert_eq!(classify_impact_level(0.3), "Notable");
    }

    #[test]
    fn test_burst_level_classification() {
        assert_eq!(classify_burst_level(0.9), "Rapidly trending");
        assert_eq!(classify_burst_level(0.7), "Rising");
        assert_eq!(classify_burst_level(0.5), "Emerging");
        assert_eq!(classify_burst_level(0.3), "Quietly notable");
    }

    #[test]
    fn test_novelty_level_classification() {
        assert_eq!(classify_novelty_level(0.9), "Highly novel");
        assert_eq!(classify_novelty_level(0.7), "Fresh");
        assert_eq!(classify_novelty_level(0.5), "Unique");
        assert_eq!(classify_novelty_level(0.3), "Different");
    }

    #[test]
    fn test_default_generator() {
        let generator = DefaultRationaleGenerator;
        let topic = make_topic(TopicRole::NeedToKnow, vec!["a1".to_string()]);
        let cluster = make_cluster(0.7, 0.3, 0.3);

        let rationale = generator.generate(&topic, &cluster);

        assert!(!rationale.is_empty());
        assert!(rationale.contains("High"));
    }

    #[test]
    fn test_default_generator_with_entity() {
        let generator = DefaultRationaleGenerator;
        let topic = make_topic(TopicRole::NeedToKnow, vec!["a1".to_string()]);
        let cluster = make_cluster_with_entities(0.7, 0.3, 0.3, vec!["Tesla"]);

        let rationale = generator.generate(&topic, &cluster);

        assert!(rationale.contains("High"));
        assert!(rationale.contains("\"Tesla\""));
    }
}
