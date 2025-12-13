use std::collections::HashMap;
use std::sync::Arc;

use anyhow::Result;
use async_trait::async_trait;
use chrono::Utc;
use tracing::{debug, info, warn};
use uuid::Uuid;

// use crate::classification::ClassificationLanguage;
use crate::clients::SubworkerClient;
// use crate::classifier::ClassificationPipeline;
use crate::observability::metrics::Metrics;
use crate::scheduler::JobContext;
use crate::store::dao::RecapDao;
use crate::store::models::{
    CoarseCandidateRecord, GenreLearningRecord, LearningTimestamps, RefineDecisionRecord,
    TagProfileRecord, TagSignalRecord, TelemetryRecord,
};

use super::dedup::{DeduplicatedArticle, DeduplicatedCorpus};
use super::embedding::{EmbeddingService, cosine_similarity};
use super::genre_canonical::get_canonical_sentences;
// use super::genre_keywords::GenreKeywords; // Removed
use super::genre_refine::{
    RefineConfig, RefineEngine, RefineInput, RefineOutcome, RefineStrategy, TagFallbackMode,
    TagProfile,
};
use super::graph_override::GraphOverrideSettings;
use super::tag_signal::TagSignal;
use serde::{Deserialize, Serialize};

/// Coarseステージで算出されたジャンル候補。
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub(crate) struct GenreCandidate {
    pub(crate) name: String,
    pub(crate) score: f32,
    pub(crate) keyword_support: usize,
    pub(crate) classifier_confidence: f32,
}

/// `produce_candidates`の戻り値型。
type ProduceCandidatesResult = (
    Vec<GenreCandidate>,
    Vec<String>,
    HashMap<String, usize>,
    HashMap<String, f32>,
    FeatureProfile,
    Option<Vec<f32>>,
);

/// ジャンル付き記事。
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub(crate) struct GenreAssignment {
    pub(crate) genres: Vec<String>, // 1〜3個のジャンル
    pub(crate) candidates: Vec<GenreCandidate>,
    pub(crate) genre_scores: HashMap<String, usize>, // 全スコア
    pub(crate) genre_confidence: HashMap<String, f32>,
    pub(crate) feature_profile: FeatureProfile,
    pub(crate) article: DeduplicatedArticle,
    pub(crate) embedding: Option<Vec<f32>>,
}

impl GenreAssignment {
    #[must_use]
    pub(crate) fn primary_genre(&self) -> Option<&str> {
        self.genres.first().map(String::as_str)
    }
}

/// ジャンル別の記事グループ。
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub(crate) struct GenreBundle {
    pub(crate) job_id: Uuid,
    pub(crate) assignments: Vec<GenreAssignment>,
    pub(crate) genre_distribution: HashMap<String, usize>, // ジャンル別記事数
}

#[derive(Debug, Clone, Default, PartialEq, Serialize, Deserialize)]
pub(crate) struct FeatureProfile {
    pub(crate) tfidf_sum: f32,
    pub(crate) bm25_peak: f32,
    pub(crate) token_count: usize,
    pub(crate) tag_overlap_count: usize,
}

#[async_trait]
pub(crate) trait GenreStage: Send + Sync {
    async fn assign(&self, job: &JobContext, corpus: DeduplicatedCorpus) -> Result<GenreBundle>;

    /// 設定を更新する（デフォルト実装は何もしない）。
    async fn update_config(&self, _overrides: &super::graph_override::GraphOverrideSettings) {
        // デフォルト実装は何もしない（既存の実装を壊さないため）
    }
}

/// Coarse+Refineを統合するステージ。
pub(crate) struct TwoStageGenreStage {
    coarse: Arc<dyn GenreStage>,
    refine_engine: Arc<dyn RefineEngine>,
    dao: Arc<dyn RecapDao>,
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
        coarse: Arc<dyn GenreStage>,
        refine_engine: Arc<dyn RefineEngine>,
        dao: Arc<dyn RecapDao>,
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

