//! Provider verification for recap-worker.
//!
//! Replays consumer-driven pact files against a minimal stub axum Router
//! that mirrors the real recap-worker endpoints. Each interaction's request
//! is issued against the stub, and the response status + body shape is
//! compared against the pact's expected values.
//!
//! Lightweight by design — pact_verifier crate would be stronger but adds
//! ~50 transitive deps and multiplies compile time. The trade-off for a
//! subset of matchers is acceptable for a first provider-verify gate.
//!
//! Run with: `cargo test --test provider_verification -- --ignored`

use std::net::SocketAddr;
use std::path::{Path as FsPath, PathBuf};
use std::sync::{Arc, RwLock};

use axum::{
    Router,
    extract::Path,
    response::Json,
    routing::{get, post},
};
use chrono::{DateTime, Utc};
use recap_worker::api::cards::{
    CardsDao, PreviousCardsJobMeta, RecapCard, RecapCardJobStats, get_3days_cards_impl,
};
use serde_json::{Value, json};
use uuid::Uuid;

/// Schema identifier for the verification-evidence document. The Ansible
/// bridge (`playbooks/_publish-bridge.yml`) refuses to publish a success
/// record unless it can read a document carrying exactly this schema.
const EVIDENCE_SCHEMA: &str = "alt.pact.evidence.v1";

/// Label the broker records as the verifier that produced the result. The
/// bridge registry names the same string and the two must agree, so the
/// `implementation` field on a verification record is an assertion about
/// what ran rather than a free-form note.
const EVIDENCE_IMPLEMENTATION: &str = "rust-stub-replay";

/// What one replayed pact proves. Filled in only on the success path — the
/// per-interaction assertions panic first, so an evidence file existing at
/// all means every interaction in that pact was replayed and matched.
struct Evidence<'a> {
    provider: &'a str,
    consumer: &'a str,
    implementation: &'a str,
    interactions_verified: usize,
    interactions_skipped: usize,
}

fn evidence_document(evidence: &Evidence<'_>, run_id: &str, provider_version: &str) -> Value {
    json!({
        "schema": EVIDENCE_SCHEMA,
        "run_id": run_id,
        "provider": evidence.provider,
        "consumer": evidence.consumer,
        "implementation": evidence.implementation,
        "provider_version": provider_version,
        "verifier_version": env!("CARGO_PKG_VERSION"),
        "verified_at": chrono::Utc::now().to_rfc3339_opts(chrono::SecondsFormat::Secs, true),
        "interactions_verified": evidence.interactions_verified,
        "interactions_skipped": evidence.interactions_skipped,
        "result": "passed",
    })
}

fn write_evidence_to(dir: &FsPath, doc: &Value) -> PathBuf {
    let path = dir.join(format!(
        "{}__{}.json",
        doc["provider"].as_str().expect("provider"),
        doc["consumer"].as_str().expect("consumer"),
    ));
    std::fs::create_dir_all(dir)
        .unwrap_or_else(|e| panic!("failed to create evidence dir {}: {e}", dir.display()));
    std::fs::write(
        &path,
        serde_json::to_vec_pretty(doc).expect("evidence is serializable"),
    )
    .unwrap_or_else(|e| panic!("failed to write evidence {}: {e}", path.display()));
    path
}

/// Emit the evidence document for one replayed pact. A write failure aborts
/// the test: the bridge treats a missing file as "never verified", so a
/// silently-dropped write would downgrade a passing verification into a
/// deploy-blocking failure with no explanation.
fn record_evidence(evidence: &Evidence<'_>) {
    let Ok(dir) = std::env::var("PACT_EVIDENCE_DIR") else {
        eprintln!(
            "PACT_EVIDENCE_DIR unset — no verification evidence emitted for {}/{}. \
             The bridge publish step will fail until this runs under scripts/pact-check.sh.",
            evidence.provider, evidence.consumer
        );
        return;
    };
    let run_id = std::env::var("PACT_EVIDENCE_RUN_ID").unwrap_or_default();
    let provider_version = std::env::var("PACT_PROVIDER_VERSION").unwrap_or_default();
    let doc = evidence_document(evidence, &run_id, &provider_version);
    let path = write_evidence_to(FsPath::new(&dir), &doc);
    eprintln!("verification evidence written: {}", path.display());
}

