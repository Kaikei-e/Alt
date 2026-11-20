//! ThemesPipeline - builds and versions per-user theme pages from read articles.

use anyhow::{Context, Result};
use chrono::{DateTime, Utc};
use regex::Regex;
use serde::{Deserialize, Serialize};
use std::collections::{HashMap, HashSet};
use std::sync::{Arc, LazyLock};
use tracing::{error, info, warn};
use uuid::Uuid;

use super::adapters::{
    DataHubThemeArticleSource, NewsCreatorThemeGenerator, SovereignThemeKnowledgePort,
    SubworkerCardVerifier, SubworkerEmbedCluster,
};
use super::params::{
    MAX_FAILURE_NOTE_CHARS, MAX_FAILURE_NOTES, MAX_LEDE_CHARS, MAX_PREV_ENTRY_CHARS,
    MAX_PREV_KEY_POINTS, MAX_PREV_OPEN_QUESTIONS, MAX_TITLE_CHARS, ThemesParams,
};
use super::ports::{
    CardVerifier, EmbedCluster, ThemeArticleSource, ThemeGenerator, ThemeKnowledgePort,
    ThemesJobRunner,
};
use crate::clients::datahub::{DataHubArticle, DataHubClient};
use crate::clients::knowledge_sovereign::{KnowledgeSovereignClient, ReadingSourceEntry};
use crate::clients::news_creator::NewsCreatorClient;
use crate::clients::news_creator::models::{
    ThemeGenerateItemInput, ThemeGenerateOutcome, ThemeGenerateRequest, ThemeGenerateResponse,
    ThemePreviousInput, truncate_error_message,
};
use crate::clients::subworker::cards::{
    ClusterStoriesRequest, ClusterStoryItem, StoryClusterParams, SubworkerCardsClient,
    VerifyCardRequest, VerifyCardResponse, VerifyItemInput, VerifySentenceInput,
    VerifySentenceKind, VerifyThresholds,
};
use crate::config::KnowledgeOwnerIds;
use crate::language_detection::detect_text_language;
use crate::pipeline::cards::rank::compute_cluster_fingerprint;
use crate::pipeline::continuation::cosine_similarity;
use crate::pipeline::embed_cache::{EmbeddingCacheDao, embed_with_cache};
use crate::pipeline::preprocess::clean_html;
use crate::store::dao::impls::UnifiedDao;
use crate::store::dao::themes::{
    LatestThemeVersion, NewThemeGenerationAttempt, NewThemeVersion, ThemeAttemptOutcome, ThemeBody,
    ThemeClaim, ThemeRunFinish, ThemeRunStatus, ThemeSource, ThemeTimelineEntry,
    ThemeVersionDetail, ThemesDaoOps, UnpublishedThemeVersion,
};

#[expect(
    clippy::expect_used,
    reason = "compile-time literal regex is valid by construction"
)]
static TRAILING_CITATION_RE: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(r"(?:\s*\[\s*\d{1,2}(?:\s*,\s*\d{1,2})*\s*\])+\s*$")
        .expect("compile-time literal regex is valid by construction")
});

/// Outcome counters returned by a single themes pipeline run.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ThemesPipelineResult {
    pub run_id: Uuid,
    pub source_count: usize,
    pub articles_with_body: usize,
    pub missing_bodies: usize,
    pub empty_bodies: usize,
    pub clusters_eligible: usize,
    pub themes_unchanged: usize,
    pub versions_created: usize,
    pub versions_deferred: usize,
    pub generation_rejected: usize,
    pub verification_dropped: usize,
    pub publish_failures: usize,
    pub embed_cache_hits: usize,
    pub embed_cache_misses: usize,
}

/// Persistence interface required by `ThemesPipeline`.
#[async_trait::async_trait]
pub trait ThemesPipelineDao: EmbeddingCacheDao + Send + Sync {
    async fn create_run(
        &self,
        run_id: Uuid,
        user_id: Uuid,
        kicked_at: DateTime<Utc>,
        params_version: &str,
    ) -> Result<()>;

    async fn finish_run(&self, run_id: Uuid, user_id: Uuid, finish: &ThemeRunFinish) -> Result<()>;

    async fn latest_versions_for_user(&self, user_id: Uuid) -> Result<Vec<LatestThemeVersion>>;

    async fn get_theme_version(
        &self,
        user_id: Uuid,
        theme_id: Uuid,
        version: Option<i32>,
    ) -> Result<Option<ThemeVersionDetail>>;

    async fn insert_theme_version(&self, input: &NewThemeVersion) -> Result<()>;

    async fn unpublished_versions(&self, user_id: Uuid) -> Result<Vec<UnpublishedThemeVersion>>;

    async fn mark_published(
        &self,
        user_id: Uuid,
        version_id: Uuid,
        published_at: DateTime<Utc>,
    ) -> Result<()>;

    async fn generation_attempt_counts_for_user(
        &self,
        user_id: Uuid,
    ) -> Result<HashMap<(Uuid, String), usize>>;

    async fn insert_generation_attempt(&self, input: &NewThemeGenerationAttempt) -> Result<()>;
}

#[async_trait::async_trait]
impl EmbeddingCacheDao for UnifiedDao {
    async fn get_cached_embeddings(
        &self,
        model: &str,
        text_hashes: &[String],
    ) -> Result<HashMap<String, Vec<f32>>> {
        self.get_cached_embeddings(model, text_hashes)
            .await
            .map_err(|e| anyhow::anyhow!("{e}"))
    }

    async fn insert_embeddings(
        &self,
        model: &str,
        dim: usize,
        entries: &[(String, Vec<f32>)],
    ) -> Result<()> {
        self.insert_embeddings(model, dim, entries)
            .await
            .map_err(|e| anyhow::anyhow!("{e}"))
    }
}

#[async_trait::async_trait]
impl ThemesPipelineDao for UnifiedDao {
    async fn create_run(
        &self,
        run_id: Uuid,
        user_id: Uuid,
        kicked_at: DateTime<Utc>,
        params_version: &str,
    ) -> Result<()> {
        ThemesDaoOps::create_run(self.pool(), run_id, user_id, kicked_at, params_version)
            .await
            .map_err(|e| anyhow::anyhow!("{e}"))
    }

    async fn finish_run(&self, run_id: Uuid, user_id: Uuid, finish: &ThemeRunFinish) -> Result<()> {
        ThemesDaoOps::finish_run(self.pool(), run_id, user_id, finish)
            .await
            .map_err(|e| anyhow::anyhow!("{e}"))
    }

    async fn latest_versions_for_user(&self, user_id: Uuid) -> Result<Vec<LatestThemeVersion>> {
        ThemesDaoOps::latest_versions_for_user(self.pool(), user_id)
            .await
            .map_err(|e| anyhow::anyhow!("{e}"))
    }

    async fn get_theme_version(
        &self,
        user_id: Uuid,
        theme_id: Uuid,
        version: Option<i32>,
    ) -> Result<Option<ThemeVersionDetail>> {
        ThemesDaoOps::get_theme_version(self.pool(), user_id, theme_id, version)
            .await
            .map_err(|e| anyhow::anyhow!("{e}"))
    }

    async fn insert_theme_version(&self, input: &NewThemeVersion) -> Result<()> {
        ThemesDaoOps::insert_theme_version(self.pool(), input)
            .await
            .map_err(|e| anyhow::anyhow!("{e}"))
    }

    async fn unpublished_versions(&self, user_id: Uuid) -> Result<Vec<UnpublishedThemeVersion>> {
        ThemesDaoOps::unpublished_versions(self.pool(), user_id)
            .await
            .map_err(|e| anyhow::anyhow!("{e}"))
    }

    async fn mark_published(
        &self,
        user_id: Uuid,
        version_id: Uuid,
        published_at: DateTime<Utc>,
    ) -> Result<()> {
        ThemesDaoOps::mark_published(self.pool(), user_id, version_id, published_at)
            .await
            .map_err(|e| anyhow::anyhow!("{e}"))
    }

    async fn generation_attempt_counts_for_user(
        &self,
        user_id: Uuid,
    ) -> Result<HashMap<(Uuid, String), usize>> {
        ThemesDaoOps::generation_attempt_counts_for_user(self.pool(), user_id)
            .await
            .map_err(|e| anyhow::anyhow!("{e}"))
    }

    async fn insert_generation_attempt(&self, input: &NewThemeGenerationAttempt) -> Result<()> {
        ThemesDaoOps::insert_generation_attempt(self.pool(), input)
            .await
            .map_err(|e| anyhow::anyhow!("{e}"))
    }
}

/// Scheduled and manual job runner for the themes pipeline.
pub struct ThemesRunner {
    pipeline: ThemesPipeline,
    owner: KnowledgeOwnerIds,
}

impl ThemesRunner {
    #[must_use]
    pub fn new(pipeline: ThemesPipeline, owner: KnowledgeOwnerIds) -> Self {
        Self { pipeline, owner }
    }
}

#[async_trait::async_trait]
impl ThemesJobRunner for ThemesRunner {
    async fn run_themes(&self, run_id: Uuid, run_time: DateTime<Utc>) -> Result<()> {
        let result = self
            .pipeline
            .run(run_id, self.owner, run_time, &ThemesParams::default())
            .await?;
        info!(
            run_id = %result.run_id,
            user_id = %self.owner.user_id,
            source_count = result.source_count,
            articles_with_body = result.articles_with_body,
            missing_bodies = result.missing_bodies,
            empty_bodies = result.empty_bodies,
            clusters_eligible = result.clusters_eligible,
            themes_unchanged = result.themes_unchanged,
            versions_created = result.versions_created,
            versions_deferred = result.versions_deferred,
            generation_rejected = result.generation_rejected,
            verification_dropped = result.verification_dropped,
            publish_failures = result.publish_failures,
            embed_cache_hits = result.embed_cache_hits,
            embed_cache_misses = result.embed_cache_misses,
            "themes_run_finished"
        );
        Ok(())
    }
}

/// Dependencies required to construct a production `ThemesPipeline`.
pub(crate) struct ThemesPipelineClients {
    pub(crate) knowledge_client: Arc<KnowledgeSovereignClient>,
    pub(crate) datahub_client: Arc<DataHubClient>,
    pub(crate) subworker_client: Arc<SubworkerCardsClient>,
    pub(crate) news_creator_client: Arc<NewsCreatorClient>,
    pub(crate) pool: sqlx::PgPool,
}

/// Pipeline orchestrating theme clustering, generation, verification, and event publishing.
pub struct ThemesPipeline {
    knowledge: Arc<dyn ThemeKnowledgePort>,
    article_source: Arc<dyn ThemeArticleSource>,
    ml_port: Arc<dyn EmbedCluster>,
    generator: Arc<dyn ThemeGenerator>,
    verifier: Arc<dyn CardVerifier>,
    dao: Arc<dyn ThemesPipelineDao>,
}

#[derive(Debug, Clone)]
struct PreparedArticle {
    id: Uuid,
    title: String,
    url: String,
    published_at: Option<DateTime<Utc>>,
    published_date: Option<String>,
    lede: String,
    cluster_pub_at: DateTime<Utc>,
}

#[derive(Debug, Clone)]
struct EligibleCluster {
    member_ids: Vec<Uuid>,
    membership_hash: String,
    centroid: Vec<f32>,
}

#[derive(Debug, Clone)]
struct ThemeCandidate {
    theme_id: Uuid,
    cluster: EligibleCluster,
    matched_latest: Option<LatestThemeVersion>,
}

#[derive(Debug, Clone)]
struct SelectedItem {
    n: usize,
    article: PreparedArticle,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum VerifySection {
    KeyPoint,
    Timeline,
    WhatChanged,
    OpenQuestion,
}

#[derive(Debug, Default)]
struct RunProgress {
    source_count: usize,
    versions_created: usize,
    versions_deferred: usize,
    generation_rejected: usize,
    verification_dropped: usize,
    publish_failures: usize,
}

impl ThemesPipeline {
    #[allow(private_interfaces)]
    #[must_use]
    pub fn new(
        knowledge: Arc<dyn ThemeKnowledgePort>,
        article_source: Arc<dyn ThemeArticleSource>,
        ml_port: Arc<dyn EmbedCluster>,
        generator: Arc<dyn ThemeGenerator>,
        verifier: Arc<dyn CardVerifier>,
        dao: Arc<dyn ThemesPipelineDao>,
    ) -> Self {
        Self {
            knowledge,
            article_source,
            ml_port,
            generator,
            verifier,
            dao,
        }
    }

    pub(crate) fn from_clients(clients: ThemesPipelineClients, params: &ThemesParams) -> Self {
        Self {
            knowledge: Arc::new(SovereignThemeKnowledgePort::new(clients.knowledge_client)),
            article_source: Arc::new(DataHubThemeArticleSource::new(clients.datahub_client)),
            ml_port: Arc::new(SubworkerEmbedCluster::with_expected(
                Arc::clone(&clients.subworker_client),
                &params.expected_embed_model,
                params.expected_embed_dim,
            )),
            generator: Arc::new(NewsCreatorThemeGenerator::new(clients.news_creator_client)),
            verifier: Arc::new(SubworkerCardVerifier::new(clients.subworker_client)),
            dao: Arc::new(UnifiedDao::new(clients.pool)),
        }
    }

    /// Execute one run for the configured knowledge owner at `run_time`.
    pub async fn run(
        &self,
        run_id: Uuid,
        owner: KnowledgeOwnerIds,
        run_time: DateTime<Utc>,
        params: &ThemesParams,
    ) -> Result<ThemesPipelineResult> {
        if owner.user_id.is_nil() || owner.tenant_id.is_nil() {
            anyhow::bail!("themes run requires non-nil owner user_id and tenant_id");
        }

        self.dao
            .create_run(run_id, owner.user_id, run_time, params.version())
            .await
            .context("failed to create read_theme_runs row")?;

        let mut progress = RunProgress::default();
        match self
            .execute_run(run_id, owner, run_time, params, &mut progress)
            .await
        {
            Ok(result) => {
                self.dao
                    .finish_run(
                        run_id,
                        owner.user_id,
                        &ThemeRunFinish {
                            status: ThemeRunStatus::Completed,
                            finished_at: run_time,
                            source_count: i32::try_from(result.source_count).unwrap_or(i32::MAX),
                            versions_created: i32::try_from(result.versions_created)
                                .unwrap_or(i32::MAX),
                            versions_deferred: i32::try_from(result.versions_deferred)
                                .unwrap_or(i32::MAX),
                            generation_rejected: i32::try_from(result.generation_rejected)
                                .unwrap_or(i32::MAX),
                            verification_dropped: i32::try_from(result.verification_dropped)
                                .unwrap_or(i32::MAX),
                            publish_failures: i32::try_from(result.publish_failures)
                                .unwrap_or(i32::MAX),
                            error_message: None,
                        },
                    )
                    .await
                    .context("failed to finish completed read_theme_runs row")?;
                Ok(result)
            }
            Err(err) => {
                let short_err = truncate_error_message(&format!("{err:#}"));
                if let Err(finish_err) = self
                    .dao
                    .finish_run(
                        run_id,
                        owner.user_id,
                        &ThemeRunFinish {
                            status: ThemeRunStatus::Failed,
                            finished_at: run_time,
                            source_count: i32::try_from(progress.source_count).unwrap_or(i32::MAX),
                            versions_created: i32::try_from(progress.versions_created)
                                .unwrap_or(i32::MAX),
                            versions_deferred: i32::try_from(progress.versions_deferred)
                                .unwrap_or(i32::MAX),
                            generation_rejected: i32::try_from(progress.generation_rejected)
                                .unwrap_or(i32::MAX),
                            verification_dropped: i32::try_from(progress.verification_dropped)
                                .unwrap_or(i32::MAX),
                            publish_failures: i32::try_from(progress.publish_failures)
                                .unwrap_or(i32::MAX),
                            error_message: Some(short_err),
                        },
                    )
                    .await
                {
                    error!(
                        %run_id,
                        original_error = %err,
                        finish_error = %finish_err,
                        "failed to record failed read_theme_runs row"
                    );
                }
                Err(err)
            }
        }
    }

