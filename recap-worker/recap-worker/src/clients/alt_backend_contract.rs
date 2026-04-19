//! Consumer-Driven Contract tests for recap-worker → alt-backend.
//!
//! Verifies the Connect-RPC `ListRecapArticles` paginated endpoint on
//! `BackendInternalService`. Service-to-service endpoint — auth is
//! established at the mTLS transport layer, no user token required.
//! Path: POST `/services.backend.v1.BackendInternalService/ListRecapArticles`, JSON body.

use pact_consumer::prelude::*;
use reqwest::Client;
use serde::Deserialize;

#[allow(dead_code)]
#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ListRecapArticlesResponse {
    total: i32,
    has_more: bool,
    articles: Vec<serde_json::Value>,
}

const PACT_DIR: &str = "../../pacts";

/// Paginated article fetch: POST /services.backend.v1.BackendInternalService/ListRecapArticles → 200 OK
#[tokio::test]
#[ignore = "CDC contract test"]
async fn contract_alt_backend_recap_articles() {
    let pact = PactBuilder::new("recap-worker", "alt-backend")
        .interaction("a paginated recap articles request", "", |mut i| {
            i.given("articles exist in the recap window");
            i.request.method("POST");
            i.request
                .path("/services.backend.v1.BackendInternalService/ListRecapArticles");
            i.request.content_type("application/json");
            i.request.json_body(json_pattern!({
                "from": like!("2026-03-19T00:00:00Z"),
                "to": like!("2026-03-26T00:00:00Z"),
                "page": like!(1i64),
                "pageSize": like!(500i64),
            }));
            i.response.status(200);
            i.response.content_type("application/json");
            i.response.json_body(json_pattern!({
                "range": json_pattern!({
                    "from": like!("2026-03-19T00:00:00Z"),
                    "to": like!("2026-03-26T00:00:00Z"),
                }),
                "total": like!(42i64),
                "page": like!(1i64),
                "pageSize": like!(500i64),
                "hasMore": like!(false),
                "articles": each_like!(json_pattern!({
                    "articleId": like!("art-001"),
                    "title": like!("Test Article Title"),
                    "fulltext": like!("Full article text content here."),
                })),
            }));
            i
        })
        .with_output_dir(PACT_DIR)
        .start_mock_server(None, None);

    let url = pact.path("/services.backend.v1.BackendInternalService/ListRecapArticles");
    let body = serde_json::json!({
        "from": "2026-03-19T00:00:00Z",
        "to": "2026-03-26T00:00:00Z",
        "page": 1,
        "pageSize": 500,
    });

    let resp = Client::new()
        .post(url)
        .header("Content-Type", "application/json")
        .json(&body)
        .send()
        .await
        .expect("request should succeed");

    assert_eq!(resp.status(), 200);
    let parsed: ListRecapArticlesResponse = resp.json().await.expect("should parse response");
    assert!(!parsed.articles.is_empty());
    assert!(parsed.total > 0);
}

#[allow(dead_code)]
#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct BatchGetTagsByArticleIDsResponse {
    items: Vec<serde_json::Value>,
}

/// Batch tag fetch: POST /services.backend.v1.BackendInternalService/BatchGetTagsByArticleIDs → 200 OK
///
/// Replaces the former `recap-worker → tag-generator /api/v1/tags/batch`
/// contract per ADR-000241 / ADR-000397 (Shared Database anti-pattern
/// elimination; alt-backend is the sole data owner of articles /
/// article_tags / feed_tags).
#[tokio::test]
#[ignore = "CDC contract test"]
async fn contract_alt_backend_batch_get_tags_by_article_ids() {
    let pact = PactBuilder::new("recap-worker", "alt-backend")
        .interaction("a batch tags request by article ids", "", |mut i| {
            i.given("tags exist for the requested articles");
            i.request.method("POST");
            i.request
                .path("/services.backend.v1.BackendInternalService/BatchGetTagsByArticleIDs");
            i.request.content_type("application/json");
            i.request.json_body(json_pattern!({
                "articleIds": each_like!(like!("art-001")),
            }));
            i.response.status(200);
            i.response.content_type("application/json");
            i.response.json_body(json_pattern!({
                "items": each_like!(json_pattern!({
                    "articleId": like!("art-001"),
                    "tags": each_like!(json_pattern!({
                        "tagName": like!("technology"),
                        "confidence": like!(0.95f64),
                        "source": like!("ml_model"),
                        "updatedAt": like!("2026-03-26T00:00:00Z"),
                    })),
                })),
            }));
            i
        })
        .with_output_dir(PACT_DIR)
        .start_mock_server(None, None);

    let url = pact.path("/services.backend.v1.BackendInternalService/BatchGetTagsByArticleIDs");
    let body = serde_json::json!({ "articleIds": ["art-001"] });

    let resp = Client::new()
        .post(url)
        .header("Content-Type", "application/json")
        .json(&body)
        .send()
        .await
        .expect("request should succeed");

    assert_eq!(resp.status(), 200);
    let parsed: BatchGetTagsByArticleIDsResponse =
        resp.json().await.expect("should parse response");
    assert!(!parsed.items.is_empty());
}
