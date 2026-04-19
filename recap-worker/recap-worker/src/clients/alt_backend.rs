/// alt-backend:9000からの記事取得クライアント。
///
/// ページング、タイムアウト、再試行をサポートします。
use std::collections::HashMap;
use std::time::Duration;

use anyhow::{Context, Result};
use chrono::{DateTime, Utc};
use reqwest::{Client, Url};
use serde::{Deserialize, Serialize};
use tracing::debug;

use crate::pipeline::tag_signal::TagSignal;

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
///
/// protojson は zero-valued フィールドを省略するため、`total`・`page`・
/// `page_size`・`has_more` は欠落した JSON でも受理できるよう
/// `#[serde(default)]` を付与する。
#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
#[allow(dead_code)] // total, page, page_size は将来使用する可能性があるため
struct RecapArticlesResponse {
    #[serde(default)]
    total: i32,
    #[serde(default)]
    page: i32,
    #[serde(default)]
    page_size: i32,
    #[serde(default)]
    has_more: bool,
    #[serde(default)]
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

/// Connect-RPC ListRecapArticles RPC のフルパス
/// (service-to-service RPC なので BackendInternalService に居る)。
const LIST_RECAP_ARTICLES_PATH: &str =
    "services.backend.v1.BackendInternalService/ListRecapArticles";

/// Connect-RPC BatchGetTagsByArticleIDs RPC path. Replaces the legacy
/// tag-generator /api/v1/tags/batch surface per ADR-000241 / ADR-000397.
const BATCH_GET_TAGS_BY_ARTICLE_IDS_PATH: &str =
    "services.backend.v1.BackendInternalService/BatchGetTagsByArticleIDs";

/// Server-side invariant mirrored here so the chunked loop below never
/// sends more than what the provider enforces.
const BATCH_GET_TAGS_MAX_BATCH_SIZE: usize = 1000;

/// Request shape for BatchGetTagsByArticleIDs. protojson uses camelCase.
#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct BatchGetTagsByArticleIDsRequest {
    article_ids: Vec<String>,
}

