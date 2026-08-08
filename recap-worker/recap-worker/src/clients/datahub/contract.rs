//! Consumer-Driven Contract test for recap-worker → alt-data-hub.
//!
//! Pins the `EnqueueNotification` wire format the notification-outbox relay
//! speaks. Recap completion has no other durable signal: without this call
//! alt-backend can only discover a finished recap by polling
//! `GET /v1/dashboard/recap_jobs`, so a silent wire-format drift here is a
//! feature that never fires rather than an error anybody sees.
//!
//! What this interaction pins beyond "the procedure exists":
//!
//!   - `dedupeKey` is derived from the business fact (`recap:<job_id>`), not
//!     minted per attempt. The relay is at-least-once; the provider's
//!     (dedupe_key, subscription_id) constraint is the only thing that turns a
//!     re-forward into a no-op, and it can only do that if the key repeats.
//!   - `payload` is protojson `bytes`, i.e. base64 — not a nested object. A
//!     client that sent the JSON inline would be accepted by no provider.
//!   - `occurredAt` / `expiresAt` are producer-supplied. The provider
//!     substitutes neither, so a consumer that omitted them would be recording
//!     a different fact (when the row was written, not when the recap became
//!     ready).
//!   - The response is a pair of counts, not an id: one enqueue fans out to
//!     every device of the user that still wants this kind.
//!
//! Run with: `cargo test --lib contract -- --ignored`.

use chrono::{TimeZone, Utc};
use pact_consumer::prelude::*;
use uuid::Uuid;

use super::client::{DataHubClient, NotificationEnqueue};

const PACT_DIR: &str = "../../pacts";

#[tokio::test]
#[ignore = "CDC contract test — run with `cargo test --lib contract -- --ignored`"]
async fn contract_datahub_enqueue_notification_recap_ready() {
    let user_id = Uuid::parse_str("3f2504e0-4f89-11d3-9a0c-0305e82c3301").unwrap();
    let job_id = Uuid::parse_str("7d2a1c34-0a5f-4e51-9b1f-1a2b3c4d5e6f").unwrap();
    let occurred_at = Utc.with_ymd_and_hms(2026, 8, 1, 9, 30, 0).unwrap();
    let expires_at = Utc.with_ymd_and_hms(2026, 8, 2, 9, 30, 0).unwrap();

    // The notification is a signal to come and look, never a delivery
    // channel: a type discriminator and a navigate target, no recap text.
    let payload = serde_json::json!({"kind": "recap_ready", "url": "/recap"});
    let expected_payload_b64 = super::client::encode_payload(&payload);
    let dedupe_key = format!("recap:{job_id}");

    let pact = PactBuilder::new("recap-worker", "alt-data-hub")
        .interaction(
            "an EnqueueNotification request from the recap-worker outbox relay",
            "",
            |mut i| {
                i.given("alt-data-hub accepts notification enqueues");
                i.request.method("POST");
                i.request
                    .path("/services.datahub.v1.DataHubService/EnqueueNotification");
                i.request.content_type("application/json");
                i.request.json_body(json_pattern!({
                    "dedupeKey": like!(dedupe_key.clone()),
                    "userId": like!(user_id.to_string()),
                    "kind": like!("recap_ready"),
                    "payload": like!(expected_payload_b64.clone()),
                    "occurredAt": like!("2026-08-01T09:30:00.000000Z"),
                    "expiresAt": like!("2026-08-02T09:30:00.000000Z"),
                }));
                i.response.status(200);
                i.response.content_type("application/json");
                i.response.json_body(json_pattern!({
                    "deliveryCount": like!(2i64),
                    "supersededCount": like!(0i64),
                }));
                i
            },
        )
        .with_output_dir(PACT_DIR)
        .start_mock_server(None, None);

    let client = DataHubClient::new_for_test(pact.url().to_string());
    let outcome = client
        .enqueue_notification(&NotificationEnqueue {
            dedupe_key: &dedupe_key,
            user_id,
            kind: "recap_ready",
            payload: &payload,
            occurred_at,
            expires_at,
        })
        .await
        .expect("enqueue should succeed against the pact mock");

    assert_eq!(
        outcome.delivery_count, 2,
        "the fan-out count is what the relay logs; collapsing it loses the \
         difference between 'reached three devices' and 'reached one'"
    );
    assert_eq!(outcome.superseded_count, 0);
}
