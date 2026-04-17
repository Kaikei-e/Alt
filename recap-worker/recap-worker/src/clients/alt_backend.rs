/// alt-backend:9000からの記事取得クライアント。
///
/// ページング、タイムアウト、再試行をサポートします。
use std::time::Duration;

use anyhow::{Context, Result};
use chrono::{DateTime, Utc};
use reqwest::{Client, Url};
use serde::{Deserialize, Serialize};
use tracing::debug;

/// alt-backendから取得した記事の構造。
///
/// Connect-RPC の JSON wire format は camelCase を使うため、proto フィールドの
/// snake_case (e.g. `article_id`) は JSON 上 `articleId` として現れる。
#[derive(Debug, Clone, PartialEq, Deserialize)]
#[serde(rename_all = "camelCase")]
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
#[serde(rename_all = "camelCase")]
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
#[serde(rename_all = "camelCase")]
#[allow(dead_code)] // total, page, page_size は将来使用する可能性があるため
struct RecapArticlesResponse {
    total: i32,
    page: i32,
    page_size: i32,
    has_more: bool,
    articles: Vec<AltBackendArticle>,
}

/// ListRecapArticles Connect-RPC リクエスト。
///
/// Connect-RPC unary は POST + JSON body で wire 化される。protojson は
/// proto field の snake_case を camelCase に変換するため、ここでも同じ
/// エンコーディングを採用する。
#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct ListRecapArticlesRequest {
    from: String,
    to: String,
    page: i32,
    page_size: i32,
}

/// Connect-RPC ListRecapArticles RPC のフルパス。
const LIST_RECAP_ARTICLES_PATH: &str = "alt.recap.v2.RecapService/ListRecapArticles";

/// alt-backendクライアントの設定。
#[derive(Debug, Clone)]
pub(crate) struct AltBackendConfig {
    pub(crate) base_url: String,
    pub(crate) connect_timeout: Duration,
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
        Self::new_with_client(config, client)
    }

    /// 外部で構築済みの `reqwest::Client` を用いてクライアントを作成する。
    /// mTLS 経路では、identity と root cert を既に反映した client を注入する。
    ///
    /// # Errors
    /// URL のパースに失敗した場合はエラーを返します。
    pub(crate) fn new_with_client(config: AltBackendConfig, client: Client) -> Result<Self> {
        let base_url = Url::parse(&config.base_url).context("invalid alt-backend base URL")?;
        Ok(Self {
            client,
            base_url,
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

    /// 単一ページの記事を取得する (Connect-RPC unary, JSON body)。
    async fn fetch_page(
        &self,
        from: DateTime<Utc>,
        to: DateTime<Utc>,
        page: i32,
    ) -> Result<RecapArticlesResponse> {
        let url = self
            .base_url
            .join(LIST_RECAP_ARTICLES_PATH)
            .context("failed to build recap articles Connect-RPC URL")?;

        let body = ListRecapArticlesRequest {
            from: from.to_rfc3339(),
            to: to.to_rfc3339(),
            page,
            page_size: 500,
        };

        // Auth is established at the TLS transport layer (mTLS).
        let response = self
            .client
            .post(url)
            .header("Content-Type", "application/json")
            .json(&body)
            .send()
            .await
            .context("alt-backend articles request failed")?;

        let status = response.status();

        if !status.is_success() {
            let error_body = response.text().await.unwrap_or_default();
            anyhow::bail!("alt-backend returned error status {status}: {error_body}");
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
    use wiremock::matchers::{body_json, method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    fn test_config(base_url: String) -> AltBackendConfig {
        AltBackendConfig {
            base_url,
            connect_timeout: Duration::from_secs(3),
            total_timeout: Duration::from_secs(30),
        }
    }

    const RPC_PATH: &str = "/alt.recap.v2.RecapService/ListRecapArticles";

    #[tokio::test]
    async fn fetch_articles_returns_single_page() {
        let server = MockServer::start().await;
        let from = Utc::now();
        let to = Utc::now();

        // Connect-RPC JSON uses camelCase for proto snake_case fields.
        let body = serde_json::json!({
            "range": {
                "from": from.to_rfc3339(),
                "to": to.to_rfc3339()
            },
            "total": 1,
            "page": 1,
            "pageSize": 500,
            "hasMore": false,
            "articles": [
                {
                    "articleId": "art-1",
                    "title": "Article 1",
                    "fulltext": "Content 1",
                    "publishedAt": "2025-01-01T00:00:00Z",
                    "sourceUrl": "https://example.com/1",
                    "langHint": "en"
                }
            ]
        });

        Mock::given(method("POST"))
            .and(path(RPC_PATH))
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

        let body1 = serde_json::json!({
            "range": {"from": from.to_rfc3339(), "to": to.to_rfc3339()},
            "total": 2,
            "page": 1,
            "pageSize": 1,
            "hasMore": true,
            "articles": [{"articleId": "art-1", "title": "Article 1", "fulltext": "C1", "publishedAt": null, "sourceUrl": null, "langHint": "en"}]
        });

        let expected_req1 = serde_json::json!({
            "from": from.to_rfc3339(),
            "to": to.to_rfc3339(),
            "page": 1,
            "pageSize": 500,
        });

        Mock::given(method("POST"))
            .and(path(RPC_PATH))
            .and(body_json(&expected_req1))
            .respond_with(ResponseTemplate::new(200).set_body_json(body1))
            .mount(&server)
            .await;

        let body2 = serde_json::json!({
            "range": {"from": from.to_rfc3339(), "to": to.to_rfc3339()},
            "total": 2,
            "page": 2,
            "pageSize": 1,
            "hasMore": false,
            "articles": [{"articleId": "art-2", "title": "Article 2", "fulltext": "C2", "publishedAt": null, "sourceUrl": null, "langHint": "ja"}]
        });

        let expected_req2 = serde_json::json!({
            "from": from.to_rfc3339(),
            "to": to.to_rfc3339(),
            "page": 2,
            "pageSize": 500,
        });

        Mock::given(method("POST"))
            .and(path(RPC_PATH))
            .and(body_json(&expected_req2))
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
