/// tag-generator `/api/v1/extract-tags` クライアント。
///
/// recap genre ごとに要約本文からセマンティックタグを抽出する用途に限定。
/// 旧 `/api/v1/tags/batch` 経路は ADR-000241 / ADR-000397 (Shared Database
/// 廃止) に伴い alt-backend `BatchGetTagsByArticleIDs` に移行した。
use std::time::Duration;

use anyhow::{Context, Result};
use reqwest::{Client, Url};
use serde::{Deserialize, Serialize};
use tracing::debug;

/// tag-generatorクライアントの設定。
#[derive(Debug, Clone)]
pub(crate) struct TagGeneratorConfig {
    pub(crate) base_url: String,
    pub(crate) connect_timeout: Duration,
    pub(crate) total_timeout: Duration,
}

/// tag-generatorとの通信を管理するクライアント。
#[derive(Debug, Clone)]
pub(crate) struct TagGeneratorClient {
    client: Client,
    base_url: Url,
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
        Ok(Self { client, base_url })
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

        // Auth is established at the TLS transport layer (mTLS).
        let request = self.client.post(url).json(&request_body);

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
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    fn test_config(base_url: String) -> TagGeneratorConfig {
        TagGeneratorConfig {
            base_url,
            connect_timeout: Duration::from_secs(3),
            total_timeout: Duration::from_secs(30),
        }
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
