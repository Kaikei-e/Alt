use std::collections::HashMap;
use std::sync::Arc;
use std::time::{Duration, Instant};

use anyhow::{Context, Result};
use async_trait::async_trait;
use rustc_hash::FxHashMap;
use serde::{Deserialize, Serialize};
use tokio::sync::RwLock;
use tracing::{error, instrument};

use crate::clients::news_creator::{
    GenreTieBreakCandidate, GenreTieBreakRequest, GenreTieBreakResponse, NewsCreatorClient,
    TagSignalPayload,
};
use crate::pipeline::dedup::DeduplicatedArticle;
use crate::pipeline::genre::GenreCandidate;
use crate::pipeline::tag_signal::TagSignal;
use crate::scheduler::JobContext;
use crate::store::dao::RecapDao;
use crate::store::models::GraphEdgeRecord;

/// タグとジャンルの共起重み。
#[cfg_attr(not(test), allow(dead_code))]
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub(crate) struct LabelEdge {
    pub(crate) genre: String,
    pub(crate) tag: String,
    pub(crate) weight: f32,
}

impl LabelEdge {
    #[must_use]
    #[cfg_attr(not(test), allow(dead_code))]
    pub(crate) fn new(genre: impl Into<String>, tag: impl Into<String>, weight: f32) -> Self {
        Self {
            genre: genre.into(),
            tag: tag.into(),
            weight,
        }
    }
}

/// タグプロファイル（Tag Generator出力の要約）。
#[derive(Debug, Clone, PartialEq, Default, Serialize, Deserialize)]
pub(crate) struct TagProfile {
    pub(crate) top_tags: Vec<TagSignal>,
    pub(crate) entropy: f32,
}

impl TagProfile {
    #[must_use]
    pub(crate) fn from_signals(signals: &[TagSignal]) -> Self {
        let entropy = compute_entropy(signals);
        Self {
            top_tags: signals.to_vec(),
            entropy,
        }
    }

    #[must_use]
    pub(crate) fn has_tags(&self) -> bool {
        !self.top_tags.is_empty()
    }
}

/// タグ有無に応じたフォールバックモード。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum TagFallbackMode {
    CoarseOnly,
    AllowRefine,
}

impl TagFallbackMode {
    #[must_use]
    pub(crate) fn require_tags(require: bool, has_tags: bool) -> Self {
        if require && !has_tags {
            Self::CoarseOnly
        } else {
            Self::AllowRefine
        }
    }
}

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

/// Refine戦略。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum RefineStrategy {
    TagConsistency,
    GraphBoost,
    WeightedScore,
    #[allow(dead_code)]
    LlmTieBreak, // 後方互換性のため保持
    FallbackOther,
    CoarseOnly,
}

/// Refine結果。
#[derive(Debug, Clone, PartialEq)]
pub(crate) struct RefineOutcome {
    pub(crate) final_genre: String,
    pub(crate) confidence: f32,
    pub(crate) strategy: RefineStrategy,
    pub(crate) llm_trace_id: Option<String>,
    pub(crate) graph_boosts: HashMap<String, f32>,
}

impl RefineOutcome {
    #[must_use]
    pub(crate) fn new(
        final_genre: impl Into<String>,
        confidence: f32,
        strategy: RefineStrategy,
        llm_trace_id: Option<String>,
        graph_boosts: HashMap<String, f32>,
    ) -> Self {
        Self {
            final_genre: final_genre.into(),
            confidence,
            strategy,
            llm_trace_id,
            graph_boosts,
        }
    }

    #[must_use]
    pub(crate) fn graph_boosts(&self) -> &HashMap<String, f32> {
        &self.graph_boosts
    }
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

/// タグラベル共起キャッシュ。
#[derive(Debug, Clone)]
pub(crate) struct TagLabelGraphCache {
    edges: Arc<FxHashMap<String, FxHashMap<String, f32>>>,
}

impl TagLabelGraphCache {
    #[must_use]
    pub(crate) fn empty() -> Self {
        Self {
            edges: Arc::new(FxHashMap::default()),
        }
    }