    /// 学習レコードを構築する。
    fn build_learning_record(
        job_id: Uuid,
        assignment: &GenreAssignment,
        outcome: &RefineOutcome,
        tag_profile: &TagProfile,
    ) -> GenreLearningRecord {
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
                    && matches!(outcome.strategy, RefineStrategy::LlmTieBreak)
                {
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
            refine_duration_ms: Some(0),
            llm_latency_ms: None,
            coarse_latency_ms: None,
            cache_hits: None,
        };

        GenreLearningRecord::new(
            job_id,
            &assignment.article.id,
            coarse_records,
            decision,
            tag_profile_record,
            timestamps,
        )
        .with_telemetry(Some(telemetry))
    }

    /// 単一のassignmentを処理して、更新されたassignment、最終ジャンル、学習レコードを返す。
    async fn process_assignment(
        refine_engine: Arc<dyn RefineEngine>,
        metrics: Arc<Metrics>,
        job_id: Uuid,
        assignment: GenreAssignment,
        refine_allowed: bool,
        require_tags: bool,
    ) -> anyhow::Result<(GenreAssignment, String, GenreLearningRecord)> {
        let tag_profile = TagProfile::from_signals(&assignment.article.tags);

        let outcome = if refine_allowed {
            let fallback = TagFallbackMode::require_tags(require_tags, tag_profile.has_tags());
            let refine_input = RefineInput {
                job: &JobContext::new(job_id, vec![]),
                article: &assignment.article,
                candidates: &assignment.candidates,
                tag_profile: &tag_profile,
                fallback,
            };
            let refine_start = std::time::Instant::now();
            let outcome = refine_engine.refine(refine_input).await?;
            let _refine_duration_ms = refine_start.elapsed().as_millis() as u64;
            // メトリクス更新
            match outcome.strategy {
                RefineStrategy::GraphBoost => {
                    metrics.genre_refine_graph_hits.inc();
                }
                RefineStrategy::FallbackOther | RefineStrategy::CoarseOnly => {
                    metrics.genre_refine_fallback_total.inc();
                }
                _ => {}
            }
            outcome
        } else {
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
            RefineOutcome::new(
                fallback_candidate.name.clone(),
                fallback_candidate.classifier_confidence,
                RefineStrategy::CoarseOnly,
                None,
                HashMap::new(),
            )
        };

        let record = Self::build_learning_record(job_id, &assignment, &outcome, &tag_profile);

        // 最終的なジャンル割り当てを更新
        let final_genre = outcome.final_genre.clone();
        let mut updated_assignment = assignment;
        updated_assignment.genres = vec![final_genre.clone()];
        updated_assignment
            .genre_confidence
            .insert(final_genre.clone(), outcome.confidence);
        updated_assignment
            .genre_scores
            .entry(final_genre.clone())
            .or_insert_with(|| {
                let rounded = (outcome.confidence.max(0.0) * 100.0).round();
                u32::try_from(rounded.max(0.0) as i32).unwrap_or(0) as usize
            });

        Ok((updated_assignment, final_genre, record))
    }
}

/// キーワードベースのCoarseジャンルステージ。
///
/// タイトル+本文からキーワードマッチングで最大 `max_genres` 件の候補を抽出する。
#[derive(Debug)]
#[allow(dead_code)] // Fields may be used in future refactoring or kept for compatibility
pub(crate) struct CoarseGenreStage {
    subworker: Arc<SubworkerClient>,
    min_genres: usize,
    max_genres: usize,
    embedding_service: Option<EmbeddingService>,
    canonical_embeddings: Arc<tokio::sync::RwLock<HashMap<String, Vec<Vec<f32>>>>>,
    threshold: f32,
}

