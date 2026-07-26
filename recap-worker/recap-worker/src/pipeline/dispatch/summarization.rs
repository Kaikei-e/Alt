//! Summarization operations for dispatch stage.

use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;

use tracing::{debug, error, info, warn};
use uuid::Uuid;

use crate::clients::NewsCreatorClient;
use crate::clients::news_creator::models::{
    BatchSummaryError, BatchSummaryResponse, SummaryOptions, SummaryRequest, SummaryResponse,
};
use crate::clients::subworker::ClusteringResponse;
use crate::config::Config;
use crate::error::{RecapError, Result};
use crate::scheduler::JobContext;
use crate::store::dao::RecapDao;
use crate::util::retry::RetryConfig;

use super::types::GenreResult;

/// Summarization operations helper.
pub(crate) struct SummarizationOps<'a> {
    pub(crate) news_creator_client: &'a Arc<NewsCreatorClient>,
    pub(crate) dao: &'a Arc<dyn RecapDao>,
    pub(crate) config: &'a Arc<Config>,
}

/// Per-chunk total attempts (first pass + deferred retries).
const BATCH_SUMMARY_CHUNK_RETRY: RetryConfig = RetryConfig::new(3, 500, 5_000);

/// Upper bound on how long a single deferred round waits, even when the
/// server's `Retry-After` asks for longer — guards against a misbehaving
/// upstream stalling the whole job on an oversized hint.
const MAX_DEFERRED_ROUND_WAIT: Duration = Duration::from_mins(2);

/// A batch-summary chunk that failed its most recent attempt and is queued
/// for a later deferred round.
struct DeferredChunk {
    chunk_idx: usize,
    chunk: Vec<SummaryRequest>,
    attempts: usize,
    retry_after: Option<Duration>,
}

/// How long to wait before the next deferred round: the larger of the
/// full-jitter computed backoff and the server's `Retry-After` hint (RFC
/// 6585 / MDN — the server's hint takes precedence over computed backoff
/// when present), capped at `MAX_DEFERRED_ROUND_WAIT` so an oversized
/// `Retry-After` can't stall the job indefinitely.
fn deferred_round_wait(round: usize, retry_after: Option<Duration>) -> Duration {
    let backoff = BATCH_SUMMARY_CHUNK_RETRY.delay_for_attempt(round);
    backoff
        .max(retry_after.unwrap_or_default())
        .min(MAX_DEFERRED_ROUND_WAIT)
}

/// Logs a chunk's genres as permanently degraded. The caller
/// (`process_batch_response`) still folds these genres into "Missing from
/// batch response" — this only surfaces *why*, loudly and by genre name.
fn log_permanent_chunk_failure(job_id: Uuid, task: &DeferredChunk, reason: &str) {
    let genres: Vec<&str> = task.chunk.iter().map(|r| r.genre.as_str()).collect();
    error!(
        job_id = %job_id,
        chunk_idx = task.chunk_idx,
        attempts = task.attempts,
        reason,
        ?genres,
        "batch summary chunk failed permanently, degrading genres to missing-from-batch"
    );
}

