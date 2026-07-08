use std::collections::{BTreeSet, HashMap};
use std::sync::{Arc, LazyLock};

use anyhow::{Context, Result};
use async_trait::async_trait;
use regex::Regex;
use tracing::{debug, info, warn};

use crate::clients::KnowledgeSovereignClient;
use crate::clients::knowledge_sovereign::publish::{ConfirmedCluster, publish_topic_snapshots};
use crate::clients::news_creator::Reference;
use crate::clients::tag_generator::TagGeneratorClient;
use crate::error::RecapError;
use crate::scheduler::JobContext;
use crate::store::dao::RecapDao;
use crate::store::models::RecapOutput;

use super::dispatch::{DispatchResult, GenreResult};
use crate::store::models::PersistedGenre;
use serde::{Deserialize, Serialize};
use serde_json::json;

/// `[1]` / `[42]` 形式の出典マーカーを抽出する。news-creator の REFERENCE_MARKER_RE と対称。
static REFERENCE_MARKER_RE: LazyLock<Regex> =
    LazyLock::new(|| Regex::new(r"\[(\d+)\]").expect("REFERENCE_MARKER_RE must compile"));

/// `references[].article_id` を信用してよいかを shape で判定する。
/// 本物の article_id は常に UUID。production で LLM がドメイン文字列を返す事例が出たため
/// (例: `"article_id": "dev.to"`)、UUID 形状でない値は捨てて URL 解決に倒す。
fn is_uuid_shape(s: &str) -> bool {
    uuid::Uuid::parse_str(s).is_ok()
}

/// URL から host 部分のみを取り出す。scheme/path/`www.` を剥がす。純ドメイン文字列
/// (例: `"dev.to"`) はそのまま host 扱いになる。比較は lowercase。
fn url_host(url: &str) -> Option<String> {
    let s = url.trim();
    let s = s
        .strip_prefix("https://")
        .or_else(|| s.strip_prefix("http://"))
        .unwrap_or(s);
    let host = s.split('/').next().unwrap_or("");
    let host = host.strip_prefix("www.").unwrap_or(host);
    if host.is_empty() {
        None
    } else {
        Some(host.to_ascii_lowercase())
    }
}

/// bullet 内の `[n]` を `references[n-1]` 経由で article_id に解決し、
/// `recap_subworker_sentences.id` の集合を返す純粋関数。
///
/// 解決戦略 (ADR-890 followup, 排他ではなく合流させる):
/// 1. `references[n-1].article_id` が UUID 形状なら候補集合に加える。
///    UUID でない (LLM がドメイン文字列を埋めた) 場合は無視する。
/// 2. `url_to_article` から URL を解決する (完全一致 + host 単位一致)。
///    `ref.url` が純ドメイン (`"dev.to"`) でも host 一致で同ホストの article 群を拾う。
/// 3. 集合 union を `article_to_sentence_ids` に問い合わせて sentence_id を集める。
/// 4. 解決できない marker は `warn!` で surface する (silent loss を避ける)。
///
/// 結果は重複除去 + 昇順ソート。bullet に `[n]` が無いまたは references が空なら `vec![]`。
fn reconcile_bullet_citations(
    bullet: &str,
    refs: &[Reference],
    url_to_article: &HashMap<String, String>,
    article_to_sentence_ids: &HashMap<String, Vec<i64>>,
) -> Vec<i64> {
    let mut seen = BTreeSet::<i64>::new();

    if refs.is_empty() {
        return Vec::new();
    }

    for cap in REFERENCE_MARKER_RE.captures_iter(bullet) {
        let n: usize = match cap[1].parse() {
            Ok(v) if v >= 1 && v <= refs.len() => v,
            Ok(_) | Err(_) => {
                debug!(marker = %&cap[0], refs_len = refs.len(),
                    "ignoring out-of-range citation marker");
                continue;
            }
        };
        let r = &refs[n - 1];
        let mut article_ids: BTreeSet<String> = BTreeSet::new();

        // Path 1: UUID-shape article_id を採用 (verbatim 信用ではなく、shape を見てから DB 照合)
        if let Some(aid) = r.article_id.as_ref() {
            if is_uuid_shape(aid) {
                article_ids.insert(aid.clone());
            }
        }

        // Path 2: URL match を常に併走。完全一致 → host 単位一致 (multi-match 許容)。
        let ref_host = url_host(&r.url);
        for (u, a) in url_to_article {
            if u == &r.url {
                article_ids.insert(a.clone());
                continue;
            }
            if let (Some(rh), Some(uh)) = (ref_host.as_deref(), url_host(u).as_deref()) {
                if rh == uh {
                    article_ids.insert(a.clone());
                }
            }
        }

        if article_ids.is_empty() {
            warn!(
                unmatched_ref_url = %r.url,
                unmatched_article_id = ?r.article_id,
                marker = %&cap[0],
                "citation marker unresolvable: article_id is non-UUID/None and URL did not match any known article (bullet citation will be silently empty)"
            );
            continue;
        }

        for aid in &article_ids {
            if let Some(ids) = article_to_sentence_ids.get(aid) {
                seen.extend(ids.iter().copied());
            }
        }
    }

    seen.into_iter().collect()
}

/// Sanitize title and summary text by removing markdown code blocks
fn sanitize_title(text: &str) -> String {
    text.replace("```json", "")
        .replace("```", "")
        .trim()
        .to_string()
}

/// per-genre の silent failure を `recap_failed_tasks` テーブルへ surface する。
/// DAO 書き込み自体が失敗しても pipeline は継続させる（二重失敗で全体を止めない）。
async fn record_failed_genre(
    dao: &dyn RecapDao,
    job_id: uuid::Uuid,
    stage: &str,
    genre: &str,
    error: &str,
) {
    let payload = json!({ "genre": genre });
    if let Err(e) = dao
        .insert_failed_task(job_id, stage, Some(&payload), Some(error))
        .await
    {
        warn!(
            job_id = %job_id,
            genre = %genre,
            stage = %stage,
            error = ?e,
            "failed to record per-genre failure to recap_failed_tasks (continuing)"
        );
    }
}

/// 永続化結果。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct PersistResult {
    pub(crate) job_id: uuid::Uuid,
    pub(crate) genres_stored: usize,
    pub(crate) genres_failed: usize,
    /// 証拠不足でスキップされたジャンル数（記事数が閾値未満）
    pub(crate) genres_skipped: usize,
    /// 記事が1件も割り当てられなかったジャンル数
    pub(crate) genres_no_evidence: usize,
    /// 設定された全ジャンル数
    pub(crate) total_genres: usize,
}

