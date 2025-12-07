use std::{collections::HashSet, sync::Arc};

use anyhow::{Result, bail};
use async_trait::async_trait;
use uuid::Uuid;

use crate::evaluation::metrics::{ClassificationMetrics, MetricsCalculator};
use crate::pipeline::dedup::DeduplicatedArticle;
use crate::pipeline::genre::GenreCandidate;
use crate::pipeline::genre_refine::{
    DefaultRefineEngine, LabelEdge, LlmDecision, LlmTieBreaker, RefineConfig, RefineEngine,
    RefineInput, TagFallbackMode, TagLabelGraphCache, TagLabelGraphSource, TagProfile,
};
use crate::pipeline::tag_signal::TagSignal;
use crate::scheduler::JobContext;

/// Evaluation candidate metadata exposed to integration tests.
#[derive(Clone, Debug)]
pub struct GenreEvaluationCandidate {
    pub genre: String,
    pub score: f32,
    pub keyword_support: usize,
    pub classifier_confidence: f32,
}

/// Minimal tag signal definition for evaluation samples.
#[derive(Clone, Debug)]
pub struct GenreEvaluationTag {
    pub label: String,
    pub confidence: f32,
}

/// Graph edge metadata used for offline evaluation.
#[derive(Clone, Debug)]
pub struct GenreEvaluationGraphEdge {
    pub genre: String,
    pub tag: String,
    pub weight: f32,
}

/// Single evaluation sample describing gold genre, coarse candidates, and tags.
#[derive(Clone, Debug)]
pub struct GenreEvaluationSample {
    pub job_id: Uuid,
    pub article_id: String,
    pub expected_genre: String,
    pub candidates: Vec<GenreEvaluationCandidate>,
    pub tags: Vec<GenreEvaluationTag>,
    pub graph_edges: Vec<GenreEvaluationGraphEdge>,
    pub sentences: Vec<String>,
    pub language: String,
}

impl GenreEvaluationSample {
    #[must_use]
    pub fn with_defaults(
        job_id: Uuid,
        article_id: impl Into<String>,
        expected_genre: impl Into<String>,
        candidates: Vec<GenreEvaluationCandidate>,
    ) -> Self {
        Self {
            job_id,
            article_id: article_id.into(),
            expected_genre: expected_genre.into(),
            candidates,
            tags: Vec::new(),
            graph_edges: Vec::new(),
            sentences: vec![],
            language: "en".to_string(),
        }
    }
}

/// Configuration overrides for evaluation suite.
#[derive(Clone, Debug, Default)]
pub struct EvaluationSettings {
    pub require_tags: bool,
    pub graph_margin: Option<f32>,
    pub tag_confidence_gate: Option<f32>,
    pub llm_tie_break_margin: Option<f32>,
    pub llm_min_confidence: Option<f32>,
}

/// Aggregated metrics produced by the offline evaluation.
pub struct GenreEvaluationReport {
    pub coarse: ClassificationMetrics,
    pub tag: ClassificationMetrics,
    pub two_stage: ClassificationMetrics,
}

struct StaticEvaluationGraph {
    cache: TagLabelGraphCache,
}

impl StaticEvaluationGraph {
    fn new(cache: TagLabelGraphCache) -> Self {
        Self { cache }
    }
}

#[async_trait]
impl TagLabelGraphSource for StaticEvaluationGraph {
    async fn snapshot(&self) -> Result<TagLabelGraphCache> {
        Ok(self.cache.clone())
    }
}

/// Lightweight LLM stub that deterministically picks the highest-scoring candidate.
#[allow(dead_code)]
#[derive(Debug, Default)]
struct GreedyEvaluationLlm;

#[async_trait]
impl LlmTieBreaker for GreedyEvaluationLlm {
    async fn tie_break(
        &self,
        _job: &JobContext,
        _article: &DeduplicatedArticle,
        candidates: &[GenreCandidate],
        _tag_profile: &TagProfile,
    ) -> Result<LlmDecision> {
        let winner = candidates
            .iter()
            .max_by(|a, b| {
                a.score
                    .partial_cmp(&b.score)
                    .unwrap_or(std::cmp::Ordering::Equal)
            })
            .cloned()
            .unwrap_or_else(|| GenreCandidate {
                name: "other".to_string(),
                score: 0.0,
                keyword_support: 0,
                classifier_confidence: 0.0,
            });
        Ok(LlmDecision::new(
            winner.name,
            winner.score.clamp(0.0, 1.0),
            None,
        ))
    }
}

fn to_internal_candidate(candidate: &GenreEvaluationCandidate) -> GenreCandidate {
    GenreCandidate {
        name: candidate.genre.clone(),
        score: candidate.score,
        keyword_support: candidate.keyword_support,
        classifier_confidence: candidate.classifier_confidence,
    }
}

fn to_internal_tag(tag: &GenreEvaluationTag) -> TagSignal {
    TagSignal::new(&tag.label, tag.confidence, None::<String>, None)
}

fn collect_graph_edges(samples: &[GenreEvaluationSample]) -> Vec<LabelEdge> {
    samples
        .iter()
        .flat_map(|sample| sample.graph_edges.iter())
        .map(|edge| LabelEdge::new(edge.genre.clone(), edge.tag.clone(), edge.weight))
        .collect()
}

