use std::collections::HashMap;
use std::sync::Arc;

use anyhow::Result;
use async_trait::async_trait;
use chrono::Utc;
use tracing::{debug, info, warn};
use uuid::Uuid;

use crate::classification::{ClassificationLanguage, GenreClassifier};
use crate::observability::metrics::Metrics;
use crate::scheduler::JobContext;
use crate::store::dao::RecapDao;
use crate::store::models::{
    CoarseCandidateRecord, GenreLearningRecord, LearningTimestamps, RefineDecisionRecord,
    TagProfileRecord, TagSignalRecord, TelemetryRecord,
};

use super::dedup::{DeduplicatedArticle, DeduplicatedCorpus};
use super::genre_keywords::GenreKeywords;
use super::genre_refine::{
    RefineEngine, RefineInput, RefineOutcome, RefineStrategy, TagFallbackMode, TagProfile,
};
use super::tag_signal::TagSignal;

/// Coarseステージで算出されたジャンル候補。
#[derive(Debug, Clone, PartialEq)]
pub(crate) struct GenreCandidate {
    pub(crate) name: String,
    pub(crate) score: f32,
    pub(crate) keyword_support: usize,
    pub(crate) classifier_confidence: f32,
}

/// ジャンル付き記事。
#[derive(Debug, Clone, PartialEq)]
pub(crate) struct GenreAssignment {
    pub(crate) genres: Vec<String>, // 1〜3個のジャンル
    pub(crate) candidates: Vec<GenreCandidate>,
    pub(crate) genre_scores: HashMap<String, usize>, // 全スコア
    pub(crate) genre_confidence: HashMap<String, f32>,
    pub(crate) feature_profile: FeatureProfile,
    pub(crate) article: DeduplicatedArticle,
}

impl GenreAssignment {
    #[must_use]
    pub(crate) fn primary_genre(&self) -> Option<&str> {
        self.genres.first().map(String::as_str)
    }
}

/// ジャンル別の記事グループ。
#[derive(Debug, Clone, PartialEq)]
pub(crate) struct GenreBundle {
    pub(crate) job_id: Uuid,
    pub(crate) assignments: Vec<GenreAssignment>,
    pub(crate) genre_distribution: HashMap<String, usize>, // ジャンル別記事数
}

#[derive(Debug, Clone, Default, PartialEq)]
pub(crate) struct FeatureProfile {
    pub(crate) tfidf_sum: f32,
    pub(crate) bm25_peak: f32,
    pub(crate) token_count: usize,
    pub(crate) tag_overlap_count: usize,
}

#[async_trait]
pub(crate) trait GenreStage: Send + Sync {
    async fn assign(&self, job: &JobContext, corpus: DeduplicatedCorpus) -> Result<GenreBundle>;
}

/// Coarse+Refineを統合するステージ。
pub(crate) struct TwoStageGenreStage {
    coarse: Arc<CoarseGenreStage>,
    refine_engine: Arc<dyn RefineEngine>,
    dao: Arc<RecapDao>,
    require_tags: bool,
    rollout: RefineRollout,
    metrics: Arc<Metrics>,
}

#[derive(Debug, Clone)]
pub(crate) struct RefineRollout {
    percentage: u8,
}

impl RefineRollout {
    #[must_use]
    pub(crate) fn new(percentage: u8) -> Self {
        Self { percentage }
    }

    #[must_use]
    pub(crate) fn allows(&self, job_id: Uuid) -> bool {
        if self.percentage == 0 {
            return false;
        }
        if self.percentage >= 100 {
            return true;
        }
        let bucket = (job_id.as_u128() % 100) as u8;
        bucket < self.percentage
    }
}

impl TwoStageGenreStage {
    pub(crate) fn new(
        coarse: Arc<CoarseGenreStage>,
        refine_engine: Arc<dyn RefineEngine>,
        dao: Arc<RecapDao>,
        require_tags: bool,
        rollout: RefineRollout,
        metrics: Arc<Metrics>,
    ) -> Self {
        Self {
            coarse,
            refine_engine,
            dao,
            require_tags,
            rollout,
            metrics,
        }
    }

