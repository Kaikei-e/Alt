use std::time::Duration;

use anyhow::{Context, Result, anyhow};
use base64::Engine;
use chrono::{DateTime, SecondsFormat, Utc};
use reqwest::{Client, Url};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use tracing::debug;
use uuid::Uuid;

/// Connect-RPC procedure path. `services.datahub.v1.DataHubService` is the
/// data plane's only namespace since the 3-binary split (ADR-000954).
const ENQUEUE_NOTIFICATION_PATH: &str = "services.datahub.v1.DataHubService/EnqueueNotification";

/// One enqueue, addressed to every device of one user that still wants this
/// kind. Borrowed rather than owned because the relay builds it per row from
/// values it already holds.
#[derive(Debug)]
pub(crate) struct NotificationEnqueue<'a> {
    /// Derived from the business fact, never minted at send time — a retry at
    /// any layer has to produce the same key for the provider's
    /// (dedupe_key, subscription_id) constraint to collapse it.
    pub(crate) dedupe_key: &'a str,
    pub(crate) user_id: Uuid,
    pub(crate) kind: &'a str,
    /// Travels as protojson `bytes`, so it is base64 of the serialized JSON.
    pub(crate) payload: &'a Value,
    /// Business time. The provider substitutes no clock of its own.
    pub(crate) occurred_at: DateTime<Utc>,
    pub(crate) expires_at: DateTime<Utc>,
}

/// What the fan-out did. Counts rather than an id: one enqueue becomes one row
/// per subscription, and zero is an ordinary answer (no device registered, or
/// the kind turned off, or this enqueue was already relayed).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct EnqueueOutcome {
    pub(crate) delivery_count: i32,
    pub(crate) superseded_count: i32,
}

#[derive(Debug, Clone)]
pub(crate) struct DataHubClient {
    client: Client,
    base_url: Url,
}

impl DataHubClient {
    /// Build a client with its own plaintext `reqwest::Client`. Used only when
    /// `MTLS_ENFORCE` is off; the production path injects an mTLS client via
    /// [`DataHubClient::new_with_client`].
    pub(crate) fn new(
        base_url: impl Into<String>,
        connect_timeout: Duration,
        total_timeout: Duration,
    ) -> Result<Self> {
        let client = Client::builder()
            .connect_timeout(connect_timeout)
            .timeout(total_timeout)
            .build()
            .context("failed to build alt-data-hub HTTP client")?;
        Self::new_with_client(base_url, client)
    }

    pub(crate) fn new_with_client(base_url: impl Into<String>, client: Client) -> Result<Self> {
        let base_url = Url::parse(&base_url.into()).context("invalid alt-data-hub base URL")?;
        Ok(Self { client, base_url })
    }

    #[cfg(test)]
    pub(crate) fn new_for_test(base_url: impl Into<String>) -> Self {
        Self {
            client: Client::new(),
            base_url: Url::parse(&base_url.into()).expect("test base URL should parse"),
        }
    }

    /// Enqueue one notification. Auth is established at the TLS transport
    /// layer (mTLS); there is no token on this call.
    pub(crate) async fn enqueue_notification(
        &self,
        input: &NotificationEnqueue<'_>,
    ) -> Result<EnqueueOutcome> {
        if input.user_id.is_nil() {
            return Err(anyhow!(
                "EnqueueNotification requires a user_id (the fan-out is per user)"
            ));
        }
        if input.dedupe_key.is_empty() {
            return Err(anyhow!(
                "EnqueueNotification requires a dedupe_key (at-least-once delivery \
                 has nothing else to collapse a retry on)"
            ));
        }

        let url = self
            .base_url
            .join(ENQUEUE_NOTIFICATION_PATH)
            .context("failed to build EnqueueNotification URL")?;

        let body = EnqueueNotificationRequest {
            dedupe_key: input.dedupe_key.to_string(),
            user_id: input.user_id.to_string(),
            kind: input.kind.to_string(),
            payload: encode_payload(input.payload),
            occurred_at: proto_timestamp(input.occurred_at),
            expires_at: proto_timestamp(input.expires_at),
        };

        debug!(
            dedupe_key = %input.dedupe_key,
            user_id = %input.user_id,
            kind = %input.kind,
            "forwarding notification to alt-data-hub"
        );

        let response = self
            .client
            .post(url)
            .header("Content-Type", "application/json")
            .json(&body)
            .send()
            .await
            .context("EnqueueNotification request failed")?;

        let status = response.status();
        if !status.is_success() {
            let error_body = response.text().await.unwrap_or_default();
            return Err(anyhow!(
                "EnqueueNotification returned status {status}: {error_body}"
            ));
        }

        let wire: EnqueueNotificationResponse = response
            .json()
            .await
            .context("failed to deserialize EnqueueNotification response")?;

        Ok(EnqueueOutcome {
            delivery_count: wire.delivery_count,
            superseded_count: wire.superseded_count,
        })
    }
}

/// protojson renders `bytes` as standard base64. Kept public to the module so
/// the contract test computes the expectation the same way the client does,
/// rather than restating the encoding and staying green through a drift.
pub(crate) fn encode_payload(payload: &Value) -> String {
    base64::engine::general_purpose::STANDARD.encode(payload.to_string().as_bytes())
}

/// The payload of a `recap_ready` notification: a type discriminator and where
/// to go. Deliberately carries no recap text, headline or title — this is a
/// signal to come and look, not a delivery channel, and a push payload is
/// stored on third-party push infrastructure.
pub(crate) fn recap_ready_payload() -> Value {
    serde_json::json!({"kind": "recap_ready", "url": "/recap"})
}

