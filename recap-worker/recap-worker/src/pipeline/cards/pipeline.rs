//! CardsPipeline - standalone pipeline for topic cards selection.

use anyhow::{Context, Result};
use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use serde_json::json;
use std::collections::HashMap;
use std::sync::Arc;
use std::time::Instant;
use tracing::info;
use uuid::Uuid;

use super::dedup::{deduplicate, deduplicate_near_duplicates};
use super::noise::{NoiseStats, check_noise};
use super::normalize::{NormalizedItem, normalize_feed};
use super::params::CardsParams;
use super::ports::{CardGenerator, CardVerifier, EmbedCluster, FeedSource, GenreTagger};
use super::rank::{RankCandidatesArgs, compute_cluster_fingerprint, rank_candidates};
use crate::clients::alt_backend::AltBackendFeed;
use crate::clients::news_creator::models::{
    CardContent, CardGenerateOutcome, CardGenerateRequest, CardItemInput,
};
use crate::clients::subworker::cards::{
    ClusterOutput, ClusterStoriesRequest, ClusterStoriesResponse, ClusterStoryItem,
    StoryClusterParams, VerifyCardRequest, VerifyItemInput, VerifySentenceInput, VerifyThresholds,
};
use crate::store::dao::cards::{
    PreviousCardSummary, PreviousCardsJob, RecapCard, RecapCardCandidate, RecapCardJobStats,
    RecapCardSnapshot, RecapEvalWindow,
};
use crate::store::dao::impls::UnifiedDao;
use crate::store::dao::traits::JobDao;
use crate::store::dao::types::JobStatus;

/// Result summary of running the front half of the CardsPipeline.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct CardsPipelineResult {
    pub items_fetched: usize,
    pub items_after_noise: usize,
    pub items_after_dedup: usize,
    pub clusters: usize,
    pub candidates: usize,
    #[serde(default)]
    pub cards_selected: usize,
    #[serde(default)]
    pub embed_cache_hits: usize,
    #[serde(default)]
    pub embed_cache_misses: usize,
}

/// Compute cache key hash for an embedding input string (xxh3-64 hex).
pub fn compute_embedding_text_hash(text: &str) -> String {
    format!("{:016x}", xxhash_rust::xxh3::xxh3_64(text.as_bytes()))
}

/// Result returned from running the pipeline in replay mode.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ReplayResult {
    pub window_id: Uuid,
    pub job_id: Uuid,
    pub stats: RecapCardJobStats,
}

/// DAO trait required by CardsPipeline for job creation, atomic persistence, and status history.
#[async_trait::async_trait]
pub trait CardsPipelineDao: Send + Sync {
    async fn create_job_with_lock_and_window(
        &self,
        job_id: Uuid,
        note: Option<&str>,
        window_days: u32,
        trigger_source: &str,
    ) -> Result<Option<Uuid>>;

    async fn get_latest_completed_cards_job(&self) -> Result<Option<PreviousCardsJob>>;

    async fn get_cached_embeddings(
        &self,
        model: &str,
        text_hashes: &[String],
    ) -> Result<HashMap<String, Vec<f32>>>;

    async fn insert_embeddings(
        &self,
        model: &str,
        dim: usize,
        entries: &[(String, Vec<f32>)],
    ) -> Result<()>;

    async fn persist_pipeline_output(
        &self,
        snapshot: &RecapCardSnapshot,
        candidates: &[RecapCardCandidate],
        cards: &[RecapCard],
        stats: &RecapCardJobStats,
        eval_window: Option<&RecapEvalWindow>,
    ) -> Result<()>;

    async fn update_job_status_with_history(
        &self,
        job_id: Uuid,
        status: JobStatus,
        last_stage: Option<&str>,
        reason: Option<&str>,
    ) -> Result<()>;
}

#[allow(clippy::too_many_lines)]
#[async_trait::async_trait]
impl CardsPipelineDao for UnifiedDao {
    async fn create_job_with_lock_and_window(
        &self,
        job_id: Uuid,
        note: Option<&str>,
        window_days: u32,
        trigger_source: &str,
    ) -> Result<Option<Uuid>> {
        JobDao::create_job_with_lock_and_window(self, job_id, note, window_days, trigger_source)
            .await
            .map_err(|e| anyhow::anyhow!("{e}"))
    }

    async fn get_latest_completed_cards_job(&self) -> Result<Option<PreviousCardsJob>> {
        crate::store::dao::cards::CardsDaoOps::get_latest_completed_cards_job(self.pool())
            .await
            .map_err(|e| anyhow::anyhow!("{e}"))
    }

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