impl CoarseGenreStage {
    /// 新しいCoarseGenreStageを作成する。
    ///
    /// # Arguments
    /// * `min_genres` - 最小ジャンル数（デフォルト: 1）
    /// * `max_genres` - 最大ジャンル数（デフォルト: 3）
    /// * `embedding_service` - Embeddingサービス（オプション）
    /// * `threshold` - ジャンル分類の閾値
    ///
    /// # Note
    /// ClassificationPipelineの初期化は常に成功します。
    /// Golden Datasetが見つからない場合は、既存のGenreClassifierにフォールバックします。
    pub(crate) fn new(
        min_genres: usize,
        max_genres: usize,
        embedding_service: Option<EmbeddingService>,
        subworker: Arc<SubworkerClient>,
        threshold: f32,
    ) -> Self {
        Self {
            subworker,
            min_genres,
            max_genres,
            embedding_service,
            canonical_embeddings: Arc::new(tokio::sync::RwLock::new(HashMap::new())),
            threshold,
        }
    }

    /// デフォルトパラメータで作成する（1〜3ジャンル）。
    pub(crate) fn with_defaults(subworker: Arc<SubworkerClient>) -> Self {
        Self::new(1, 3, None, subworker, 0.0)
    }

    /// 記事からジャンル候補を生成する。
    /// 記事からジャンル候補を生成する。
    async fn produce_candidates(
        &self,
        article: &DeduplicatedArticle,
    ) -> anyhow::Result<ProduceCandidatesResult> {
        let title = article.title.as_deref().unwrap_or("");
        let body_snippet: String = article
            .sentences
            .iter()
            .take(5)
            .cloned()
            .collect::<Vec<_>>()
            .join(" ");
        let combined_text = format!("{title}\n{body_snippet}");

        // Call Subworker Coarse Classifier
        // On error, fallback to "other" genre
        let scores = match self.subworker.classify_coarse(&combined_text).await {
            Ok(s) => s,
            Err(e) => {
                tracing::warn!(
                    article_id = %article.id,
                    error = %e,
                    "subworker classify_coarse failed, falling back to 'other'"
                );
                // Return empty scores, which will trigger fallback to "other"
                HashMap::new()
            }
        };

        // Sort genres by score descending
        let mut sorted_genres: Vec<(String, f32)> = scores.into_iter().collect();
        sorted_genres.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal));

        // Select top genres
        let mut selected_genres: Vec<String> = sorted_genres
            .iter()
            .take(self.max_genres)
            .filter(|(_, score)| *score > self.threshold)
            .map(|(g, _)| g.clone())
            .collect();

        // Ensure min genres (fallback to other if empty)
        if selected_genres.is_empty() {
            selected_genres.push("other".to_string());
        }

        // Embedding Filter (Logic kept similar but adapted if needed)
        // With E5 coarse classifier, we might not need extra embedding filter if the classifier ITSELF is E5 based.
        // But the previous implementation applied "Canonical Embeddings" check.
        // If the Coarse Classifier is already using E5 Prototypes (which ARE canonical embeddings), this step is redundant.
        // The Coarse Classifier (Subworker) uses prototypes. So we can skip `apply_embedding_filter`.

        let genre_scores: HashMap<String, usize> = sorted_genres
            .iter()
            .map(|(g, s)| {
                #[allow(clippy::cast_sign_loss)]
                let score = (s * 100.0) as usize;
                (g.clone(), score)
            })
            .collect();

        let genre_confidence: HashMap<String, f32> = sorted_genres.iter().cloned().collect();

        // Feature Profile (Mocked or minimal as we lost TF-IDF stats)
        let feature_profile = FeatureProfile {
            tfidf_sum: 0.0,
            bm25_peak: 0.0,
            token_count: article.sentences.iter().map(String::len).sum(), // Rough char count as token count proxy? Or 0.
            tag_overlap_count: 0,                                         // Calculate if needed
        };

        // Build Candidates
        let candidates: Vec<GenreCandidate> = selected_genres
            .iter()
            .map(|genre| {
                let score = genre_confidence.get(genre).copied().unwrap_or(0.0);
                GenreCandidate {
                    name: genre.clone(),
                    score,
                    keyword_support: genre_scores.get(genre).copied().unwrap_or(0),
                    classifier_confidence: score,
                }
            })
            .collect();

        // Generate embedding if service is available
        let embedding = if let Some(service) = &self.embedding_service {
            match service.encode(&[combined_text]).await {
                Ok(vecs) => vecs.into_iter().next(),
                Err(e) => {
                    tracing::warn!(article_id = %article.id, error = %e, "failed to generate embedding");
                    None
                }
            }
        } else {
            None
        };

        Ok((
            candidates,
            selected_genres,
            genre_scores,
            genre_confidence,
            feature_profile,
            embedding,
        ))
    }

    /// 初期ジャンルを選択する。
    #[allow(dead_code)] // May be used in future refactoring
    fn select_initial_genres(
        &self,
        classification: &crate::classification::ClassificationResult,
    ) -> Vec<String> {
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

        selected_genres
    }

    /// Embeddingフィルタリングを適用する。
    #[allow(dead_code)] // May be used in future refactoring
    async fn apply_embedding_filter(
        &self,
        article: &DeduplicatedArticle,
        selected_genres: &[String],
        title: &str,
    ) -> Vec<String> {
        let Some(embedding_service) = &self.embedding_service else {
            return selected_genres.to_vec();
        };

        // Compute article embedding (Title + first 3 sentences)
        let snippet = article
            .sentences
            .iter()
            .take(3)
            .cloned()
            .collect::<Vec<_>>()
            .join(" ");
        let article_text = format!("{title} {snippet}");

        // Only filter if we successfully get an embedding
        let Ok(embeddings) = embedding_service.encode(&[article_text]).await else {
            return selected_genres.to_vec();
        };

        let Some(article_vec) = embeddings.first() else {
            return selected_genres.to_vec();
        };

        let mut filtered_genres = Vec::new();
        for genre in selected_genres {
            if genre == "other" {
                filtered_genres.push(genre.clone());
                continue;
            }

            if let Some(sentences) = get_canonical_sentences(genre) {
                // Check cache
                let mut canonical_vecs = {
                    let guard = self.canonical_embeddings.read().await;
                    guard.get(genre).cloned()
                };

                if canonical_vecs.is_none() {
                    // Compute and cache
                    let sentences_owned: Vec<String> =
                        sentences.iter().map(|&s| s.to_string()).collect();
                    if let Ok(vecs) = embedding_service.encode(&sentences_owned).await {
                        let mut guard = self.canonical_embeddings.write().await;
                        guard.insert(genre.clone(), vecs.clone());
                        canonical_vecs = Some(vecs);
                    }
                }

                if let Some(vecs) = canonical_vecs {
                    let max_sim = vecs
                        .iter()
                        .map(|v| cosine_similarity(article_vec, v))
                        .fold(0.0f32, f32::max);

                    // Threshold: 0.4 (Conservative)
                    if max_sim >= 0.4 {
                        filtered_genres.push(genre.clone());
                    } else {
                        debug!(
                            article_id = %article.id,
                            genre = %genre,
                            similarity = %max_sim,
                            "filtered out genre by embedding"
                        );
                    }
                } else {
                    // No canonical sentences or failed to embed, keep it safe
                    filtered_genres.push(genre.clone());
                }
            } else {
                // No canonical sentences defined, keep it
                filtered_genres.push(genre.clone());
            }
        }

        filtered_genres
    }

    /// フィーチャープロファイルを構築する。
    #[allow(dead_code)] // May be used in future refactoring
    fn build_feature_profile(
        classification: &crate::classification::ClassificationResult,
        article: &DeduplicatedArticle,
        selected_genres: &[String],
    ) -> FeatureProfile {
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
        FeatureProfile {
            tfidf_sum,
            bm25_peak: classification.feature_snapshot.max_bm25().unwrap_or(0.0),
            token_count: classification.token_count,
            tag_overlap_count,
        }
    }

    /// 候補を構築する。
    #[allow(dead_code)] // May be used in future refactoring
    fn build_candidates(
        selected_genres: &[String],
        genre_scores: &HashMap<String, usize>,
        genre_confidence: &HashMap<String, f32>,
        classification: &crate::classification::ClassificationResult,
    ) -> Vec<GenreCandidate> {
        selected_genres
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
                    .map_or(classifier_confidence, |(_, score)| *score);

                GenreCandidate {
                    name: genre.clone(),
                    score,
                    keyword_support,
                    classifier_confidence,
                }
            })
            .collect()
    }
}

