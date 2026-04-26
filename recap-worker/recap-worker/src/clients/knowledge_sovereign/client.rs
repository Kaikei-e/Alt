use anyhow::{Context, Result, anyhow};
use base64::Engine;
use chrono::{DateTime, Utc};
use reqwest::{Client, Url};
use serde::Serialize;
use serde_json::json;
use tracing::{debug, warn};
use uuid::Uuid;

/// Input for [`KnowledgeSovereignClient::emit_recap_topic_snapshotted`].
///
/// Every field is event-time bound; the client itself never reads
/// wall-clock time for business facts. The caller is responsible for
/// producing a stable `recap_topic_snapshot_id` (UUIDv7) — same input
/// from a retry must yield the same id so the sovereign dedupe index
/// can elide the duplicate.
// `dead_code` is allowed on this whole module because the persist-side
// integration that calls into this client lives in a follow-up PR; the
// types are exercised today only by the `--ignored` Pact contract test
// (`clients/knowledge_sovereign/contract.rs`) and the unit tests below.
#[allow(dead_code)]
#[derive(Debug, Clone)]
pub(crate) struct TopicSnapshottedInput {
    /// Owning user. Wave 4-B canonical contract §6.4.1 requires the
    /// event row to carry tenant_id + user_id.
    pub(crate) user_id: Uuid,
    pub(crate) tenant_id: Uuid,
    /// UUIDv7 of the topic snapshot the projector should treat as the
    /// stable identifier for deep-linking.
    pub(crate) recap_topic_snapshot_id: Uuid,
    /// Aggregate id; a stable user-scoped identifier (typically
    /// "recap-topic-snapshot:<user_id>:<window_start>") so the
    /// projector can index the event.
    pub(crate) aggregate_id: String,
    /// Top terms produced by the recap clustering stage. Persisted
    /// verbatim into the event payload.
    pub(crate) top_terms: Vec<String>,
    pub(crate) cluster_id: i64,
    pub(crate) snapshot_window_start: DateTime<Utc>,
    pub(crate) snapshot_window_end: DateTime<Utc>,
}

#[allow(dead_code)]
#[derive(Debug, Clone)]
pub(crate) struct KnowledgeSovereignClient {
    client: Client,
    base_url: Url,
}

#[allow(dead_code)]
impl KnowledgeSovereignClient {
    pub(crate) fn new(base_url: impl Into<String>) -> Result<Self> {
        let base_url =
            Url::parse(&base_url.into()).context("invalid knowledge-sovereign base URL")?;
        let client = Client::builder()
            .timeout(std::time::Duration::from_secs(5))
            .build()
            .context("failed to build knowledge-sovereign HTTP client")?;
        Ok(Self { client, base_url })
    }

    #[cfg(test)]
    pub(crate) fn new_for_test(base_url: impl Into<String>) -> Self {
        Self {
            client: Client::new(),
            base_url: Url::parse(&base_url.into()).unwrap(),
        }
    }

