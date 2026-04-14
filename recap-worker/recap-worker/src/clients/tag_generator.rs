/// tag-generatorからのタグ取得クライアント。
///
/// バッチ取得、タイムアウト、再試行をサポートします。
use std::collections::HashMap;
use std::time::Duration;

use anyhow::{Context, Result};
use chrono::{DateTime, Utc};
use reqwest::{Client, Url};
use serde::{Deserialize, Serialize};
use tracing::debug;

use crate::pipeline::tag_signal::TagSignal;

/// tag-generatorのバッチ取得リクエスト。
#[derive(Debug, Serialize)]
struct BatchTagsRequest {
    article_ids: Vec<String>,
}

/// tag-generatorのバッチ取得レスポンス。
#[derive(Debug, Deserialize)]
struct BatchTagsResponse {
    success: bool,
    tags: HashMap<String, Vec<TagResponse>>,
}

/// tag-generatorから取得したタグの構造。
#[derive(Debug, Clone, PartialEq, Deserialize)]
struct TagResponse {
    tag: String,
    confidence: f32,
    source: String,
    updated_at: String,
}

/// tag-generatorクライアントの設定。
#[derive(Debug, Clone)]
pub(crate) struct TagGeneratorConfig {
    pub(crate) base_url: String,
    pub(crate) connect_timeout: Duration,
    pub(crate) total_timeout: Duration,
    pub(crate) service_token: Option<String>,
}

/// tag-generatorとの通信を管理するクライアント。
#[derive(Debug, Clone)]
pub(crate) struct TagGeneratorClient {
    client: Client,
    base_url: Url,
    service_token: Option<String>,
}

impl TagGeneratorClient {
    /// 新しいtag-generatorクライアントを作成する。
    ///
    /// # Errors
    /// URLのパースまたはHTTPクライアントの構築に失敗した場合はエラーを返します。
    pub(crate) fn new(config: TagGeneratorConfig) -> Result<Self> {
        let client = Client::builder()
            .connect_timeout(config.connect_timeout)
            .timeout(config.total_timeout)
            .build()
            .context("failed to build tag-generator HTTP client")?;
        Self::new_with_client(config, client)
    }

    /// 外部で構築済みの `reqwest::Client` を用いてクライアントを作成する。
    /// mTLS 経路では、identity と root cert を既に反映した client を注入する。
    ///
    /// # Errors
    /// URL のパースに失敗した場合はエラーを返します。
    pub(crate) fn new_with_client(config: TagGeneratorConfig, client: Client) -> Result<Self> {
        let base_url = Url::parse(&config.base_url).context("invalid tag-generator base URL")?;
        Ok(Self {
            client,
            base_url,
            service_token: config.service_token,
        })
    }

    /// 複数の記事IDに対してタグをバッチ取得する。
    ///
    /// # Arguments
    /// * `article_ids` - 記事IDのスライス
    ///
    /// # Errors
    /// HTTPリクエストまたはレスポンスのパースに失敗した場合はエラーを返します。
    pub(crate) async fn fetch_tags_batch(
        &self,
        article_ids: &[String],
    ) -> Result<HashMap<String, Vec<TagSignal>>> {
        if article_ids.is_empty() {
            return Ok(HashMap::new());
        }

        const MAX_BATCH_SIZE: usize = 1000;
        let url = self
            .base_url
            .join("api/v1/tags/batch")
            .context("failed to build batch tags URL")?;

        let mut result = HashMap::new();

        // Split article_ids into chunks of MAX_BATCH_SIZE
        for chunk in article_ids.chunks(MAX_BATCH_SIZE) {
            debug!(
                chunk_size = chunk.len(),
                total = article_ids.len(),
                "fetching tags in batch chunk"
            );

            let request_body = BatchTagsRequest {
                article_ids: chunk.to_vec(),
            };

            let mut request = self.client.post(url.clone()).json(&request_body);

            // Add service authentication token if configured
            if let Some(ref token) = self.service_token {
                request = request.header("X-Service-Token", token);
            }

            let response = request
                .send()
                .await
                .context("tag-generator batch tags request failed")?;

            let status = response.status();

            if !status.is_success() {
                let error_body = response.text().await.unwrap_or_default();
                anyhow::bail!(
                    "tag-generator returned error status {}: {}",
                    status,
                    error_body
                );
            }

            let batch_response: BatchTagsResponse = response
                .json()
                .await
                .context("failed to deserialize tag-generator batch tags response")?;

            if !batch_response.success {
                anyhow::bail!("tag-generator returned success=false");
            }

            // Convert TagResponse to TagSignal and merge into result
            for (article_id, tags) in batch_response.tags {
                let signals: Vec<TagSignal> = tags
                    .into_iter()
                    .map(|tag| {
                        let updated_at = DateTime::parse_from_rfc3339(&tag.updated_at)
                            .ok()
                            .map(|dt| dt.with_timezone(&Utc));

                        TagSignal::new(tag.tag, tag.confidence, Some(tag.source), updated_at)
                    })
                    .collect();

                result.insert(article_id, signals);
            }
        }

        debug!(
            count = result.len(),
            total_requested = article_ids.len(),
            "fetched tags for articles",
        );

        Ok(result)
    }

