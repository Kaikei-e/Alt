use std::collections::{BTreeSet, HashMap};
use std::sync::{Arc, LazyLock};

use anyhow::{Context, Result};
use async_trait::async_trait;
use regex::Regex;
use tracing::{debug, info, warn};

use crate::clients::news_creator::Reference;
use crate::clients::tag_generator::TagGeneratorClient;
use crate::scheduler::JobContext;
use crate::store::dao::RecapDao;
use crate::store::models::RecapOutput;

use super::dispatch::DispatchResult;
use crate::store::models::PersistedGenre;
use serde::{Deserialize, Serialize};
use serde_json::json;

/// `[1]` / `[42]` 形式の出典マーカーを抽出する。news-creator の REFERENCE_MARKER_RE と対称。
static REFERENCE_MARKER_RE: LazyLock<Regex> =
    LazyLock::new(|| Regex::new(r"\[(\d+)\]").expect("REFERENCE_MARKER_RE must compile"));

/// bullet 内の `[n]` を `references[n-1]` 経由で article_id に解決し、
/// `recap_subworker_sentences.id` の集合を返す純粋関数。
///
/// 解決順:
/// 1. `references[n-1].article_id` がある → そのまま採用 (singleton)
/// 2. 無ければ `references[n-1].url` を `url_to_article` に当てて解決 (multi-match 許容)
/// 3. それでも解決しない marker は無視 (`debug!` log)
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
        let mut article_ids: Vec<&String> = Vec::new();
        if let Some(aid) = r.article_id.as_ref() {
            article_ids.push(aid);
        } else {
            for (u, a) in url_to_article {
                if u == &r.url {
                    article_ids.push(a);
                }
            }
        }

        if article_ids.is_empty() {
            debug!(unmatched_ref_url = %r.url, "could not resolve reference url to article_id");
            continue;
        }

        for aid in article_ids {
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
#[allow(dead_code)]
pub(crate) struct FinalSectionPersistStage {
    dao: Arc<dyn RecapDao>,
    tag_generator: Option<Arc<TagGeneratorClient>>,
}

impl FinalSectionPersistStage {
    pub(crate) fn new(
        dao: Arc<dyn RecapDao>,
        tag_generator: Option<Arc<TagGeneratorClient>>,
    ) -> Self {
        Self { dao, tag_generator }
    }
}

#[async_trait]
#[allow(clippy::too_many_lines)]
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
            // エラーがある場合は分類
            if let Some(error_msg) = &genre_result.error {
                // エラーメッセージから分類
                if error_msg.contains("no evidence") || error_msg.contains("no articles assigned") {
                    // 記事が1件も割り当てられなかった
                    genres_no_evidence += 1;
                } else if error_msg.contains("insufficient documents")
                    || error_msg.contains("expected >=")
                {
                    // 証拠不足でスキップ（記事数が閾値未満）
                    genres_skipped += 1;
                } else {
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
                    genres_failed += 1;
                }
                continue;
            }

            // summary_responseがNoneの場合、データベースから再取得を試みる
            let summary_response = match (
                &genre_result.summary_response_id,
                &genre_result.summary_response,
            ) {
                (Some(_), Some(response)) => response.clone(),
                (Some(summary_id), None) => {
                    // リジューム時: データベースから再取得
                    match self.dao.get_recap_output_body_json(job.job_id, genre).await {
                        Ok(Some(body_json)) => {
                            match serde_json::from_value::<
                                crate::clients::news_creator::SummaryResponse,
                            >(body_json)
                            {
                                Ok(response) => {
                                    info!(
                                        job_id = %job.job_id,
                                        genre = %genre,
                                        "recovered summary_response from database"
                                    );
                                    response
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
                                    genres_failed += 1;
                                    continue;
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
                                &format!(
                                    "summary_response not found in database for id {summary_id}"
                                ),
                            )
                            .await;
                            genres_failed += 1;
                            continue;
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
                            genres_failed += 1;
                            continue;
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
                    genres_failed += 1;
                    continue;
                }
            };

            // Collect source articles for bullets
            let mut sources_metadata: Vec<serde_json::Value> = Vec::new();
            if let Some(clustering) = &genre_result.clustering_response {
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
            } else {
                // リジューム時: clustering_responseがNoneの場合
                // body_jsonからクラスタ情報を取得するか、データベースから直接取得
                // ここでは簡易的にbody_jsonから取得を試みる
                if let Ok(Some(body_json)) =
                    self.dao.get_recap_output_body_json(job.job_id, genre).await
                {
                    // body_jsonからクラスタ情報を抽出（構造に依存）
                    if let Some(clusters) = body_json.get("clusters").and_then(|c| c.as_array()) {
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
                    }
                }
            }

            // Sort sources by published_at desc
            sources_metadata.sort_by(|a, b| {
                let a_time = a
                    .as_object()
                    .and_then(|m| m.get("published_at"))
                    .and_then(|v| v.as_str());
                let b_time = b
                    .as_object()
                    .and_then(|m| m.get("published_at"))
                    .and_then(|v| v.as_str());
                b_time.cmp(&a_time)
            });

            // Limit sources to top 5
            let top_sources: Vec<serde_json::Value> =
                sources_metadata.into_iter().take(5).collect();

            let summary_id = genre_result
                .summary_response_id
                .as_ref()
                .expect("checked above")
                .clone();

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
            let bullets_json = serde_json::Value::Array(bullet_values);

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
                let tag_content = format!("{}\n{}", sanitized_summary, summary_bullets.join("\n"));
                match tg.extract_tags(genre.as_str(), &tag_content).await {
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
                genre.as_str(),
                summary_id.clone(),
                sanitized_title,
                sanitized_summary,
                bullets_json,
                body_json,
            )
            .with_tags(tags);

            let mut persisted_successfully = true;
            let mut write_errors: Vec<String> = Vec::new();

            if let Err(err) = self.dao.upsert_recap_output(&output).await {
                warn!(
                    job_id = %job.job_id,
                    genre = %genre,
                    error = ?err,
                    "failed to persist recap output"
                );
                write_errors.push(format!("upsert_recap_output failed: {err}"));
                persisted_successfully = false;
            }

            let persisted_genre =
                PersistedGenre::new(job.job_id, genre.as_str()).with_response_id(Some(summary_id));
            if let Err(err) = self.dao.upsert_genre(&persisted_genre).await {
                warn!(
                    job_id = %job.job_id,
                    genre = %genre,
                    error = ?err,
                    "failed to persist recap section pointer"
                );
                write_errors.push(format!("upsert_genre failed: {err}"));
                persisted_successfully = false;
            }

            if persisted_successfully {
                debug!(
                    job_id = %job.job_id,
                    genre = %genre,
                    "genre processed successfully"
                );
                genres_stored += 1;
            } else {
                record_failed_genre(
                    self.dao.as_ref(),
                    job.job_id,
                    "persist_write",
                    genre,
                    &write_errors.join("; "),
                )
                .await;
                genres_failed += 1;
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
        let mut genre_results = HashMap::new();
        genre_results.insert(
            genre.to_string(),
            GenreResult {
                genre: genre.to_string(),
                clustering_response: None,
                summary_response_id: None,
                summary_response: None,
                error: Some(error.to_string()),
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
        let dispatch = dispatch_with_error("consumer_tech", "no evidence for genre");
        let job = JobContext::new(dispatch.job_id, dispatch.all_genres.clone());

        stage.persist(&job, dispatch).await.expect("persist ok");

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
        let dispatch = dispatch_with_error("consumer_tech", "insufficient documents expected >= 3");
        let job = JobContext::new(dispatch.job_id, dispatch.all_genres.clone());

        stage.persist(&job, dispatch).await.expect("persist ok");

        let recorded = dao.failed_tasks();
        assert!(
            recorded.is_empty(),
            "insufficient documents is an expected skip, must not pollute recap_failed_tasks"
        );
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
        let refs = vec![
            ref_with_id(1, "https://example.com/a", Some("a-1")),
            ref_with_id(2, "https://example.com/b", Some("b-1")),
        ];
        // 重複 [1] [1] [2] → dedup + sorted union
        let result = reconcile_bullet_citations(
            "foo [1] bar [1] baz [2]",
            &refs,
            &url_map(&[]),
            &sid_map(&[("a-1", vec![30, 31]), ("b-1", vec![40])]),
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
}