    async fn execute_run(
        &self,
        run_id: Uuid,
        owner: KnowledgeOwnerIds,
        run_time: DateTime<Utc>,
        params: &ThemesParams,
        progress: &mut RunProgress,
    ) -> Result<ThemesPipelineResult> {
        let initial_publish_failures = self
            .publish_pending_versions(run_id, owner, run_time)
            .await?;
        progress.publish_failures = initial_publish_failures;

        let (source_count, articles, missing_bodies, empty_bodies) = match self
            .load_prepared_articles(
                run_id,
                owner.user_id,
                run_time,
                params.min_theme_articles,
                progress,
            )
            .await?
        {
            PreparedCorpusOutcome::Ready {
                source_count,
                articles,
                missing_bodies,
                empty_bodies,
            } => (source_count, articles, missing_bodies, empty_bodies),
            PreparedCorpusOutcome::EarlyExit(result) => return Ok(result),
        };
        let articles_with_body = articles.len();

        let (
            eligible_clusters,
            article_by_id,
            embedding_by_id,
            embed_cache_hits,
            embed_cache_misses,
        ) = self
            .embed_and_cluster_articles(run_id, articles, params)
            .await?;
        let clusters_eligible = eligible_clusters.len();

        let (ranked_candidates, themes_unchanged, versions_deferred) = self
            .rank_changed_candidates(run_id, owner.user_id, &eligible_clusters, params)
            .await?;
        progress.versions_deferred = versions_deferred;

        let candidate_ctx = CandidateRunContext {
            run_id,
            owner,
            run_time,
            params,
            article_by_id: &article_by_id,
            embedding_by_id: &embedding_by_id,
        };
        for (_, candidate) in ranked_candidates {
            let outcome = self.process_candidate(&candidate_ctx, candidate).await?;
            match outcome {
                CandidateOutcome::Created { publish_failed } => {
                    progress.versions_created += 1;
                    if publish_failed {
                        progress.publish_failures += 1;
                    }
                }
                CandidateOutcome::GenerationRejected => progress.generation_rejected += 1,
                CandidateOutcome::VerificationDropped => progress.verification_dropped += 1,
            }
        }

        Ok(ThemesPipelineResult {
            run_id,
            source_count,
            articles_with_body,
            missing_bodies,
            empty_bodies,
            clusters_eligible,
            themes_unchanged,
            versions_created: progress.versions_created,
            versions_deferred,
            generation_rejected: progress.generation_rejected,
            verification_dropped: progress.verification_dropped,
            publish_failures: progress.publish_failures,
            embed_cache_hits,
            embed_cache_misses,
        })
    }

    async fn load_prepared_articles(
        &self,
        run_id: Uuid,
        user_id: Uuid,
        run_time: DateTime<Utc>,
        min_theme_articles: usize,
        progress: &mut RunProgress,
    ) -> Result<PreparedCorpusOutcome> {
        let raw_sources = self
            .knowledge
            .list_reading_sources_in_window(user_id, DateTime::<Utc>::UNIX_EPOCH, run_time)
            .await
            .context("failed to list reading sources")?;

        let source_first_read = collect_first_read_dates(raw_sources);
        let source_count = source_first_read.len();
        progress.source_count = source_count;
        if source_count < min_theme_articles {
            info!(
                %run_id,
                source_count,
                min_theme_articles,
                "fewer than minimum sources, completing run with zero counts"
            );
            return Ok(PreparedCorpusOutcome::EarlyExit(early_exit_result(
                run_id,
                source_count,
                0,
                0,
                0,
                progress.publish_failures,
            )));
        }

        let mut article_ids: Vec<Uuid> = source_first_read.keys().copied().collect();
        article_ids.sort();

        let fetched_articles = self
            .article_source
            .batch_get_articles_by_ids(&article_ids)
            .await
            .context("failed to fetch article bodies from datahub")?;

        let (articles, missing_bodies, empty_bodies) =
            prepare_articles(&article_ids, fetched_articles, &source_first_read);
        let articles_with_body = articles.len();
        info!(
            %run_id,
            source_count,
            articles_with_body,
            missing_bodies,
            empty_bodies,
            "themes article bodies prepared"
        );

        if articles_with_body < min_theme_articles {
            return Ok(PreparedCorpusOutcome::EarlyExit(early_exit_result(
                run_id,
                source_count,
                articles_with_body,
                missing_bodies,
                empty_bodies,
                progress.publish_failures,
            )));
        }

        Ok(PreparedCorpusOutcome::Ready {
            source_count,
            articles,
            missing_bodies,
            empty_bodies,
        })
    }

    async fn embed_and_cluster_articles(
        &self,
        run_id: Uuid,
        articles: Vec<PreparedArticle>,
        params: &ThemesParams,
    ) -> Result<(
        Vec<EligibleCluster>,
        HashMap<Uuid, PreparedArticle>,
        HashMap<Uuid, Vec<f32>>,
        usize,
        usize,
    )> {
        let texts: Vec<String> = articles
            .iter()
            .map(|a| format!("{} — {}", a.title, a.lede))
            .collect();
        let (embeddings, embed_cache_hits, embed_cache_misses) = embed_with_cache(
            self.dao.as_ref(),
            self.ml_port.as_ref(),
            run_id,
            &texts,
            &params.expected_embed_model,
            params.expected_embed_dim,
        )
        .await?;

        let mut article_by_id: HashMap<Uuid, PreparedArticle> =
            HashMap::with_capacity(articles.len());
        let mut embedding_by_id: HashMap<Uuid, Vec<f32>> = HashMap::with_capacity(articles.len());
        let mut cluster_items = Vec::with_capacity(articles.len());
        for (article, emb) in articles.into_iter().zip(embeddings) {
            let lang = detect_text_language(&format!("{} {}", article.title, article.lede));
            cluster_items.push(ClusterStoryItem {
                id: article.id,
                embedding: emb.clone(),
                published_at: article.cluster_pub_at,
                language: Some(lang.to_string()),
            });
            embedding_by_id.insert(article.id, emb);
            article_by_id.insert(article.id, article);
        }

        let cluster_req = ClusterStoriesRequest {
            items: cluster_items,
            params: StoryClusterParams {
                threshold: params.threshold,
                linkage: params.linkage.clone(),
                time_decay_per_day: params.time_decay_per_day,
                min_cluster_size: params.min_theme_articles,
                min_cluster_size_by_language: None,
            },
        };
        let cluster_resp = self
            .ml_port
            .cluster_stories(&cluster_req)
            .await
            .context("failed to cluster theme articles")?;

        let eligible_clusters = build_eligible_clusters(
            &cluster_resp.clusters,
            &article_by_id,
            params.min_theme_articles,
        );

        Ok((
            eligible_clusters,
            article_by_id,
            embedding_by_id,
            embed_cache_hits,
            embed_cache_misses,
        ))
    }

    async fn rank_changed_candidates(
        &self,
        run_id: Uuid,
        user_id: Uuid,
        eligible_clusters: &[EligibleCluster],
        params: &ThemesParams,
    ) -> Result<(Vec<(usize, ThemeCandidate)>, usize, usize)> {
        let latest_versions = self
            .dao
            .latest_versions_for_user(user_id)
            .await
            .context("failed to fetch latest theme versions for user")?;

        let assignments = assign_clusters_to_themes(
            eligible_clusters,
            &latest_versions,
            params.theta_continuation,
            run_id,
        )?;

        let mut themes_unchanged = 0usize;
        let mut candidates = Vec::new();
        for assignment in assignments {
            if let Some(ref prev) = assignment.matched_latest {
                if assignment.cluster.membership_hash == prev.membership_hash {
                    themes_unchanged += 1;
                    continue;
                }
            }
            candidates.push(assignment);
        }

        let prior_attempts = if candidates.is_empty() {
            HashMap::new()
        } else {
            self.dao
                .generation_attempt_counts_for_user(user_id)
                .await
                .context("failed to query prior theme generation attempts")?
        };

        let mut ranked_candidates: Vec<(usize, ThemeCandidate)> = candidates
            .into_iter()
            .map(|candidate| {
                let attempts = prior_attempts
                    .get(&(
                        candidate.theme_id,
                        candidate.cluster.membership_hash.clone(),
                    ))
                    .copied()
                    .unwrap_or(0);
                (attempts, candidate)
            })
            .collect();

        ranked_candidates.sort_by(|(a_attempts, a), (b_attempts, b)| {
            a_attempts
                .cmp(b_attempts)
                .then_with(|| b.cluster.member_ids.len().cmp(&a.cluster.member_ids.len()))
                .then_with(|| a.theme_id.cmp(&b.theme_id))
        });

        let versions_deferred = ranked_candidates
            .len()
            .saturating_sub(params.max_generations_per_run);
        ranked_candidates.truncate(params.max_generations_per_run);

        Ok((ranked_candidates, themes_unchanged, versions_deferred))
    }

    async fn publish_pending_versions(
        &self,
        run_id: Uuid,
        owner: KnowledgeOwnerIds,
        run_time: DateTime<Utc>,
    ) -> Result<usize> {
        let unpublished = self
            .dao
            .unpublished_versions(owner.user_id)
            .await
            .context("failed to query unpublished theme versions")?;
        if unpublished.is_empty() {
            return Ok(0);
        }

        let outcome = self
            .knowledge
            .publish_theme_versions(owner.user_id, owner.tenant_id, &unpublished)
            .await;

        for version_id in &outcome.succeeded_version_ids {
            self.dao
                .mark_published(owner.user_id, *version_id, run_time)
                .await
                .with_context(|| format!("failed to mark theme version {version_id} published"))?;
        }

        if outcome.failed > 0 {
            error!(
                %run_id,
                user_id = %owner.user_id,
                succeeded = outcome.succeeded_version_ids.len(),
                failed = outcome.failed,
                "failed to publish pending theme versions before run"
            );
        }

        Ok(outcome.failed)
    }

    async fn load_previous_theme_detail(
        &self,
        user_id: Uuid,
        matched_latest: Option<&LatestThemeVersion>,
    ) -> Result<Option<ThemeVersionDetail>> {
        let Some(latest) = matched_latest else {
            return Ok(None);
        };
        let detail = self
            .dao
            .get_theme_version(user_id, latest.theme_id, Some(latest.version))
            .await
            .context("failed to load previous theme version detail")?
            .with_context(|| {
                format!(
                    "missing previous theme version {} for theme {}",
                    latest.version, latest.theme_id
                )
            })?;
        Ok(Some(detail))
    }

    async fn generate_or_record_rejection(
        &self,
        ctx: &CandidateRunContext<'_>,
        candidate: &ThemeCandidate,
        gen_req: &ThemeGenerateRequest,
        phase: GenerationPhase,
    ) -> Result<Option<ThemeGenerateResponse>> {
        let theme_id = candidate.theme_id;
        let run_id = ctx.run_id;
        let outcome =
            self.generator
                .generate_theme(gen_req)
                .await
                .with_context(|| match phase {
                    GenerationPhase::Initial => {
                        format!("theme generation failed for theme {theme_id}")
                    }
                    GenerationPhase::Regeneration => {
                        format!("theme regeneration failed for theme {theme_id}")
                    }
                })?;
        match outcome {
            ThemeGenerateOutcome::Success(resp) => Ok(Some(resp)),
            ThemeGenerateOutcome::Rejected(rej) => {
                match phase {
                    GenerationPhase::Initial => warn!(
                        %run_id,
                        %theme_id,
                        reason = %rej.reason,
                        attempts = rej.attempts,
                        "theme generation rejected, skipping theme"
                    ),
                    GenerationPhase::Regeneration => warn!(
                        %run_id,
                        %theme_id,
                        reason = %rej.reason,
                        attempts = rej.attempts,
                        "theme regeneration rejected, skipping theme"
                    ),
                }
                self.record_generation_attempt(
                    run_id,
                    ctx.owner.user_id,
                    theme_id,
                    &candidate.cluster.membership_hash,
                    ThemeAttemptOutcome::GenerationRejected,
                    ctx.run_time,
                )
                .await?;
                Ok(None)
            }
        }
    }

    async fn verify_and_maybe_regenerate(
        &self,
        ctx: &CandidateRunContext<'_>,
        candidate: &ThemeCandidate,
        selected_items: &[SelectedItem],
        gen_req: &mut ThemeGenerateRequest,
        mut generated: ThemeGenerateResponse,
    ) -> Result<
        Option<(
            ThemeGenerateResponse,
            Vec<(VerifySection, usize)>,
            VerifyCardResponse,
        )>,
    > {
        let theme_id = candidate.theme_id;
        strip_response_citation_markers(&mut generated);
        let (verify_req, mut sentence_map) = build_verify_request(
            ctx.run_id,
            theme_id,
            &generated,
            selected_items,
            ctx.params.tau_a,
        );
        let mut verify_resp = self.verify_sentences(&verify_req, theme_id).await?;

        let failure_notes = build_failure_notes(&verify_resp, ctx.params.tau_a);
        if !failure_notes.is_empty() {
            gen_req.failure_notes = failure_notes;
            let Some(regenerated) = self
                .generate_or_record_rejection(
                    ctx,
                    candidate,
                    gen_req,
                    GenerationPhase::Regeneration,
                )
                .await?
            else {
                return Ok(None);
            };
            generated = regenerated;
            strip_response_citation_markers(&mut generated);
            let rebuilt = build_verify_request(
                ctx.run_id,
                theme_id,
                &generated,
                selected_items,
                ctx.params.tau_a,
            );
            sentence_map = rebuilt.1;
            verify_resp = self.verify_sentences(&rebuilt.0, theme_id).await?;
        }

        Ok(Some((generated, sentence_map, verify_resp)))
    }

    async fn persist_and_publish_version(
        &self,
        ctx: &CandidateRunContext<'_>,
        candidate: ThemeCandidate,
        generated_title: &str,
        body: ThemeBody,
    ) -> Result<bool> {
        let run_id = ctx.run_id;
        let theme_id = candidate.theme_id;
        let next_version = candidate
            .matched_latest
            .as_ref()
            .map_or(1, |prev| prev.version + 1);
        let version_id = Uuid::new_v4();
        let title = clip_chars(generated_title.trim(), MAX_TITLE_CHARS);
        let new_version = NewThemeVersion {
            version_id,
            user_id: ctx.owner.user_id,
            theme_id,
            version: next_version,
            run_id,
            title,
            member_article_ids: candidate.cluster.member_ids,
            membership_hash: candidate.cluster.membership_hash,
            centroid: candidate.cluster.centroid,
            body,
            created_at: ctx.run_time,
        };

        self.dao
            .insert_theme_version(&new_version)
            .await
            .context("failed to insert new theme version")?;

        let unpublished = UnpublishedThemeVersion {
            version_id,
            user_id: ctx.owner.user_id,
            theme_id,
            version: next_version,
            title: new_version.title,
            member_article_ids: new_version.member_article_ids,
            created_at: ctx.run_time,
        };
        let pub_outcome = self
            .knowledge
            .publish_theme_versions(ctx.owner.user_id, ctx.owner.tenant_id, &[unpublished])
            .await;

        if pub_outcome.succeeded_version_ids.contains(&version_id) {
            self.dao
                .mark_published(ctx.owner.user_id, version_id, ctx.run_time)
                .await
                .context("failed to mark new theme version published")?;
            Ok(false)
        } else {
            error!(
                %run_id,
                %theme_id,
                %version_id,
                version = next_version,
                "failed to publish theme version event; leaving version unpublished for retry"
            );
            Ok(true)
        }
    }

