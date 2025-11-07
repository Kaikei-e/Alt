/// alt-backend:9000からの記事取得クライアント。
///
/// ページング、タイムアウト、再試行をサポートします。
use std::time::Duration;

use anyhow::{Context, Result};
use chrono::{DateTime, Utc};
use reqwest::{Client, StatusCode, Url};
use serde::{Deserialize, Serialize};
use tracing::{debug, warn};
use uuid::Uuid;

/// alt-backendから取得した記事の構造。
#[derive(Debug, Clone, PartialEq, Deserialize)]
pub(crate) struct AltBackendArticle {
    pub(crate) id: String,
    pub(crate) title: Option<String>,
    pub(crate) content: String,
    pub(crate) published_at: Option<DateTime<Utc>>,
    pub(crate) source_url: Option<String>,
    pub(crate) lang: Option<String>,
}

/// alt-backendのページング付き応答。
#[derive(Debug, Deserialize)]
struct ArticlesResponse {
    data: Vec<AltBackendArticle>,
    next_cursor: Option<String>,
}

/// alt-backendクライアントの設定。
#[derive(Debug, Clone)]
pub(crate) struct AltBackendConfig {
    pub(crate) base_url: String,
    pub(crate) connect_timeout: Duration,
    pub(crate) read_timeout: Duration,
    pub(crate) total_timeout: Duration,
}

/// alt-backendとの通信を管理するクライアント。
#[derive(Debug, Clone)]
pub(crate) struct AltBackendClient {
    client: Client,
    base_url: Url,
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

        let base_url = Url::parse(&config.base_url)
            .context("invalid alt-backend base URL")?;

        Ok(Self { client, base_url })
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
        let mut cursor: Option<String> = None;
        let mut page_count = 0;

        loop {
            page_count += 1;
            debug!(page = page_count, cursor = ?cursor, "fetching articles page");

            let page = self.fetch_page(from, to, cursor.as_deref()).await?;
            let articles_count = page.data.len();

            all_articles.extend(page.data);

            debug!(
                page = page_count,
                articles = articles_count,
                total = all_articles.len(),
                "fetched articles page"
            );

            if page.next_cursor.is_none() {
                break;
            }

            cursor = page.next_cursor;
        }

        Ok(all_articles)
    }

    /// 単一ページの記事を取得する。
    async fn fetch_page(
        &self,
        from: DateTime<Utc>,
        to: DateTime<Utc>,
        cursor: Option<&str>,
    ) -> Result<ArticlesResponse> {
        let mut url = self
            .base_url
            .join("v1/articles")
            .context("failed to build articles URL")?;

        // クエリパラメータを構築
        {
            let mut query_pairs = url.query_pairs_mut();
            query_pairs.append_pair("from", &from.to_rfc3339());
            query_pairs.append_pair("to", &to.to_rfc3339());
            query_pairs.append_pair("fields", "id,title,content,published_at,source_url,lang");
            query_pairs.append_pair("limit", "1000");

            if let Some(c) = cursor {
                query_pairs.append_pair("cursor", c);
            }
        }

        let response = self
            .client
            .get(url.clone())
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
            .json::<ArticlesResponse>()
            .await
            .context("failed to deserialize alt-backend articles response")
    }

    /// ヘルスチェックエンドポイントを呼び出す。
    ///
    /// # Errors
    /// リクエストが失敗した場合、またはサーバーがエラー状態を返した場合はエラーを返します。
    pub(crate) async fn ping(&self) -> Result<()> {
        let url = self
            .base_url
            .join("v1/health")
            .context("failed to build health URL")?;

        self.client
            .get(url)
            .send()
            .await
            .context("alt-backend health request failed")?
            .error_for_status()
            .context("alt-backend health endpoint returned error status")?;

        Ok(())
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
            read_timeout: Duration::from_secs(20),
            total_timeout: Duration::from_secs(30),
        }
    }

    #[tokio::test]
    async fn ping_succeeds_for_ok_response() {
        let server = MockServer::start().await;
        Mock::given(method("GET"))
            .and(path("/v1/health"))
            .respond_with(ResponseTemplate::new(200))
            .mount(&server)
            .await;

        let client = AltBackendClient::new(test_config(server.uri())).expect("client should build");

        client.ping().await.expect("ping should succeed");
    }

    #[tokio::test]
    async fn fetch_articles_returns_single_page() {
        let server = MockServer::start().await;
        let from = Utc::now();
        let to = Utc::now();

        let body = serde_json::json!({
            "data": [
                {
                    "id": "art-1",
                    "title": "Article 1",
                    "content": "Content 1",
                    "published_at": "2025-01-01T00:00:00Z",
                    "source_url": "https://example.com/1",
                    "lang": "en"
                }
            ],
            "next_cursor": null
        });

        Mock::given(method("GET"))
            .and(path("/v1/articles"))
            .respond_with(ResponseTemplate::new(200).set_body_json(body))
            .mount(&server)
            .await;

        let client = AltBackendClient::new(test_config(server.uri())).expect("client should build");
        let articles = client
            .fetch_articles(from, to)
            .await
            .expect("fetch should succeed");

        assert_eq!(articles.len(), 1);
        assert_eq!(articles[0].id, "art-1");
        assert_eq!(articles[0].title.as_deref(), Some("Article 1"));
    }

    #[tokio::test]
    async fn fetch_articles_paginates_multiple_pages() {
        let server = MockServer::start().await;
        let from = Utc::now();
        let to = Utc::now();

        // First page
        let body1 = serde_json::json!({
            "data": [{"id": "art-1", "title": "Article 1", "content": "C1", "published_at": null, "source_url": null, "lang": "en"}],
            "next_cursor": "cursor-2"
        });

        Mock::given(method("GET"))
            .and(path("/v1/articles"))
            .and(query_param("cursor", "cursor-2").not())
            .respond_with(ResponseTemplate::new(200).set_body_json(body1))
            .mount(&server)
            .await;

        // Second page
        let body2 = serde_json::json!({
            "data": [{"id": "art-2", "title": "Article 2", "content": "C2", "published_at": null, "source_url": null, "lang": "ja"}],
            "next_cursor": null
        });

        Mock::given(method("GET"))
            .and(path("/v1/articles"))
            .and(query_param("cursor", "cursor-2"))
            .respond_with(ResponseTemplate::new(200).set_body_json(body2))
            .mount(&server)
            .await;

        let client = AltBackendClient::new(test_config(server.uri())).expect("client should build");
        let articles = client
            .fetch_articles(from, to)
            .await
            .expect("fetch should succeed");

        assert_eq!(articles.len(), 2);
        assert_eq!(articles[0].id, "art-1");
        assert_eq!(articles[1].id, "art-2");
    }
}

