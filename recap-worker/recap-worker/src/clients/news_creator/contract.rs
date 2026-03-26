//! Consumer-Driven Contract tests for recap-worker → news-creator.
//!
//! These tests verify that recap-worker's expectations of the news-creator
//! HTTP/REST API are documented as Pact contracts. Run with:
//!   cargo test contract -- --ignored
//!
//! Generated pact files are written to `pacts/` and later verified by
//! the news-creator provider verification tests.

use pact_consumer::prelude::*;
use reqwest::Client;
use serde_json::json;

use super::models::{BatchSummaryResponse, SummaryResponse};

const PACT_DIR: &str = "../../pacts";

/// Normal summary generation: POST /v1/summary/generate → 200 OK
#[tokio::test]
#[ignore = "CDC contract test — run with `cargo test contract -- --ignored`"]
async fn contract_news_creator_summary_generate() {
    let pact = PactBuilder::new("recap-worker", "news-creator")
        .interaction("a summary generate request for tech genre", "", |mut i| {
            i.given("the LLM model is loaded and ready");
            i.request.method("POST");
            i.request.path("/v1/summary/generate");
            i.request.content_type("application/json");
            i.request.json_body(json_pattern!({
                "job_id": like!("00000000-0000-0000-0000-000000000001"),
                "genre": like!("tech"),
                "clusters": each_like!(json_pattern!({
                    "cluster_id": like!(0i64),
                    "representative_sentences": each_like!(json_pattern!({
                        "text": like!("AI advances in 2026"),
                    })),
                })),
            }));
            i.response.status(200);
            i.response.content_type("application/json");
            i.response.json_body(json_pattern!({
                "job_id": like!("00000000-0000-0000-0000-000000000001"),
                "genre": like!("tech"),
                "summary": json_pattern!({
                    "title": like!("テクノロジー週間要約"),
                    "bullets": each_like!(like!("AI関連の進展が報告された。")),
                    "language": like!("ja"),
                }),
                "metadata": json_pattern!({
                    "model": like!("gemma3:4b-it-qat"),
                }),
            }));
            i
        })
        .with_output_dir(PACT_DIR)
        .start_mock_server(None, None);

    let url = pact.path("/v1/summary/generate");
    let body = json!({
        "job_id": "00000000-0000-0000-0000-000000000001",
        "genre": "tech",
        "clusters": [{
            "cluster_id": 0,
            "representative_sentences": [{
                "text": "AI advances in 2026",
            }],
        }],
    });

    let resp = Client::new()
        .post(url)
        .json(&body)


        .send()
        .await
        .expect("request should succeed");

    assert_eq!(resp.status(), 200);
    let parsed: SummaryResponse = resp.json().await.expect("should parse as SummaryResponse");
    assert_eq!(parsed.genre, "tech");
    assert!(!parsed.summary.title.is_empty());
    assert!(!parsed.summary.bullets.is_empty());
}

/// Batch summary generation: POST /v1/summary/generate/batch → 200 OK
#[tokio::test]
#[ignore = "CDC contract test"]
async fn contract_news_creator_batch_summary_generate() {
    let pact = PactBuilder::new("recap-worker", "news-creator")
        .interaction("a batch summary generate request", "", |mut i| {
            i.given("the LLM model is loaded and ready");
            i.request.method("POST");
            i.request.path("/v1/summary/generate/batch");
            i.request.content_type("application/json");
            i.request.json_body(json_pattern!({
                "requests": each_like!(json_pattern!({
                    "job_id": like!("00000000-0000-0000-0000-000000000001"),
                    "genre": like!("tech"),
                    "clusters": each_like!(json_pattern!({
                        "cluster_id": like!(0i64),
                        "representative_sentences": each_like!(json_pattern!({
                            "text": like!("Test sentence"),
                        })),
                    })),
                })),
            }));
            i.response.status(200);
            i.response.content_type("application/json");
            i.response.json_body(json_pattern!({
                "responses": each_like!(json_pattern!({
                    "job_id": like!("00000000-0000-0000-0000-000000000001"),
                    "genre": like!("tech"),
                    "summary": json_pattern!({
                        "title": like!("Summary Title"),
                        "bullets": each_like!(like!("Bullet 1")),
                        "language": like!("ja"),
                    }),
                    "metadata": json_pattern!({
                        "model": like!("gemma3:4b-it-qat"),
                    }),
                })),
                "errors": [],
            }));
            i
        })
        .with_output_dir(PACT_DIR)
        .start_mock_server(None, None);

    let url = pact.path("/v1/summary/generate/batch");
    let body = json!({
        "requests": [{
            "job_id": "00000000-0000-0000-0000-000000000001",
            "genre": "tech",
            "clusters": [{
                "cluster_id": 0,
                "representative_sentences": [{"text": "Test sentence"}],
            }],
        }],
    });

    let resp = Client::new()
        .post(url)
        .json(&body)


        .send()
        .await
        .expect("request should succeed");

    assert_eq!(resp.status(), 200);
    let parsed: BatchSummaryResponse =
        resp.json().await.expect("should parse as BatchSummaryResponse");
    assert!(!parsed.responses.is_empty());
    assert!(parsed.errors.is_empty());
}

/// Queue full scenario: POST /v1/summary/generate → 429 Too Many Requests
#[tokio::test]
#[ignore = "CDC contract test"]
async fn contract_news_creator_summary_queue_full() {
    let pact = PactBuilder::new("recap-worker", "news-creator")
        .interaction(
            "a summary generate request when queue is full",
            "",
            |mut i| {
                i.given("the LLM queue is full");
                i.request.method("POST");
                i.request.path("/v1/summary/generate");
                i.request.content_type("application/json");
                i.request.json_body(json_pattern!({
                    "job_id": like!("00000000-0000-0000-0000-000000000002"),
                    "genre": like!("politics"),
                    "clusters": each_like!(json_pattern!({
                        "cluster_id": like!(0i64),
                        "representative_sentences": each_like!(json_pattern!({
                            "text": like!("Queue full test sentence"),
                        })),
                    })),
                }));
                i.response.status(429);
                i.response.content_type("application/json");
                i.response.header("Retry-After", "30");
                i.response.json_body(json_pattern!({
                    "error": like!("queue full"),
                }));
                i
            },
        )
        .with_output_dir(PACT_DIR)
        .start_mock_server(None, None);

    let url = pact.path("/v1/summary/generate");
    let body = json!({
        "job_id": "00000000-0000-0000-0000-000000000002",
        "genre": "politics",
        "clusters": [{
            "cluster_id": 0,
            "representative_sentences": [{"text": "Queue full test sentence"}],
        }],
    });

    let resp = Client::new()
        .post(url)
        .json(&body)


        .send()
        .await
        .expect("request should succeed");

    assert_eq!(resp.status(), 429);
    assert!(resp.headers().get("Retry-After").is_some());
}
