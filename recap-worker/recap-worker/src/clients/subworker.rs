use std::{
    cmp,
    collections::HashMap,
    fmt,
    sync::Arc,
    time::{Duration, Instant},
};

use anyhow::{Context, Result, anyhow};
use futures::stream::{self, StreamExt};
use reqwest::{Client, Response, Url};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use tokio::{sync::Semaphore, time::sleep};
use tracing::{debug, error, info, warn};
use uuid::Uuid;

use crate::pipeline::evidence::{ArticleFeatureSignal, CorpusMetadata, EvidenceCorpus};
use crate::schema::{subworker::CLUSTERING_RESPONSE_SCHEMA, validate_json};

#[derive(Debug, Clone)]
pub(crate) struct SubworkerClient {
    client: Client,
    base_url: Url,
    min_documents_per_genre: usize,
}

const DEFAULT_MAX_SENTENCES_TOTAL: usize = 2_000;
const DEFAULT_UMAP_N_COMPONENTS: usize = 25;
const DEFAULT_HDBSCAN_MIN_CLUSTER_SIZE: usize = 5;
const DEFAULT_MMR_LAMBDA: f32 = 0.35;
const MIN_PARAGRAPH_LEN: usize = 30;
const MAX_POLL_ATTEMPTS: usize = 200; // Covers ~100 minutes with exponential backoff (2s -> 30s)
const INITIAL_POLL_INTERVAL_MS: u64 = 2_000; // 2 seconds - start checking quickly
const MAX_POLL_INTERVAL_MS: u64 = 30_000; // 30 seconds - cap at 30s for long-running jobs
const SUBWORKER_TIMEOUT_SECS: u64 = 3600; // 1 hour to match server timeout and allow for large classification jobs
const MAX_ERROR_MESSAGE_LENGTH: usize = 500;
const EXTRACTION_TIMEOUT_SECS: u64 = 30; // 30 seconds for content extraction
const MIN_FALLBACK_DOCUMENTS: usize = 2;
const ADMIN_JOB_INITIAL_BACKOFF_MS: u64 = 5_000;
const ADMIN_JOB_MAX_BACKOFF_MS: u64 = 20_000;
const ADMIN_JOB_TIMEOUT_SECS: u64 = 600; // 10 minutes
const CLASSIFY_POST_RETRIES: usize = 3;
const CLASSIFY_POST_BACKOFF_MS: u64 = 5_000;
const CLASSIFY_CHUNK_SIZE: usize = 200; // Number of texts per chunk for parallel processing
const CLASSIFY_MAX_CONCURRENT: usize = 12; // Increased to 12 to match subworker's expanded 12-core capacity
const POLL_REQUEST_RETRIES: usize = 3; // Number of retries for polling requests
const POLL_REQUEST_RETRY_DELAY_MS: u64 = 1_000; // Retry delay between polling request retries (1 second)

/// エラーメッセージを要約して切り詰める。
fn truncate_error_message(msg: &str) -> String {
    let char_count = msg.chars().count();
    if char_count <= MAX_ERROR_MESSAGE_LENGTH {
        return msg.to_string();
    }
    let truncated: String = msg.chars().take(MAX_ERROR_MESSAGE_LENGTH).collect();
    format!("{truncated}... (truncated, {char_count} chars)")
}

/// バリデーションエラーのリストを要約する。
fn summarize_validation_errors(errors: &[String]) -> Vec<String> {
    errors.iter().map(|e| truncate_error_message(e)).collect()
}

/// クラスタリングレスポンス (POST/GET `/v1/runs`).
#[allow(dead_code)]
#[derive(Debug, Clone, Deserialize)]
pub(crate) struct ClusteringResponse {
    pub(crate) run_id: i64,
    pub(crate) job_id: Uuid,
    pub(crate) genre: String,
    pub(crate) status: ClusterJobStatus,
    #[serde(default)]
    pub(crate) cluster_count: usize,
    #[serde(default)]
    pub(crate) clusters: Vec<ClusterInfo>,
    #[serde(default)]
    pub(crate) genre_highlights: Option<Vec<ClusterRepresentative>>,
    #[serde(default)]
    pub(crate) diagnostics: Value,
}

impl ClusteringResponse {
    fn is_success(&self) -> bool {
        self.status.is_success()
    }
}

#[derive(Debug, Clone, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "lowercase")]
pub(crate) enum ClusterJobStatus {
    Running,
    Succeeded,
    Partial,
    Failed,
}

impl ClusterJobStatus {
    fn is_running(&self) -> bool {
        matches!(self, ClusterJobStatus::Running)
    }

    fn is_success(&self) -> bool {
        matches!(
            self,
            ClusterJobStatus::Succeeded | ClusterJobStatus::Partial
        )
    }

    fn is_terminal(&self) -> bool {
        !self.is_running()
    }
}

impl fmt::Display for ClusterJobStatus {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            ClusterJobStatus::Running => write!(f, "running"),
            ClusterJobStatus::Succeeded => write!(f, "succeeded"),
            ClusterJobStatus::Partial => write!(f, "partial"),
            ClusterJobStatus::Failed => write!(f, "failed"),
        }
    }
}

#[allow(dead_code)]
#[derive(Debug, Clone, Deserialize)]
pub(crate) struct ClusterInfo {
    pub(crate) cluster_id: i32,
    pub(crate) size: usize,
    #[serde(default)]
    pub(crate) label: Option<String>,
    #[serde(default)]
    pub(crate) top_terms: Vec<String>,
    #[serde(default)]
    pub(crate) stats: Value,
    #[serde(default)]
    pub(crate) representatives: Vec<ClusterRepresentative>,
}

#[allow(dead_code)]
#[derive(Debug, Clone, Deserialize)]
pub(crate) struct ClusterRepresentative {
    #[serde(default)]
    pub(crate) article_id: String,
    #[serde(default)]
    pub(crate) paragraph_idx: Option<i32>,
    #[serde(rename = "sentence_text")]
    pub(crate) text: String,
    #[serde(default)]
    pub(crate) lang: Option<String>,
    #[serde(default)]
    pub(crate) score: Option<f32>,
    #[serde(default)]
    pub(crate) reasons: Vec<String>,
}

#[derive(Debug, Clone, Serialize)]
struct ClusterJobRequest<'a> {
    params: ClusterJobParams,
    documents: Vec<ClusterDocument<'a>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    metadata: Option<&'a CorpusMetadata>,
}

#[derive(Debug, Clone, Serialize)]
struct ClusterJobParams {
    max_sentences_total: usize,
    max_sentences_per_cluster: usize,
    umap_n_components: usize,
    hdbscan_min_cluster_size: usize,
    mmr_lambda: f32,
}

