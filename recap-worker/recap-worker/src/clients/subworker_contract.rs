//! Consumer-Driven Contract tests for recap-worker → recap-subworker.
//!
//! Verifies classification job submission, polling, coarse classification,
//! and clustering run submission/polling (POST /v1/runs, GET /v1/runs/{run_id}).

use pact_consumer::prelude::*;
use reqwest::Client;
use serde::Deserialize;
use serde_json::json;
use std::collections::HashMap;

use super::subworker::cards::SubworkerCardsClient;

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
            i.request
                .header("Authorization", "Bearer test-recap-subworker-token-42");
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
        .header("Authorization", "Bearer test-recap-subworker-token-42")
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
            i.request
                .header("Authorization", "Bearer test-recap-subworker-token-42");
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
        .header("Authorization", "Bearer test-recap-subworker-token-42")
        .send()
        .await
        .expect("request should succeed");

    assert_eq!(resp.status(), 200);
    let parsed: ClusterRunResponse = resp.json().await.expect("should parse response");
    assert_eq!(parsed.status, "succeeded");
    assert_eq!(parsed.run_id, 42);
}

/// Text embedding: POST /v1/embed → 200 OK
#[tokio::test]
#[ignore = "CDC contract test"]
async fn contract_subworker_embed() {
    let pact = PactBuilder::new("recap-worker", "recap-subworker")
        .interaction("an embedding request", "", |mut i| {
            i.given("the embedder is ready");
            i.request.method("POST");
            i.request.path("/v1/embed");
            i.request.content_type("application/json");
            i.request
                .header("Authorization", "Bearer test-recap-subworker-token-42");
            i.request.json_body(json_pattern!({
                "texts": each_like!(like!("Example headline")),
                "normalize": like!(true),
            }));
            i.response.status(200);
            i.response.content_type("application/json");
            i.response.json_body(json_pattern!({
                "model": like!("bge-m3"),
                "dim": like!(1024i64),
                "embeddings": each_like!(each_like!(like!(0.1f64))),
            }));
            i
        })
        .with_output_dir(PACT_DIR)
        .start_mock_server(None, None);

    let client = SubworkerCardsClient::new(pact.path("/").as_str())
        .expect("client creation should succeed")
        .with_admin_token(Some("test-recap-subworker-token-42".to_string()));

    let resp = client
        .embed(&["Example headline".to_string()], true)
        .await
        .expect("request should succeed");

    assert_eq!(resp.model, "bge-m3");
    assert_eq!(resp.dim, 1024);
    assert_eq!(resp.embeddings.len(), 1);
    assert!(!resp.embeddings[0].is_empty());
}

/// Story clustering: POST /v1/cluster-stories → 200 OK
#[tokio::test]
#[ignore = "CDC contract test"]
async fn contract_subworker_cluster_stories() {
    let id1 = "00000000-0000-0000-0000-000000000001";
    let id2 = "00000000-0000-0000-0000-000000000002";
    let pact = PactBuilder::new("recap-worker", "recap-subworker")
        .interaction("a story clustering request", "", |mut i| {
            i.given("the story clusterer is ready");
            i.request.method("POST");
            i.request.path("/v1/cluster-stories");
            i.request.content_type("application/json");
            i.request
                .header("Authorization", "Bearer test-recap-subworker-token-42");
            i.request.json_body(json_pattern!({
                "items": each_like!(json_pattern!({
                    "id": like!(id1),
                    "embedding": each_like!(like!(0.1f64)),
                    "published_at": like!("2026-09-21T00:00:00Z"),
                }), min = 2),
                "params": json_pattern!({
                    "threshold": like!(0.78f64),
                    "linkage": like!("average"),
                    "time_decay_per_day": like!(0.02f64),
                    "min_cluster_size": like!(1i64),
                }),
            }));
            i.response.status(200);
            i.response.content_type("application/json");
            i.response.json_body(json_pattern!({
                "clusters": each_like!(json_pattern!({
                    "cluster_id": like!(0i64),
                    "member_ids": each_like!(like!(id1)),
                    "centroid": each_like!(like!(0.1f64)),
                })),
                "params": json_pattern!({
                    "threshold": like!(0.78f64),
                    "linkage": like!("average"),
                    "time_decay_per_day": like!(0.02f64),
                    "min_cluster_size": like!(1i64),
                }),
            }));
            i
        })
        .with_output_dir(PACT_DIR)
        .start_mock_server(None, None);

    let client = SubworkerCardsClient::new(pact.path("/").as_str())
        .expect("client creation should succeed")
        .with_admin_token(Some("test-recap-subworker-token-42".to_string()));

    let req = crate::clients::subworker::cards::ClusterStoriesRequest {
        items: vec![
            crate::clients::subworker::cards::ClusterStoryItem {
                id: uuid::Uuid::parse_str(id1).unwrap(),
                embedding: vec![0.1, 0.2, 0.3, 0.4],
                published_at: chrono::DateTime::parse_from_rfc3339("2026-09-21T00:00:00Z")
                    .unwrap()
                    .with_timezone(&chrono::Utc),
                language: None,
            },
            crate::clients::subworker::cards::ClusterStoryItem {
                id: uuid::Uuid::parse_str(id2).unwrap(),
                embedding: vec![0.1, 0.2, 0.3, 0.4],
                published_at: chrono::DateTime::parse_from_rfc3339("2026-09-21T00:00:00Z")
                    .unwrap()
                    .with_timezone(&chrono::Utc),
                language: None,
            },
        ],
        params: crate::clients::subworker::cards::StoryClusterParams::default(),
    };

    let resp = client
        .cluster_stories(&req)
        .await
        .expect("request should succeed");

    assert_eq!(resp.clusters.len(), 1);
    assert_eq!(resp.clusters[0].cluster_id, 0);
    assert!(!resp.clusters[0].member_ids.is_empty());
    assert!(!resp.clusters[0].centroid.is_empty());
}

