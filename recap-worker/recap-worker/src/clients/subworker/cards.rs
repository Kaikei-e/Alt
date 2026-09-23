//! Subworker cards client for cross-cutting topic card embedding and clustering.
//!
//! Provides `SubworkerCardsClient` to interact with recap-subworker's
//! embedding, story clustering, and card verification endpoints.

use chrono::{DateTime, Utc};
use reqwest::{Client, Url};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::time::Duration;
use uuid::Uuid;

use super::utils::truncate_error_message;
use crate::error::{RecapError, Result};
use crate::util::retry::{RetryConfig, is_retryable_error};

pub const DEFAULT_EMBED_BATCH_SIZE: usize = 128;
pub const DEFAULT_EMBED_TIMEOUT_SECS: u64 = 120;
pub const DEFAULT_CLUSTER_TIMEOUT_SECS: u64 = 120;
pub const DEFAULT_RETRY_ATTEMPTS: usize = 3;
pub const DEFAULT_RETRY_BACKOFF_BASE_MS: u64 = 200;
pub const DEFAULT_RETRY_BACKOFF_CAP_MS: u64 = 2000;

#[derive(Debug, Clone, Serialize, PartialEq)]
pub struct EmbedRequest<'a> {
    pub texts: &'a [String],
    pub normalize: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct EmbedResponse {
    pub model: String,
    pub dim: usize,
    pub embeddings: Vec<Vec<f32>>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct ClusterStoryItem {
    pub id: Uuid,
    pub embedding: Vec<f32>,
    pub published_at: DateTime<Utc>,
    /// Optional ISO language code (e.g. "ja", "en").
    /// `None` = previous behaviour (language-agnostic clustering).
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub language: Option<String>,
}

impl ClusterStoryItem {
    pub fn new(id: Uuid, embedding: Vec<f32>, published_at: DateTime<Utc>) -> Self {
        Self {
            id,
            embedding,
            published_at,
            language: None,
        }
    }

    pub fn with_language(mut self, language: impl Into<String>) -> Self {
        self.language = Some(language.into());
        self
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct StoryClusterParams {
    #[serde(default = "default_threshold")]
    pub threshold: f32,
    #[serde(default = "default_linkage")]
    pub linkage: String,
    #[serde(default = "default_time_decay")]
    pub time_decay_per_day: f32,
    #[serde(default = "default_min_cluster_size")]
    pub min_cluster_size: usize,
    /// Optional per-language minimum cluster size overrides.
    /// `None` = previous behaviour (uniform `min_cluster_size` across all languages).
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub min_cluster_size_by_language: Option<HashMap<String, usize>>,
}

fn default_threshold() -> f32 {
    0.78
}

fn default_linkage() -> String {
    "average".to_string()
}

fn default_time_decay() -> f32 {
    0.02
}

fn default_min_cluster_size() -> usize {
    1
}

impl Default for StoryClusterParams {
    fn default() -> Self {
        Self {
            threshold: default_threshold(),
            linkage: default_linkage(),
            time_decay_per_day: default_time_decay(),
            min_cluster_size: default_min_cluster_size(),
            min_cluster_size_by_language: None,
        }
    }
}

impl StoryClusterParams {
    pub fn with_min_cluster_size_by_language(
        mut self,
        min_by_lang: HashMap<String, usize>,
    ) -> Self {
        self.min_cluster_size_by_language = Some(min_by_lang);
        self
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct ClusterStoriesRequest {
    pub items: Vec<ClusterStoryItem>,
    #[serde(default)]
    pub params: StoryClusterParams,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct ClusterOutput {
    pub cluster_id: usize,
    pub member_ids: Vec<Uuid>,
    pub centroid: Vec<f32>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct ClusterStoriesResponse {
    pub clusters: Vec<ClusterOutput>,
    pub params: StoryClusterParams,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct VerifySentenceInput {
    pub idx: usize,
    pub kind: String,
    pub text: String,
    pub refs: Vec<i64>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct VerifyItemInput {
    pub n: i64,
    pub title: String,
    pub lede: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct VerifyThresholds {
    #[serde(default = "default_attribution_cos")]
    pub attribution_cos: f32,
}

fn default_attribution_cos() -> f32 {
    0.55
}

impl Default for VerifyThresholds {
    fn default() -> Self {
        Self {
            attribution_cos: default_attribution_cos(),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct VerifyCardRequest {
    pub job_id: Uuid,
    pub card_id: Uuid,
    pub language: String,
    pub sentences: Vec<VerifySentenceInput>,
    pub items: Vec<VerifyItemInput>,
    #[serde(default)]
    pub thresholds: VerifyThresholds,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct VerifyAttribution {
    pub max_cos: f32,
    pub best_n: Option<i64>,
    pub pass: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct VerifyFiller {
    pub matched: Vec<String>,
    pub pass: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct VerifySpecificity {
    pub proper_nouns: usize,
    pub numbers: usize,
    pub tokens: usize,
    pub density: f32,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct VerifySentenceResult {
    pub idx: usize,
    pub attribution: VerifyAttribution,
    pub filler: VerifyFiller,
    pub specificity: VerifySpecificity,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct VerifyEmbeddingInfo {
    pub model: String,
    pub identity: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct VerifyCardResponse {
    pub sentences: Vec<VerifySentenceResult>,
    pub why_hint_present: bool,
    pub embedding: VerifyEmbeddingInfo,
}

#[derive(Debug, Clone)]
pub struct SubworkerCardsClient {
    client: Client,
    base_url: Url,
    embed_timeout: Duration,
    cluster_timeout: Duration,
    admin_token: Option<String>,
    batch_size: usize,
    retry_config: RetryConfig,
}

impl SubworkerCardsClient {
    pub(crate) fn new(endpoint: impl Into<String>) -> Result<Self> {
        let client = Client::builder()
            .timeout(Duration::from_secs(DEFAULT_CLUSTER_TIMEOUT_SECS))
            .build()
            .map_err(|e| {
                RecapError::Clustering(format!("failed to build subworker cards HTTP client: {e}"))
            })?;
        Self::new_with_client(endpoint, client)
    }

    pub(crate) fn new_with_client(endpoint: impl Into<String>, client: Client) -> Result<Self> {
        let endpoint_str = endpoint.into();
        let base_url = Url::parse(&endpoint_str).map_err(|e| {
            RecapError::Clustering(format!("invalid subworker base URL '{endpoint_str}': {e}"))
        })?;
        Ok(Self::new_with_url(base_url, client))
    }

    pub(crate) fn new_with_url(base_url: Url, client: Client) -> Self {
        Self {
            client,
            base_url,
            embed_timeout: Duration::from_secs(DEFAULT_EMBED_TIMEOUT_SECS),
            cluster_timeout: Duration::from_secs(DEFAULT_CLUSTER_TIMEOUT_SECS),
            admin_token: None,
            batch_size: DEFAULT_EMBED_BATCH_SIZE,
            retry_config: RetryConfig::new(
                DEFAULT_RETRY_ATTEMPTS,
                DEFAULT_RETRY_BACKOFF_BASE_MS,
                DEFAULT_RETRY_BACKOFF_CAP_MS,
            ),
        }
    }

    #[must_use]
    pub(crate) fn with_admin_token(mut self, token: Option<String>) -> Self {
        self.admin_token = token;
        self
    }

    #[cfg(test)]
    #[must_use]
    pub(crate) fn with_batch_size(mut self, batch_size: usize) -> Self {
        self.batch_size = batch_size;
        self
    }

    fn apply_auth(&self, builder: reqwest::RequestBuilder) -> reqwest::RequestBuilder {
        match &self.admin_token {
            Some(token) => builder.bearer_auth(token),
            None => builder,
        }
    }

    fn build_endpoint_url(&self, segments: &[&str]) -> Result<Url> {
        let mut url = self.base_url.clone();
        url.path_segments_mut()
            .map_err(|()| {
                RecapError::Clustering("subworker base URL must be absolute".to_string())
            })?
            .pop_if_empty()
            .extend(segments);
        Ok(url)
    }

    pub(crate) async fn embed(&self, texts: &[String], normalize: bool) -> Result<EmbedResponse> {
        if texts.is_empty() {
            return Err(RecapError::Clustering(
                "EmptyEmbedInput: texts input cannot be empty".to_string(),
            ));
        }

        let url = self.build_endpoint_url(&["v1", "embed"])?;
        let batch_size = if self.batch_size == 0 {
            DEFAULT_EMBED_BATCH_SIZE
        } else {
            self.batch_size
        };

        let mut all_embeddings = Vec::with_capacity(texts.len());
        let mut response_model = String::new();
        let mut response_dim = 0;

        for chunk in texts.chunks(batch_size) {
            let chunk_response = self.send_embed_chunk(&url, chunk, normalize).await?;
            if response_model.is_empty() {
                response_model = chunk_response.model;
                response_dim = chunk_response.dim;
            } else {
                if response_model != chunk_response.model {
                    return Err(RecapError::Clustering(format!(
                        "embedding model mismatch across chunks: expected {response_model}, got {}",
                        chunk_response.model
                    )));
                }
                if response_dim != chunk_response.dim {
                    return Err(RecapError::Clustering(format!(
                        "embedding dimension mismatch across chunks: expected {response_dim}, got {}",
                        chunk_response.dim
                    )));
                }
            }
            all_embeddings.extend(chunk_response.embeddings);
        }

        Ok(EmbedResponse {
            model: response_model,
            dim: response_dim,
            embeddings: all_embeddings,
        })
    }

    async fn send_embed_chunk(
        &self,
        url: &Url,
        texts: &[String],
        normalize: bool,
    ) -> Result<EmbedResponse> {
        let request_body = EmbedRequest { texts, normalize };
        let mut attempt = 0;

        loop {
            let send_result = self
                .apply_auth(self.client.post(url.clone()))
                .timeout(self.embed_timeout)
                .json(&request_body)
                .send()
                .await;

            let response = match send_result {
                Ok(resp) => resp,
                Err(err) => {
                    attempt += 1;
                    if !is_retryable_error(&err) || !self.retry_config.can_retry(attempt) {
                        return Err(RecapError::Clustering(format!(
                            "subworker embed request failed: {err}"
                        )));
                    }
                    let delay = self.retry_config.delay_for_attempt(attempt);
                    tracing::warn!(
                        attempt,
                        delay_ms = delay.as_millis(),
                        error = %err,
                        "subworker embed request failed, retrying"
                    );
                    tokio::time::sleep(delay).await;
                    continue;
                }
            };

            let status = response.status();
            if !status.is_success() {
                let is_retryable =
                    status.is_server_error() || status == reqwest::StatusCode::TOO_MANY_REQUESTS;
                let body = response.text().await.unwrap_or_default();
                let excerpt = truncate_error_message(&body);

                attempt += 1;
                if !is_retryable || !self.retry_config.can_retry(attempt) {
                    return Err(RecapError::Clustering(format!(
                        "subworker embed endpoint returned error status {status}: {excerpt}"
                    )));
                }

                let delay = self.retry_config.delay_for_attempt(attempt);
                tracing::warn!(
                    attempt,
                    delay_ms = delay.as_millis(),
                    %status,
                    "subworker embed endpoint returned error status, retrying"
                );
                tokio::time::sleep(delay).await;
                continue;
            }

            let parsed: EmbedResponse = response.json().await.map_err(|err| {
                RecapError::Clustering(format!(
                    "failed to deserialize subworker embed response: {err}"
                ))
            })?;

            return Ok(parsed);
        }
    }

    pub(crate) async fn cluster_stories(
        &self,
        req: &ClusterStoriesRequest,
    ) -> Result<ClusterStoriesResponse> {
        let url = self.build_endpoint_url(&["v1", "cluster-stories"])?;
        let mut attempt = 0;

        loop {
            let send_result = self
                .apply_auth(self.client.post(url.clone()))
                .timeout(self.cluster_timeout)
                .json(req)
                .send()
                .await;

            let response = match send_result {
                Ok(resp) => resp,
                Err(err) => {
                    attempt += 1;
                    if !is_retryable_error(&err) || !self.retry_config.can_retry(attempt) {
                        return Err(RecapError::Clustering(format!(
                            "subworker cluster-stories request failed: {err}"
                        )));
                    }
                    let delay = self.retry_config.delay_for_attempt(attempt);
                    tracing::warn!(
                        attempt,
                        delay_ms = delay.as_millis(),
                        error = %err,
                        "subworker cluster-stories request failed, retrying"
                    );
                    tokio::time::sleep(delay).await;
                    continue;
                }
            };

            let status = response.status();
            if !status.is_success() {
                let is_retryable =
                    status.is_server_error() || status == reqwest::StatusCode::TOO_MANY_REQUESTS;
                let body = response.text().await.unwrap_or_default();
                let excerpt = truncate_error_message(&body);

                attempt += 1;
                if !is_retryable || !self.retry_config.can_retry(attempt) {
                    return Err(RecapError::Clustering(format!(
                        "subworker cluster-stories endpoint returned error status {status}: {excerpt}"
                    )));
                }

                let delay = self.retry_config.delay_for_attempt(attempt);
                tracing::warn!(
                    attempt,
                    delay_ms = delay.as_millis(),
                    %status,
                    "subworker cluster-stories endpoint returned error status, retrying"
                );
                tokio::time::sleep(delay).await;
                continue;
            }

            let parsed: ClusterStoriesResponse = response.json().await.map_err(|err| {
                RecapError::Clustering(format!(
                    "failed to deserialize subworker cluster-stories response: {err}"
                ))
            })?;

            return Ok(parsed);
        }
    }

    pub(crate) async fn verify_card(&self, req: &VerifyCardRequest) -> Result<VerifyCardResponse> {
        let url = self.build_endpoint_url(&["v1", "verify"])?;
        let mut attempt = 0;

        loop {
            let send_result = self
                .apply_auth(self.client.post(url.clone()))
                .timeout(self.embed_timeout)
                .json(req)
                .send()
                .await;

            let response = match send_result {
                Ok(resp) => resp,
                Err(err) => {
                    attempt += 1;
                    if !is_retryable_error(&err) || !self.retry_config.can_retry(attempt) {
                        return Err(RecapError::Clustering(format!(
                            "subworker verify request failed: {err}"
                        )));
                    }
                    let delay = self.retry_config.delay_for_attempt(attempt);
                    tracing::warn!(
                        attempt,
                        delay_ms = delay.as_millis(),
                        error = %err,
                        "subworker verify request failed, retrying"
                    );
                    tokio::time::sleep(delay).await;
                    continue;
                }
            };

            let status = response.status();
            if !status.is_success() {
                let is_retryable =
                    status.is_server_error() || status == reqwest::StatusCode::TOO_MANY_REQUESTS;
                let body = response.text().await.unwrap_or_default();
                let excerpt = truncate_error_message(&body);

                attempt += 1;
                if !is_retryable || !self.retry_config.can_retry(attempt) {
                    return Err(RecapError::Clustering(format!(
                        "subworker verify endpoint returned error status {status}: {excerpt}"
                    )));
                }

                let delay = self.retry_config.delay_for_attempt(attempt);
                tracing::warn!(
                    attempt,
                    delay_ms = delay.as_millis(),
                    %status,
                    "subworker verify endpoint returned error status, retrying"
                );
                tokio::time::sleep(delay).await;
                continue;
            }

            let parsed: VerifyCardResponse = response.json().await.map_err(|err| {
                RecapError::Clustering(format!(
                    "failed to deserialize subworker verify response: {err}"
                ))
            })?;

            return Ok(parsed);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::TimeZone;
    use wiremock::matchers::{body_json, header, method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    #[tokio::test]
    async fn test_embed_happy_path() {
        let server = MockServer::start().await;
        let expected_req = serde_json::json!({
            "texts": ["Headline 1", "Headline 2"],
            "normalize": true,
        });
        let res_body = serde_json::json!({
            "model": "bge-m3",
            "dim": 4,
            "embeddings": [
                [0.1, 0.2, 0.3, 0.4],
                [0.5, 0.6, 0.7, 0.8]
            ]
        });

        Mock::given(method("POST"))
            .and(path("/v1/embed"))
            .and(body_json(&expected_req))
            .respond_with(ResponseTemplate::new(200).set_body_json(&res_body))
            .mount(&server)
            .await;

        let client = SubworkerCardsClient::new(server.uri()).expect("client creation succeeds");
        let texts = vec!["Headline 1".to_string(), "Headline 2".to_string()];
        let resp = client
            .embed(&texts, true)
            .await
            .expect("embed should succeed");

        assert_eq!(resp.model, "bge-m3");
        assert_eq!(resp.dim, 4);
        assert_eq!(resp.embeddings.len(), 2);
        assert_eq!(resp.embeddings[0], vec![0.1, 0.2, 0.3, 0.4]);
    }

    #[tokio::test]
    async fn test_embed_empty_input_no_request() {
        let server = MockServer::start().await;
        let client = SubworkerCardsClient::new(server.uri()).expect("client creation succeeds");
        let err = client
            .embed(&[], true)
            .await
            .expect_err("empty embed must fail");

        assert!(
            err.to_string().contains("EmptyEmbedInput"),
            "Error should mention EmptyEmbedInput: {err}"
        );
        assert_eq!(server.received_requests().await.unwrap().len(), 0);
    }

    #[tokio::test]
    async fn test_embed_chunking_300_texts() {
        let server = MockServer::start().await;

        Mock::given(method("POST"))
            .and(path("/v1/embed"))
            .respond_with(|req: &wiremock::Request| {
                let body: serde_json::Value = req.body_json().unwrap();
                let count = body["texts"].as_array().unwrap().len();
                let embeddings: Vec<Vec<f32>> = vec![vec![0.1, 0.2, 0.3, 0.4]; count];
                ResponseTemplate::new(200).set_body_json(serde_json::json!({
                    "model": "bge-m3",
                    "dim": 4,
                    "embeddings": embeddings,
                }))
            })
            .mount(&server)
            .await;

        let client = SubworkerCardsClient::new(server.uri()).expect("client creation succeeds");
        let texts: Vec<String> = (0..300).map(|i| format!("Text {i}")).collect();
        let resp = client
            .embed(&texts, true)
            .await
            .expect("chunked embed succeeds");

        assert_eq!(resp.embeddings.len(), 300);
        let requests = server.received_requests().await.unwrap();
        assert_eq!(requests.len(), 3);

        let body1: serde_json::Value = requests[0].body_json().unwrap();
        let body2: serde_json::Value = requests[1].body_json().unwrap();
        let body3: serde_json::Value = requests[2].body_json().unwrap();
        assert_eq!(body1["texts"].as_array().unwrap().len(), 128);
        assert_eq!(body2["texts"].as_array().unwrap().len(), 128);
        assert_eq!(body3["texts"].as_array().unwrap().len(), 44);
    }

    #[tokio::test]
    async fn test_embed_chunking_dimension_mismatch() {
        let server = MockServer::start().await;
        let count = std::sync::atomic::AtomicUsize::new(0);
        Mock::given(method("POST"))
            .and(path("/v1/embed"))
            .respond_with(move |_: &wiremock::Request| {
                let prev = count.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
                let dim = if prev == 0 { 4 } else { 8 };
                ResponseTemplate::new(200).set_body_json(serde_json::json!({
                    "model": "bge-m3",
                    "dim": dim,
                    "embeddings": vec![vec![0.1; dim]],
                }))
            })
            .mount(&server)
            .await;

        let client = SubworkerCardsClient::new(server.uri())
            .expect("client creation succeeds")
            .with_batch_size(1);
        let texts = vec!["Text 1".to_string(), "Text 2".to_string()];
        let err = client
            .embed(&texts, true)
            .await
            .expect_err("dimension mismatch must fail");

        assert!(
            err.to_string().contains("dimension mismatch"),
            "Error should mention dimension mismatch: {err}"
        );
    }

    #[tokio::test]
    async fn test_embed_chunking_model_mismatch() {
        let server = MockServer::start().await;
        let count = std::sync::atomic::AtomicUsize::new(0);
        Mock::given(method("POST"))
            .and(path("/v1/embed"))
            .respond_with(move |_: &wiremock::Request| {
                let prev = count.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
                let model = if prev == 0 {
                    "bge-m3"
                } else {
                    "text-embedding-3-small"
                };
                ResponseTemplate::new(200).set_body_json(serde_json::json!({
                    "model": model,
                    "dim": 4,
                    "embeddings": vec![vec![0.1; 4]],
                }))
            })
            .mount(&server)
            .await;

        let client = SubworkerCardsClient::new(server.uri())
            .expect("client creation succeeds")
            .with_batch_size(1);
        let texts = vec!["Text 1".to_string(), "Text 2".to_string()];
        let err = client
            .embed(&texts, true)
            .await
            .expect_err("model mismatch must fail");

        assert!(
            err.to_string().contains("model mismatch"),
            "Error should mention model mismatch: {err}"
        );
    }

    #[tokio::test]
    async fn test_embed_422_error_mapping() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/v1/embed"))
            .respond_with(ResponseTemplate::new(422).set_body_json(serde_json::json!({
                "detail": "text at index 0 must not be empty or whitespace"
            })))
            .mount(&server)
            .await;

        let client = SubworkerCardsClient::new(server.uri()).expect("client creation succeeds");
        let err = client
            .embed(&["   ".to_string()], true)
            .await
            .expect_err("embed must fail on 422");

        let err_msg = err.to_string();
        assert!(
            err_msg.contains("422"),
            "Error message should contain status code: {err_msg}"
        );
        assert!(
            err_msg.contains("must not be empty or whitespace"),
            "Error message should contain excerpt: {err_msg}"
        );
    }

    #[tokio::test]
    async fn test_cluster_stories_happy_path() {
        let server = MockServer::start().await;
        let id1 = Uuid::new_v4();
        let id2 = Uuid::new_v4();

        let res_body = serde_json::json!({
            "clusters": [
                {
                    "cluster_id": 0,
                    "member_ids": [id1, id2],
                    "centroid": [0.1, 0.2, 0.3, 0.4]
                }
            ],
            "params": {
                "threshold": 0.78,
                "linkage": "average",
                "time_decay_per_day": 0.02,
                "min_cluster_size": 1
            }
        });

        Mock::given(method("POST"))
            .and(path("/v1/cluster-stories"))
            .respond_with(ResponseTemplate::new(200).set_body_json(&res_body))
            .mount(&server)
            .await;

        let client = SubworkerCardsClient::new(server.uri()).expect("client creation succeeds");
        let req = ClusterStoriesRequest {
            items: vec![
                ClusterStoryItem {
                    id: id1,
                    embedding: vec![0.1, 0.2, 0.3, 0.4],
                    published_at: Utc.with_ymd_and_hms(2026, 9, 21, 0, 0, 0).unwrap(),
                    language: None,
                },
                ClusterStoryItem {
                    id: id2,
                    embedding: vec![0.1, 0.2, 0.3, 0.4],
                    published_at: Utc.with_ymd_and_hms(2026, 9, 21, 0, 0, 0).unwrap(),
                    language: None,
                },
            ],
            params: StoryClusterParams::default(),
        };

        let resp = client
            .cluster_stories(&req)
            .await
            .expect("cluster_stories should succeed");
        assert_eq!(resp.clusters.len(), 1);
        assert_eq!(resp.clusters[0].cluster_id, 0);
        assert_eq!(resp.clusters[0].member_ids, vec![id1, id2]);
        assert_eq!(resp.clusters[0].centroid, vec![0.1, 0.2, 0.3, 0.4]);
        assert!((resp.params.threshold - 0.78).abs() < 1e-4);
    }

    #[tokio::test]
    async fn test_cluster_stories_422_error_mapping() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/v1/cluster-stories"))
            .respond_with(ResponseTemplate::new(422).set_body_json(serde_json::json!({
                "detail": "embedding dimension mismatch at index 1: expected 4, got 3"
            })))
            .mount(&server)
            .await;

        let client = SubworkerCardsClient::new(server.uri()).expect("client creation succeeds");
        let req = ClusterStoriesRequest {
            items: vec![],
            params: StoryClusterParams::default(),
        };

        let err = client
            .cluster_stories(&req)
            .await
            .expect_err("cluster_stories must fail on 422");

        let err_msg = err.to_string();
        assert!(
            err_msg.contains("422"),
            "Error message should contain status code: {err_msg}"
        );
        assert!(
            err_msg.contains("embedding dimension mismatch"),
            "Error message should contain excerpt: {err_msg}"
        );
    }

    #[test]
    fn test_cluster_stories_json_shape_and_defaults() {
        let id = Uuid::parse_str("00000000-0000-0000-0000-000000000001").unwrap();
        let item = ClusterStoryItem {
            id,
            embedding: vec![0.1, 0.2],
            published_at: Utc.with_ymd_and_hms(2026, 9, 21, 12, 0, 0).unwrap(),
            language: None,
        };
        let req = ClusterStoriesRequest {
            items: vec![item],
            params: StoryClusterParams::default(),
        };

        let json_val = serde_json::to_value(&req).expect("serialization succeeds");
        assert_eq!(
            json_val["items"][0]["id"],
            "00000000-0000-0000-0000-000000000001"
        );
        assert_eq!(json_val["items"][0]["published_at"], "2026-09-21T12:00:00Z");
        assert!((json_val["params"]["threshold"].as_f64().unwrap() - 0.78).abs() < 1e-4);
        assert_eq!(json_val["params"]["linkage"], "average");
        assert!((json_val["params"]["time_decay_per_day"].as_f64().unwrap() - 0.02).abs() < 1e-4);
        assert_eq!(json_val["params"]["min_cluster_size"], 1);
        assert!(json_val["items"][0].get("language").is_none());
        assert!(
            json_val["params"]
                .get("min_cluster_size_by_language")
                .is_none()
        );
    }

    #[test]
    fn test_cluster_stories_optional_fields_serialization() {
        let id = Uuid::parse_str("00000000-0000-0000-0000-000000000001").unwrap();
        let mut min_by_lang = HashMap::new();
        min_by_lang.insert("ja".to_string(), 2usize);
        min_by_lang.insert("en".to_string(), 3usize);

        let item = ClusterStoryItem {
            id,
            embedding: vec![0.1, 0.2],
            published_at: Utc.with_ymd_and_hms(2026, 9, 21, 12, 0, 0).unwrap(),
            language: Some("ja".to_string()),
        };
        let req = ClusterStoriesRequest {
            items: vec![item],
            params: StoryClusterParams {
                min_cluster_size_by_language: Some(min_by_lang),
                ..StoryClusterParams::default()
            },
        };

        let json_val = serde_json::to_value(&req).expect("serialization succeeds");
        assert_eq!(json_val["items"][0]["language"], "ja");
        assert_eq!(json_val["params"]["min_cluster_size_by_language"]["ja"], 2);
        assert_eq!(json_val["params"]["min_cluster_size_by_language"]["en"], 3);

        let deserialized: ClusterStoriesRequest =
            serde_json::from_value(json_val).expect("deserialization succeeds");
        assert_eq!(deserialized, req);
    }

    #[tokio::test]
    async fn test_verify_card_happy_path() {
        let server = MockServer::start().await;
        let job_id = Uuid::new_v4();
        let card_id = Uuid::new_v4();

        let res_body = serde_json::json!({
            "sentences": [
                {
                    "idx": 0,
                    "attribution": {
                        "max_cos": 0.85,
                        "best_n": 1,
                        "pass": true
                    },
                    "filler": {
                        "matched": [],
                        "pass": true
                    },
                    "specificity": {
                        "proper_nouns": 1,
                        "numbers": 0,
                        "tokens": 5,
                        "density": 0.2
                    }
                }
            ],
            "why_hint_present": true,
            "embedding": {
                "model": "bge-m3",
                "identity": "bge-m3"
            }
        });

        Mock::given(method("POST"))
            .and(path("/v1/verify"))
            .respond_with(ResponseTemplate::new(200).set_body_json(&res_body))
            .mount(&server)
            .await;

        let client = SubworkerCardsClient::new(server.uri()).expect("client creation succeeds");
        let req = VerifyCardRequest {
            job_id,
            card_id,
            language: "ja".to_string(),
            sentences: vec![VerifySentenceInput {
                idx: 0,
                kind: "what".to_string(),
                text: "新しいAIモデルが発表された。".to_string(),
                refs: vec![1],
            }],
            items: vec![VerifyItemInput {
                n: 1,
                title: "AIモデルの発表".to_string(),
                lede: "新モデルが本日発表された。".to_string(),
            }],
            thresholds: VerifyThresholds::default(),
        };

        let resp = client
            .verify_card(&req)
            .await
            .expect("verify_card should succeed");
        assert_eq!(resp.sentences.len(), 1);
        assert_eq!(resp.sentences[0].idx, 0);
        assert!(resp.sentences[0].attribution.pass);
        assert_eq!(resp.sentences[0].attribution.best_n, Some(1));
        assert!(resp.sentences[0].filler.pass);
        assert!(resp.why_hint_present);
        assert_eq!(resp.embedding.model, "bge-m3");
    }

    #[tokio::test]
    async fn test_verify_card_502_error_mapping() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/v1/verify"))
            .respond_with(ResponseTemplate::new(502).set_body_json(serde_json::json!({
                "detail": {
                    "reason": "Embedding provider error: connection refused"
                }
            })))
            .mount(&server)
            .await;

        let client = SubworkerCardsClient::new(server.uri()).expect("client creation succeeds");
        let req = VerifyCardRequest {
            job_id: Uuid::new_v4(),
            card_id: Uuid::new_v4(),
            language: "ja".to_string(),
            sentences: vec![],
            items: vec![],
            thresholds: VerifyThresholds::default(),
        };

        let err = client
            .verify_card(&req)
            .await
            .expect_err("verify_card must fail on 502");

        let err_msg = err.to_string();
        assert!(
            err_msg.contains("502"),
            "Error message should contain status code: {err_msg}"
        );
        assert!(
            err_msg.contains("connection refused"),
            "Error message should contain excerpt: {err_msg}"
        );
    }

    #[test]
    fn test_verify_card_json_serialization() {
        let job_id = Uuid::parse_str("00000000-0000-0000-0000-000000000001").unwrap();
        let card_id = Uuid::parse_str("00000000-0000-0000-0000-000000000002").unwrap();

        let req = VerifyCardRequest {
            job_id,
            card_id,
            language: "ja".to_string(),
            sentences: vec![VerifySentenceInput {
                idx: 0,
                kind: "what".to_string(),
                text: "テキスト".to_string(),
                refs: vec![1, 2],
            }],
            items: vec![VerifyItemInput {
                n: 1,
                title: "タイトル".to_string(),
                lede: "リード".to_string(),
            }],
            thresholds: VerifyThresholds::default(),
        };

        let json_val = serde_json::to_value(&req).expect("serialization succeeds");
        assert_eq!(json_val["job_id"], "00000000-0000-0000-0000-000000000001");
        assert_eq!(json_val["card_id"], "00000000-0000-0000-0000-000000000002");
        assert_eq!(json_val["language"], "ja");
        assert_eq!(json_val["sentences"][0]["idx"], 0);
        assert_eq!(json_val["sentences"][0]["kind"], "what");
        assert_eq!(json_val["sentences"][0]["refs"][0], 1);
        assert_eq!(json_val["items"][0]["n"], 1);
        assert!((json_val["thresholds"]["attribution_cos"].as_f64().unwrap() - 0.55).abs() < 1e-4);
    }

    #[tokio::test]
    async fn test_subworker_cards_client_authorization_header_on_wire() {
        let server = MockServer::start().await;
        let token = "test-recap-subworker-token-42";

        Mock::given(method("POST"))
            .and(path("/v1/embed"))
            .and(header("authorization", format!("Bearer {token}").as_str()))
            .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
                "model": "bge-m3",
                "dim": 4,
                "embeddings": [[0.1, 0.2, 0.3, 0.4]],
            })))
            .mount(&server)
            .await;

        Mock::given(method("POST"))
            .and(path("/v1/cluster-stories"))
            .and(header("authorization", format!("Bearer {token}").as_str()))
            .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
                "clusters": [],
                "params": {
                    "threshold": 0.8,
                    "min_cluster_size": 2,
                    "time_decay_half_life_hours": 24.0,
                }
            })))
            .mount(&server)
            .await;

        Mock::given(method("POST"))
            .and(path("/v1/verify"))
            .and(header("authorization", format!("Bearer {token}").as_str()))
            .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
                "sentences": [],
                "why_hint_present": true,
                "embedding": {
                    "model": "bge-m3",
                    "identity": "bge-m3"
                }
            })))
            .mount(&server)
            .await;

        let client = SubworkerCardsClient::new(server.uri())
            .expect("client creation succeeds")
            .with_admin_token(Some(token.to_string()));

        let embed_resp = client
            .embed(&["Headline 1".to_string()], true)
            .await
            .expect("embed should send auth header and succeed");
        assert_eq!(embed_resp.dim, 4);

        let cluster_req = ClusterStoriesRequest {
            items: vec![],
            params: StoryClusterParams::default(),
        };
        let cluster_resp = client
            .cluster_stories(&cluster_req)
            .await
            .expect("cluster_stories should send auth header and succeed");
        assert_eq!(cluster_resp.clusters.len(), 0);

        let verify_req = VerifyCardRequest {
            job_id: Uuid::new_v4(),
            card_id: Uuid::new_v4(),
            language: "ja".to_string(),
            sentences: vec![],
            items: vec![],
            thresholds: VerifyThresholds::default(),
        };
        let verify_resp = client
            .verify_card(&verify_req)
            .await
            .expect("verify_card should send auth header and succeed");
        assert!(verify_resp.why_hint_present);
    }
}