#[derive(Debug, Clone, Serialize)]
struct ClusterDocument<'a> {
    article_id: &'a str,
    #[serde(skip_serializing_if = "Option::is_none")]
    title: Option<&'a String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    lang_hint: Option<&'a String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    published_at: Option<&'a String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    source_url: Option<&'a String>,
    paragraphs: Vec<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    genre_scores: Option<&'a HashMap<String, usize>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    confidence: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    signals: Option<&'a ArticleFeatureSignal>,
}

#[derive(Debug, Clone, Serialize)]
struct ClassificationRequest {
    texts: Vec<String>,
}

#[derive(Debug, Clone, Deserialize)]
pub(crate) struct ClassificationResult {
    pub(crate) top_genre: String,
    pub(crate) confidence: f32,
    pub(crate) scores: HashMap<String, f32>,
}

#[allow(dead_code)]
#[derive(Debug, Clone, Deserialize)]
struct ClassificationResponse {
    results: Vec<ClassificationResult>,
}

#[allow(dead_code)]
#[derive(Debug, Clone, Deserialize)]
struct ClassificationJobResponse {
    run_id: i64,
    job_id: String,
    status: String,
    result_count: usize,
    results: Option<Vec<ClassificationResult>>,
    error_message: Option<String>,
}

#[derive(Debug, Clone, Deserialize)]
struct AdminJobKickResponse {
    job_id: Uuid,
}

#[derive(Debug, Clone, Deserialize)]
#[allow(dead_code)]
struct AdminJobStatusResponse {
    job_id: Uuid,
    kind: String,
    status: String,
    #[serde(default)]
    result: Option<Value>,
    #[serde(default)]
    error: Option<String>,
}

#[derive(Debug, Clone, Serialize)]
struct ExtractRequest<'a> {
    html: &'a str,
    include_comments: bool,
}

#[derive(Debug, Clone, Deserialize)]
struct ExtractResponse {
    text: String,
}

#[derive(Debug, Clone, Serialize)]
struct CoarseClassifyRequest<'a> {
    text: &'a str,
}

#[derive(Debug, Clone, Deserialize)]
struct CoarseClassifyResponse {
    scores: HashMap<String, f32>,
}

#[derive(Debug, serde::Serialize)]
#[allow(dead_code)]
pub(crate) struct SubClusterOtherRequest {
    pub(crate) sentences: Vec<String>,
}

#[derive(Debug, serde::Deserialize)]
#[allow(dead_code)]
pub(crate) struct SubClusterOtherResponse {
    pub(crate) cluster_ids: Vec<i32>,
    pub(crate) labels: Option<Vec<i32>>,
    pub(crate) centers: Option<Vec<Vec<f32>>>,
}

impl SubworkerClient {
    pub(crate) fn new(endpoint: impl Into<String>, min_documents_per_genre: usize) -> Result<Self> {
        let client = Client::builder()
            .timeout(Duration::from_secs(SUBWORKER_TIMEOUT_SECS))
            .build()
            .context("failed to build subworker client")?;

        let base_url = Url::parse(&endpoint.into()).context("invalid subworker base URL")?;

        Ok(Self {
            client,
            base_url,
            min_documents_per_genre,
        })
    }

    pub(crate) async fn classify_texts(
        &self,
        job_id: Uuid,
        texts: Vec<String>,
    ) -> Result<Vec<ClassificationResult>> {
        let total_texts = texts.len();

        // If texts count is small, process in a single request
        if total_texts <= CLASSIFY_CHUNK_SIZE {
            return self.classify_chunk(job_id, texts, 0).await;
        }

        // Split into chunks for parallel processing
        let chunks: Vec<(usize, Vec<String>)> = texts
            .chunks(CLASSIFY_CHUNK_SIZE)
            .enumerate()
            .map(|(idx, chunk)| (idx, chunk.to_vec()))
            .collect();

        info!(
            job_id = %job_id,
            total_texts,
            chunk_count = chunks.len(),
            chunk_size = CLASSIFY_CHUNK_SIZE,
            max_concurrent = CLASSIFY_MAX_CONCURRENT,
            "splitting classification into parallel chunks"
        );

        // Use semaphore to limit concurrent requests
        let semaphore = Arc::new(Semaphore::new(CLASSIFY_MAX_CONCURRENT));
        let client = Arc::new(self.clone());

        // Process chunks in parallel using futures::stream
        // This allows completed chunks to immediately start the next waiting chunk
        let mut results: Vec<(usize, Result<Vec<ClassificationResult>>)> = stream::iter(chunks)
            .map(|(chunk_idx, chunk_texts)| {
                let semaphore = semaphore.clone();
                let client = client.clone();
                let job_id_copy = job_id; // Uuid is Copy, but clippy wants explicit copy

                async move {
                    // Acquire permit before processing (limits concurrency)
                    let _permit = semaphore.acquire().await;

                    // Process chunk
                    let result = client
                        .classify_chunk(job_id_copy, chunk_texts, chunk_idx)
                        .await;
                    (chunk_idx, result)
                }
            })
            .buffer_unordered(CLASSIFY_MAX_CONCURRENT) // Process up to 6 chunks concurrently
            .collect()
            .await;

        // Sort by chunk index to maintain original order
        results.sort_by_key(|(idx, _)| *idx);

        // Combine results in order
        let mut combined_results = Vec::with_capacity(total_texts);
        for (chunk_idx, chunk_result) in results {
            let chunk_results = chunk_result
                .with_context(|| format!("classification chunk {} failed", chunk_idx))?;
            combined_results.extend(chunk_results);
        }

        info!(
            job_id = %job_id,
            total_texts,
            result_count = combined_results.len(),
            "classification parallel processing completed"
        );

        Ok(combined_results)
    }

    async fn classify_chunk(
        &self,
        job_id: Uuid,
        texts: Vec<String>,
        chunk_idx: usize,
    ) -> Result<Vec<ClassificationResult>> {
        let request = ClassificationRequest {
            texts: texts.clone(),
        };
        let url = self.build_classify_url()?;

        info!(
            job_id = %job_id,
            chunk_idx,
            text_count = texts.len(),
            "processing classification chunk"
        );

        // Generate unique idempotency key for this chunk
        let idempotency_key = format!("{}-chunk-{}", job_id, chunk_idx);

        let response = self
            .send_classify_request(job_id, &request, &url, Some(&idempotency_key))
            .await
            .with_context(|| {
                format!(
                    "classify-runs POST request failed for job {} chunk {}",
                    job_id, chunk_idx
                )
            })?;
        let body = self
            .parse_classify_response(job_id, response)
            .await
            .with_context(|| {
                format!(
                    "failed to parse classify-runs response for job {} chunk {}",
                    job_id, chunk_idx
                )
            })?;

        let results = self
            .process_classify_body(job_id, body)
            .await
            .with_context(|| {
                format!(
                    "failed to process classification chunk {} for job {}",
                    chunk_idx, job_id
                )
            })?;

        info!(
            job_id = %job_id,
            chunk_idx,
            result_count = results.len(),
            "classification chunk completed"
        );

        Ok(results)
    }