fn build_cluster_stories_per_lang_interaction(
    mut i: pact_consumer::builders::InteractionBuilder,
    id1: &str,
) -> pact_consumer::builders::InteractionBuilder {
    i.given("the story clusterer is ready");
    i.request.method("POST");
    i.request.path("/v1/cluster-stories");
    i.request.content_type("application/json");
    i.request
        .header("Authorization", "Bearer test-recap-subworker-token-42");
    i.request.json_body(json_pattern!({
        "items": each_like!(json_pattern!({
            "id": like!(id1),
            "embedding": each_like!(like!(0.1f64)),
            "published_at": like!("2026-09-21T00:00:00Z"),
        }), min = 2),
        "params": json_pattern!({
            "threshold": like!(0.78f64),
            "linkage": like!("average"),
            "time_decay_per_day": like!(0.02f64),
            "min_cluster_size": like!(2i64),
            "min_cluster_size_by_language": json_pattern!({
                "ja": like!(1i64),
                "en": like!(2i64),
            }),
        }),
    }));
    i.response.status(200);
    i.response.content_type("application/json");
    i.response.json_body(json_pattern!({
        "clusters": each_like!(json_pattern!({
            "cluster_id": like!(0i64),
            "member_ids": each_like!(like!(id1)),
            "centroid": each_like!(like!(0.1f64)),
        })),
        "params": json_pattern!({
            "threshold": like!(0.78f64),
            "linkage": like!("average"),
            "time_decay_per_day": like!(0.02f64),
            "min_cluster_size": like!(2i64),
            "min_cluster_size_by_language": json_pattern!({
                "ja": like!(1i64),
                "en": like!(2i64),
            }),
        }),
    }));
    i
}

/// Story clustering with per-language min cluster size: POST /v1/cluster-stories → 200 OK
#[tokio::test]
#[ignore = "CDC contract test"]
async fn contract_subworker_cluster_stories_with_per_language_min_size() {
    let id1 = "00000000-0000-0000-0000-000000000001";
    let id2 = "00000000-0000-0000-0000-000000000002";
    let pact = PactBuilder::new("recap-worker", "recap-subworker")
        .interaction(
            "a story clustering request with per-language min cluster size",
            "",
            |i| build_cluster_stories_per_lang_interaction(i, id1),
        )
        .with_output_dir(PACT_DIR)
        .start_mock_server(None, None);

    let client = SubworkerCardsClient::new(pact.path("/").as_str())
        .expect("client creation should succeed")
        .with_admin_token(Some("test-recap-subworker-token-42".to_string()));

    let mut min_by_lang = std::collections::HashMap::new();
    min_by_lang.insert("ja".to_string(), 1);
    min_by_lang.insert("en".to_string(), 2);

    let req = crate::clients::subworker::cards::ClusterStoriesRequest {
        items: vec![
            crate::clients::subworker::cards::ClusterStoryItem {
                id: uuid::Uuid::parse_str(id1).unwrap(),
                embedding: vec![0.1, 0.2, 0.3, 0.4],
                published_at: chrono::DateTime::parse_from_rfc3339("2026-09-21T00:00:00Z")
                    .unwrap()
                    .with_timezone(&chrono::Utc),
                language: None,
            },
            crate::clients::subworker::cards::ClusterStoryItem {
                id: uuid::Uuid::parse_str(id2).unwrap(),
                embedding: vec![0.1, 0.2, 0.3, 0.4],
                published_at: chrono::DateTime::parse_from_rfc3339("2026-09-21T00:00:00Z")
                    .unwrap()
                    .with_timezone(&chrono::Utc),
                language: None,
            },
        ],
        params: crate::clients::subworker::cards::StoryClusterParams {
            threshold: 0.78,
            linkage: "average".to_string(),
            time_decay_per_day: 0.02,
            min_cluster_size: 2,
            min_cluster_size_by_language: Some(min_by_lang),
        },
    };

    let resp = client
        .cluster_stories(&req)
        .await
        .expect("request should succeed");

    assert_eq!(resp.clusters.len(), 1);
    assert_eq!(resp.clusters[0].cluster_id, 0);
    assert!(!resp.clusters[0].member_ids.is_empty());
    assert!(!resp.clusters[0].centroid.is_empty());
    assert_eq!(resp.params.min_cluster_size, 2);
    let resp_min_by_lang = resp
        .params
        .min_cluster_size_by_language
        .as_ref()
        .expect("min_cluster_size_by_language should be echoed");
    assert_eq!(resp_min_by_lang.get("ja"), Some(&1));
    assert_eq!(resp_min_by_lang.get("en"), Some(&2));
}