/// Runs every batch-summary chunk through a first pass (one attempt each,
/// draining the easy wins), then retries first-pass failures in deferred
/// rounds with full-jitter backoff between rounds. Draining successes
/// before spending retry budget on a stalled chunk gives a struggling
/// news-creator a chance to recover instead of being hammered with
/// immediate retries per chunk while healthy chunks wait behind it in the
/// queue (Google SRE: bounded per-request retries, don't hammer an
/// overloaded server).
///
/// If more than half of a deferred round's chunks fail again, the server is
/// treated as overloaded and every remaining deferred chunk is degraded
/// immediately instead of spending further rounds on it. A lone chunk
/// failing again does not trip this — "widespread failure" requires more
/// than one chunk in flight — so an isolated struggling chunk still gets
/// its full per-chunk attempt budget.
async fn run_batch_summary_chunks(
    news_creator_client: &NewsCreatorClient,
    job_id: Uuid,
    chunks: Vec<Vec<SummaryRequest>>,
) -> (Vec<SummaryResponse>, Vec<BatchSummaryError>) {
    let mut all_responses: Vec<SummaryResponse> = Vec::new();
    let mut all_errors: Vec<BatchSummaryError> = Vec::new();
    let mut deferred: Vec<DeferredChunk> = Vec::new();

    for (chunk_idx, chunk) in chunks.into_iter().enumerate() {
        info!(
            job_id = %job_id,
            chunk_idx,
            chunk_size = chunk.len(),
            "processing batch summary chunk (first pass)"
        );
        match news_creator_client
            .generate_batch_summary(chunk.clone())
            .await
        {
            Ok(response) => {
                all_responses.extend(response.responses);
                all_errors.extend(response.errors);
            }
            Err(e) => {
                warn!(
                    job_id = %job_id,
                    chunk_idx,
                    error = ?e,
                    "batch summary chunk failed on first pass; deferring retry until every chunk has had its first attempt"
                );
                deferred.push(DeferredChunk {
                    chunk_idx,
                    chunk,
                    attempts: 1,
                    retry_after: e.retry_after(),
                });
            }
        }
    }

    let mut round = 1usize;
    while !deferred.is_empty() {
        let retry_after_wait = deferred.iter().filter_map(|c| c.retry_after).max();
        let wait = deferred_round_wait(round, retry_after_wait);
        info!(
            job_id = %job_id,
            round,
            deferred_count = deferred.len(),
            wait_ms = wait.as_millis(),
            "waiting before deferred batch summary retry round"
        );
        tokio::time::sleep(wait).await;

        let round_size = deferred.len();
        let mut still_pending: Vec<DeferredChunk> = Vec::new();
        let mut failures_this_round = 0usize;

        for mut task in deferred.drain(..) {
            match news_creator_client
                .generate_batch_summary(task.chunk.clone())
                .await
            {
                Ok(response) => {
                    all_responses.extend(response.responses);
                    all_errors.extend(response.errors);
                }
                Err(e) => {
                    failures_this_round += 1;
                    task.attempts += 1;
                    task.retry_after = e.retry_after();
                    if BATCH_SUMMARY_CHUNK_RETRY.can_retry(task.attempts) {
                        still_pending.push(task);
                    } else {
                        log_permanent_chunk_failure(job_id, &task, "exhausted all attempts");
                    }
                }
            }
        }

        // Overload circuit (Google SRE: widespread failure -> stop
        // retrying, don't hammer). Only meaningful with more than one
        // chunk in flight this round.
        if round_size > 1 && failures_this_round * 2 > round_size {
            let genres: Vec<&str> = still_pending
                .iter()
                .flat_map(|t| t.chunk.iter().map(|r| r.genre.as_str()))
                .collect();
            error!(
                job_id = %job_id,
                round,
                failures_this_round,
                round_size,
                ?genres,
                "batch summary overload detected: more than half of this deferred round failed again; stopping retries and degrading remaining genres immediately"
            );
            for task in &still_pending {
                log_permanent_chunk_failure(
                    job_id,
                    task,
                    "overload circuit: retries stopped early",
                );
            }
            return (all_responses, all_errors);
        }

        deferred = still_pending;
        round += 1;
    }

    (all_responses, all_errors)
}

fn apply_recap_request_defaults(
    summary_request: &mut SummaryRequest,
    window_days: u32,
    temperature: f64,
) {
    summary_request.options = Some(SummaryOptions {
        max_bullets: None,
        temperature: Some(temperature),
    });
    summary_request.window_days = Some(window_days);
}