#[async_trait]
pub(crate) trait PersistStage: Send + Sync {
    async fn persist(&self, job: &JobContext, result: DispatchResult) -> Result<PersistResult>;
}

/// 最終確定物をJSONBフィールドに保存するPersistStage。
///
/// `sovereign` is the optional Knowledge Sovereign client used to emit
/// `recap.topic_snapshotted.v1` events after the genre persist loop. When
/// `None`, the emit is skipped silently (warn-and-continue). When `Some`,
/// every successful cluster's `(cluster_id, top_terms)` is forwarded so
/// Surface Planner v2 can light up `topic_overlap_count` (ADR-000905 §PR-C2).
#[allow(dead_code)]
pub(crate) struct FinalSectionPersistStage {
    dao: Arc<dyn RecapDao>,
    tag_generator: Option<Arc<TagGeneratorClient>>,
    sovereign: Option<Arc<KnowledgeSovereignClient>>,
}

impl FinalSectionPersistStage {
    pub(crate) fn new(
        dao: Arc<dyn RecapDao>,
        tag_generator: Option<Arc<TagGeneratorClient>>,
    ) -> Self {
        Self {
            dao,
            tag_generator,
            sovereign: None,
        }
    }

    /// Inject the Knowledge Sovereign client used by the
    /// `recap.topic_snapshotted.v1` publish pass. Defaults to `None` so
    /// existing call sites (orchestrator, tests) remain source-compatible
    /// and the emit stays opt-in.
    #[allow(dead_code)]
    pub(crate) fn with_sovereign(
        mut self,
        sovereign: Option<Arc<KnowledgeSovereignClient>>,
    ) -> Self {
        self.sovereign = sovereign;
        self
    }
}

/// Outcome of persisting a single genre, folded into `PersistResult`'s
/// aggregate counters by the caller.
enum GenreOutcome {
    Stored,
    Failed,
    Skipped,
    NoEvidence,
}

impl FinalSectionPersistStage {
    /// Resolve `(summary_id, SummaryResponse)` for a genre, either from the
    /// dispatch result directly or (on resume) by refetching the persisted
    /// `body_json` from the database. On any failure this already records
    /// the per-genre failure via `record_failed_genre` — the caller only
    /// needs to fold the `Err` into `GenreOutcome::Failed`.
    async fn resolve_summary_response(
        &self,
        job: &JobContext,
        genre: &str,
        genre_result: &GenreResult,
    ) -> Result<(String, crate::clients::news_creator::SummaryResponse), ()> {
        match (
            &genre_result.summary_response_id,
            &genre_result.summary_response,
        ) {
            (Some(summary_id), Some(response)) => Ok((summary_id.clone(), response.clone())),
            (Some(summary_id), None) => {
                // リジューム時: データベースから再取得
                match self.dao.get_recap_output_body_json(job.job_id, genre).await {
                    Ok(Some(body_json)) => {
                        match serde_json::from_value::<crate::clients::news_creator::SummaryResponse>(
                            body_json,
                        ) {
                            Ok(response) => {
                                info!(
                                    job_id = %job.job_id,
                                    genre = %genre,
                                    "recovered summary_response from database"
                                );
                                Ok((summary_id.clone(), response))
                            }
                            Err(e) => {
                                warn!(
                                    job_id = %job.job_id,
                                    genre = %genre,
                                    error = ?e,
                                    "failed to deserialize summary_response from database"
                                );
                                record_failed_genre(
                                    self.dao.as_ref(),
                                    job.job_id,
                                    "persist_lookup",
                                    genre,
                                    &format!("deserialize summary_response failed: {e}"),
                                )
                                .await;
                                Err(())
                            }
                        }
                    }
                    Ok(None) => {
                        warn!(
                            job_id = %job.job_id,
                            genre = %genre,
                            summary_id = %summary_id,
                            "summary_response not found in database"
                        );
                        record_failed_genre(
                            self.dao.as_ref(),
                            job.job_id,
                            "persist_lookup",
                            genre,
                            &format!("summary_response not found in database for id {summary_id}"),
                        )
                        .await;
                        Err(())
                    }
                    Err(e) => {
                        warn!(
                            job_id = %job.job_id,
                            genre = %genre,
                            error = ?e,
                            "failed to fetch summary_response from database"
                        );
                        record_failed_genre(
                            self.dao.as_ref(),
                            job.job_id,
                            "persist_lookup",
                            genre,
                            &format!("fetch summary_response failed: {e}"),
                        )
                        .await;
                        Err(())
                    }
                }
            }
            (None, _) => {
                warn!(
                    job_id = %job.job_id,
                    genre = %genre,
                    "genre missing summary response id"
                );
                record_failed_genre(
                    self.dao.as_ref(),
                    job.job_id,
                    "persist_lookup",
                    genre,
                    "genre missing summary_response_id",
                )
                .await;
                Err(())
            }
        }
    }

    /// Live path for `build_sources`: article IDs come straight from the
    /// dispatch result's `clustering_response`, so representative titles are
    /// available without a extra lookup.
    async fn build_sources_from_clustering(
        &self,
        job: &JobContext,
        genre: &str,
        clustering: &crate::clients::subworker::ClusteringResponse,
    ) -> Vec<serde_json::Value> {
        let mut sources_metadata: Vec<serde_json::Value> = Vec::new();

        // Collect all article IDs from representatives
        let article_ids: Vec<String> = clustering
            .clusters
            .iter()
            .flat_map(|c| c.representatives.iter().map(|r| r.article_id.clone()))
            .collect::<std::collections::HashSet<_>>()
            .into_iter()
            .collect();

        // Pre-compute article ID to title mapping
        let article_titles: std::collections::HashMap<String, String> = clustering
            .clusters
            .iter()
            .flat_map(|c| &c.representatives)
            .map(|r| (r.article_id.clone(), r.text.clone()))
            .collect();

        match self
            .dao
            .get_article_metadata(job.job_id, &article_ids)
            .await
        {
            Ok(metadata) => {
                // Convert to source objects
                for (article_id, (published_at, source_url)) in metadata {
                    let title = article_titles
                        .get(&article_id)
                        .cloned()
                        .unwrap_or_else(|| "Source Article".to_string());

                    sources_metadata.push(json!({
                        "title": title,
                        "url": source_url,
                        "published_at": published_at,
                        "article_id": article_id
                    }));
                }
            }
            Err(e) => {
                warn!(
                    job_id = %job.job_id,
                    genre = %genre,
                    error = ?e,
                    "failed to fetch article metadata for sources"
                );
            }
        }

        sources_metadata
    }

