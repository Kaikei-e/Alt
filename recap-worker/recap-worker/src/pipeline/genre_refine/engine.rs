//! Refine engine implementation for genre refinement.

use std::collections::HashMap;
use std::sync::Arc;

use anyhow::Result;
use async_trait::async_trait;
use tokio::sync::RwLock;
use tracing::{error, instrument};

use crate::clients::news_creator::{
    GenreTieBreakCandidate, GenreTieBreakRequest, GenreTieBreakResponse, NewsCreatorClient,
    TagSignalPayload,
};
use crate::pipeline::dedup::DeduplicatedArticle;
use crate::pipeline::genre::GenreCandidate;
use crate::scheduler::JobContext;

use super::cache::{TagLabelGraphCache, TagLabelGraphSource};
use super::config::RefineConfig;
use super::scoring::{
    compute_graph_boosts, compute_weighted_tie_break_score, expand_candidates_from_tags,
    graph_boosts_to_owned, normalize, tag_consistency_score, tag_consistency_winner,
};
use super::strategy::{RefineOutcome, RefineStrategy};
use super::tag_profile::{TagFallbackMode, TagProfile};

/// Refineステージ入力。
#[derive(Debug)]
pub(crate) struct RefineInput<'a> {
    pub(crate) job: &'a JobContext,
    #[allow(dead_code)]
    pub(crate) article: &'a DeduplicatedArticle,
    pub(crate) candidates: &'a [GenreCandidate],
    pub(crate) tag_profile: &'a TagProfile,
    pub(crate) fallback: TagFallbackMode,
}

/// LLMタイブレーク用応答（後方互換性のため保持）。
#[allow(dead_code)]
#[derive(Debug, Clone, PartialEq)]
pub(crate) struct LlmDecision {
    pub(crate) genre: String,
    pub(crate) confidence: f32,
    pub(crate) trace_id: Option<String>,
}

#[allow(dead_code)]
impl LlmDecision {
    #[must_use]
    pub(crate) fn new(genre: impl Into<String>, confidence: f32, trace_id: Option<String>) -> Self {
        Self {
            genre: genre.into(),
            confidence,
            trace_id,
        }
    }
}

/// LLM呼び出し用インタフェース（後方互換性のため保持）。
#[allow(dead_code)]
#[async_trait]
pub(crate) trait LlmTieBreaker: Send + Sync {
    async fn tie_break(
        &self,
        job: &JobContext,
        article: &DeduplicatedArticle,
        candidates: &[GenreCandidate],
        tag_profile: &TagProfile,
    ) -> Result<LlmDecision>;
}

/// HTTP経由でNews Creator LLMを呼び出す実装（後方互換性のため保持）。
#[allow(dead_code)]
#[derive(Debug, Clone)]
pub(crate) struct NewsCreatorLlmTieBreaker {
    client: Arc<NewsCreatorClient>,
}

#[allow(dead_code)]
impl NewsCreatorLlmTieBreaker {
    pub(crate) fn new(client: Arc<NewsCreatorClient>) -> Self {
        Self { client }
    }
}

#[allow(dead_code)]
#[async_trait]
impl LlmTieBreaker for NewsCreatorLlmTieBreaker {
    async fn tie_break(
        &self,
        job: &JobContext,
        article: &DeduplicatedArticle,
        candidates: &[GenreCandidate],
        tag_profile: &TagProfile,
    ) -> Result<LlmDecision> {
        let request = GenreTieBreakRequest {
            job_id: job.job_id,
            article_id: article.id.clone(),
            language: article.language.clone(),
            body_preview: build_body_preview(article),
            candidates: candidates
                .iter()
                .map(|candidate| GenreTieBreakCandidate {
                    name: candidate.name.clone(),
                    score: candidate.score,
                    keyword_support: candidate.keyword_support,
                    classifier_confidence: candidate.classifier_confidence,
                })
                .collect(),
            tags: tag_profile
                .top_tags
                .iter()
                .map(|tag| TagSignalPayload {
                    label: tag.label.clone(),
                    confidence: tag.confidence,
                    source: tag.source.clone(),
                })
                .collect(),
        };

        let response: GenreTieBreakResponse = self.client.tie_break_genre(&request).await?;

        Ok(LlmDecision::new(
            response.genre,
            response.confidence,
            response.trace_id,
        ))
    }
}

