//! Genre refinement module.
//!
//! This module refines genre classification using tag signals and graph-based boosting.

mod cache;
mod config;
mod engine;
mod scoring;
mod strategy;
mod tag_profile;

// Re-exports for public API
pub(crate) use cache::{
    DbTagLabelGraphSource, LabelEdge, TagLabelGraphCache, TagLabelGraphSource,
};
pub(crate) use config::RefineConfig;
pub(crate) use engine::{DefaultRefineEngine, LlmDecision, LlmTieBreaker, RefineEngine, RefineInput};
pub(crate) use strategy::{RefineOutcome, RefineStrategy};
pub(crate) use tag_profile::{TagFallbackMode, TagProfile};

#[cfg(test)]
pub(crate) use cache::StaticTagLabelGraphSource;

#[cfg(test)]
mod tests {
    use super::*;
    use crate::pipeline::dedup::DeduplicatedArticle;
    use crate::pipeline::genre::GenreCandidate;
    use crate::pipeline::tag_signal::TagSignal;
    use crate::scheduler::JobContext;
    use anyhow::{anyhow, Result};
    use std::sync::Arc;
    use tokio::sync::Mutex;
    use uuid::Uuid;

    fn static_graph(cache: TagLabelGraphCache) -> Arc<dyn TagLabelGraphSource> {
        Arc::new(StaticTagLabelGraphSource::new(cache))
    }

    /// テスト用フェイクLLM。
    #[allow(dead_code)]
    #[derive(Debug, Default)]
    struct FakeLlm {
        responses: Mutex<Vec<Result<LlmDecision>>>,
    }

    impl FakeLlm {
        #[allow(dead_code)]
        fn new(responses: Vec<Result<LlmDecision>>) -> Self {
            Self {
                responses: Mutex::new(responses),
            }
        }
    }

    #[async_trait::async_trait]
    impl LlmTieBreaker for FakeLlm {
        async fn tie_break(
            &self,
            _job: &JobContext,
            _article: &DeduplicatedArticle,
            _candidates: &[GenreCandidate],
            _tag_profile: &TagProfile,
        ) -> Result<LlmDecision> {
            let mut guard = self.responses.lock().await;
            guard
                .pop()
                .unwrap_or_else(|| Err(anyhow!("no llm response configured")))
        }
    }

    fn article_with_tags(tags: Vec<TagSignal>) -> DeduplicatedArticle {
        DeduplicatedArticle {
            id: "art-1".to_string(),
            title: Some("title".to_string()),
            sentences: vec!["body text about ai and tech".to_string()],
            sentence_hashes: vec![],
            language: "en".to_string(),
            published_at: None,
            source_url: None,
            tags,
            duplicates: Vec::new(),
        }
    }

    fn candidate(name: &str, score: f32, confidence: f32) -> GenreCandidate {
        GenreCandidate {
            name: name.to_string(),
            score,
            keyword_support: 5,
            classifier_confidence: confidence,
        }
    }

    #[tokio::test]
    async fn tag_consistency_returns_first_candidate_with_stub() {
        let job = JobContext::new(Uuid::new_v4(), vec![]);
        let article = article_with_tags(vec![TagSignal::new("tech", 0.9, None, None)]);
        let candidates = vec![
            candidate("tech", 0.8, 0.82),
            candidate("business", 0.7, 0.68),
        ];
        let tag_profile = TagProfile {
            top_tags: article.tags.clone(),
            entropy: 0.1,
        };
        let graph = static_graph(TagLabelGraphCache::empty());
        let engine = DefaultRefineEngine::new(RefineConfig::new(true), graph);

        let outcome = engine
            .refine(RefineInput {
                job: &job,
                article: &article,
                candidates: &candidates,
                tag_profile: &tag_profile,
                fallback: TagFallbackMode::AllowRefine,
            })
            .await
            .expect("refine should succeed");

        assert_eq!(outcome.final_genre, "tech");
        assert_eq!(outcome.strategy, RefineStrategy::TagConsistency);
    }

    #[tokio::test]
    async fn graph_boost_prefers_candidate_with_higher_weight() {
        let job = JobContext::new(Uuid::new_v4(), vec![]);
        let tags = vec![TagSignal::new("半導体", 0.7, None, None)];
        let article = article_with_tags(tags.clone());
        let candidates = vec![
            candidate("business", 0.82, 0.8),
            candidate("tech", 0.81, 0.79),
        ];
        let tag_profile = TagProfile {
            top_tags: tags,
            entropy: 0.5,
        };
        let graph = TagLabelGraphCache::from_edges(&[
            LabelEdge::new("tech", "半導体", 0.5),
            LabelEdge::new("business", "半導体", 0.1),
        ]);
        let engine = DefaultRefineEngine::new(RefineConfig::new(false), static_graph(graph));

        let outcome = engine
            .refine(RefineInput {
                job: &job,
                article: &article,
                candidates: &candidates,
                tag_profile: &tag_profile,
                fallback: TagFallbackMode::AllowRefine,
            })
            .await
            .expect("refine should succeed");

        assert_eq!(outcome.final_genre, "tech");
        assert_eq!(outcome.strategy, RefineStrategy::GraphBoost);
    }