    fn build_classify_url(&self) -> Result<Url> {
        self.base_url
            .join("v1/classify-runs")
            .context("failed to build classify-runs URL")
    }

    async fn send_classify_request(
        &self,
        job_id: Uuid,
        request: &ClassificationRequest,
        url: &Url,
        idempotency_key: Option<&str>,
    ) -> Result<Response> {
        info!(
            job_id = %job_id,
            text_count = request.texts.len(),
            url = %url,
            idempotency_key = idempotency_key,
            "sending classification job request"
        );

        let mut response = None;
        for attempt in 0..CLASSIFY_POST_RETRIES {
            let mut req = self
                .client
                .post(url.clone())
                .timeout(Duration::from_secs(30)) // Fail fast if connection hangs/drops
                .header("X-Alt-Job-Id", job_id.to_string());

            // Add idempotency key header if provided
            if let Some(key) = idempotency_key {
                req = req.header("Idempotency-Key", key);
            }

            let req = req.json(request);
            match req.send().await {
                Ok(res) => {
                    response = Some(res);
                    break;
                }
                Err(err) => {
                    let attempt_num = attempt + 1;
                    let backoff_ms = CLASSIFY_POST_BACKOFF_MS * (attempt_num as u64);
                    warn!(
                        job_id = %job_id,
                        attempt = attempt_num,
                        max_attempts = CLASSIFY_POST_RETRIES,
                        backoff_ms,
                        error = %err,
                        "classify-runs POST request failed, waiting before retry"
                    );
                    if attempt_num >= CLASSIFY_POST_RETRIES {
                        return Err(anyhow!(
                            "classify-runs POST request failed for job {} after {} attempts: {}",
                            job_id,
                            CLASSIFY_POST_RETRIES,
                            err
                        ));
                    }
                    sleep(Duration::from_millis(backoff_ms)).await;
                }
            }
        }

        response.with_context(|| format!("classify-runs POST request failed for job {}", job_id))
    }

    async fn parse_classify_response(
        &self,
        job_id: Uuid,
        response: Response,
    ) -> Result<ClassificationJobResponse> {
        let status = response.status();
        info!(
            job_id = %job_id,
            http_status = %status,
            "received classification job response"
        );

        if !status.is_success() {
            let body = response.text().await.unwrap_or_default();
            let truncated_body = truncate_error_message(&body);
            error!(
                job_id = %job_id,
                http_status = %status,
                error_body = %truncated_body,
                "classify-runs endpoint returned error status"
            );
            return Err(anyhow!(
                "classify-runs endpoint returned error status {}: {}",
                status,
                truncated_body
            ));
        }

        response
            .json()
            .await
            .context("failed to parse classify-runs response")
    }

    async fn process_classify_body(
        &self,
        job_id: Uuid,
        body: ClassificationJobResponse,
    ) -> Result<Vec<ClassificationResult>> {
        info!(
            job_id = %job_id,
            run_id = body.run_id,
            status = %body.status,
            "classification job created"
        );

        if body.status == "running" {
            info!(
                job_id = %job_id,
                run_id = body.run_id,
                "starting classification run polling"
            );
            let results = self.poll_classification_run(body.run_id).await?;
            info!(
                job_id = %job_id,
                run_id = body.run_id,
                result_count = results.len(),
                "classification run polling completed"
            );
            Ok(results)
        } else if body.status == "succeeded" {
            info!(
                job_id = %job_id,
                run_id = body.run_id,
                result_count = body.results.as_ref().map_or(0, Vec::len),
                "classification job already completed"
            );
            Ok(body.results.unwrap_or_default())
        } else {
            error!(
                job_id = %job_id,
                run_id = body.run_id,
                status = %body.status,
                error_message = %body.error_message.as_deref().unwrap_or(""),
                "classification run finished with non-success status"
            );
            Err(anyhow!(
                "classification run {} finished with status {}: {}",
                body.run_id,
                body.status,
                body.error_message.unwrap_or_default()
            ))
        }
    }

    async fn poll_classification_run(&self, run_id: i64) -> Result<Vec<ClassificationResult>> {
        let run_url = self
            .base_url
            .join(&format!("v1/classify-runs/{}", run_id))
            .with_context(|| format!("failed to build classify-run URL for run_id {}", run_id))?;

        info!(run_id, url = %run_url, "starting classification run polling");

        for attempt in 0..MAX_POLL_ATTEMPTS {
            Self::log_polling_attempt(run_id, attempt, &run_url);

            let response = self
                .send_poll_request_with_retry(run_id, &run_url, attempt)
                .await?;

            let status = response.status();
            if !status.is_success() {
                let body = response.text().await.unwrap_or_default();
                let truncated_body = truncate_error_message(&body);
                return Err(anyhow!(
                    "classification run polling endpoint returned error status {} for run_id {}: {}",
                    status,
                    run_id,
                    truncated_body
                ));
            }

            let body: ClassificationJobResponse = response.json().await.with_context(|| {
                format!(
                    "failed to deserialize polling response for run_id {}",
                    run_id
                )
            })?;

            if let Some(result) = Self::handle_classification_status(run_id, attempt, &body)? {
                return Ok(result);
            }

            Self::log_progress(run_id, attempt, &body.status);
            Self::sleep_with_backoff(attempt).await;
        }

        Err(anyhow!(
            "classification run {} did not complete within timeout ({} attempts)",
            run_id,
            MAX_POLL_ATTEMPTS
        ))
    }

    fn log_polling_attempt(run_id: i64, attempt: usize, run_url: &Url) {
        #[allow(clippy::manual_is_multiple_of)]
        if attempt == 0 || (attempt + 1) % 5 == 0 {
            info!(
                run_id,
                attempt = attempt + 1,
                max_attempts = MAX_POLL_ATTEMPTS,
                "polling classification run status"
            );
        } else {
            debug!(
                run_id,
                attempt = attempt + 1,
                url = %run_url,
                "sending classification run polling request"
            );
        }
    }

    fn handle_classification_status(
        run_id: i64,
        attempt: usize,
        body: &ClassificationJobResponse,
    ) -> Result<Option<Vec<ClassificationResult>>> {
        if body.status == "succeeded" {
            info!(
                run_id,
                attempt = attempt + 1,
                result_count = body.results.as_ref().map_or(0, Vec::len),
                "classification run completed successfully"
            );
            Ok(Some(body.results.clone().unwrap_or_default()))
        } else if body.status == "failed" {
            error!(
                run_id,
                attempt = attempt + 1,
                error_message = %body.error_message.as_deref().unwrap_or(""),
                "classification run failed"
            );
            Err(anyhow!(
                "classification run {} failed: {}",
                run_id,
                body.error_message.as_deref().unwrap_or("")
            ))
        } else {
            Ok(None)
        }
    }

