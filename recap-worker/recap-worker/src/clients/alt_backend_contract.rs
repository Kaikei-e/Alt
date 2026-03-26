//! Consumer-Driven Contract tests for recap-worker → alt-backend.
//!
//! Verifies the GET /v1/recap/articles paginated endpoint contract.

use pact_consumer::prelude::*;
use reqwest::Client;
use serde::Deserialize;

#[allow(dead_code)]
#[derive(Debug, Deserialize)]
struct RecapArticlesResponse {
    total: i32,
    has_more: bool,
    articles: Vec<serde_json::Value>,
}

const PACT_DIR: &str = "../../pacts";

/// Paginated article fetch: GET /v1/recap/articles → 200 OK
#[tokio::test]
#[ignore = "CDC contract test"]
async fn contract_alt_backend_recap_articles() {
    let pact = PactBuilder::new("recap-worker", "alt-backend")
        .interaction("a paginated recap articles request", "", |mut i| {
            i.given("articles exist in the recap window");
            i.request.method("GET");
            i.request.path("/v1/recap/articles");
            i.request.query_param("from", "2026-03-19T00:00:00Z");
            i.request.query_param("to", "2026-03-26T00:00:00Z");
            i.request.query_param("page", "1");
            i.request.query_param("page_size", "500");
            i.response.status(200);
            i.response.content_type("application/json");
            i.response.json_body(json_pattern!({
                "range": json_pattern!({
                    "from": like!("2026-03-19T00:00:00Z"),
                    "to": like!("2026-03-26T00:00:00Z"),
                }),
                "total": like!(42i64),
                "page": like!(1i64),
                "page_size": like!(500i64),
                "has_more": like!(false),
                "articles": each_like!(json_pattern!({
                    "article_id": like!("art-001"),
                    "title": like!("Test Article Title"),
                    "fulltext": like!("Full article text content here."),
                    "tags": each_like!(json_pattern!({
                        "label": like!("technology"),
                    })),
                })),
            }));
            i
        })
        .with_output_dir(PACT_DIR)
        .start_mock_server(None, None);

    let url = pact.path("/v1/recap/articles");

    let resp = Client::new()
        .get(url)
        .query(&[
            ("from", "2026-03-19T00:00:00Z"),
            ("to", "2026-03-26T00:00:00Z"),
            ("page", "1"),
            ("page_size", "500"),
        ])
        .send()
        .await
        .expect("request should succeed");

    assert_eq!(resp.status(), 200);
    let parsed: RecapArticlesResponse = resp.json().await.expect("should parse response");
    assert!(!parsed.articles.is_empty());
    assert!(parsed.total > 0);
}