    #[must_use]
    #[cfg_attr(not(test), allow(dead_code))]
    pub(crate) fn from_edges(edges: &[LabelEdge]) -> Self {
        let mut map: FxHashMap<String, FxHashMap<String, f32>> = FxHashMap::default();
        for edge in edges {
            let genre_key = edge.genre.to_lowercase();
            let tag_key = edge.tag.to_lowercase();
            map.entry(genre_key)
                .or_insert_with(FxHashMap::default)
                .insert(tag_key, edge.weight);
        }
        Self {
            edges: Arc::new(map),
        }
    }

    #[must_use]
    pub(crate) fn from_records(records: &[GraphEdgeRecord]) -> Self {
        let mut map: FxHashMap<String, FxHashMap<String, f32>> = FxHashMap::default();
        for record in records {
            let genre_key = record.genre.to_lowercase();
            let tag_key = record.tag.to_lowercase();
            map.entry(genre_key)
                .or_insert_with(FxHashMap::default)
                .insert(tag_key, record.weight);
        }
        Self {
            edges: Arc::new(map),
        }
    }

    #[must_use]
    pub(crate) fn weight(&self, genre: &str, tag: &str) -> Option<f32> {
        self.edges
            .get(&genre.to_lowercase())
            .and_then(|tags| tags.get(&tag.to_lowercase()).copied())
    }
}

#[async_trait]
pub(crate) trait TagLabelGraphSource: Send + Sync {
    async fn snapshot(&self) -> Result<TagLabelGraphCache>;
}

pub(crate) struct DbTagLabelGraphSource {
    dao: Arc<RecapDao>,
    window_label: String,
    ttl: Duration,
    state: RwLock<TagLabelGraphState>,
}

struct TagLabelGraphState {
    cache: TagLabelGraphCache,
    loaded_at: Option<Instant>,
}

impl TagLabelGraphState {
    fn is_fresh(&self, ttl: Duration) -> bool {
        self.loaded_at
            .map(|instant| instant.elapsed() < ttl)
            .unwrap_or(false)
    }
}

impl DbTagLabelGraphSource {
    pub(crate) fn new(dao: Arc<RecapDao>, window_label: impl Into<String>, ttl: Duration) -> Self {
        Self {
            dao,
            window_label: window_label.into(),
            ttl,
            state: RwLock::new(TagLabelGraphState {
                cache: TagLabelGraphCache::empty(),
                loaded_at: None,
            }),
        }
    }

    pub(crate) async fn preload(&self) -> Result<()> {
        self.refresh().await
    }