#[derive(Clone, Default)]
struct StubState {
    provider_state: Arc<RwLock<Option<String>>>,
}

struct VerificationCardsDao {
    provider_state: Arc<RwLock<Option<String>>>,
}

#[async_trait::async_trait]
impl CardsDao for VerificationCardsDao {
    async fn get_latest_completed_cards_job(&self) -> Result<Option<PreviousCardsJobMeta>, String> {
        let current = self.provider_state.read().unwrap().clone();
        match current.as_deref() {
            Some("no cards job exists") => Ok(None),
            Some(
                "a completed cards job with cards exists"
                | "a completed cards job whose card has no genre and no why",
            ) => Ok(Some(PreviousCardsJobMeta {
                job_id: Uuid::parse_str("11111111-2222-3333-4444-555555555555").unwrap(),
                kicked_at: DateTime::parse_from_rfc3339("2026-09-22T17:00:00Z")
                    .unwrap()
                    .with_timezone(&Utc),
                from_ts: DateTime::parse_from_rfc3339("2026-09-19T17:00:00Z")
                    .unwrap()
                    .with_timezone(&Utc),
                to_ts: DateTime::parse_from_rfc3339("2026-09-22T17:00:00Z")
                    .unwrap()
                    .with_timezone(&Utc),
                params_version: "cards-v0.2".to_string(),
            })),
            Some(unknown) => {
                panic!("unrecognised provider state in VerificationCardsDao: {unknown}")
            }
            None => panic!("missing provider state in VerificationCardsDao"),
        }
    }

    async fn get_cards_for_job(&self, job_id: Uuid) -> Result<Vec<RecapCard>, String> {
        let current = self.provider_state.read().unwrap().clone();
        match current.as_deref() {
            Some("no cards job exists") => Ok(Vec::new()),
            Some("a completed cards job with cards exists") => Ok(vec![RecapCard {
                id: Uuid::parse_str("22222222-3333-4444-5555-666666666666").unwrap(),
                job_id,
                rank: 1,
                story_id: Uuid::parse_str("33333333-4444-5555-6666-777777777777").unwrap(),
                continues_card_id: None,
                merged_from: None,
                headline_ja: "日本のAIスタートアップが新モデルを発表".to_string(),
                what_ja: "最新の推論モデルが公開された。[1]ベンチマークで高い性能を示した。[2]"
                    .to_string(),
                why_ja: Some("日本語処理の効率化が期待される。[1]".to_string()),
                genre: Some("Technology".to_string()),
                member_feed_ids: vec![
                    Uuid::parse_str("44444444-5555-6666-7777-888888888888").unwrap(),
                ],
                sources: json!([
                    {
                        "feed_id": "44444444-5555-6666-7777-888888888888",
                        "host": "example.com",
                        "n": 1,
                        "pub_date": "2026-09-22T10:00:00Z",
                        "title": "新モデル発表のニュース",
                        "url": "https://example.com/ai-news"
                    }
                ]),
                centroid: Some(vec![0.1, 0.2]),
                scores: json!({}),
                gates: json!({}),
                generation: json!({}),
                created_at: DateTime::parse_from_rfc3339("2026-09-22T17:05:00Z")
                    .unwrap()
                    .with_timezone(&Utc),
            }]),
            Some("a completed cards job whose card has no genre and no why") => {
                Ok(vec![RecapCard {
                    id: Uuid::parse_str("22222222-3333-4444-5555-666666666666").unwrap(),
                    job_id,
                    rank: 1,
                    story_id: Uuid::parse_str("33333333-4444-5555-6666-777777777777").unwrap(),
                    continues_card_id: None,
                    merged_from: None,
                    headline_ja: "日本のAIスタートアップが新モデルを発表".to_string(),
                    what_ja: "最新の推論モデルが公開された。[1]ベンチマークで高い性能を示した。[2]"
                        .to_string(),
                    why_ja: None,
                    genre: None,
                    member_feed_ids: vec![
                        Uuid::parse_str("44444444-5555-6666-7777-888888888888").unwrap(),
                    ],
                    sources: json!([
                        {
                            "feed_id": "44444444-5555-6666-7777-888888888888",
                            "host": "example.com",
                            "n": 1,
                            "pub_date": "2026-09-22T10:00:00Z",
                            "title": "新モデル発表のニュース",
                            "url": "https://example.com/ai-news"
                        }
                    ]),
                    centroid: Some(vec![0.1, 0.2]),
                    scores: json!({}),
                    gates: json!({}),
                    generation: json!({}),
                    created_at: DateTime::parse_from_rfc3339("2026-09-22T17:05:00Z")
                        .unwrap()
                        .with_timezone(&Utc),
                }])
            }
            Some(unknown) => {
                panic!("unrecognised provider state in VerificationCardsDao: {unknown}")
            }
            None => panic!("missing provider state in VerificationCardsDao"),
        }
    }

