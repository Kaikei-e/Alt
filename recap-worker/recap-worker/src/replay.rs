use std::convert::TryInto;
use std::fs::File;
use std::io::{BufRead, BufReader};
use std::path::PathBuf;
use std::sync::Arc;
use std::time::{Duration, Instant};

use anyhow::{Context, Result};
use async_trait::async_trait;
use chrono::Utc;
use serde::Deserialize;
use serde_json;
use sqlx::postgres::PgPoolOptions;
use uuid::Uuid;

use crate::pipeline::dedup::DeduplicatedArticle;
use crate::pipeline::genre::GenreCandidate;
use crate::pipeline::genre_refine::{
    DbTagLabelGraphSource, DefaultRefineEngine, LlmDecision, LlmTieBreaker, RefineConfig,
    RefineEngine, RefineInput, RefineOutcome, RefineStrategy, TagFallbackMode, TagLabelGraphSource,
    TagProfile,
};
use crate::pipeline::tag_signal::TagSignal;
use crate::scheduler::JobContext;
use crate::store::dao::RecapDao;
use crate::store::models::{
    CoarseCandidateRecord, GenreLearningRecord, GraphEdgeRecord, LearningTimestamps,
    RefineDecisionRecord, TagProfileRecord, TagSignalRecord, TelemetryRecord,
};

/// Configuration required by the offline replay helper.
pub struct ReplayConfig {
    pub dataset: PathBuf,
    pub dsn: String,
    pub graph_window: String,
    pub graph_ttl_seconds: u64,
    pub require_tags: bool,
    pub dry_run: bool,
}

/// Replay the genre pipeline offline and persist refreshed learning rows.
pub async fn replay_genre_pipeline(config: ReplayConfig) -> Result<()> {
    let pool = PgPoolOptions::new()
        .max_connections(5)
        .connect_lazy(&config.dsn)
        .context("failed to configure postgres pool")?;
    let dao = Arc::new(RecapDao::new(pool));

    let graph_loader = Arc::new(DbTagLabelGraphSource::new(
        Arc::clone(&dao),
        config.graph_window.clone(),
        Duration::from_secs(config.graph_ttl_seconds),
    ));
    graph_loader
        .preload()
        .await
        .context("failed to preload tag label graph")?;
    let graph_source: Arc<dyn TagLabelGraphSource> = graph_loader;

    let refine_engine = Arc::new(DefaultRefineEngine::new(
        RefineConfig::new(config.require_tags),
        graph_source,
        Arc::new(ReplayGreedyLlm::default()),
    ));

    let file = File::open(&config.dataset)
        .with_context(|| format!("failed to open dataset at {}", config.dataset.display()))?;
    let reader = BufReader::new(file);

    let mut processed = 0usize;
    let mut stored = 0usize;

    for (idx, line) in reader.lines().enumerate() {
        let line = line.context("failed to read dataset line")?;
        if line.trim().is_empty() {
            continue;
        }
        let record: ReplayRecord = serde_json::from_str(&line)
            .with_context(|| format!("failed to parse JSON on line {}", idx + 1))?;
        processed += 1;

        if process_record(&record, &refine_engine, &dao, config.dry_run).await? {
            stored += 1;
        }
    }

    println!(
        "Processed {} records ({} persisted to recap_genre_learning_results, dry_run={})",
        processed, stored, config.dry_run
    );

    Ok(())
}