    async fn refresh(&self) -> Result<()> {
        let records = self
            .dao
            .load_tag_label_graph(&self.window_label)
            .await
            .with_context(|| {
                format!(
                    "failed to load tag_label_graph window {}",
                    self.window_label
                )
            })?;
        let cache = TagLabelGraphCache::from_records(&records);
        let mut guard = self.state.write().await;
        guard.cache = cache;
        guard.loaded_at = Some(Instant::now());
        Ok(())
    }
}

#[async_trait]
impl TagLabelGraphSource for DbTagLabelGraphSource {
    async fn snapshot(&self) -> Result<TagLabelGraphCache> {
        {
            let guard = self.state.read().await;
            if guard.is_fresh(self.ttl) {
                return Ok(guard.cache.clone());
            }
        }

        if let Err(err) = self.refresh().await {
            {
                let guard = self.state.read().await;
                if guard.loaded_at.is_some() {
                    error!(
                        window = %self.window_label,
                        error = ?err,
                        "serving stale tag label graph after refresh failure"
                    );
                    return Ok(guard.cache.clone());
                }
            }
            return Err(err);
        }

        let guard = self.state.read().await;
        Ok(guard.cache.clone())
    }
}

#[cfg(test)]
pub(crate) struct StaticTagLabelGraphSource {
    cache: TagLabelGraphCache,
}

#[cfg(test)]
impl StaticTagLabelGraphSource {
    pub(crate) fn new(cache: TagLabelGraphCache) -> Self {
        Self { cache }
    }
}

#[cfg(test)]
#[async_trait]
impl TagLabelGraphSource for StaticTagLabelGraphSource {
    async fn snapshot(&self) -> Result<TagLabelGraphCache> {
        Ok(self.cache.clone())
    }
}

/// Refine用設定値。
#[derive(Debug, Clone)]
pub(crate) struct RefineConfig {
    pub(crate) require_tags: bool,
    pub(crate) tag_confidence_gate: f32,
    pub(crate) graph_margin: f32,
    pub(crate) weighted_tie_break_margin: f32,
    pub(crate) fallback_genre: String,
}

impl RefineConfig {
    #[must_use]
    pub(crate) fn new(require_tags: bool) -> Self {
        Self {
            require_tags,
            tag_confidence_gate: 0.6,
            graph_margin: 0.15,
            weighted_tie_break_margin: 0.05,
            fallback_genre: "other".to_string(),
        }
    }
}

/// Refineステージのインタフェース。
#[async_trait]
pub(crate) trait RefineEngine: Send + Sync {
    async fn refine(&self, input: RefineInput<'_>) -> Result<RefineOutcome>;
}

/// デフォルト実装。
pub(crate) struct DefaultRefineEngine {
    config: RefineConfig,
    graph: Arc<dyn TagLabelGraphSource>,
}

impl DefaultRefineEngine {
    pub(crate) fn new(
        config: RefineConfig,
        graph: Arc<dyn TagLabelGraphSource>,
    ) -> Self {
        Self { config, graph }
    }
}

#[async_trait]
impl RefineEngine for DefaultRefineEngine {
    #[instrument(skip_all, fields(job_id = %input.job.job_id))]
    async fn refine(&self, input: RefineInput<'_>) -> Result<RefineOutcome> {
        if matches!(input.fallback, TagFallbackMode::CoarseOnly) {
            return Ok(RefineOutcome::new(
                input
                    .candidates
                    .first()
                    .map(|c| c.name.clone())
                    .unwrap_or_else(|| self.config.fallback_genre.clone()),
                input
                    .candidates
                    .first()
                    .map(|c| c.classifier_confidence)
                    .unwrap_or(0.0),
                RefineStrategy::CoarseOnly,
                None,
                HashMap::new(),
            ));
        }

        if self.config.require_tags && !input.tag_profile.has_tags() {
            return Ok(RefineOutcome::new(
                input
                    .candidates
                    .first()
                    .map(|c| c.name.clone())
                    .unwrap_or_else(|| self.config.fallback_genre.clone()),
                input
                    .candidates
                    .first()
                    .map(|c| c.classifier_confidence)
                    .unwrap_or(0.0),
                RefineStrategy::CoarseOnly,
                None,
                HashMap::new(),
            ));
        }

        if input.candidates.is_empty() {
            return Ok(RefineOutcome::new(
                self.config.fallback_genre.clone(),
                0.0,
                RefineStrategy::FallbackOther,
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

        let normalized_candidates: Vec<(String, &GenreCandidate)> = input
            .candidates
            .iter()
            .map(|candidate| (normalize(&candidate.name), candidate))
            .collect();

        let consistent_candidate = tag_consistency_winner(
            &self.config,
            &normalized_candidates,
            &input.tag_profile.top_tags,
        );
        if let Some((winner_name, confidence)) = consistent_candidate {
            let outcome_conf = confidence.max(
                input
                    .candidates
                    .iter()
                    .find(|c| c.name.eq_ignore_ascii_case(&winner_name))
                    .map(|c| c.classifier_confidence)
                    .unwrap_or(0.0),
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

        let mut scored: Vec<(&GenreCandidate, f32)> = input
            .candidates
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

        if let Some(second) = scored.get(1) {
            let margin = top.1 - second.1;
            if margin >= self.config.graph_margin && top_boost > 0.0 {
                return Ok(RefineOutcome::new(
                    top.0.name.clone(),
                    (top.0.classifier_confidence + top_boost).clamp(0.0, 1.0),
                    RefineStrategy::GraphBoost,
                    None,
                    graph_boosts_to_owned(&graph_boosts),
                ));
            }

            // マージンが小さい場合、重み付きスコアリングでタイブレーク
            if margin.abs() < self.config.weighted_tie_break_margin {
                let mut weighted_scores: Vec<(&GenreCandidate, f32)> = input
                    .candidates
                    .iter()
                    .map(|candidate| {
                        let normalized = normalize(&candidate.name);
                        let boost = graph_boosts
                            .get(&normalized)
                            .copied()
                            .unwrap_or_default();
                        let tag_consistency = tag_consistency_score(
                            &self.config,
                            &candidate.name,
                            &input.tag_profile.top_tags,
                        );
                        let weighted_score = compute_weighted_tie_break_score(
                            &self.config,
                            candidate,
                            boost,
                            tag_consistency,
                        );
                        (candidate, weighted_score)
                    })
                    .collect();

                weighted_scores.sort_by(|a, b| {
                    b.1.partial_cmp(&a.1)
                        .unwrap_or(std::cmp::Ordering::Equal)
                });

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

        Ok(RefineOutcome::new(
            top.0.name.clone(),
            top.0.classifier_confidence.clamp(0.0, 1.0),
            RefineStrategy::GraphBoost,
            None,
            graph_boosts_to_owned(&graph_boosts),
        ))
    }
}

fn graph_boosts_to_owned(boosts: &FxHashMap<String, f32>) -> HashMap<String, f32> {
    boosts
        .iter()
        .map(|(k, v)| (k.clone(), *v))
        .collect::<HashMap<_, _>>()
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

fn compute_entropy(tags: &[TagSignal]) -> f32 {
    if tags.is_empty() {
        return 0.0;
    }
    let total: f32 = tags.iter().map(|t| t.confidence.max(0.0)).sum();
    if total <= f32::EPSILON {
        return 0.0;
    }
    let entropy = tags
        .iter()
        .filter_map(|tag| {
            let p = (tag.confidence.max(0.0)) / total;
            if p <= 0.0 {
                None
            } else {
                Some(-p * (p.ln() / std::f32::consts::LN_2))
            }
        })
        .sum::<f32>();
    entropy
}

fn compute_graph_boosts(
    graph: &TagLabelGraphCache,
    candidates: &[(String, &GenreCandidate)],
    tags: &[TagSignal],
) -> FxHashMap<String, f32> {
    let mut boosts: FxHashMap<String, f32> = FxHashMap::default();
    for (normalized, candidate) in candidates {
        let boost = tags
            .iter()
            .map(|tag| {
                let tag_norm = normalize(&tag.label);
                let weight = graph.weight(&candidate.name, &tag_norm).unwrap_or(0.0);
                weight * tag.confidence
            })
            .sum::<f32>();
        boosts.insert(normalized.clone(), boost);
    }
    boosts
}

fn normalize(value: &str) -> String {
    value.trim().to_lowercase()
}

fn tag_consistency_winner(
    config: &RefineConfig,
    candidates: &[(String, &GenreCandidate)],
    tags: &[TagSignal],
) -> Option<(String, f32)> {
    let mut matched: Vec<(String, f32)> = Vec::new();
    for tag in tags {
        if tag.confidence < config.tag_confidence_gate {
            continue;
        }
        let normalized_tag = normalize(&tag.label);
        if let Some((candidate_name, _)) = candidates
            .iter()
            .find(|(normalized, _)| normalized == &normalized_tag)
        {
            matched.push((candidate_name.clone(), tag.confidence));
        }
    }

    if matched.is_empty() {
        return None;
    }

    let unique: FxHashMap<String, f32> =
        matched
            .into_iter()
            .fold(FxHashMap::default(), |mut acc, (name, conf)| {
                let entry = acc.entry(name).or_insert(0.0f32);
                *entry = entry.max(conf);
                acc
            });

    if unique.len() == 1 {
        let (name, confidence) = unique.into_iter().next().unwrap();
        return Some((name, confidence));
    }

    None
}

/// タグ一貫性スコアを計算する（強化版）。
/// タグの信頼度、出現頻度、ジャンル名との部分一致も考慮する。
fn tag_consistency_score(
    config: &RefineConfig,
    candidate_name: &str,
    tags: &[TagSignal],
) -> f32 {
    let normalized_candidate = normalize(candidate_name);
    let mut score = 0.0;
    let mut match_count = 0;

    for tag in tags {
        if tag.confidence < config.tag_confidence_gate {
            continue;
        }
        let normalized_tag = normalize(&tag.label);

        // 完全一致
        if normalized_tag == normalized_candidate {
            score += tag.confidence;
            match_count += 1;
        }
        // 部分一致（タグがジャンル名を含む、またはジャンル名がタグを含む）
        else if normalized_tag.contains(&normalized_candidate)
            || normalized_candidate.contains(&normalized_tag)
        {
            score += tag.confidence * 0.5;
            match_count += 1;
        }
    }

    // 複数のタグが一致する場合、信頼度の合計を返す（最大1.0にクランプ）
    if match_count > 0 {
        score.min(1.0)
    } else {
        0.0
    }
}

/// 重み付きスコアリングでタイブレークを決定する。
/// 各候補に対して以下の要素を重み付きで評価：
/// - keyword_support (重み: 0.25)
/// - classifier_confidence (重み: 0.30)
/// - graph_boost (重み: 0.25)
/// - tag_consistency_score (重み: 0.20)
fn compute_weighted_tie_break_score(
    _config: &RefineConfig,
    candidate: &GenreCandidate,
    graph_boost: f32,
    tag_consistency: f32,
) -> f32 {
    // 各要素を正規化
    let keyword_score = (candidate.keyword_support as f32 / 10.0).min(1.0); // 最大10で正規化
    let classifier_score = candidate.classifier_confidence.clamp(0.0, 1.0);
    let graph_score = graph_boost.clamp(0.0, 1.0);
    let tag_score = tag_consistency.clamp(0.0, 1.0);

    // 重み付き合計
    keyword_score * 0.25
        + classifier_score * 0.30
        + graph_score * 0.25
        + tag_score * 0.20
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::pipeline::dedup::DeduplicatedArticle;
    use crate::scheduler::JobContext;
    use anyhow::anyhow;
    use tokio::sync::Mutex;
    use uuid::Uuid;

    fn static_graph(cache: TagLabelGraphCache) -> Arc<dyn TagLabelGraphSource> {
        Arc::new(StaticTagLabelGraphSource::new(cache))
    }

    /// テスト用フェイクLLM。
    #[derive(Debug, Default)]
    struct FakeLlm {
        responses: Mutex<Vec<Result<LlmDecision>>>,
    }

    impl FakeLlm {
        fn new(responses: Vec<Result<LlmDecision>>) -> Self {
            Self {
                responses: Mutex::new(responses),
            }
        }
    }

    #[async_trait]
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
            tags,
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
        let engine =
            DefaultRefineEngine::new(RefineConfig::new(true), graph);

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
        let engine = DefaultRefineEngine::new(
            RefineConfig::new(false),
            static_graph(graph),
        );

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
        let engine =
            DefaultRefineEngine::new(RefineConfig::new(true), graph);

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
}
