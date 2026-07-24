//! Consumer-Driven Contract tests for recap-worker → recap-subworker.
//!
//! Verifies classification job submission, polling, coarse classification,
//! and clustering run submission/polling (POST /v1/runs, GET /v1/runs/{run_id}).

use pact_consumer::prelude::*;
use reqwest::Client;
use serde::Deserialize;
use serde_json::json;
use std::collections::HashMap;

#[derive(Debug, Deserialize)]
struct ClassificationJobResponse {
    run_id: i64,
    status: String,
}

#[derive(Debug, Deserialize)]
struct CoarseClassifyResponse {
    scores: HashMap<String, f32>,
}

#[derive(Debug, Deserialize)]
struct ClusterRunResponse {
    run_id: i64,
    status: String,
}

const PACT_DIR: &str = "../../pacts";

/// Submit classification job: POST /v1/classify-runs → 200 OK
#[tokio::test]
#[ignore = "CDC contract test"]
async fn contract_subworker_classify_submit() {
    let pact = PactBuilder::new("recap-worker", "recap-subworker")
        .interaction("a classification job submission", "", |mut i| {
            i.given("the classification model is loaded");
            i.request.method("POST");
            i.request.path("/v1/classify-runs");
            i.request.content_type("application/json");
            i.request.json_body(json_pattern!({
                "texts": each_like!(like!("Article text to classify")),
            }));
            i.response.status(200);
            i.response.content_type("application/json");
            i.response.json_body(json_pattern!({
                "run_id": like!(1i64),
                "job_id": like!("00000000-0000-0000-0000-000000000001"),
                "status": like!("running"),
                "result_count": like!(0i64),
            }));
            i
        })
        .with_output_dir(PACT_DIR)
        .start_mock_server(None, None);

    let url = pact.path("/v1/classify-runs");
    let body = json!({"texts": ["Article text to classify"]});

    let resp = Client::new()
        .post(url)
        .json(&body)
        .send()
        .await
        .expect("request should succeed");

    assert_eq!(resp.status(), 200);
    let parsed: ClassificationJobResponse = resp.json().await.expect("should parse response");
    assert_eq!(parsed.status, "running");
    assert!(parsed.run_id > 0);
}

/// Poll classification job status: GET /v1/classify-runs/{run_id} → 200 OK (succeeded)
#[tokio::test]
#[ignore = "CDC contract test"]
async fn contract_subworker_classify_poll_succeeded() {
    let pact = PactBuilder::new("recap-worker", "recap-subworker")
        .interaction("polling a completed classification job", "", |mut i| {
            i.given("classification job 42 has succeeded");
            i.request.method("GET");
            i.request.path("/v1/classify-runs/42");
            i.response.status(200);
            i.response.content_type("application/json");
            i.response.json_body(json_pattern!({
                "run_id": like!(42i64),
                "job_id": like!("00000000-0000-0000-0000-000000000001"),
                "status": like!("succeeded"),
                "result_count": like!(5i64),
                "results": each_like!(json_pattern!({
                    "top_genre": like!("technology"),
                    "confidence": like!(0.85f64),
                    "scores": json_pattern!({
                        "technology": like!(0.85f64),
                        "science": like!(0.10f64),
                    }),
                })),
            }));
            i
        })
        .with_output_dir(PACT_DIR)
        .start_mock_server(None, None);

    let url = pact.path("/v1/classify-runs/42");

    let resp = Client::new()
        .get(url)
        .send()
        .await
        .expect("request should succeed");

    assert_eq!(resp.status(), 200);
}

/// Coarse genre classification: POST /v1/classify/coarse → 200 OK
#[tokio::test]
#[ignore = "CDC contract test"]
async fn contract_subworker_coarse_classify() {
    let pact = PactBuilder::new("recap-worker", "recap-subworker")
        .interaction("a coarse classification request", "", |mut i| {
            i.given("the classification model is loaded");
            i.request.method("POST");
            i.request.path("/v1/classify/coarse");
            i.request.content_type("application/json");
            i.request.json_body(json_pattern!({
                "text": like!("Breaking news about AI technology"),
            }));
            i.response.status(200);
            i.response.content_type("application/json");
            i.response.json_body(json_pattern!({
                "scores": json_pattern!({
                    "technology": like!(0.80f64),
                    "science": like!(0.15f64),
                }),
            }));
            i
        })
        .with_output_dir(PACT_DIR)
        .start_mock_server(None, None);

    let url = pact.path("/v1/classify/coarse");
    let body = json!({"text": "Breaking news about AI technology"});

    let resp = Client::new()
        .post(url)
        .json(&body)
        .send()
        .await
        .expect("request should succeed");

    assert_eq!(resp.status(), 200);
    let parsed: CoarseClassifyResponse = resp.json().await.expect("should parse response");
    assert!(parsed.scores.contains_key("technology"));
}