    async fn process_candidate(
        &self,
        ctx: &CandidateRunContext<'_>,
        candidate: ThemeCandidate,
    ) -> Result<CandidateOutcome> {
        let theme_id = candidate.theme_id;
        let run_id = ctx.run_id;
        let prev_detail = self
            .load_previous_theme_detail(ctx.owner.user_id, candidate.matched_latest.as_ref())
            .await?;

        let (selected_items, new_item_ns) = select_candidate_items(
            &candidate.cluster.member_ids,
            &candidate.cluster.centroid,
            prev_detail
                .as_ref()
                .map(|d| d.member_article_ids.as_slice()),
            ctx.article_by_id,
            ctx.embedding_by_id,
            ctx.params.max_items_per_generation,
        )?;

        let mut gen_req = build_theme_generate_request(
            run_id,
            theme_id,
            &selected_items,
            new_item_ns,
            &candidate,
        );

        let Some(generated) = self
            .generate_or_record_rejection(ctx, &candidate, &gen_req, GenerationPhase::Initial)
            .await?
        else {
            return Ok(CandidateOutcome::GenerationRejected);
        };

        let Some((generated, sentence_map, verify_resp)) = self
            .verify_and_maybe_regenerate(ctx, &candidate, &selected_items, &mut gen_req, generated)
            .await?
        else {
            return Ok(CandidateOutcome::GenerationRejected);
        };

        let body =
            build_verified_theme_body(&generated, &selected_items, &sentence_map, &verify_resp);
        if body.key_points.is_empty() {
            warn!(
                %run_id,
                %theme_id,
                "all key points failed verification, skipping theme version"
            );
            self.record_generation_attempt(
                run_id,
                ctx.owner.user_id,
                theme_id,
                &candidate.cluster.membership_hash,
                ThemeAttemptOutcome::VerificationDropped,
                ctx.run_time,
            )
            .await?;
            return Ok(CandidateOutcome::VerificationDropped);
        }

        let publish_failed = self
            .persist_and_publish_version(ctx, candidate, &generated.title, body)
            .await?;

        Ok(CandidateOutcome::Created { publish_failed })
    }

    async fn record_generation_attempt(
        &self,
        run_id: Uuid,
        user_id: Uuid,
        theme_id: Uuid,
        membership_hash: &str,
        outcome: ThemeAttemptOutcome,
        created_at: DateTime<Utc>,
    ) -> Result<()> {
        self.dao
            .insert_generation_attempt(&NewThemeGenerationAttempt {
                attempt_id: Uuid::new_v4(),
                run_id,
                user_id,
                theme_id,
                membership_hash: membership_hash.to_string(),
                outcome,
                created_at,
            })
            .await
            .with_context(|| {
                format!("failed to record theme generation attempt for theme {theme_id}")
            })
    }

    async fn verify_sentences(
        &self,
        verify_req: &VerifyCardRequest,
        theme_id: Uuid,
    ) -> Result<VerifyCardResponse> {
        if verify_req.sentences.is_empty() {
            return Ok(VerifyCardResponse {
                sentences: Vec::new(),
                why_hint_present: false,
                embedding: crate::clients::subworker::cards::VerifyEmbeddingInfo {
                    model: String::new(),
                    identity: String::new(),
                },
            });
        }
        self.verifier
            .verify_card(verify_req)
            .await
            .with_context(|| format!("theme verification failed for theme {theme_id}"))
    }
}

struct CandidateRunContext<'a> {
    run_id: Uuid,
    owner: KnowledgeOwnerIds,
    run_time: DateTime<Utc>,
    params: &'a ThemesParams,
    article_by_id: &'a HashMap<Uuid, PreparedArticle>,
    embedding_by_id: &'a HashMap<Uuid, Vec<f32>>,
}

#[derive(Clone, Copy)]
enum GenerationPhase {
    Initial,
    Regeneration,
}

fn build_theme_generate_request(
    run_id: Uuid,
    theme_id: Uuid,
    selected_items: &[SelectedItem],
    new_item_ns: Vec<usize>,
    candidate: &ThemeCandidate,
) -> ThemeGenerateRequest {
    let items = selected_items
        .iter()
        .map(|it| ThemeGenerateItemInput {
            n: it.n,
            title: it.article.title.clone(),
            published_at: it.article.published_date.clone(),
            text: it.article.lede.clone(),
        })
        .collect();
    let previous = candidate.matched_latest.as_ref().map(build_previous_input);
    ThemeGenerateRequest {
        job_id: run_id,
        theme_id,
        language: "ja".to_string(),
        items,
        new_item_ns,
        previous,
        failure_notes: Vec::new(),
    }
}

enum PreparedCorpusOutcome {
    Ready {
        source_count: usize,
        articles: Vec<PreparedArticle>,
        missing_bodies: usize,
        empty_bodies: usize,
    },
    EarlyExit(ThemesPipelineResult),
}

enum CandidateOutcome {
    Created { publish_failed: bool },
    GenerationRejected,
    VerificationDropped,
}

#[must_use]
pub(crate) fn clip_chars(value: &str, max_chars: usize) -> String {
    value.chars().take(max_chars).collect()
}

fn collect_first_read_dates(raw_sources: Vec<ReadingSourceEntry>) -> HashMap<Uuid, DateTime<Utc>> {
    let mut source_first_read: HashMap<Uuid, DateTime<Utc>> = HashMap::new();
    for entry in raw_sources {
        source_first_read
            .entry(entry.article_id)
            .and_modify(|ts| {
                if entry.first_read_at < *ts {
                    *ts = entry.first_read_at;
                }
            })
            .or_insert(entry.first_read_at);
    }
    source_first_read
}

fn early_exit_result(
    run_id: Uuid,
    source_count: usize,
    articles_with_body: usize,
    missing_bodies: usize,
    empty_bodies: usize,
    publish_failures: usize,
) -> ThemesPipelineResult {
    ThemesPipelineResult {
        run_id,
        source_count,
        articles_with_body,
        missing_bodies,
        empty_bodies,
        clusters_eligible: 0,
        themes_unchanged: 0,
        versions_created: 0,
        versions_deferred: 0,
        generation_rejected: 0,
        verification_dropped: 0,
        publish_failures,
        embed_cache_hits: 0,
        embed_cache_misses: 0,
    }
}

#[must_use]
pub(crate) fn strip_trailing_citation_markers(text: &str) -> String {
    let trimmed = text.trim();
    TRAILING_CITATION_RE
        .replace(trimmed, "")
        .trim_end()
        .to_string()
}

fn prepare_articles(
    article_ids: &[Uuid],
    fetched: Vec<DataHubArticle>,
    first_read_by_id: &HashMap<Uuid, DateTime<Utc>>,
) -> (Vec<PreparedArticle>, usize, usize) {
    let mut by_id: HashMap<Uuid, DataHubArticle> = HashMap::with_capacity(fetched.len());
    for article in fetched {
        by_id.insert(article.id, article);
    }

    let mut prepared = Vec::with_capacity(article_ids.len());
    let mut missing_bodies = 0usize;
    let mut empty_bodies = 0usize;

    for id in article_ids {
        let Some(article) = by_id.get(id) else {
            missing_bodies += 1;
            continue;
        };

        let cleaned = match clean_html(&article.content) {
            Ok((plain, _)) => plain,
            Err(err) => {
                warn!(article_id = %id, error = %err, "clean_html failed on article body, dropping");
                empty_bodies += 1;
                continue;
            }
        };

        let trimmed_body = cleaned.trim();
        if trimmed_body.is_empty() {
            empty_bodies += 1;
            continue;
        }

        let lede = clip_chars(trimmed_body, MAX_LEDE_CHARS).trim().to_string();
        if lede.is_empty() {
            empty_bodies += 1;
            continue;
        }

        let raw_title = clip_chars(article.title.trim(), MAX_TITLE_CHARS)
            .trim()
            .to_string();
        let title = if raw_title.is_empty() {
            let fallback = clip_chars(article.url.trim(), MAX_TITLE_CHARS)
                .trim()
                .to_string();
            if fallback.is_empty() {
                "Untitled".to_string()
            } else {
                fallback
            }
        } else {
            raw_title
        };

        let published_date = article
            .published_at
            .map(|dt| dt.format("%Y-%m-%d").to_string());
        let cluster_pub_at = article
            .published_at
            .or_else(|| first_read_by_id.get(id).copied())
            .unwrap_or(DateTime::<Utc>::UNIX_EPOCH);

        prepared.push(PreparedArticle {
            id: *id,
            title,
            url: article.url.clone(),
            published_at: article.published_at,
            published_date,
            lede,
            cluster_pub_at,
        });
    }

    (prepared, missing_bodies, empty_bodies)
}

fn build_eligible_clusters(
    clusters: &[crate::clients::subworker::cards::ClusterOutput],
    article_by_id: &HashMap<Uuid, PreparedArticle>,
    min_theme_articles: usize,
) -> Vec<EligibleCluster> {
    let mut eligible = Vec::new();
    for c in clusters {
        let mut seen = HashSet::new();
        let mut member_ids: Vec<Uuid> = c
            .member_ids
            .iter()
            .copied()
            .filter(|id| article_by_id.contains_key(id) && seen.insert(*id))
            .collect();
        member_ids.sort();
        if member_ids.len() < min_theme_articles {
            continue;
        }
        let membership_hash = compute_cluster_fingerprint(&member_ids);
        eligible.push(EligibleCluster {
            member_ids,
            membership_hash,
            centroid: c.centroid.clone(),
        });
    }
    eligible
}

#[derive(Debug)]
struct MatchPair {
    cluster_idx: usize,
    theme_idx: usize,
    similarity: f32,
    theme_id: Uuid,
    membership_hash: String,
}

fn assign_clusters_to_themes(
    clusters: &[EligibleCluster],
    latest_versions: &[LatestThemeVersion],
    theta_continuation: f32,
    run_id: Uuid,
) -> Result<Vec<ThemeCandidate>> {
    let mut pairs = Vec::new();
    for (cluster_idx, cluster) in clusters.iter().enumerate() {
        for (theme_idx, theme) in latest_versions.iter().enumerate() {
            let similarity = cosine_similarity(&cluster.centroid, &theme.centroid)?;
            if similarity >= theta_continuation {
                pairs.push(MatchPair {
                    cluster_idx,
                    theme_idx,
                    similarity,
                    theme_id: theme.theme_id,
                    membership_hash: cluster.membership_hash.clone(),
                });
            }
        }
    }

    pairs.sort_by(|a, b| {
        b.similarity
            .total_cmp(&a.similarity)
            .then_with(|| a.theme_id.cmp(&b.theme_id))
            .then_with(|| a.membership_hash.cmp(&b.membership_hash))
    });

    let mut matched_cluster_to_theme: HashMap<usize, usize> = HashMap::new();
    let mut used_themes: HashSet<usize> = HashSet::new();
    for pair in pairs {
        if matched_cluster_to_theme.contains_key(&pair.cluster_idx)
            || used_themes.contains(&pair.theme_idx)
        {
            continue;
        }
        matched_cluster_to_theme.insert(pair.cluster_idx, pair.theme_idx);
        used_themes.insert(pair.theme_idx);
    }

    let existing_theme_ids: HashSet<Uuid> = latest_versions.iter().map(|t| t.theme_id).collect();
    let mut assigned_new_ids: HashSet<Uuid> = HashSet::new();

    let mut out = Vec::with_capacity(clusters.len());
    for (cluster_idx, cluster) in clusters.iter().enumerate() {
        if let Some(&theme_idx) = matched_cluster_to_theme.get(&cluster_idx) {
            let matched = &latest_versions[theme_idx];
            out.push(ThemeCandidate {
                theme_id: matched.theme_id,
                cluster: cluster.clone(),
                matched_latest: Some(matched.clone()),
            });
        } else {
            let base_id = Uuid::new_v5(&Uuid::NAMESPACE_OID, cluster.membership_hash.as_bytes());
            let theme_id =
                if existing_theme_ids.contains(&base_id) || assigned_new_ids.contains(&base_id) {
                    let combined = format!("{}:{}", cluster.membership_hash, run_id);
                    Uuid::new_v5(&Uuid::NAMESPACE_OID, combined.as_bytes())
                } else {
                    base_id
                };
            assigned_new_ids.insert(theme_id);
            out.push(ThemeCandidate {
                theme_id,
                cluster: cluster.clone(),
                matched_latest: None,
            });
        }
    }

    Ok(out)
}

fn select_candidate_items(
    member_ids: &[Uuid],
    centroid: &[f32],
    prev_member_ids: Option<&[Uuid]>,
    article_by_id: &HashMap<Uuid, PreparedArticle>,
    embedding_by_id: &HashMap<Uuid, Vec<f32>>,
    max_items: usize,
) -> Result<(Vec<SelectedItem>, Vec<usize>)> {
    let mut scored_members: Vec<(PreparedArticle, f32)> = Vec::with_capacity(member_ids.len());
    for id in member_ids {
        let article = article_by_id
            .get(id)
            .with_context(|| format!("missing prepared article {id}"))?
            .clone();
        let emb = embedding_by_id
            .get(id)
            .with_context(|| format!("missing embedding for article {id}"))?;
        let sim = cosine_similarity(emb, centroid)?;
        scored_members.push((article, sim));
    }

    let (ordered, new_count) = if let Some(prev_ids) = prev_member_ids {
        let prev_set: HashSet<Uuid> = prev_ids.iter().copied().collect();
        let mut not_in_prev: Vec<(PreparedArticle, f32)> = Vec::new();
        let mut in_prev: Vec<(PreparedArticle, f32)> = Vec::new();
        for entry in scored_members {
            if prev_set.contains(&entry.0.id) {
                in_prev.push(entry);
            } else {
                not_in_prev.push(entry);
            }
        }
        not_in_prev.sort_by(|a, b| {
            b.0.published_at
                .cmp(&a.0.published_at)
                .then_with(|| a.0.id.cmp(&b.0.id))
        });
        in_prev.sort_by(|a, b| b.1.total_cmp(&a.1).then_with(|| a.0.id.cmp(&b.0.id)));
        let not_in_prev_len = not_in_prev.len();
        not_in_prev.extend(in_prev);
        not_in_prev.truncate(max_items);
        let new_count = not_in_prev_len.min(not_in_prev.len());
        (not_in_prev, new_count)
    } else {
        scored_members.sort_by(|a, b| b.1.total_cmp(&a.1).then_with(|| a.0.id.cmp(&b.0.id)));
        scored_members.truncate(max_items);
        (scored_members, 0)
    };

    let selected: Vec<SelectedItem> = ordered
        .into_iter()
        .enumerate()
        .map(|(i, (article, _))| SelectedItem { n: i + 1, article })
        .collect();
    let new_item_ns: Vec<usize> = (1..=new_count).collect();
    Ok((selected, new_item_ns))
}