impl SummarizationOps<'_> {
    /// クラスタリングエラー時の結果を構築する。
    pub(crate) fn clustering_error_result(genre: &str, e: RecapError) -> GenreResult {
        warn!(
            genre = %genre,
            error = ?e,
            "clustering failed"
        );
        GenreResult {
            genre: genre.to_string(),
            clustering_response: None,
            summary_response_id: None,
            summary_response: None,
            error: Some(format!("Clustering failed: {}", e)),
            error_kind: Some(e),
        }
    }

    /// 要約生成結果からGenreResultを構築する。
    /// 後方互換性のため保持（バッチ API ではインライン構築）。
    #[allow(dead_code)]
    pub(crate) fn build_genre_result(
        genre: &str,
        clustering_response: ClusteringResponse,
        summary_result: Result<SummaryResponse>,
    ) -> GenreResult {
        match summary_result {
            Ok(summary_response) => {
                info!(
                    genre = %genre,
                    bullet_count = summary_response.summary.bullets.len(),
                    "summary generation completed successfully"
                );
                let summary_id = format!("{}-{}", summary_response.job_id, summary_response.genre);
                GenreResult {
                    genre: genre.to_string(),
                    clustering_response: Some(clustering_response),
                    summary_response_id: Some(summary_id),
                    summary_response: Some(summary_response),
                    error: None,
                    error_kind: None,
                }
            }
            Err(e) => {
                warn!(
                    genre = %genre,
                    error = ?e,
                    "summary generation failed"
                );
                GenreResult {
                    genre: genre.to_string(),
                    clustering_response: Some(clustering_response),
                    summary_response_id: None,
                    summary_response: None,
                    error: Some(format!("Summary generation failed: {}", e)),
                    error_kind: Some(e),
                }
            }
        }
    }

    /// メタデータを取得して要約リクエストを構築し、要約を生成する。
    /// 後方互換性のため保持（バッチ API への移行後は未使用）。
    #[allow(dead_code)]
    pub(crate) async fn generate_summary_with_metadata(
        &self,
        job_id: Uuid,
        window_days: u32,
        genre: &str,
        clustering_response: &ClusteringResponse,
    ) -> Result<SummaryResponse> {
        // 記事IDのリストを収集
        let article_ids: Vec<String> = clustering_response
            .clusters
            .iter()
            .flat_map(|cluster| {
                cluster
                    .representatives
                    .iter()
                    .map(|rep| rep.article_id.clone())
            })
            .collect::<std::collections::HashSet<_>>()
            .into_iter()
            .collect();

        // メタデータを取得
        let article_metadata = match self.dao.get_article_metadata(job_id, &article_ids).await {
            Ok(metadata) => metadata,
            Err(e) => {
                warn!(
                    job_id = %job_id,
                    genre = %genre,
                    error = ?e,
                    "failed to fetch article metadata, proceeding without metadata"
                );
                std::collections::HashMap::new()
            }
        };

        let news_creator_client = self.news_creator_client.clone();
        let clustering_response_clone = clustering_response.clone();

        // Step 1: Build Summary Request (Budget Allocation & Sentence Selection)
        // This runs on a blocking thread because token counting is CPU-bound
        let mut summary_request = tokio::task::spawn_blocking(move || {
            news_creator_client.build_summary_request(
                job_id,
                &clustering_response_clone,
                8, // Plan 9: "5-8 sentences" for actual summary input
                &article_metadata,
            )
        })
        .await
        .map_err(|e| {
            RecapError::Summary(format!("failed to join build_summary_request task: {e}"))
        })?;

        // Step 2: Direct Summarization (Single Shot)
        // Skip Map phase (intermediate summarization) and directly generate final summary
        // from the selected sentences.
        info!(
            job_id = %job_id,
            genre = %genre,
            cluster_count = summary_request.clusters.len(),
            "starting single-shot summarization (skipping Map phase)"
        );

        // Enforce strict output format options
        apply_recap_request_defaults(
            &mut summary_request,
            window_days,
            self.config.recap_summary_temperature(),
        );

        self.news_creator_client
            .generate_summary(&summary_request)
            .await
    }

    /// メタデータを取得して SummaryRequest を構築する（HTTP 呼び出しなし）。
    /// バッチ処理用にリクエスト構築のみを行う。
    pub(crate) async fn build_summary_request_for_batch(
        &self,
        job_id: Uuid,
        window_days: u32,
        genre: &str,
        clustering_response: &ClusteringResponse,
    ) -> Result<SummaryRequest> {
        // 記事IDのリストを収集
        let article_ids: Vec<String> = clustering_response
            .clusters
            .iter()
            .flat_map(|cluster| {
                cluster
                    .representatives
                    .iter()
                    .map(|rep| rep.article_id.clone())
            })
            .collect::<std::collections::HashSet<_>>()
            .into_iter()
            .collect();

        // メタデータを取得
        let article_metadata = match self.dao.get_article_metadata(job_id, &article_ids).await {
            Ok(metadata) => metadata,
            Err(e) => {
                warn!(
                    job_id = %job_id,
                    genre = %genre,
                    error = ?e,
                    "failed to fetch article metadata, proceeding without metadata"
                );
                std::collections::HashMap::new()
            }
        };

        let news_creator_client = self.news_creator_client.clone();
        let clustering_response_clone = clustering_response.clone();

        // SummaryRequest 構築（CPU集約的なためブロッキングタスクで実行）
        let mut summary_request = tokio::task::spawn_blocking(move || {
            news_creator_client.build_summary_request(
                job_id,
                &clustering_response_clone,
                8, // Plan 9: "5-8 sentences" for actual summary input
                &article_metadata,
            )
        })
        .await
        .map_err(|e| {
            RecapError::Summary(format!("failed to join build_summary_request task: {e}"))
        })?;

        // 出力フォーマットオプションを設定
        apply_recap_request_defaults(
            &mut summary_request,
            window_days,
            self.config.recap_summary_temperature(),
        );

        Ok(summary_request)
    }

    /// 要約メトリクスを保存するヘルパー。
    pub(crate) async fn save_summary_metrics(
        &self,
        job_id: Uuid,
        genre: &str,
        response: &SummaryResponse,
    ) {
        match serde_json::to_value(&response.metadata) {
            Ok(metadata_value) => {
                if let Err(e) = self
                    .dao
                    .save_system_metrics(job_id, "summarization", &metadata_value)
                    .await
                {
                    warn!(
                        job_id = %job_id,
                        genre = %genre,
                        error = ?e,
                        "failed to save summarization metrics"
                    );
                }
            }
            Err(e) => {
                warn!(
                    job_id = %job_id,
                    genre = %genre,
                    error = ?e,
                    "failed to serialize summary metadata for metrics"
                );
            }
        }
    }

    /// Phase 2: サマリー生成をバッチ API で実行（1回の HTTP 呼び出しで全ジャンル処理）
    #[allow(clippy::too_many_lines)]
    pub(crate) async fn generate_summaries_with_batch(
        &self,
        job: &JobContext,
        clustering_results: HashMap<String, Result<ClusteringResponse>>,
    ) -> HashMap<String, GenreResult> {
        let total_genres = clustering_results.len();
        info!(
            job_id = %job.job_id,
            genre_count = total_genres,
            alt.processing.stage = "dispatch",
            alt.processing.phase = "summarization",
            alt.processing.progress.total = total_genres,
            "starting batch summary generation"
        );

        // 1. クラスタリング成功/失敗を分離
        let mut successful: Vec<(String, ClusteringResponse)> = Vec::new();
        let mut genre_results: HashMap<String, GenreResult> = HashMap::new();

        for (genre, result) in clustering_results {
            match result {
                Ok(clustering_response) => {
                    successful.push((genre, clustering_response));
                }
                Err(e) => {
                    warn!(
                        job_id = %job.job_id,
                        genre = %genre,
                        error = ?e,
                        "skipping summary generation due to clustering failure"
                    );
                    genre_results.insert(genre.clone(), Self::clustering_error_result(&genre, e));
                }
            }
        }

        if successful.is_empty() {
            info!(
                job_id = %job.job_id,
                "no successful clustering results, skipping batch summary generation"
            );
            return genre_results;
        }

        // 2. 全リクエストを並列構築
        info!(
            job_id = %job.job_id,
            genre_count = successful.len(),
            "building summary requests for batch"
        );

        let request_futures: Vec<_> = successful
            .iter()
            .map(|(genre, clustering_response)| {
                self.build_summary_request_for_batch(
                    job.job_id,
                    job.window_days(),
                    genre,
                    clustering_response,
                )
            })
            .collect();

        let request_results = futures::future::join_all(request_futures).await;

        // 3. 有効なリクエストを収集し、失敗したものはエラーとして記録
        let mut valid_requests: Vec<SummaryRequest> = Vec::new();
        let mut genre_clustering_map: HashMap<String, ClusteringResponse> = HashMap::new();

        for ((genre, clustering_response), req_result) in
            successful.into_iter().zip(request_results)
        {
            match req_result {
                Ok(mut request) => {
                    // Filter out clusters with empty representative_sentences
                    // This prevents 422 errors from the batch summary API
                    let original_cluster_count = request.clusters.len();
                    request
                        .clusters
                        .retain(|cluster| !cluster.representative_sentences.is_empty());

                    let filtered_count = original_cluster_count - request.clusters.len();
                    if filtered_count > 0 {
                        debug!(
                            job_id = %job.job_id,
                            genre = %genre,
                            filtered_count = filtered_count,
                            remaining_count = request.clusters.len(),
                            "filtered out clusters with empty representative_sentences"
                        );
                    }

                    // If all clusters were filtered out, treat as error
                    if request.clusters.is_empty() {
                        warn!(
                            job_id = %job.job_id,
                            genre = %genre,
                            original_cluster_count = original_cluster_count,
                            "all clusters had empty representative_sentences"
                        );
                        genre_results.insert(
                            genre.clone(),
                            GenreResult {
                                genre,
                                clustering_response: Some(clustering_response),
                                summary_response_id: None,
                                summary_response: None,
                                error: Some(
                                    "All clusters had empty representative_sentences".to_string(),
                                ),
                                error_kind: None,
                            },
                        );
                        continue;
                    }

                    genre_clustering_map.insert(genre, clustering_response);
                    valid_requests.push(request);
                }
                Err(e) => {
                    warn!(
                        job_id = %job.job_id,
                        genre = %genre,
                        error = ?e,
                        "failed to build summary request"
                    );
                    genre_results.insert(
                        genre.clone(),
                        GenreResult {
                            genre,
                            clustering_response: Some(clustering_response),
                            summary_response_id: None,
                            summary_response: None,
                            error: Some(format!("Failed to build request: {}", e)),
                            error_kind: None,
                        },
                    );
                }
            }
        }

        if valid_requests.is_empty() {
            info!(
                job_id = %job.job_id,
                "no valid summary requests, skipping batch API call"
            );
            return genre_results;
        }

        // 4. バッチ API 呼び出し（設定可能なチャンクサイズで分割）
        let batch_summary_chunk_size = self.config.batch_summary_chunk_size();
        let request_count = valid_requests.len();
        let chunks: Vec<Vec<SummaryRequest>> = valid_requests
            .chunks(batch_summary_chunk_size)
            .map(<[SummaryRequest]>::to_vec)
            .collect();

        info!(
            job_id = %job.job_id,
            request_count,
            chunk_count = chunks.len(),
            "calling batch summary API in chunks"
        );

        let (all_responses, all_errors) =
            run_batch_summary_chunks(self.news_creator_client, job.job_id, chunks).await;

        let batch_response = BatchSummaryResponse {
            responses: all_responses,
            errors: all_errors,
        };

        // 5. レスポンスをマッピング
        self.process_batch_response(
            job,
            Ok(batch_response),
            genre_clustering_map,
            &mut genre_results,
        )
        .await;

        info!(
            job_id = %job.job_id,
            completed_count = genre_results.len(),
            alt.processing.stage = "dispatch",
            alt.processing.phase = "summarization",
            alt.processing.progress.current = genre_results.len(),
            alt.processing.progress.total = total_genres,
            alt.processing.status = "completed",
            "completed batch summary generation phase"
        );

        genre_results
    }

    /// バッチサマリーレスポンスを処理し、ジャンル結果を更新する。
    async fn process_batch_response(
        &self,
        job: &JobContext,
        batch_result: Result<BatchSummaryResponse>,
        mut genre_clustering_map: HashMap<String, ClusteringResponse>,
        genre_results: &mut HashMap<String, GenreResult>,
    ) {
        match batch_result {
            Ok(response) => {
                info!(
                    job_id = %job.job_id,
                    success_count = response.responses.len(),
                    error_count = response.errors.len(),
                    "batch summary API completed"
                );

                // 成功したレスポンスを処理
                for summary_response in response.responses {
                    let genre = summary_response.genre.clone();
                    if let Some(clustering_response) = genre_clustering_map.remove(&genre) {
                        self.save_summary_metrics(job.job_id, &genre, &summary_response)
                            .await;

                        let summary_id =
                            format!("{}-{}", summary_response.job_id, summary_response.genre);
                        genre_results.insert(
                            genre.clone(),
                            GenreResult {
                                genre,
                                clustering_response: Some(clustering_response),
                                summary_response_id: Some(summary_id),
                                summary_response: Some(summary_response),
                                error: None,
                                error_kind: None,
                            },
                        );
                    }
                }

                // エラーを処理
                for error in response.errors {
                    let genre = error.genre.clone();
                    if let Some(clustering_response) = genre_clustering_map.remove(&genre) {
                        warn!(
                            job_id = %job.job_id,
                            genre = %genre,
                            error = %error.error,
                            "batch summary failed for genre"
                        );
                        genre_results.insert(
                            genre.clone(),
                            GenreResult {
                                genre,
                                clustering_response: Some(clustering_response),
                                summary_response_id: None,
                                summary_response: None,
                                error: Some(error.error),
                                error_kind: None,
                            },
                        );
                    }
                }

                // 残ったジャンル（レスポンスもエラーもない）を処理
                for (genre, clustering_response) in genre_clustering_map {
                    warn!(
                        job_id = %job.job_id,
                        genre = %genre,
                        "genre missing from batch response"
                    );
                    genre_results.insert(
                        genre.clone(),
                        GenreResult {
                            genre,
                            clustering_response: Some(clustering_response),
                            summary_response_id: None,
                            summary_response: None,
                            error: Some("Missing from batch response".to_string()),
                            error_kind: None,
                        },
                    );
                }
            }
            Err(e) => {
                // バッチ全体が失敗した場合
                error!(
                    job_id = %job.job_id,
                    error = ?e,
                    "batch summary API failed completely"
                );
                for (genre, clustering_response) in genre_clustering_map {
                    genre_results.insert(
                        genre.clone(),
                        GenreResult {
                            genre,
                            clustering_response: Some(clustering_response),
                            summary_response_id: None,
                            summary_response: None,
                            error: Some(format!("Batch API failed: {}", e)),
                            error_kind: None,
                        },
                    );
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Arc;
    use std::sync::atomic::{AtomicUsize, Ordering};
    use uuid::Uuid;
    use wiremock::matchers::{body_string_contains, method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    fn single_request(genre: &str) -> SummaryRequest {
        SummaryRequest {
            job_id: Uuid::new_v4(),
            genre: genre.to_string(),
            clusters: vec![],
            genre_highlights: None,
            options: None,
            window_days: None,
        }
    }

    fn success_body(genre: &str) -> serde_json::Value {
        serde_json::json!({
            "responses": [{
                "job_id": Uuid::new_v4(),
                "genre": genre,
                "summary": { "title": "t", "bullets": ["b"], "language": "ja" },
                "metadata": { "model": "gemma4-e4b-q4km" }
            }],
            "errors": []
        })
    }

    /// RED→GREEN regression: the batch-summary chunk loop must attempt
    /// every chunk exactly once in the first pass (draining the easy wins)
    /// before spending any retry budget on a chunk that failed, instead of
    /// hammering the failing chunk with immediate retries while healthy
    /// chunks behind it in the queue wait. Pins the observable request
    /// order: chunk_a's retry must land strictly after chunk_b and
    /// chunk_c's first-pass attempts.
    #[tokio::test]
    async fn run_batch_summary_chunks_retries_failed_chunk_after_other_chunks_first_pass() {
        let server = MockServer::start().await;
        let chunk_a_attempts = Arc::new(AtomicUsize::new(0));
        let attempts = chunk_a_attempts.clone();

        Mock::given(method("POST"))
            .and(path("/v1/summary/generate/batch"))
            .and(body_string_contains("\"genre\":\"chunk_a\""))
            .respond_with(move |_req: &wiremock::Request| {
                if attempts.fetch_add(1, Ordering::SeqCst) == 0 {
                    ResponseTemplate::new(503)
                } else {
                    ResponseTemplate::new(200).set_body_json(success_body("chunk_a"))
                }
            })
            .mount(&server)
            .await;
        Mock::given(method("POST"))
            .and(path("/v1/summary/generate/batch"))
            .and(body_string_contains("\"genre\":\"chunk_b\""))
            .respond_with(ResponseTemplate::new(200).set_body_json(success_body("chunk_b")))
            .mount(&server)
            .await;
        Mock::given(method("POST"))
            .and(path("/v1/summary/generate/batch"))
            .and(body_string_contains("\"genre\":\"chunk_c\""))
            .respond_with(ResponseTemplate::new(200).set_body_json(success_body("chunk_c")))
            .mount(&server)
            .await;

        let client = NewsCreatorClient::new_for_test(server.uri());
        let job_id = Uuid::new_v4();
        let chunks = vec![
            vec![single_request("chunk_a")],
            vec![single_request("chunk_b")],
            vec![single_request("chunk_c")],
        ];

        let (responses, errors) = run_batch_summary_chunks(&client, job_id, chunks).await;

        assert!(
            errors.is_empty(),
            "no per-genre errors expected: {errors:?}"
        );
        let genres: std::collections::BTreeSet<&str> =
            responses.iter().map(|r| r.genre.as_str()).collect();
        assert_eq!(
            genres,
            std::collections::BTreeSet::from(["chunk_a", "chunk_b", "chunk_c"]),
            "every genre must eventually succeed once the deferred retry runs"
        );

        let requests = server
            .received_requests()
            .await
            .expect("request recording enabled by default");
        let sequence: Vec<&str> = requests
            .iter()
            .map(|r| {
                let body = String::from_utf8_lossy(&r.body).into_owned();
                if body.contains("chunk_a") {
                    "a"
                } else if body.contains("chunk_b") {
                    "b"
                } else {
                    "c"
                }
            })
            .collect();
        assert_eq!(
            sequence,
            vec!["a", "b", "c", "a"],
            "chunk_a's retry must come after chunk_b/chunk_c's first-pass attempts, not immediately"
        );
    }

    /// RFC 6585 / MDN: when a chunk fails with 429 + `Retry-After`, the
    /// deferred-retry scheduler must wait at least that long before
    /// retrying — not just its own computed full-jitter backoff.
    #[tokio::test]
    async fn run_batch_summary_chunks_honors_retry_after_before_deferred_retry() {
        let server = MockServer::start().await;
        let attempts = Arc::new(AtomicUsize::new(0));
        let attempts_clone = attempts.clone();

        Mock::given(method("POST"))
            .and(path("/v1/summary/generate/batch"))
            .respond_with(move |_req: &wiremock::Request| {
                if attempts_clone.fetch_add(1, Ordering::SeqCst) == 0 {
                    ResponseTemplate::new(429).insert_header("Retry-After", "1")
                } else {
                    ResponseTemplate::new(200).set_body_json(success_body("chunk_a"))
                }
            })
            .mount(&server)
            .await;

        let client = NewsCreatorClient::new_for_test(server.uri());
        let job_id = Uuid::new_v4();
        let chunks = vec![vec![single_request("chunk_a")]];

        let start = std::time::Instant::now();
        let (responses, errors) = run_batch_summary_chunks(&client, job_id, chunks).await;
        let elapsed = start.elapsed();

        assert!(errors.is_empty());
        assert_eq!(responses.len(), 1);
        assert!(
            elapsed >= Duration::from_millis(950),
            "must wait at least the server's Retry-After (1s) before retrying, waited {elapsed:?}"
        );
    }

    /// SRE guidance: widespread failure = overload, stop retrying rather
    /// than hammering every chunk to its full attempt cap. With 4 chunks all
    /// returning 5xx, the first pass sends 4 requests and the single
    /// deferred round (100% failure, well over half) must trip the circuit —
    /// total requests must stay far below the 12 a naive 3-attempts-per-chunk
    /// hammer would send.
    #[tokio::test]
    async fn run_batch_summary_chunks_overload_circuit_stops_hammering_all_failing_chunks() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/v1/summary/generate/batch"))
            .respond_with(ResponseTemplate::new(503))
            .mount(&server)
            .await;

        let client = NewsCreatorClient::new_for_test(server.uri());
        let job_id = Uuid::new_v4();
        let chunks = vec![
            vec![single_request("g1")],
            vec![single_request("g2")],
            vec![single_request("g3")],
            vec![single_request("g4")],
        ];

        let (responses, errors) = run_batch_summary_chunks(&client, job_id, chunks).await;
        assert!(responses.is_empty());
        assert!(errors.is_empty());

        let requests = server
            .received_requests()
            .await
            .expect("request recording enabled by default");
        assert_eq!(
            requests.len(),
            8,
            "first pass (4) + one deferred round (4) = 8; the overload circuit must stop \
             before a second deferred round, well short of the 12 a 3-attempt-per-chunk hammer would send"
        );
    }

    /// Companion: a single struggling chunk is not "widespread failure" — it
    /// must still get its full per-chunk attempt budget (3) instead of
    /// being cut short by the overload circuit, which only makes sense with
    /// more than one chunk in flight.
    #[tokio::test]
    async fn run_batch_summary_chunks_single_failing_chunk_gets_full_attempt_budget() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/v1/summary/generate/batch"))
            .respond_with(ResponseTemplate::new(503))
            .expect(3)
            .mount(&server)
            .await;

        let client = NewsCreatorClient::new_for_test(server.uri());
        let job_id = Uuid::new_v4();
        let chunks = vec![vec![single_request("g1")]];

        let (responses, errors) = run_batch_summary_chunks(&client, job_id, chunks).await;
        assert!(responses.is_empty());
        assert!(errors.is_empty());
        server.verify().await;
    }

    #[test]
    fn deferred_round_wait_prefers_retry_after_when_larger_than_backoff() {
        let wait = deferred_round_wait(1, Some(Duration::from_secs(90)));
        assert_eq!(wait, Duration::from_secs(90));
    }

    #[test]
    fn deferred_round_wait_caps_at_max_deferred_round_wait() {
        let wait = deferred_round_wait(1, Some(Duration::from_mins(10)));
        assert_eq!(wait, MAX_DEFERRED_ROUND_WAIT);
    }

    #[test]
    fn deferred_round_wait_falls_back_to_computed_backoff_without_retry_after() {
        let wait = deferred_round_wait(1, None);
        assert!(wait <= Duration::from_millis(BATCH_SUMMARY_CHUNK_RETRY.base_delay_ms));
    }

    #[test]
    fn apply_request_defaults_threads_3day_window_and_temperature() {
        let mut request = SummaryRequest {
            job_id: Uuid::new_v4(),
            genre: "ai".to_string(),
            clusters: Vec::new(),
            genre_highlights: None,
            options: None,
            window_days: None,
        };

        apply_recap_request_defaults(&mut request, 3, 0.0);

        let options = request.options.expect("options should be set");
        assert_eq!(request.window_days, Some(3));
        assert_eq!(options.temperature, Some(0.0));
        assert_eq!(options.max_bullets, None);
    }

    #[test]
    fn apply_request_defaults_threads_7day_window_without_forcing_bullet_override() {
        let mut request = SummaryRequest {
            job_id: Uuid::new_v4(),
            genre: "tech".to_string(),
            clusters: Vec::new(),
            genre_highlights: None,
            options: Some(SummaryOptions {
                max_bullets: Some(15),
                temperature: Some(0.7),
            }),
            window_days: None,
        };

        apply_recap_request_defaults(&mut request, 7, 0.0);

        let options = request.options.expect("options should be set");
        assert_eq!(request.window_days, Some(7));
        assert_eq!(options.temperature, Some(0.0));
        assert_eq!(options.max_bullets, None);
    }

    /// A typed `RecapError` from the clustering chain must arrive in
    /// `GenreResult::error_kind` unchanged, so `pipeline::persist` can
    /// classify the genre outcome without inspecting message text.
    #[test]
    fn clustering_error_result_carries_typed_error_kind() {
        let err = RecapError::InsufficientDocuments { min: 2, found: 1 };

        let result = SummarizationOps::clustering_error_result("gaming", err.clone());

        assert_eq!(result.error_kind, Some(err));
        assert_eq!(
            result.error.as_deref(),
            Some(
                "Clustering failed: insufficient documents for clustering: expected >= 2, found 1"
            )
        );
    }

    /// A plain clustering failure keeps its variant too — persist must be
    /// able to distinguish it from the benign skip states.
    #[test]
    fn clustering_error_result_carries_clustering_variant() {
        let err = RecapError::Clustering("run 42 finished with status failed".to_string());

        let result = SummarizationOps::clustering_error_result("gaming", err.clone());

        assert_eq!(result.error_kind, Some(err));
    }
}
