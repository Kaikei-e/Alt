//! Consumer-Driven Contract tests for recap-worker → recap-subworker.
//!
//! Verifies classification job submission, polling, and coarse classification.

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
struct ClusteringResponse {
    status: String,
}

const PACT_DIR: &str = "../../pacts";

/// Submit classification job: POST /v1/classify-runs → 200 OK
#[tokio::test]
#[ignore]
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
#[ignore]
async fn contract_subworker_classify_poll_succeeded() {
    let pact = PactBuilder::new("recap-worker", "recap-subworker")
        .interaction(
            "polling a completed classification job",
            "",
            |mut i| {
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
            },
        )
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
#[ignore]
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

/// Clustering execution: POST /v1/clustering/{run_id} → 200 OK
#[tokio::test]
#[ignore]
async fn contract_subworker_clustering() {
    let pact = PactBuilder::new("recap-worker", "recap-subworker")
        .interaction("a clustering execution request", "", |mut i| {
            i.given("classified articles are ready for clustering");
            i.request.method("POST");
            i.request.path("/v1/clustering/42");
            i.request.content_type("application/json");
            i.response.status(200);
            i.response.content_type("application/json");
            i.response.json_body(json_pattern!({
                "run_id": like!(42i64),
                "job_id": like!("00000000-0000-0000-0000-000000000001"),
                "genre": like!("technology"),
                "status": like!("succeeded"),
                "cluster_count": like!(3i64),
                "clusters": each_like!(json_pattern!({
                    "cluster_id": like!(0i64),
                    "size": like!(5i64),
                    "top_terms": each_like!(like!("AI")),
                    "representatives": each_like!(json_pattern!({
                        "sentence_text": like!("AI is transforming industries."),
                    })),
                })),
            }));
            i
        })
        .with_output_dir(PACT_DIR)
        .start_mock_server(None, None);

    let url = pact.path("/v1/clustering/42");
    let body = json!({});

    let resp = Client::new()
        .post(url)
        .json(&body)
        .send()
        .await
        .expect("request should succeed");

    assert_eq!(resp.status(), 200);
    let parsed: ClusteringResponse = resp.json().await.expect("should parse response");
    assert_eq!(parsed.status, "succeeded");
}