/// Clustering run submission: POST /v1/runs → 202 Accepted
///
/// Mirrors the real request the client sends in
/// `subworker/clustering.rs::cluster_corpus_with_timeout`: `X-Alt-Job-Id` /
/// `X-Alt-Genre` headers plus a `ClusterJobRequest` body (`params`,
/// `documents`, `metadata`). recap-subworker always accepts the run
/// asynchronously (`status: "running"`, no clusters yet) — the client
/// polls `GET /v1/runs/{run_id}` for the terminal result.
#[tokio::test]
#[ignore = "CDC contract test"]
async fn contract_subworker_clustering_submit() {
    let pact = PactBuilder::new("recap-worker", "recap-subworker")
        .interaction("a clustering run submission", "", |mut i| {
            i.given("the clustering pipeline accepts a new run");
            i.request.method("POST");
            i.request.path("/v1/runs");
            i.request.content_type("application/json");
            i.request
                .header("X-Alt-Job-Id", "00000000-0000-0000-0000-000000000001");
            i.request.header("X-Alt-Genre", "technology");
            i.request.json_body(json_pattern!({
                "params": json_pattern!({
                    "max_sentences_total": like!(2000i64),
                    "max_sentences_per_cluster": like!(20i64),
                    "umap_n_components": like!(25i64),
                    "hdbscan_min_cluster_size": like!(5i64),
                    "mmr_lambda": like!(0.35f64),
                }),
                "documents": each_like!(json_pattern!({
                    "article_id": like!("art-001"),
                    "paragraphs": each_like!(like!(
                        "AI is transforming industries across every major economic sector."
                    )),
                }), min = 3),
                "metadata": json_pattern!({
                    "article_count": like!(3i64),
                    "sentence_count": like!(10i64),
                    "primary_language": like!("en"),
                    "character_count": like!(500i64),
                }),
            }));
            i.response.status(202);
            i.response.content_type("application/json");
            i.response.json_body(json_pattern!({
                "run_id": like!(42i64),
                "job_id": like!("00000000-0000-0000-0000-000000000001"),
                "genre": like!("technology"),
                "status": like!("running"),
                "cluster_count": like!(0i64),
                "clusters": [],
                "diagnostics": json_pattern!({}),
            }));
            i
        })
        .with_output_dir(PACT_DIR)
        .start_mock_server(None, None);

    let url = pact.path("/v1/runs");
    let body = json!({
        "params": {
            "max_sentences_total": 2000,
            "max_sentences_per_cluster": 20,
            "umap_n_components": 25,
            "hdbscan_min_cluster_size": 5,
            "mmr_lambda": 0.35,
        },
        "documents": [
            {
                "article_id": "art-001",
                "paragraphs": ["AI is transforming industries across every major economic sector."],
            },
            {
                "article_id": "art-002",
                "paragraphs": ["AI is transforming industries across every major economic sector."],
            },
            {
                "article_id": "art-003",
                "paragraphs": ["AI is transforming industries across every major economic sector."],
            },
        ],
        "metadata": {
            "article_count": 3,
            "sentence_count": 10,
            "primary_language": "en",
            "character_count": 500,
        },
    });

    let resp = Client::new()
        .post(url)
        .header("X-Alt-Job-Id", "00000000-0000-0000-0000-000000000001")
        .header("X-Alt-Genre", "technology")
        .json(&body)
        .send()
        .await
        .expect("request should succeed");

    assert_eq!(resp.status(), 202);
    let parsed: ClusterRunResponse = resp.json().await.expect("should parse response");
    assert_eq!(parsed.status, "running");
    assert_eq!(parsed.run_id, 42);
}

/// Clustering run poll: GET /v1/runs/{run_id} → 200 OK (succeeded)
///
/// Covers the terminal-state shape `poll_run_once` deserializes and
/// validates against `CLUSTERING_RESPONSE_SCHEMA` in
/// `subworker/clustering.rs`.
#[tokio::test]
#[ignore = "CDC contract test"]
async fn contract_subworker_clustering_poll_succeeded() {
    let pact = PactBuilder::new("recap-worker", "recap-subworker")
        .interaction("polling a completed clustering run", "", |mut i| {
            i.given("clustering run 42 has succeeded");
            i.request.method("GET");
            i.request.path("/v1/runs/42");
            i.response.status(200);
            i.response.content_type("application/json");
            i.response.json_body(json_pattern!({
                "run_id": like!(42i64),
                "job_id": like!("00000000-0000-0000-0000-000000000001"),
                "genre": like!("technology"),
                "status": like!("succeeded"),
                "cluster_count": like!(1i64),
                "clusters": each_like!(json_pattern!({
                    "cluster_id": like!(0i64),
                    "size": like!(5i64),
                    "label": like!("technology"),
                    "top_terms": each_like!(like!("AI")),
                    "stats": json_pattern!({}),
                    "representatives": each_like!(json_pattern!({
                        "article_id": like!("art-001"),
                        "paragraph_idx": like!(0i64),
                        "sentence_text": like!(
                            "AI is transforming industries across every major economic sector."
                        ),
                        "lang": like!("en"),
                        "score": like!(0.95f64),
                    })),
                })),
                "diagnostics": json_pattern!({
                    "dedup_pairs": like!(0i64),
                }),
            }));
            i
        })
        .with_output_dir(PACT_DIR)
        .start_mock_server(None, None);

    let url = pact.path("/v1/runs/42");

    let resp = Client::new()
        .get(url)
        .send()
        .await
        .expect("request should succeed");

    assert_eq!(resp.status(), 200);
    let parsed: ClusterRunResponse = resp.json().await.expect("should parse response");
    assert_eq!(parsed.status, "succeeded");
    assert_eq!(parsed.run_id, 42);
}