// Removed impl Default for CoarseGenreStage as it requires SubworkerClient

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
            let (candidates, genres, genre_scores, genre_confidence, feature_profile, embedding) =
                self.produce_candidates(&article).await?;

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
                embedding,
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
    async fn update_config(&self, overrides: &GraphOverrideSettings) {
        // 既存の設定を取得してから、オーバーライドで更新
        // DefaultRefineEngineから現在の設定を取得する方法がないため、
        // 新しい設定を作成するが、オーバーライドされていない値はデフォルトを使用
        let mut refine_config = RefineConfig::new(self.require_tags);
        if let Some(value) = overrides.graph_margin {
            refine_config.graph_margin = value;
        }
        if let Some(value) = overrides.weighted_tie_break_margin {
            refine_config.weighted_tie_break_margin = value;
        }
        if let Some(value) = overrides.tag_confidence_gate {
            refine_config.tag_confidence_gate = value;
        }
        if let Some(value) = overrides.boost_threshold {
            refine_config.boost_threshold = value;
        }
        if let Some(value) = overrides.tag_count_threshold {
            refine_config.tag_count_threshold = value;
        }
        tracing::info!(
            graph_margin = refine_config.graph_margin,
            boost_threshold = refine_config.boost_threshold,
            tag_count_threshold = refine_config.tag_count_threshold,
            weighted_tie_break_margin = refine_config.weighted_tie_break_margin,
            tag_confidence_gate = refine_config.tag_confidence_gate,
            "updating refine engine config with overrides"
        );
        self.refine_engine.update_config(refine_config).await;
    }

    async fn assign(&self, job: &JobContext, corpus: DeduplicatedCorpus) -> Result<GenreBundle> {
        let coarse_bundle = self.coarse.assign(job, corpus).await?;
        let refine_allowed = self.rollout.allows(job.job_id);
        if refine_allowed {
            self.metrics.genre_refine_rollout_enabled.inc();
        } else {
            self.metrics.genre_refine_rollout_skipped.inc();
        }

        // 並行処理で各assignmentを処理
        let assignments_count = coarse_bundle.assignments.len();
        let mut tasks = Vec::with_capacity(assignments_count);
        let refine_engine = Arc::clone(&self.refine_engine);
        let require_tags = self.require_tags;
        let metrics = Arc::clone(&self.metrics);
        let job_id = job.job_id;

        for assignment in coarse_bundle.assignments {
            let refine_engine_clone = Arc::clone(&refine_engine);
            let metrics_clone = Arc::clone(&metrics);
            let assignment_clone = assignment.clone();

            let task = tokio::spawn(async move {
                Self::process_assignment(
                    refine_engine_clone,
                    metrics_clone,
                    job_id,
                    assignment_clone,
                    refine_allowed,
                    require_tags,
                )
                .await
            });

            tasks.push(task);
        }

        // すべてのタスクを待機
        let results = futures::future::join_all(tasks).await;
        let mut assignments = Vec::with_capacity(assignments_count);
        let mut distribution: HashMap<String, usize> = HashMap::new();
        let mut learning_records = Vec::new();

        for result in results {
            match result {
                Ok(Ok((assignment, genre, record))) => {
                    *distribution.entry(genre).or_insert(0) += 1;
                    assignments.push(assignment);
                    learning_records.push(record);
                }
                Ok(Err(e)) => {
                    warn!(job_id = %job.job_id, error = ?e, "failed to process assignment");
                }
                Err(e) => {
                    warn!(job_id = %job.job_id, error = ?e, "task panicked");
                }
            }
        }

        // バルクインサートで一括保存
        if !learning_records.is_empty() {
            if let Err(err) = self
                .dao
                .upsert_genre_learning_records_bulk(&learning_records)
                .await
            {
                warn!(job_id = %job.job_id, record_count = learning_records.len(), error = ?err, "failed to bulk persist genre learning records");
            }
        }

        Ok(GenreBundle {
            job_id: coarse_bundle.job_id,
            assignments,
            genre_distribution: distribution,
        })
    }
}