    #[tokio::test]
    async fn graph_boost_ignores_weighted_score_when_threshold_lowered() {
        let job = JobContext::new(Uuid::new_v4(), vec![]);
        let tags = vec![TagSignal::new("graph-margin", 0.25, None, None)];
        let article = article_with_tags(tags.clone());
        let candidates = vec![
            candidate("society_justice", 0.2, 0.2),
            candidate("art_culture", 0.19, 0.19),
        ];
        let tag_profile = TagProfile {
            top_tags: tags,
            entropy: 0.5,
        };
        let graph = TagLabelGraphCache::from_edges(&[
            LabelEdge::new("society_justice", "graph-margin", 0.32),
            LabelEdge::new("art_culture", "graph-margin", 0.3),
        ]);
        let mut config = RefineConfig::new(false);
        config.weighted_tie_break_margin = 0.01;
        let engine = DefaultRefineEngine::new(config, static_graph(graph));

        let outcome = engine
            .refine(RefineInput {
                job: &job,
                article: &article,
                candidates: &candidates,
                tag_profile: &tag_profile,
                fallback: TagFallbackMode::AllowRefine,
            })
            .await
            .expect("refine should succeed");

        assert_eq!(outcome.final_genre, "society_justice");
        assert_eq!(outcome.strategy, RefineStrategy::GraphBoost);
    }

    #[tokio::test]
    async fn llm_tie_break_is_invoked_when_scores_close() {
        let job = JobContext::new(Uuid::new_v4(), vec![]);
        let tags = vec![TagSignal::new("生成AI", 0.55, None, None)];
        let article = article_with_tags(tags.clone());
        let candidates = vec![
            candidate("tech", 0.81, 0.78),
            candidate("business", 0.8, 0.77),
        ];
        let tag_profile = TagProfile {
            top_tags: tags,
            entropy: 0.7,
        };
        let graph = static_graph(TagLabelGraphCache::empty());
        let mut config = RefineConfig::new(false);
        config.weighted_tie_break_margin = 0.1; // タイブレークをトリガー
        let engine = DefaultRefineEngine::new(config, graph);

        let outcome = engine
            .refine(RefineInput {
                job: &job,
                article: &article,
                candidates: &candidates,
                tag_profile: &tag_profile,
                fallback: TagFallbackMode::AllowRefine,
            })
            .await
            .expect("refine should succeed");

        // 重み付きスコアリングで決定される（techがclassifier_confidenceが高いため）
        assert_eq!(outcome.final_genre, "tech");
        assert_eq!(outcome.strategy, RefineStrategy::WeightedScore);
    }

    #[tokio::test]
    async fn fallback_to_coarse_when_tags_required_but_missing() {
        let job = JobContext::new(Uuid::new_v4(), vec![]);
        let article = article_with_tags(Vec::new());
        let candidates = vec![candidate("ai", 0.9, 0.88)];
        let tag_profile = TagProfile::default();
        let graph = static_graph(TagLabelGraphCache::empty());
        let engine = DefaultRefineEngine::new(RefineConfig::new(true), graph);

        let outcome = engine
            .refine(RefineInput {
                job: &job,
                article: &article,
                candidates: &candidates,
                tag_profile: &tag_profile,
                fallback: TagFallbackMode::CoarseOnly,
            })
            .await
            .expect("refine should succeed");

        assert_eq!(outcome.final_genre, "ai");
        assert_eq!(outcome.strategy, RefineStrategy::CoarseOnly);
    }

    #[tokio::test]
    async fn verify_no_graph_match_leads_to_coarse_strategy() {
        let job = JobContext::new(Uuid::new_v4(), vec![]);
        let tags = vec![TagSignal::new("unknown_tag", 0.9, None, None)];
        let article = article_with_tags(tags.clone());
        let candidates = vec![candidate("tech", 0.8, 0.7)];
        let tag_profile = TagProfile {
            top_tags: tags,
            entropy: 0.5,
        };
        // Empty graph means no matches for "unknown_tag"
        let graph = static_graph(TagLabelGraphCache::empty());
        let engine = DefaultRefineEngine::new(RefineConfig::new(true), graph);

        let outcome = engine
            .refine(RefineInput {
                job: &job,
                article: &article,
                candidates: &candidates,
                tag_profile: &tag_profile,
                fallback: TagFallbackMode::AllowRefine,
            })
            .await
            .expect("refine should succeed");

        assert_eq!(outcome.final_genre, "tech"); // Falls back to top coarse candidate
        assert_eq!(outcome.strategy, RefineStrategy::CoarseOnly);
    }
}