    /// Resume path for `build_sources`: `clustering_response` is `None`
    /// (the in-memory dispatch result didn't survive a crash/restart), so
    /// cluster/article IDs are recovered from the persisted `body_json`
    /// instead.
    async fn build_sources_from_resume(
        &self,
        job: &JobContext,
        genre: &str,
    ) -> Vec<serde_json::Value> {
        let mut sources_metadata: Vec<serde_json::Value> = Vec::new();

        // リジューム時: clustering_responseがNoneの場合
        // body_jsonからクラスタ情報を取得するか、データベースから直接取得
        // ここでは簡易的にbody_jsonから取得を試みる
        let Ok(Some(body_json)) = self.dao.get_recap_output_body_json(job.job_id, genre).await
        else {
            return sources_metadata;
        };

        // body_jsonからクラスタ情報を抽出（構造に依存）
        let Some(clusters) = body_json.get("clusters").and_then(|c| c.as_array()) else {
            return sources_metadata;
        };

        let article_ids: Vec<String> = clusters
            .iter()
            .flat_map(|cluster| {
                cluster
                    .get("representatives")
                    .and_then(|r| r.as_array())
                    .map_or(&[] as &[serde_json::Value], |arr| arr.as_slice())
                    .iter()
                    .filter_map(|rep| {
                        rep.get("article_id")
                            .and_then(|id| id.as_str())
                            .map(str::to_string)
                    })
            })
            .collect::<std::collections::HashSet<_>>()
            .into_iter()
            .collect();

        match self
            .dao
            .get_article_metadata(job.job_id, &article_ids)
            .await
        {
            Ok(metadata) => {
                for (article_id, (published_at, source_url)) in metadata {
                    sources_metadata.push(json!({
                        "title": "Source Article",
                        "url": source_url,
                        "published_at": published_at,
                        "article_id": article_id
                    }));
                }
            }
            Err(e) => {
                warn!(
                    job_id = %job.job_id,
                    genre = %genre,
                    error = ?e,
                    "failed to fetch article metadata for sources (resume)"
                );
            }
        }

        sources_metadata
    }

    /// Collect and rank the top-5 source articles for a genre's bullets,
    /// either from the live `clustering_response` or (on resume) from the
    /// persisted `body_json`. Sorted by `published_at` desc, parsed to an
    /// actual instant (rather than compared as RFC3339 strings) so entries
    /// differing in offset or fractional-second precision still order
    /// correctly.
    async fn build_sources(
        &self,
        job: &JobContext,
        genre: &str,
        genre_result: &GenreResult,
    ) -> Vec<serde_json::Value> {
        let mut sources_metadata = if let Some(clustering) = &genre_result.clustering_response {
            self.build_sources_from_clustering(job, genre, clustering)
                .await
        } else {
            self.build_sources_from_resume(job, genre).await
        };

        // Sort sources by published_at desc. Parsing to an actual instant
        // (rather than comparing the RFC3339 strings lexicographically)
        // avoids mis-ordering when entries differ in offset or in
        // fractional-second precision.
        let parse_published_at = |v: &serde_json::Value| {
            v.as_object()
                .and_then(|m| m.get("published_at"))
                .and_then(|v| v.as_str())
                .and_then(|s| chrono::DateTime::parse_from_rfc3339(s).ok())
                .map(|dt| dt.with_timezone(&chrono::Utc))
        };
        sources_metadata.sort_by_key(|a| std::cmp::Reverse(parse_published_at(a)));

        // Limit sources to top 5
        sources_metadata.into_iter().take(5).collect()
    }

    /// Build the per-bullet JSON array (text + sources + reconciled
    /// citation sentence IDs) for a genre's summary response.
    async fn build_bullets_json(
        &self,
        job: &JobContext,
        genre: &str,
        genre_result: &GenreResult,
        summary_response: &crate::clients::news_creator::SummaryResponse,
        top_sources: &[serde_json::Value],
    ) -> serde_json::Value {
        // Build (url -> article_id) for URL-fallback in citation reconciliation.
        // top_sources は [{title, url, published_at, article_id}, ...] 形式。
        let url_to_article: HashMap<String, String> = top_sources
            .iter()
            .filter_map(|s| {
                let aid = s.get("article_id")?.as_str()?.to_string();
                let url = s.get("url")?.as_str()?.to_string();
                Some((url, aid))
            })
            .collect();

        // Fetch (article_id -> Vec<sentence DB id>) for this genre's run.
        // 失敗・resume パス・run_id 0 は空 map で degrade (fail-open)。
        let article_to_sentence_ids: HashMap<String, Vec<i64>> =
            match genre_result.clustering_response.as_ref().map(|c| c.run_id) {
                Some(run_id) if run_id > 0 => self
                    .dao
                    .get_sentence_ids_by_run(run_id)
                    .await
                    .unwrap_or_else(|e| {
                        warn!(
                            job_id = %job.job_id,
                            genre = %genre,
                            error = ?e,
                            "failed to fetch sentence ids for citation reconciliation"
                        );
                        HashMap::new()
                    }),
                // TODO(ADR-followup): resume path needs run_id from recap_subworker_runs lookup.
                _ => HashMap::new(),
            };

        let refs: &[Reference] = summary_response
            .summary
            .references
            .as_deref()
            .unwrap_or(&[]);

        let bullet_values = summary_response
            .summary
            .bullets
            .iter()
            .map(|bullet| {
                let sentence_ids = reconcile_bullet_citations(
                    bullet,
                    refs,
                    &url_to_article,
                    &article_to_sentence_ids,
                );
                json!({
                    "text": bullet,
                    "sources": top_sources,
                    "source_sentence_ids": sentence_ids,
                })
            })
            .collect::<Vec<_>>();
        serde_json::Value::Array(bullet_values)
    }

