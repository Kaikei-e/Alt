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
use uuid::Uuid;

use super::client::NewsCreatorClient;
use super::models::{
    BatchSummaryResponse, CardGenerateOutcome, CardGenerateRequest, CardItemInput, SummaryResponse,
};

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
            // ADR-890 followup: references[].url is a full http(s):// URL,
            // references[].article_id is a UUID-or-null (production article_ids
            // are always UUIDs; LLM hallucinations like "dev.to" must be
            // null-ed by news-creator's sanitizer before they reach us).
            i.response.json_body(json_pattern!({
                "job_id": like!("00000000-0000-0000-0000-000000000001"),
                "genre": like!("tech"),
                "summary": json_pattern!({
                    "title": like!("テクノロジー週間要約"),
                    "bullets": each_like!(like!("AI関連の進展が報告された。 [1]")),
                    "language": like!("ja"),
                    "references": each_like!(json_pattern!({
                        "id": like!(1i64),
                        "url": term!(
                            r"^https?://[A-Za-z0-9.-]+(?:/[^\s]*)?$",
                            "https://example.com/article-1"
                        ),
                        "domain": like!("example.com"),
                        "article_id": term!(
                            r"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$",
                            "1dce453b-e23d-4a32-9030-7e4529fad645"
                        ),
                    })),
                }),
                "metadata": json_pattern!({
                    "model": like!("gemma4-e4b-q4km"),
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
                        "model": like!("gemma4-e4b-q4km"),
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
    let parsed: BatchSummaryResponse = resp
        .json()
        .await
        .expect("should parse as BatchSummaryResponse");
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

/// Card generation: POST /v1/cards/generate → 200 OK
#[tokio::test]
#[ignore = "CDC contract test — run with `cargo test contract -- --ignored`"]
async fn contract_news_creator_card_generate() {
    let pact = PactBuilder::new("recap-worker", "news-creator")
        .interaction("a card generate request", "", |mut i| {
            i.given("the LLM model is loaded and ready for card generation");
            i.request.method("POST");
            i.request.path("/v1/cards/generate");
            i.request.content_type("application/json");
            i.request.header("X-Job-ID", "00000000-0000-0000-0000-000000000001");
            i.request.json_body(json_pattern!({
                "job_id": like!("00000000-0000-0000-0000-000000000001"),
                "candidate_id": like!("00000000-0000-0000-0000-000000000002"),
                "prompt_version": like!("recap_card.v1"),
                "items": each_like!(json_pattern!({
                    "n": like!(1i64),
                    "feed_id": like!("00000000-0000-0000-0000-000000000003"),
                    "title": like!("Example Article Title"),
                    "host": like!("example.com"),
                    "url": like!("https://example.com/article-1"),
                    "pub_date": like!("2026-09-21T00:00:00Z"),
                    "lede": like!("Example article lead text."),
                })),
            }));
            i.response.status(200);
            i.response.content_type("application/json");
            i.response.json_body(json_pattern!({
                "card": json_pattern!({
                    "headline_ja": like!("テストヘッドライン"),
                    "what_ja": each_like!(json_pattern!({
                        "text": like!("テストの出来事が発生した。[1]"),
                        "refs": each_like!(like!(1i64)),
                    })),
                    "why_ja": json_pattern!({
                        "text": like!("テストの影響が生じる。[1]"),
                        "refs": each_like!(like!(1i64)),
                    }),
                    "used_refs": each_like!(like!(1i64)),
                }),
                "generation": json_pattern!({
                    "model": like!("gemma4-e4b-12k"),
                    "prompt_version": like!("recap_card.v1"),
                    "cache_hit": like!(false),
                    "prompt_tokens": like!(100i64),
                    "completion_tokens": like!(50i64),
                    "ms": like!(200i64),
                    "raw_text": like!("【見出し】\nテストヘッドライン\n【何が起きた】\nテストの出来事が発生した。[1]\n【なぜ重要】\nテストの影響が生じる。[1]\n【出典】\n[1]"),
                }),
                "ja_ratio": like!(0.83f64),
            }));
            i
        })
        .with_output_dir(PACT_DIR)
        .start_mock_server(None, None);

    let client = NewsCreatorClient::new_for_test(pact.url().to_string());
    let request = CardGenerateRequest {
        job_id: Uuid::parse_str("00000000-0000-0000-0000-000000000001").unwrap(),
        candidate_id: Uuid::parse_str("00000000-0000-0000-0000-000000000002").unwrap(),
        prompt_version: "recap_card.v1".to_string(),
        items: vec![CardItemInput {
            n: 1,
            feed_id: Uuid::parse_str("00000000-0000-0000-0000-000000000003").unwrap(),
            title: "Example Article Title".to_string(),
            host: "example.com".to_string(),
            url: "https://example.com/article-1".to_string(),
            pub_date: Some("2026-09-21T00:00:00Z".to_string()),
            lede: "Example article lead text.".to_string(),
        }],
        revision_note: None,
    };

    let outcome = client
        .generate_card(&request)
        .await
        .expect("request should succeed");

    match outcome {
        CardGenerateOutcome::Success(parsed) => {
            assert_eq!(parsed.card.headline_ja, "テストヘッドライン");
            assert!(!parsed.card.what_ja.is_empty());
            assert_eq!(parsed.generation.prompt_version, "recap_card.v1");
            assert!((parsed.ja_ratio - 0.83).abs() < 1e-4);
        }
        CardGenerateOutcome::Rejected(rejected) => {
            panic!("expected Success, got Rejected: {rejected:?}");
        }
    }
}

/// Card generation rejected: POST /v1/cards/generate → 422 Unprocessable Entity
#[tokio::test]
#[ignore = "CDC contract test — run with `cargo test contract -- --ignored`"]
async fn contract_news_creator_card_generate_rejected() {
    let pact = PactBuilder::new("recap-worker", "news-creator")
        .interaction(
            "a card generate request that is rejected by validation",
            "",
            |mut i| {
                i.given("card generation fails validation and is rejected");
                i.request.method("POST");
                i.request.path("/v1/cards/generate");
                i.request.content_type("application/json");
                i.request
                    .header("X-Job-ID", "00000000-0000-0000-0000-000000000004");
                i.request.json_body(json_pattern!({
                    "job_id": like!("00000000-0000-0000-0000-000000000004"),
                    "candidate_id": like!("00000000-0000-0000-0000-000000000005"),
                    "prompt_version": like!("recap_card.v1"),
                    "items": each_like!(json_pattern!({
                        "n": like!(1i64),
                        "feed_id": like!("00000000-0000-0000-0000-000000000006"),
                        "title": like!("Invalid Format Article Title"),
                        "host": like!("example.com"),
                        "url": like!("https://example.com/article-2"),
                        "pub_date": like!("2026-09-21T00:00:00Z"),
                        "lede": like!("Invalid format article lead text."),
                    })),
                }));
                i.response.status(422);
                i.response.content_type("application/json");
                i.response.header("X-Card-Rejection", "1");
                i.response.json_body(json_pattern!({
                    "reason": like!("parse_failed"),
                    "attempts": like!(2i64),
                    "raw_text": like!("【見出し】不正なフォーマット"),
                    "detail": like!("sentence_count"),
                }));
                i
            },
        )
        .with_output_dir(PACT_DIR)
        .start_mock_server(None, None);

    let client = NewsCreatorClient::new_for_test(pact.url().to_string());
    let request = CardGenerateRequest {
        job_id: Uuid::parse_str("00000000-0000-0000-0000-000000000004").unwrap(),
        candidate_id: Uuid::parse_str("00000000-0000-0000-0000-000000000005").unwrap(),
        prompt_version: "recap_card.v1".to_string(),
        items: vec![CardItemInput {
            n: 1,
            feed_id: Uuid::parse_str("00000000-0000-0000-0000-000000000006").unwrap(),
            title: "Invalid Format Article Title".to_string(),
            host: "example.com".to_string(),
            url: "https://example.com/article-2".to_string(),
            pub_date: Some("2026-09-21T00:00:00Z".to_string()),
            lede: "Invalid format article lead text.".to_string(),
        }],
        revision_note: None,
    };

    let outcome = client
        .generate_card(&request)
        .await
        .expect("request should complete with outcome");

    match outcome {
        CardGenerateOutcome::Rejected(rejected) => {
            assert_eq!(rejected.reason, "parse_failed");
            assert_eq!(rejected.attempts, 2);
            assert!(!rejected.raw_text.is_empty());
            assert_eq!(rejected.detail.as_deref(), Some("sentence_count"));
        }
        CardGenerateOutcome::Success(success) => {
            panic!("expected Rejected, got Success: {success:?}");
        }
    }
}