    /// Emit `recap.topic_snapshotted.v1` to knowledge-sovereign. The wire
    /// shape mirrors canonical contract §6.4.1 and is pinned by the
    /// `clients/knowledge_sovereign/contract.rs` Pact consumer test.
    /// Errors are returned to the caller; failure handling lives at the
    /// integration site (warn-and-continue per ADR-000853).
    pub(crate) async fn emit_recap_topic_snapshotted(
        &self,
        input: &TopicSnapshottedInput,
    ) -> Result<()> {
        if input.user_id.is_nil() {
            return Err(anyhow!(
                "TopicSnapshottedInput.user_id required (cross-user isolation)"
            ));
        }
        if input.tenant_id.is_nil() {
            return Err(anyhow!(
                "TopicSnapshottedInput.tenant_id required (Wave 4-A strictness)"
            ));
        }

        let url = self
            .base_url
            .join("/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent")
            .context("failed to build AppendKnowledgeEvent URL")?;

        let payload_bytes = serde_json::to_vec(&json!({
            "recap_topic_snapshot_id": input.recap_topic_snapshot_id.to_string(),
            "top_terms": input.top_terms,
            "cluster_id": input.cluster_id,
            "snapshot_window_start": input.snapshot_window_start.to_rfc3339(),
            "snapshot_window_end": input.snapshot_window_end.to_rfc3339(),
        }))
        .context("failed to serialize recap.topic_snapshotted payload")?;
        let payload_b64 = base64::engine::general_purpose::STANDARD.encode(&payload_bytes);

        let dedupe_key = format!(
            "recap.topic_snapshotted.v1:{}:{}",
            input.user_id, input.recap_topic_snapshot_id
        );

        let body = AppendKnowledgeEventRequest {
            event: KnowledgeEventWire {
                event_id: Uuid::new_v4().to_string(),
                occurred_at: input.snapshot_window_end.to_rfc3339(),
                tenant_id: input.tenant_id.to_string(),
                user_id: input.user_id.to_string(),
                actor_type: "service:recap-worker".to_string(),
                actor_id: "persist-stage".to_string(),
                event_type: "recap.topic_snapshotted.v1".to_string(),
                aggregate_type: "knowledge_loop_session".to_string(),
                aggregate_id: input.aggregate_id.clone(),
                dedupe_key,
                payload: payload_b64,
            },
        };

        debug!(
            user_id = %input.user_id,
            cluster_id = input.cluster_id,
            snapshot_id = %input.recap_topic_snapshot_id,
            "emitting recap.topic_snapshotted.v1 to knowledge-sovereign"
        );

        let response = self
            .client
            .post(url)
            .header("Content-Type", "application/json")
            .json(&body)
            .send()
            .await
            .context("AppendKnowledgeEvent request failed")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            warn!(
                status = %status,
                body = %body,
                "AppendKnowledgeEvent returned non-2xx (continuing)"
            );
            return Err(anyhow!(
                "AppendKnowledgeEvent returned status {status}: {body}"
            ));
        }

        Ok(())
    }
}

// Wire types match the protojson camelCase emission of
// services.sovereign.v1.AppendKnowledgeEventRequest. `payload` is the
// base64-encoded inner payload (proto field is `bytes`).

#[allow(dead_code)]
#[derive(Debug, Serialize)]
struct AppendKnowledgeEventRequest {
    event: KnowledgeEventWire,
}

#[allow(dead_code)]
#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct KnowledgeEventWire {
    event_id: String,
    occurred_at: String,
    tenant_id: String,
    user_id: String,
    actor_type: String,
    actor_id: String,
    event_type: String,
    aggregate_type: String,
    aggregate_id: String,
    dedupe_key: String,
    payload: String,
}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::TimeZone;

    #[test]
    fn input_rejects_nil_user_id() {
        let client = KnowledgeSovereignClient::new_for_test("http://localhost");
        let input = TopicSnapshottedInput {
            user_id: Uuid::nil(),
            tenant_id: Uuid::new_v4(),
            recap_topic_snapshot_id: Uuid::new_v4(),
            aggregate_id: "agg".into(),
            top_terms: vec!["a".into()],
            cluster_id: 0,
            snapshot_window_start: Utc.with_ymd_and_hms(2026, 4, 26, 0, 0, 0).unwrap(),
            snapshot_window_end: Utc.with_ymd_and_hms(2026, 4, 26, 1, 0, 0).unwrap(),
        };

        let rt = tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .unwrap();
        let result = rt.block_on(client.emit_recap_topic_snapshotted(&input));
        assert!(result.is_err(), "nil user_id must be rejected");
        assert!(
            format!("{:?}", result.unwrap_err()).contains("user_id"),
            "error must mention user_id"
        );
    }

    #[test]
    fn input_rejects_nil_tenant_id() {
        let client = KnowledgeSovereignClient::new_for_test("http://localhost");
        let input = TopicSnapshottedInput {
            user_id: Uuid::new_v4(),
            tenant_id: Uuid::nil(),
            recap_topic_snapshot_id: Uuid::new_v4(),
            aggregate_id: "agg".into(),
            top_terms: vec!["a".into()],
            cluster_id: 0,
            snapshot_window_start: Utc.with_ymd_and_hms(2026, 4, 26, 0, 0, 0).unwrap(),
            snapshot_window_end: Utc.with_ymd_and_hms(2026, 4, 26, 1, 0, 0).unwrap(),
        };

        let rt = tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .unwrap();
        let result = rt.block_on(client.emit_recap_topic_snapshotted(&input));
        assert!(result.is_err(), "nil tenant_id must be rejected");
        assert!(
            format!("{:?}", result.unwrap_err()).contains("tenant_id"),
            "error must mention tenant_id"
        );
    }
}