    fn log_progress(run_id: i64, attempt: usize, status: &str) {
        #[allow(clippy::manual_is_multiple_of)]
        if (attempt + 1) % 5 == 0 {
            info!(
                run_id,
                attempt = attempt + 1,
                status = %status,
                "classification run still in progress"
            );
        } else {
            debug!(
                run_id,
                attempt,
                status = %status,
                "classification run still in progress"
            );
        }
    }

    async fn send_poll_request_with_retry(
        &self,
        run_id: i64,
        url: &Url,
        poll_attempt: usize,
    ) -> Result<Response> {
        let mut last_error = None;

        for retry in 0..POLL_REQUEST_RETRIES {
            match self.client.get(url.clone()).send().await {
                Ok(response) => return Ok(response),
                Err(e) => {
                    // Retry on transient errors (timeout, connection errors)
                    if e.is_timeout() || e.is_connect() {
                        warn!(
                            run_id,
                            poll_attempt = poll_attempt + 1,
                            retry = retry + 1,
                            max_retries = POLL_REQUEST_RETRIES,
                            error = %e,
                            "polling request failed, retrying"
                        );
                        sleep(Duration::from_millis(POLL_REQUEST_RETRY_DELAY_MS)).await;
                        last_error = Some(e);
                        continue;
                    }
                    // Return immediately for other errors
                    return Err(anyhow::Error::from(e)).with_context(|| {
                        format!(
                            "classification run polling request failed for run_id {} (attempt {})",
                            run_id,
                            poll_attempt + 1
                        )
                    });
                }
            }
        }

        match last_error {
            Some(e) => Err(anyhow!(
                "classification run polling request failed after {} retries for run_id {} (attempt {}): {}",
                POLL_REQUEST_RETRIES,
                run_id,
                poll_attempt + 1,
                e
            )),
            None => Err(anyhow!(
                "classification run polling request failed after {} retries for run_id {} (attempt {})",
                POLL_REQUEST_RETRIES,
                run_id,
                poll_attempt + 1
            )),
        }
    }

    async fn sleep_with_backoff(attempt: usize) {
        // Exponential backoff: start at 2s, double until reaching 30s cap
        // Sequence: 2s, 4s, 8s, 16s, 30s, 30s, ...
        let mut interval_ms = INITIAL_POLL_INTERVAL_MS;
        for _ in 0..attempt {
            interval_ms = std::cmp::min(interval_ms * 2, MAX_POLL_INTERVAL_MS);
        }
        sleep(Duration::from_millis(interval_ms)).await;
    }

    pub(crate) async fn ping(&self) -> Result<()> {
        let url = self
            .base_url
            .join("health")
            .context("failed to build subworker health URL")?;

        self.client
            .get(url)
            .send()
            .await
            .context("subworker health request failed")?
            .error_for_status()
            .context("subworker health endpoint returned error status")?;

        Ok(())
    }

    /// /admin/build-graphを呼び出してtag_label_graphを再構築
    #[allow(dead_code)]
    pub(crate) async fn trigger_graph_rebuild(&self) -> Result<()> {
        let url = self
            .base_url
            .join("admin/build-graph")
            .context("failed to build subworker build-graph URL")?;

        tracing::info!("triggering tag_label_graph rebuild");
        let response = self
            .client
            .post(url)
            .send()
            .await
            .context("subworker build-graph request failed")?;

        let status = response.status();
        if !status.is_success() {
            let error_text = response
                .text()
                .await
                .unwrap_or_else(|_| "unknown error".to_string());
            return Err(anyhow!(
                "subworker build-graph endpoint returned error status {}: {}",
                status,
                error_text
            ));
        }

        let result: Value = response
            .json()
            .await
            .context("failed to parse build-graph response")?;

        tracing::info!(
            result = ?result,
            "tag_label_graph rebuild completed"
        );

        Ok(())
    }

    /// /admin/learningを呼び出してジャンル学習を実行
    #[allow(dead_code)]
    pub(crate) async fn trigger_learning(&self) -> Result<()> {
        let url = self
            .base_url
            .join("admin/learning")
            .context("failed to build subworker learning URL")?;

        tracing::info!("triggering genre learning");
        let response = self
            .client
            .post(url)
            .send()
            .await
            .context("subworker learning request failed")?;

        let status = response.status();
        // 202 Accepted は正常なレスポンス（非同期処理開始）
        if status != reqwest::StatusCode::ACCEPTED && !status.is_success() {
            let error_text = response
                .text()
                .await
                .unwrap_or_else(|_| "unknown error".to_string());
            return Err(anyhow!(
                "subworker learning endpoint returned error status {}: {}",
                status,
                error_text
            ));
        }

        tracing::info!(
            status = %status,
            "genre learning triggered successfully"
        );

        Ok(())
    }

    /// グラフ最新化シーケンス（build-graph -> learning）を実行
    pub(crate) async fn refresh_graph_and_learning(&self) -> Result<()> {
        self.kick_and_poll_admin_job("admin/graph-jobs")
            .await
            .context("graph job failed")?;
        self.kick_and_poll_admin_job("admin/learning-jobs")
            .await
            .context("learning job failed")?;
        tracing::info!("graph refresh and learning sequence completed via admin jobs");
        Ok(())
    }

    async fn kick_and_poll_admin_job(&self, endpoint: &str) -> Result<()> {
        let job_id = self
            .start_admin_job(endpoint)
            .await
            .with_context(|| format!("failed to start admin job at {}", endpoint))?;
        self.poll_admin_job(endpoint, job_id).await?;
        Ok(())
    }

    async fn start_admin_job(&self, endpoint: &str) -> Result<Uuid> {
        let url = self
            .base_url
            .join(endpoint)
            .with_context(|| format!("failed to build admin job URL for {}", endpoint))?;

        tracing::info!(endpoint = %url, "starting admin job");
        let response = self
            .client
            .post(url.clone())
            .send()
            .await
            .with_context(|| format!("admin job POST request failed for {}", endpoint))?;

        let status = response.status();
        if status != reqwest::StatusCode::ACCEPTED && !status.is_success() {
            let error_text = response
                .text()
                .await
                .unwrap_or_else(|_| "unknown error".to_string());
            return Err(anyhow!(
                "admin job endpoint {} returned error status {}: {}",
                endpoint,
                status,
                truncate_error_message(&error_text)
            ));
        }

        let body: AdminJobKickResponse = response
            .json()
            .await
            .with_context(|| format!("failed to parse admin job kick response for {}", endpoint))?;
        tracing::info!(
            job_id = %body.job_id,
            endpoint,
            "admin job started, beginning polling"
        );
        Ok(body.job_id)
    }

