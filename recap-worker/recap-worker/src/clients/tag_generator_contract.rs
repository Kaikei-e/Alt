//! Consumer-Driven Contract tests for recap-worker → tag-generator.
//!
//! tag-generator `/api/v1/extract-tags` の契約のみを検証する。
//! `/api/v1/tags/batch` 相互作用は ADR-000241 / ADR-000397 (Shared
//! Database anti-pattern 解消) により alt-backend 側
//! `BatchGetTagsByArticleIDs` に移行したため、この契約から除外した
//! (置換版の contract は `alt_backend_contract.rs` にある)。

use pact_consumer::prelude::*;
use reqwest::Client;
use serde::Deserialize;
use serde_json::json;

#[derive(Debug, Deserialize)]
struct ExtractTagsResponse {
    success: bool,
    tags: Vec<String>,
}

const PACT_DIR: &str = "../../pacts";

/// Tag extraction: POST /api/v1/extract-tags → 200 OK
#[tokio::test]
#[ignore = "CDC contract test"]
async fn contract_tag_generator_extract_tags() {
    let pact = PactBuilder::new("recap-worker", "tag-generator")
        .interaction("a tag extraction request", "", |mut i| {
            i.given("the tag extraction model is loaded");
            i.request.method("POST");
            i.request.path("/api/v1/extract-tags");
            i.request.content_type("application/json");
            i.request.json_body(json_pattern!({
                "title": like!("AI Advances in 2026"),
                "content": like!("Detailed article content about artificial intelligence."),
            }));
            i.response.status(200);
            i.response.content_type("application/json");
            i.response.json_body(json_pattern!({
                "success": like!(true),
                "tags": each_like!(like!("artificial-intelligence")),
                "confidence": like!(0.92f64),
            }));
            i
        })
        .with_output_dir(PACT_DIR)
        .start_mock_server(None, None);

    let url = pact.path("/api/v1/extract-tags");
    let body = json!({
        "title": "AI Advances in 2026",
        "content": "Detailed article content about artificial intelligence.",
    });

    let resp = Client::new()
        .post(url)
        .json(&body)
        .send()
        .await
        .expect("request should succeed");

    assert_eq!(resp.status(), 200);
    let parsed: ExtractTagsResponse = resp.json().await.expect("should parse response");
    assert!(parsed.success);
    assert!(!parsed.tags.is_empty());
}
