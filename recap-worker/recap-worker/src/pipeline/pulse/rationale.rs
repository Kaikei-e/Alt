//! Rationale generation for Evening Pulse topics.
//!
//! This module generates human-readable explanations for why each topic
//! was selected, based on the role and scoring data.
//!
//! ## Templates
//!
//! Each role has a specific template that emphasizes the relevant metrics:
//!
//! - **NeedToKnow**: "{impact_level}影響度を持つ重要ニュース。{article_count}件のソースが報道。"
//! - **Trend**: "{burst_level}のトピック。直近の報道量が増加傾向。"
//! - **Serendipity**: "{novelty_level}視点からのトピック。他では見られない切り口。"

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
#[must_use]
pub fn generate_rationale(topic: &PulseTopic, cluster: &ClusterWithMetrics) -> String {
    match topic.role {
        TopicRole::NeedToKnow => format_need_to_know_rationale(topic, cluster),
        TopicRole::Trend => format_trend_rationale(topic, cluster),
        TopicRole::Serendipity => format_serendipity_rationale(topic, cluster),
    }
}

/// Format rationale for NeedToKnow role.
fn format_need_to_know_rationale(topic: &PulseTopic, cluster: &ClusterWithMetrics) -> String {
    let impact_level = classify_impact_level(cluster.impact_score);
    let article_count = topic.articles.len();

    let source_text = if article_count == 1 {
        "1件のソースが報道".to_string()
    } else {
        format!("{article_count}件のソースが報道")
    };

    format!("{impact_level}影響度を持つ重要ニュース。{source_text}。")
}

/// Format rationale for Trend role.
fn format_trend_rationale(_topic: &PulseTopic, cluster: &ClusterWithMetrics) -> String {
    let burst_level = classify_burst_level(cluster.burst_score);
    format!("{burst_level}のトピック。直近の報道量が増加傾向。")
}

/// Format rationale for Serendipity role.
fn format_serendipity_rationale(_topic: &PulseTopic, cluster: &ClusterWithMetrics) -> String {
    let novelty_level = classify_novelty_level(cluster.novelty_score);
    format!("{novelty_level}視点からのトピック。他では見られない切り口。")
}

/// Classify impact score into a human-readable level.
fn classify_impact_level(score: f32) -> &'static str {
    if score > 0.8 {
        "非常に高い"
    } else if score > 0.6 {
        "高い"
    } else if score > 0.4 {
        "中程度の"
    } else {
        "一定の"
    }
}

/// Classify burst score into a human-readable level.
fn classify_burst_level(score: f32) -> &'static str {
    if score > 0.8 {
        "急上昇中"
    } else if score > 0.6 {
        "注目度上昇中"
    } else if score > 0.4 {
        "話題になりつつある"
    } else {
        "静かに注目されている"
    }
}

/// Classify novelty score into a human-readable level.
fn classify_novelty_level(score: f32) -> &'static str {
    if score > 0.8 {
        "斬新な"
    } else if score > 0.6 {
        "新規性の高い"
    } else if score > 0.4 {
        "ユニークな"
    } else {
        "異なる"
    }
}

/// Generate English rationale (for potential future use).
#[must_use]
pub fn generate_rationale_en(topic: &PulseTopic, cluster: &ClusterWithMetrics) -> String {
    match topic.role {
        TopicRole::NeedToKnow => format_need_to_know_rationale_en(topic, cluster),
        TopicRole::Trend => format_trend_rationale_en(cluster),
        TopicRole::Serendipity => format_serendipity_rationale_en(cluster),
    }
}

fn format_need_to_know_rationale_en(topic: &PulseTopic, cluster: &ClusterWithMetrics) -> String {
    let impact_level = classify_impact_level_en(cluster.impact_score);
    let article_count = topic.articles.len();
    format!(
        "{} impact news. Reported by {} source{}.",
        impact_level,
        article_count,
        if article_count == 1 { "" } else { "s" }
    )
}

fn format_trend_rationale_en(cluster: &ClusterWithMetrics) -> String {
    let burst_level = classify_burst_level_en(cluster.burst_score);
    format!("{} topic. Increasing coverage in recent hours.", burst_level)
}

fn format_serendipity_rationale_en(cluster: &ClusterWithMetrics) -> String {
    let novelty_level = classify_novelty_level_en(cluster.novelty_score);
    format!(
        "{} perspective on this topic. A unique angle not seen elsewhere.",
        novelty_level
    )
}