    async fn persist_learning_record(
        &self,
        job: &JobContext,
        assignment: &GenreAssignment,
        tag_profile: &TagProfile,
        outcome: &RefineOutcome,
        refine_duration_ms: u64,
    ) {
        let tag_profile_record = TagProfileRecord {
            top_tags: tag_profile
                .top_tags
                .iter()
                .map(|tag| TagSignalRecord {
                    label: tag.label.clone(),
                    confidence: tag.confidence,
                    source: tag.source.clone(),
                    source_ts: tag.source_ts,
                })
                .collect(),
            entropy: tag_profile.entropy,
        };

        let graph_boosts = outcome.graph_boosts();
        let coarse_records: Vec<CoarseCandidateRecord> = assignment
            .candidates
            .iter()
            .map(|candidate| {
                let normalized = normalize_label(&candidate.name);
                let boost = graph_boosts.get(&normalized).copied();
                let llm_conf = if candidate.name.eq_ignore_ascii_case(&outcome.final_genre)
                    && matches!(
                        outcome.strategy,
                        super::genre_refine::RefineStrategy::LlmTieBreak
                    ) {
                    Some(outcome.confidence)
                } else {
                    None
                };
                CoarseCandidateRecord {
                    genre: candidate.name.clone(),
                    score: candidate.score,
                    keyword_support: candidate.keyword_support,
                    classifier_confidence: candidate.classifier_confidence,
                    tag_overlap_count: Some(assignment.feature_profile.tag_overlap_count),
                    graph_boost: boost,
                    llm_confidence: llm_conf,
                }
            })
            .collect();

        let decision = RefineDecisionRecord {
            final_genre: outcome.final_genre.clone(),
            confidence: outcome.confidence,
            strategy: format_strategy(outcome.strategy),
            llm_trace_id: outcome.llm_trace_id.clone(),
            notes: None,
        };

        let timestamps = LearningTimestamps::new(Utc::now(), Utc::now());
        let telemetry = TelemetryRecord {
            refine_duration_ms: Some(refine_duration_ms),
            llm_latency_ms: None,
            coarse_latency_ms: None,
            cache_hits: None,
        };

        let record = GenreLearningRecord::new(
            job.job_id,
            &assignment.article.id,
            coarse_records,
            decision,
            tag_profile_record,
            timestamps,
        )
        .with_telemetry(Some(telemetry));

        if let Err(err) = self.dao.upsert_genre_learning_record(&record).await {
            warn!(job_id = %job.job_id, article_id = %assignment.article.id, error = ?err, "failed to persist genre learning record");
        }
    }

    async fn apply_refine_outcome(
        &self,
        job: &JobContext,
        assignment: &mut GenreAssignment,
        tag_profile: &TagProfile,
        outcome: &RefineOutcome,
        refine_duration_ms: u64,
        distribution: &mut HashMap<String, usize>,
    ) {
        let final_genre = outcome.final_genre.clone();
        assignment.genres = vec![final_genre.clone()];
        assignment
            .genre_confidence
            .insert(final_genre.clone(), outcome.confidence);
        assignment
            .genre_scores
            .entry(final_genre.clone())
            .or_insert_with(|| (outcome.confidence * 100.0).round() as usize);

        *distribution.entry(final_genre.clone()).or_insert(0) += 1;

        match outcome.strategy {
            RefineStrategy::GraphBoost => {
                self.metrics.genre_refine_graph_hits.inc();
            }
            RefineStrategy::FallbackOther | RefineStrategy::CoarseOnly => {
                self.metrics.genre_refine_fallback_total.inc();
            }
            _ => {}
        }

        self.persist_learning_record(job, assignment, tag_profile, outcome, refine_duration_ms)
            .await;
    }
}

/// キーワードベースのCoarseジャンルステージ。
///
/// タイトル+本文からキーワードマッチングで最大 `max_genres` 件の候補を抽出する。
#[derive(Debug)]
pub(crate) struct CoarseGenreStage {
    classifier: GenreClassifier,
    fallback_keywords: GenreKeywords,
    min_genres: usize,
    max_genres: usize,
}

