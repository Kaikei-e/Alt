//! Consumer-Driven Contract tests for recap-worker → alt-data-hub.
//!
//! Verifies the Connect-RPC `ListRecapArticles` / `BatchGetTagsByArticleIDs`
//! endpoints on `alt.datahub.v1.DataHubService` (ADR-000954 D7 — the
//! `services.backend.v1.BackendInternalService` namespace is being retired;
//! RPC names and protojson wire shapes are unchanged, only the package /
//! service prefix of the path moves). Service-to-service endpoint — auth is
//! established at the mTLS transport layer, no user token required.
//! Path: POST `/alt.datahub.v1.DataHubService/ListRecapArticles`, JSON body.
//!
//! These tests drive the *production* `AltBackendClient` rather than a
//! hand-rolled reqwest call. That is deliberate: a contract test that restates
//! the path on both the expectation and the request side is self-consistent
//! and stays green no matter which path the shipped client actually uses —
//! exactly the silent-drift failure mode CLAUDE.md Rule 7 / ADR-000928 warn
//! about. Driving the real client makes the pact the single place the RPC path
//! is asserted.

use std::time::Duration;

use chrono::{DateTime, Utc};
use pact_consumer::prelude::*;

use crate::clients::alt_backend::{AltBackendClient, AltBackendConfig};

const PACT_DIR: &str = "../../pacts";

fn contract_client(base_url: String) -> AltBackendClient {
    AltBackendClient::new(AltBackendConfig {
        base_url,
        connect_timeout: Duration::from_secs(3),
        total_timeout: Duration::from_secs(30),
    })
    .expect("alt-data-hub client should build")
}

fn ts(raw: &str) -> DateTime<Utc> {
    DateTime::parse_from_rfc3339(raw)
        .expect("fixture timestamp should parse")
        .with_timezone(&Utc)
}

/// Paginated article fetch: POST /alt.datahub.v1.DataHubService/ListRecapArticles → 200 OK
#[tokio::test]
#[ignore = "CDC contract test"]
async fn contract_alt_backend_recap_articles() {
    let pact = PactBuilder::new("recap-worker", "alt-backend")
        .interaction("a paginated recap articles request", "", |mut i| {
            i.given("articles exist in the recap window");
            i.request.method("POST");
            i.request
                .path("/alt.datahub.v1.DataHubService/ListRecapArticles");
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

    let articles = contract_client(pact.url().to_string())
        .fetch_articles(ts("2026-03-19T00:00:00Z"), ts("2026-03-26T00:00:00Z"))
        .await
        .expect("fetch_articles should succeed against the pact mock");

    assert!(!articles.is_empty());
    assert_eq!(articles[0].article_id, "art-001");
}

/// Batch tag fetch: POST /alt.datahub.v1.DataHubService/BatchGetTagsByArticleIDs → 200 OK
///
/// Replaces the former `recap-worker → tag-generator /api/v1/tags/batch`
/// contract per ADR-000241 / ADR-000397 (Shared Database anti-pattern
/// elimination; the data-hub is the sole data owner of articles /
/// article_tags / feed_tags).
#[tokio::test]
#[ignore = "CDC contract test"]
async fn contract_alt_backend_batch_get_tags_by_article_ids() {
    let pact = PactBuilder::new("recap-worker", "alt-backend")
        .interaction("a batch tags request by article ids", "", |mut i| {
            i.given("tags exist for the requested articles");
            i.request.method("POST");
            i.request
                .path("/alt.datahub.v1.DataHubService/BatchGetTagsByArticleIDs");
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

    let tags = contract_client(pact.url().to_string())
        .batch_get_tags_by_article_ids(&["art-001".to_string()])
        .await
        .expect("batch_get_tags_by_article_ids should succeed against the pact mock");

    let signals = tags
        .get("art-001")
        .expect("response should carry tags for the requested article id");
    assert!(!signals.is_empty());
}