#[allow(dead_code)]
fn build_body_preview(article: &DeduplicatedArticle) -> Option<String> {
    if article.sentences.is_empty() {
        return None;
    }
    let mut preview = article
        .sentences
        .iter()
        .take(3)
        .map(|s| s.trim())
        .filter(|s| !s.is_empty())
        .collect::<Vec<_>>()
        .join("\n");
    if preview.len() > 1500 {
        preview.truncate(1500);
    }
    if preview.is_empty() {
        None
    } else {
        Some(preview)
    }
}

/// Refineステージのインタフェース。
#[async_trait]
pub(crate) trait RefineEngine: Send + Sync {
    async fn refine(&self, input: RefineInput<'_>) -> Result<RefineOutcome>;

    /// 設定を更新する（デフォルト実装は何もしない）。
    #[allow(dead_code)]
    async fn update_config(&self, _new_config: RefineConfig) {
        // デフォルト実装は何もしない（既存の実装を壊さないため）
    }
}

/// デフォルト実装。
pub(crate) struct DefaultRefineEngine {
    config: RwLock<RefineConfig>,
    graph: Arc<dyn TagLabelGraphSource>,
}

impl DefaultRefineEngine {
    pub(crate) fn new(config: RefineConfig, graph: Arc<dyn TagLabelGraphSource>) -> Self {
        Self {
            config: RwLock::new(config),
            graph,
        }
    }
}

#[async_trait]
impl RefineEngine for DefaultRefineEngine {
    async fn update_config(&self, new_config: RefineConfig) {
        let mut config = self.config.write().await;
        tracing::debug!(
            old_graph_margin = config.graph_margin,
            new_graph_margin = new_config.graph_margin,
            old_boost_threshold = config.boost_threshold,
            new_boost_threshold = new_config.boost_threshold,
            old_tag_count_threshold = config.tag_count_threshold,
            new_tag_count_threshold = new_config.tag_count_threshold,
            "updating refine config"
        );
        *config = new_config;
    }

    #[instrument(skip_all, fields(job_id = %input.job.job_id))]
    async fn refine(&self, input: RefineInput<'_>) -> Result<RefineOutcome> {
        let config = self.config.read().await;

        if matches!(input.fallback, TagFallbackMode::CoarseOnly) {
            return Ok(RefineOutcome::new(
                input
                    .candidates
                    .first()
                    .map_or_else(|| config.fallback_genre.clone(), |c| c.name.clone()),
                input
                    .candidates
                    .first()
                    .map_or(0.0, |c| c.classifier_confidence),
                RefineStrategy::CoarseOnly,
                None,
                HashMap::new(),
            ));
        }

        if config.require_tags && !input.tag_profile.has_tags() {
            tracing::info!(
                article_id = %input.article.id,
                "falling back to coarse strategy due to missing tags (require_tags=true)"
            );
            return Ok(RefineOutcome::new(
                input
                    .candidates
                    .first()
                    .map_or_else(|| config.fallback_genre.clone(), |c| c.name.clone()),
                input
                    .candidates
                    .first()
                    .map_or(0.0, |c| c.classifier_confidence),
                RefineStrategy::CoarseOnly,
                None,
                HashMap::new(),
            ));
        }

        let graph_cache = match self.graph.snapshot().await {
            Ok(cache) => cache,
            Err(err) => {
                error!(error = ?err, "failed to refresh tag label graph, proceeding without boosts");
                TagLabelGraphCache::empty()
            }
        };

        // タグから候補ジャンルを拡大
        let expanded_candidates = expand_candidates_from_tags(
            &graph_cache,
            input.candidates,
            input.tag_profile.top_tags.as_slice(),
        );

        if expanded_candidates.len() > input.candidates.len() {
            let added_count = expanded_candidates.len() - input.candidates.len();
            let added_genres: Vec<String> = expanded_candidates
                .iter()
                .skip(input.candidates.len())
                .map(|c| c.name.clone())
                .collect();
            tracing::debug!(
                article_id = %input.article.id,
                original_candidates = input.candidates.len(),
                expanded_candidates = expanded_candidates.len(),
                added_count = added_count,
                added_genres = ?added_genres,
                "expanded candidates from tags"
            );
        }

        if expanded_candidates.is_empty() {
            return Ok(RefineOutcome::new(
                config.fallback_genre.clone(),
                0.0,
                RefineStrategy::FallbackOther,
                None,
                HashMap::new(),
            ));
        }

        let normalized_candidates: Vec<(String, &GenreCandidate)> = expanded_candidates
            .iter()
            .map(|candidate| (normalize(&candidate.name), candidate))
            .collect();

        let consistent_candidate =
            tag_consistency_winner(&config, &normalized_candidates, &input.tag_profile.top_tags);
        if let Some((winner_name, confidence)) = consistent_candidate {
            let outcome_conf = confidence.max(
                expanded_candidates
                    .iter()
                    .find(|c| c.name.eq_ignore_ascii_case(&winner_name))
                    .map_or(0.0, |c| c.classifier_confidence),
            );
            return Ok(RefineOutcome::new(
                winner_name,
                outcome_conf.clamp(0.0, 1.0),
                RefineStrategy::TagConsistency,
                None,
                HashMap::new(),
            ));
        }

        let graph_boosts = compute_graph_boosts(
            &graph_cache,
            &normalized_candidates,
            &input.tag_profile.top_tags,
        );

        // Log graph boost effectiveness
        let active_boost_count = graph_boosts.values().filter(|&&v| v > 0.0).count();
        let total_boost_sum: f32 = graph_boosts.values().sum();
        if active_boost_count == 0 {
            // WARN -> INFO/DEBUG if valid tags exist but no matches
            if !input.tag_profile.top_tags.is_empty() {
                tracing::debug!(
                    total_tags = input.tag_profile.top_tags.len(),
                    "no graph boost matches found for tags - graph may be incomplete or tags are new"
                );
            }
        } else {
            tracing::debug!(
                active_boost_count = active_boost_count,
                total_boost_sum = total_boost_sum,
                "graph boosts active"
            );
        }

        let mut scored: Vec<(&GenreCandidate, f32)> = expanded_candidates
            .iter()
            .map(|candidate| {
                let boost = graph_boosts
                    .get(&normalize(&candidate.name))
                    .copied()
                    .unwrap_or_default();
                (candidate, candidate.score + boost)
            })
            .collect();

        scored.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal));