    async fn poll_admin_job(&self, endpoint: &str, job_id: Uuid) -> Result<()> {
        let mut backoff = Duration::from_millis(ADMIN_JOB_INITIAL_BACKOFF_MS);
        let deadline = Instant::now() + Duration::from_secs(ADMIN_JOB_TIMEOUT_SECS);
        let url = self
            .base_url
            .join(&format!("{}/{}", endpoint, job_id))
            .with_context(|| format!("failed to build admin job status URL for {}", job_id))?;

        tracing::info!(
            job_id = %job_id,
            endpoint = %url,
            "starting admin job polling"
        );

        let mut poll_count = 0u32;
        loop {
            if Instant::now() > deadline {
                return Err(anyhow!(
                    "admin job {} at {} timed out after {:?}",
                    job_id,
                    endpoint,
                    Duration::from_secs(ADMIN_JOB_TIMEOUT_SECS)
                ));
            }

            let response = self.client.get(url.clone()).send().await.with_context(|| {
                format!(
                    "admin job polling request failed for job_id {} at {}",
                    job_id, endpoint
                )
            })?;

            let status = response.status();
            if !status.is_success() {
                let body = response.text().await.unwrap_or_default();
                return Err(anyhow!(
                    "admin job polling endpoint returned error status {} for job_id {}: {}",
                    status,
                    job_id,
                    truncate_error_message(&body)
                ));
            }

            let body: AdminJobStatusResponse = response.json().await.with_context(|| {
                format!(
                    "failed to deserialize admin job status for job_id {}",
                    job_id
                )
            })?;

            poll_count += 1;
            let elapsed = deadline.saturating_duration_since(Instant::now());

            match body.status.as_str() {
                "succeeded" | "partial" => {
                    tracing::info!(
                        job_id = %job_id,
                        endpoint,
                        status = %body.status,
                        poll_count,
                        "admin job completed successfully"
                    );
                    return Ok(());
                }
                "failed" => {
                    return Err(anyhow!(
                        "admin job {} failed: {}",
                        job_id,
                        body.error.unwrap_or_else(|| "unknown error".to_string())
                    ));
                }
                _ => {
                    // Log progress every 10 polls
                    if poll_count.is_multiple_of(10) {
                        tracing::info!(
                            job_id = %job_id,
                            endpoint,
                            status = %body.status,
                            poll_count,
                            elapsed_seconds = elapsed.as_secs(),
                            backoff_ms = backoff.as_millis(),
                            "admin job still running"
                        );
                    } else {
                        tracing::debug!(
                            job_id = %job_id,
                            status = %body.status,
                            poll_count,
                            backoff_ms = backoff.as_millis(),
                            "admin job still running"
                        );
                    }
                    tokio::time::sleep(backoff).await;
                    backoff =
                        std::cmp::min(backoff * 2, Duration::from_millis(ADMIN_JOB_MAX_BACKOFF_MS));
                }
            }
        }
    }

    pub(crate) async fn extract_content(&self, html: &str) -> Result<String> {
        let url = self
            .base_url
            .join("v1/extract")
            .context("failed to build extract URL")?;

        let request = ExtractRequest {
            html,
            include_comments: false,
        };

        let response = self
            .client
            .post(url)
            .json(&request)
            .timeout(Duration::from_secs(EXTRACTION_TIMEOUT_SECS))
            .send()
            .await
            .context("extract request failed")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(anyhow!("extract endpoint error {}: {}", status, body));
        }