/// Card verification: POST /v1/verify → 200 OK
#[tokio::test]
#[ignore = "CDC contract test"]
async fn contract_subworker_verify_card() {
    let job_id = "00000000-0000-0000-0000-000000000001";
    let card_id = "00000000-0000-0000-0000-000000000002";
    let pact = PactBuilder::new("recap-worker", "recap-subworker")
        .interaction("a card verification request", "", |mut i| {
            i.given("the card verifier is ready");
            i.request.method("POST");
            i.request.path("/v1/verify");
            i.request.content_type("application/json");
            i.request
                .header("Authorization", "Bearer test-recap-subworker-token-42");
            i.request.json_body(json_pattern!({
                "job_id": like!(job_id),
                "card_id": like!(card_id),
                "language": like!("ja"),
                "sentences": each_like!(json_pattern!({
                    "idx": like!(0i64),
                    "kind": like!("what"),
                    "text": like!("新しいAIモデルが発表された。"),
                    "refs": each_like!(like!(1i64)),
                })),
                "items": each_like!(json_pattern!({
                    "n": like!(1i64),
                    "title": like!("新しいAIモデルの発表"),
                    "lede": like!("大手企業が新しいAIモデルを正式に発表した。"),
                })),
                "thresholds": json_pattern!({
                    "attribution_cos": like!(0.55f64),
                }),
            }));
            i.response.status(200);
            i.response.content_type("application/json");
            i.response.json_body(json_pattern!({
                "sentences": each_like!(json_pattern!({
                    "idx": like!(0i64),
                    "attribution": json_pattern!({
                        "max_cos": like!(0.85f64),
                        "best_n": like!(1i64),
                        "pass": like!(true),
                    }),
                    "filler": json_pattern!({
                        "matched": json_pattern!([]),
                        "pass": like!(true),
                    }),
                    "specificity": json_pattern!({
                        "proper_nouns": like!(1i64),
                        "numbers": like!(0i64),
                        "tokens": like!(5i64),
                        "density": like!(0.2f64),
                    }),
                })),
                "why_hint_present": like!(true),
                "embedding": json_pattern!({
                    "model": like!("bge-m3"),
                    "identity": like!("bge-m3"),
                }),
            }));
            i
        })
        .with_output_dir(PACT_DIR)
        .start_mock_server(None, None);

    let client = SubworkerCardsClient::new(pact.path("/").as_str())
        .expect("client creation should succeed")
        .with_admin_token(Some("test-recap-subworker-token-42".to_string()));

    let req = crate::clients::subworker::cards::VerifyCardRequest {
        job_id: uuid::Uuid::parse_str(job_id).unwrap(),
        card_id: uuid::Uuid::parse_str(card_id).unwrap(),
        language: "ja".to_string(),
        sentences: vec![crate::clients::subworker::cards::VerifySentenceInput {
            idx: 0,
            kind: "what".to_string(),
            text: "新しいAIモデルが発表された。".to_string(),
            refs: vec![1],
        }],
        items: vec![crate::clients::subworker::cards::VerifyItemInput {
            n: 1,
            title: "新しいAIモデルの発表".to_string(),
            lede: "大手企業が新しいAIモデルを正式に発表した。".to_string(),
        }],
        thresholds: crate::clients::subworker::cards::VerifyThresholds {
            attribution_cos: 0.55,
        },
    };

    let resp = client
        .verify_card(&req)
        .await
        .expect("request should succeed");

    assert_eq!(resp.sentences.len(), 1);
    assert_eq!(resp.sentences[0].idx, 0);
    assert!(resp.sentences[0].attribution.pass);
    assert!(resp.sentences[0].filler.pass);
    assert!(resp.sentences[0].specificity.density >= 0.0);
    assert!(resp.why_hint_present);
    assert_eq!(resp.embedding.model, "bge-m3");
}