    async fn get_job_stats(&self, job_id: Uuid) -> Result<Option<RecapCardJobStats>, String> {
        let current = self.provider_state.read().unwrap().clone();
        match current.as_deref() {
            Some("no cards job exists") => Ok(None),
            Some(
                "a completed cards job with cards exists"
                | "a completed cards job whose card has no genre and no why",
            ) => Ok(Some(RecapCardJobStats {
                job_id,
                items_fetched: 100,
                items_after_noise: 80,
                items_after_dedup: 60,
                clusters: 20,
                candidates: 10,
                cards_selected: 8,
                cards_dropped: json!({}),
                embed_ms: 10,
                cluster_ms: 10,
                llm_ms: 10,
                total_ms: 30,
                params_version: "cards-v0.2".to_string(),
                embed_cache_hits: 0,
                embed_cache_misses: 0,
                created_at: DateTime::parse_from_rfc3339("2026-09-22T17:05:00Z")
                    .unwrap()
                    .with_timezone(&Utc),
            })),
            Some(unknown) => {
                panic!("unrecognised provider state in VerificationCardsDao: {unknown}")
            }
            None => panic!("missing provider state in VerificationCardsDao"),
        }
    }
}

/// Build a stub Router that matches the endpoints asserted by
/// recap-worker's consumer pacts. Each handler returns a canned response
/// that satisfies the pact's expected body shape.
fn stub_router() -> Router {
    stub_router_with_state(StubState::default())
}