    /// テキストからセマンティックタグを抽出する。
    ///
    /// recap出力（summary + bullets）のテキストに対してtag-generatorの
    /// KeyBERT抽出を適用し、タグ名のリストを返す。
    ///
    /// # Arguments
    /// * `title` - タイトル（ジャンル名など）
    /// * `content` - コンテンツ（サマリー + バレット）
    ///
    /// # Errors
    /// HTTPリクエストまたはレスポンスのパースに失敗した場合はエラーを返します。
    pub(crate) async fn extract_tags(&self, title: &str, content: &str) -> Result<Vec<String>> {
        let url = self
            .base_url
            .join("api/v1/extract-tags")
            .context("failed to build extract-tags URL")?;

        let request_body = ExtractTagsRequest {
            title: title.to_string(),
            content: content.to_string(),
        };

        let mut request = self.client.post(url).json(&request_body);

        if let Some(ref token) = self.service_token {
            request = request.header("X-Service-Token", token);
        }

        let response = request
            .send()
            .await
            .context("tag-generator extract-tags request failed")?;

        let status = response.status();
        if !status.is_success() {
            let error_body = response.text().await.unwrap_or_default();
            anyhow::bail!(
                "tag-generator extract-tags returned error status {}: {}",
                status,
                error_body
            );
        }

        let extract_response: ExtractTagsResponse = response
            .json()
            .await
            .context("failed to deserialize tag-generator extract-tags response")?;

        if !extract_response.success {
            anyhow::bail!("tag-generator extract-tags returned success=false");
        }

        debug!(
            tag_count = extract_response.tags.len(),
            confidence = extract_response.confidence,
            "extracted tags from text"
        );

        Ok(extract_response.tags)
    }
}

/// tag-generatorのテキストベースタグ抽出リクエスト。
#[derive(Debug, Serialize)]
struct ExtractTagsRequest {
    title: String,
    content: String,
}

/// tag-generatorのテキストベースタグ抽出レスポンス。
#[derive(Debug, Deserialize)]
struct ExtractTagsResponse {
    success: bool,
    tags: Vec<String>,
    #[serde(default)]
    confidence: f64,
}

#[cfg(test)]
mod tests {
    use super::*;
    use wiremock::matchers::{body_json, header, method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    fn test_config(base_url: String) -> TagGeneratorConfig {
        TagGeneratorConfig {
            base_url,
            connect_timeout: Duration::from_secs(3),
            total_timeout: Duration::from_secs(30),
            service_token: Some("test-token".to_string()),
        }
    }

    #[tokio::test]
    async fn fetch_tags_batch_returns_tags() {
        let server = MockServer::start().await;

        let request_body = serde_json::json!({
            "article_ids": ["article-1", "article-2"]
        });

        let response_body = serde_json::json!({
            "success": true,
            "tags": {
                "article-1": [
                    {
                        "tag": "tech",
                        "confidence": 0.85,
                        "source": "ml_model",
                        "updated_at": "2025-11-13T12:00:00Z"
                    }
                ],
                "article-2": [
                    {
                        "tag": "ai",
                        "confidence": 0.90,
                        "source": "ml_model",
                        "updated_at": "2025-11-13T12:00:00Z"
                    }
                ]
            }
        });

        Mock::given(method("POST"))
            .and(path("/api/v1/tags/batch"))
            .and(header("X-Service-Token", "test-token"))
            .and(body_json(&request_body))
            .respond_with(ResponseTemplate::new(200).set_body_json(response_body))
            .mount(&server)
            .await;

        let client =
            TagGeneratorClient::new(test_config(server.uri())).expect("client should build");
        let tags = client
            .fetch_tags_batch(&["article-1".to_string(), "article-2".to_string()])
            .await
            .expect("fetch should succeed");

        assert_eq!(tags.len(), 2);
        assert_eq!(tags.get("article-1").unwrap().len(), 1);
        assert_eq!(tags.get("article-1").unwrap()[0].label, "tech");
        assert_eq!(tags.get("article-2").unwrap().len(), 1);
        assert_eq!(tags.get("article-2").unwrap()[0].label, "ai");
    }

    #[tokio::test]
    async fn fetch_tags_batch_handles_empty_list() {
        let client = TagGeneratorClient::new(test_config("http://localhost:8000".to_string()))
            .expect("client should build");
        let tags = client
            .fetch_tags_batch(&[])
            .await
            .expect("fetch should succeed");

        assert_eq!(tags.len(), 0);
    }

    #[tokio::test]
    async fn extract_tags_returns_tags() {
        let server = MockServer::start().await;

        let response_body = serde_json::json!({
            "success": true,
            "tags": ["artificial-intelligence", "machine-learning", "gpu-computing"],
            "confidence": 0.85,
            "inference_ms": 150.0,
            "language": "en"
        });

        Mock::given(method("POST"))
            .and(path("/api/v1/extract-tags"))
            .and(header("X-Service-Token", "test-token"))
            .respond_with(ResponseTemplate::new(200).set_body_json(response_body))
            .mount(&server)
            .await;

        let client =
            TagGeneratorClient::new(test_config(server.uri())).expect("client should build");
        let tags = client
            .extract_tags("Technology", "AI and ML are transforming GPU computing")
            .await
            .expect("extract should succeed");

        assert_eq!(tags.len(), 3);
        assert!(tags.contains(&"artificial-intelligence".to_string()));
        assert!(tags.contains(&"machine-learning".to_string()));
    }

    #[tokio::test]
    async fn extract_tags_handles_server_error() {
        let server = MockServer::start().await;

        Mock::given(method("POST"))
            .and(path("/api/v1/extract-tags"))
            .respond_with(ResponseTemplate::new(503).set_body_string("service unavailable"))
            .mount(&server)
            .await;

        let client =
            TagGeneratorClient::new(test_config(server.uri())).expect("client should build");
        let result = client.extract_tags("Test", "content").await;

        assert!(result.is_err());
    }
}