impl CoarseGenreStage {
    /// 新しいCoarseGenreStageを作成する。
    ///
    /// # Arguments
    /// * `min_genres` - 最小ジャンル数（デフォルト: 1）
    /// * `max_genres` - 最大ジャンル数（デフォルト: 3）
    pub(crate) fn new(min_genres: usize, max_genres: usize) -> Self {
        Self {
            classifier: GenreClassifier::new_default(),
            fallback_keywords: GenreKeywords::default_keywords(),
            min_genres,
            max_genres,
        }
    }

    /// デフォルトパラメータで作成する（1〜3ジャンル）。
    pub(crate) fn with_defaults() -> Self {
        Self::new(1, 3)
    }

    /// 記事からジャンル候補を生成する。
    fn produce_candidates(
        &self,
        article: &DeduplicatedArticle,
    ) -> anyhow::Result<(
        Vec<GenreCandidate>,
        Vec<String>,
        HashMap<String, usize>,
        HashMap<String, f32>,
        FeatureProfile,
    )> {
        let title = article.title.as_deref().unwrap_or("");
        let body = article.sentences.join(" ");
        let language = ClassificationLanguage::from_code(&article.language);

        let classification = self.classifier.predict(title, &body, language)?;
        let mut selected_genres = classification.top_genres.clone();

        // 最低ジャンル数を満たすまでランキングから補完
        if selected_genres.len() < self.min_genres {
            for (candidate, _) in &classification.ranking {
                if selected_genres.contains(candidate) {
                    continue;
                }
                selected_genres.push(candidate.clone());
                if selected_genres.len() == self.min_genres {
                    break;
                }
            }
        }

        if selected_genres.is_empty() {
            selected_genres.push("other".to_string());
        }

        if selected_genres.len() > self.max_genres {
            selected_genres.truncate(self.max_genres);
        }

        let mut genre_scores = classification.keyword_hits.clone();
        for genre in &selected_genres {
            genre_scores.entry(genre.clone()).or_insert_with(|| {
                classification
                    .scores
                    .get(genre)
                    .map(|score| (score.max(0.0) * 100.0).round() as usize)
                    .unwrap_or(0)
            });
        }

        if genre_scores.is_empty() {
            // フォールバックとしてキーワードスコアを計算
            let combined = format!("{title} {body}");
            genre_scores = self.fallback_keywords.score_text(&combined);
        }

        let low_support = selected_genres
            .iter()
            .all(|genre| genre_scores.get(genre).copied().unwrap_or(0) == 0);
        if low_support {
            selected_genres.clear();
            selected_genres.push("other".to_string());
            genre_scores.entry("other".to_string()).or_insert(100);
        }

        let mut genre_confidence: HashMap<String, f32> = classification
            .scores
            .iter()
            .map(|(genre, score)| (genre.clone(), score.clamp(0.0, 1.0)))
            .collect();
        for genre in &selected_genres {
            genre_confidence.entry(genre.clone()).or_insert(0.0);
        }

        let tfidf_sum: f32 = classification.feature_snapshot.tfidf.iter().sum();
        let lowercase_genres: Vec<String> =
            selected_genres.iter().map(|g| g.to_lowercase()).collect();
        let tag_overlap_count = article
            .tags
            .iter()
            .filter(|TagSignal { label, .. }| {
                let normalized = label.to_lowercase();
                lowercase_genres.iter().any(|g| g == &normalized)
            })
            .count();
        let feature_profile = FeatureProfile {
            tfidf_sum,
            bm25_peak: classification.feature_snapshot.max_bm25().unwrap_or(0.0),
            token_count: classification.token_count,
            tag_overlap_count,
        };

        let candidates = selected_genres
            .iter()
            .map(|genre| {
                let keyword_support = genre_scores.get(genre).copied().unwrap_or(0);
                let classifier_confidence = genre_confidence
                    .get(genre)
                    .copied()
                    .unwrap_or_default()
                    .clamp(0.0, 1.0);
                let score = classification
                    .ranking
                    .iter()
                    .find(|(name, _)| name == genre)
                    .map(|(_, score)| *score)
                    .unwrap_or(classifier_confidence);

                GenreCandidate {
                    name: genre.clone(),
                    score,
                    keyword_support,
                    classifier_confidence,
                }
            })
            .collect();

        Ok((
            candidates,
            selected_genres,
            genre_scores,
            genre_confidence,
            feature_profile,
        ))
    }
}