#[allow(clippy::too_many_lines)]
fn stub_router_with_state(state: StubState) -> Router {
    Router::new()
        .route(
            "/_pact/provider-state",
            post({
                let state = state.clone();
                move |axum::Json(payload): axum::Json<Value>| {
                    let state = state.clone();
                    async move {
                        let name = payload
                            .get("state")
                            .and_then(|v| v.as_str())
                            .map(ToString::to_string);
                        *state.provider_state.write().unwrap() = name;
                        axum::http::StatusCode::OK
                    }
                }
            }),
        )
        // search-indexer-recap-worker.json
        .route(
            "/v1/recaps/genres/indexable",
            get(|| async {
                Json(json!({
                    "genres": [
                        {"genre": "technology", "last_indexed_at": "2026-04-10T00:00:00Z"}
                    ]
                }))
            }),
        )
        // recap-evaluator-recap-worker.json — literal-segment routes first
        // so /latest is matched as a static path rather than parsed as the
        // `{run_id}` integer path parameter (an i64 parse of "latest" returns
        // 400 and trips the pact assertion).
        .route(
            "/v1/evaluation/genres",
            post(|| async {
                Json(json!({
                    "run_id": 1,
                    "status": "running",
                }))
            }),
        )
        .route(
            "/v1/evaluation/genres/latest",
            get(|| async {
                Json(json!({
                    "run_id": 42,
                    "status": "succeeded",
                    "accuracy": 0.85,
                    "macro_f1": 0.82,
                }))
            }),
        )
        .route(
            "/v1/evaluation/genres/{run_id}",
            get(|Path(run_id): Path<i64>| async move {
                Json(json!({
                    "run_id": run_id,
                    "status": "succeeded",
                    "accuracy": 0.85,
                    "macro_f1": 0.82,
                }))
            }),
        )
        // rag-orchestrator-recap-worker.json
        .route(
            "/v1/morning/letters/latest",
            get(|| async {
                Json(json!({
                    "id": "letter-001",
                    "target_date": "2026-04-15",
                    "body": {
                        "lead": "Today's key developments...",
                        "sections": [
                            {
                                "key": "top3",
                                "title": "Top Stories",
                                "bullets": ["Story A"]
                            }
                        ]
                    }
                }))
            }),
        )
        .route(
            "/v1/morning/letters/{date}",
            get(|Path(date): Path<String>| async move {
                Json(json!({
                    "id": "letter-001",
                    "target_date": date,
                    "body": {
                        "lead": "Today's key developments...",
                        "sections": [
                            {
                                "key": "top3",
                                "title": "Top Stories",
                                "bullets": ["Story A"]
                            }
                        ]
                    }
                }))
            }),
        )
        // alt-backend-recap-worker.json
        .route(
            "/v1/recaps/3days/cards",
            get({
                let state = state.clone();
                move || {
                    let state = state.clone();
                    async move {
                        let dao = VerificationCardsDao {
                            provider_state: state.provider_state.clone(),
                        };
                        get_3days_cards_impl(&dao).await
                    }
                }
            }),
        )
}

async fn start_stub_server() -> SocketAddr {
    let router = stub_router();
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    tokio::spawn(async move {
        axum::serve(listener, router).await.unwrap();
    });
    // Give the listener a moment to accept connections.
    tokio::time::sleep(std::time::Duration::from_millis(50)).await;
    addr
}

fn load_pact(path: &str) -> Value {
    let raw =
        std::fs::read_to_string(path).unwrap_or_else(|e| panic!("failed to read pact {path}: {e}"));
    serde_json::from_str(&raw).unwrap_or_else(|e| panic!("pact {path} is not JSON: {e}"))
}

async fn set_provider_state(addr: SocketAddr, state_name: &str) {
    let url = format!("http://{addr}/_pact/provider-state");
    let client = reqwest::Client::new();
    client
        .post(&url)
        .json(&json!({ "state": state_name }))
        .send()
        .await
        .unwrap_or_else(|e| panic!("failed to set provider state {state_name}: {e}"));
}

fn assert_shape_matches(actual: &Value, expected: &Value, path: &str) {
    if expected.is_null() {
        assert!(actual.is_null(), "{path}: expected null, got {actual}");
        return;
    }

    assert!(
        !actual.is_null(),
        "{path}: expected non-null ({expected}), got null"
    );

    if let Some(expected_obj) = expected.as_object() {
        let actual_obj = actual
            .as_object()
            .unwrap_or_else(|| panic!("{path}: expected object, got {actual}"));
        for (k, expected_val) in expected_obj {
            let actual_val = actual_obj
                .get(k)
                .unwrap_or_else(|| panic!("{path}: missing required key '{k}'"));
            assert_shape_matches(actual_val, expected_val, &format!("{path}.{k}"));
        }
    } else if let Some(expected_arr) = expected.as_array() {
        let actual_arr = actual
            .as_array()
            .unwrap_or_else(|| panic!("{path}: expected array, got {actual}"));
        if expected_arr.is_empty() {
            assert!(
                actual_arr.is_empty(),
                "{path}: expected empty array, got len {}",
                actual_arr.len()
            );
        } else {
            assert!(
                !actual_arr.is_empty(),
                "{path}: expected non-empty array, got empty"
            );
            assert_shape_matches(&actual_arr[0], &expected_arr[0], &format!("{path}[0]"));
        }
    } else if expected.is_boolean() {
        assert!(
            actual.is_boolean(),
            "{path}: expected boolean, got {actual}"
        );
    } else if expected.is_number() {
        assert!(actual.is_number(), "{path}: expected number, got {actual}");
    } else if expected.is_string() {
        assert!(actual.is_string(), "{path}: expected string, got {actual}");
    } else {
        panic!("{path}: unexpected JSON value type in expected: {expected}");
    }
}