    /// Write a genre's resolved output through `persist_genre_output`
    /// (single transaction covering `recap_outputs` + the genre pointer),
    /// recording a failed-genre entry and returning `GenreOutcome::Failed`
    /// on error rather than propagating it.
    async fn write_genre_output(
        &self,
        job: &JobContext,
        genre: &str,
        output: &RecapOutput,
        persisted_genre: &PersistedGenre,
    ) -> Result<GenreOutcome> {
        // Written in a single transaction (`persist_genre_output`) so a
        // failure on one write can't leave the other stranded — e.g. a
        // recap_outputs row with no matching recap_sections pointer.
        if let Err(err) = self.dao.persist_genre_output(output, persisted_genre).await {
            warn!(
                job_id = %job.job_id,
                genre = %genre,
                error = ?err,
                "failed to persist recap output and section pointer"
            );
            record_failed_genre(
                self.dao.as_ref(),
                job.job_id,
                "persist_write",
                genre,
                &format!("persist_genre_output failed: {err}"),
            )
            .await;
            return Ok(GenreOutcome::Failed);
        }

        debug!(
            job_id = %job.job_id,
            genre = %genre,
            "genre processed successfully"
        );
        Ok(GenreOutcome::Stored)
    }

    /// Persist a single genre's final section: classify a pre-existing
    /// dispatch error, resolve the summary response, build sources and
    /// citations, extract tags, and write the output row. Returns the
    /// outcome bucket the caller folds into `PersistResult`'s counters.
    async fn persist_genre(
        &self,
        job: &JobContext,
        genre: &str,
        genre_result: &GenreResult,
    ) -> Result<GenreOutcome> {
        // エラーがある場合は分類
        if let Some(error_msg) = &genre_result.error {
            // `error_kind` を見て分類する。文字列 (`error_msg`) は診断用に
            // 保持するのみで、分類には使わない — 任意のエラーメッセージが
            // 偶然 "no evidence" 等の部分文字列を含んでいても、本物の
            // no-evidence/insufficient-documents 状態と誤分類されない。
            return Ok(match &genre_result.error_kind {
                Some(RecapError::NoEvidence) => GenreOutcome::NoEvidence,
                Some(RecapError::InsufficientDocuments { .. }) => GenreOutcome::Skipped,
                _ => {
                    // その他のエラー（クラスタリング失敗、サマリー生成失敗など）
                    warn!(
                        job_id = %job.job_id,
                        genre = %genre,
                        error = ?genre_result.error,
                        "skipping genre with error"
                    );
                    record_failed_genre(
                        self.dao.as_ref(),
                        job.job_id,
                        "dispatch_summary",
                        genre,
                        error_msg,
                    )
                    .await;
                    GenreOutcome::Failed
                }
            });
        }

        // summary_responseがNoneの場合、データベースから再取得を試みる
        let Ok((summary_id, summary_response)) = self
            .resolve_summary_response(job, genre, genre_result)
            .await
        else {
            return Ok(GenreOutcome::Failed);
        };

        // Collect source articles for bullets
        let top_sources = self.build_sources(job, genre, genre_result).await;

        let bullets_json = self
            .build_bullets_json(job, genre, genre_result, &summary_response, &top_sources)
            .await;

        // 先に必要な値を取得してからsummary_responseを移動
        let summary_title = summary_response.summary.title.clone();
        let summary_bullets = summary_response.summary.bullets.clone();
        let summary_text = summary_bullets.join("\n");
        let body_json = serde_json::to_value(&summary_response)
            .context("failed to convert summary response to JSON")?;

        // Sanitize title to remove markdown code blocks
        let sanitized_title = sanitize_title(&summary_title);
        let sanitized_summary = sanitize_title(&summary_text);

        // tag-generatorでセマンティックタグを抽出
        let tags = if let Some(ref tg) = self.tag_generator {
            // `sanitized_summary` is already `summary_bullets.join("\n")`
            // run through `sanitize_title`; appending the unsanitized
            // bullets again here just sent the same content twice.
            match tg.extract_tags(genre, &sanitized_summary).await {
                Ok(tags) => {
                    debug!(
                        job_id = %job.job_id,
                        genre = %genre,
                        tag_count = tags.len(),
                        "extracted semantic tags for genre"
                    );
                    tags
                }
                Err(e) => {
                    warn!(
                        job_id = %job.job_id,
                        genre = %genre,
                        error = ?e,
                        "failed to extract semantic tags, continuing without tags"
                    );
                    Vec::new()
                }
            }
        } else {
            Vec::new()
        };

        let output = RecapOutput::new(
            job.job_id,
            genre,
            summary_id.clone(),
            sanitized_title,
            sanitized_summary,
            bullets_json,
            body_json,
        )
        .with_tags(tags);
        let persisted_genre =
            PersistedGenre::new(job.job_id, genre).with_response_id(Some(summary_id));

        self.write_genre_output(job, genre, &output, &persisted_genre)
            .await
    }
}