impl Default for CoarseGenreStage {
    fn default() -> Self {
        Self::with_defaults()
    }
}

#[async_trait]
impl GenreStage for CoarseGenreStage {
    async fn assign(&self, job: &JobContext, corpus: DeduplicatedCorpus) -> Result<GenreBundle> {
        let total_articles = corpus.articles.len();
        info!(
            job_id = %job.job_id,
            count = total_articles,
            "starting genre assignment with keyword heuristics"
        );

        let mut assignments = Vec::with_capacity(total_articles);
        let mut genre_distribution: HashMap<String, usize> = HashMap::new();

        for article in corpus.articles {
            let (candidates, genres, genre_scores, genre_confidence, feature_profile) =
                self.produce_candidates(&article)?;

            debug!(
                article_id = %article.id,
                genres = ?genres,
                candidates = ?candidates,
                "assigned genres to article"
            );

            // 分布を更新
            for genre in &genres {
                *genre_distribution.entry(genre.clone()).or_insert(0) += 1;
            }

            assignments.push(GenreAssignment {
                genres,
                candidates,
                genre_scores,
                genre_confidence,
                feature_profile,
                article,
            });
        }

        info!(
            job_id = %job.job_id,
            total_assignments = assignments.len(),
            genre_distribution = ?genre_distribution,
            "completed genre assignment"
        );

        Ok(GenreBundle {
            job_id: job.job_id,
            assignments,
            genre_distribution,
        })
    }
}