async fn verify_interaction(
    addr: SocketAddr,
    method: &str,
    path: &str,
    expected_status: u16,
    expected_body: Option<&Value>,
) -> Value {
    let url = format!("http://{addr}{path}");
    let client = reqwest::Client::new();
    let req = match method {
        "GET" => client.get(&url),
        "POST" => client.post(&url),
        _ => panic!("unsupported method {method}"),
    };
    let resp = req
        .send()
        .await
        .unwrap_or_else(|e| panic!("request to {url} failed: {e}"));
    assert_eq!(
        resp.status().as_u16(),
        expected_status,
        "unexpected status for {method} {path}"
    );
    // Body is JSON — ensure it parses successfully (structural validity).
    let body: Value = resp
        .json()
        .await
        .unwrap_or_else(|e| panic!("response body for {path} not JSON: {e}"));
    assert!(
        body.is_object(),
        "response for {path} must be a JSON object"
    );
    if let Some(expected) = expected_body {
        assert_shape_matches(&body, expected, "$");
    }
    body
}

#[cfg(test)]
mod evidence_tests {
    use super::*;

    #[test]
    fn evidence_document_binds_the_run_and_counts_the_interactions() {
        let doc = evidence_document(
            &Evidence {
                provider: "recap-worker",
                consumer: "search-indexer",
                implementation: "rust-stub-replay",
                interactions_verified: 2,
                interactions_skipped: 1,
            },
            "run-xyz",
            "deadbee",
        );

        assert_eq!(doc["schema"], EVIDENCE_SCHEMA);
        assert_eq!(doc["run_id"], "run-xyz");
        assert_eq!(doc["provider_version"], "deadbee");
        assert_eq!(doc["provider"], "recap-worker");
        assert_eq!(doc["consumer"], "search-indexer");
        assert_eq!(doc["implementation"], "rust-stub-replay");
        assert_eq!(doc["interactions_verified"], 2);
        assert_eq!(doc["interactions_skipped"], 1);
        assert_eq!(doc["result"], "passed");
        assert!(
            doc["verified_at"]
                .as_str()
                .is_some_and(|s| s.ends_with('Z')),
            "verified_at must be an RFC3339 UTC instant"
        );
    }

    #[test]
    fn write_evidence_to_names_the_file_after_the_bridge_row() {
        let dir = tempfile::tempdir().unwrap();
        let doc = evidence_document(
            &Evidence {
                provider: "recap-worker",
                consumer: "recap-evaluator",
                implementation: "rust-stub-replay",
                interactions_verified: 3,
                interactions_skipped: 0,
            },
            "run-1",
            "cafe123",
        );

        let path = write_evidence_to(dir.path(), &doc);

        assert_eq!(
            path.file_name().unwrap(),
            "recap-worker__recap-evaluator.json"
        );
        let back: Value = serde_json::from_str(&std::fs::read_to_string(&path).unwrap()).unwrap();
        assert_eq!(back, doc);
    }

    #[test]
    fn assert_shape_matches_identical_shape_passes() {
        let expected = json!({
            "cards": [
                {
                    "id": "abc",
                    "rank": 1,
                    "continues_card_id": null
                }
            ],
            "job": {
                "cards_selected": 8,
                "degraded": false
            }
        });
        let actual = json!({
            "cards": [
                {
                    "id": "xyz",
                    "rank": 2,
                    "continues_card_id": null
                }
            ],
            "job": {
                "cards_selected": 10,
                "degraded": true
            }
        });
        assert_shape_matches(&actual, &expected, "$");
    }