    async fn persist_pipeline_output(
        &self,
        snapshot: &RecapCardSnapshot,
        candidates: &[RecapCardCandidate],
        cards: &[RecapCard],
        stats: &RecapCardJobStats,
        eval_window: Option<&RecapEvalWindow>,
    ) -> Result<()> {
        let mut tx = self
            .pool()
            .begin()
            .await
            .map_err(|e| anyhow::anyhow!("failed to begin db tx: {e}"))?;

        // 1. Snapshot
        sqlx::query(
            r"
            INSERT INTO recap_card_snapshots (
                job_id, from_ts, to_ts, feed_ids, read_feed_ids,
                previous_job_id, previous_cards, params_version, params
            ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
            ",
        )
        .bind(snapshot.job_id)
        .bind(snapshot.from_ts)
        .bind(snapshot.to_ts)
        .bind(&snapshot.feed_ids)
        .bind(&snapshot.read_feed_ids)
        .bind(snapshot.previous_job_id)
        .bind(sqlx::types::Json(&snapshot.previous_cards))
        .bind(&snapshot.params_version)
        .bind(sqlx::types::Json(&snapshot.params))
        .execute(&mut *tx)
        .await
        .map_err(|e| anyhow::anyhow!("failed to insert recap_card_snapshot: {e}"))?;

        // 2. Candidates
        for c in candidates {
            sqlx::query(
                r"
                INSERT INTO recap_card_candidates (
                    id, job_id, rank, cluster_fingerprint, size,
                    domains, items, scores, centroid
                ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
                ",
            )
            .bind(c.id)
            .bind(c.job_id)
            .bind(c.rank)
            .bind(&c.cluster_fingerprint)
            .bind(c.size)
            .bind(sqlx::types::Json(&c.domains))
            .bind(sqlx::types::Json(&c.items))
            .bind(sqlx::types::Json(&c.scores))
            .bind(&c.centroid)
            .execute(&mut *tx)
            .await
            .map_err(|e| anyhow::anyhow!("failed to insert recap_card_candidate: {e}"))?;
        }

        // 2b. Cards
        for c in cards {
            sqlx::query(
                r"
                INSERT INTO recap_cards (
                    id, job_id, rank, story_id, continues_card_id, merged_from,
                    headline_ja, what_ja, why_ja, genre, member_feed_ids,
                    sources, centroid, scores, gates, generation
                ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
                ",
            )
            .bind(c.id)
            .bind(c.job_id)
            .bind(c.rank)
            .bind(c.story_id)
            .bind(c.continues_card_id)
            .bind(c.merged_from.as_deref())
            .bind(&c.headline_ja)
            .bind(&c.what_ja)
            .bind(c.why_ja.as_deref())
            .bind(c.genre.as_deref())
            .bind(&c.member_feed_ids)
            .bind(sqlx::types::Json(&c.sources))
            .bind(c.centroid.as_deref())
            .bind(sqlx::types::Json(&c.scores))
            .bind(sqlx::types::Json(&c.gates))
            .bind(sqlx::types::Json(&c.generation))
            .execute(&mut *tx)
            .await
            .map_err(|e| anyhow::anyhow!("failed to insert recap_card: {e}"))?;
        }

        // 3. Stats
        sqlx::query(
            r"
            INSERT INTO recap_card_job_stats (
                job_id, items_fetched, items_after_noise, items_after_dedup,
                clusters, candidates, cards_selected, cards_dropped,
                embed_ms, cluster_ms, llm_ms, total_ms, params_version, params,
                embed_cache_hits, embed_cache_misses
            ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
            ",
        )
        .bind(stats.job_id)
        .bind(stats.items_fetched)
        .bind(stats.items_after_noise)
        .bind(stats.items_after_dedup)
        .bind(stats.clusters)
        .bind(stats.candidates)
        .bind(stats.cards_selected)
        .bind(sqlx::types::Json(&stats.cards_dropped))
        .bind(stats.embed_ms)
        .bind(stats.cluster_ms)
        .bind(stats.llm_ms)
        .bind(stats.total_ms)
        .bind(&stats.params_version)
        .bind(sqlx::types::Json(&snapshot.params))
        .bind(i32::try_from(stats.embed_cache_hits).unwrap_or(i32::MAX))
        .bind(i32::try_from(stats.embed_cache_misses).unwrap_or(i32::MAX))
        .execute(&mut *tx)
        .await
        .map_err(|e| anyhow::anyhow!("failed to insert recap_card_job_stats: {e}"))?;

        // 4. Eval Window
        if let Some(w) = eval_window {
            sqlx::query(
                r"
                INSERT INTO recap_eval_windows (
                    id, from_ts, to_ts, snapshot_job_id
                ) VALUES ($1, $2, $3, $4)
                ",
            )
            .bind(w.id)
            .bind(w.from_ts)
            .bind(w.to_ts)
            .bind(w.snapshot_job_id)
            .execute(&mut *tx)
            .await
            .map_err(|e| anyhow::anyhow!("failed to insert recap_eval_window: {e}"))?;
        }

        tx.commit()
            .await
            .map_err(|e| anyhow::anyhow!("failed to commit pipeline output tx: {e}"))?;

        Ok(())
    }

    async fn update_job_status_with_history(
        &self,
        job_id: Uuid,
        status: JobStatus,
        last_stage: Option<&str>,
        reason: Option<&str>,
    ) -> Result<()> {
        JobDao::update_job_status_with_history(self, job_id, status, last_stage, reason)
            .await
            .map_err(|e| anyhow::anyhow!("{e}"))
    }
}

struct StageCounts {
    items_fetched: usize,
    items_after_noise: usize,
    items_after_dedup: usize,
    clusters: usize,
    candidates: usize,
    cards_dropped: serde_json::Value,
    embed_cache_hits: usize,
    embed_cache_misses: usize,
}

struct StageTimings {
    embed: i64,
    cluster: i64,
    llm: i64,
    total: i64,
}

struct PersistArgs<'a> {
    job_id: Uuid,
    from: DateTime<Utc>,
    to: DateTime<Utc>,
    snapshot: &'a RecapCardSnapshot,
    candidates: &'a [RecapCardCandidate],
    cards: &'a [RecapCard],
    counts: StageCounts,
    timings: StageTimings,
    params_version: &'a str,
    replay_window_id: Option<Uuid>,
    created_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum PipelineMode {
    SelectionOnly,
    Full,
}

/// Standalone CardsPipeline runner.
pub struct CardsPipeline {
    feed_source: Arc<dyn FeedSource>,
    ml_port: Arc<dyn EmbedCluster>,
    dao: Arc<dyn CardsPipelineDao>,
    mode: PipelineMode,
    card_generator: Option<Arc<dyn CardGenerator>>,
    card_verifier: Option<Arc<dyn CardVerifier>>,
    genre_tagger: Arc<dyn GenreTagger>,
    user_id: Option<Uuid>,
}

impl CardsPipeline {
    pub fn selection_only(
        feed_source: Arc<dyn FeedSource>,
        ml_port: Arc<dyn EmbedCluster>,
        dao: Arc<dyn CardsPipelineDao>,
        genre_tagger: Arc<dyn GenreTagger>,
    ) -> Self {
        Self {
            feed_source,
            ml_port,
            dao,
            mode: PipelineMode::SelectionOnly,
            card_generator: None,
            card_verifier: None,
            genre_tagger,
            user_id: None,
        }
    }

    #[allow(private_interfaces)]
    pub fn full(
        feed_source: Arc<dyn FeedSource>,
        ml_port: Arc<dyn EmbedCluster>,
        dao: Arc<dyn CardsPipelineDao>,
        card_generator: Arc<dyn CardGenerator>,
        card_verifier: Arc<dyn CardVerifier>,
        genre_tagger: Arc<dyn GenreTagger>,
    ) -> Self {
        Self {
            feed_source,
            ml_port,
            dao,
            mode: PipelineMode::Full,
            card_generator: Some(card_generator),
            card_verifier: Some(card_verifier),
            genre_tagger,
            user_id: None,
        }
    }

    #[must_use]
    pub fn with_user_id(mut self, user_id: Uuid) -> Self {
        self.user_id = Some(user_id);
        self
    }

    /// Execute the CardsPipeline for a given job.
    pub async fn run(
        &self,
        job_id: Uuid,
        from: DateTime<Utc>,
        to: DateTime<Utc>,
        params: &CardsParams,
    ) -> Result<CardsPipelineResult> {
        let stats = self
            .execute_pipeline(job_id, from, to, params, None)
            .await?;
        Ok(CardsPipelineResult {
            items_fetched: usize::try_from(stats.items_fetched).unwrap_or(0),
            items_after_noise: usize::try_from(stats.items_after_noise).unwrap_or(0),
            items_after_dedup: usize::try_from(stats.items_after_dedup).unwrap_or(0),
            clusters: usize::try_from(stats.clusters).unwrap_or(0),
            candidates: usize::try_from(stats.candidates).unwrap_or(0),
            cards_selected: usize::try_from(stats.cards_selected).unwrap_or(0),
            embed_cache_hits: stats.embed_cache_hits,
            embed_cache_misses: stats.embed_cache_misses,
        })
    }

    /// Execute the front half of the CardsPipeline in replay mode, creating an eval window.
    pub async fn run_replay(
        &self,
        from: DateTime<Utc>,
        to: DateTime<Utc>,
        params: &CardsParams,
    ) -> Result<ReplayResult> {
        let job_id = Uuid::new_v4();
        let window_id = Uuid::new_v4();
        let stats = self
            .execute_pipeline(job_id, from, to, params, Some(window_id))
            .await?;
        Ok(ReplayResult {
            window_id,
            job_id,
            stats,
        })
    }

    async fn execute_pipeline(
        &self,
        job_id: Uuid,
        from: DateTime<Utc>,
        to: DateTime<Utc>,
        params: &CardsParams,
        replay_window_id: Option<Uuid>,
    ) -> Result<RecapCardJobStats> {
        let trigger_source = if replay_window_id.is_some() {
            "cards_replay"
        } else {
            "cards"
        };

        // 1. Create job record in recap_jobs with trigger_source and window_days = 3
        let lock = self
            .dao
            .create_job_with_lock_and_window(job_id, Some("cards pipeline"), 3, trigger_source)
            .await
            .context("failed to create recap_jobs row")?;

        if lock.is_none() {
            anyhow::bail!("failed to acquire advisory lock for job {job_id}");
        }

        // 2. Initial status transition to Running
        self.dao
            .update_job_status_with_history(
                job_id,
                JobStatus::Running,
                Some("cards_snapshot"),
                None,
            )
            .await
            .context("failed to transition job to Running")?;

        let total_start = Instant::now();

        // 3. Execute stages with fail-fast status tracking
        match self
            .execute_stages(job_id, from, to, params, replay_window_id, total_start)
            .await
        {
            Ok(stats) => {
                // Propagate status update error
                self.dao
                    .update_job_status_with_history(
                        job_id,
                        JobStatus::Completed,
                        Some("cards_persist"),
                        None,
                    )
                    .await?;
                Ok(stats)
            }
            Err(err) => {
                // On failure, log error if status update fails and return original error
                let err_msg = format!("{err:#}");
                if let Err(status_err) = self
                    .dao
                    .update_job_status_with_history(
                        job_id,
                        JobStatus::Failed,
                        Some("cards_pipeline"),
                        Some(&err_msg),
                    )
                    .await
                {
                    tracing::error!(
                        job_id = %job_id,
                        original_error = %err_msg,
                        status_update_error = %status_err,
                        "failed to update job status to failed after pipeline failure"
                    );
                }
                Err(err)
            }
        }
    }

    async fn fetch_read_items(
        &self,
        job_id: Uuid,
        user_id: Uuid,
        to: DateTime<Utc>,
    ) -> Result<(Vec<Uuid>, Vec<(DateTime<Utc>, String)>, usize)> {
        let since = to - chrono::Duration::days(30);
        let read_feed_ids = self
            .feed_source
            .get_all_read_feed_ids(user_id, Some(since))
            .await
            .context("failed to fetch read feed ids")?;

        if read_feed_ids.is_empty() {
            tracing::warn!(job_id = %job_id, "personal_vector_disabled");
            return Ok((read_feed_ids, Vec::new(), 0));
        }

        let read_window_from = to - chrono::Duration::days(30);
        let candidate_feeds = self
            .feed_source
            .fetch_all_feeds_in_window(read_window_from, to)
            .await
            .context("failed to fetch feeds for personal vector")?;
        let fetched_count = candidate_feeds.len();

        let read_set: std::collections::HashSet<Uuid> = read_feed_ids.iter().copied().collect();
        let mut items: Vec<(DateTime<Utc>, String)> = Vec::new();

        for f in &candidate_feeds {
            let Ok(id) = Uuid::parse_str(&f.id) else {
                continue;
            };
            if !read_set.contains(&id) {
                continue;
            }
            if let Ok(Ok(norm)) = normalize_feed(f) {
                let text = format!("{} — {}", norm.title, norm.lede);
                items.push((norm.pub_date, text));
            }
        }

        if items.is_empty() {
            tracing::warn!(
                job_id = %job_id,
                "no valid items found for read feed ids; personal_vector_disabled"
            );
        }

        Ok((read_feed_ids, items, fetched_count))
    }

    async fn stage_snapshot(
        &self,
        job_id: Uuid,
        from: DateTime<Utc>,
        to: DateTime<Utc>,
        params: &CardsParams,
        created_at: DateTime<Utc>,
    ) -> Result<(
        Vec<AltBackendFeed>,
        RecapCardSnapshot,
        Vec<(DateTime<Utc>, String)>,
        Vec<PreviousCardSummary>,
        Option<DateTime<Utc>>,
    )> {
        let user_id = self
            .user_id
            .ok_or_else(|| anyhow::anyhow!("cards_user_id_missing"))?;

        let feeds = self
            .feed_source
            .fetch_all_feeds_in_window(from, to)
            .await
            .context("failed to fetch feeds in window")?;
        let items_fetched = feeds.len();

        let mut feed_ids = Vec::with_capacity(feeds.len());
        for f in &feeds {
            let id = Uuid::parse_str(&f.id).map_err(|e| {
                anyhow::anyhow!("contract violation: unparseable feed id '{}': {e}", f.id)
            })?;
            feed_ids.push(id);
        }

        let (read_feed_ids, read_items, read_items_fetched) =
            self.fetch_read_items(job_id, user_id, to).await?;

        let (previous_job_id, previous_cards_val, prev_cards_list, previous_job_to) =
            if let Some(prev) = self
                .dao
                .get_latest_completed_cards_job()
                .await
                .context("failed to query latest completed cards job")?
            {
                let cards_json = serde_json::to_value(&prev.cards)
                    .context("failed to serialize previous cards")?;
                (Some(prev.job_id), cards_json, prev.cards, Some(prev.to_ts))
            } else {
                info!(
                    job_id = %job_id,
                    "no previous completed cards job found; novelty will default to 1.0"
                );
                (None, json!([]), Vec::new(), None)
            };

        let snapshot = RecapCardSnapshot {
            job_id,
            from_ts: from,
            to_ts: to,
            feed_ids,
            read_feed_ids: read_feed_ids.clone(),
            previous_job_id,
            previous_cards: previous_cards_val,
            params_version: params.params_version.clone(),
            params: serde_json::to_value(params)
                .context("failed to serialize cards params for snapshot")?,
            created_at,
        };

        info!(
            job_id = %job_id,
            items_fetched,
            read_feed_ids_count = read_feed_ids.len(),
            read_items_fetched,
            has_previous_job = previous_job_id.is_some(),
            "cards snapshot staged in memory"
        );
        Ok((
            feeds,
            snapshot,
            read_items,
            prev_cards_list,
            previous_job_to,
        ))
    }

    async fn embed_with_cache(
        &self,
        job_id: Uuid,
        texts: &[String],
        params: &CardsParams,
    ) -> Result<(Vec<Vec<f32>>, usize, usize)> {
        if texts.is_empty() {
            return Ok((Vec::new(), 0, 0));
        }

        let text_hashes: Vec<String> = texts
            .iter()
            .map(|t| compute_embedding_text_hash(t))
            .collect();

        let cached = self
            .dao
            .get_cached_embeddings(&params.expected_embed_model, &text_hashes)
            .await
            .context("failed to query embedding cache")?;

        for (h, vec) in &cached {
            if vec.len() != params.expected_embed_dim {
                anyhow::bail!(
                    "cached embedding dimension mismatch for hash {h}: expected {}, got {}",
                    params.expected_embed_dim,
                    vec.len()
                );
            }
        }

        let mut misses_indices = Vec::new();
        let mut misses_texts = Vec::new();
        for (i, hash) in text_hashes.iter().enumerate() {
            if !cached.contains_key(hash) {
                misses_indices.push(i);
                misses_texts.push(texts[i].clone());
            }
        }

        let hits_count = texts.len() - misses_texts.len();
        let misses_count = misses_texts.len();

        let new_embeddings = if misses_texts.is_empty() {
            Vec::new()
        } else {
            let embs = self
                .ml_port
                .embed(&misses_texts)
                .await
                .context("failed to generate ML embeddings for cache misses")?;

            if embs.len() != misses_texts.len() {
                anyhow::bail!(
                    "subworker embedding count mismatch for cache misses: expected {}, got {}",
                    misses_texts.len(),
                    embs.len()
                );
            }
            embs
        };

        if !new_embeddings.is_empty() {
            let mut entries_to_insert = Vec::with_capacity(new_embeddings.len());
            let mut seen_hashes = std::collections::HashSet::new();
            for (&idx, emb) in misses_indices.iter().zip(new_embeddings.iter()) {
                let hash = &text_hashes[idx];
                if seen_hashes.insert(hash.clone()) {
                    entries_to_insert.push((hash.clone(), emb.clone()));
                }
            }
            let insert_dim = new_embeddings
                .first()
                .map_or(params.expected_embed_dim, Vec::len);
            self.dao
                .insert_embeddings(&params.expected_embed_model, insert_dim, &entries_to_insert)
                .await
                .context("failed to insert new embeddings into cache")?;
        }

        let mut final_embeddings = Vec::with_capacity(texts.len());
        let mut miss_map: HashMap<usize, Vec<f32>> = HashMap::new();
        for (&idx, emb) in misses_indices.iter().zip(new_embeddings) {
            miss_map.insert(idx, emb);
        }

        for (i, hash) in text_hashes.iter().enumerate() {
            if let Some(emb) = cached.get(hash) {
                final_embeddings.push(emb.clone());
            } else if let Some(emb) = miss_map.remove(&i) {
                final_embeddings.push(emb);
            } else {
                anyhow::bail!("internal error: missing embedding for index {i}");
            }
        }

        tracing::debug!(
            job_id = %job_id,
            total_texts = texts.len(),
            cache_hits = hits_count,
            cache_misses = misses_count,
            "embed_with_cache completed"
        );

        Ok((final_embeddings, hits_count, misses_count))
    }

    async fn stage_personal_vector(
        &self,
        job_id: Uuid,
        read_items: &[(DateTime<Utc>, String)],
        to: DateTime<Utc>,
        params: &CardsParams,
    ) -> Result<(Option<Vec<f32>>, usize, usize)> {
        if read_items.is_empty() {
            return Ok((None, 0, 0));
        }

        let texts: Vec<String> = read_items.iter().map(|(_, t)| t.clone()).collect();
        let (embeddings, hits, misses) = self
            .embed_with_cache(job_id, &texts, params)
            .await
            .context("failed to embed read items for personal vector")?;

        if embeddings.is_empty() || embeddings.len() != read_items.len() {
            anyhow::bail!("embedding count mismatch for read items");
        }

        let dim = embeddings[0].len();
        let mut u = vec![0.0_f32; dim];
        let mut total_w = 0.0_f32;

        for ((pub_date, _), emb) in read_items.iter().zip(embeddings.iter()) {
            let delta_secs = (to - *pub_date).num_seconds().max(0);
            let delta_days = delta_secs as f32 / 86400.0;
            let w = (-delta_days / params.recency_tau_days).exp();
            total_w += w;
            for k in 0..dim {
                u[k] += w * emb[k];
            }
        }

        if total_w > 1e-9 {
            for val in &mut u {
                *val /= total_w;
            }
        }

        let norm_sq: f32 = u.iter().map(|x| x * x).sum();
        let norm = norm_sq.sqrt();
        if norm > 1e-9 {
            for val in &mut u {
                *val /= norm;
            }
        }

        info!(
            job_id = %job_id,
            read_items_count = read_items.len(),
            embed_cache_hits = hits,
            embed_cache_misses = misses,
            "computed personal vector"
        );

        Ok((Some(u), hits, misses))
    }

    fn stage_normalize_and_noise(
        job_id: Uuid,
        feeds: &[AltBackendFeed],
    ) -> Result<Vec<NormalizedItem>> {
        let mut normalized = Vec::new();
        let mut noise_stats = NoiseStats::default();

        for feed in feeds {
            match normalize_feed(feed)? {
                Ok(item) => match check_noise(&item) {
                    Some(rule) => noise_stats.record_dropped(rule),
                    None => normalized.push(item),
                },
                Err(rule) => noise_stats.record_dropped(rule),
            }
        }

        let items_after_noise = normalized.len();
        info!(
            job_id = %job_id,
            dropped_title_regex = noise_stats.dropped_title_regex,
            dropped_language = noise_stats.dropped_language,
            dropped_short_lede = noise_stats.dropped_short_lede,
            dropped_no_date = noise_stats.dropped_no_date,
            dropped_html_strip_failed = noise_stats.dropped_html_strip_failed,
            total_dropped = noise_stats.total_dropped,
            items_after_noise,
            "cards normalize and noise filtering complete"
        );
        Ok(normalized)
    }

    async fn stage_embed(
        &self,
        job_id: Uuid,
        deduped: &[NormalizedItem],
        params: &CardsParams,
    ) -> Result<(Vec<Vec<f32>>, i64, usize, usize)> {
        let embed_start = Instant::now();
        let texts_to_embed: Vec<String> = deduped
            .iter()
            .map(|it| format!("{} — {}", it.title, it.lede))
            .collect();

        let (embeddings, hits, misses) = self
            .embed_with_cache(job_id, &texts_to_embed, params)
            .await?;
        let embed_ms = i64::try_from(embed_start.elapsed().as_millis()).unwrap_or(i64::MAX);

        info!(
            job_id = %job_id,
            embeddings_count = embeddings.len(),
            embed_cache_hits = hits,
            embed_cache_misses = misses,
            embed_ms,
            "cards embedding complete"
        );
        Ok((embeddings, embed_ms, hits, misses))
    }

    async fn stage_cluster(
        &self,
        job_id: Uuid,
        deduped: &[NormalizedItem],
        embeddings: &[Vec<f32>],
        params: &CardsParams,
    ) -> Result<(ClusterStoriesResponse, i64)> {
        let cluster_start = Instant::now();
        let cluster_items: Vec<ClusterStoryItem> = deduped
            .iter()
            .zip(embeddings.iter())
            .map(|(it, emb)| ClusterStoryItem {
                id: it.feed_id,
                embedding: emb.clone(),
                published_at: it.pub_date,
                language: Some(it.language.clone()),
            })
            .collect();

        let cluster_req = ClusterStoriesRequest {
            items: cluster_items,
            params: StoryClusterParams {
                threshold: params.threshold,
                linkage: params.linkage.clone(),
                time_decay_per_day: params.time_decay_per_day,
                min_cluster_size: params.min_cluster_size,
                min_cluster_size_by_language: Some(params.min_cluster_size_by_language.clone()),
            },
        };

        let cluster_resp = if cluster_req.items.is_empty() {
            ClusterStoriesResponse {
                clusters: Vec::new(),
                params: cluster_req.params,
            }
        } else {
            self.ml_port
                .cluster_stories(&cluster_req)
                .await
                .context("failed to cluster stories")?
        };
        let cluster_ms = i64::try_from(cluster_start.elapsed().as_millis()).unwrap_or(i64::MAX);
        let clusters_count = cluster_resp.clusters.len();
        info!(
            job_id = %job_id,
            clusters = clusters_count,
            cluster_ms,
            "cards clustering complete"
        );
        Ok((cluster_resp, cluster_ms))
    }

    async fn stage_persist(&self, args: PersistArgs<'_>) -> Result<RecapCardJobStats> {
        let stats = RecapCardJobStats {
            job_id: args.job_id,
            items_fetched: i32::try_from(args.counts.items_fetched).unwrap_or(i32::MAX),
            items_after_noise: i32::try_from(args.counts.items_after_noise).unwrap_or(i32::MAX),
            items_after_dedup: i32::try_from(args.counts.items_after_dedup).unwrap_or(i32::MAX),
            clusters: i32::try_from(args.counts.clusters).unwrap_or(i32::MAX),
            candidates: i32::try_from(args.counts.candidates).unwrap_or(i32::MAX),
            cards_selected: i32::try_from(args.cards.len()).unwrap_or(i32::MAX),
            cards_dropped: args.counts.cards_dropped,
            embed_ms: args.timings.embed,
            cluster_ms: args.timings.cluster,
            llm_ms: args.timings.llm,
            total_ms: args.timings.total,
            params_version: args.params_version.to_string(),
            embed_cache_hits: args.counts.embed_cache_hits,
            embed_cache_misses: args.counts.embed_cache_misses,
            created_at: args.created_at,
        };

        let eval_window = args.replay_window_id.map(|win_id| RecapEvalWindow {
            id: win_id,
            from_ts: args.from,
            to_ts: args.to,
            snapshot_job_id: args.job_id,
            created_at: args.created_at,
        });

        self.dao
            .persist_pipeline_output(
                args.snapshot,
                args.candidates,
                args.cards,
                &stats,
                eval_window.as_ref(),
            )
            .await
            .context("failed to atomically persist pipeline output")?;

        info!(job_id = %args.job_id, "cards pipeline output atomically persisted");
        Ok(stats)
    }

    #[allow(clippy::too_many_arguments, clippy::too_many_lines)]
    async fn stage_generation_and_gates(
        &self,
        job_id: Uuid,
        candidates: &[RecapCardCandidate],
        deduped_items: &[NormalizedItem],
        feeds: &[AltBackendFeed],
        cluster_resp: &ClusterStoriesResponse,
        params: &CardsParams,
        job_created_at: DateTime<Utc>,
        card_generator: Arc<dyn CardGenerator>,
        card_verifier: Arc<dyn CardVerifier>,
    ) -> Result<(Vec<RecapCard>, serde_json::Value, i64)> {
        let llm_start = Instant::now();
        let mut item_by_id: HashMap<Uuid, &NormalizedItem> = HashMap::new();
        for item in deduped_items {
            item_by_id.insert(item.feed_id, item);
        }

        let mut feed_by_id: HashMap<Uuid, &AltBackendFeed> = HashMap::new();
        for f in feeds {
            if let Ok(id) = Uuid::parse_str(&f.id) {
                feed_by_id.insert(id, f);
            }
        }

        let mut cluster_by_fp: HashMap<String, &ClusterOutput> = HashMap::new();
        for c in &cluster_resp.clusters {
            let fp = compute_cluster_fingerprint(&c.member_ids);
            cluster_by_fp.insert(fp, c);
        }

        let mut cards = Vec::new();
        let mut cards_dropped: HashMap<String, usize> = HashMap::new();

        for candidate in candidates.iter().take(12) {
            let candidate_items_val = candidate.items.as_array();
            let mut card_items: Vec<CardItemInput> = Vec::new();
            if let Some(arr) = candidate_items_val {
                for (idx, it_val) in arr.iter().take(6).enumerate() {
                    let feed_id_str =
                        it_val
                            .get("feed_id")
                            .and_then(|v| v.as_str())
                            .ok_or_else(|| {
                                anyhow::anyhow!(
                                    "recap_card_candidate {} item {} missing or invalid feed_id",
                                    candidate.id,
                                    idx
                                )
                            })?;
                    let feed_id = Uuid::parse_str(feed_id_str).map_err(|_| {
                        anyhow::anyhow!(
                            "recap_card_candidate {} item {} missing or invalid feed_id",
                            candidate.id,
                            idx
                        )
                    })?;
                    let (title, host, url, pub_date, lede) =
                        if let Some(norm) = item_by_id.get(&feed_id) {
                            let lede = if norm.host == "dev.to" {
                                feed_by_id
                                    .get(&feed_id)
                                    .and_then(|f| f.description.as_deref())
                                    .and_then(|desc| {
                                        super::normalize::extract_lede_with_limit(desc, 1200)
                                    })
                                    .unwrap_or_else(|| norm.lede.clone())
                            } else {
                                norm.lede.clone()
                            };
                            (
                                norm.title.clone(),
                                norm.host.clone(),
                                norm.url.clone(),
                                Some(norm.pub_date.to_rfc3339()),
                                lede,
                            )
                        } else {
                            let title = it_val
                                .get("title")
                                .and_then(|v| v.as_str())
                                .unwrap_or("")
                                .to_string();
                            let host = it_val
                                .get("host")
                                .and_then(|v| v.as_str())
                                .unwrap_or("")
                                .to_string();
                            let url = it_val
                                .get("url")
                                .and_then(|v| v.as_str())
                                .unwrap_or("")
                                .to_string();
                            let pub_date = it_val
                                .get("pub_date")
                                .and_then(|v| v.as_str())
                                .map(ToString::to_string);
                            (title, host, url, pub_date, String::new())
                        };

                    let n = card_items.len() + 1;
                    card_items.push(CardItemInput {
                        n,
                        feed_id,
                        title,
                        host,
                        url,
                        pub_date,
                        lede,
                    });
                }
            }

            if card_items.is_empty() {
                *cards_dropped.entry("empty_items".to_string()).or_insert(0) += 1;
                continue;
            }

            let mut gen_req = CardGenerateRequest {
                job_id,
                candidate_id: candidate.id,
                prompt_version: "recap_card.v1".to_string(),
                items: card_items.clone(),
                revision_note: None,
            };

            let outcome = card_generator.generate_card(&gen_req).await?;
            let (mut current_card, mut generation_meta) = match outcome {
                CardGenerateOutcome::Success(resp) => (resp.card, resp.generation),
                CardGenerateOutcome::Rejected(resp) => {
                    *cards_dropped.entry(resp.reason).or_insert(0) += 1;
                    continue;
                }
            };

            let mut regeneration_count = 0;

            // Gate G1
            let mut ja_ratio = calculate_ja_ratio(&full_card_text(&current_card));
            if ja_ratio < 0.6 {
                gen_req.revision_note = Some("日本語で書き直す".to_string());
                let outcome = card_generator.generate_card(&gen_req).await?;
                regeneration_count += 1;
                match outcome {
                    CardGenerateOutcome::Success(resp) => {
                        current_card = resp.card;
                        generation_meta = resp.generation;
                        ja_ratio = calculate_ja_ratio(&full_card_text(&current_card));
                        if ja_ratio < 0.6 {
                            *cards_dropped.entry("g1_language".to_string()).or_insert(0) += 1;
                            continue;
                        }
                    }
                    CardGenerateOutcome::Rejected(resp) => {
                        *cards_dropped.entry(resp.reason).or_insert(0) += 1;
                        continue;
                    }
                }
            }

            // Gate G2
            let mut invalid_cites = Vec::new();
            for s in &current_card.what_ja {
                if s.refs.is_empty() {
                    invalid_cites.push("引用なし".to_string());
                } else {
                    for &r in &s.refs {
                        if r < 1 || r > card_items.len() {
                            invalid_cites.push(format!("[{r}]"));
                        }
                    }
                }
            }
            if let Some(ref w) = current_card.why_ja {
                if w.refs.is_empty() {
                    invalid_cites.push("引用なし".to_string());
                } else {
                    for &r in &w.refs {
                        if r < 1 || r > card_items.len() {
                            invalid_cites.push(format!("[{r}]"));
                        }
                    }
                }
            }

            if !invalid_cites.is_empty() {
                gen_req.revision_note = Some(format!("不正な引用: {}", invalid_cites.join(", ")));
                let outcome = card_generator.generate_card(&gen_req).await?;
                regeneration_count += 1;
                match outcome {
                    CardGenerateOutcome::Success(resp) => {
                        current_card = resp.card;
                        generation_meta = resp.generation;
                        ja_ratio = calculate_ja_ratio(&full_card_text(&current_card));
                        if ja_ratio < 0.6 {
                            *cards_dropped.entry("g1_language".to_string()).or_insert(0) += 1;
                            continue;
                        }
                    }
                    CardGenerateOutcome::Rejected(resp) => {
                        *cards_dropped.entry(resp.reason).or_insert(0) += 1;
                        continue;
                    }
                }
            }

            let mut g2_citations_valid = validate_citations(&current_card, card_items.len());
            filter_valid_citations(&mut current_card, card_items.len());
            if current_card.what_ja.is_empty() {
                *cards_dropped.entry("g2_citation".to_string()).or_insert(0) += 1;
                continue;
            }

            // Gate G3 & G4 via card_verifier
            let mut verify_req = build_verify_request(
                job_id,
                candidate.id,
                &current_card,
                &card_items,
                params.tau_a,
            );
            let mut verify_resp = card_verifier.verify_card(&verify_req).await?;

            let mut failing_indices = Vec::new();
            let mut failure_notes = Vec::new();
            for s in &verify_resp.sentences {
                if !s.attribution.pass || !s.filler.pass {
                    failing_indices.push(s.idx);
                    if !s.attribution.pass {
                        failure_notes.push(format!(
                            "文{}: 帰属類似度不足 (cos < {})",
                            s.idx + 1,
                            params.tau_a
                        ));
                    }
                    if !s.filler.pass {
                        failure_notes.push(format!(
                            "文{}: 推測語 [{}] を含めないでください",
                            s.idx + 1,
                            s.filler.matched.join(", ")
                        ));
                    }
                }
            }

            if !failing_indices.is_empty() {
                // Regenerate ONCE with revision_note naming failing sentences
                gen_req.revision_note = Some(failure_notes.join("; "));
                let outcome2 = card_generator.generate_card(&gen_req).await?;
                regeneration_count += 1;
                match outcome2 {
                    CardGenerateOutcome::Success(resp2) => {
                        current_card = resp2.card;
                        generation_meta = resp2.generation;

                        // Recheck G1
                        ja_ratio = calculate_ja_ratio(&full_card_text(&current_card));
                        if ja_ratio < 0.6 {
                            *cards_dropped.entry("g1_language".to_string()).or_insert(0) += 1;
                            continue;
                        }

                        // Recheck G2
                        g2_citations_valid = validate_citations(&current_card, card_items.len());
                        filter_valid_citations(&mut current_card, card_items.len());
                        if current_card.what_ja.is_empty() {
                            *cards_dropped.entry("g2_citation".to_string()).or_insert(0) += 1;
                            continue;
                        }

                        // Re-verify
                        verify_req = build_verify_request(
                            job_id,
                            candidate.id,
                            &current_card,
                            &card_items,
                            params.tau_a,
                        );
                        verify_resp = card_verifier.verify_card(&verify_req).await?;
                    }
                    CardGenerateOutcome::Rejected(resp2) => {
                        *cards_dropped.entry(resp2.reason).or_insert(0) += 1;
                        continue;
                    }
                }
            }

            // Post-verification: delete any remaining failing sentences
            let mut deleted_sentence_indices = Vec::new();
            let mut had_g3_fail = false;
            let mut had_g4_fail = false;
            let mut bad_indices = std::collections::HashSet::new();

            for s in &verify_resp.sentences {
                if !s.attribution.pass {
                    had_g3_fail = true;
                    bad_indices.insert(s.idx);
                }
                if !s.filler.pass {
                    had_g4_fail = true;
                    bad_indices.insert(s.idx);
                }
            }

            if !bad_indices.is_empty() {
                let mut new_what = Vec::new();
                for (i, s) in current_card.what_ja.into_iter().enumerate() {
                    if bad_indices.contains(&i) {
                        deleted_sentence_indices.push(i);
                    } else {
                        new_what.push(s);
                    }
                }
                current_card.what_ja = new_what;

                // Lookup why sentence by kind == "why"
                if let Some(why_sent) = verify_req.sentences.iter().find(|s| s.kind == "why") {
                    if bad_indices.contains(&why_sent.idx) {
                        deleted_sentence_indices.push(why_sent.idx);
                        current_card.why_ja = None;
                    }
                }

                if current_card.what_ja.is_empty() {
                    let reason = if had_g3_fail {
                        "g3_attribution"
                    } else {
                        "g4_filler"
                    };
                    *cards_dropped.entry(reason.to_string()).or_insert(0) += 1;
                    continue;
                }
            }

            // Gate G6: if why_hint_present is false, drop why_ja
            if !verify_resp.why_hint_present {
                current_card.why_ja = None;
            }

            // Gate G5 & gates json
            let gates_json = json!({
                "g1_ja_ratio": ja_ratio,
                "g2_citations_valid": g2_citations_valid,
                "regeneration_count": regeneration_count,
                "deleted_sentence_indices": deleted_sentence_indices,
                "g3_attribution_pass": !had_g3_fail,
                "g4_filler_pass": !had_g4_fail,
                "g5_specificity": verify_resp.sentences.iter().map(|s| {
                    json!({
                        "idx": s.idx,
                        "proper_nouns": s.specificity.proper_nouns,
                        "numbers": s.specificity.numbers,
                        "tokens": s.specificity.tokens,
                        "density": s.specificity.density,
                    })
                }).collect::<Vec<_>>(),
                "g6_why_hint_present": verify_resp.why_hint_present,
                "verification_embedding": {
                    "model": verify_resp.embedding.model,
                    "identity": verify_resp.embedding.identity,
                }
            });

            let card_rank = (cards.len() + 1) as i32;
            let story_id = candidate
                .scores
                .get("story_id")
                .and_then(|v| v.as_str())
                .and_then(|s| Uuid::parse_str(s).ok())
                .context("missing story_id in candidate scores")?;

            let continues_card_id = candidate
                .scores
                .get("continues_card_id")
                .and_then(|v| v.as_str())
                .and_then(|s| Uuid::parse_str(s).ok());

            let merged_from: Option<Vec<Uuid>> = candidate
                .scores
                .get("merged_from")
                .and_then(|v| serde_json::from_value(v.clone()).ok());

            let genre = candidate
                .scores
                .get("genre")
                .and_then(|v| v.as_str())
                .map(ToString::to_string);

            let member_feed_ids = cluster_by_fp
                .get(&candidate.cluster_fingerprint)
                .map(|c| c.member_ids.clone())
                .with_context(|| {
                    format!(
                        "missing cluster for fingerprint {}",
                        candidate.cluster_fingerprint
                    )
                })?;

            let sources = json!(
                card_items
                    .iter()
                    .map(|it| {
                        json!({
                            "n": it.n,
                            "feed_id": it.feed_id,
                            "url": it.url,
                            "host": it.host,
                            "title": it.title,
                            "pub_date": it.pub_date,
                        })
                    })
                    .collect::<Vec<_>>()
            );

            let what_ja_text = current_card
                .what_ja
                .iter()
                .map(|s| s.text.as_str())
                .collect::<Vec<_>>()
                .join(" ");

            let card = RecapCard {
                id: Uuid::new_v4(),
                job_id,
                rank: card_rank,
                story_id,
                continues_card_id,
                merged_from,
                headline_ja: current_card.headline_ja,
                what_ja: what_ja_text,
                why_ja: current_card.why_ja.map(|s| s.text),
                genre,
                member_feed_ids,
                sources,
                centroid: candidate.centroid.clone(),
                scores: candidate.scores.clone(),
                gates: gates_json,
                generation: json!({
                    "model": generation_meta.model,
                    "prompt_version": generation_meta.prompt_version,
                    "cache_hit": generation_meta.cache_hit,
                    "prompt_tokens": generation_meta.prompt_tokens,
                    "completion_tokens": generation_meta.completion_tokens,
                    "ms": generation_meta.ms,
                }),
                created_at: job_created_at,
            };

            cards.push(card);
        }

        let llm_ms = i64::try_from(llm_start.elapsed().as_millis()).unwrap_or(i64::MAX);
        Ok((cards, json!(cards_dropped), llm_ms))
    }

    #[allow(clippy::too_many_lines)]
    async fn execute_stages(
        &self,
        job_id: Uuid,
        from: DateTime<Utc>,
        to: DateTime<Utc>,
        params: &CardsParams,
        replay_window_id: Option<Uuid>,
        total_start: Instant,
    ) -> Result<RecapCardJobStats> {
        // Stage 1: Snapshot
        let (feeds, snapshot, read_items, prev_cards, prev_job_to) =
            self.stage_snapshot(job_id, from, to, params, to).await?;
        let items_fetched = feeds.len();

        // Stage 2 & 3: Normalize & Noise
        let normalized = Self::stage_normalize_and_noise(job_id, &feeds)?;
        let items_after_noise = normalized.len();

        // Stage 4: Dedup (exact)
        let (mut deduped_exact, dedup_dropped) = deduplicate(normalized);
        info!(
            job_id = %job_id,
            items_before_dedup = items_after_noise,
            items_after_exact_dedup = deduped_exact.len(),
            duplicates_dropped = dedup_dropped,
            "cards exact deduplication complete"
        );

        // Stage 4b: Genre Tagging (after exact dedup)
        if params.genre_tagging {
            info!(job_id = %job_id, "genre_tagging_enabled");
            let texts: Vec<String> = deduped_exact
                .iter()
                .map(|item| format!("{} {}", item.title, item.lede))
                .collect();
            let batch_scores = self
                .genre_tagger
                .tag_genres(&texts, params.genre_concurrency)
                .await
                .with_context(|| format!("batch genre classification failed for job {job_id}"))?;
            for (item, scores) in deduped_exact.iter_mut().zip(batch_scores) {
                let mut sorted_scores: Vec<(String, f32)> = scores.into_iter().collect();
                sorted_scores.sort_by(|a, b| b.1.total_cmp(&a.1).then_with(|| a.0.cmp(&b.0)));
                if let Some((genre, score)) = sorted_scores.into_iter().next() {
                    if score >= params.genre_min_confidence {
                        item.genre = Some(genre);
                    } else {
                        item.genre = None;
                    }
                } else {
                    item.genre = None;
                }
            }
        } else {
            info!(job_id = %job_id, "genre_tagging_disabled");
        }

        // Stage 5: Embed
        let (embeddings_exact, embed_ms, embed_hits, embed_misses) =
            self.stage_embed(job_id, &deduped_exact, params).await?;

        // Stage 5b: Near-duplicate Dedup
        let (deduped, embeddings, near_dup_dropped) = deduplicate_near_duplicates(
            deduped_exact,
            embeddings_exact,
            params.near_dup_threshold,
        )?;
        let items_after_dedup = deduped.len();
        info!(
            job_id = %job_id,
            near_duplicates_dropped = near_dup_dropped,
            items_after_dedup,
            "cards near-duplicate deduplication complete"
        );

        // Stage 5c: Personal Vector
        let (personal_vector, pv_hits, pv_misses) = self
            .stage_personal_vector(job_id, &read_items, to, params)
            .await?;

        let total_cache_hits = embed_hits + pv_hits;
        let total_cache_misses = embed_misses + pv_misses;
        info!(
            job_id = %job_id,
            embed_cache_hits = total_cache_hits,
            embed_cache_misses = total_cache_misses,
            "embedding cache totals"
        );

        // Stage 6: Cluster
        let (cluster_resp, cluster_ms) = self
            .stage_cluster(job_id, &deduped, &embeddings, params)
            .await?;

        // Stage 7: Provisional Ranking
        let rank_args = RankCandidatesArgs {
            job_id,
            clusters: &cluster_resp.clusters,
            deduped_items: &deduped,
            personal_vector: personal_vector.as_deref(),
            previous_cards: &prev_cards,
            previous_job_to: prev_job_to,
            alpha: params.alpha,
            theta_novelty: params.theta_novelty,
            created_at: to,
        };
        let candidates = rank_candidates(rank_args)?;
        let candidates_count = candidates.len();
        info!(
            job_id = %job_id,
            candidates = candidates_count,
            "cards ranking complete"
        );

        // Stage 7b: Card Generation & Gates
        let (cards, cards_dropped, llm_ms) = match self.mode {
            PipelineMode::SelectionOnly => {
                info!(job_id = %job_id, "generation_skipped_selection_only");
                (Vec::new(), json!({}), 0)
            }
            PipelineMode::Full => {
                let generator = self
                    .card_generator
                    .as_ref()
                    .context("card_generator required in full mode")?;
                let verifier = self
                    .card_verifier
                    .as_ref()
                    .context("card_verifier required in full mode")?;

                let (cards, dropped, llm_ms) = self
                    .stage_generation_and_gates(
                        job_id,
                        &candidates,
                        &deduped,
                        &feeds,
                        &cluster_resp,
                        params,
                        to,
                        Arc::clone(generator),
                        Arc::clone(verifier),
                    )
                    .await?;

                if cards.len() < 5 {
                    tracing::warn!(
                        job_id = %job_id,
                        cards_selected = cards.len(),
                        "cards_job_degraded"
                    );
                    self.dao
                        .update_job_status_with_history(
                            job_id,
                            JobStatus::Running,
                            Some("cards_job_degraded"),
                            Some("selected cards count below threshold of 5"),
                        )
                        .await?;
                }

                (cards, dropped, llm_ms)
            }
        };

        // Stage 8: Persist
        let stage_counts = StageCounts {
            items_fetched,
            items_after_noise,
            items_after_dedup,
            clusters: cluster_resp.clusters.len(),
            candidates: candidates_count,
            cards_dropped,
            embed_cache_hits: total_cache_hits,
            embed_cache_misses: total_cache_misses,
        };
        let timings = StageTimings {
            embed: embed_ms,
            cluster: cluster_ms,
            llm: llm_ms,
            total: i64::try_from(total_start.elapsed().as_millis()).unwrap_or(i64::MAX),
        };

        self.stage_persist(PersistArgs {
            job_id,
            from,
            to,
            snapshot: &snapshot,
            candidates: &candidates,
            cards: &cards,
            counts: stage_counts,
            timings,
            params_version: &params.params_version,
            replay_window_id,
            created_at: to,
        })
        .await
    }
}

pub fn calculate_ja_ratio(text: &str) -> f32 {
    let mut ja_count = 0;
    let mut total = 0;

    for c in text.chars() {
        if c.is_whitespace() {
            continue;
        }
        total += 1;
        if matches!(c, '\u{3040}'..='\u{309F}' | '\u{30A0}'..='\u{30FF}' | '\u{4E00}'..='\u{9FAF}')
        {
            ja_count += 1;
        }
    }

    if total == 0 {
        0.0
    } else {
        ja_count as f32 / total as f32
    }
}

fn build_verify_request(
    job_id: Uuid,
    card_id: Uuid,
    card: &CardContent,
    items: &[CardItemInput],
    tau_a: f32,
) -> VerifyCardRequest {
    let mut sentences = Vec::new();
    for (i, s) in card.what_ja.iter().enumerate() {
        sentences.push(VerifySentenceInput {
            idx: i,
            kind: "what".to_string(),
            text: s.text.clone(),
            refs: s.refs.iter().map(|&r| r as i64).collect(),
        });
    }
    if let Some(ref w) = card.why_ja {
        sentences.push(VerifySentenceInput {
            idx: card.what_ja.len(),
            kind: "why".to_string(),
            text: w.text.clone(),
            refs: w.refs.iter().map(|&r| r as i64).collect(),
        });
    }

    let verify_items = items
        .iter()
        .map(|it| VerifyItemInput {
            n: it.n as i64,
            title: it.title.clone(),
            lede: it.lede.clone(),
        })
        .collect();

    VerifyCardRequest {
        job_id,
        card_id,
        language: "ja".to_string(),
        sentences,
        items: verify_items,
        thresholds: VerifyThresholds {
            attribution_cos: tau_a,
        },
    }
}

fn validate_citations(card: &CardContent, item_count: usize) -> bool {
    let what_valid = !card.what_ja.is_empty()
        && card
            .what_ja
            .iter()
            .all(|s| !s.refs.is_empty() && s.refs.iter().all(|&r| r >= 1 && r <= item_count));
    let why_valid = match &card.why_ja {
        Some(w) => !w.refs.is_empty() && w.refs.iter().all(|&r| r >= 1 && r <= item_count),
        None => true,
    };
    what_valid && why_valid
}

fn filter_valid_citations(card: &mut CardContent, item_count: usize) {
    card.what_ja
        .retain(|s| !s.refs.is_empty() && s.refs.iter().all(|&r| r >= 1 && r <= item_count));
    if let Some(ref w) = card.why_ja {
        if w.refs.is_empty() || !w.refs.iter().all(|&r| r >= 1 && r <= item_count) {
            card.why_ja = None;
        }
    }
}

fn full_card_text(card: &CardContent) -> String {
    let mut text = card.headline_ja.clone();
    for s in &card.what_ja {
        text.push(' ');
        text.push_str(&s.text);
    }
    if let Some(ref w) = card.why_ja {
        text.push(' ');
        text.push_str(&w.text);
    }
    text
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::clients::alt_backend::AltBackendFeed;
    use crate::clients::news_creator::models::{CardGenerateResponse, CardSentence};
    use crate::clients::subworker::cards::{
        ClusterOutput, ClusterStoriesResponse, VerifyCardResponse,
    };
    use crate::pipeline::cards::fakes::{
        FakeCardGenerator, FakeCardVerifier, FakeEmbedCluster, FakeFeedSource, FakeGenreTagger,
    };
    use std::sync::Mutex;

    type MockJobEntry = (Uuid, Option<String>, u32, String);
    type MockStatusEntry = (Uuid, JobStatus, Option<String>, Option<String>);
    type MockEmbeddingCache = Mutex<HashMap<String, (String, usize, Vec<f32>)>>;

    #[derive(Default)]
    pub struct MockCardsPipelineDao {
        pub jobs: Mutex<Vec<MockJobEntry>>,
        pub snapshots: Mutex<Vec<RecapCardSnapshot>>,
        pub candidates: Mutex<Vec<RecapCardCandidate>>,
        pub cards: Mutex<Vec<RecapCard>>,
        pub stats: Mutex<Vec<RecapCardJobStats>>,
        pub windows: Mutex<Vec<RecapEvalWindow>>,
        pub statuses: Mutex<Vec<MockStatusEntry>>,
        pub previous_job: Mutex<Option<PreviousCardsJob>>,
        pub cache: MockEmbeddingCache,
    }

    #[async_trait::async_trait]
    impl CardsPipelineDao for MockCardsPipelineDao {
        async fn create_job_with_lock_and_window(
            &self,
            job_id: Uuid,
            note: Option<&str>,
            window_days: u32,
            trigger_source: &str,
        ) -> Result<Option<Uuid>> {
            self.jobs.lock().unwrap().push((
                job_id,
                note.map(String::from),
                window_days,
                trigger_source.to_string(),
            ));
            Ok(Some(job_id))
        }

        async fn get_latest_completed_cards_job(&self) -> Result<Option<PreviousCardsJob>> {
            Ok(self.previous_job.lock().unwrap().clone())
        }

        async fn get_cached_embeddings(
            &self,
            model: &str,
            text_hashes: &[String],
        ) -> Result<HashMap<String, Vec<f32>>> {
            let lock = self.cache.lock().unwrap();
            let mut map = HashMap::new();
            for h in text_hashes {
                if let Some((stored_model, dim, vec)) = lock.get(h) {
                    if stored_model != model {
                        anyhow::bail!(
                            "embedding cache model identity mismatch for hash {h}: expected {model}, got {stored_model}"
                        );
                    }
                    if *dim != vec.len() {
                        anyhow::bail!(
                            "cached embedding dimension mismatch for hash {h}: dim {dim} != vec len {}",
                            vec.len()
                        );
                    }
                    map.insert(h.clone(), vec.clone());
                }
            }
            Ok(map)
        }

        async fn insert_embeddings(
            &self,
            model: &str,
            dim: usize,
            entries: &[(String, Vec<f32>)],
        ) -> Result<()> {
            let mut lock = self.cache.lock().unwrap();
            for (h, vec) in entries {
                if vec.len() != dim {
                    anyhow::bail!(
                        "cannot insert embedding for hash {h}: vector length {} != specified dim {dim}",
                        vec.len()
                    );
                }
                lock.entry(h.clone())
                    .or_insert_with(|| (model.to_string(), dim, vec.clone()));
            }
            Ok(())
        }

        async fn persist_pipeline_output(
            &self,
            snapshot: &RecapCardSnapshot,
            candidates: &[RecapCardCandidate],
            cards: &[RecapCard],
            stats: &RecapCardJobStats,
            eval_window: Option<&RecapEvalWindow>,
        ) -> Result<()> {
            self.snapshots.lock().unwrap().push(snapshot.clone());
            self.candidates.lock().unwrap().extend(candidates.to_vec());
            self.cards.lock().unwrap().extend(cards.to_vec());
            self.stats.lock().unwrap().push(stats.clone());
            if let Some(w) = eval_window {
                self.windows.lock().unwrap().push(w.clone());
            }
            Ok(())
        }

        async fn update_job_status_with_history(
            &self,
            job_id: Uuid,
            status: JobStatus,
            last_stage: Option<&str>,
            reason: Option<&str>,
        ) -> Result<()> {
            self.statuses.lock().unwrap().push((
                job_id,
                status,
                last_stage.map(String::from),
                reason.map(String::from),
            ));
            Ok(())
        }
    }

    fn sample_feed(id: Uuid, title: &str, host: &str, pub_date_rfc3339: &str) -> AltBackendFeed {
        AltBackendFeed {
            id: id.to_string(),
            title: title.to_string(),
            description: Some(
                "<p>This is a valid and sufficiently long lede text.</p>".to_string(),
            ),
            website_url: format!("https://{host}/post/{id}"),
            pub_date: DateTime::parse_from_rfc3339(pub_date_rfc3339)
                .ok()
                .map(|dt| dt.with_timezone(&Utc)),
            created_at: Some(Utc::now()),
            updated_at: Some(Utc::now()),
            article_id: Some("art-1".to_string()),
            is_read: false,
            feed_link_id: Some(Uuid::new_v4().to_string()),
            og_image_url: None,
        }
    }

    #[tokio::test]
    async fn test_cards_pipeline_determinism() {
        let id1 = Uuid::new_v4();
        let id2 = Uuid::new_v4();
        let id3 = Uuid::new_v4();

        let feeds = vec![
            sample_feed(
                id1,
                "Example headline 1",
                "example.com",
                "2026-03-20T10:00:00Z",
            ),
            sample_feed(
                id2,
                "Example headline 2",
                "example.org",
                "2026-03-20T11:00:00Z",
            ),
            sample_feed(
                id3,
                "Example headline 3",
                "example.net",
                "2026-03-20T12:00:00Z",
            ),
        ];

        let fixed_embs = vec![
            vec![1.0, 0.0, 0.0, 0.0],
            vec![0.0, 1.0, 0.0, 0.0],
            vec![0.0, 0.0, 1.0, 0.0],
        ];

        let fixed_cluster = ClusterStoriesResponse {
            clusters: vec![
                ClusterOutput {
                    cluster_id: 0,
                    member_ids: vec![id1, id2],
                    centroid: vec![0.5, 0.5, 0.0, 0.0],
                },
                ClusterOutput {
                    cluster_id: 1,
                    member_ids: vec![id3],
                    centroid: vec![0.0, 0.0, 1.0, 0.0],
                },
            ],
            params: StoryClusterParams::default(),
        };

        let feed_source = Arc::new(FakeFeedSource::new(feeds));
        let ml_port = Arc::new(FakeEmbedCluster::with_fixed(fixed_embs, fixed_cluster));
        let dao1 = Arc::new(MockCardsPipelineDao::default());
        let dao2 = Arc::new(MockCardsPipelineDao::default());

        let user_id = Uuid::new_v4();
        let pipeline1 = CardsPipeline::selection_only(
            feed_source.clone(),
            ml_port.clone(),
            dao1.clone(),
            Arc::new(FakeGenreTagger::new()),
        )
        .with_user_id(user_id);
        let pipeline2 = CardsPipeline::selection_only(
            feed_source.clone(),
            ml_port.clone(),
            dao2.clone(),
            Arc::new(FakeGenreTagger::new()),
        )
        .with_user_id(user_id);

        let job_id = Uuid::new_v4();
        let from = Utc::now();
        let to = Utc::now();
        let params = CardsParams::default();

        let res1 = pipeline1
            .run(job_id, from, to, &params)
            .await
            .expect("pipeline1 succeeds");
        let res2 = pipeline2
            .run(job_id, from, to, &params)
            .await
            .expect("pipeline2 succeeds");

        assert_eq!(res1, res2);

        let candidates1 = dao1.candidates.lock().unwrap().clone();
        let candidates2 = dao2.candidates.lock().unwrap().clone();

        assert_eq!(candidates1.len(), candidates2.len());
        for (c1, c2) in candidates1.iter().zip(candidates2.iter()) {
            assert_eq!(c1.rank, c2.rank);
            assert_eq!(c1.cluster_fingerprint, c2.cluster_fingerprint);
            assert_eq!(c1.scores, c2.scores);
            assert_eq!(c1.items, c2.items);
        }
    }

    #[tokio::test]
    async fn test_cards_pipeline_replay_creates_eval_window_and_replay_job() {
        let id1 = Uuid::new_v4();
        let feeds = vec![sample_feed(
            id1,
            "Example headline 1",
            "example.com",
            "2026-03-20T10:00:00Z",
        )];

        let feed_source = Arc::new(FakeFeedSource::new(feeds));
        let ml_port = Arc::new(FakeEmbedCluster::new());
        let dao = Arc::new(MockCardsPipelineDao::default());

        let pipeline = CardsPipeline::selection_only(
            feed_source,
            ml_port,
            dao.clone(),
            Arc::new(FakeGenreTagger::new()),
        )
        .with_user_id(Uuid::new_v4());
        let from = Utc::now();
        let to = Utc::now();
        let params = CardsParams::default();

        let replay_res = pipeline
            .run_replay(from, to, &params)
            .await
            .expect("run_replay succeeds");

        // Verify job was created with trigger_source = 'cards_replay' and window_days = 3
        let jobs = dao.jobs.lock().unwrap().clone();
        assert_eq!(jobs.len(), 1);
        assert_eq!(jobs[0].0, replay_res.job_id);
        assert_eq!(jobs[0].2, 3);
        assert_eq!(jobs[0].3, "cards_replay");

        // Verify recap_eval_windows row created
        let windows = dao.windows.lock().unwrap().clone();
        assert_eq!(windows.len(), 1);
        assert_eq!(windows[0].id, replay_res.window_id);
        assert_eq!(windows[0].snapshot_job_id, replay_res.job_id);
    }

    #[tokio::test]
    async fn test_cards_pipeline_failure_path_persists_nothing_except_failed_status() {
        let id1 = Uuid::new_v4();
        let feeds = vec![sample_feed(
            id1,
            "Example headline 1",
            "example.com",
            "2026-03-20T10:00:00Z",
        )];

        let feed_source = Arc::new(FakeFeedSource::new(feeds));

        // Fake embed port that returns an error
        struct FailingEmbedPort;
        #[async_trait::async_trait]
        impl EmbedCluster for FailingEmbedPort {
            async fn embed(&self, _texts: &[String]) -> Result<Vec<Vec<f32>>> {
                anyhow::bail!("simulated subworker embed failure");
            }
            async fn cluster_stories(
                &self,
                _req: &ClusterStoriesRequest,
            ) -> Result<ClusterStoriesResponse> {
                anyhow::bail!("should not reach cluster");
            }
        }

        let ml_port = Arc::new(FailingEmbedPort);
        let dao = Arc::new(MockCardsPipelineDao::default());

        let pipeline = CardsPipeline::selection_only(
            feed_source,
            ml_port,
            dao.clone(),
            Arc::new(FakeGenreTagger::new()),
        )
        .with_user_id(Uuid::new_v4());
        let job_id = Uuid::new_v4();
        let from = Utc::now();
        let to = Utc::now();
        let params = CardsParams::default();

        let res = pipeline.run(job_id, from, to, &params).await;
        assert!(res.is_err(), "pipeline should fail when embed fails");

        // Assert: nothing is persisted to snapshots, candidates, stats, or windows!
        assert_eq!(
            dao.snapshots.lock().unwrap().len(),
            0,
            "no snapshot persisted on failure"
        );
        assert_eq!(
            dao.candidates.lock().unwrap().len(),
            0,
            "no candidates persisted on failure"
        );
        assert_eq!(
            dao.stats.lock().unwrap().len(),
            0,
            "no stats persisted on failure"
        );
        assert_eq!(
            dao.windows.lock().unwrap().len(),
            0,
            "no windows persisted on failure"
        );

        // Status history must have transitioned to Failed
        let statuses = dao.statuses.lock().unwrap().clone();
        assert_eq!(statuses.len(), 2);
        assert_eq!(statuses[0].1, JobStatus::Running);
        assert_eq!(statuses[1].1, JobStatus::Failed);
        assert_eq!(statuses[1].2.as_deref(), Some("cards_pipeline"));
        assert!(
            statuses[1]
                .3
                .as_deref()
                .unwrap()
                .contains("simulated subworker embed failure"),
            "status failure reason must record the original error"
        );
    }

    #[tokio::test]
    async fn test_cards_pipeline_embedding_count_mismatch_fails() {
        let id1 = Uuid::new_v4();
        let feeds = vec![sample_feed(
            id1,
            "Example headline 1",
            "example.com",
            "2026-03-20T10:00:00Z",
        )];

        let feed_source = Arc::new(FakeFeedSource::new(feeds));
        // Return 0 embeddings for 1 item
        let ml_port = Arc::new(FakeEmbedCluster::with_fixed(
            vec![],
            ClusterStoriesResponse {
                clusters: vec![],
                params: StoryClusterParams::default(),
            },
        ));
        let dao = Arc::new(MockCardsPipelineDao::default());

        let pipeline = CardsPipeline::selection_only(
            feed_source,
            ml_port,
            dao.clone(),
            Arc::new(FakeGenreTagger::new()),
        )
        .with_user_id(Uuid::new_v4());
        let job_id = Uuid::new_v4();
        let res = pipeline
            .run(job_id, Utc::now(), Utc::now(), &CardsParams::default())
            .await;
        assert!(res.is_err());
        let err_str = res.unwrap_err().to_string();
        assert!(
            err_str.contains("subworker embedding count mismatch"),
            "error was: {err_str}"
        );
    }

    #[tokio::test]
    async fn test_cards_pipeline_unparseable_feed_id_fails_loud() {
        let bad_feed = AltBackendFeed {
            id: "bad-feed-uuid".to_string(),
            title: "Example Title".to_string(),
            description: Some("<p>Lede text</p>".to_string()),
            website_url: "https://example.com/item".to_string(),
            pub_date: Some(Utc::now()),
            created_at: None,
            updated_at: None,
            article_id: None,
            is_read: false,
            feed_link_id: None,
            og_image_url: None,
        };

        let feed_source = Arc::new(FakeFeedSource::new(vec![bad_feed]));
        let ml_port = Arc::new(FakeEmbedCluster::new());
        let dao = Arc::new(MockCardsPipelineDao::default());

        let pipeline = CardsPipeline::selection_only(
            feed_source,
            ml_port,
            dao.clone(),
            Arc::new(FakeGenreTagger::new()),
        )
        .with_user_id(Uuid::new_v4());
        let res = pipeline
            .run(
                Uuid::new_v4(),
                Utc::now(),
                Utc::now(),
                &CardsParams::default(),
            )
            .await;
        assert!(res.is_err());
        assert!(
            res.unwrap_err()
                .to_string()
                .contains("contract violation: unparseable feed id 'bad-feed-uuid'")
        );
    }

    #[tokio::test]
    async fn test_cards_pipeline_with_personal_vector_and_previous_cards() {
        let id1 = Uuid::new_v4();
        let feed = sample_feed(
            id1,
            "Continuation headline",
            "example.com",
            "2026-03-20T10:00:00Z",
        );

        let feed_source = Arc::new(FakeFeedSource::with_read_ids(vec![feed], vec![id1]));

        let fixed_embs = vec![vec![1.0, 0.0, 0.0, 0.0]];
        let fixed_cluster = ClusterStoriesResponse {
            clusters: vec![ClusterOutput {
                cluster_id: 0,
                member_ids: vec![id1],
                centroid: vec![1.0, 0.0, 0.0, 0.0],
            }],
            params: StoryClusterParams::default(),
        };
        let ml_port = Arc::new(FakeEmbedCluster::with_fixed(fixed_embs, fixed_cluster));
        let dao = Arc::new(MockCardsPipelineDao::default());

        let prev_job_id = Uuid::new_v4();
        let prev_card_id = Uuid::new_v4();
        let prev_story_id = Uuid::new_v4();
        let prev_card = PreviousCardSummary {
            id: prev_card_id,
            story_id: prev_story_id,
            centroid: vec![1.0, 0.0, 0.0, 0.0],
        };
        *dao.previous_job.lock().unwrap() = Some(PreviousCardsJob {
            job_id: prev_job_id,
            to_ts: DateTime::parse_from_rfc3339("2026-03-19T00:00:00Z")
                .unwrap()
                .with_timezone(&Utc),
            cards: vec![prev_card],
        });

        let pipeline = CardsPipeline::selection_only(
            feed_source,
            ml_port,
            dao.clone(),
            Arc::new(FakeGenreTagger::new()),
        )
        .with_user_id(Uuid::new_v4());
        let job_id = Uuid::new_v4();
        let from = DateTime::parse_from_rfc3339("2026-03-19T00:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let to = DateTime::parse_from_rfc3339("2026-03-21T00:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let params = CardsParams::default()
            .with_override("expected_embed_dim", &serde_json::json!(4))
            .unwrap();

        let res = pipeline
            .run(job_id, from, to, &params)
            .await
            .expect("pipeline run succeeds");
        assert_eq!(res.candidates, 1);

        // Verify snapshot captured read_feed_ids and previous_job_id
        let snapshots = dao.snapshots.lock().unwrap().clone();
        assert_eq!(snapshots.len(), 1);
        assert_eq!(snapshots[0].read_feed_ids, vec![id1]);
        assert_eq!(snapshots[0].previous_job_id, Some(prev_job_id));

        // Verify candidate continuation and personal vector boost in scores JSON
        let candidates = dao.candidates.lock().unwrap().clone();
        assert_eq!(candidates.len(), 1);
        let c = &candidates[0];
        assert_eq!(c.scores["story_id"], json!(prev_story_id));
        assert_eq!(c.scores["continues_card_id"], json!(prev_card_id));
        assert!((c.scores["personal"].as_f64().unwrap() - 1.0).abs() < 1e-4);
        assert!((c.scores["total"].as_f64().unwrap() - 1.5).abs() < 1e-4);
    }

    #[tokio::test]
    async fn test_cards_pipeline_personal_vector_fetch_failure_fails_fast_in_snapshot() {
        let id1 = Uuid::new_v4();
        let feed = sample_feed(
            id1,
            "Continuation headline",
            "example.com",
            "2026-03-20T10:00:00Z",
        );

        // First fetch (window feeds) succeeds; second fetch (30-day read feeds) fails
        let feed_source = Arc::new(FakeFeedSource::with_fail_after_n_fetches(
            vec![feed],
            vec![id1],
            1,
        ));
        let ml_port = Arc::new(FakeEmbedCluster::new());
        let dao = Arc::new(MockCardsPipelineDao::default());
        let tagger = Arc::new(FakeGenreTagger::new());

        let pipeline = CardsPipeline::selection_only(
            feed_source.clone(),
            ml_port.clone(),
            dao.clone(),
            tagger.clone(),
        )
        .with_user_id(Uuid::new_v4());

        let job_id = Uuid::new_v4();
        let from = DateTime::parse_from_rfc3339("2026-03-19T00:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let to = DateTime::parse_from_rfc3339("2026-03-21T00:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let params = CardsParams::default();

        let res = pipeline.run(job_id, from, to, &params).await;

        assert!(res.is_err(), "pipeline run must fail fast");
        let err_msg = format!("{:#}", res.unwrap_err());
        assert!(
            err_msg.contains("failed to fetch feeds for personal vector"),
            "expected error to mention failed to fetch feeds for personal vector, got: {err_msg}"
        );
        assert!(
            err_msg.contains("date range exceeds 8 days"),
            "expected error to contain underlying cause, got: {err_msg}"
        );

        // Fail-fast assertion: genre classification was never invoked
        assert_eq!(
            tagger.calls.lock().unwrap().len(),
            0,
            "genre tagger must not be called when snapshot stage fails"
        );

        // Nothing was persisted to DAO
        assert_eq!(dao.snapshots.lock().unwrap().len(), 0);
        assert_eq!(dao.candidates.lock().unwrap().len(), 0);
    }

    #[tokio::test]
    async fn test_cards_pipeline_user_id_missing_fails_loud() {
        let id1 = Uuid::new_v4();
        let feeds = vec![sample_feed(
            id1,
            "Headline 1",
            "example.com",
            "2026-03-20T10:00:00Z",
        )];
        let feed_source = Arc::new(FakeFeedSource::new(feeds));
        let ml_port = Arc::new(FakeEmbedCluster::new());
        let dao = Arc::new(MockCardsPipelineDao::default());

        // Do NOT call with_user_id
        let pipeline = CardsPipeline::selection_only(
            feed_source,
            ml_port,
            dao.clone(),
            Arc::new(FakeGenreTagger::new()),
        );
        let res = pipeline
            .run(
                Uuid::new_v4(),
                Utc::now(),
                Utc::now(),
                &CardsParams::default(),
            )
            .await;
        assert!(res.is_err(), "missing user_id must fail");
        assert!(
            res.unwrap_err()
                .to_string()
                .contains("cards_user_id_missing"),
            "error should contain cards_user_id_missing"
        );
    }

    #[tokio::test]
    async fn test_cards_pipeline_generation_and_persistence_happy_path() {
        let id1 = Uuid::new_v4();
        let feed = sample_feed(
            id1,
            "テックニュース発表",
            "example.com",
            "2026-03-20T10:00:00Z",
        );
        let feed_source = Arc::new(FakeFeedSource::new(vec![feed]));
        let ml_port = Arc::new(FakeEmbedCluster::new());
        let dao = Arc::new(MockCardsPipelineDao::default());
        let card_gen = Arc::new(FakeCardGenerator::new());
        let card_ver = Arc::new(FakeCardVerifier::new());

        let pipeline = CardsPipeline::full(
            feed_source,
            ml_port,
            dao.clone(),
            card_gen.clone(),
            card_ver.clone(),
            Arc::new(FakeGenreTagger::new()),
        )
        .with_user_id(Uuid::new_v4());

        let job_id = Uuid::new_v4();
        let from = Utc::now();
        let to = Utc::now();
        let params = CardsParams::default();

        let res = pipeline
            .run(job_id, from, to, &params)
            .await
            .expect("pipeline should succeed");

        assert_eq!(res.cards_selected, 1);

        let cards = dao.cards.lock().unwrap().clone();
        assert_eq!(cards.len(), 1);
        let card = &cards[0];
        assert_eq!(card.job_id, job_id);
        assert_eq!(card.rank, 1);
        assert!(
            card.what_ja.contains("[1]"),
            "what_ja must have [1] citations"
        );
        assert!(card.why_ja.is_some());
        assert_eq!(card.sources.as_array().unwrap().len(), 1);
        assert_eq!(card.sources[0]["n"], 1);
        assert_eq!(card.sources[0]["feed_id"], id1.to_string());
        assert!(card.gates["g1_ja_ratio"].as_f64().unwrap() >= 0.6);
        assert!(card.gates["g3_attribution_pass"].as_bool().unwrap());
        assert_eq!(card.generation["model"], "gemma4-e4b");

        let stats = dao.stats.lock().unwrap().clone();
        assert_eq!(stats.len(), 1);
        assert_eq!(stats[0].cards_selected, 1);

        // Check that degraded status was recorded because cards < 5
        let statuses = dao.statuses.lock().unwrap().clone();
        assert!(
            statuses
                .iter()
                .any(|s| s.2.as_deref() == Some("cards_job_degraded"))
        );
    }

    #[tokio::test]
    async fn test_cards_pipeline_generation_422_rejected_records_dropped_reason() {
        let id1 = Uuid::new_v4();
        let feed = sample_feed(
            id1,
            "テックニュース発表",
            "example.com",
            "2026-03-20T10:00:00Z",
        );
        let feed_source = Arc::new(FakeFeedSource::new(vec![feed]));
        let ml_port = Arc::new(FakeEmbedCluster::new());
        let dao = Arc::new(MockCardsPipelineDao::default());

        let card_gen = Arc::new(FakeCardGenerator::with_responses(vec![
            CardGenerateOutcome::Rejected(
                crate::clients::news_creator::models::CardGenerate422Response {
                    reason: "gemma_turn_parse_failed".to_string(),
                    attempts: 2,
                    raw_text: "bad response".to_string(),
                },
            ),
        ]));
        let card_ver = Arc::new(FakeCardVerifier::new());

        let pipeline = CardsPipeline::full(
            feed_source,
            ml_port,
            dao.clone(),
            card_gen,
            card_ver,
            Arc::new(FakeGenreTagger::new()),
        )
        .with_user_id(Uuid::new_v4());

        let job_id = Uuid::new_v4();
        let res = pipeline
            .run(job_id, Utc::now(), Utc::now(), &CardsParams::default())
            .await
            .expect("pipeline should complete with dropped card");

        assert_eq!(res.cards_selected, 0);
        let cards = dao.cards.lock().unwrap().clone();
        assert_eq!(cards.len(), 0);

        let stats = dao.stats.lock().unwrap().clone();
        assert_eq!(stats.len(), 1);
        assert_eq!(stats[0].cards_selected, 0);
        assert_eq!(stats[0].cards_dropped["gemma_turn_parse_failed"], 1);
    }

    #[tokio::test]
    async fn test_cards_pipeline_gate_g1_language_drop() {
        let id1 = Uuid::new_v4();
        let feed = sample_feed(
            id1,
            "Tech announcement",
            "example.com",
            "2026-03-20T10:00:00Z",
        );
        let feed_source = Arc::new(FakeFeedSource::new(vec![feed]));
        let ml_port = Arc::new(FakeEmbedCluster::new());
        let dao = Arc::new(MockCardsPipelineDao::default());

        // Card with purely English text (ja_ratio < 0.6)
        let english_card = CardContent {
            headline_ja: "English Headline Only".to_string(),
            what_ja: vec![CardSentence {
                text: "Something completely in English happened.[1]".to_string(),
                refs: vec![1],
            }],
            why_ja: None,
            used_refs: vec![1],
        };

        let card_gen = Arc::new(FakeCardGenerator::with_responses(vec![
            CardGenerateOutcome::Success(CardGenerateResponse {
                card: english_card.clone(),
                generation: crate::clients::news_creator::models::CardGenerationMetadata {
                    model: "gemma4-e4b".to_string(),
                    prompt_version: "recap_card.v1".to_string(),
                    cache_hit: false,
                    prompt_tokens: 10,
                    completion_tokens: 10,
                    ms: 10,
                    raw_text: String::new(),
                },
            }),
            CardGenerateOutcome::Success(CardGenerateResponse {
                card: english_card,
                generation: crate::clients::news_creator::models::CardGenerationMetadata {
                    model: "gemma4-e4b".to_string(),
                    prompt_version: "recap_card.v1".to_string(),
                    cache_hit: false,
                    prompt_tokens: 10,
                    completion_tokens: 10,
                    ms: 10,
                    raw_text: String::new(),
                },
            }),
        ]));
        let card_ver = Arc::new(FakeCardVerifier::new());

        let pipeline = CardsPipeline::full(
            feed_source,
            ml_port,
            dao.clone(),
            card_gen.clone(),
            card_ver,
            Arc::new(FakeGenreTagger::new()),
        )
        .with_user_id(Uuid::new_v4());

        let res = pipeline
            .run(
                Uuid::new_v4(),
                Utc::now(),
                Utc::now(),
                &CardsParams::default(),
            )
            .await
            .expect("pipeline completes");

        assert_eq!(res.cards_selected, 0);
        let stats = dao.stats.lock().unwrap().clone();
        assert_eq!(stats[0].cards_dropped["g1_language"], 1);
        assert_eq!(card_gen.requests.lock().unwrap().len(), 2);
    }

    #[tokio::test]
    async fn test_cards_pipeline_gate_g2_citation_invalid_drop() {
        let id1 = Uuid::new_v4();
        let feed = sample_feed(
            id1,
            "テックニュース発表",
            "example.com",
            "2026-03-20T10:00:00Z",
        );
        let feed_source = Arc::new(FakeFeedSource::new(vec![feed]));
        let ml_port = Arc::new(FakeEmbedCluster::new());
        let dao = Arc::new(MockCardsPipelineDao::default());

        // Card with invalid refs (e.g. ref 99 when only 1 item exists)
        let invalid_card = CardContent {
            headline_ja: "主要なテクノロジー動向の進展".to_string(),
            what_ja: vec![CardSentence {
                text: "重大な進展が発生した。[99]".to_string(),
                refs: vec![99],
            }],
            why_ja: None,
            used_refs: vec![99],
        };

        let card_gen = Arc::new(FakeCardGenerator::with_responses(vec![
            CardGenerateOutcome::Success(CardGenerateResponse {
                card: invalid_card.clone(),
                generation: crate::clients::news_creator::models::CardGenerationMetadata {
                    model: "gemma4-e4b".to_string(),
                    prompt_version: "recap_card.v1".to_string(),
                    cache_hit: false,
                    prompt_tokens: 10,
                    completion_tokens: 10,
                    ms: 10,
                    raw_text: String::new(),
                },
            }),
            CardGenerateOutcome::Success(CardGenerateResponse {
                card: invalid_card,
                generation: crate::clients::news_creator::models::CardGenerationMetadata {
                    model: "gemma4-e4b".to_string(),
                    prompt_version: "recap_card.v1".to_string(),
                    cache_hit: false,
                    prompt_tokens: 10,
                    completion_tokens: 10,
                    ms: 10,
                    raw_text: String::new(),
                },
            }),
        ]));
        let card_ver = Arc::new(FakeCardVerifier::new());

        let pipeline = CardsPipeline::full(
            feed_source,
            ml_port,
            dao.clone(),
            card_gen.clone(),
            card_ver,
            Arc::new(FakeGenreTagger::new()),
        )
        .with_user_id(Uuid::new_v4());

        let res = pipeline
            .run(
                Uuid::new_v4(),
                Utc::now(),
                Utc::now(),
                &CardsParams::default(),
            )
            .await
            .expect("pipeline completes");

        assert_eq!(res.cards_selected, 0);
        let stats = dao.stats.lock().unwrap().clone();
        assert_eq!(stats[0].cards_dropped["g2_citation"], 1);
        assert_eq!(card_gen.requests.lock().unwrap().len(), 2);
    }

    #[allow(clippy::too_many_lines)]
    #[tokio::test]
    async fn test_cards_pipeline_gate_g3_regeneration_then_success() {
        let id1 = Uuid::new_v4();
        let feed = sample_feed(
            id1,
            "テックニュース発表",
            "example.com",
            "2026-03-20T10:00:00Z",
        );
        let feed_source = Arc::new(FakeFeedSource::new(vec![feed]));
        let ml_port = Arc::new(FakeEmbedCluster::new());
        let dao = Arc::new(MockCardsPipelineDao::default());

        let initial_card = CardContent {
            headline_ja: "テクノロジー動向の進展".to_string(),
            what_ja: vec![CardSentence {
                text: "初期の文ですが類似度が低い。[1]".to_string(),
                refs: vec![1],
            }],
            why_ja: None,
            used_refs: vec![1],
        };

        let regenerated_card = CardContent {
            headline_ja: "テクノロジー動向の進展".to_string(),
            what_ja: vec![CardSentence {
                text: "修正後の高品質な文です。[1]".to_string(),
                refs: vec![1],
            }],
            why_ja: None,
            used_refs: vec![1],
        };

        let card_gen = Arc::new(FakeCardGenerator::with_responses(vec![
            CardGenerateOutcome::Success(CardGenerateResponse {
                card: initial_card,
                generation: crate::clients::news_creator::models::CardGenerationMetadata {
                    model: "gemma4-e4b".to_string(),
                    prompt_version: "recap_card.v1".to_string(),
                    cache_hit: false,
                    prompt_tokens: 10,
                    completion_tokens: 10,
                    ms: 10,
                    raw_text: String::new(),
                },
            }),
            CardGenerateOutcome::Success(CardGenerateResponse {
                card: regenerated_card,
                generation: crate::clients::news_creator::models::CardGenerationMetadata {
                    model: "gemma4-e4b".to_string(),
                    prompt_version: "recap_card.v1".to_string(),
                    cache_hit: false,
                    prompt_tokens: 10,
                    completion_tokens: 10,
                    ms: 10,
                    raw_text: String::new(),
                },
            }),
        ]));

        // First verify fails G3 (attribution pass = false), second verify passes
        let fail_verify = VerifyCardResponse {
            sentences: vec![crate::clients::subworker::cards::VerifySentenceResult {
                idx: 0,
                attribution: crate::clients::subworker::cards::VerifyAttribution {
                    max_cos: 0.3,
                    best_n: Some(1),
                    pass: false,
                },
                filler: crate::clients::subworker::cards::VerifyFiller {
                    matched: vec![],
                    pass: true,
                },
                specificity: crate::clients::subworker::cards::VerifySpecificity {
                    proper_nouns: 1,
                    numbers: 0,
                    tokens: 5,
                    density: 0.2,
                },
            }],
            why_hint_present: true,
            embedding: crate::clients::subworker::cards::VerifyEmbeddingInfo {
                model: "bge-m3".to_string(),
                identity: "bge-m3".to_string(),
            },
        };

        let pass_verify = VerifyCardResponse {
            sentences: vec![crate::clients::subworker::cards::VerifySentenceResult {
                idx: 0,
                attribution: crate::clients::subworker::cards::VerifyAttribution {
                    max_cos: 0.85,
                    best_n: Some(1),
                    pass: true,
                },
                filler: crate::clients::subworker::cards::VerifyFiller {
                    matched: vec![],
                    pass: true,
                },
                specificity: crate::clients::subworker::cards::VerifySpecificity {
                    proper_nouns: 1,
                    numbers: 0,
                    tokens: 5,
                    density: 0.2,
                },
            }],
            why_hint_present: true,
            embedding: crate::clients::subworker::cards::VerifyEmbeddingInfo {
                model: "bge-m3".to_string(),
                identity: "bge-m3".to_string(),
            },
        };

        let card_ver = Arc::new(FakeCardVerifier::with_responses(vec![
            fail_verify,
            pass_verify,
        ]));

        let pipeline = CardsPipeline::full(
            feed_source,
            ml_port,
            dao.clone(),
            card_gen.clone(),
            card_ver,
            Arc::new(FakeGenreTagger::new()),
        )
        .with_user_id(Uuid::new_v4());

        let res = pipeline
            .run(
                Uuid::new_v4(),
                Utc::now(),
                Utc::now(),
                &CardsParams::default(),
            )
            .await
            .expect("pipeline completes");

        assert_eq!(res.cards_selected, 1);
        let cards = dao.cards.lock().unwrap().clone();
        assert_eq!(cards.len(), 1);
        assert_eq!(cards[0].what_ja, "修正後の高品質な文です。[1]");

        // Verify that card_generator received revision_note
        let gen_requests = card_gen.requests.lock().unwrap().clone();
        assert_eq!(gen_requests.len(), 2);
        assert!(gen_requests[1].revision_note.is_some());
        assert!(
            gen_requests[1]
                .revision_note
                .as_ref()
                .unwrap()
                .contains("帰属類似度不足")
        );
    }

    #[allow(clippy::too_many_lines)]
    #[tokio::test]
    async fn test_cards_pipeline_gate_g4_filler_deletion_and_survival() {
        let id1 = Uuid::new_v4();
        let feed = sample_feed(
            id1,
            "テックニュース発表",
            "example.com",
            "2026-03-20T10:00:00Z",
        );
        let feed_source = Arc::new(FakeFeedSource::new(vec![feed]));
        let ml_port = Arc::new(FakeEmbedCluster::new());
        let dao = Arc::new(MockCardsPipelineDao::default());

        let initial_card = CardContent {
            headline_ja: "テクノロジー動向の進展".to_string(),
            what_ja: vec![
                CardSentence {
                    text: "主要なテクノロジー動向の進展が確認された。[1]".to_string(),
                    refs: vec![1],
                },
                CardSentence {
                    text: "今後の動向に影響を与えると思われる。[1]".to_string(),
                    refs: vec![1],
                },
            ],
            why_ja: None,
            used_refs: vec![1],
        };

        let second_card = CardContent {
            headline_ja: "テクノロジー動向の進展".to_string(),
            what_ja: vec![
                CardSentence {
                    text: "主要なテクノロジー動向の進展が確認された。[1]".to_string(),
                    refs: vec![1],
                },
                CardSentence {
                    text: "依然として大きな影響を与えると思われる。[1]".to_string(),
                    refs: vec![1],
                },
            ],
            why_ja: None,
            used_refs: vec![1],
        };

        let card_gen = Arc::new(FakeCardGenerator::with_responses(vec![
            CardGenerateOutcome::Success(CardGenerateResponse {
                card: initial_card,
                generation: crate::clients::news_creator::models::CardGenerationMetadata {
                    model: "gemma4-e4b".to_string(),
                    prompt_version: "recap_card.v1".to_string(),
                    cache_hit: false,
                    prompt_tokens: 10,
                    completion_tokens: 10,
                    ms: 10,
                    raw_text: String::new(),
                },
            }),
            CardGenerateOutcome::Success(CardGenerateResponse {
                card: second_card,
                generation: crate::clients::news_creator::models::CardGenerationMetadata {
                    model: "gemma4-e4b".to_string(),
                    prompt_version: "recap_card.v1".to_string(),
                    cache_hit: false,
                    prompt_tokens: 10,
                    completion_tokens: 10,
                    ms: 10,
                    raw_text: String::new(),
                },
            }),
        ]));

        let make_verify_resp = || VerifyCardResponse {
            sentences: vec![
                crate::clients::subworker::cards::VerifySentenceResult {
                    idx: 0,
                    attribution: crate::clients::subworker::cards::VerifyAttribution {
                        max_cos: 0.85,
                        best_n: Some(1),
                        pass: true,
                    },
                    filler: crate::clients::subworker::cards::VerifyFiller {
                        matched: vec![],
                        pass: true,
                    },
                    specificity: crate::clients::subworker::cards::VerifySpecificity {
                        proper_nouns: 1,
                        numbers: 0,
                        tokens: 5,
                        density: 0.2,
                    },
                },
                crate::clients::subworker::cards::VerifySentenceResult {
                    idx: 1,
                    attribution: crate::clients::subworker::cards::VerifyAttribution {
                        max_cos: 0.85,
                        best_n: Some(1),
                        pass: true,
                    },
                    filler: crate::clients::subworker::cards::VerifyFiller {
                        matched: vec!["と思われる".to_string()],
                        pass: false,
                    },
                    specificity: crate::clients::subworker::cards::VerifySpecificity {
                        proper_nouns: 1,
                        numbers: 0,
                        tokens: 5,
                        density: 0.2,
                    },
                },
            ],
            why_hint_present: true,
            embedding: crate::clients::subworker::cards::VerifyEmbeddingInfo {
                model: "bge-m3".to_string(),
                identity: "bge-m3".to_string(),
            },
        };

        let card_ver = Arc::new(FakeCardVerifier::with_responses(vec![
            make_verify_resp(),
            make_verify_resp(),
        ]));

        let pipeline = CardsPipeline::full(
            feed_source,
            ml_port,
            dao.clone(),
            card_gen.clone(),
            card_ver.clone(),
            Arc::new(FakeGenreTagger::new()),
        )
        .with_user_id(Uuid::new_v4());

        let res = pipeline
            .run(
                Uuid::new_v4(),
                Utc::now(),
                Utc::now(),
                &CardsParams::default(),
            )
            .await
            .expect("pipeline completes");

        assert_eq!(res.cards_selected, 1);
        let cards = dao.cards.lock().unwrap().clone();
        assert_eq!(cards.len(), 1);

        let card = &cards[0];
        assert_eq!(
            card.what_ja,
            "主要なテクノロジー動向の進展が確認された。[1]"
        );
        assert_eq!(card.gates["regeneration_count"], 1);
        assert_eq!(
            card.gates["deleted_sentence_indices"],
            serde_json::json!([1])
        );

        let gen_requests = card_gen.requests.lock().unwrap().clone();
        assert_eq!(gen_requests.len(), 2);
        assert!(
            gen_requests[1]
                .revision_note
                .as_ref()
                .unwrap()
                .contains("推測語")
        );

        let ver_requests = card_ver.requests.lock().unwrap().clone();
        assert_eq!(ver_requests.len(), 2);
    }

    #[tokio::test]
    async fn test_cards_pipeline_gate_g6_why_removed_when_no_hint() {
        let id1 = Uuid::new_v4();
        let feed = sample_feed(
            id1,
            "テックニュース発表",
            "example.com",
            "2026-03-20T10:00:00Z",
        );
        let feed_source = Arc::new(FakeFeedSource::new(vec![feed]));
        let ml_port = Arc::new(FakeEmbedCluster::new());
        let dao = Arc::new(MockCardsPipelineDao::default());

        let card_gen = Arc::new(FakeCardGenerator::new());
        // Verifier says why_hint_present is false
        let verifier_resp = VerifyCardResponse {
            sentences: vec![
                crate::clients::subworker::cards::VerifySentenceResult {
                    idx: 0,
                    attribution: crate::clients::subworker::cards::VerifyAttribution {
                        max_cos: 0.85,
                        best_n: Some(1),
                        pass: true,
                    },
                    filler: crate::clients::subworker::cards::VerifyFiller {
                        matched: vec![],
                        pass: true,
                    },
                    specificity: crate::clients::subworker::cards::VerifySpecificity {
                        proper_nouns: 1,
                        numbers: 0,
                        tokens: 5,
                        density: 0.2,
                    },
                },
                crate::clients::subworker::cards::VerifySentenceResult {
                    idx: 1,
                    attribution: crate::clients::subworker::cards::VerifyAttribution {
                        max_cos: 0.85,
                        best_n: Some(1),
                        pass: true,
                    },
                    filler: crate::clients::subworker::cards::VerifyFiller {
                        matched: vec![],
                        pass: true,
                    },
                    specificity: crate::clients::subworker::cards::VerifySpecificity {
                        proper_nouns: 1,
                        numbers: 0,
                        tokens: 5,
                        density: 0.2,
                    },
                },
            ],
            why_hint_present: false, // NO why hint
            embedding: crate::clients::subworker::cards::VerifyEmbeddingInfo {
                model: "bge-m3".to_string(),
                identity: "bge-m3".to_string(),
            },
        };
        let card_ver = Arc::new(FakeCardVerifier::with_responses(vec![verifier_resp]));

        let pipeline = CardsPipeline::full(
            feed_source,
            ml_port,
            dao.clone(),
            card_gen,
            card_ver,
            Arc::new(FakeGenreTagger::new()),
        )
        .with_user_id(Uuid::new_v4());

        let res = pipeline
            .run(
                Uuid::new_v4(),
                Utc::now(),
                Utc::now(),
                &CardsParams::default(),
            )
            .await
            .expect("pipeline completes");

        assert_eq!(res.cards_selected, 1);
        let cards = dao.cards.lock().unwrap().clone();
        assert_eq!(cards.len(), 1);
        assert_eq!(
            cards[0].why_ja, None,
            "why_ja must be dropped when why_hint_present is false"
        );
    }

    #[tokio::test]
    async fn test_genre_tagging_disabled_leaves_genre_none() {
        let id1 = Uuid::new_v4();
        let feed = sample_feed(
            id1,
            "Tech announcement",
            "example.com",
            "2026-03-20T10:00:00Z",
        );
        let feed_source = Arc::new(FakeFeedSource::new(vec![feed]));
        let ml_port = Arc::new(FakeEmbedCluster::new());
        let dao = Arc::new(MockCardsPipelineDao::default());

        let params = CardsParams {
            genre_tagging: false,
            ..Default::default()
        };

        let pipeline = CardsPipeline::selection_only(
            feed_source,
            ml_port,
            dao.clone(),
            Arc::new(FakeGenreTagger::new()),
        )
        .with_user_id(Uuid::new_v4());

        let res = pipeline
            .run(Uuid::new_v4(), Utc::now(), Utc::now(), &params)
            .await
            .expect("pipeline completes when genre tagging is disabled");

        assert_eq!(res.candidates, 1);
        let candidates = dao.candidates.lock().unwrap().clone();
        assert_eq!(candidates.len(), 1);
        let items = candidates[0].items.as_array().unwrap();
        assert!(items[0]["genre"].is_null());
    }

    #[tokio::test]
    async fn test_genre_tagger_bounded_concurrency_and_order_preserved() {
        let tagger = FakeGenreTagger::new();
        let texts: Vec<String> = (0..20).map(|i| format!("Text item {i}")).collect();
        let concurrency = 4;
        let results = tagger
            .tag_genres(&texts, concurrency)
            .await
            .expect("tag_genres succeeds");

        assert_eq!(results.len(), 20);
        let calls = tagger.calls.lock().unwrap().clone();
        assert_eq!(calls.len(), 20);
        for (i, call) in calls.iter().enumerate() {
            assert_eq!(call, &format!("Text item {i}"));
        }
        let max_in_flight = tagger
            .max_in_flight
            .load(std::sync::atomic::Ordering::SeqCst);
        assert!(
            max_in_flight <= concurrency,
            "max in-flight ({max_in_flight}) exceeded bound ({concurrency})"
        );
        assert!(
            max_in_flight > 1,
            "expected concurrency to be exercised, got {max_in_flight}"
        );
    }

    #[tokio::test]
    async fn test_genre_classifier_error_fails_job() {
        let id1 = Uuid::new_v4();
        let feed = sample_feed(
            id1,
            "Tech announcement",
            "example.com",
            "2026-03-20T10:00:00Z",
        );
        let feed_source = Arc::new(FakeFeedSource::new(vec![feed]));
        let ml_port = Arc::new(FakeEmbedCluster::new());
        let dao = Arc::new(MockCardsPipelineDao::default());

        let pipeline = CardsPipeline::selection_only(
            feed_source,
            ml_port,
            dao.clone(),
            Arc::new(FakeGenreTagger::with_failure()),
        )
        .with_user_id(Uuid::new_v4());

        let res = pipeline
            .run(
                Uuid::new_v4(),
                Utc::now(),
                Utc::now(),
                &CardsParams::default(),
            )
            .await;

        assert!(res.is_err());
        let err_str = res.unwrap_err().to_string();
        assert!(
            err_str.contains("batch genre classification failed")
                || err_str.contains("simulated classifier failure")
        );
    }

    #[tokio::test]
    async fn test_genre_score_below_threshold_leaves_genre_none() {
        let id1 = Uuid::new_v4();
        let feed = sample_feed(
            id1,
            "Tech announcement",
            "example.com",
            "2026-03-20T10:00:00Z",
        );
        let feed_source = Arc::new(FakeFeedSource::new(vec![feed]));
        let ml_port = Arc::new(FakeEmbedCluster::new());
        let dao = Arc::new(MockCardsPipelineDao::default());

        let mut low_scores = HashMap::new();
        low_scores.insert("technology".to_string(), 0.3_f32); // 0.3 < default threshold 0.5

        let pipeline = CardsPipeline::selection_only(
            feed_source,
            ml_port,
            dao.clone(),
            Arc::new(FakeGenreTagger::with_fixed(low_scores)),
        )
        .with_user_id(Uuid::new_v4());

        let res = pipeline
            .run(
                Uuid::new_v4(),
                Utc::now(),
                Utc::now(),
                &CardsParams::default(),
            )
            .await
            .expect("pipeline succeeds");

        assert_eq!(res.candidates, 1);
        let candidates = dao.candidates.lock().unwrap().clone();
        let items = candidates[0].items.as_array().unwrap();
        assert!(items[0]["genre"].is_null());
    }

    #[tokio::test]
    async fn test_snapshot_params_equals_effective_params_and_stats_row_carries_them() {
        let id1 = Uuid::new_v4();
        let feed = sample_feed(
            id1,
            "Tech announcement",
            "example.com",
            "2026-03-20T10:00:00Z",
        );
        let feed_source = Arc::new(FakeFeedSource::new(vec![feed]));
        let ml_port = Arc::new(FakeEmbedCluster::new());
        let dao = Arc::new(MockCardsPipelineDao::default());

        let params = CardsParams::default()
            .with_override("alpha", &serde_json::json!(0.7))
            .expect("valid override");

        let pipeline = CardsPipeline::selection_only(
            feed_source,
            ml_port,
            dao.clone(),
            Arc::new(FakeGenreTagger::new()),
        )
        .with_user_id(Uuid::new_v4());

        pipeline
            .run(Uuid::new_v4(), Utc::now(), Utc::now(), &params)
            .await
            .expect("pipeline succeeds");

        let snapshots = dao.snapshots.lock().unwrap().clone();
        assert_eq!(snapshots.len(), 1);
        assert!((snapshots[0].params["alpha"].as_f64().unwrap() - 0.7).abs() < 1e-4);
        assert_eq!(snapshots[0].params_version, "cards-v0.2+alpha=0.7");

        let stats = dao.stats.lock().unwrap().clone();
        assert_eq!(stats.len(), 1);
        assert_eq!(stats[0].params_version, "cards-v0.2+alpha=0.7");
    }

    #[tokio::test]
    async fn test_embedding_cache_determinism_first_run_embeds_second_run_cached_and_identical_candidates()
     {
        let id1 = Uuid::new_v4();
        let id2 = Uuid::new_v4();
        let id3 = Uuid::new_v4();

        let feeds = vec![
            sample_feed(id1, "Article Alpha", "alpha.com", "2026-03-20T10:00:00Z"),
            sample_feed(id2, "Article Beta", "beta.org", "2026-03-20T11:00:00Z"),
            sample_feed(id3, "Article Gamma", "gamma.net", "2026-03-20T12:00:00Z"),
        ];

        let feed_source = Arc::new(FakeFeedSource::new(feeds));
        let ml_port = Arc::new(FakeEmbedCluster::new());
        let dao = Arc::new(MockCardsPipelineDao::default());
        let tagger = Arc::new(FakeGenreTagger::new());
        let user_id = Uuid::new_v4();

        let pipeline = CardsPipeline::selection_only(
            feed_source.clone(),
            ml_port.clone(),
            dao.clone(),
            tagger.clone(),
        )
        .with_user_id(user_id);

        let from = Utc::now();
        let to = Utc::now();
        let params = CardsParams::default()
            .with_override("expected_embed_dim", &serde_json::json!(4))
            .unwrap();

        let job_id1 = Uuid::new_v4();
        let res1 = pipeline
            .run(job_id1, from, to, &params)
            .await
            .expect("first run succeeds");

        assert_eq!(res1.embed_cache_hits, 0);
        assert_eq!(res1.embed_cache_misses, 3);
        assert_eq!(ml_port.embed_calls.lock().unwrap().len(), 1);
        assert_eq!(ml_port.embed_calls.lock().unwrap()[0].len(), 3);

        // Run 2: same inputs on the same DAO
        let job_id2 = Uuid::new_v4();
        let res2 = pipeline
            .run(job_id2, from, to, &params)
            .await
            .expect("second run succeeds");

        assert_eq!(res2.embed_cache_hits, 3);
        assert_eq!(res2.embed_cache_misses, 0);
        // ml_port.embed must NOT have been called again!
        assert_eq!(ml_port.embed_calls.lock().unwrap().len(), 1);

        // Compare candidates from run 1 and run 2: must be identical
        let all_candidates = dao.candidates.lock().unwrap().clone();
        let candidates1: Vec<_> = all_candidates
            .iter()
            .filter(|c| c.job_id == job_id1)
            .collect();
        let candidates2: Vec<_> = all_candidates
            .iter()
            .filter(|c| c.job_id == job_id2)
            .collect();

        assert_eq!(candidates1.len(), candidates2.len());
        assert!(!candidates1.is_empty());
        for (c1, c2) in candidates1.iter().zip(candidates2.iter()) {
            assert_eq!(c1.rank, c2.rank);
            assert_eq!(c1.cluster_fingerprint, c2.cluster_fingerprint);
            assert_eq!(c1.size, c2.size);
            assert_eq!(c1.scores, c2.scores);
            assert_eq!(c1.items, c2.items);
            assert_eq!(c1.domains, c2.domains);
            assert_eq!(c1.centroid, c2.centroid);
        }

        // Stats row verification
        let stats = dao.stats.lock().unwrap().clone();
        let stats1 = stats.iter().find(|s| s.job_id == job_id1).unwrap();
        let stats2 = stats.iter().find(|s| s.job_id == job_id2).unwrap();
        assert_eq!(stats1.embed_cache_hits, 0);
        assert_eq!(stats1.embed_cache_misses, 3);
        assert_eq!(stats2.embed_cache_hits, 3);
        assert_eq!(stats2.embed_cache_misses, 0);
        assert!(stats1.cards_dropped.get("embed_cache_hits").is_none());
        assert!(stats1.cards_dropped.get("embed_cache_misses").is_none());
        assert!(stats2.cards_dropped.get("embed_cache_hits").is_none());
        assert!(stats2.cards_dropped.get("embed_cache_misses").is_none());
    }

    #[tokio::test]
    async fn test_embedding_cache_dimension_mismatch_fails_loudly() {
        let id1 = Uuid::new_v4();
        let feed = sample_feed(
            id1,
            "Article Dimension",
            "example.com",
            "2026-03-20T10:00:00Z",
        );

        let feed_source = Arc::new(FakeFeedSource::new(vec![feed]));
        let ml_port = Arc::new(FakeEmbedCluster::new());
        let dao = Arc::new(MockCardsPipelineDao::default());

        // Pre-populate cache with a 8-dimensional vector
        let norm_text = "Article Dimension — This is a valid and sufficiently long lede text.";
        let hash = compute_embedding_text_hash(norm_text);
        dao.cache.lock().unwrap().insert(
            hash.clone(),
            (
                "bge-m3".to_string(),
                8,
                vec![1.0, 0.0, 0.0, 0.0, 0.5, 0.5, 0.5, 0.5],
            ),
        );

        let pipeline = CardsPipeline::selection_only(
            feed_source,
            ml_port,
            dao.clone(),
            Arc::new(FakeGenreTagger::new()),
        )
        .with_user_id(Uuid::new_v4());

        // Pipeline expects 4-dimensional embeddings
        let params = CardsParams::default()
            .with_override("expected_embed_dim", &serde_json::json!(4))
            .unwrap();

        let res = pipeline
            .run(Uuid::new_v4(), Utc::now(), Utc::now(), &params)
            .await;
        assert!(res.is_err(), "pipeline must fail on dimension mismatch");
        let err = format!("{:#}", res.unwrap_err());
        assert!(
            err.contains("dimension mismatch"),
            "expected error to mention dimension mismatch, got: {err}"
        );
    }

    #[tokio::test]
    async fn test_embedding_cache_model_mismatch_fails_closed() {
        let id1 = Uuid::new_v4();
        let feed = sample_feed(id1, "Article Model", "example.com", "2026-03-20T10:00:00Z");

        let feed_source = Arc::new(FakeFeedSource::new(vec![feed]));
        let ml_port = Arc::new(FakeEmbedCluster::new());
        let dao = Arc::new(MockCardsPipelineDao::default());

        // Pre-populate cache with a different model identity
        let norm_text = "Article Model — This is a valid and sufficiently long lede text.";
        let hash = compute_embedding_text_hash(norm_text);
        dao.cache.lock().unwrap().insert(
            hash.clone(),
            ("other-model-v1".to_string(), 4, vec![1.0, 0.0, 0.0, 0.0]),
        );

        let pipeline = CardsPipeline::selection_only(
            feed_source,
            ml_port,
            dao.clone(),
            Arc::new(FakeGenreTagger::new()),
        )
        .with_user_id(Uuid::new_v4());

        // Pipeline expects "bge-m3"
        let params = CardsParams::default()
            .with_override("expected_embed_dim", &serde_json::json!(4))
            .unwrap();

        let res = pipeline
            .run(Uuid::new_v4(), Utc::now(), Utc::now(), &params)
            .await;
        assert!(
            res.is_err(),
            "pipeline must fail closed on model identity mismatch"
        );
        let err = format!("{:#}", res.unwrap_err());
        assert!(
            err.contains("model identity mismatch"),
            "expected error to mention model identity mismatch, got: {err}"
        );
    }

    #[tokio::test]
    async fn test_embedding_cache_partial_hit_preserves_input_order() {
        let id1 = Uuid::new_v4();
        let id2 = Uuid::new_v4();
        let id3 = Uuid::new_v4();

        let feeds = vec![
            sample_feed(id1, "Item First", "first.com", "2026-03-20T10:00:00Z"),
            sample_feed(
                id2,
                "Item Second Cached",
                "second.com",
                "2026-03-20T11:00:00Z",
            ),
            sample_feed(id3, "Item Third", "third.com", "2026-03-20T12:00:00Z"),
        ];

        let feed_source = Arc::new(FakeFeedSource::new(feeds));
        let ml_port = Arc::new(FakeEmbedCluster::new());
        let dao = Arc::new(MockCardsPipelineDao::default());

        // Pre-populate cache for Item Second Cached only
        let text2 = "Item Second Cached — This is a valid and sufficiently long lede text.";
        let hash2 = compute_embedding_text_hash(text2);
        let cached_vec = vec![0.7, 0.7, 0.0, 0.0];
        dao.cache
            .lock()
            .unwrap()
            .insert(hash2.clone(), ("bge-m3".to_string(), 4, cached_vec.clone()));

        let pipeline = CardsPipeline::selection_only(
            feed_source,
            ml_port.clone(),
            dao.clone(),
            Arc::new(FakeGenreTagger::new()),
        )
        .with_user_id(Uuid::new_v4());

        let params = CardsParams::default()
            .with_override("expected_embed_dim", &serde_json::json!(4))
            .unwrap();

        let res = pipeline
            .run(Uuid::new_v4(), Utc::now(), Utc::now(), &params)
            .await
            .expect("pipeline succeeds with partial cache hits");

        assert_eq!(res.embed_cache_hits, 1);
        assert_eq!(res.embed_cache_misses, 2);

        // Only Item First and Item Third should have been sent to ml_port.embed
        let calls = ml_port.embed_calls.lock().unwrap().clone();
        assert_eq!(calls.len(), 1);
        assert_eq!(calls[0].len(), 2);
        assert!(calls[0][0].starts_with("Item First"));
        assert!(calls[0][1].starts_with("Item Third"));

        // All 3 items should now be in the cache
        let cache_lock = dao.cache.lock().unwrap();
        assert_eq!(cache_lock.len(), 3);
        assert!(cache_lock.contains_key(&hash2));
    }
}
