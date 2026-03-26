//! Consumer-Driven Contract tests for recap-worker → tag-generator.
//!
//! Verifies batch tag fetch and tag extraction endpoint contracts.

use pact_consumer::prelude::*;
use reqwest::Client;
use serde::Deserialize;
use serde_json::json;
use std::collections::HashMap;

#[derive(Debug, Deserialize)]
struct BatchTagsResponse {
    success: bool,
    tags: HashMap<String, Vec<serde_json::Value>>,
}

#[derive(Debug, Deserialize)]
struct ExtractTagsResponse {
    success: bool,
    tags: Vec<String>,
}

const PACT_DIR: &str = "../../pacts";

/// Batch tag fetch: POST /api/v1/tags/batch → 200 OK
#[tokio::test]
#[ignore = "CDC contract test"]
async fn contract_tag_generator_batch_tags() {
    let pact = PactBuilder::new("recap-worker", "tag-generator")
        .interaction("a batch tags request", "", |mut i| {
            i.given("tags exist for the requested articles");
            i.request.method("POST");
            i.request.path("/api/v1/tags/batch");
            i.request.content_type("application/json");
            i.request.json_body(json_pattern!({
                "article_ids": each_like!(like!("art-001")),
            }));
            i.response.status(200);
            i.response.content_type("application/json");
            i.response.json_body(json_pattern!({
                "success": like!(true),
                "tags": json_pattern!({
                    "art-001": each_like!(json_pattern!({
                        "tag": like!("technology"),
                        "confidence": like!(0.95f64),
                        "source": like!("classifier"),
                        "updated_at": like!("2026-03-26T00:00:00Z"),
                    })),
                }),
            }));
            i
        })
        .with_output_dir(PACT_DIR)
        .start_mock_server(None, None);

    let url = pact.path("/api/v1/tags/batch");
    let body = json!({"article_ids": ["art-001"]});

    let resp = Client::new()
        .post(url)
        .json(&body)
        .send()
        .await
        .expect("request should succeed");

    assert_eq!(resp.status(), 200);
    let parsed: BatchTagsResponse = resp.json().await.expect("should parse response");
    assert!(parsed.success);
    assert!(parsed.tags.contains_key("art-001"));
}

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