/// Response shape for BatchGetTagsByArticleIDs. `items` may be absent
/// when there are no tagged articles in the batch (protojson omits
/// zero-valued fields).
#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct BatchGetTagsByArticleIDsResponse {
    #[serde(default)]
    items: Vec<ArticleTagsEntry>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ArticleTagsEntry {
    article_id: String,
    #[serde(default)]
    tags: Vec<ArticleTagEntry>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ArticleTagEntry {
    tag_name: String,
    #[serde(default)]
    confidence: f32,
    #[serde(default)]
    source: String,
    #[serde(default)]
    updated_at: Option<DateTime<Utc>>,
}

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

    /// 記事IDの配列に紐づくタグをバッチ取得する (Connect-RPC unary, JSON body)。
    ///
    /// 旧 tag-generator `/api/v1/tags/batch` の置き換え (ADR-000241 /
    /// ADR-000397)。alt-backend を唯一のデータオーナーとして、articles /
    /// article_tags / feed_tags JOIN を alt-backend 側で実行する。
    ///
    /// 空入力では HTTP 呼び出しを行わず空 map を返す。1000件を超える ids
    /// は `BATCH_GET_TAGS_MAX_BATCH_SIZE` チャンクで分割送信する。
    ///
    /// # Errors
    /// HTTP リクエストまたはレスポンスのデシリアライズに失敗した場合。
    pub(crate) async fn batch_get_tags_by_article_ids(
        &self,
        article_ids: &[String],
    ) -> Result<HashMap<String, Vec<TagSignal>>> {
        if article_ids.is_empty() {
            return Ok(HashMap::new());
        }

        let url = self
            .base_url
            .join(BATCH_GET_TAGS_BY_ARTICLE_IDS_PATH)
            .context("failed to build batch tags Connect-RPC URL")?;

        let mut result: HashMap<String, Vec<TagSignal>> = HashMap::new();

        for chunk in article_ids.chunks(BATCH_GET_TAGS_MAX_BATCH_SIZE) {
            debug!(
                chunk_size = chunk.len(),
                total = article_ids.len(),
                "fetching tags in batch chunk via alt-backend"
            );

            let body = BatchGetTagsByArticleIDsRequest {
                article_ids: chunk.to_vec(),
            };

            // Auth is established at the TLS transport layer (mTLS).
            let response = self
                .client
                .post(url.clone())
                .header("Content-Type", "application/json")
                .json(&body)
                .send()
                .await
                .context("alt-backend batch tags request failed")?;

            let status = response.status();
            if !status.is_success() {
                let error_body = response.text().await.unwrap_or_default();
                anyhow::bail!("alt-backend returned error status {status}: {error_body}");
            }

            let batch: BatchGetTagsByArticleIDsResponse = response
                .json()
                .await
                .context("failed to deserialize alt-backend batch tags response")?;

            for entry in batch.items {
                let signals: Vec<TagSignal> = entry
                    .tags
                    .into_iter()
                    .map(|t| {
                        let source = if t.source.is_empty() {
                            None
                        } else {
                            Some(t.source)
                        };
                        TagSignal::new(t.tag_name, t.confidence, source, t.updated_at)
                    })
                    .collect();
                result.insert(entry.article_id, signals);
            }
        }

        debug!(
            count = result.len(),
            total_requested = article_ids.len(),
            "fetched tags for articles via alt-backend",
        );
        Ok(result)
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

    const RPC_PATH: &str = "/services.backend.v1.BackendInternalService/ListRecapArticles";
    const BATCH_TAGS_RPC_PATH: &str =
        "/services.backend.v1.BackendInternalService/BatchGetTagsByArticleIDs";

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

    #[tokio::test]
    async fn batch_get_tags_by_article_ids_returns_tags() {
        let server = MockServer::start().await;

        let expected_req = serde_json::json!({
            "articleIds": ["a1", "a2"]
        });

        let body = serde_json::json!({
            "items": [
                {
                    "articleId": "a1",
                    "tags": [
                        {
                            "tagName": "technology",
                            "confidence": 0.95,
                            "source": "ml_model",
                            "updatedAt": "2026-03-26T00:00:00Z"
                        }
                    ]
                },
                {
                    "articleId": "a2",
                    "tags": [
                        {
                            "tagName": "ai",
                            "confidence": 0.9,
                            "source": "",
                            "updatedAt": "2026-03-26T00:00:00Z"
                        }
                    ]
                }
            ]
        });

        Mock::given(method("POST"))
            .and(path(BATCH_TAGS_RPC_PATH))
            .and(body_json(&expected_req))
            .respond_with(ResponseTemplate::new(200).set_body_json(body))
            .mount(&server)
            .await;

        let client = AltBackendClient::new(test_config(server.uri())).expect("client should build");
        let tags = client
            .batch_get_tags_by_article_ids(&["a1".to_string(), "a2".to_string()])
            .await
            .expect("batch tags should succeed");

        assert_eq!(tags.len(), 2);
        let a1 = tags.get("a1").expect("a1 present");
        assert_eq!(a1.len(), 1);
        assert_eq!(a1[0].label, "technology");
        assert!((a1[0].confidence - 0.95).abs() < 1e-4);
        assert_eq!(a1[0].source.as_deref(), Some("ml_model"));

        let a2 = tags.get("a2").expect("a2 present");
        assert_eq!(a2.len(), 1);
        // Empty source string collapses to None to match the existing
        // TagSignal semantics used by alt-backend ListRecapArticles tags.
        assert_eq!(a2[0].source, None);
    }

    #[tokio::test]
    async fn batch_get_tags_by_article_ids_handles_empty_list() {
        let client = AltBackendClient::new(test_config("http://localhost:0".to_string()))
            .expect("client should build");
        let tags = client
            .batch_get_tags_by_article_ids(&[])
            .await
            .expect("empty call short-circuits");
        assert!(tags.is_empty());
    }

    #[tokio::test]
    async fn batch_get_tags_by_article_ids_propagates_server_error() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path(BATCH_TAGS_RPC_PATH))
            .respond_with(ResponseTemplate::new(500).set_body_string("boom"))
            .mount(&server)
            .await;

        let client = AltBackendClient::new(test_config(server.uri())).expect("client should build");
        let err = client
            .batch_get_tags_by_article_ids(&["a1".to_string()])
            .await
            .expect_err("should fail on 500");
        assert!(err.to_string().contains("error status"));
    }
}