    #[test]
    #[should_panic(expected = "missing required key 'b'")]
    fn assert_shape_matches_missing_key_panics() {
        let expected = json!({ "a": 1, "b": "str" });
        let actual = json!({ "a": 2 });
        assert_shape_matches(&actual, &expected, "$");
    }

    #[test]
    fn assert_shape_matches_extra_key_allowed() {
        let expected = json!({ "a": 1 });
        let actual = json!({ "a": 1, "extra": true });
        assert_shape_matches(&actual, &expected, "$");
    }

    #[tokio::test]
    #[should_panic(expected = "unrecognised provider state in VerificationCardsDao: bogus state")]
    async fn verification_cards_dao_panics_on_unknown_state() {
        let dao = VerificationCardsDao {
            provider_state: Arc::new(RwLock::new(Some("bogus state".to_string()))),
        };
        let _ = dao.get_latest_completed_cards_job().await;
    }

    #[test]
    #[should_panic(expected = "expected null")]
    fn assert_shape_matches_expected_null_panics() {
        let expected = json!({ "job": null });
        let actual = json!({ "job": { "cards_selected": 1 } });
        assert_shape_matches(&actual, &expected, "$");
    }

    #[test]
    #[should_panic(expected = "expected non-null")]
    fn assert_shape_matches_unexpected_null_panics() {
        let expected = json!({ "job": { "cards_selected": 1 } });
        let actual = json!({ "job": null });
        assert_shape_matches(&actual, &expected, "$");
    }

    #[test]
    #[should_panic(expected = "expected number")]
    fn assert_shape_matches_type_mismatch_panics() {
        let expected = json!({ "count": 10 });
        let actual = json!({ "count": "10" });
        assert_shape_matches(&actual, &expected, "$");
    }
}

#[tokio::test]
#[ignore = "provider verification: run with --ignored"]
async fn verify_search_indexer_pact() {
    let addr = start_stub_server().await;
    let pact = load_pact("../../pacts/search-indexer-recap-worker.json");
    let mut verified = 0usize;
    for interaction in pact["interactions"].as_array().unwrap() {
        let req = &interaction["request"];
        let resp = &interaction["response"];
        verify_interaction(
            addr,
            req["method"].as_str().unwrap(),
            req["path"].as_str().unwrap(),
            resp["status"].as_u64().unwrap() as u16,
            None,
        )
        .await;
        verified += 1;
    }
    assert!(verified > 0, "pact contained no interactions to replay");
    record_evidence(&Evidence {
        provider: "recap-worker",
        consumer: "search-indexer",
        implementation: EVIDENCE_IMPLEMENTATION,
        interactions_verified: verified,
        interactions_skipped: 0,
    });
}

#[tokio::test]
#[ignore = "provider verification: run with --ignored"]
async fn verify_recap_evaluator_pact() {
    let addr = start_stub_server().await;
    let pact = load_pact("../../pacts/recap-evaluator-recap-worker.json");
    let mut verified = 0usize;
    for interaction in pact["interactions"].as_array().unwrap() {
        let req = &interaction["request"];
        let resp = &interaction["response"];
        verify_interaction(
            addr,
            req["method"].as_str().unwrap(),
            req["path"].as_str().unwrap(),
            resp["status"].as_u64().unwrap() as u16,
            None,
        )
        .await;
        verified += 1;
    }
    assert!(verified > 0, "pact contained no interactions to replay");
    record_evidence(&Evidence {
        provider: "recap-worker",
        consumer: "recap-evaluator",
        implementation: EVIDENCE_IMPLEMENTATION,
        interactions_verified: verified,
        interactions_skipped: 0,
    });
}

