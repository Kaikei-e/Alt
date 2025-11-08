use std::cmp;
use std::fmt;
use std::time::Duration;

use anyhow::{Context, Result, anyhow};
use reqwest::{Client, Url};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use tokio::time::sleep;
use tracing::{debug, warn};
use uuid::Uuid;

use crate::pipeline::evidence::EvidenceCorpus;
use crate::schema::{subworker::CLUSTERING_RESPONSE_SCHEMA, validate_json};

#[derive(Debug, Clone)]
pub(crate) struct SubworkerClient {
    client: Client,
    base_url: Url,
}

const DEFAULT_MAX_SENTENCES_TOTAL: usize = 2_000;
const DEFAULT_UMAP_N_COMPONENTS: usize = 25;
const DEFAULT_HDBSCAN_MIN_CLUSTER_SIZE: usize = 5;
const DEFAULT_MMR_LAMBDA: f32 = 0.35;
const MIN_PARAGRAPH_LEN: usize = 30;
const MAX_POLL_ATTEMPTS: usize = 30;
const INITIAL_POLL_INTERVAL_MS: u64 = 500;
const MAX_POLL_INTERVAL_MS: u64 = 5_000;
const SUBWORKER_TIMEOUT_SECS: u64 = 120;

/// クラスタリングレスポンス (POST/GET `/v1/runs`).
#[derive(Debug, Clone, Deserialize)]
pub(crate) struct ClusteringResponse {
    pub(crate) run_id: i64,
    pub(crate) _job_id: Uuid,
    pub(crate) genre: String,
    pub(crate) status: ClusterJobStatus,
    #[serde(default)]
    pub(crate) _cluster_count: usize,
    pub(crate) clusters: Vec<ClusterInfo>,
    #[serde(default)]
    pub(crate) _diagnostics: Value,
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

#[derive(Debug, Clone, Deserialize)]
pub(crate) struct ClusterInfo {
    pub(crate) cluster_id: usize,
    #[serde(default)]
    pub(crate) _size: usize,
    #[serde(default)]
    pub(crate) _label: Option<String>,
    #[serde(default)]
    pub(crate) top_terms: Vec<String>,
    #[serde(default)]
    pub(crate) _stats: Value,
    #[serde(default)]
    pub(crate) representatives: Vec<ClusterRepresentative>,
}

#[derive(Debug, Clone, Deserialize)]
pub(crate) struct ClusterRepresentative {
    #[serde(default)]
    pub(crate) _article_id: String,
    #[serde(default)]
    pub(crate) _paragraph_idx: Option<i32>,
    #[serde(rename = "sentence_text")]
    pub(crate) text: String,
    #[serde(default)]
    pub(crate) _lang: Option<String>,
    #[serde(default)]
    pub(crate) _score: Option<f32>,
}

#[derive(Debug, Clone, Serialize)]
struct ClusterJobRequest<'a> {
    params: ClusterJobParams,
    documents: Vec<ClusterDocument<'a>>,
}

#[derive(Debug, Clone, Serialize)]
struct ClusterJobParams {
    max_sentences_total: usize,
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
}

impl SubworkerClient {
    pub(crate) fn new(endpoint: impl Into<String>) -> Result<Self> {
        let client = Client::builder()
            .timeout(Duration::from_secs(SUBWORKER_TIMEOUT_SECS))
            .build()
            .context("failed to build subworker client")?;

        let base_url = Url::parse(&endpoint.into()).context("invalid subworker base URL")?;

        Ok(Self { client, base_url })
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
        let idempotency_key = format!("{}::{}", job_id, corpus.genre);

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
            return Err(anyhow!(
                "clustering endpoint returned error status {}: {}",
                status,
                body
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
            warn!(
                job_id = %job_id,
                genre = %corpus.genre,
                errors = ?validation.errors,
                "clustering response failed JSON Schema validation"
            );
            return Err(anyhow!(
                "clustering response validation failed: {:?}",
                validation.errors
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
                return Err(anyhow!(
                    "run polling endpoint returned error status {}: {}",
                    status,
                    body
                ));
            }

            let response_json: Value = response
                .json()
                .await
                .context("failed to deserialize polling response as JSON")?;

            let validation = validate_json(&CLUSTERING_RESPONSE_SCHEMA, &response_json);
            if !validation.valid {
                warn!(
                    run_id,
                    errors = ?validation.errors,
                    "polling response failed JSON Schema validation"
                );
                return Err(anyhow!(
                    "run polling response validation failed: {:?}",
                    validation.errors
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
        .map_err(|_| anyhow!("subworker base URL must be absolute"))?
        .extend(["v1", "runs"]);
    Ok(url)
}

fn build_run_url(base: &Url, run_id: i64) -> Result<Url> {
    let mut url = base.clone();
    url.path_segments_mut()
        .map_err(|_| anyhow!("subworker base URL must be absolute"))?
        .extend(["v1", "runs", &run_id.to_string()]);
    Ok(url)
}

fn build_cluster_job_request(corpus: &EvidenceCorpus) -> ClusterJobRequest<'_> {
    let max_sentences_total = corpus
        .total_sentences
        .max(MIN_PARAGRAPH_LEN)
        .min(DEFAULT_MAX_SENTENCES_TOTAL);

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
        })
        .collect();

    ClusterJobRequest {
        params: ClusterJobParams {
            max_sentences_total,
            umap_n_components: DEFAULT_UMAP_N_COMPONENTS,
            hdbscan_min_cluster_size: DEFAULT_HDBSCAN_MIN_CLUSTER_SIZE,
            mmr_lambda: DEFAULT_MMR_LAMBDA,
        },
        documents,
    }
}

fn build_paragraph(sentences: &[String]) -> String {
    if sentences.is_empty() {
        return "No content available.".repeat(2);
    }

    let mut paragraph = sentences.join(" ");
    let filler = sentences.last().unwrap();

    while paragraph.len() < MIN_PARAGRAPH_LEN {
        paragraph.push(' ');
        paragraph.push_str(filler);
    }

    paragraph
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

        let client = SubworkerClient::new(server.uri()).expect("client should build");

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

        let client = SubworkerClient::new(server.uri()).expect("client should build");

        let error = client.ping().await.expect_err("ping should fail");
        assert!(error.to_string().contains("error status"));
    }
}