async fn process_record(
    record: &ReplayRecord,
    engine: &Arc<DefaultRefineEngine>,
    dao: &Arc<RecapDao>,
    dry_run: bool,
) -> Result<bool> {
    let candidates: Vec<GenreCandidate> = record
        .coarse_candidates
        .iter()
        .map(|candidate| GenreCandidate {
            name: candidate.genre.clone(),
            score: candidate.score,
            keyword_support: candidate.keyword_support,
            classifier_confidence: candidate.classifier_confidence,
        })
        .collect();

    let (tag_profile, tag_signals) = build_tag_profile(&record.tag_profile);

    let article = DeduplicatedArticle {
        id: record.article_id.clone(),
        title: record.title.clone(),
        sentences: build_sentences(record),
        sentence_hashes: Vec::new(),
        language: record
            .lang_hint
            .clone()
            .unwrap_or_else(|| "unknown".to_string()),
        tags: tag_signals.clone(),
    };

    let job = JobContext::new(record.job_id, Vec::new());
    let input = RefineInput {
        job: &job,
        article: &article,
        candidates: &candidates,
        tag_profile: &tag_profile,
        fallback: TagFallbackMode::AllowRefine,
    };

    let start = Instant::now();
    let outcome = engine.refine(input).await?;
    let duration = start.elapsed();

    let decision = RefineDecisionRecord {
        final_genre: outcome.final_genre.clone(),
        confidence: outcome.confidence,
        strategy: strategy_label(outcome.strategy),
        llm_trace_id: outcome.llm_trace_id.clone(),
        notes: None,
    };

    let coarse_records = refresh_coarse_records(&record.coarse_candidates, &outcome);
    let timestamps = LearningTimestamps::new(Utc::now(), Utc::now());
    let telemetry = TelemetryRecord {
        refine_duration_ms: Some(duration.as_millis().try_into().unwrap_or(u64::MAX)),
        llm_latency_ms: None,
        coarse_latency_ms: None,
        cache_hits: None,
    };

    let mut learning_record = GenreLearningRecord::new(
        record.job_id,
        &record.article_id,
        coarse_records,
        decision,
        record.tag_profile.clone(),
        timestamps,
    )
    .with_telemetry(Some(telemetry));

    learning_record.graph_context = record.graph_context.clone().unwrap_or_default();

    if dry_run {
        println!(
            "[dry-run] would replay article {} with strategy {}",
            record.article_id, learning_record.refine_decision.strategy
        );
        return Ok(false);
    }

    dao.upsert_genre_learning_record(&learning_record)
        .await
        .with_context(|| {
            format!(
                "failed to persist learning record for article {}",
                record.article_id
            )
        })?;
    Ok(true)
}

fn refresh_coarse_records(
    base: &[CoarseCandidateRecord],
    outcome: &RefineOutcome,
) -> Vec<CoarseCandidateRecord> {
    base.iter()
        .map(|record| {
            let normalized = normalize(&record.genre);
            let boost = outcome.graph_boosts().get(&normalized).copied();
            let llm_confidence = if record.genre.eq_ignore_ascii_case(&outcome.final_genre)
                && matches!(outcome.strategy, RefineStrategy::LlmTieBreak)
            {
                Some(outcome.confidence)
            } else {
                None
            };
            CoarseCandidateRecord {
                genre: record.genre.clone(),
                score: record.score,
                keyword_support: record.keyword_support,
                classifier_confidence: record.classifier_confidence,
                tag_overlap_count: record.tag_overlap_count,
                graph_boost: boost,
                llm_confidence,
            }
        })
        .collect()
}

fn strategy_label(strategy: RefineStrategy) -> String {
    match strategy {
        RefineStrategy::TagConsistency => "tag_consistency",
        RefineStrategy::GraphBoost => "graph_boost",
        RefineStrategy::LlmTieBreak => "llm_tie_break",
        RefineStrategy::FallbackOther => "fallback_other",
        RefineStrategy::CoarseOnly => "coarse_only",
    }
    .to_string()
}

fn normalize(value: &str) -> String {
    value.trim().to_lowercase()
}

fn build_tag_profile(record: &TagProfileRecord) -> (TagProfile, Vec<TagSignal>) {
    let signals = record
        .top_tags
        .iter()
        .map(tag_signal_from_record)
        .collect::<Vec<_>>();
    (TagProfile::from_signals(&signals), signals)
}

fn tag_signal_from_record(record: &TagSignalRecord) -> TagSignal {
    TagSignal {
        label: record.label.clone(),
        confidence: record.confidence,
        source: record.source.clone(),
        source_ts: record.source_ts,
    }
}

fn build_sentences(record: &ReplayRecord) -> Vec<String> {
    if let Some(sentences) = &record.sentences {
        let filtered = sentences
            .iter()
            .filter(|sentence| !sentence.trim().is_empty())
            .cloned()
            .collect::<Vec<_>>();
        if !filtered.is_empty() {
            return filtered;
        }
    }
    if let Some(body) = &record.body_excerpt {
        if !body.trim().is_empty() {
            return vec![body.clone()];
        }
    }
    Vec::new()
}

#[derive(Deserialize)]
struct ReplayRecord {
    job_id: Uuid,
    article_id: String,
    lang_hint: Option<String>,
    title: Option<String>,
    body_excerpt: Option<String>,
    sentences: Option<Vec<String>>,
    coarse_candidates: Vec<CoarseCandidateRecord>,
    tag_profile: TagProfileRecord,
    graph_context: Option<Vec<GraphEdgeRecord>>,
}

#[derive(Debug, Default)]
struct ReplayGreedyLlm;

#[async_trait]
impl LlmTieBreaker for ReplayGreedyLlm {
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