        let top = scored[0];
        let top_boost = graph_boosts
            .get(&normalize(&top.0.name))
            .copied()
            .unwrap_or_default();
        let tag_count = input.tag_profile.top_tags.len();

        if let Some(second) = scored.get(1) {
            let margin = top.1 - second.1;
            if margin >= config.graph_margin
                && top_boost >= config.boost_threshold
                && tag_count >= config.tag_count_threshold
            {
                return Ok(RefineOutcome::new(
                    top.0.name.clone(),
                    (top.0.classifier_confidence + top_boost).clamp(0.0, 1.0),
                    RefineStrategy::GraphBoost,
                    None,
                    graph_boosts_to_owned(&graph_boosts),
                ));
            }

            // マージンが小さい場合、重み付きスコアリングでタイブレーク
            if margin.abs() < config.weighted_tie_break_margin {
                let mut weighted_scores: Vec<(&GenreCandidate, f32)> = input
                    .candidates
                    .iter()
                    .map(|candidate| {
                        let normalized = normalize(&candidate.name);
                        let boost = graph_boosts.get(&normalized).copied().unwrap_or_default();
                        let tag_consistency = tag_consistency_score(
                            &config,
                            &candidate.name,
                            &input.tag_profile.top_tags,
                        );
                        let weighted_score = compute_weighted_tie_break_score(
                            &config,
                            candidate,
                            boost,
                            tag_consistency,
                        );
                        (candidate, weighted_score)
                    })
                    .collect();

                weighted_scores
                    .sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal));

                if let Some((winner, score)) = weighted_scores.first() {
                    return Ok(RefineOutcome::new(
                        winner.name.clone(),
                        score.clamp(0.0, 1.0),
                        RefineStrategy::WeightedScore,
                        None,
                        graph_boosts_to_owned(&graph_boosts),
                    ));
                }
            }
        }

        // Determine actual strategy: GraphBoost only if boost is active
        let actual_strategy = if top_boost > 0.0 {
            RefineStrategy::GraphBoost
        } else {
            RefineStrategy::CoarseOnly
        };
        Ok(RefineOutcome::new(
            top.0.name.clone(),
            top.0.classifier_confidence.clamp(0.0, 1.0),
            actual_strategy,
            None,
            graph_boosts_to_owned(&graph_boosts),
        ))
    }
}