fn classify_impact_level_en(score: f32) -> &'static str {
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

fn classify_burst_level_en(score: f32) -> &'static str {
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

fn classify_novelty_level_en(score: f32) -> &'static str {
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
        }
    }

    #[test]
    fn test_need_to_know_rationale_high_impact() {
        let topic = make_topic(TopicRole::NeedToKnow, vec!["a1".to_string(), "a2".to_string()]);
        let cluster = make_cluster(0.9, 0.3, 0.3);

        let rationale = generate_rationale(&topic, &cluster);

        assert!(rationale.contains("非常に高い"));
        assert!(rationale.contains("2件のソースが報道"));
    }

    #[test]
    fn test_need_to_know_rationale_single_source() {
        let topic = make_topic(TopicRole::NeedToKnow, vec!["a1".to_string()]);
        let cluster = make_cluster(0.5, 0.3, 0.3);

        let rationale = generate_rationale(&topic, &cluster);

        assert!(rationale.contains("1件のソースが報道"));
    }

    #[test]
    fn test_trend_rationale_high_burst() {
        let topic = make_topic(TopicRole::Trend, vec!["a1".to_string()]);
        let cluster = make_cluster(0.3, 0.9, 0.3);

        let rationale = generate_rationale(&topic, &cluster);

        assert!(rationale.contains("急上昇中"));
        assert!(rationale.contains("増加傾向"));
    }

    #[test]
    fn test_serendipity_rationale_high_novelty() {
        let topic = make_topic(TopicRole::Serendipity, vec!["a1".to_string()]);
        let cluster = make_cluster(0.3, 0.3, 0.9);

        let rationale = generate_rationale(&topic, &cluster);

        assert!(rationale.contains("斬新な"));
        assert!(rationale.contains("他では見られない"));
    }

    #[test]
    fn test_impact_level_classification() {
        assert_eq!(classify_impact_level(0.9), "非常に高い");
        assert_eq!(classify_impact_level(0.7), "高い");
        assert_eq!(classify_impact_level(0.5), "中程度の");
        assert_eq!(classify_impact_level(0.3), "一定の");
    }

    #[test]
    fn test_burst_level_classification() {
        assert_eq!(classify_burst_level(0.9), "急上昇中");
        assert_eq!(classify_burst_level(0.7), "注目度上昇中");
        assert_eq!(classify_burst_level(0.5), "話題になりつつある");
        assert_eq!(classify_burst_level(0.3), "静かに注目されている");
    }

    #[test]
    fn test_novelty_level_classification() {
        assert_eq!(classify_novelty_level(0.9), "斬新な");
        assert_eq!(classify_novelty_level(0.7), "新規性の高い");
        assert_eq!(classify_novelty_level(0.5), "ユニークな");
        assert_eq!(classify_novelty_level(0.3), "異なる");
    }

    #[test]
    fn test_english_rationale_need_to_know() {
        let topic = make_topic(TopicRole::NeedToKnow, vec!["a1".to_string(), "a2".to_string()]);
        let cluster = make_cluster(0.9, 0.3, 0.3);

        let rationale = generate_rationale_en(&topic, &cluster);

        assert!(rationale.contains("Very high"));
        assert!(rationale.contains("2 sources"));
    }

    #[test]
    fn test_english_rationale_trend() {
        let topic = make_topic(TopicRole::Trend, vec!["a1".to_string()]);
        let cluster = make_cluster(0.3, 0.9, 0.3);

        let rationale = generate_rationale_en(&topic, &cluster);

        assert!(rationale.contains("Rapidly trending"));
    }

    #[test]
    fn test_english_rationale_serendipity() {
        let topic = make_topic(TopicRole::Serendipity, vec!["a1".to_string()]);
        let cluster = make_cluster(0.3, 0.3, 0.9);

        let rationale = generate_rationale_en(&topic, &cluster);

        assert!(rationale.contains("Highly novel"));
    }

    #[test]
    fn test_default_generator() {
        let generator = DefaultRationaleGenerator;
        let topic = make_topic(TopicRole::NeedToKnow, vec!["a1".to_string()]);
        let cluster = make_cluster(0.7, 0.3, 0.3);

        let rationale = generator.generate(&topic, &cluster);

        assert!(!rationale.is_empty());
        assert!(rationale.contains("高い"));
    }
}
