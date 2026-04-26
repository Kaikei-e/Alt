//! Consumer-Driven Contract test for recap-worker → knowledge-sovereign.
//!
//! Pins the `AppendKnowledgeEvent` wire format for the
//! `recap.topic_snapshotted.v1` emit path (Wave 4-B, ADR-000853 /
//! canonical contract §6.4.1). Surface Planner v2's `topic_overlap`
//! resolver depends on this event being persisted with the exact
//! camelCase shape pinned here, otherwise the projector silently
//! drops the signal and the Continue / Decide planes never light up.
//!
//! Run with: `cargo test contract -- --ignored`. The test is
//! `#[ignore]`d so it doesn't run on every `cargo test`; the dedicated
//! contract step in CI publishes the generated pact.

use base64::Engine;
use chrono::TimeZone;
use chrono::Utc;
use pact_consumer::prelude::*;
use uuid::Uuid;

use super::client::{KnowledgeSovereignClient, TopicSnapshottedInput};

const PACT_DIR: &str = "../../pacts";

#[tokio::test]
#[ignore = "CDC contract test — run with `cargo test contract -- --ignored`"]
async fn contract_recap_topic_snapshotted_v1() {
    let user_id = Uuid::parse_str("44444444-4444-4444-4444-444444444444").unwrap();
    let tenant_id = Uuid::parse_str("11111111-1111-1111-1111-111111111111").unwrap();
    let snapshot_id = Uuid::parse_str("01939001-aaaa-7000-8000-000000000001").unwrap();
    let window_start = Utc.with_ymd_and_hms(2026, 4, 26, 0, 0, 0).unwrap();
    let window_end = Utc.with_ymd_and_hms(2026, 4, 26, 1, 0, 0).unwrap();
    let aggregate_id = format!(
        "recap-topic-snapshot:{}:{}",
        user_id,
        window_start.to_rfc3339()
    );

    // Compute the expected base64 payload exactly as the client builds
    // it. Keys must match the inner JSON shape so a wire-format drift
    // (eg renaming snapshot_window_end) is caught here.
    let expected_payload_bytes = serde_json::to_vec(&serde_json::json!({
        "recap_topic_snapshot_id": snapshot_id.to_string(),
        "top_terms": ["ai", "llm"],
        "cluster_id": 7i64,
        "snapshot_window_start": window_start.to_rfc3339(),
        "snapshot_window_end": window_end.to_rfc3339(),
    }))
    .unwrap();
    let expected_payload_b64 =
        base64::engine::general_purpose::STANDARD.encode(&expected_payload_bytes);

    let expected_dedupe_key = format!("recap.topic_snapshotted.v1:{}:{}", user_id, snapshot_id);

    let pact = PactBuilder::new("recap-worker", "knowledge-sovereign")
        .interaction(
            "an AppendKnowledgeEvent request for recap.topic_snapshotted.v1",
            "",
            |mut i| {
                i.given("sovereign accepts append-only Loop transition events");
                i.request.method("POST");
                i.request
                    .path("/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent");
                i.request.content_type("application/json");
                i.request.json_body(json_pattern!({
                    "event": json_pattern!({
                        "eventId": term!(
                            r"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$",
                            "00000000-0000-0000-0000-000000000000"
                        ),
                        "occurredAt": like!(window_end.to_rfc3339()),
                        "tenantId": like!(tenant_id.to_string()),
                        "userId": like!(user_id.to_string()),
                        "actorType": like!("service:recap-worker"),
                        "actorId": like!("persist-stage"),
                        "eventType": like!("recap.topic_snapshotted.v1"),
                        "aggregateType": like!("knowledge_loop_session"),
                        "aggregateId": like!(aggregate_id.clone()),
                        "dedupeKey": like!(expected_dedupe_key.clone()),
                        "payload": like!(expected_payload_b64.clone()),
                    }),
                }));
                i.response.status(200);
                i.response.content_type("application/json");
                i.response.json_body(json_pattern!({
                    "success": like!(true),
                }));
                i
            },
        )
        .with_output_dir(PACT_DIR)
        .start_mock_server(None, None);

    let url = pact.url();
    let client = KnowledgeSovereignClient::new(url.to_string()).expect("client should build");
    let input = TopicSnapshottedInput {
        user_id,
        tenant_id,
        recap_topic_snapshot_id: snapshot_id,
        aggregate_id,
        top_terms: vec!["ai".to_string(), "llm".to_string()],
        cluster_id: 7,
        snapshot_window_start: window_start,
        snapshot_window_end: window_end,
    };

    client
        .emit_recap_topic_snapshotted(&input)
        .await
        .expect("emit should succeed against mock provider");
}
