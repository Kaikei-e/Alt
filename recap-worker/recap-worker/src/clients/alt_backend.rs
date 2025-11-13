/// alt-backend:9000からの記事取得クライアント。
///
/// ページング、タイムアウト、再試行をサポートします。
use std::time::Duration;

use anyhow::{Context, Result};
use chrono::{DateTime, Utc};
use reqwest::{Client, Url};
use serde::Deserialize;
use tracing::debug;

/// alt-backendから取得した記事の構造。
#[derive(Debug, Clone, PartialEq, Deserialize)]
pub(crate) struct AltBackendTag {
    pub(crate) label: String,
    #[serde(default)]
    pub(crate) confidence: Option<f32>,
    #[serde(default)]
    pub(crate) source: Option<String>,
    #[serde(default)]
    pub(crate) updated_at: Option<DateTime<Utc>>,
}

#[derive(Debug, Clone, PartialEq, Deserialize)]
pub(crate) struct AltBackendArticle {
    pub(crate) article_id: String,
    pub(crate) title: Option<String>,
    pub(crate) fulltext: String,
    pub(crate) published_at: Option<DateTime<Utc>>,
    pub(crate) source_url: Option<String>,
    pub(crate) lang_hint: Option<String>,
    #[serde(default)]
    pub(crate) tags: Vec<AltBackendTag>,
}

/// alt-backendのページング付き応答。
#[derive(Debug, Deserialize)]
#[allow(dead_code)] // total, page, page_size は将来使用する可能性があるため
struct RecapArticlesResponse {
    // range: RecapRange,  // 不要なので省略
    total: i32,
    page: i32,
    page_size: i32,
    has_more: bool,
    articles: Vec<AltBackendArticle>,
}

/// alt-backendクライアントの設定。
#[derive(Debug, Clone)]
pub(crate) struct AltBackendConfig {
    pub(crate) base_url: String,
    pub(crate) connect_timeout: Duration,
    pub(crate) total_timeout: Duration,
    pub(crate) service_token: Option<String>,
}

/// alt-backendとの通信を管理するクライアント。
#[derive(Debug, Clone)]
pub(crate) struct AltBackendClient {
    client: Client,
    base_url: Url,
    service_token: Option<String>,
}

impl AltBackendClient {
    /// 新しいalt-backendクライアントを作成する。
    ///
    /// # Errors
    /// URLのパースまたはHTTPクライアントの構築に失敗した場合はエラーを返します。
    pub(crate) fn new(config: AltBackendConfig) -> Result<Self> {
        let client = Client::builder()
            .connect_timeout(config.connect_timeout)
            .timeout(config.total_timeout)
            .build()
            .context("failed to build alt-backend HTTP client")?;

        let base_url = Url::parse(&config.base_url).context("invalid alt-backend base URL")?;

        Ok(Self {
            client,
            base_url,
            service_token: config.service_token,
        })
    }

    /// 指定された期間の記事を全件取得する。
    ///
    /// ページングを自動的に処理し、すべての記事を返します。
    ///
    /// # Arguments
    /// * `from` - 取得開始日時
    /// * `to` - 取得終了日時
    ///
    /// # Errors
    /// HTTPリクエストまたはレスポンスのパースに失敗した場合はエラーを返します。
    pub(crate) async fn fetch_articles(
        &self,
        from: DateTime<Utc>,
        to: DateTime<Utc>,
    ) -> Result<Vec<AltBackendArticle>> {
        let mut all_articles = Vec::new();
        let mut current_page = 1;

        loop {
            debug!(page = current_page, "fetching articles page");

            let response = self.fetch_page(from, to, current_page).await?;
            let articles_count = response.articles.len();

            all_articles.extend(response.articles);

            debug!(
                page = current_page,
                articles = articles_count,
                total = all_articles.len(),
                has_more = response.has_more,
                "fetched articles page"
            );

            if !response.has_more {
                break;
            }

            current_page += 1;
        }

        Ok(all_articles)
    }