fn compute_tag_prediction(
    candidates: &[(String, GenreCandidate)],
    tags: &[TagSignal],
    gate: f32,
) -> Option<String> {
    let mut matches: Vec<(String, f32)> = Vec::new();
    for tag in tags {
        if tag.confidence < gate {
            continue;
        }
        let tag_norm = tag.label.trim().to_lowercase();
        if let Some((candidate_name, _)) = candidates.iter().find(|(norm, _)| *norm == tag_norm) {
            matches.push((candidate_name.clone(), tag.confidence));
        }
    }

    if matches.is_empty() {
        return None;
    }

    let mut winner = std::collections::HashMap::new();
    for (name, confidence) in matches {
        winner
            .entry(name)
            .and_modify(|existing: &mut f32| {
                if confidence > *existing {
                    *existing = confidence;
                }
            })
            .or_insert(confidence);
    }

    if winner.len() == 1 {
        winner.into_iter().next().map(|(name, _)| name)
    } else {
        None
    }
}

fn to_hashset(label: &str) -> HashSet<String> {
    HashSet::from([label.to_string()])
}

async fn refine_prediction(
    engine: Arc<DefaultRefineEngine>,
    sample: &GenreEvaluationSample,
    candidates: &[GenreCandidate],
    tag_profile: &TagProfile,
) -> Result<String> {
    let article = DeduplicatedArticle {
        id: sample.article_id.clone(),
        title: None,
        sentences: if sample.sentences.is_empty() {
            vec![sample.expected_genre.clone()]
        } else {
            sample.sentences.clone()
        },
        sentence_hashes: Vec::new(),
        language: sample.language.clone(),
        published_at: None,
        source_url: None,
        tags: tag_profile.top_tags.clone(),
        duplicates: Vec::new(),
    };
    let job = JobContext::new(sample.job_id, Vec::new());
    let input = RefineInput {
        job: &job,
        article: &article,
        candidates,
        tag_profile,
        fallback: TagFallbackMode::AllowRefine,
    };
    let outcome = engine.refine(input).await?;
    Ok(outcome.final_genre)
}

/// Runs the offline two-stage genre evaluation and returns aggregated metrics.
pub async fn evaluate_two_stage(
    samples: &[GenreEvaluationSample],
    settings: EvaluationSettings,
) -> Result<GenreEvaluationReport> {
    if samples.is_empty() {
        bail!("evaluation requires at least one sample");
    }

    let mut config = RefineConfig::new(settings.require_tags);
    if let Some(value) = settings.graph_margin {
        config.graph_margin = value;
    }
    if let Some(value) = settings.tag_confidence_gate {
        config.tag_confidence_gate = value;
    }
    if let Some(value) = settings.llm_tie_break_margin {
        config.weighted_tie_break_margin = value;
    }

    let tag_gate = config.tag_confidence_gate;
    let graph_cache = TagLabelGraphCache::from_edges(&collect_graph_edges(samples));
    let graph_source = Arc::new(StaticEvaluationGraph::new(graph_cache));
    let engine = Arc::new(DefaultRefineEngine::new(config, graph_source));

    let mut coarse_calc = MetricsCalculator::new(2);
    let mut tag_calc = MetricsCalculator::new(2);
    let mut two_stage_calc = MetricsCalculator::new(2);

    for sample in samples {
        let internal_candidates: Vec<GenreCandidate> = sample
            .candidates
            .iter()
            .map(to_internal_candidate)
            .collect();

        let normalized_candidates: Vec<(String, GenreCandidate)> = internal_candidates
            .iter()
            .map(|candidate| (candidate.name.trim().to_lowercase(), candidate.clone()))
            .collect();

        let tag_signals: Vec<TagSignal> = sample.tags.iter().map(to_internal_tag).collect();
        let tag_profile = TagProfile::from_signals(&tag_signals);

        let expected_set = to_hashset(&sample.expected_genre);

        let coarse_pred = internal_candidates
            .iter()
            .max_by(|a, b| {
                a.score
                    .partial_cmp(&b.score)
                    .unwrap_or(std::cmp::Ordering::Equal)
            })
            .map_or_else(|| "other".to_string(), |candidate| candidate.name.clone());
        let mut sorted_candidates = internal_candidates.clone();
        sorted_candidates.sort_by(|a, b| {
            b.score
                .partial_cmp(&a.score)
                .unwrap_or(std::cmp::Ordering::Equal)
        });
        let top_k_list: Vec<String> = sorted_candidates.iter().map(|c| c.name.clone()).collect();

        coarse_calc.push(
            expected_set.clone(),
            to_hashset(&coarse_pred),
            Some(&top_k_list),
        );

        let tag_pred = compute_tag_prediction(&normalized_candidates, &tag_signals, tag_gate)
            .unwrap_or_else(|| coarse_pred.clone());
        tag_calc.push(expected_set.clone(), to_hashset(&tag_pred), None);

        let two_stage_pred =
            refine_prediction(engine.clone(), sample, &internal_candidates, &tag_profile).await?;
        two_stage_calc.push(expected_set, to_hashset(&two_stage_pred), None);
    }

    Ok(GenreEvaluationReport {
        coarse: coarse_calc.finalize(),
        tag: tag_calc.finalize(),
        two_stage: two_stage_calc.finalize(),
    })
}