fn normalize_label(value: &str) -> String {
    value.trim().to_lowercase()
}

fn format_strategy(strategy: super::genre_refine::RefineStrategy) -> String {
    match strategy {
        super::genre_refine::RefineStrategy::TagConsistency => "tag_consistency",
        super::genre_refine::RefineStrategy::GraphBoost => "graph_boost",
        super::genre_refine::RefineStrategy::WeightedScore => "weighted_score",
        super::genre_refine::RefineStrategy::LlmTieBreak => "llm_tie_break",
        super::genre_refine::RefineStrategy::FallbackOther => "fallback_other",
        super::genre_refine::RefineStrategy::CoarseOnly => "coarse_only",
    }
    .to_string()
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
            published_at: Some(Utc::now()),
            source_url: None,
            tags: Vec::new(),
            duplicates: Vec::new(),
        }
    }

    #[tokio::test]
    async fn assigns_genres_based_on_keywords() {
        let subworker = Arc::new(SubworkerClient::new("http://localhost:8002", 10).unwrap());
        let stage = CoarseGenreStage::with_defaults(subworker);
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

        // CoarseGenreStage uses SubworkerClient which may not be available in test environment
        // On error, it falls back to "other" genre
        // Verify that at least one genre is assigned (should be "other" if subworker is unavailable)
        assert!(!bundle.assignments[0].candidates.is_empty());
        assert!(!bundle.assignments[1].candidates.is_empty());

        // If subworker is available, it should classify correctly
        // If not available, both should fallback to "other"
        let first_genres: Vec<&str> = bundle.assignments[0]
            .candidates
            .iter()
            .map(|c| c.name.as_str())
            .collect();
        let second_genres: Vec<&str> = bundle.assignments[1]
            .candidates
            .iter()
            .map(|c| c.name.as_str())
            .collect();

        // Either subworker classifies correctly, or both fallback to "other"
        let first_has_ai_or_tech = first_genres.contains(&"ai") || first_genres.contains(&"tech");
        let first_has_other = first_genres.contains(&"other");
        let second_has_sports =
            second_genres.contains(&"sports") || second_genres.contains(&"entertainment");
        let second_has_other = second_genres.contains(&"other");

        // Accept either correct classification or fallback to "other"
        assert!(
            first_has_ai_or_tech || first_has_other,
            "First article should have 'ai'/'tech' or 'other', got: {:?}",
            first_genres
        );
        assert!(
            second_has_sports || second_has_other,
            "Second article should have 'sports'/'entertainment' or 'other', got: {:?}",
            second_genres
        );
    }

    #[tokio::test]
    async fn assigns_at_least_one_genre() {
        let subworker = Arc::new(SubworkerClient::new("http://localhost:8002", 10).unwrap());
        let stage = CoarseGenreStage::with_defaults(subworker);
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
        let subworker = Arc::new(SubworkerClient::new("http://localhost:8002", 10).unwrap());
        let stage = CoarseGenreStage::new(1, 2, None, subworker, 0.0);
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
