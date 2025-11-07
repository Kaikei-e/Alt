use std::time::Duration;

use anyhow::{anyhow, Context, Result};
use reqwest::{Client, StatusCode, Url};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use tracing::{debug, warn};
use uuid::Uuid;

use crate::pipeline::evidence::EvidenceCorpus;
use crate::schema::{subworker::CLUSTERING_RESPONSE_SCHEMA, validate_json};

#[derive(Debug, Clone)]
pub(crate) struct SubworkerClient {
    client: Client,
    base_url: Url,
}

#[derive(Debug, Deserialize, PartialEq, Eq)]
pub(crate) struct SubworkerArticle {
    pub(crate) id: Uuid,
    pub(crate) title: String,
    pub(crate) body: String,
    #[serde(default)]
    pub(crate) language: Option<String>,
}

#[derive(Debug, Deserialize, PartialEq, Eq)]
pub(crate) struct SubworkerCorpus {
    pub(crate) job_id: Uuid,
    pub(crate) articles: Vec<SubworkerArticle>,
}

/// クラスタリングレスポンス。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct ClusteringResponse {
    pub(crate) job_id: Uuid,
    pub(crate) genre: String,
    pub(crate) clusters: Vec<Cluster>,
    pub(crate) metadata: ClusteringMetadata,
}

/// クラスター。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct Cluster {
    pub(crate) cluster_id: usize,
    pub(crate) sentences: Vec<ClusterSentence>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) centroid: Option<Vec<f64>>,
    pub(crate) top_terms: Vec<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) coherence_score: Option<f64>,
}

/// クラスター内の文。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct ClusterSentence {
    pub(crate) sentence_id: usize,
    pub(crate) text: String,
    pub(crate) source_article_id: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) embedding: Option<Vec<f64>>,
}

/// クラスタリングメタデータ。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct ClusteringMetadata {
    pub(crate) total_sentences: usize,
    pub(crate) cluster_count: usize,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) processing_time_ms: Option<usize>,
}

impl SubworkerClient {
    pub(crate) fn new(endpoint: impl Into<String>) -> Result<Self> {
        let client = Client::builder()
            .timeout(Duration::from_secs(10))
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

    pub(crate) async fn fetch_corpus(&self, job_id: Uuid) -> Result<SubworkerCorpus> {
        let url = self
            .base_url
            .join(&format!("jobs/{job_id}"))
            .context("failed to build subworker jobs URL")?;

        let response = self
            .client
            .get(url)
            .send()
            .await
            .context("subworker corpus request failed")?;

        if response.status() == StatusCode::NOT_FOUND {
            return Ok(SubworkerCorpus {
                job_id,
                articles: Vec::new(),
            });
        }

        let response = response
            .error_for_status()
            .context("subworker corpus endpoint returned error status")?;

        response
            .json::<SubworkerCorpus>()
            .await
            .context("failed to deserialize subworker corpus response")
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
        let url = self
            .base_url
            .join(&format!("cluster/{}", corpus.genre))
            .context("failed to build clustering URL")?;

        debug!(
            job_id = %job_id,
            genre = %corpus.genre,
            article_count = corpus.articles.len(),
            "sending evidence corpus to subworker"
        );

        let response = self
            .client
            .post(url)
            .json(corpus)
            .header("X-Job-ID", job_id.to_string())
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
        serde_json::from_value(response_json)
            .context("failed to deserialize validated clustering response")
    }
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

    #[tokio::test]
    async fn fetch_corpus_returns_articles() {
        let server = MockServer::start().await;
        let job_id = Uuid::new_v4();
        let body = serde_json::json!({
            "job_id": job_id,
            "articles": [
                {
                    "id": Uuid::new_v4(),
                    "title": "Title",
                    "body": "Body",
                    "language": "en"
                }
            ]
        });

        Mock::given(method("GET"))
            .and(path(format!("/jobs/{job_id}").as_str()))
            .respond_with(ResponseTemplate::new(200).set_body_json(body.clone()))
            .mount(&server)
            .await;

        let client = SubworkerClient::new(server.uri()).expect("client should build");
        let corpus = client
            .fetch_corpus(job_id)
            .await
            .expect("fetch should succeed");

        let expected: SubworkerCorpus = serde_json::from_value(body).expect("valid body");
        assert_eq!(corpus, expected);
    }

    #[tokio::test]
    async fn fetch_corpus_propagates_error_status() {
        let server = MockServer::start().await;
        let job_id = Uuid::new_v4();

        Mock::given(method("GET"))
            .and(path(format!("/jobs/{job_id}").as_str()))
            .respond_with(ResponseTemplate::new(404))
            .mount(&server)
            .await;

        let client = SubworkerClient::new(server.uri()).expect("client should build");
        let corpus = client
            .fetch_corpus(job_id)
            .await
            .expect("404 should return empty corpus");
        assert!(corpus.articles.is_empty());
    }
}