        let body: ExtractResponse = response
            .json()
            .await
            .context("failed to parse extract response")?;
        Ok(body.text)
    }

    pub(crate) async fn classify_coarse(&self, text: &str) -> Result<HashMap<String, f32>> {
        let url = self
            .base_url
            .join("v1/classify/coarse")
            .context("failed to build classify_coarse URL")?;

        // Truncate text if too long to avoid huge payload?
        // Implementer responsibility, but subworker endpoint handles 2000 chars prefix.
        // We send full text, let server handle it or truncate here?
        // Server truncates.
        let request = CoarseClassifyRequest { text };

        let response = self
            .client
            .post(url)
            .json(&request)
            .send()
            .await
            .context("classify_coarse request failed")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(anyhow!(
                "classify_coarse endpoint error {}: {}",
                status,
                body
            ));
        }

        let body: CoarseClassifyResponse = response
            .json()
            .await
            .context("failed to parse classify_coarse response")?;
        Ok(body.scores)
    }

    #[allow(dead_code)]
    pub(crate) async fn cluster_other(
        &self,
        sentences: Vec<String>,
    ) -> anyhow::Result<(Vec<i32>, Option<Vec<i32>>, Option<Vec<Vec<f32>>>)> {
        let url = self
            .base_url
            .join("v1/cluster/other")
            .context("failed to build cluster_other URL")?;

        let body = SubClusterOtherRequest { sentences };
        let response = self
            .client
            .post(url)
            .json(&body)
            .send()
            .await
            .context("cluster_other request failed")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(anyhow!("cluster_other endpoint error {}: {}", status, body));
        }

        let body: SubClusterOtherResponse = response
            .json()
            .await
            .context("failed to parse cluster_other response")?;
        Ok((body.cluster_ids, body.labels, body.centers))
    }

    /// フォールバック用の単一クラスタレスポンスを生成する。
    ///
    /// # Arguments
    /// * `job_id` - ジョブID
    /// * `corpus` - 証拠コーパス
    ///
    /// # Returns
    /// 全記事を含む単一クラスタのレスポンス
    pub(crate) fn create_fallback_response(
        job_id: Uuid,
        corpus: &EvidenceCorpus,
    ) -> ClusteringResponse {
        // 全記事を含む単一クラスタを構築
        let mut representatives = Vec::new();
        for article in &corpus.articles {
            // 各記事の最初の文を代表として使用
            if let Some(first_sentence) = article.sentences.first() {
                representatives.push(ClusterRepresentative {
                    article_id: article.article_id.clone(),
                    paragraph_idx: Some(0),
                    text: first_sentence.clone(),
                    lang: Some(article.language.clone()),
                    score: Some(1.0),
                    reasons: vec!["fallback".to_string()],
                });
            }
        }

        let cluster = ClusterInfo {
            cluster_id: 0,
            size: corpus.articles.len(),
            label: Some(corpus.genre.clone()),
            top_terms: Vec::new(),
            stats: serde_json::json!({}),
            representatives,
        };

        ClusteringResponse {
            run_id: 0, // フォールバックのため run_id は 0
            job_id,
            genre: corpus.genre.clone(),
            status: ClusterJobStatus::Succeeded,
            cluster_count: 1,
            clusters: vec![cluster],
            genre_highlights: None,
            diagnostics: serde_json::json!({
                "fallback": true,
                "reason": "insufficient_documents_for_clustering"
            }),
        }
    }

    /// 証拠コーパスを送信してクラスタリング結果を取得する。
    ///
    /// # Arguments
    /// * `job_id` - ジョブID
    /// * `corpus` - 証拠コーパス
    ///
    /// # Returns
    /// クラスタリング結果（JSON Schema検証済み）
    pub(crate) async fn cluster_corpus(
        &self,
        job_id: Uuid,
        corpus: &EvidenceCorpus,
    ) -> Result<ClusteringResponse> {
        let runs_url = build_runs_url(&self.base_url)?;

        debug!(
            job_id = %job_id,
            genre = %corpus.genre,
            article_count = corpus.articles.len(),
            "sending evidence corpus to subworker"
        );

        let request_payload = build_cluster_job_request(corpus);
        let idempotency_key = format!("{}::{}::{}", job_id, corpus.genre, Uuid::new_v4());
        let document_count = request_payload.documents.len();

        if document_count < self.min_documents_per_genre {
            if document_count >= MIN_FALLBACK_DOCUMENTS {
                // フォールバック: 単一クラスタとして処理
                warn!(
                    job_id = %job_id,
                    genre = %corpus.genre,
                    document_count,
                    min_required = self.min_documents_per_genre,
                    "using fallback single-cluster response due to insufficient documents"
                );
                return Ok(Self::create_fallback_response(job_id, corpus));
            }
            // 記事数が極端に少ない場合はエラー
            warn!(
                job_id = %job_id,
                genre = %corpus.genre,
                document_count,
                min_fallback = MIN_FALLBACK_DOCUMENTS,
                "skipping clustering because document count is below minimum fallback threshold"
            );
            return Err(anyhow!(
                "insufficient documents for clustering: expected >= {}, found {}",
                MIN_FALLBACK_DOCUMENTS,
                document_count
            ));
        }

        let response = self
            .client
            .post(runs_url.clone())
            .json(&request_payload)
            .header("X-Alt-Job-Id", job_id.to_string())
            .header("X-Alt-Genre", &corpus.genre)
            .header("Idempotency-Key", idempotency_key)
            .send()
            .await
            .context("clustering request failed")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            let truncated_body = truncate_error_message(&body);
            return Err(anyhow!(
                "clustering endpoint returned error status {}: {}",
                status,
                truncated_body
            ));
        }

        // レスポンスをJSONとして取得
        let response_json: Value = response
            .json()
            .await
            .context("failed to deserialize clustering response as JSON")?;

        // JSON Schemaで検証
        let validation = validate_json(&CLUSTERING_RESPONSE_SCHEMA, &response_json);
        if !validation.valid {
            let summarized_errors = summarize_validation_errors(&validation.errors);
            warn!(
                job_id = %job_id,
                genre = %corpus.genre,
                error_count = validation.errors.len(),
                first_error = %summarized_errors.first().map_or("unknown", String::as_str),
                "clustering response failed JSON Schema validation"
            );
            return Err(anyhow!(
                "clustering response validation failed: {} errors (first: {})",
                validation.errors.len(),
                summarized_errors.first().map_or("unknown", String::as_str)
            ));
        }

        debug!(
            job_id = %job_id,
            genre = %corpus.genre,
            "clustering response passed JSON Schema validation"
        );

        // 検証済みのJSONを構造体にデシリアライズ
        let mut run: ClusteringResponse = serde_json::from_value(response_json)
            .context("failed to deserialize validated clustering response")?;

        if run.status.is_running() {
            run = self.poll_run(run.run_id).await?;
        }

        if !run.is_success() {
            return Err(anyhow!(
                "clustering run {} finished with status {}",
                run.run_id,
                run.status
            ));
        }

        Ok(run)
    }

    async fn poll_run(&self, run_id: i64) -> Result<ClusteringResponse> {
        let run_url = build_run_url(&self.base_url, run_id)?;

        for attempt in 0..MAX_POLL_ATTEMPTS {
            let response = self
                .client
                .get(run_url.clone())
                .send()
                .await
                .context("clustering run polling request failed")?;

            if !response.status().is_success() {
                let status = response.status();
                let body = response.text().await.unwrap_or_default();
                let truncated_body = truncate_error_message(&body);
                return Err(anyhow!(
                    "run polling endpoint returned error status {}: {}",
                    status,
                    truncated_body
                ));
            }

            let response_json: Value = response
                .json()
                .await
                .context("failed to deserialize polling response as JSON")?;

            let validation = validate_json(&CLUSTERING_RESPONSE_SCHEMA, &response_json);
            if !validation.valid {
                let summarized_errors = summarize_validation_errors(&validation.errors);
                warn!(
                    run_id,
                    error_count = validation.errors.len(),
                    first_error = %summarized_errors.first().map_or("unknown", String::as_str),
                    "polling response failed JSON Schema validation"
                );
                return Err(anyhow!(
                    "run polling response validation failed: {} errors (first: {})",
                    validation.errors.len(),
                    summarized_errors.first().map_or("unknown", String::as_str)
                ));
            }

            let run: ClusteringResponse = serde_json::from_value(response_json)
                .context("failed to deserialize validated polling response")?;

            if run.status.is_terminal() {
                if !run.is_success() {
                    warn!(
                        run_id,
                        status = %run.status,
                        "clustering run completed with non-success status"
                    );
                }
                return Ok(run);
            }

            debug!(
                run_id,
                attempt,
                status = %run.status,
                "clustering run still in progress"
            );

            let backoff = cmp::min(
                INITIAL_POLL_INTERVAL_MS * (1_u64 << attempt.min(10)),
                MAX_POLL_INTERVAL_MS,
            );
            sleep(Duration::from_millis(backoff)).await;
        }

        Err(anyhow!(
            "clustering run {} did not complete within timeout",
            run_id
        ))
    }
}

fn build_runs_url(base: &Url) -> Result<Url> {
    let mut url = base.clone();
    url.path_segments_mut()
        .map_err(|()| anyhow!("subworker base URL must be absolute"))?
        .extend(["v1", "runs"]);
    Ok(url)
}

fn build_run_url(base: &Url, run_id: i64) -> Result<Url> {
    let mut url = base.clone();
    url.path_segments_mut()
        .map_err(|()| anyhow!("subworker base URL must be absolute"))?
        .extend(["v1", "runs", &run_id.to_string()]);
    Ok(url)
}

fn build_cluster_job_request(corpus: &EvidenceCorpus) -> ClusterJobRequest<'_> {
    let max_sentences_total = corpus
        .total_sentences
        .clamp(MIN_PARAGRAPH_LEN, DEFAULT_MAX_SENTENCES_TOTAL);

    let documents = corpus
        .articles
        .iter()
        .map(|article| ClusterDocument {
            article_id: &article.article_id,
            title: article.title.as_ref(),
            lang_hint: Some(&article.language),
            published_at: None,
            source_url: None,
            paragraphs: vec![build_paragraph(&article.sentences)],
            genre_scores: article.genre_scores.as_ref(),
            confidence: article.confidence,
            signals: article.signals.as_ref(),
        })
        .collect();

    ClusterJobRequest {
        params: ClusterJobParams {
            max_sentences_total,
            max_sentences_per_cluster: 20, /* Strict limit from Plan 9 */
            umap_n_components: DEFAULT_UMAP_N_COMPONENTS,
            hdbscan_min_cluster_size: DEFAULT_HDBSCAN_MIN_CLUSTER_SIZE,
            mmr_lambda: DEFAULT_MMR_LAMBDA,
        },
        documents,
        metadata: Some(&corpus.metadata),
    }
}

