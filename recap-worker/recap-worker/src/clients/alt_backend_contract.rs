//! Consumer-Driven Contract tests for recap-worker → alt-data-hub.
//!
//! Verifies the Connect-RPC `ListRecapArticles` / `BatchGetTagsByArticleIDs`
//! endpoints on `services.datahub.v1.DataHubService` (ADR-000954 D7 — the
//! `services.backend.v1.BackendInternalService` namespace is being retired;
//! RPC names and protojson wire shapes are unchanged, only the package /
//! service prefix of the path moves). Service-to-service endpoint — auth is
//! established at the mTLS transport layer, no user token required.
//! Path: POST `/services.datahub.v1.DataHubService/ListRecapArticles`, JSON body.
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

/// Paginated article fetch: POST /services.datahub.v1.DataHubService/ListRecapArticles → 200 OK
#[tokio::test]
#[ignore = "CDC contract test"]
async fn contract_alt_backend_recap_articles() {
    let pact = PactBuilder::new("recap-worker", "alt-backend")
        .interaction("a paginated recap articles request", "", |mut i| {
            i.given("articles exist in the recap window");
            i.request.method("POST");
            i.request
                .path("/services.datahub.v1.DataHubService/ListRecapArticles");
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

/// Batch tag fetch: POST /services.datahub.v1.DataHubService/BatchGetTagsByArticleIDs → 200 OK
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
                .path("/services.datahub.v1.DataHubService/BatchGetTagsByArticleIDs");
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

/// Paginated feeds in window fetch: POST /services.datahub.v1.DataHubService/ListFeedsInWindow → 200 OK
#[tokio::test]
#[ignore = "CDC contract test"]
async fn contract_alt_backend_list_feeds_in_window() {
    let pact = PactBuilder::new("recap-worker", "alt-backend")
        .interaction("a paginated feeds in window request", "", |mut i| {
            i.given("feeds exist in the window");
            i.request.method("POST");
            i.request
                .path("/services.datahub.v1.DataHubService/ListFeedsInWindow");
            i.request.content_type("application/json");
            i.request.json_body(json_pattern!({
                "from": like!("2026-09-18T17:00:00Z"),
                "to": like!("2026-09-21T17:00:00Z"),
                "page": like!(1i64),
                "pageSize": like!(500i64),
            }));
            i.response.status(200);
            i.response.content_type("application/json");
            i.response.json_body(json_pattern!({
                "feeds": each_like!(json_pattern!({
                    "id": like!("00000000-0000-0000-0000-000000000001"),
                    "title": like!("Example headline"),
                    "description": like!("<p>Example lede.</p>"),
                    "websiteUrl": like!("https://example.com/post"),
                    "pubDate": like!("2026-09-20T10:00:00Z"),
                    "createdAt": like!("2026-09-20T10:05:00Z"),
                    "updatedAt": like!("2026-09-20T10:05:00Z"),
                    "isRead": like!(false),
                    "feedLinkId": like!("00000000-0000-0000-0000-000000000002"),
                })),
                "total": like!(1i64),
                "page": like!(1i64),
                "pageSize": like!(500i64),
                "hasMore": like!(false),
            }));
            i
        })
        .with_output_dir(PACT_DIR)
        .start_mock_server(None, None);

    let feeds = contract_client(pact.url().to_string())
        .fetch_all_feeds_in_window(ts("2026-09-18T17:00:00Z"), ts("2026-09-21T17:00:00Z"))
        .await
        .expect("fetch_all_feeds_in_window should succeed against the pact mock");

    assert!(!feeds.is_empty());
    assert_eq!(feeds[0].title, "Example headline");
    assert_eq!(feeds[0].website_url, "https://example.com/post");
    assert_eq!(
        feeds[0].description.as_deref(),
        Some("<p>Example lede.</p>")
    );
}

/// All read feed IDs with optional since: POST /services.datahub.v1.DataHubService/GetAllReadFeedIDs → 200 OK
#[tokio::test]
#[ignore = "CDC contract test"]
async fn contract_alt_backend_get_all_read_feed_ids() {
    let pact = PactBuilder::new("recap-worker", "alt-backend")
        .interaction("a get all read feed ids request with since", "", |mut i| {
            i.given("read feeds exist in the period");
            i.request.method("POST");
            i.request
                .path("/services.datahub.v1.DataHubService/GetAllReadFeedIDs");
            i.request.content_type("application/json");
            i.request.json_body(json_pattern!({
                "userId": like!("00000000-0000-0000-0000-000000000001"),
                "since": like!("2026-08-19T17:00:00Z"),
            }));
            i.response.status(200);
            i.response.content_type("application/json");
            i.response.json_body(json_pattern!({
                "readFeedIds": each_like!(like!("00000000-0000-0000-0000-000000000001")),
            }));
            i
        })
        .with_output_dir(PACT_DIR)
        .start_mock_server(None, None);

    let user_id = uuid::Uuid::parse_str("00000000-0000-0000-0000-000000000001").unwrap();
    let read_ids = contract_client(pact.url().to_string())
        .get_all_read_feed_ids(user_id, Some(ts("2026-08-19T17:00:00Z")))
        .await
        .expect("get_all_read_feed_ids should succeed against the pact mock");

    assert!(!read_ids.is_empty());
    assert_eq!(
        read_ids[0],
        uuid::Uuid::parse_str("00000000-0000-0000-0000-000000000001").unwrap()
    );
}