fn build_previous_input(latest: &LatestThemeVersion) -> ThemePreviousInput {
    let title = clip_chars(latest.title.trim(), MAX_TITLE_CHARS)
        .trim()
        .to_string();
    let key_points = latest
        .key_point_texts
        .iter()
        .map(|t| {
            clip_chars(t.trim(), MAX_PREV_ENTRY_CHARS)
                .trim()
                .to_string()
        })
        .filter(|t| !t.is_empty())
        .take(MAX_PREV_KEY_POINTS)
        .collect();
    let open_questions = latest
        .open_question_texts
        .iter()
        .map(|t| {
            clip_chars(t.trim(), MAX_PREV_ENTRY_CHARS)
                .trim()
                .to_string()
        })
        .filter(|t| !t.is_empty())
        .take(MAX_PREV_OPEN_QUESTIONS)
        .collect();

    ThemePreviousInput {
        title,
        key_points,
        open_questions,
    }
}

fn strip_response_citation_markers(resp: &mut ThemeGenerateResponse) {
    for kp in &mut resp.key_points {
        kp.text = strip_trailing_citation_markers(&kp.text);
    }
    for tl in &mut resp.timeline {
        tl.text = strip_trailing_citation_markers(&tl.text);
    }
    for wc in &mut resp.what_changed {
        wc.text = strip_trailing_citation_markers(&wc.text);
    }
    for oq in &mut resp.open_questions {
        oq.text = strip_trailing_citation_markers(&oq.text);
    }
}

fn build_verify_request(
    run_id: Uuid,
    theme_id: Uuid,
    generated: &ThemeGenerateResponse,
    items: &[SelectedItem],
    tau_a: f32,
) -> (VerifyCardRequest, Vec<(VerifySection, usize)>) {
    let mut sentences = Vec::new();
    let mut mapping = Vec::new();

    let mut push_sentence = |kind: VerifySentenceKind,
                             section: VerifySection,
                             pos: usize,
                             text: &str,
                             refs: &[usize]| {
        let idx = sentences.len();
        sentences.push(VerifySentenceInput::with_kind(
            idx,
            kind,
            text.to_string(),
            refs.iter().map(|&r| r as i64).collect(),
        ));
        mapping.push((section, pos));
    };

    for (pos, kp) in generated.key_points.iter().enumerate() {
        push_sentence(
            VerifySentenceKind::KeyPoint,
            VerifySection::KeyPoint,
            pos,
            &kp.text,
            &kp.refs,
        );
    }
    for (pos, tl) in generated.timeline.iter().enumerate() {
        push_sentence(
            VerifySentenceKind::Timeline,
            VerifySection::Timeline,
            pos,
            &tl.text,
            &tl.refs,
        );
    }
    for (pos, wc) in generated.what_changed.iter().enumerate() {
        push_sentence(
            VerifySentenceKind::WhatChanged,
            VerifySection::WhatChanged,
            pos,
            &wc.text,
            &wc.refs,
        );
    }
    for (pos, oq) in generated.open_questions.iter().enumerate() {
        push_sentence(
            VerifySentenceKind::OpenQuestion,
            VerifySection::OpenQuestion,
            pos,
            &oq.text,
            &oq.refs,
        );
    }

    let verify_items = items
        .iter()
        .map(|it| VerifyItemInput {
            n: it.n as i64,
            title: it.article.title.clone(),
            lede: it.article.lede.clone(),
        })
        .collect();

    let req = VerifyCardRequest {
        job_id: run_id,
        card_id: theme_id,
        language: "ja".to_string(),
        sentences,
        items: verify_items,
        thresholds: VerifyThresholds {
            attribution_cos: tau_a,
        },
    };
    (req, mapping)
}

fn build_failure_notes(verify_resp: &VerifyCardResponse, tau_a: f32) -> Vec<String> {
    let mut notes = Vec::new();
    for s in &verify_resp.sentences {
        if !s.attribution.pass {
            let note = format!("Sentence {}: low attribution (cos < {tau_a})", s.idx + 1);
            notes.push(clip_chars(&note, MAX_FAILURE_NOTE_CHARS));
            if notes.len() >= MAX_FAILURE_NOTES {
                break;
            }
        }
        if !s.filler.pass {
            let note = format!(
                "Sentence {}: avoid speculative filler [{}]",
                s.idx + 1,
                s.filler.matched.join(", ")
            );
            notes.push(clip_chars(&note, MAX_FAILURE_NOTE_CHARS));
            if notes.len() >= MAX_FAILURE_NOTES {
                break;
            }
        }
    }
    notes
}

fn map_refs_to_article_ids(refs: &[usize], n_to_id: &HashMap<usize, Uuid>) -> Vec<Uuid> {
    let mut seen = HashSet::new();
    let mut ids = Vec::with_capacity(refs.len());
    for r in refs {
        if let Some(&article_id) = n_to_id.get(r) {
            if seen.insert(article_id) {
                ids.push(article_id);
            }
        }
    }
    ids
}