/// 文のリストからパラグラフを構築する。
/// 改行で分割した各パラグラフが30文字以上になることを保証する。
/// Unicode文字数を使用して正確にカウントする。
fn build_paragraph(sentences: &[String]) -> String {
    fn ensure_min_length(mut line: String) -> String {
        let mut filler = line.trim().to_string();
        if filler.is_empty() {
            filler = "No content available.".to_string();
        }

        if line.trim().is_empty() {
            line.clone_from(&filler);
        }

        while line.chars().count() < MIN_PARAGRAPH_LEN {
            if !line.is_empty() {
                line.push(' ');
            }
            line.push_str(&filler);
        }

        line
    }

    if sentences.is_empty() {
        let line = ensure_min_length(String::new());
        return format!("{line}\n{line}");
    }

    let mut lines = Vec::new();
    let mut current = String::new();

    for sentence in sentences {
        let trimmed = sentence.trim();
        if trimmed.is_empty() {
            continue;
        }

        if !current.is_empty() {
            current.push(' ');
        }
        current.push_str(trimmed);

        if current.chars().count() >= MIN_PARAGRAPH_LEN {
            let line = ensure_min_length(std::mem::take(&mut current));
            lines.push(line);
        }
    }

    if !current.trim().is_empty() {
        let line = ensure_min_length(std::mem::take(&mut current));
        lines.push(line);
    }

    if lines.is_empty() {
        let line = ensure_min_length(String::new());
        return format!("{line}\n{line}");
    }

    lines.join("\n")
}

#[cfg(test)]
mod tests {
    use super::*;
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    #[tokio::test]
    async fn ping_succeeds_for_ok_response() {
        let server = MockServer::start().await;
        Mock::given(method("GET"))
            .and(path("/health"))
            .respond_with(ResponseTemplate::new(204))
            .mount(&server)
            .await;

        let client = SubworkerClient::new(server.uri(), 10).expect("client should build");

        client.ping().await.expect("ping should succeed");
    }

    #[tokio::test]
    async fn ping_fails_on_error_status() {
        let server = MockServer::start().await;
        Mock::given(method("GET"))
            .and(path("/health"))
            .respond_with(ResponseTemplate::new(500))
            .mount(&server)
            .await;

        let client = SubworkerClient::new(server.uri(), 10).expect("client should build");

        let error = client.ping().await.expect_err("ping should fail");
        assert!(error.to_string().contains("error status"));
    }

    #[test]
    fn build_paragraph_uses_newlines_and_extends_length() {
        let sentences = vec![
            "This sentence is plenty long for clustering purposes.".to_string(),
            "Another sufficiently detailed sentence to pass validation.".to_string(),
        ];

        let paragraph = build_paragraph(&sentences);
        assert!(paragraph.contains('\n'));
        // Unicode文字数で検証
        assert!(paragraph.chars().count() >= MIN_PARAGRAPH_LEN);
        // recap-subworker側はsplitlines()を使用するので、それに合わせて検証
        let parts: Vec<&str> = paragraph.lines().collect();
        assert!(parts.iter().all(|part| !part.trim().is_empty()));
        // 各パラグラフが30文字以上であることを確認
        for part in parts {
            assert!(
                part.chars().count() >= MIN_PARAGRAPH_LEN,
                "Each paragraph must be at least {} characters, got {}",
                MIN_PARAGRAPH_LEN,
                part.chars().count()
            );
        }
    }

    #[test]
    fn build_paragraph_falls_back_for_empty_articles() {
        let paragraph = build_paragraph(&[]);
        assert!(paragraph.contains('\n'));
        assert!(paragraph.chars().count() >= MIN_PARAGRAPH_LEN);
        // 分割後の各パラグラフが30文字以上であることを確認（splitlines()を使用）
        let parts: Vec<&str> = paragraph.lines().collect();
        for part in parts {
            assert!(
                part.chars().count() >= MIN_PARAGRAPH_LEN,
                "Each paragraph must be at least {} characters",
                MIN_PARAGRAPH_LEN
            );
        }
    }

    #[test]
    fn build_paragraph_handles_japanese_short_sentences() {
        // エラーが発生した実際のケース: 18文字の日本語文
        let sentences = vec!["やっぱり洗練されたサーバーの構成は美しい".to_string()];

        let paragraph = build_paragraph(&sentences);
        // 分割後の各パラグラフが30文字以上であることを確認（splitlines()を使用）
        let parts: Vec<&str> = paragraph.lines().collect();
        for part in parts {
            let char_count = part.chars().count();
            assert!(
                char_count >= MIN_PARAGRAPH_LEN,
                "Each paragraph must be at least {} characters, got {}: '{}'",
                MIN_PARAGRAPH_LEN,
                char_count,
                part
            );
        }
    }

    #[test]
    fn build_paragraph_handles_multiple_short_sentences() {
        // 複数の短い文を結合するケース
        let sentences = vec![
            "短い文1".to_string(),
            "短い文2".to_string(),
            "短い文3".to_string(),
            "短い文4".to_string(),
        ];

        let paragraph = build_paragraph(&sentences);
        let parts: Vec<&str> = paragraph.lines().collect();
        for part in parts {
            assert!(
                part.chars().count() >= MIN_PARAGRAPH_LEN,
                "Each paragraph must be at least {} characters",
                MIN_PARAGRAPH_LEN
            );
        }
    }

    #[test]
    fn build_paragraph_boundary_test_29_chars() {
        // 29文字の文（境界値テスト）
        let sentence_29 = "a".repeat(29);
        let sentences = vec![sentence_29.clone()];

        let paragraph = build_paragraph(&sentences);
        let parts: Vec<&str> = paragraph.lines().collect();
        for part in parts {
            assert!(
                part.chars().count() >= MIN_PARAGRAPH_LEN,
                "29-char sentence should be extended to at least {} characters",
                MIN_PARAGRAPH_LEN
            );
        }
    }

    #[test]
    fn build_paragraph_boundary_test_30_chars() {
        // 30文字の文（境界値テスト）
        let sentence_30 = "a".repeat(30);
        let sentences = vec![sentence_30.clone()];

        let paragraph = build_paragraph(&sentences);
        let parts: Vec<&str> = paragraph.lines().collect();
        for part in parts {
            assert!(
                part.chars().count() >= MIN_PARAGRAPH_LEN,
                "30-char sentence should be at least {} characters",
                MIN_PARAGRAPH_LEN
            );
        }
    }