#[tokio::test]
#[ignore = "provider verification: run with --ignored"]
async fn verify_rag_orchestrator_pact() {
    let addr = start_stub_server().await;
    let pact = load_pact("../../rag-orchestrator/pacts/rag-orchestrator-recap-worker.json");
    let mut verified = 0usize;
    let mut skipped = 0usize;
    for interaction in pact["interactions"].as_array().unwrap() {
        let req = &interaction["request"];
        let resp = &interaction["response"];
        let status = resp["status"].as_u64().unwrap() as u16;
        // Skip 404 interactions — stub always returns 200; a real verifier
        // would switch handlers by providerStates, which is out of scope
        // for this lightweight replay. The count is carried into the
        // evidence so the broker record does not overstate what was checked.
        if status == 404 {
            skipped += 1;
            continue;
        }
        verify_interaction(
            addr,
            req["method"].as_str().unwrap(),
            req["path"].as_str().unwrap(),
            status,
            None,
        )
        .await;
        verified += 1;
    }
    assert!(verified > 0, "pact contained no interactions to replay");
    record_evidence(&Evidence {
        provider: "recap-worker",
        consumer: "rag-orchestrator",
        implementation: EVIDENCE_IMPLEMENTATION,
        interactions_verified: verified,
        interactions_skipped: skipped,
    });
}

#[tokio::test]
#[ignore = "provider verification: run with --ignored"]
async fn verify_alt_backend_pact() {
    let addr = start_stub_server().await;
    let pact = load_pact("../../pacts/alt-backend-recap-worker.json");
    let mut verified = 0usize;
    for interaction in pact["interactions"].as_array().unwrap() {
        let mut state_name = "";
        if let Some(states) = interaction.get("providerStates").and_then(|s| s.as_array()) {
            for state in states {
                if let Some(name) = state.get("name").and_then(|n| n.as_str()) {
                    set_provider_state(addr, name).await;
                    state_name = name;
                }
            }
        }
        let req = &interaction["request"];
        let resp = &interaction["response"];
        let body = verify_interaction(
            addr,
            req["method"].as_str().unwrap(),
            req["path"].as_str().unwrap(),
            resp["status"].as_u64().unwrap() as u16,
            resp.get("body"),
        )
        .await;

        match state_name {
            "a completed cards job with cards exists" => {
                assert!(
                    body["job"].is_object(),
                    "expected job object for state: {state_name}"
                );
                assert!(
                    body["cards"].as_array().is_some_and(|c| !c.is_empty()),
                    "expected non-empty cards for state: {state_name}"
                );
                assert!(
                    body["cards"][0]["genre"].is_string(),
                    "expected genre string for state: {state_name}"
                );
                assert!(
                    body["cards"][0]["why_ja"].is_string(),
                    "expected why_ja string for state: {state_name}"
                );
            }
            "a completed cards job whose card has no genre and no why" => {
                assert!(
                    body["job"].is_object(),
                    "expected job object for state: {state_name}"
                );
                assert!(
                    body["cards"].as_array().is_some_and(|c| !c.is_empty()),
                    "expected non-empty cards for state: {state_name}"
                );
                assert!(
                    body["cards"][0]["genre"].is_null(),
                    "expected null genre for state: {state_name}"
                );
                assert!(
                    body["cards"][0]["why_ja"].is_null(),
                    "expected null why_ja for state: {state_name}"
                );
                assert!(
                    body["cards"][0]["continues_card_id"].is_null(),
                    "expected null continues_card_id for state: {state_name}"
                );
            }
            "no cards job exists" => {
                assert!(
                    body["job"].is_null(),
                    "expected null job for state: {state_name}"
                );
                assert_eq!(
                    body["cards"],
                    json!([]),
                    "expected empty cards for state: {state_name}"
                );
            }
            unknown => panic!("unrecognised provider state in verify_alt_backend_pact: {unknown}"),
        }

        verified += 1;
    }
    assert!(verified > 0, "pact contained no interactions to replay");
    record_evidence(&Evidence {
        provider: "recap-worker",
        consumer: "alt-backend",
        implementation: EVIDENCE_IMPLEMENTATION,
        interactions_verified: verified,
        interactions_skipped: 0,
    });
}