fn build_verified_theme_body(
    generated: &ThemeGenerateResponse,
    selected_items: &[SelectedItem],
    sentence_map: &[(VerifySection, usize)],
    verify_resp: &VerifyCardResponse,
) -> ThemeBody {
    let mut bad_indices = HashSet::new();
    for s in &verify_resp.sentences {
        if !s.attribution.pass || !s.filler.pass {
            bad_indices.insert(s.idx);
        }
    }

    let mut bad_key_points = HashSet::new();
    let mut bad_timeline = HashSet::new();
    let mut bad_what_changed = HashSet::new();
    let mut bad_open_questions = HashSet::new();

    for idx in bad_indices {
        if let Some(&(section, pos)) = sentence_map.get(idx) {
            match section {
                VerifySection::KeyPoint => {
                    bad_key_points.insert(pos);
                }
                VerifySection::Timeline => {
                    bad_timeline.insert(pos);
                }
                VerifySection::WhatChanged => {
                    bad_what_changed.insert(pos);
                }
                VerifySection::OpenQuestion => {
                    bad_open_questions.insert(pos);
                }
            }
        }
    }

    let n_to_id: HashMap<usize, Uuid> = selected_items
        .iter()
        .map(|it| (it.n, it.article.id))
        .collect();

    let key_points = generated
        .key_points
        .iter()
        .enumerate()
        .filter(|(pos, kp)| !bad_key_points.contains(pos) && !kp.text.trim().is_empty())
        .filter_map(|(_, kp)| {
            let source_article_ids = map_refs_to_article_ids(&kp.refs, &n_to_id);
            (!source_article_ids.is_empty()).then(|| ThemeClaim {
                text: kp.text.clone(),
                source_article_ids,
            })
        })
        .collect();

    let timeline = generated
        .timeline
        .iter()
        .enumerate()
        .filter(|(pos, tl)| !bad_timeline.contains(pos) && !tl.text.trim().is_empty())
        .filter_map(|(_, tl)| {
            let source_article_ids = map_refs_to_article_ids(&tl.refs, &n_to_id);
            (!source_article_ids.is_empty()).then(|| ThemeTimelineEntry {
                date: tl.date.clone(),
                text: tl.text.clone(),
                source_article_ids,
            })
        })
        .collect();

    let what_changed = generated
        .what_changed
        .iter()
        .enumerate()
        .filter(|(pos, wc)| !bad_what_changed.contains(pos) && !wc.text.trim().is_empty())
        .filter_map(|(_, wc)| {
            let source_article_ids = map_refs_to_article_ids(&wc.refs, &n_to_id);
            (!source_article_ids.is_empty()).then(|| ThemeClaim {
                text: wc.text.clone(),
                source_article_ids,
            })
        })
        .collect();

    let open_questions = generated
        .open_questions
        .iter()
        .enumerate()
        .filter(|(pos, oq)| !bad_open_questions.contains(pos) && !oq.text.trim().is_empty())
        .filter_map(|(_, oq)| {
            let source_article_ids = map_refs_to_article_ids(&oq.refs, &n_to_id);
            (!source_article_ids.is_empty()).then(|| ThemeClaim {
                text: oq.text.clone(),
                source_article_ids,
            })
        })
        .collect();

    let sources = selected_items
        .iter()
        .map(|it| ThemeSource {
            article_id: it.article.id,
            title: it.article.title.clone(),
            url: it.article.url.clone(),
        })
        .collect();

    ThemeBody {
        key_points,
        timeline,
        what_changed,
        open_questions,
        sources,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::clients::knowledge_sovereign::ReadingSourceEntry;
    use crate::clients::news_creator::models::{
        CardGenerate422Response, ThemeSentenceOutput, ThemeTimelineOutput,
    };
    use crate::clients::subworker::cards::{
        ClusterOutput, ClusterStoriesResponse, StoryClusterParams, VerifyAttribution,
        VerifyEmbeddingInfo, VerifyFiller, VerifySentenceResult, VerifySpecificity,
    };
    use crate::pipeline::themes::fakes::{
        FakeCardVerifier, FakeEmbedCluster, FakeThemeArticleSource, FakeThemeGenerator,
        FakeThemeKnowledgePort,
    };
    use crate::store::dao::themes::extract_previous_prompt_texts;
    use chrono::TimeZone;
    use std::sync::Mutex;

    type RecordedRunStart = (Uuid, Uuid, DateTime<Utc>, String);
    type CachedEmbedding = (String, usize, Vec<f32>);

    #[derive(Default)]
    struct MockThemesPipelineDao {
        runs: Mutex<Vec<RecordedRunStart>>,
        finishes: Mutex<Vec<(Uuid, Uuid, ThemeRunFinish)>>,
        versions: Mutex<Vec<NewThemeVersion>>,
        published: Mutex<HashMap<Uuid, DateTime<Utc>>>,
        cache: Mutex<HashMap<String, CachedEmbedding>>,
        attempts: Mutex<Vec<NewThemeGenerationAttempt>>,
    }

    impl MockThemesPipelineDao {
        fn seed_version(&self, version: NewThemeVersion, mark_published_at: Option<DateTime<Utc>>) {
            let version_id = version.version_id;
            self.versions.lock().unwrap().push(version);
            if let Some(ts) = mark_published_at {
                self.published.lock().unwrap().insert(version_id, ts);
            }
        }

        fn seed_attempt(&self, attempt: NewThemeGenerationAttempt) {
            self.attempts.lock().unwrap().push(attempt);
        }
    }

    #[async_trait::async_trait]
    impl EmbeddingCacheDao for MockThemesPipelineDao {
        async fn get_cached_embeddings(
            &self,
            model: &str,
            text_hashes: &[String],
        ) -> Result<HashMap<String, Vec<f32>>> {
            let lock = self.cache.lock().unwrap();
            let mut out = HashMap::new();
            for h in text_hashes {
                if let Some((stored_model, _dim, vec)) = lock.get(h) {
                    if stored_model == model {
                        out.insert(h.clone(), vec.clone());
                    }
                }
            }
            Ok(out)
        }

        async fn insert_embeddings(
            &self,
            model: &str,
            dim: usize,
            entries: &[(String, Vec<f32>)],
        ) -> Result<()> {
            let mut lock = self.cache.lock().unwrap();
            for (h, vec) in entries {
                lock.entry(h.clone())
                    .or_insert_with(|| (model.to_string(), dim, vec.clone()));
            }
            Ok(())
        }
    }

    #[async_trait::async_trait]
    impl ThemesPipelineDao for MockThemesPipelineDao {
        async fn create_run(
            &self,
            run_id: Uuid,
            user_id: Uuid,
            kicked_at: DateTime<Utc>,
            params_version: &str,
        ) -> Result<()> {
            self.runs.lock().unwrap().push((
                run_id,
                user_id,
                kicked_at,
                params_version.to_string(),
            ));
            Ok(())
        }

        async fn finish_run(
            &self,
            run_id: Uuid,
            user_id: Uuid,
            finish: &ThemeRunFinish,
        ) -> Result<()> {
            self.finishes
                .lock()
                .unwrap()
                .push((run_id, user_id, finish.clone()));
            Ok(())
        }

        async fn latest_versions_for_user(&self, user_id: Uuid) -> Result<Vec<LatestThemeVersion>> {
            let lock = self.versions.lock().unwrap();
            let mut latest_by_theme: HashMap<Uuid, &NewThemeVersion> = HashMap::new();
            for v in lock.iter().filter(|v| v.user_id == user_id) {
                latest_by_theme
                    .entry(v.theme_id)
                    .and_modify(|existing| {
                        if v.version > existing.version {
                            *existing = v;
                        }
                    })
                    .or_insert(v);
            }
            let mut out: Vec<LatestThemeVersion> = latest_by_theme
                .into_values()
                .map(|v| {
                    let (key_point_texts, open_question_texts) =
                        extract_previous_prompt_texts(&v.body);
                    LatestThemeVersion {
                        theme_id: v.theme_id,
                        version_id: v.version_id,
                        version: v.version,
                        membership_hash: v.membership_hash.clone(),
                        centroid: v.centroid.clone(),
                        title: v.title.clone(),
                        key_point_texts,
                        open_question_texts,
                        created_at: v.created_at,
                    }
                })
                .collect();
            out.sort_by_key(|v| v.theme_id);
            Ok(out)
        }

        async fn get_theme_version(
            &self,
            user_id: Uuid,
            theme_id: Uuid,
            version: Option<i32>,
        ) -> Result<Option<ThemeVersionDetail>> {
            let lock = self.versions.lock().unwrap();
            let matching: Vec<&NewThemeVersion> = lock
                .iter()
                .filter(|v| v.user_id == user_id && v.theme_id == theme_id)
                .collect();
            let Some(latest_version) = matching.iter().map(|v| v.version).max() else {
                return Ok(None);
            };
            let target = match version {
                Some(ver) => matching.into_iter().find(|v| v.version == ver),
                None => matching.into_iter().max_by_key(|v| v.version),
            };
            Ok(target.map(|v| ThemeVersionDetail {
                theme_id: v.theme_id,
                version_id: v.version_id,
                version: v.version,
                latest_version,
                title: v.title.clone(),
                body: v.body.clone(),
                member_article_ids: v.member_article_ids.clone(),
                created_at: v.created_at,
            }))
        }

        async fn insert_theme_version(&self, input: &NewThemeVersion) -> Result<()> {
            self.versions.lock().unwrap().push(input.clone());
            Ok(())
        }

        async fn unpublished_versions(
            &self,
            user_id: Uuid,
        ) -> Result<Vec<UnpublishedThemeVersion>> {
            let versions = self.versions.lock().unwrap();
            let published = self.published.lock().unwrap();
            let mut out: Vec<UnpublishedThemeVersion> = versions
                .iter()
                .filter(|v| v.user_id == user_id && !published.contains_key(&v.version_id))
                .map(|v| UnpublishedThemeVersion {
                    version_id: v.version_id,
                    user_id: v.user_id,
                    theme_id: v.theme_id,
                    version: v.version,
                    title: v.title.clone(),
                    member_article_ids: v.member_article_ids.clone(),
                    created_at: v.created_at,
                })
                .collect();
            out.sort_by_key(|v| (v.created_at, v.version));
            Ok(out)
        }

        async fn mark_published(
            &self,
            _user_id: Uuid,
            version_id: Uuid,
            published_at: DateTime<Utc>,
        ) -> Result<()> {
            self.published
                .lock()
                .unwrap()
                .entry(version_id)
                .or_insert(published_at);
            Ok(())
        }

        async fn generation_attempt_counts_for_user(
            &self,
            user_id: Uuid,
        ) -> Result<HashMap<(Uuid, String), usize>> {
            let lock = self.attempts.lock().unwrap();
            let mut counts = HashMap::new();
            for attempt in lock.iter().filter(|a| a.user_id == user_id) {
                *counts
                    .entry((attempt.theme_id, attempt.membership_hash.clone()))
                    .or_insert(0) += 1;
            }
            Ok(counts)
        }

        async fn insert_generation_attempt(&self, input: &NewThemeGenerationAttempt) -> Result<()> {
            self.attempts.lock().unwrap().push(input.clone());
            Ok(())
        }
    }

    fn owner() -> KnowledgeOwnerIds {
        KnowledgeOwnerIds {
            user_id: Uuid::parse_str("11111111-1111-1111-1111-111111111111").unwrap(),
            tenant_id: Uuid::parse_str("22222222-2222-2222-2222-222222222222").unwrap(),
        }
    }

    fn run_clock() -> DateTime<Utc> {
        Utc.with_ymd_and_hms(2026, 11, 18, 19, 0, 0).unwrap()
    }

    fn test_params() -> ThemesParams {
        ThemesParams {
            expected_embed_dim: 4,
            ..ThemesParams::default()
        }
    }

    fn make_source(id: Uuid, rule_version: i32) -> ReadingSourceEntry {
        ReadingSourceEntry {
            article_id: id,
            first_read_at: Utc.with_ymd_and_hms(2026, 11, 15, 10, 0, 0).unwrap(),
            read_rule_version: rule_version,
        }
    }

    fn make_article(id: Uuid, title: &str, content: &str, pub_day: Option<u32>) -> DataHubArticle {
        DataHubArticle {
            id,
            title: title.to_string(),
            url: format!("https://example.com/{id}"),
            content: content.to_string(),
            published_at: pub_day.map(|d| Utc.with_ymd_and_hms(2026, 11, d, 8, 0, 0).unwrap()),
        }
    }

    fn uuid_from_u128(v: u128) -> Uuid {
        Uuid::from_u128(v)
    }

    #[tokio::test]
    async fn fewer_than_three_sources_completes_with_zero_counts() {
        let a1 = uuid_from_u128(1);
        let a2 = uuid_from_u128(2);
        let knowledge = Arc::new(FakeThemeKnowledgePort::with_sources(vec![
            make_source(a1, 1),
            make_source(a2, 2),
        ]));
        let article_source = Arc::new(FakeThemeArticleSource::default());
        let ml_port = Arc::new(FakeEmbedCluster::new());
        let generator = Arc::new(FakeThemeGenerator::new());
        let verifier = Arc::new(FakeCardVerifier::new());
        let dao = Arc::new(MockThemesPipelineDao::default());

        let pipeline = ThemesPipeline::new(
            knowledge,
            article_source.clone(),
            ml_port.clone(),
            generator.clone(),
            verifier.clone(),
            dao.clone(),
        );

        let run_id = Uuid::new_v4();
        let res = pipeline
            .run(run_id, owner(), run_clock(), &test_params())
            .await
            .expect("run succeeds");

        assert_eq!(res.source_count, 2);
        assert_eq!(res.versions_created, 0);
        assert_eq!(res.versions_deferred, 0);
        assert!(article_source.fetch_calls.lock().unwrap().is_empty());
        assert!(ml_port.cluster_calls.lock().unwrap().is_empty());
        assert!(generator.requests.lock().unwrap().is_empty());
        assert!(verifier.requests.lock().unwrap().is_empty());
        assert!(dao.versions.lock().unwrap().is_empty());

        let finishes = dao.finishes.lock().unwrap();
        assert_eq!(finishes.len(), 1);
        assert_eq!(finishes[0].2.status, ThemeRunStatus::Completed);
        assert_eq!(finishes[0].2.versions_created, 0);
        assert_eq!(finishes[0].2.versions_deferred, 0);
    }

    #[tokio::test]
    async fn unchanged_membership_makes_zero_generate_or_verify_calls() {
        let a1 = uuid_from_u128(1);
        let a2 = uuid_from_u128(2);
        let a3 = uuid_from_u128(3);
        let member_ids = vec![a1, a2, a3];
        let hash = compute_cluster_fingerprint(&member_ids);
        let theme_id = Uuid::new_v5(&Uuid::NAMESPACE_OID, hash.as_bytes());
        let centroid = vec![1.0, 0.0, 0.0, 0.0];

        let dao = Arc::new(MockThemesPipelineDao::default());
        dao.seed_version(
            NewThemeVersion {
                version_id: Uuid::new_v4(),
                user_id: owner().user_id,
                theme_id,
                version: 1,
                run_id: Uuid::new_v4(),
                title: "Existing Theme".to_string(),
                member_article_ids: member_ids.clone(),
                membership_hash: hash,
                centroid: centroid.clone(),
                body: ThemeBody {
                    key_points: vec![],
                    timeline: vec![],
                    what_changed: vec![],
                    open_questions: vec![],
                    sources: vec![],
                },
                created_at: run_clock() - chrono::Duration::days(1),
            },
            Some(run_clock() - chrono::Duration::days(1)),
        );

        let knowledge = Arc::new(FakeThemeKnowledgePort::with_sources(vec![
            make_source(a1, 1),
            make_source(a2, 2),
            make_source(a3, 1),
        ]));
        let article_source = Arc::new(FakeThemeArticleSource::with_articles(vec![
            make_article(a1, "T1", "<p>Body 1</p>", Some(10)),
            make_article(a2, "T2", "<p>Body 2</p>", Some(11)),
            make_article(a3, "T3", "<p>Body 3</p>", Some(12)),
        ]));
        let ml_port = Arc::new(FakeEmbedCluster::with_fixed(
            vec![centroid.clone(), centroid.clone(), centroid.clone()],
            ClusterStoriesResponse {
                clusters: vec![ClusterOutput {
                    cluster_id: 0,
                    member_ids,
                    centroid,
                }],
                params: StoryClusterParams::default(),
            },
        ));
        let generator = Arc::new(FakeThemeGenerator::new());
        let verifier = Arc::new(FakeCardVerifier::new());

        let pipeline = ThemesPipeline::new(
            knowledge,
            article_source,
            ml_port.clone(),
            generator.clone(),
            verifier.clone(),
            dao.clone(),
        );

        let res = pipeline
            .run(Uuid::new_v4(), owner(), run_clock(), &test_params())
            .await
            .expect("run succeeds");

        assert_eq!(res.themes_unchanged, 1);
        assert_eq!(res.versions_created, 0);
        assert!(generator.requests.lock().unwrap().is_empty());
        assert!(verifier.requests.lock().unwrap().is_empty());
        assert_eq!(dao.versions.lock().unwrap().len(), 1);
        let cluster_call = &ml_port.cluster_calls.lock().unwrap()[0];
        assert!(cluster_call.params.time_decay_per_day.abs() < f32::EPSILON);
        assert!((cluster_call.params.threshold - 0.65).abs() < f32::EPSILON);
    }

    #[tokio::test]
    async fn new_theme_creates_version_one_with_empty_new_item_ns_and_strips_citations() {
        let a1 = uuid_from_u128(1);
        let a2 = uuid_from_u128(2);
        let a3 = uuid_from_u128(3);
        let centroid = vec![1.0, 0.0, 0.0, 0.0];

        let knowledge = Arc::new(FakeThemeKnowledgePort::with_sources(vec![
            make_source(a1, 1),
            make_source(a2, 2),
            make_source(a3, 1),
        ]));
        let article_source = Arc::new(FakeThemeArticleSource::with_articles(vec![
            make_article(a1, "Article 1", "<p>Body one</p>", Some(10)),
            make_article(a2, "Article 2", "<p>Body two</p>", Some(11)),
            make_article(a3, "Article 3", "<p>Body three</p>", Some(12)),
        ]));
        let ml_port = Arc::new(FakeEmbedCluster::with_fixed(
            vec![centroid.clone(), centroid.clone(), centroid.clone()],
            ClusterStoriesResponse {
                clusters: vec![ClusterOutput {
                    cluster_id: 0,
                    member_ids: vec![a3, a1, a2],
                    centroid,
                }],
                params: StoryClusterParams::default(),
            },
        ));
        let generator = Arc::new(FakeThemeGenerator::with_outcomes(vec![Ok(
            ThemeGenerateOutcome::Success(ThemeGenerateResponse {
                title: "Theme Alpha".to_string(),
                key_points: vec![ThemeSentenceOutput {
                    text: "Key claim grounded in sources. [1] [2]".to_string(),
                    refs: vec![1, 2],
                }],
                timeline: vec![ThemeTimelineOutput {
                    date: "2026-11-10".to_string(),
                    text: "Milestone happened. [1]".to_string(),
                    refs: vec![1],
                }],
                what_changed: vec![],
                open_questions: vec![ThemeSentenceOutput {
                    text: "Open question remains? [2]".to_string(),
                    refs: vec![2],
                }],
                model: "gemma4-e4b".to_string(),
            }),
        )]));
        let verifier = Arc::new(FakeCardVerifier::new());
        let dao = Arc::new(MockThemesPipelineDao::default());

        let pipeline = ThemesPipeline::new(
            knowledge.clone(),
            article_source,
            ml_port,
            generator.clone(),
            verifier.clone(),
            dao.clone(),
        );

        let clock = run_clock();
        let res = pipeline
            .run(Uuid::new_v4(), owner(), clock, &test_params())
            .await
            .expect("run succeeds");

        assert_eq!(res.versions_created, 1);
        let gen_reqs = generator.requests.lock().unwrap();
        assert_eq!(gen_reqs.len(), 1);
        assert!(gen_reqs[0].new_item_ns.is_empty());
        assert!(gen_reqs[0].previous.is_none());

        let verify_reqs = verifier.requests.lock().unwrap();
        assert_eq!(verify_reqs.len(), 1);
        assert_eq!(verify_reqs[0].sentences.len(), 3);
        assert_eq!(verify_reqs[0].sentences[0].kind, "key_point");
        assert_eq!(verify_reqs[0].sentences[1].kind, "timeline");
        assert_eq!(verify_reqs[0].sentences[2].kind, "open_question");
        assert_eq!(
            verify_reqs[0].sentences[0].text,
            "Key claim grounded in sources."
        );

        let versions = dao.versions.lock().unwrap();
        assert_eq!(versions.len(), 1);
        assert_eq!(versions[0].version, 1);
        assert_eq!(versions[0].created_at, clock);
        assert_eq!(versions[0].member_article_ids, vec![a1, a2, a3]);
        assert_eq!(
            versions[0].body.key_points[0].text,
            "Key claim grounded in sources."
        );
        assert_eq!(versions[0].body.timeline[0].text, "Milestone happened.");
        assert_eq!(
            versions[0].body.open_questions[0].text,
            "Open question remains?"
        );
        assert_eq!(versions[0].body.sources.len(), 3);
        assert!(
            dao.published
                .lock()
                .unwrap()
                .contains_key(&versions[0].version_id)
        );
        assert_eq!(knowledge.publish_calls.lock().unwrap().len(), 1);
    }

    #[tokio::test]
    async fn new_member_increments_version_with_new_item_ns_and_previous() {
        let a1 = uuid_from_u128(1);
        let a2 = uuid_from_u128(2);
        let a3 = uuid_from_u128(3);
        let a4 = uuid_from_u128(4);
        let old_members = vec![a1, a2, a3];
        let old_hash = compute_cluster_fingerprint(&old_members);
        let theme_id = Uuid::new_v5(&Uuid::NAMESPACE_OID, old_hash.as_bytes());
        let centroid = vec![1.0, 0.0, 0.0, 0.0];

        let dao = Arc::new(MockThemesPipelineDao::default());
        dao.seed_version(
            NewThemeVersion {
                version_id: Uuid::new_v4(),
                user_id: owner().user_id,
                theme_id,
                version: 1,
                run_id: Uuid::new_v4(),
                title: "Prev Theme Title".to_string(),
                member_article_ids: old_members,
                membership_hash: old_hash,
                centroid: centroid.clone(),
                body: ThemeBody {
                    key_points: vec![ThemeClaim {
                        text: "Prev KP".to_string(),
                        source_article_ids: vec![a1],
                    }],
                    timeline: vec![],
                    what_changed: vec![],
                    open_questions: vec![ThemeClaim {
                        text: "Prev OQ".to_string(),
                        source_article_ids: vec![a2],
                    }],
                    sources: vec![],
                },
                created_at: run_clock() - chrono::Duration::days(1),
            },
            Some(run_clock() - chrono::Duration::days(1)),
        );

        let knowledge = Arc::new(FakeThemeKnowledgePort::with_sources(vec![
            make_source(a1, 1),
            make_source(a2, 1),
            make_source(a3, 1),
            make_source(a4, 2),
        ]));
        let article_source = Arc::new(FakeThemeArticleSource::with_articles(vec![
            make_article(a1, "Old 1", "<p>Body 1</p>", Some(10)),
            make_article(a2, "Old 2", "<p>Body 2</p>", Some(11)),
            make_article(a3, "Old 3", "<p>Body 3</p>", Some(12)),
            make_article(a4, "New 4", "<p>Body 4</p>", Some(18)),
        ]));
        let ml_port = Arc::new(FakeEmbedCluster::with_fixed(
            vec![
                centroid.clone(),
                centroid.clone(),
                centroid.clone(),
                centroid.clone(),
            ],
            ClusterStoriesResponse {
                clusters: vec![ClusterOutput {
                    cluster_id: 0,
                    member_ids: vec![a1, a2, a3, a4],
                    centroid,
                }],
                params: StoryClusterParams::default(),
            },
        ));
        let generator = Arc::new(FakeThemeGenerator::new());
        let verifier = Arc::new(FakeCardVerifier::new());

        let pipeline = ThemesPipeline::new(
            knowledge,
            article_source,
            ml_port,
            generator.clone(),
            verifier,
            dao.clone(),
        );

        let res = pipeline
            .run(Uuid::new_v4(), owner(), run_clock(), &test_params())
            .await
            .expect("run succeeds");
        assert_eq!(res.versions_created, 1);

        let req = &generator.requests.lock().unwrap()[0];
        assert_eq!(req.new_item_ns, vec![1]);
        assert_eq!(req.items[0].title, "New 4");
        let prev = req.previous.as_ref().expect("previous populated");
        assert_eq!(prev.title, "Prev Theme Title");
        assert_eq!(prev.key_points, vec!["Prev KP"]);
        assert_eq!(prev.open_questions, vec!["Prev OQ"]);

        let versions = dao.versions.lock().unwrap();
        assert_eq!(versions.len(), 2);
        let v2 = versions
            .iter()
            .find(|v| v.version == 2)
            .expect("v2 created");
        assert_eq!(v2.theme_id, theme_id);
        assert_eq!(v2.member_article_ids, vec![a1, a2, a3, a4]);
    }

    #[tokio::test]
    async fn cap_limits_generations_to_five_and_defers_remaining() {
        let mut sources = Vec::new();
        let mut articles = Vec::new();
        let mut embeddings = Vec::new();
        let mut clusters = Vec::new();

        for c_idx in 0..7usize {
            let mut member_ids = Vec::new();
            for m_idx in 0..3usize {
                let id = uuid_from_u128((c_idx * 10 + m_idx + 1) as u128);
                sources.push(make_source(id, 1));
                articles.push(make_article(
                    id,
                    &format!("Title {c_idx}-{m_idx}"),
                    &format!("<p>Body {c_idx}-{m_idx}</p>"),
                    Some(15),
                ));
                embeddings.push(vec![1.0, 0.0, 0.0, 0.0]);
                member_ids.push(id);
            }
            clusters.push(ClusterOutput {
                cluster_id: c_idx,
                member_ids,
                centroid: vec![1.0, 0.0, 0.0, 0.0],
            });
        }

        let knowledge = Arc::new(FakeThemeKnowledgePort::with_sources(sources));
        let article_source = Arc::new(FakeThemeArticleSource::with_articles(articles));
        let ml_port = Arc::new(FakeEmbedCluster::with_fixed(
            embeddings,
            ClusterStoriesResponse {
                clusters,
                params: StoryClusterParams::default(),
            },
        ));
        let generator = Arc::new(FakeThemeGenerator::new());
        let verifier = Arc::new(FakeCardVerifier::new());
        let dao = Arc::new(MockThemesPipelineDao::default());

        let pipeline = ThemesPipeline::new(
            knowledge,
            article_source,
            ml_port,
            generator.clone(),
            verifier,
            dao.clone(),
        );

        let res = pipeline
            .run(Uuid::new_v4(), owner(), run_clock(), &test_params())
            .await
            .expect("run succeeds");

        assert_eq!(res.clusters_eligible, 7);
        assert_eq!(res.versions_created, 5);
        assert_eq!(res.versions_deferred, 2);
        assert_eq!(generator.requests.lock().unwrap().len(), 5);
        assert_eq!(dao.versions.lock().unwrap().len(), 5);
    }

    #[tokio::test]
    #[allow(clippy::too_many_lines)]
    async fn failing_sentence_triggers_one_regeneration_then_drops_still_failing() {
        let a1 = uuid_from_u128(1);
        let a2 = uuid_from_u128(2);
        let a3 = uuid_from_u128(3);
        let centroid = vec![1.0, 0.0, 0.0, 0.0];

        let knowledge = Arc::new(FakeThemeKnowledgePort::with_sources(vec![
            make_source(a1, 1),
            make_source(a2, 1),
            make_source(a3, 1),
        ]));
        let article_source = Arc::new(FakeThemeArticleSource::with_articles(vec![
            make_article(a1, "T1", "<p>Body 1</p>", Some(10)),
            make_article(a2, "T2", "<p>Body 2</p>", Some(11)),
            make_article(a3, "T3", "<p>Body 3</p>", Some(12)),
        ]));
        let ml_port = Arc::new(FakeEmbedCluster::with_fixed(
            vec![centroid.clone(), centroid.clone(), centroid.clone()],
            ClusterStoriesResponse {
                clusters: vec![ClusterOutput {
                    cluster_id: 0,
                    member_ids: vec![a1, a2, a3],
                    centroid,
                }],
                params: StoryClusterParams::default(),
            },
        ));

        let resp = || {
            Ok(ThemeGenerateOutcome::Success(ThemeGenerateResponse {
                title: "Theme".to_string(),
                key_points: vec![
                    ThemeSentenceOutput {
                        text: "Good point [1]".to_string(),
                        refs: vec![1],
                    },
                    ThemeSentenceOutput {
                        text: "Bad point [2]".to_string(),
                        refs: vec![2],
                    },
                ],
                timeline: vec![],
                what_changed: vec![],
                open_questions: vec![],
                model: "gemma4-e4b".to_string(),
            }))
        };
        let generator = Arc::new(FakeThemeGenerator::with_outcomes(vec![resp(), resp()]));

        let fail_second_kp = || VerifyCardResponse {
            sentences: vec![
                VerifySentenceResult {
                    idx: 0,
                    attribution: VerifyAttribution {
                        max_cos: 0.9,
                        best_n: Some(1),
                        pass: true,
                    },
                    filler: VerifyFiller {
                        matched: vec![],
                        pass: true,
                    },
                    specificity: VerifySpecificity {
                        proper_nouns: 1,
                        numbers: 0,
                        tokens: 3,
                        density: 0.3,
                    },
                },
                VerifySentenceResult {
                    idx: 1,
                    attribution: VerifyAttribution {
                        max_cos: 0.3,
                        best_n: Some(2),
                        pass: false,
                    },
                    filler: VerifyFiller {
                        matched: vec!["maybe".to_string()],
                        pass: false,
                    },
                    specificity: VerifySpecificity {
                        proper_nouns: 0,
                        numbers: 0,
                        tokens: 3,
                        density: 0.0,
                    },
                },
            ],
            why_hint_present: false,
            embedding: VerifyEmbeddingInfo {
                model: "bge-m3".to_string(),
                identity: "bge-m3".to_string(),
            },
        };
        let verifier = Arc::new(FakeCardVerifier::with_responses(vec![
            fail_second_kp(),
            fail_second_kp(),
        ]));
        let dao = Arc::new(MockThemesPipelineDao::default());

        let pipeline = ThemesPipeline::new(
            knowledge,
            article_source,
            ml_port,
            generator.clone(),
            verifier.clone(),
            dao.clone(),
        );

        let res = pipeline
            .run(Uuid::new_v4(), owner(), run_clock(), &test_params())
            .await
            .expect("run succeeds");

        assert_eq!(res.versions_created, 1);
        let gen_reqs = generator.requests.lock().unwrap();
        assert_eq!(gen_reqs.len(), 2);
        assert!(gen_reqs[0].failure_notes.is_empty());
        assert!(!gen_reqs[1].failure_notes.is_empty());
        assert!(gen_reqs[1].failure_notes.len() <= MAX_FAILURE_NOTES);
        assert!(
            gen_reqs[1]
                .failure_notes
                .iter()
                .all(|n| n.chars().count() <= MAX_FAILURE_NOTE_CHARS)
        );
        assert_eq!(verifier.requests.lock().unwrap().len(), 2);

        let versions = dao.versions.lock().unwrap();
        assert_eq!(versions.len(), 1);
        assert_eq!(versions[0].body.key_points.len(), 1);
        assert_eq!(versions[0].body.key_points[0].text, "Good point");
    }

    #[tokio::test]
    async fn all_key_points_failing_creates_no_version() {
        let a1 = uuid_from_u128(1);
        let a2 = uuid_from_u128(2);
        let a3 = uuid_from_u128(3);
        let centroid = vec![1.0, 0.0, 0.0, 0.0];

        let knowledge = Arc::new(FakeThemeKnowledgePort::with_sources(vec![
            make_source(a1, 1),
            make_source(a2, 1),
            make_source(a3, 1),
        ]));
        let article_source = Arc::new(FakeThemeArticleSource::with_articles(vec![
            make_article(a1, "T1", "<p>Body 1</p>", Some(10)),
            make_article(a2, "T2", "<p>Body 2</p>", Some(11)),
            make_article(a3, "T3", "<p>Body 3</p>", Some(12)),
        ]));
        let ml_port = Arc::new(FakeEmbedCluster::with_fixed(
            vec![centroid.clone(), centroid.clone(), centroid.clone()],
            ClusterStoriesResponse {
                clusters: vec![ClusterOutput {
                    cluster_id: 0,
                    member_ids: vec![a1, a2, a3],
                    centroid,
                }],
                params: StoryClusterParams::default(),
            },
        ));
        let generator = Arc::new(FakeThemeGenerator::new());

        let fail_all = VerifyCardResponse {
            sentences: vec![VerifySentenceResult {
                idx: 0,
                attribution: VerifyAttribution {
                    max_cos: 0.2,
                    best_n: None,
                    pass: false,
                },
                filler: VerifyFiller {
                    matched: vec![],
                    pass: true,
                },
                specificity: VerifySpecificity {
                    proper_nouns: 0,
                    numbers: 0,
                    tokens: 3,
                    density: 0.0,
                },
            }],
            why_hint_present: false,
            embedding: VerifyEmbeddingInfo {
                model: "bge-m3".to_string(),
                identity: "bge-m3".to_string(),
            },
        };
        let verifier = Arc::new(FakeCardVerifier::with_responses(vec![
            fail_all.clone(),
            fail_all,
        ]));
        let dao = Arc::new(MockThemesPipelineDao::default());

        let pipeline = ThemesPipeline::new(
            knowledge,
            article_source,
            ml_port,
            generator,
            verifier,
            dao.clone(),
        );

        let res = pipeline
            .run(Uuid::new_v4(), owner(), run_clock(), &test_params())
            .await
            .expect("run succeeds");

        assert_eq!(res.versions_created, 0);
        assert_eq!(res.verification_dropped, 1);
        assert!(dao.versions.lock().unwrap().is_empty());
    }

    #[tokio::test]
    async fn generation_rejected_skips_theme_and_completes_run() {
        let a1 = uuid_from_u128(1);
        let a2 = uuid_from_u128(2);
        let a3 = uuid_from_u128(3);
        let centroid = vec![1.0, 0.0, 0.0, 0.0];

        let knowledge = Arc::new(FakeThemeKnowledgePort::with_sources(vec![
            make_source(a1, 1),
            make_source(a2, 1),
            make_source(a3, 1),
        ]));
        let article_source = Arc::new(FakeThemeArticleSource::with_articles(vec![
            make_article(a1, "T1", "<p>Body 1</p>", Some(10)),
            make_article(a2, "T2", "<p>Body 2</p>", Some(11)),
            make_article(a3, "T3", "<p>Body 3</p>", Some(12)),
        ]));
        let ml_port = Arc::new(FakeEmbedCluster::with_fixed(
            vec![centroid.clone(), centroid.clone(), centroid.clone()],
            ClusterStoriesResponse {
                clusters: vec![ClusterOutput {
                    cluster_id: 0,
                    member_ids: vec![a1, a2, a3],
                    centroid,
                }],
                params: StoryClusterParams::default(),
            },
        ));
        let generator = Arc::new(FakeThemeGenerator::with_outcomes(vec![Ok(
            ThemeGenerateOutcome::Rejected(CardGenerate422Response {
                reason: "nothing_usable".to_string(),
                attempts: 2,
                raw_text: "bad".to_string(),
                detail: Some("no_key_points".to_string()),
            }),
        )]));
        let verifier = Arc::new(FakeCardVerifier::new());
        let dao = Arc::new(MockThemesPipelineDao::default());

        let pipeline = ThemesPipeline::new(
            knowledge,
            article_source,
            ml_port,
            generator,
            verifier,
            dao.clone(),
        );

        let res = pipeline
            .run(Uuid::new_v4(), owner(), run_clock(), &test_params())
            .await
            .expect("run succeeds");

        assert_eq!(res.generation_rejected, 1);
        assert_eq!(res.versions_created, 0);
        assert!(dao.versions.lock().unwrap().is_empty());
        assert_eq!(
            dao.finishes.lock().unwrap()[0].2.status,
            ThemeRunStatus::Completed
        );
    }

    #[tokio::test]
    async fn publish_failure_leaves_version_unpublished_and_next_run_publishes_it_first() {
        let a1 = uuid_from_u128(1);
        let a2 = uuid_from_u128(2);
        let a3 = uuid_from_u128(3);
        let centroid = vec![1.0, 0.0, 0.0, 0.0];

        let knowledge = Arc::new(FakeThemeKnowledgePort::with_sources(vec![
            make_source(a1, 1),
            make_source(a2, 1),
            make_source(a3, 1),
        ]));
        knowledge.set_fail_all_publishes(true);

        let article_source = Arc::new(FakeThemeArticleSource::with_articles(vec![
            make_article(a1, "T1", "<p>Body 1</p>", Some(10)),
            make_article(a2, "T2", "<p>Body 2</p>", Some(11)),
            make_article(a3, "T3", "<p>Body 3</p>", Some(12)),
        ]));
        let ml_port = Arc::new(FakeEmbedCluster::with_fixed(
            vec![centroid.clone(), centroid.clone(), centroid.clone()],
            ClusterStoriesResponse {
                clusters: vec![ClusterOutput {
                    cluster_id: 0,
                    member_ids: vec![a1, a2, a3],
                    centroid: centroid.clone(),
                }],
                params: StoryClusterParams::default(),
            },
        ));
        let generator = Arc::new(FakeThemeGenerator::new());
        let verifier = Arc::new(FakeCardVerifier::new());
        let dao = Arc::new(MockThemesPipelineDao::default());

        let pipeline = ThemesPipeline::new(
            knowledge.clone(),
            article_source,
            ml_port,
            generator,
            verifier,
            dao.clone(),
        );

        let res1 = pipeline
            .run(Uuid::new_v4(), owner(), run_clock(), &test_params())
            .await
            .expect("first run succeeds despite publish failure");
        assert_eq!(res1.versions_created, 1);
        assert_eq!(res1.publish_failures, 1);
        let stored_v1_id = dao.versions.lock().unwrap()[0].version_id;
        assert!(!dao.published.lock().unwrap().contains_key(&stored_v1_id));

        knowledge.set_fail_all_publishes(false);
        knowledge.publish_calls.lock().unwrap().clear();

        let next_clock = run_clock() + chrono::Duration::days(1);
        let res2 = pipeline
            .run(Uuid::new_v4(), owner(), next_clock, &test_params())
            .await
            .expect("second run publishes pending version");
        assert_eq!(res2.versions_created, 0);
        assert_eq!(res2.publish_failures, 0);
        assert_eq!(
            dao.published.lock().unwrap().get(&stored_v1_id),
            Some(&next_clock)
        );
        let calls = knowledge.publish_calls.lock().unwrap();
        assert_eq!(calls.len(), 1);
        assert_eq!(calls[0].2[0].version_id, stored_v1_id);
    }

    #[tokio::test]
    #[allow(clippy::too_many_lines)]
    async fn item_selection_order_and_bounds_enforced() {
        let mut sources = Vec::new();
        let mut articles = Vec::new();
        let mut prev_ids = Vec::new();
        let mut embeddings = Vec::new();

        for i in 1..=7u128 {
            let id = uuid_from_u128(i);
            prev_ids.push(id);
            sources.push(make_source(id, 1));
            let long_title = format!("{} {}", "あ".repeat(130), i);
            let long_body = format!("<p>{}</p>", "い".repeat(450));
            articles.push(make_article(id, &long_title, &long_body, Some(5)));
            // Higher similarity for id 7 than id 1
            let cos_x = 0.8 + (i as f32) * 0.02;
            let cos_y = (1.0 - cos_x * cos_x).max(0.0).sqrt();
            embeddings.push(vec![cos_x, cos_y, 0.0, 0.0]);
        }

        // Two new articles: id 8 (published Nov 12) and id 9 (published Nov 18)
        let id8 = uuid_from_u128(8);
        let id9 = uuid_from_u128(9);
        sources.push(make_source(id8, 1));
        sources.push(make_source(id9, 2));
        articles.push(make_article(id8, "New Older", "<p>Body 8</p>", Some(12)));
        articles.push(make_article(id9, "New Newer", "<p>Body 9</p>", Some(18)));
        embeddings.push(vec![0.85, 0.5, 0.0, 0.0]);
        embeddings.push(vec![0.85, 0.5, 0.0, 0.0]);

        // Add missing-body source and empty-body source
        let id_missing = uuid_from_u128(10);
        let id_empty = uuid_from_u128(11);
        sources.push(make_source(id_missing, 1));
        sources.push(make_source(id_empty, 1));
        articles.push(make_article(id_empty, "Empty", "<p>   </p>", Some(18)));

        let all_valid: Vec<Uuid> = (1..=9).map(uuid_from_u128).collect();
        let old_hash = compute_cluster_fingerprint(&prev_ids);
        let theme_id = Uuid::new_v5(&Uuid::NAMESPACE_OID, old_hash.as_bytes());
        let centroid = vec![1.0, 0.0, 0.0, 0.0];

        let dao = Arc::new(MockThemesPipelineDao::default());
        dao.seed_version(
            NewThemeVersion {
                version_id: Uuid::new_v4(),
                user_id: owner().user_id,
                theme_id,
                version: 1,
                run_id: Uuid::new_v4(),
                title: "T".repeat(130),
                member_article_ids: prev_ids,
                membership_hash: old_hash,
                centroid: centroid.clone(),
                body: ThemeBody {
                    key_points: (0..9)
                        .map(|i| ThemeClaim {
                            text: format!("kp-{i}-{}", "k".repeat(210)),
                            source_article_ids: vec![uuid_from_u128(1)],
                        })
                        .collect(),
                    timeline: vec![],
                    what_changed: vec![],
                    open_questions: (0..6)
                        .map(|i| ThemeClaim {
                            text: format!("oq-{i}-{}", "q".repeat(210)),
                            source_article_ids: vec![uuid_from_u128(1)],
                        })
                        .collect(),
                    sources: vec![],
                },
                created_at: run_clock() - chrono::Duration::days(1),
            },
            Some(run_clock() - chrono::Duration::days(1)),
        );

        let knowledge = Arc::new(FakeThemeKnowledgePort::with_sources(sources));
        let article_source = Arc::new(FakeThemeArticleSource::with_articles(articles));
        let ml_port = Arc::new(FakeEmbedCluster::with_fixed(
            embeddings,
            ClusterStoriesResponse {
                clusters: vec![ClusterOutput {
                    cluster_id: 0,
                    member_ids: all_valid,
                    centroid,
                }],
                params: StoryClusterParams::default(),
            },
        ));
        let generator = Arc::new(FakeThemeGenerator::new());
        let verifier = Arc::new(FakeCardVerifier::new());

        let pipeline = ThemesPipeline::new(
            knowledge,
            article_source,
            ml_port,
            generator.clone(),
            verifier,
            dao.clone(),
        );

        let res = pipeline
            .run(Uuid::new_v4(), owner(), run_clock(), &test_params())
            .await
            .expect("run succeeds");

        assert_eq!(res.missing_bodies, 1);
        assert_eq!(res.empty_bodies, 1);
        assert_eq!(res.versions_created, 1);

        let req = &generator.requests.lock().unwrap()[0];
        assert_eq!(req.items.len(), 8);
        assert_eq!(req.new_item_ns, vec![1, 2]);
        // First two are new items sorted newest first: id9 (Nov 18), then id8 (Nov 12)
        assert_eq!(req.items[0].title, "New Newer");
        assert_eq!(req.items[1].title, "New Older");
        // Next 6 are highest centroid similarity among 1..=7: 7, 6, 5, 4, 3, 2
        assert!(req.items[2].title.ends_with(" 7") || req.items[2].title.chars().count() == 120);
        for item in &req.items {
            assert!(item.title.chars().count() <= 120);
            assert!(item.text.chars().count() <= 400);
        }

        let prev = req.previous.as_ref().unwrap();
        assert_eq!(prev.title.chars().count(), 120);
        assert_eq!(prev.key_points.len(), 7);
        assert!(prev.key_points.iter().all(|k| k.chars().count() <= 200));
        assert_eq!(prev.open_questions.len(), 4);
        assert!(prev.open_questions.iter().all(|q| q.chars().count() <= 200));
    }

    #[test]
    fn assignment_tie_breaks_are_deterministic_and_one_to_one() {
        let id_low = uuid_from_u128(1);
        let id_high = uuid_from_u128(2);
        let latest = vec![
            LatestThemeVersion {
                theme_id: id_high,
                version_id: Uuid::new_v4(),
                version: 1,
                membership_hash: "h_high".to_string(),
                centroid: vec![1.0, 0.0, 0.0, 0.0],
                title: "High".to_string(),
                key_point_texts: vec![],
                open_question_texts: vec![],
                created_at: run_clock(),
            },
            LatestThemeVersion {
                theme_id: id_low,
                version_id: Uuid::new_v4(),
                version: 1,
                membership_hash: "h_low".to_string(),
                centroid: vec![1.0, 0.0, 0.0, 0.0],
                title: "Low".to_string(),
                key_point_texts: vec![],
                open_question_texts: vec![],
                created_at: run_clock(),
            },
        ];

        let clusters = vec![
            EligibleCluster {
                member_ids: vec![uuid_from_u128(10), uuid_from_u128(11), uuid_from_u128(12)],
                membership_hash: "bbb".to_string(),
                centroid: vec![1.0, 0.0, 0.0, 0.0],
            },
            EligibleCluster {
                member_ids: vec![uuid_from_u128(20), uuid_from_u128(21), uuid_from_u128(22)],
                membership_hash: "aaa".to_string(),
                centroid: vec![1.0, 0.0, 0.0, 0.0],
            },
        ];

        let assigned = assign_clusters_to_themes(&clusters, &latest, 0.80, Uuid::new_v4()).unwrap();
        // Equal similarity across both themes and clusters -> lowest theme_id (id_low) first,
        // then lowest membership_hash ("aaa", cluster index 1) gets id_low; cluster index 0 gets id_high.
        assert_eq!(assigned[1].theme_id, id_low);
        assert_eq!(assigned[0].theme_id, id_high);
    }

    #[test]
    fn unmatched_cluster_id_never_collides_with_existing_theme() {
        let member_ids = vec![uuid_from_u128(1), uuid_from_u128(2), uuid_from_u128(3)];
        let hash = compute_cluster_fingerprint(&member_ids);
        let colliding_id = Uuid::new_v5(&Uuid::NAMESPACE_OID, hash.as_bytes());
        let run_id = Uuid::new_v4();

        // Existing theme has the colliding theme_id, but orthogonal centroid so it does NOT match
        let latest = vec![LatestThemeVersion {
            theme_id: colliding_id,
            version_id: Uuid::new_v4(),
            version: 2,
            membership_hash: "other-hash".to_string(),
            centroid: vec![0.0, 1.0, 0.0, 0.0],
            title: "Other".to_string(),
            key_point_texts: vec![],
            open_question_texts: vec![],
            created_at: run_clock(),
        }];
        let clusters = vec![EligibleCluster {
            member_ids,
            membership_hash: hash.clone(),
            centroid: vec![1.0, 0.0, 0.0, 0.0],
        }];

        let assigned = assign_clusters_to_themes(&clusters, &latest, 0.80, run_id).unwrap();
        assert_ne!(assigned[0].theme_id, colliding_id);
        assert!(assigned[0].matched_latest.is_none());
        let expected_fallback =
            Uuid::new_v5(&Uuid::NAMESPACE_OID, format!("{hash}:{run_id}").as_bytes());
        assert_eq!(assigned[0].theme_id, expected_fallback);
    }

    #[test]
    fn strip_trailing_citation_markers_removes_trailing_refs_only() {
        assert_eq!(
            strip_trailing_citation_markers("Summary sentence. [1]"),
            "Summary sentence."
        );
        assert_eq!(
            strip_trailing_citation_markers("Summary sentence. [1] [2]"),
            "Summary sentence."
        );
        assert_eq!(
            strip_trailing_citation_markers("日本語の文です。[1][2]"),
            "日本語の文です。"
        );
        assert_eq!(
            strip_trailing_citation_markers("In [2026] systems evolved. [1, 2] "),
            "In [2026] systems evolved."
        );
    }

    #[tokio::test]
    async fn aborting_error_finishes_run_as_failed_with_message() {
        let knowledge = Arc::new(FakeThemeKnowledgePort::default());
        knowledge.set_list_error(Some("sovereign unavailable".to_string()));
        let article_source = Arc::new(FakeThemeArticleSource::default());
        let ml_port = Arc::new(FakeEmbedCluster::new());
        let generator = Arc::new(FakeThemeGenerator::new());
        let verifier = Arc::new(FakeCardVerifier::new());
        let dao = Arc::new(MockThemesPipelineDao::default());

        let pipeline = ThemesPipeline::new(
            knowledge,
            article_source,
            ml_port,
            generator,
            verifier,
            dao.clone(),
        );

        let run_id = Uuid::new_v4();
        let err = pipeline
            .run(run_id, owner(), run_clock(), &test_params())
            .await
            .expect_err("list error must fail run");
        assert!(format!("{err:#}").contains("sovereign unavailable"));

        let finishes = dao.finishes.lock().unwrap();
        assert_eq!(finishes.len(), 1);
        assert_eq!(finishes[0].0, run_id);
        assert_eq!(finishes[0].2.status, ThemeRunStatus::Failed);
        assert!(
            finishes[0]
                .2
                .error_message
                .as_deref()
                .is_some_and(|m| m.contains("sovereign unavailable"))
        );
    }

    struct SequenceCardVerifier {
        outcomes: Mutex<std::collections::VecDeque<Result<VerifyCardResponse>>>,
    }

    impl SequenceCardVerifier {
        fn new(outcomes: Vec<Result<VerifyCardResponse>>) -> Self {
            Self {
                outcomes: Mutex::new(outcomes.into()),
            }
        }
    }

    #[async_trait::async_trait]
    impl CardVerifier for SequenceCardVerifier {
        async fn verify_card(&self, req: &VerifyCardRequest) -> Result<VerifyCardResponse> {
            if let Some(next) = self.outcomes.lock().unwrap().pop_front() {
                return next;
            }
            Ok(passing_verify_response(req.sentences.len()))
        }
    }

    fn passing_verify_response(sentence_count: usize) -> VerifyCardResponse {
        VerifyCardResponse {
            sentences: (0..sentence_count)
                .map(|idx| VerifySentenceResult {
                    idx,
                    attribution: VerifyAttribution {
                        max_cos: 0.9,
                        best_n: Some(1),
                        pass: true,
                    },
                    filler: VerifyFiller {
                        matched: vec![],
                        pass: true,
                    },
                    specificity: VerifySpecificity {
                        proper_nouns: 1,
                        numbers: 0,
                        tokens: 4,
                        density: 0.25,
                    },
                })
                .collect(),
            why_hint_present: false,
            embedding: VerifyEmbeddingInfo {
                model: "bge-m3".to_string(),
                identity: "bge-m3".to_string(),
            },
        }
    }

    fn failing_verify_response(sentence_count: usize) -> VerifyCardResponse {
        VerifyCardResponse {
            sentences: (0..sentence_count)
                .map(|idx| VerifySentenceResult {
                    idx,
                    attribution: VerifyAttribution {
                        max_cos: 0.2,
                        best_n: None,
                        pass: false,
                    },
                    filler: VerifyFiller {
                        matched: vec![],
                        pass: true,
                    },
                    specificity: VerifySpecificity {
                        proper_nouns: 0,
                        numbers: 0,
                        tokens: 3,
                        density: 0.0,
                    },
                })
                .collect(),
            why_hint_present: false,
            embedding: VerifyEmbeddingInfo {
                model: "bge-m3".to_string(),
                identity: "bge-m3".to_string(),
            },
        }
    }

    struct ClusterFixture {
        sources: Vec<ReadingSourceEntry>,
        articles: Vec<DataHubArticle>,
        embeddings: Vec<Vec<f32>>,
        clusters: Vec<ClusterOutput>,
        cluster_themes: Vec<(Uuid, String)>,
    }

    fn build_clusters_fixture(sizes: &[usize]) -> ClusterFixture {
        let mut sources = Vec::new();
        let mut articles = Vec::new();
        let mut embeddings = Vec::new();
        let mut clusters = Vec::new();
        let mut cluster_themes = Vec::new();
        let mut next_id = 1u128;

        for (c_idx, &size) in sizes.iter().enumerate() {
            let mut member_ids = Vec::with_capacity(size);
            for m_idx in 0..size {
                let id = uuid_from_u128(next_id);
                next_id += 1;
                sources.push(make_source(id, 1));
                articles.push(make_article(
                    id,
                    &format!("Title {c_idx}-{m_idx}"),
                    &format!("<p>Body {c_idx}-{m_idx}</p>"),
                    Some(15),
                ));
                embeddings.push(vec![1.0, 0.0, 0.0, 0.0]);
                member_ids.push(id);
            }
            member_ids.sort();
            let hash = compute_cluster_fingerprint(&member_ids);
            let theme_id = Uuid::new_v5(&Uuid::NAMESPACE_OID, hash.as_bytes());
            cluster_themes.push((theme_id, hash));
            clusters.push(ClusterOutput {
                cluster_id: c_idx,
                member_ids,
                centroid: vec![1.0, 0.0, 0.0, 0.0],
            });
        }

        ClusterFixture {
            sources,
            articles,
            embeddings,
            clusters,
            cluster_themes,
        }
    }

    #[tokio::test]
    async fn generation_error_fails_run_and_preserves_earlier_versions() {
        let fx = build_clusters_fixture(&[4, 3]);
        let knowledge = Arc::new(FakeThemeKnowledgePort::with_sources(fx.sources));
        let article_source = Arc::new(FakeThemeArticleSource::with_articles(fx.articles));
        let ml_port = Arc::new(FakeEmbedCluster::with_fixed(
            fx.embeddings,
            ClusterStoriesResponse {
                clusters: fx.clusters,
                params: StoryClusterParams::default(),
            },
        ));
        let generator = Arc::new(FakeThemeGenerator::with_outcomes(vec![
            Ok(ThemeGenerateOutcome::Success(ThemeGenerateResponse {
                title: "First Theme".to_string(),
                key_points: vec![ThemeSentenceOutput {
                    text: "Grounded claim [1]".to_string(),
                    refs: vec![1],
                }],
                timeline: vec![],
                what_changed: vec![],
                open_questions: vec![],
                model: "gemma4-e4b".to_string(),
            })),
            Err(anyhow::anyhow!("news-creator 503 unavailable")),
        ]));
        let verifier = Arc::new(FakeCardVerifier::new());
        let dao = Arc::new(MockThemesPipelineDao::default());

        let pipeline = ThemesPipeline::new(
            knowledge,
            article_source,
            ml_port,
            generator,
            verifier,
            dao.clone(),
        );

        let err = pipeline
            .run(Uuid::new_v4(), owner(), run_clock(), &test_params())
            .await
            .expect_err("generation error must fail the run");
        let msg = format!("{err:#}");
        let failing_theme_id = fx.cluster_themes[1].0.to_string();
        assert!(msg.contains(&failing_theme_id), "unexpected error: {msg}");
        assert!(msg.contains("news-creator 503 unavailable"));

        let finishes = dao.finishes.lock().unwrap();
        assert_eq!(finishes.len(), 1);
        assert_eq!(finishes[0].2.status, ThemeRunStatus::Failed);
        assert_eq!(finishes[0].2.versions_created, 1);
        assert!(
            finishes[0]
                .2
                .error_message
                .as_deref()
                .is_some_and(|m| m.contains(&failing_theme_id))
        );

        let versions = dao.versions.lock().unwrap();
        assert_eq!(versions.len(), 1);
        assert_eq!(versions[0].theme_id, fx.cluster_themes[0].0);
        assert!(dao.attempts.lock().unwrap().is_empty());
    }

    #[tokio::test]
    async fn regeneration_error_fails_run() {
        let fx = build_clusters_fixture(&[3]);
        let knowledge = Arc::new(FakeThemeKnowledgePort::with_sources(fx.sources));
        let article_source = Arc::new(FakeThemeArticleSource::with_articles(fx.articles));
        let ml_port = Arc::new(FakeEmbedCluster::with_fixed(
            fx.embeddings,
            ClusterStoriesResponse {
                clusters: fx.clusters,
                params: StoryClusterParams::default(),
            },
        ));
        let generator = Arc::new(FakeThemeGenerator::with_outcomes(vec![
            Ok(ThemeGenerateOutcome::Success(ThemeGenerateResponse {
                title: "Initial Theme".to_string(),
                key_points: vec![ThemeSentenceOutput {
                    text: "Needs regen [1]".to_string(),
                    refs: vec![1],
                }],
                timeline: vec![],
                what_changed: vec![],
                open_questions: vec![],
                model: "gemma4-e4b".to_string(),
            })),
            Err(anyhow::anyhow!("news-creator timeout on regen")),
        ]));
        let verifier = Arc::new(SequenceCardVerifier::new(vec![Ok(
            failing_verify_response(1),
        )]));
        let dao = Arc::new(MockThemesPipelineDao::default());

        let pipeline = ThemesPipeline::new(
            knowledge,
            article_source,
            ml_port,
            generator,
            verifier,
            dao.clone(),
        );

        let err = pipeline
            .run(Uuid::new_v4(), owner(), run_clock(), &test_params())
            .await
            .expect_err("regeneration error must fail the run");
        let msg = format!("{err:#}");
        let theme_id_str = fx.cluster_themes[0].0.to_string();
        assert!(msg.contains(&theme_id_str), "unexpected error: {msg}");
        assert!(msg.contains("news-creator timeout on regen"));

        let finishes = dao.finishes.lock().unwrap();
        assert_eq!(finishes.len(), 1);
        assert_eq!(finishes[0].2.status, ThemeRunStatus::Failed);
        assert!(dao.attempts.lock().unwrap().is_empty());
    }

    #[tokio::test]
    async fn verification_transport_error_fails_run_and_preserves_earlier_versions() {
        let fx = build_clusters_fixture(&[4, 3]);
        let knowledge = Arc::new(FakeThemeKnowledgePort::with_sources(fx.sources));
        let article_source = Arc::new(FakeThemeArticleSource::with_articles(fx.articles));
        let ml_port = Arc::new(FakeEmbedCluster::with_fixed(
            fx.embeddings,
            ClusterStoriesResponse {
                clusters: fx.clusters,
                params: StoryClusterParams::default(),
            },
        ));
        let generator = Arc::new(FakeThemeGenerator::new());
        let verifier = Arc::new(SequenceCardVerifier::new(vec![
            Ok(passing_verify_response(4)),
            Err(anyhow::anyhow!("subworker connection refused")),
        ]));
        let dao = Arc::new(MockThemesPipelineDao::default());

        let pipeline = ThemesPipeline::new(
            knowledge,
            article_source,
            ml_port,
            generator,
            verifier,
            dao.clone(),
        );

        let err = pipeline
            .run(Uuid::new_v4(), owner(), run_clock(), &test_params())
            .await
            .expect_err("verification transport error must fail the run");
        let msg = format!("{err:#}");
        let failing_theme_id = fx.cluster_themes[1].0.to_string();
        assert!(msg.contains(&failing_theme_id), "unexpected error: {msg}");
        assert!(msg.contains("subworker connection refused"));

        let finishes = dao.finishes.lock().unwrap();
        assert_eq!(finishes.len(), 1);
        assert_eq!(finishes[0].2.status, ThemeRunStatus::Failed);
        assert_eq!(finishes[0].2.versions_created, 1);
        assert!(
            finishes[0]
                .2
                .error_message
                .as_deref()
                .is_some_and(|m| m.contains("subworker connection refused"))
        );

        let versions = dao.versions.lock().unwrap();
        assert_eq!(versions.len(), 1);
        assert_eq!(versions[0].theme_id, fx.cluster_themes[0].0);
        assert!(dao.attempts.lock().unwrap().is_empty());
    }

    #[tokio::test]
    async fn prior_failed_attempt_defers_largest_candidate_and_ordering_is_deterministic() {
        let fx = build_clusters_fixture(&[6, 5, 4, 3, 3, 3]);
        let largest_theme_id = fx.cluster_themes[0].0;
        let largest_hash = fx.cluster_themes[0].1.clone();

        let dao = Arc::new(MockThemesPipelineDao::default());
        dao.seed_attempt(NewThemeGenerationAttempt {
            attempt_id: Uuid::new_v4(),
            run_id: Uuid::new_v4(),
            user_id: owner().user_id,
            theme_id: largest_theme_id,
            membership_hash: largest_hash,
            outcome: ThemeAttemptOutcome::GenerationRejected,
            created_at: run_clock() - chrono::Duration::days(1),
        });

        let knowledge = Arc::new(FakeThemeKnowledgePort::with_sources(fx.sources));
        let article_source = Arc::new(FakeThemeArticleSource::with_articles(fx.articles));
        let ml_port = Arc::new(FakeEmbedCluster::with_fixed(
            fx.embeddings,
            ClusterStoriesResponse {
                clusters: fx.clusters,
                params: StoryClusterParams::default(),
            },
        ));
        let generator = Arc::new(FakeThemeGenerator::new());
        let verifier = Arc::new(FakeCardVerifier::new());

        let pipeline = ThemesPipeline::new(
            knowledge,
            article_source,
            ml_port,
            generator.clone(),
            verifier,
            dao.clone(),
        );

        let res = pipeline
            .run(Uuid::new_v4(), owner(), run_clock(), &test_params())
            .await
            .expect("run succeeds");
        assert_eq!(res.clusters_eligible, 6);
        assert_eq!(res.versions_created, 5);
        assert_eq!(res.versions_deferred, 1);

        let generated_ids: Vec<Uuid> = generator
            .requests
            .lock()
            .unwrap()
            .iter()
            .map(|r| r.theme_id)
            .collect();
        assert!(!generated_ids.contains(&largest_theme_id));
        // Zero-attempt candidates are ordered by member count DESC, then theme_id ASC.
        assert_eq!(generated_ids[0], fx.cluster_themes[1].0);
        assert_eq!(generated_ids[1], fx.cluster_themes[2].0);
        let mut tie_ids = vec![
            fx.cluster_themes[3].0,
            fx.cluster_themes[4].0,
            fx.cluster_themes[5].0,
        ];
        tie_ids.sort();
        assert_eq!(generated_ids[2..5], tie_ids);
    }

    #[tokio::test]
    async fn rejected_and_verification_dropped_record_attempts_while_created_does_not() {
        let fx = build_clusters_fixture(&[5, 4, 3]);
        let knowledge = Arc::new(FakeThemeKnowledgePort::with_sources(fx.sources));
        let article_source = Arc::new(FakeThemeArticleSource::with_articles(fx.articles));
        let ml_port = Arc::new(FakeEmbedCluster::with_fixed(
            fx.embeddings,
            ClusterStoriesResponse {
                clusters: fx.clusters,
                params: StoryClusterParams::default(),
            },
        ));

        let ok_resp = || {
            Ok(ThemeGenerateOutcome::Success(ThemeGenerateResponse {
                title: "Theme".to_string(),
                key_points: vec![ThemeSentenceOutput {
                    text: "Point [1]".to_string(),
                    refs: vec![1],
                }],
                timeline: vec![],
                what_changed: vec![],
                open_questions: vec![],
                model: "gemma4-e4b".to_string(),
            }))
        };
        let generator = Arc::new(FakeThemeGenerator::with_outcomes(vec![
            Ok(ThemeGenerateOutcome::Rejected(CardGenerate422Response {
                reason: "nothing_usable".to_string(),
                attempts: 2,
                raw_text: "bad".to_string(),
                detail: None,
            })),
            ok_resp(),
            ok_resp(),
            ok_resp(),
        ]));
        let verifier = Arc::new(SequenceCardVerifier::new(vec![
            Ok(failing_verify_response(1)),
            Ok(failing_verify_response(1)),
            Ok(passing_verify_response(1)),
        ]));
        let dao = Arc::new(MockThemesPipelineDao::default());

        let pipeline = ThemesPipeline::new(
            knowledge,
            article_source,
            ml_port,
            generator,
            verifier,
            dao.clone(),
        );

        let run_id = Uuid::new_v4();
        let clock = run_clock();
        let res = pipeline
            .run(run_id, owner(), clock, &test_params())
            .await
            .expect("run succeeds");

        assert_eq!(res.generation_rejected, 1);
        assert_eq!(res.verification_dropped, 1);
        assert_eq!(res.versions_created, 1);
        assert_eq!(res.publish_failures, 0);

        let attempts = dao.attempts.lock().unwrap();
        assert_eq!(attempts.len(), 2);
        assert_eq!(attempts[0].run_id, run_id);
        assert_eq!(attempts[0].user_id, owner().user_id);
        assert_eq!(attempts[0].theme_id, fx.cluster_themes[0].0);
        assert_eq!(attempts[0].membership_hash, fx.cluster_themes[0].1);
        assert_eq!(attempts[0].outcome, ThemeAttemptOutcome::GenerationRejected);
        assert_eq!(attempts[0].created_at, clock);

        assert_eq!(attempts[1].run_id, run_id);
        assert_eq!(attempts[1].user_id, owner().user_id);
        assert_eq!(attempts[1].theme_id, fx.cluster_themes[1].0);
        assert_eq!(attempts[1].membership_hash, fx.cluster_themes[1].1);
        assert_eq!(
            attempts[1].outcome,
            ThemeAttemptOutcome::VerificationDropped
        );
        assert_eq!(attempts[1].created_at, clock);

        let versions = dao.versions.lock().unwrap();
        assert_eq!(versions.len(), 1);
        assert_eq!(versions[0].theme_id, fx.cluster_themes[2].0);

        let finishes = dao.finishes.lock().unwrap();
        assert_eq!(finishes[0].2.generation_rejected, 1);
        assert_eq!(finishes[0].2.verification_dropped, 1);
        assert_eq!(finishes[0].2.publish_failures, 0);
        assert_eq!(finishes[0].2.versions_created, 1);
    }

    #[tokio::test]
    async fn changed_membership_resets_prior_attempt_count_to_zero() {
        let fx = build_clusters_fixture(&[6, 4, 4, 4, 4, 3]);
        let largest_theme_id = fx.cluster_themes[0].0;
        let dao = Arc::new(MockThemesPipelineDao::default());
        dao.seed_attempt(NewThemeGenerationAttempt {
            attempt_id: Uuid::new_v4(),
            run_id: Uuid::new_v4(),
            user_id: owner().user_id,
            theme_id: largest_theme_id,
            membership_hash: "stale-old-membership-hash".to_string(),
            outcome: ThemeAttemptOutcome::GenerationRejected,
            created_at: run_clock() - chrono::Duration::days(1),
        });

        let knowledge = Arc::new(FakeThemeKnowledgePort::with_sources(fx.sources));
        let article_source = Arc::new(FakeThemeArticleSource::with_articles(fx.articles));
        let ml_port = Arc::new(FakeEmbedCluster::with_fixed(
            fx.embeddings,
            ClusterStoriesResponse {
                clusters: fx.clusters,
                params: StoryClusterParams::default(),
            },
        ));
        let generator = Arc::new(FakeThemeGenerator::new());
        let verifier = Arc::new(FakeCardVerifier::new());

        let pipeline = ThemesPipeline::new(
            knowledge,
            article_source,
            ml_port,
            generator.clone(),
            verifier,
            dao,
        );

        let res = pipeline
            .run(Uuid::new_v4(), owner(), run_clock(), &test_params())
            .await
            .expect("run succeeds");
        assert_eq!(res.versions_created, 5);
        assert_eq!(res.versions_deferred, 1);

        let generated_ids: Vec<Uuid> = generator
            .requests
            .lock()
            .unwrap()
            .iter()
            .map(|r| r.theme_id)
            .collect();
        assert_eq!(generated_ids[0], largest_theme_id);
        assert!(!generated_ids.contains(&fx.cluster_themes[5].0));
    }

    #[tokio::test]
    async fn failed_run_persists_partial_counters_in_theme_run_finish() {
        let fx = build_clusters_fixture(&[5, 4, 3]);
        let knowledge = Arc::new(FakeThemeKnowledgePort::with_sources(fx.sources));
        knowledge.set_fail_all_publishes(true);
        let article_source = Arc::new(FakeThemeArticleSource::with_articles(fx.articles));
        let ml_port = Arc::new(FakeEmbedCluster::with_fixed(
            fx.embeddings,
            ClusterStoriesResponse {
                clusters: fx.clusters,
                params: StoryClusterParams::default(),
            },
        ));
        let generator = Arc::new(FakeThemeGenerator::with_outcomes(vec![
            Ok(ThemeGenerateOutcome::Rejected(CardGenerate422Response {
                reason: "nothing_usable".to_string(),
                attempts: 1,
                raw_text: "bad".to_string(),
                detail: None,
            })),
            Ok(ThemeGenerateOutcome::Success(ThemeGenerateResponse {
                title: "Theme Two".to_string(),
                key_points: vec![ThemeSentenceOutput {
                    text: "Point [1]".to_string(),
                    refs: vec![1],
                }],
                timeline: vec![],
                what_changed: vec![],
                open_questions: vec![],
                model: "gemma4-e4b".to_string(),
            })),
            Err(anyhow::anyhow!("downstream 500")),
        ]));
        let verifier = Arc::new(FakeCardVerifier::new());
        let dao = Arc::new(MockThemesPipelineDao::default());

        let pipeline = ThemesPipeline::new(
            knowledge,
            article_source,
            ml_port,
            generator,
            verifier,
            dao.clone(),
        );

        let err = pipeline
            .run(Uuid::new_v4(), owner(), run_clock(), &test_params())
            .await
            .expect_err("downstream 500 must fail run");
        assert!(format!("{err:#}").contains("downstream 500"));

        let finishes = dao.finishes.lock().unwrap();
        assert_eq!(finishes.len(), 1);
        assert_eq!(finishes[0].2.status, ThemeRunStatus::Failed);
        assert_eq!(finishes[0].2.source_count, 12);
        assert_eq!(finishes[0].2.versions_created, 1);
        assert_eq!(finishes[0].2.generation_rejected, 1);
        assert_eq!(finishes[0].2.verification_dropped, 0);
        assert_eq!(finishes[0].2.publish_failures, 1);
    }

    #[tokio::test]
    async fn themes_runner_logs_and_completes_cleanly() {
        let fx = build_clusters_fixture(&[3]);
        let knowledge = Arc::new(FakeThemeKnowledgePort::with_sources(fx.sources));
        let article_source = Arc::new(FakeThemeArticleSource::with_articles(fx.articles));
        let ml_port = Arc::new(FakeEmbedCluster::with_fixed(
            fx.embeddings,
            ClusterStoriesResponse {
                clusters: fx.clusters,
                params: StoryClusterParams::default(),
            },
        ));
        let generator = Arc::new(FakeThemeGenerator::new());
        let verifier = Arc::new(FakeCardVerifier::new());
        let dao = Arc::new(MockThemesPipelineDao::default());

        let pipeline = ThemesPipeline::new(
            knowledge,
            article_source,
            ml_port,
            generator,
            verifier,
            dao.clone(),
        );
        let runner = ThemesRunner::new(pipeline, owner());
        runner
            .run_themes(Uuid::new_v4(), run_clock())
            .await
            .expect("runner succeeds");
        assert_eq!(dao.finishes.lock().unwrap().len(), 1);
    }
}