    /// 単一ページの記事を取得する。
    async fn fetch_page(
        &self,
        from: DateTime<Utc>,
        to: DateTime<Utc>,
        page: i32,
    ) -> Result<RecapArticlesResponse> {
        let mut url = self
            .base_url
            .join("v1/recap/articles")
            .context("failed to build recap articles URL")?;

        // クエリパラメータを構築
        {
            let mut query_pairs = url.query_pairs_mut();
            query_pairs.append_pair("from", &from.to_rfc3339());
            query_pairs.append_pair("to", &to.to_rfc3339());
            query_pairs.append_pair("page", &page.to_string());
            query_pairs.append_pair("page_size", "500");
        }

        let mut request = self.client.get(url.clone());

        // Add service authentication token if configured
        if let Some(ref token) = self.service_token {
            request = request.header("X-Service-Token", token);
        }

        let response = request
            .send()
            .await
            .context("alt-backend articles request failed")?;

        let status = response.status();

        if !status.is_success() {
            let error_body = response.text().await.unwrap_or_default();
            anyhow::bail!(
                "alt-backend returned error status {}: {}",
                status,
                error_body
            );
        }

        response
            .json::<RecapArticlesResponse>()
            .await
            .context("failed to deserialize alt-backend articles response")
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use wiremock::matchers::{method, path, query_param};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    fn test_config(base_url: String) -> AltBackendConfig {
        AltBackendConfig {
            base_url,
            connect_timeout: Duration::from_secs(3),
            total_timeout: Duration::from_secs(30),
            service_token: None,
        }
    }

    #[tokio::test]
    async fn fetch_articles_returns_single_page() {
        let server = MockServer::start().await;
        let from = Utc::now();
        let to = Utc::now();

        let body = serde_json::json!({
            "range": {
                "from": from.to_rfc3339(),
                "to": to.to_rfc3339()
            },
            "total": 1,
            "page": 1,
            "page_size": 500,
            "has_more": false,
            "articles": [
                {
                    "article_id": "art-1",
                    "title": "Article 1",
                    "fulltext": "Content 1",
                    "published_at": "2025-01-01T00:00:00Z",
                    "source_url": "https://example.com/1",
                    "lang_hint": "en"
                }
            ]
        });

        Mock::given(method("GET"))
            .and(path("/v1/recap/articles"))
            .respond_with(ResponseTemplate::new(200).set_body_json(body))
            .mount(&server)
            .await;

        let client = AltBackendClient::new(test_config(server.uri())).expect("client should build");
        let articles = client
            .fetch_articles(from, to)
            .await
            .expect("fetch should succeed");

        assert_eq!(articles.len(), 1);
        assert_eq!(articles[0].article_id, "art-1");
        assert_eq!(articles[0].title.as_deref(), Some("Article 1"));
    }

    #[tokio::test]
    async fn fetch_articles_paginates_multiple_pages() {
        let server = MockServer::start().await;
        let from = Utc::now();
        let to = Utc::now();

        // First page
        let body1 = serde_json::json!({
            "range": {"from": from.to_rfc3339(), "to": to.to_rfc3339()},
            "total": 2,
            "page": 1,
            "page_size": 1,
            "has_more": true,
            "articles": [{"article_id": "art-1", "title": "Article 1", "fulltext": "C1", "published_at": null, "source_url": null, "lang_hint": "en"}]
        });

        Mock::given(method("GET"))
            .and(path("/v1/recap/articles"))
            .and(query_param("page", "1"))
            .respond_with(ResponseTemplate::new(200).set_body_json(body1))
            .mount(&server)
            .await;

        // Second page
        let body2 = serde_json::json!({
            "range": {"from": from.to_rfc3339(), "to": to.to_rfc3339()},
            "total": 2,
            "page": 2,
            "page_size": 1,
            "has_more": false,
            "articles": [{"article_id": "art-2", "title": "Article 2", "fulltext": "C2", "published_at": null, "source_url": null, "lang_hint": "ja"}]
        });

        Mock::given(method("GET"))
            .and(path("/v1/recap/articles"))
            .and(query_param("page", "2"))
            .respond_with(ResponseTemplate::new(200).set_body_json(body2))
            .mount(&server)
            .await;

        let client = AltBackendClient::new(test_config(server.uri())).expect("client should build");
        let articles = client
            .fetch_articles(from, to)
            .await
            .expect("fetch should succeed");

        assert_eq!(articles.len(), 2);
        assert_eq!(articles[0].article_id, "art-1");
        assert_eq!(articles[1].article_id, "art-2");
    }
}