/// protojson `google.protobuf.Timestamp` is RFC 3339 with a `Z` zone. chrono's
/// plain `to_rfc3339` emits `+00:00`, which parses but does not match the shape
/// the sibling producers pin in their pacts.
fn proto_timestamp(at: DateTime<Utc>) -> String {
    at.to_rfc3339_opts(SecondsFormat::Micros, true)
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct EnqueueNotificationRequest {
    dedupe_key: String,
    user_id: String,
    kind: String,
    payload: String,
    occurred_at: String,
    expires_at: String,
}

/// protojson omits zero-valued fields, so both counts must survive absence.
#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct EnqueueNotificationResponse {
    #[serde(default)]
    delivery_count: i32,
    #[serde(default)]
    superseded_count: i32,
}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::TimeZone;
    use wiremock::matchers::{body_json, method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    const RPC_PATH: &str = "/services.datahub.v1.DataHubService/EnqueueNotification";

    fn fixture_user() -> Uuid {
        Uuid::parse_str("3f2504e0-4f89-11d3-9a0c-0305e82c3301").unwrap()
    }

    #[tokio::test]
    async fn enqueue_notification_sends_protojson_and_reads_counts() {
        let server = MockServer::start().await;
        let payload = recap_ready_payload();

        let expected = serde_json::json!({
            "dedupeKey": "recap:7d2a1c34-0a5f-4e51-9b1f-1a2b3c4d5e6f",
            "userId": fixture_user().to_string(),
            "kind": "recap_ready",
            "payload": encode_payload(&payload),
            "occurredAt": "2026-08-01T09:30:00.000000Z",
            "expiresAt": "2026-08-02T09:30:00.000000Z",
        });

        Mock::given(method("POST"))
            .and(path(RPC_PATH))
            .and(body_json(&expected))
            .respond_with(
                ResponseTemplate::new(200)
                    .set_body_json(serde_json::json!({"deliveryCount": 3, "supersededCount": 1})),
            )
            .mount(&server)
            .await;

        let client = DataHubClient::new_for_test(server.uri());
        let outcome = client
            .enqueue_notification(&NotificationEnqueue {
                dedupe_key: "recap:7d2a1c34-0a5f-4e51-9b1f-1a2b3c4d5e6f",
                user_id: fixture_user(),
                kind: "recap_ready",
                payload: &payload,
                occurred_at: Utc.with_ymd_and_hms(2026, 8, 1, 9, 30, 0).unwrap(),
                expires_at: Utc.with_ymd_and_hms(2026, 8, 2, 9, 30, 0).unwrap(),
            })
            .await
            .expect("enqueue should succeed");

        assert_eq!(outcome.delivery_count, 3);
        assert_eq!(outcome.superseded_count, 1);
    }

    #[tokio::test]
    async fn enqueue_notification_treats_absent_counts_as_zero() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path(RPC_PATH))
            .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({})))
            .mount(&server)
            .await;

        let payload = recap_ready_payload();
        let client = DataHubClient::new_for_test(server.uri());
        let outcome = client
            .enqueue_notification(&NotificationEnqueue {
                dedupe_key: "recap:abc",
                user_id: fixture_user(),
                kind: "recap_ready",
                payload: &payload,
                occurred_at: Utc.with_ymd_and_hms(2026, 8, 1, 9, 30, 0).unwrap(),
                expires_at: Utc.with_ymd_and_hms(2026, 8, 2, 9, 30, 0).unwrap(),
            })
            .await
            .expect("an all-zero protojson response is a valid answer");

        assert_eq!(outcome.delivery_count, 0);
        assert_eq!(outcome.superseded_count, 0);
    }

    #[tokio::test]
    async fn enqueue_notification_propagates_server_error() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path(RPC_PATH))
            .respond_with(ResponseTemplate::new(503).set_body_string("unavailable"))
            .mount(&server)
            .await;

        let payload = recap_ready_payload();
        let client = DataHubClient::new_for_test(server.uri());
        let err = client
            .enqueue_notification(&NotificationEnqueue {
                dedupe_key: "recap:abc",
                user_id: fixture_user(),
                kind: "recap_ready",
                payload: &payload,
                occurred_at: Utc.with_ymd_and_hms(2026, 8, 1, 9, 30, 0).unwrap(),
                expires_at: Utc.with_ymd_and_hms(2026, 8, 2, 9, 30, 0).unwrap(),
            })
            .await
            .expect_err("a 503 must surface so the relay retries with backoff");

        assert!(
            err.to_string().contains("503"),
            "the status has to survive into the error: {err}"
        );
    }

    #[tokio::test]
    async fn enqueue_notification_rejects_empty_dedupe_key() {
        let payload = recap_ready_payload();
        let client = DataHubClient::new_for_test("http://localhost:1");
        let err = client
            .enqueue_notification(&NotificationEnqueue {
                dedupe_key: "",
                user_id: fixture_user(),
                kind: "recap_ready",
                payload: &payload,
                occurred_at: Utc.with_ymd_and_hms(2026, 8, 1, 9, 30, 0).unwrap(),
                expires_at: Utc.with_ymd_and_hms(2026, 8, 2, 9, 30, 0).unwrap(),
            })
            .await
            .expect_err("an empty dedupe key makes at-least-once delivery unbounded");

        assert!(format!("{err:#}").contains("dedupe_key"));
    }
}