#[async_trait]
impl GenreStage for TwoStageGenreStage {
    async fn assign(&self, job: &JobContext, corpus: DeduplicatedCorpus) -> Result<GenreBundle> {
        let coarse_bundle = self.coarse.assign(job, corpus).await?;
        let mut assignments = Vec::with_capacity(coarse_bundle.assignments.len());
        let mut distribution: HashMap<String, usize> = HashMap::new();
        let refine_allowed = self.rollout.allows(job.job_id);
        if refine_allowed {
            self.metrics.genre_refine_rollout_enabled.inc();
        } else {
            self.metrics.genre_refine_rollout_skipped.inc();
        }

        for mut assignment in coarse_bundle.assignments {
            let tag_profile = TagProfile::from_signals(&assignment.article.tags);
            if !refine_allowed {
                let fallback_candidate =
                    assignment
                        .candidates
                        .first()
                        .cloned()
                        .unwrap_or_else(|| GenreCandidate {
                            name: "other".to_string(),
                            score: 0.0,
                            keyword_support: 0,
                            classifier_confidence: 0.0,
                        });
                let outcome = RefineOutcome::new(
                    fallback_candidate.name.clone(),
                    fallback_candidate.classifier_confidence,
                    RefineStrategy::CoarseOnly,
                    None,
                    HashMap::new(),
                );
                self.apply_refine_outcome(
                    job,
                    &mut assignment,
                    &tag_profile,
                    &outcome,
                    0,
                    &mut distribution,
                )
                .await;
                assignments.push(assignment);
                continue;
            }

            let fallback = TagFallbackMode::require_tags(self.require_tags, tag_profile.has_tags());
            let refine_input = RefineInput {
                job,
                article: &assignment.article,
                candidates: &assignment.candidates,
                tag_profile: &tag_profile,
                fallback,
            };

            let refine_start = std::time::Instant::now();
            let outcome = self.refine_engine.refine(refine_input).await?;
            let refine_duration_ms = refine_start.elapsed().as_millis() as u64;
            self.apply_refine_outcome(
                job,
                &mut assignment,
                &tag_profile,
                &outcome,
                refine_duration_ms,
                &mut distribution,
            )
            .await;
            assignments.push(assignment);
        }

        Ok(GenreBundle {
            job_id: coarse_bundle.job_id,
            assignments,
            genre_distribution: distribution,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::super::dedup::DedupStats;
    use super::*;

    fn article(id: &str, title: Option<&str>, sentences: Vec<&str>) -> DeduplicatedArticle {
        DeduplicatedArticle {
            id: id.to_string(),
            title: title.map(String::from),
            sentences: sentences.into_iter().map(String::from).collect(),
            sentence_hashes: vec![],
            language: "en".to_string(),
            tags: Vec::new(),
        }
    }

    #[tokio::test]
    async fn assigns_genres_based_on_keywords() {
        let stage = CoarseGenreStage::with_defaults();
        let job = JobContext::new(Uuid::new_v4(), vec![]);
        let corpus = DeduplicatedCorpus {
            job_id: job.job_id,
            articles: vec![
                article(
                    "art-1",
                    Some("Machine Learning and AI"),
                    vec!["This article discusses artificial intelligence and deep learning."],
                ),
                article(
                    "art-2",
                    Some("Football Championship"),
                    vec!["The team won the tournament in an exciting match."],
                ),
            ],
            stats: DedupStats::default(),
        };

        let bundle = stage.assign(&job, corpus).await.unwrap();

        assert_eq!(bundle.assignments.len(), 2);

        // 最初の記事はAI関連のキーワードを含む
        assert!(
            bundle.assignments[0]
                .candidates
                .iter()
                .any(|candidate| candidate.name == "ai" || candidate.name == "tech")
        );

        // 2番目の記事はスポーツ関連のキーワードを含む
        assert!(
            bundle.assignments[1]
                .candidates
                .iter()
                .any(|candidate| candidate.name == "sports")
        );
    }

    #[tokio::test]
    async fn assigns_at_least_one_genre() {
        let stage = CoarseGenreStage::with_defaults();
        let job = JobContext::new(Uuid::new_v4(), vec![]);
        let corpus = DeduplicatedCorpus {
            job_id: job.job_id,
            articles: vec![article(
                "art-1",
                Some("Generic Article"),
                vec!["This is a generic article with no specific keywords."],
            )],
            stats: DedupStats::default(),
        };

        let bundle = stage.assign(&job, corpus).await.unwrap();

        assert_eq!(bundle.assignments.len(), 1);
        assert!(!bundle.assignments[0].candidates.is_empty());
        // キーワードマッチがない場合は"other"が付与される
        assert!(
            bundle.assignments[0]
                .candidates
                .iter()
                .any(|candidate| candidate.name == "other")
        );
    }

    #[tokio::test]
    async fn respects_max_genres_limit() {
        let stage = CoarseGenreStage::new(1, 2);
        let job = JobContext::new(Uuid::new_v4(), vec![]);
        let corpus = DeduplicatedCorpus {
            job_id: job.job_id,
            articles: vec![article(
                "art-1",
                Some("Tech Science Business AI Health"),
                vec!["Technology, science, business, AI, and health news."],
            )],
            stats: DedupStats::default(),
        };

        let bundle = stage.assign(&job, corpus).await.unwrap();

        assert_eq!(bundle.assignments.len(), 1);
        // 最大2ジャンル
        assert!(bundle.assignments[0].candidates.len() <= 2);
    }
}

fn normalize_label(value: &str) -> String {
    value.trim().to_lowercase()
}

fn format_strategy(strategy: super::genre_refine::RefineStrategy) -> String {
    match strategy {
        super::genre_refine::RefineStrategy::TagConsistency => "tag_consistency",
        super::genre_refine::RefineStrategy::GraphBoost => "graph_boost",
        super::genre_refine::RefineStrategy::LlmTieBreak => "llm_tie_break",
        super::genre_refine::RefineStrategy::FallbackOther => "fallback_other",
        super::genre_refine::RefineStrategy::CoarseOnly => "coarse_only",
    }
    .to_string()
}