    #[test]
    fn build_paragraph_boundary_test_31_chars() {
        // 31文字の文（境界値テスト）
        let sentence_31 = "a".repeat(31);
        let sentences = vec![sentence_31.clone()];

        let paragraph = build_paragraph(&sentences);
        let parts: Vec<&str> = paragraph.lines().collect();
        for part in parts {
            assert!(
                part.chars().count() >= MIN_PARAGRAPH_LEN,
                "31-char sentence should be at least {} characters",
                MIN_PARAGRAPH_LEN
            );
        }
    }

    #[test]
    fn build_paragraph_handles_mixed_languages() {
        // 日本語と英語が混在するケース
        let sentences = vec![
            "This is a short English sentence.".to_string(),
            "これは短い日本語の文です。".to_string(),
        ];

        let paragraph = build_paragraph(&sentences);
        let parts: Vec<&str> = paragraph.lines().collect();
        for part in parts {
            assert!(
                part.chars().count() >= MIN_PARAGRAPH_LEN,
                "Each paragraph must be at least {} characters",
                MIN_PARAGRAPH_LEN
            );
        }
    }

    #[test]
    fn build_paragraph_verifies_each_split_paragraph() {
        // 複数の長い文で、分割後の各パラグラフが30文字以上であることを確認
        let sentences = vec![
            "This is a very long sentence that exceeds the minimum paragraph length requirement."
                .to_string(),
            "Another long sentence that also exceeds the minimum requirement for paragraph length."
                .to_string(),
            "A third sentence that is also sufficiently long to meet the requirements.".to_string(),
        ];

        let paragraph = build_paragraph(&sentences);
        // 改行で分割した各パラグラフを検証（splitlines()を使用）
        let parts: Vec<&str> = paragraph.lines().collect();
        for (idx, part) in parts.iter().enumerate() {
            let char_count = part.chars().count();
            assert!(
                char_count >= MIN_PARAGRAPH_LEN,
                "Paragraph {} must be at least {} characters, got {}: '{}'",
                idx,
                MIN_PARAGRAPH_LEN,
                char_count,
                part
            );
        }
    }

    #[test]
    fn truncate_error_message_handles_short_messages() {
        let msg = "Short error message";
        let result = truncate_error_message(msg);
        assert_eq!(result, msg);
    }

    #[test]
    fn truncate_error_message_handles_long_ascii_messages() {
        let msg = "a".repeat(600);
        let result = truncate_error_message(&msg);
        assert!(result.starts_with('a'));
        assert!(result.contains("... (truncated"));
        assert!(result.contains("600 chars"));
        // 切り詰められた部分が正確に500文字であることを確認
        let truncated_part = result.split("...").next().unwrap();
        assert_eq!(truncated_part.chars().count(), MAX_ERROR_MESSAGE_LENGTH);
    }

    #[test]
    fn truncate_error_message_handles_japanese_at_boundary() {
        // ログで実際に発生したケース: 500文字目が日本語文字の途中
        // 'れ' (bytes 498..501) や 'ン' (bytes 499..502) が境界に来る
        let mut msg = "a".repeat(498);
        msg.push_str("れン");
        msg.push_str(&"b".repeat(100));

        // パニックが発生しないことを確認
        let result = truncate_error_message(&msg);
        assert!(result.contains("... (truncated"));
        // 切り詰められた部分が文字境界で切れていることを確認
        let truncated_part = result.split("...").next().unwrap();
        assert_eq!(truncated_part.chars().count(), MAX_ERROR_MESSAGE_LENGTH);
        // 文字列が有効なUTF-8であることを確認（パニックしない）
        assert!(truncated_part.is_char_boundary(truncated_part.len()));
    }

    #[test]
    fn truncate_error_message_handles_mixed_languages() {
        let mut msg = String::new();
        msg.push_str("This is an English error message. ");
        msg.push_str("これは日本語のエラーメッセージです。 ");
        msg.push_str("This is more English text. ");
        msg.push_str("さらに日本語のテキストが続きます。 ");
        // 500文字を超えるように繰り返す
        msg = msg.repeat(20);

        let result = truncate_error_message(&msg);
        assert!(result.contains("... (truncated"));
        let truncated_part = result.split("...").next().unwrap();
        assert_eq!(truncated_part.chars().count(), MAX_ERROR_MESSAGE_LENGTH);
        // 文字列が有効なUTF-8であることを確認
        assert!(truncated_part.is_char_boundary(truncated_part.len()));
    }

    #[test]
    fn truncate_error_message_preserves_exact_length_messages() {
        let msg = "a".repeat(MAX_ERROR_MESSAGE_LENGTH);
        let result = truncate_error_message(&msg);
        assert_eq!(result, msg);
        assert!(!result.contains("... (truncated"));
    }

    #[test]
    fn create_fallback_response_creates_single_cluster() {
        use crate::pipeline::evidence::{CorpusMetadata, EvidenceArticle};
        use std::collections::HashMap;
        use uuid::Uuid;

        let job_id = Uuid::new_v4();

        let corpus = EvidenceCorpus {
            genre: "politics".to_string(),
            articles: vec![
                EvidenceArticle {
                    article_id: "article-1".to_string(),
                    title: Some("Test Article 1".to_string()),
                    sentences: vec!["First sentence of article 1.".to_string()],
                    language: "en".to_string(),
                    published_at: None,
                    source_url: None,
                    score: 0.0,
                    genre_scores: Some(HashMap::new()),
                    confidence: Some(0.9),
                    signals: None,
                },
                EvidenceArticle {
                    article_id: "article-2".to_string(),
                    title: Some("Test Article 2".to_string()),
                    sentences: vec!["First sentence of article 2.".to_string()],
                    language: "en".to_string(),
                    published_at: None,
                    source_url: None,
                    score: 0.0,
                    genre_scores: None,
                    confidence: Some(0.7),
                    signals: None,
                },
            ],
            total_sentences: 2,
            metadata: CorpusMetadata {
                article_count: 2,
                sentence_count: 2,
                primary_language: "en".to_string(),
                language_distribution: {
                    let mut map = HashMap::new();
                    map.insert("en".to_string(), 2);
                    map
                },
                character_count: 50,
                classifier: None,
            },
        };

        let response = SubworkerClient::create_fallback_response(job_id, &corpus);

        assert_eq!(response.job_id, job_id);
        assert_eq!(response.genre, "politics");
        assert_eq!(response.status, ClusterJobStatus::Succeeded);
        assert_eq!(response.cluster_count, 1);
        assert_eq!(response.clusters.len(), 1);
        assert_eq!(response.clusters[0].cluster_id, 0);
        assert_eq!(response.clusters[0].size, 2);
        assert_eq!(response.clusters[0].label, Some("politics".to_string()));
        assert_eq!(response.clusters[0].representatives.len(), 2);
        assert!(
            response
                .diagnostics
                .get("fallback")
                .and_then(serde_json::Value::as_bool)
                .unwrap_or(false)
        );
    }
}