#[async_trait]
impl PersistStage for FinalSectionPersistStage {
    async fn persist(&self, job: &JobContext, result: DispatchResult) -> Result<PersistResult> {
        info!(
            job_id = %job.job_id,
            genre_count = result.genre_results.len(),
            "persisting final sections to database"
        );

        let mut genres_stored = 0;
        let mut genres_failed = 0;
        let mut genres_skipped = 0;
        let mut genres_no_evidence = 0;
        let total_genres = result.all_genres.len();

        for (genre, genre_result) in &result.genre_results {
            match self.persist_genre(job, genre, genre_result).await? {
                GenreOutcome::Stored => genres_stored += 1,
                GenreOutcome::Failed => genres_failed += 1,
                GenreOutcome::Skipped => genres_skipped += 1,
                GenreOutcome::NoEvidence => genres_no_evidence += 1,
            }
        }

        let persist_result = PersistResult {
            job_id: job.job_id,
            genres_stored,
            genres_failed,
            genres_skipped,
            genres_no_evidence,
            total_genres,
        };

        info!(
            job_id = %persist_result.job_id,
            total_genres = persist_result.total_genres,
            genres_stored = persist_result.genres_stored,
            genres_failed = persist_result.genres_failed,
            genres_skipped = persist_result.genres_skipped,
            genres_no_evidence = persist_result.genres_no_evidence,
            "completed persisting final sections"
        );

        // Knowledge Loop Completion Phase 1 §2 — emit `recap.topic_snapshotted.v1`
        // for every successful cluster. The publish helper handles its own
        // warn-and-continue logging; emit failures must NOT fail the recap
        // pipeline (ADR-000905 §PR-C2). When the client or user/tenant scope
        // is absent (system batch without a single-user owner) the helper
        // short-circuits and the recap completes as today.
        if let (Some(sovereign), Some(user_id), Some(tenant_id)) =
            (self.sovereign.as_ref(), job.user_id, job.tenant_id)
        {
            let window_end = chrono::Utc::now();
            let window_start = window_end - chrono::Duration::days(i64::from(job.window_days));

            let confirmed: Vec<ConfirmedCluster> = result
                .genre_results
                .iter()
                .filter(|(_genre, genre_result)| genre_result.error.is_none())
                .filter_map(|(_genre, genre_result)| genre_result.clustering_response.as_ref())
                .flat_map(|clustering| {
                    clustering
                        .clusters
                        .iter()
                        .filter(|cluster| !cluster.top_terms.is_empty())
                        .map(|cluster| ConfirmedCluster {
                            cluster_id: i64::from(cluster.cluster_id),
                            top_terms: cluster.top_terms.clone(),
                        })
                })
                .collect();

            if !confirmed.is_empty() {
                let outcome = publish_topic_snapshots(
                    sovereign.as_ref(),
                    user_id,
                    tenant_id,
                    window_start,
                    window_end,
                    &confirmed,
                )
                .await;
                info!(
                    job_id = %persist_result.job_id,
                    user_id = %user_id,
                    attempted = outcome.attempted,
                    succeeded = outcome.succeeded,
                    failed = outcome.failed,
                    skipped_empty_terms = outcome.skipped_empty_terms,
                    "recap.topic_snapshotted.v1 publish pass complete"
                );
            }
        }

        Ok(persist_result)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::clients::news_creator::{Reference, SummaryResponse};
    use crate::clients::subworker::ClusteringResponse;
    use crate::pipeline::dispatch::GenreResult;
    use crate::scheduler::JobContext;
    use crate::store::dao::mock::MockRecapDao;

    /// reconciler 用テストフィクスチャ。`refs` / `url_to_article` / `article_to_sentence_ids` を
    /// 個別のテストで一行で組み立てるためのヘルパ。
    fn ref_with_id(id: i32, url: &str, article_id: Option<&str>) -> Reference {
        Reference {
            id,
            url: url.to_string(),
            domain: "example.com".to_string(),
            article_id: article_id.map(str::to_string),
        }
    }

    fn url_map(pairs: &[(&str, &str)]) -> HashMap<String, String> {
        pairs
            .iter()
            .map(|(u, a)| ((*u).to_string(), (*a).to_string()))
            .collect()
    }

    fn sid_map(pairs: &[(&str, Vec<i64>)]) -> HashMap<String, Vec<i64>> {
        pairs
            .iter()
            .map(|(a, ids)| ((*a).to_string(), ids.clone()))
            .collect()
    }

    #[test]
    fn persist_result_tracks_success_and_failure() {
        let result = PersistResult {
            job_id: uuid::Uuid::new_v4(),
            genres_stored: 5,
            genres_failed: 2,
            genres_skipped: 1,
            genres_no_evidence: 1,
            total_genres: 9,
        };

        assert_eq!(result.genres_stored, 5);
        assert_eq!(result.genres_failed, 2);
        assert_eq!(result.genres_skipped, 1);
        assert_eq!(result.genres_no_evidence, 1);
        assert_eq!(result.total_genres, 9);
    }

    fn dispatch_with_error(genre: &str, error: &str) -> DispatchResult {
        dispatch_with_error_kind(genre, error, None)
    }

    fn dispatch_with_error_kind(
        genre: &str,
        error: &str,
        error_kind: Option<RecapError>,
    ) -> DispatchResult {
        let mut genre_results = HashMap::new();
        genre_results.insert(
            genre.to_string(),
            GenreResult {
                genre: genre.to_string(),
                clustering_response: None,
                summary_response_id: None,
                summary_response: None,
                error: Some(error.to_string()),
                error_kind,
            },
        );
        DispatchResult {
            job_id: uuid::Uuid::new_v4(),
            genre_results,
            success_count: 0,
            failure_count: 1,
            all_genres: vec![genre.to_string()],
        }
    }

    #[tokio::test]
    async fn persist_records_failed_task_on_generic_genre_error() {
        let dao = Arc::new(MockRecapDao::new());
        let stage = FinalSectionPersistStage::new(dao.clone(), None);
        let dispatch = dispatch_with_error("consumer_tech", "LLM batch summary failed");
        let job = JobContext::new(dispatch.job_id, dispatch.all_genres.clone());

        let result = stage.persist(&job, dispatch).await.expect("persist ok");

        assert_eq!(result.genres_failed, 1);

        let recorded = dao.failed_tasks();
        assert_eq!(
            recorded.len(),
            1,
            "expected insert_failed_task to be called once"
        );
        let call = &recorded[0];
        assert_eq!(call.stage, "dispatch_summary");
        assert_eq!(
            call.error.as_deref(),
            Some("LLM batch summary failed"),
            "error text should be preserved for diagnosis"
        );
        let payload = call
            .payload
            .as_ref()
            .expect("payload should identify which genre failed");
        assert_eq!(
            payload
                .get("genre")
                .and_then(|g| g.as_str())
                .expect("payload.genre"),
            "consumer_tech"
        );
    }

    #[tokio::test]
    async fn persist_does_not_record_failed_task_for_no_evidence() {
        let dao = Arc::new(MockRecapDao::new());
        let stage = FinalSectionPersistStage::new(dao.clone(), None);
        let dispatch = dispatch_with_error_kind(
            "consumer_tech",
            "no evidence for genre",
            Some(RecapError::NoEvidence),
        );
        let job = JobContext::new(dispatch.job_id, dispatch.all_genres.clone());

        let result = stage.persist(&job, dispatch).await.expect("persist ok");

        assert_eq!(result.genres_no_evidence, 1);
        let recorded = dao.failed_tasks();
        assert!(
            recorded.is_empty(),
            "no evidence is an expected outcome, must not pollute recap_failed_tasks"
        );
    }

    #[tokio::test]
    async fn persist_does_not_record_failed_task_for_insufficient_documents() {
        let dao = Arc::new(MockRecapDao::new());
        let stage = FinalSectionPersistStage::new(dao.clone(), None);
        let dispatch = dispatch_with_error_kind(
            "consumer_tech",
            "insufficient documents expected >= 3",
            Some(RecapError::InsufficientDocuments { min: 3, found: 1 }),
        );
        let job = JobContext::new(dispatch.job_id, dispatch.all_genres.clone());

        let result = stage.persist(&job, dispatch).await.expect("persist ok");

        assert_eq!(result.genres_skipped, 1);
        let recorded = dao.failed_tasks();
        assert!(
            recorded.is_empty(),
            "insufficient documents is an expected skip, must not pollute recap_failed_tasks"
        );
    }

    /// RED case for the stringly-typed anti-pattern this fix replaces: a
    /// *real* failure whose message happens to contain the substring
    /// "no evidence" (e.g. an LLM diagnostic quoted inside the error text)
    /// must NOT be swallowed as the benign "no genre evidence" completion
    /// state. Classification must be driven by `error_kind`, not by
    /// `error.contains(...)` on arbitrary message text.
    #[tokio::test]
    async fn persist_records_failed_task_when_message_merely_mentions_no_evidence() {
        let dao = Arc::new(MockRecapDao::new());
        let stage = FinalSectionPersistStage::new(dao.clone(), None);
        let dispatch = dispatch_with_error(
            "consumer_tech",
            "Summary generation failed: model returned no evidence for its citations",
        );
        let job = JobContext::new(dispatch.job_id, dispatch.all_genres.clone());

        let result = stage.persist(&job, dispatch).await.expect("persist ok");

        assert_eq!(
            result.genres_no_evidence, 0,
            "a real summary-generation failure must not be miscounted as no-evidence"
        );
        assert_eq!(result.genres_failed, 1);
        let recorded = dao.failed_tasks();
        assert_eq!(
            recorded.len(),
            1,
            "a real failure whose text happens to contain 'no evidence' must still be recorded"
        );
    }

    /// Same RED case for the "insufficient documents" bucket: a batch-API
    /// failure message that happens to mention "insufficient documents" (e.g.
    /// an unrelated cache eviction diagnostic) must not be misclassified as
    /// the benign skip.
    #[tokio::test]
    async fn persist_records_failed_task_when_message_merely_mentions_insufficient_documents() {
        let dao = Arc::new(MockRecapDao::new());
        let stage = FinalSectionPersistStage::new(dao.clone(), None);
        let dispatch = dispatch_with_error(
            "consumer_tech",
            "Batch API failed: insufficient documents in response cache, evicting",
        );
        let job = JobContext::new(dispatch.job_id, dispatch.all_genres.clone());

        let result = stage.persist(&job, dispatch).await.expect("persist ok");

        assert_eq!(
            result.genres_skipped, 0,
            "an unrelated failure must not be miscounted as the insufficient-documents skip"
        );
        assert_eq!(result.genres_failed, 1);
        let recorded = dao.failed_tasks();
        assert_eq!(recorded.len(), 1);
    }

    // === reconcile_bullet_citations: edge case pins (ADR-832 followup) ===

    #[test]
    fn reconcile_returns_empty_when_bullet_has_no_marker() {
        let refs = vec![ref_with_id(1, "https://example.com/a", Some("a-1"))];
        let result = reconcile_bullet_citations(
            "プレーン文章で出典マーカーは無い",
            &refs,
            &url_map(&[]),
            &sid_map(&[("a-1", vec![10, 11])]),
        );
        assert!(result.is_empty(), "no marker → empty");
    }

    #[test]
    fn reconcile_ignores_marker_index_out_of_range() {
        let refs = vec![ref_with_id(1, "https://example.com/a", Some("a-1"))];
        // [5] は refs.len()==1 を超えるため無視
        let result = reconcile_bullet_citations(
            "本文 [5] の続き",
            &refs,
            &url_map(&[]),
            &sid_map(&[("a-1", vec![10])]),
        );
        assert!(result.is_empty(), "out-of-range marker silently skipped");
    }

    #[test]
    fn reconcile_treats_zero_index_as_invalid() {
        let refs = vec![ref_with_id(1, "https://example.com/a", Some("a-1"))];
        // [0] は 1-indexed なので invalid
        let result = reconcile_bullet_citations(
            "本文 [0]",
            &refs,
            &url_map(&[]),
            &sid_map(&[("a-1", vec![10])]),
        );
        assert!(result.is_empty(), "[0] must be treated as invalid");
    }

    #[test]
    fn reconcile_uses_article_id_when_present_on_reference() {
        let refs = vec![ref_with_id(1, "https://example.com/a", Some("a-1"))];
        let result = reconcile_bullet_citations(
            "事実 [1]",
            &refs,
            &url_map(&[("https://example.com/a", "a-1")]),
            &sid_map(&[("a-1", vec![10, 11])]),
        );
        assert_eq!(result, vec![10, 11]);
    }

    #[test]
    fn reconcile_falls_back_to_url_when_article_id_missing() {
        // article_id 欠落 → URL から article_id を解決
        let refs = vec![ref_with_id(1, "https://example.com/a", None)];
        let result = reconcile_bullet_citations(
            "事実 [1]",
            &refs,
            &url_map(&[("https://example.com/a", "a-1")]),
            &sid_map(&[("a-1", vec![20, 21])]),
        );
        assert_eq!(result, vec![20, 21]);
    }

    #[test]
    fn reconcile_skips_unresolvable_url_without_panic() {
        // article_id 欠落 + URL も map に無い → そのマーカーは捨てる
        let refs = vec![ref_with_id(1, "https://unknown.example/x", None)];
        let result = reconcile_bullet_citations(
            "事実 [1]",
            &refs,
            &url_map(&[("https://example.com/a", "a-1")]),
            &sid_map(&[("a-1", vec![10])]),
        );
        assert!(result.is_empty(), "unresolvable url must not panic");
    }

    #[test]
    fn reconcile_returns_empty_when_article_has_no_sentences() {
        let refs = vec![ref_with_id(1, "https://example.com/a", Some("a-1"))];
        // article_id は解決するが sentence map に entry 無し → empty
        let result = reconcile_bullet_citations(
            "事実 [1]",
            &refs,
            &url_map(&[]),
            &sid_map(&[("b-1", vec![99])]),
        );
        assert!(result.is_empty());
    }

    #[test]
    fn reconcile_dedupes_and_sorts_repeated_markers() {
        // ADR-890 followup: production の article_id は常に UUID。fixture も UUID で揃える。
        let aid_a = "1dce453b-e23d-4a32-9030-7e4529fad645";
        let aid_b = "5ab015b7-1060-4e79-910d-c3e19acbb5cc";
        let refs = vec![
            ref_with_id(1, "https://example.com/a", Some(aid_a)),
            ref_with_id(2, "https://example.com/b", Some(aid_b)),
        ];
        // 重複 [1] [1] [2] → dedup + sorted union
        let result = reconcile_bullet_citations(
            "foo [1] bar [1] baz [2]",
            &refs,
            &url_map(&[]),
            &sid_map(&[(aid_a, vec![30, 31]), (aid_b, vec![40])]),
        );
        assert_eq!(result, vec![30, 31, 40]);
    }

    #[test]
    fn reconcile_returns_empty_when_references_is_none() {
        // references が空 (LLM が出さなかったケース) では bullet 内 [n] は全て無視
        let refs: Vec<Reference> = Vec::new();
        let result = reconcile_bullet_citations(
            "事実 [1] と [2]",
            &refs,
            &url_map(&[("https://example.com/a", "a-1")]),
            &sid_map(&[("a-1", vec![10, 11])]),
        );
        assert!(result.is_empty(), "empty refs → no grounding");
    }

    // === ADR-890 followup: reconciler must not trust non-UUID article_id verbatim ===
    // production の LLM 出力で `article_id="dev.to"` (domain string) が観測されたため、
    // UUID-shape でない article_id は捨てて URL fallback に倒す。

    #[test]
    fn reconcile_falls_back_to_url_when_article_id_is_non_uuid_string() {
        // LLM が article_id をドメイン文字列で埋めた典型例。Some("dev.to") を信用して
        // article_to_sentence_ids.get("dev.to") を引いて空になる、という現バグを潰す。
        let refs = vec![ref_with_id(
            1,
            "https://dev.to/foo/bar",
            Some("dev.to"), // non-UUID
        )];
        let valid_uuid = "1dce453b-e23d-4a32-9030-7e4529fad645";
        let result = reconcile_bullet_citations(
            "事実 [1]",
            &refs,
            &url_map(&[("https://dev.to/foo/bar", valid_uuid)]),
            &sid_map(&[(valid_uuid, vec![100, 101])]),
        );
        assert_eq!(
            result,
            vec![100, 101],
            "non-UUID article_id must be discarded and URL fallback must succeed"
        );
    }

    #[test]
    fn reconcile_falls_back_to_url_when_uuid_article_id_does_not_resolve() {
        // UUID 形状だが article_to_sentence_ids にエントリ無し → URL fallback に倒す。
        let stale_uuid = "00000000-0000-4000-8000-000000000000";
        let live_uuid = "1dce453b-e23d-4a32-9030-7e4529fad645";
        let refs = vec![ref_with_id(
            1,
            "https://example.com/article-x",
            Some(stale_uuid),
        )];
        let result = reconcile_bullet_citations(
            "事実 [1]",
            &refs,
            &url_map(&[("https://example.com/article-x", live_uuid)]),
            &sid_map(&[(live_uuid, vec![200, 201])]),
        );
        assert_eq!(
            result,
            vec![200, 201],
            "valid-shape UUID that misses must fall through to URL match"
        );
    }

    #[test]
    fn reconcile_matches_by_host_when_ref_url_is_domain_only() {
        // ref.url が "dev.to" のような scheme/path 無しの純ドメイン。host 部一致で
        // 同ホストの article 群を全て拾う (multi-match 許容: ADR-890 §Decision §3)。
        let refs = vec![ref_with_id(1, "dev.to", None)];
        let uuid_a = "1dce453b-e23d-4a32-9030-7e4529fad645";
        let uuid_b = "5ab015b7-1060-4e79-910d-c3e19acbb5cc";
        let result = reconcile_bullet_citations(
            "事実 [1]",
            &refs,
            &url_map(&[
                ("https://dev.to/foo", uuid_a),
                ("https://dev.to/bar", uuid_b),
                ("https://other.example.com/x", "x-1"),
            ]),
            &sid_map(&[(uuid_a, vec![300]), (uuid_b, vec![301, 302])]),
        );
        assert_eq!(
            result,
            vec![300, 301, 302],
            "domain-only ref.url must host-match all same-host articles"
        );
    }

    #[test]
    fn reconcile_merges_uuid_article_id_and_url_match_non_exclusively() {
        // article_id が有効 UUID で sentence にヒットしても、URL からも合流できる場合は
        // BTreeSet に両方放り込んで union を返す (排他にしない)。
        let uuid_a = "1dce453b-e23d-4a32-9030-7e4529fad645";
        let uuid_b = "5ab015b7-1060-4e79-910d-c3e19acbb5cc";
        let refs = vec![ref_with_id(1, "https://example.com/dual", Some(uuid_a))];
        let result = reconcile_bullet_citations(
            "事実 [1]",
            &refs,
            &url_map(&[("https://example.com/dual", uuid_b)]),
            &sid_map(&[(uuid_a, vec![10]), (uuid_b, vec![20, 21])]),
        );
        assert_eq!(
            result,
            vec![10, 20, 21],
            "valid article_id and matching URL must merge (union), not be exclusive"
        );
    }

    // === Integration: persist が source_sentence_ids を埋めて upsert する ===

    #[tokio::test]
    async fn persist_writes_non_empty_source_sentence_ids_when_references_resolve() {
        use uuid::Uuid;

        let job_id = Uuid::new_v4();
        let genre = "consumer_tech".to_string();
        let run_id: i64 = 4242;
        let article_id = "a-1".to_string();
        let summary_id = Uuid::new_v4().to_string();

        let dao = Arc::new(MockRecapDao::new());
        // article metadata: url + article_id 紐付け
        dao.set_article_metadata({
            let mut m = HashMap::new();
            m.insert(
                article_id.clone(),
                (
                    Some(chrono::Utc::now()),
                    Some("https://example.com/a-1".to_string()),
                ),
            );
            m
        });
        // sentence_id store: run_id 4242 で a-1 → [10, 11]
        dao.set_sentence_ids(run_id, {
            let mut m = HashMap::new();
            m.insert(article_id.clone(), vec![10_i64, 11_i64]);
            m
        });

        // SummaryResponse: bullet が [1] を 1 つ含み、references[0].article_id = "a-1"
        // SummaryMetadata は private field を含むため serde 経由で構築。
        let summary_response: SummaryResponse = serde_json::from_value(serde_json::json!({
            "job_id": job_id,
            "genre": genre,
            "summary": {
                "title": "テストタイトル",
                "bullets": ["事実 [1]"],
                "language": "ja",
                "references": [{
                    "id": 1,
                    "url": "https://example.com/a-1",
                    "domain": "example.com",
                    "article_id": article_id,
                }]
            },
            "metadata": { "model": "gemma-test" }
        }))
        .expect("summary_response fixture must deserialize");

        // ClusteringResponse: representative に article_id を含む。run_id を持つ。
        let clustering_response: ClusteringResponse = serde_json::from_value(serde_json::json!({
            "run_id": run_id,
            "job_id": job_id,
            "genre": genre,
            "status": "succeeded",
            "cluster_count": 1,
            "clusters": [{
                "cluster_id": 0,
                "size": 1,
                "representatives": [{
                    "article_id": article_id,
                    "sentence_text": "代表文",
                    "lang": "ja",
                    "score": 1.0
                }]
            }]
        }))
        .expect("clustering_response fixture must deserialize");

        let mut genre_results = HashMap::new();
        genre_results.insert(
            genre.clone(),
            GenreResult {
                genre: genre.clone(),
                clustering_response: Some(clustering_response),
                summary_response_id: Some(summary_id.clone()),
                summary_response: Some(summary_response),
                error: None,
                error_kind: None,
            },
        );
        let dispatch = DispatchResult {
            job_id,
            genre_results,
            success_count: 1,
            failure_count: 0,
            all_genres: vec![genre.clone()],
        };
        let job = JobContext::new(dispatch.job_id, dispatch.all_genres.clone());

        let stage = FinalSectionPersistStage::new(dao.clone(), None);
        let result = stage.persist(&job, dispatch).await.expect("persist ok");
        assert_eq!(result.genres_stored, 1);

        let outputs = dao.outputs();
        assert_eq!(outputs.len(), 1, "expected 1 RecapOutput upserted");
        let output = &outputs[0];
        let bullets = output
            .bullets_ja
            .as_array()
            .expect("bullets_ja must be JSON array");
        assert_eq!(bullets.len(), 1);
        let sids = bullets[0]
            .get("source_sentence_ids")
            .and_then(|v| v.as_array())
            .expect("source_sentence_ids must be present and an array");
        let parsed: Vec<i64> = sids.iter().filter_map(serde_json::Value::as_i64).collect();
        assert_eq!(
            parsed,
            vec![10_i64, 11_i64],
            "bullet [1] must be grounded to a-1 sentences"
        );
    }

    /// Build a minimal DispatchResult with a single genre whose clustering
    /// response carries one cluster with non-empty `top_terms` — the shape the
    /// persist stage forwards to the `recap.topic_snapshotted.v1` publish pass.
    fn dispatch_with_terms(genre: &str) -> DispatchResult {
        let job_id = uuid::Uuid::new_v4();
        let clustering_response: ClusteringResponse = serde_json::from_value(serde_json::json!({
            "run_id": 7_i64,
            "job_id": job_id.to_string(),
            "genre": genre,
            "status": "succeeded",
            "cluster_count": 1,
            "clusters": [{
                "cluster_id": 7,
                "size": 1,
                "top_terms": ["ai", "llm"],
                "representatives": []
            }]
        }))
        .expect("clustering_response fixture must deserialize");

        let mut genre_results = HashMap::new();
        genre_results.insert(
            genre.to_string(),
            GenreResult {
                genre: genre.to_string(),
                clustering_response: Some(clustering_response),
                summary_response_id: None,
                // No summary_response: the persist loop classifies this as
                // "no evidence" (genres_skipped), which keeps the genre out of
                // the DB write path while the publish pass below still sees a
                // confirmed cluster (error.is_none() + non-empty top_terms).
                summary_response: None,
                error: None,
                error_kind: None,
            },
        );
        DispatchResult {
            job_id,
            genre_results,
            success_count: 0,
            failure_count: 0,
            all_genres: vec![genre.to_string()],
        }
    }

    /// RED→GREEN end-to-end: a JobContext carrying an owner (user_id +
    /// tenant_id) drives the persist stage to actually emit
    /// `recap.topic_snapshotted.v1`. Before PR-7 the owner was never wired, so
    /// the guard `(Some, Some, _)` was unreachable and zero events were emitted
    /// in production. The mock pins exactly one AppendKnowledgeEvent RPC.
    #[tokio::test]
    async fn persist_emits_topic_snapshot_when_owner_present() {
        use wiremock::matchers::{method, path};
        use wiremock::{Mock, MockServer, ResponseTemplate};

        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path(
                "/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent",
            ))
            .respond_with(
                ResponseTemplate::new(200)
                    .insert_header("Content-Type", "application/json")
                    .set_body_string(r#"{"success":true,"eventSeq":1}"#),
            )
            .expect(1)
            .mount(&server)
            .await;

        let dao = Arc::new(MockRecapDao::new());
        let sovereign = Arc::new(KnowledgeSovereignClient::new_for_test(server.uri()));
        let stage = FinalSectionPersistStage::new(dao, None).with_sovereign(Some(sovereign));

        let dispatch = dispatch_with_terms("ai_data");
        let job = JobContext::new(dispatch.job_id, dispatch.all_genres.clone())
            .with_user_scope(uuid::Uuid::new_v4(), uuid::Uuid::new_v4());

        stage.persist(&job, dispatch).await.expect("persist ok");
        // `server` drop verifies `.expect(1)`: exactly one emit must have fired.
    }

    /// Companion: with NO owner on the JobContext (the production default
    /// before PR-7), the guard short-circuits and the persist stage emits
    /// nothing even though a sovereign client is wired and a cluster has terms.
    #[tokio::test]
    async fn persist_skips_topic_snapshot_when_owner_absent() {
        use wiremock::matchers::method;
        use wiremock::{Mock, MockServer, ResponseTemplate};

        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .respond_with(ResponseTemplate::new(500))
            .expect(0)
            .mount(&server)
            .await;

        let dao = Arc::new(MockRecapDao::new());
        let sovereign = Arc::new(KnowledgeSovereignClient::new_for_test(server.uri()));
        let stage = FinalSectionPersistStage::new(dao, None).with_sovereign(Some(sovereign));

        let dispatch = dispatch_with_terms("ai_data");
        let job = JobContext::new(dispatch.job_id, dispatch.all_genres.clone());

        stage.persist(&job, dispatch).await.expect("persist ok");
        // `server` drop verifies `.expect(0)`: no emit when owner is unset.
    }
}
