use anyhow::{Context, Result, anyhow};
use base64::Engine;
use chrono::{DateTime, Utc};
use reqwest::{Client, Url};
use serde::{Deserialize, Serialize};
use serde_json::json;
use tracing::{debug, error, warn};
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
#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct ReadingSourceEntry {
    pub(crate) article_id: Uuid,
    pub(crate) first_read_at: DateTime<Utc>,
    pub(crate) read_rule_version: i32,
}

#[allow(dead_code)]
#[derive(Debug, Clone)]
pub(crate) struct ThemeVersionCreatedInput {
    pub(crate) user_id: Uuid,
    pub(crate) tenant_id: Uuid,
    pub(crate) theme_id: Uuid,
    pub(crate) version: i32,
    pub(crate) title: String,
    pub(crate) member_article_ids: Vec<Uuid>,
    pub(crate) created_at: DateTime<Utc>,
}

#[allow(dead_code)]
#[derive(Debug, Clone)]
pub(crate) struct KnowledgeSovereignClient {
    client: Client,
    base_url: Url,
    token: Option<String>,
}

#[allow(dead_code)]
impl KnowledgeSovereignClient {
    pub(crate) fn new(base_url: impl Into<String>, token: Option<String>) -> Result<Self> {
        let base_url =
            Url::parse(&base_url.into()).context("invalid knowledge-sovereign base URL")?;
        let client = Client::builder()
            .timeout(std::time::Duration::from_secs(5))
            .build()
            .context("failed to build knowledge-sovereign HTTP client")?;
        Ok(Self {
            client,
            base_url,
            token,
        })
    }

    #[cfg(test)]
    pub(crate) fn new_for_test(base_url: impl Into<String>) -> Self {
        Self {
            client: Client::new(),
            base_url: Url::parse(&base_url.into()).unwrap(),
            token: None,
        }
    }

    #[cfg(test)]
    pub(crate) fn new_for_test_with_token(
        base_url: impl Into<String>,
        token: Option<String>,
    ) -> Self {
        Self {
            client: Client::new(),
            base_url: Url::parse(&base_url.into()).unwrap(),
            token,
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

        self.send_append_knowledge_event(&body).await
    }

    pub(crate) async fn emit_theme_version_created(
        &self,
        input: &ThemeVersionCreatedInput,
    ) -> Result<()> {
        if input.user_id.is_nil() {
            return Err(anyhow!("ThemeVersionCreatedInput.user_id required"));
        }
        if input.tenant_id.is_nil() {
            return Err(anyhow!("ThemeVersionCreatedInput.tenant_id required"));
        }
        if input.theme_id.is_nil() {
            return Err(anyhow!("ThemeVersionCreatedInput.theme_id required"));
        }
        if input.version < 1 {
            return Err(anyhow!(
                "ThemeVersionCreatedInput.version must be >= 1, got {}",
                input.version
            ));
        }
        if input.title.trim().is_empty() {
            return Err(anyhow!("ThemeVersionCreatedInput.title must not be empty"));
        }

        let created_at_rfc3339 = input.created_at.to_rfc3339();
        let payload = ThemeVersionCreatedPayload {
            theme_id: input.theme_id,
            version: input.version,
            title: &input.title,
            member_article_ids: &input.member_article_ids,
            article_count: input.member_article_ids.len(),
            created_at: &created_at_rfc3339,
        };
        let payload_bytes =
            serde_json::to_vec(&payload).context("failed to serialize theme version payload")?;
        let payload_b64 = base64::engine::general_purpose::STANDARD.encode(&payload_bytes);
        let dedupe_key = format!(
            "reading.theme_version_created:{}:{}",
            input.theme_id, input.version
        );

        let body = AppendKnowledgeEventRequest {
            event: KnowledgeEventWire {
                event_id: Uuid::new_v4().to_string(),
                occurred_at: created_at_rfc3339.clone(),
                tenant_id: input.tenant_id.to_string(),
                user_id: input.user_id.to_string(),
                actor_type: "service:recap-worker".to_string(),
                actor_id: "themes".to_string(),
                event_type: "reading.theme_version_created.v1".to_string(),
                aggregate_type: "reading_theme".to_string(),
                aggregate_id: input.theme_id.to_string(),
                dedupe_key,
                payload: payload_b64,
            },
        };

        debug!(
            user_id = %input.user_id,
            theme_id = %input.theme_id,
            version = input.version,
            "emitting reading.theme_version_created.v1 to knowledge-sovereign"
        );

        self.send_append_knowledge_event(&body).await
    }

    pub(crate) async fn list_reading_sources_in_window(
        &self,
        user_id: Uuid,
        from: DateTime<Utc>,
        to: DateTime<Utc>,
    ) -> Result<Vec<ReadingSourceEntry>> {
        if user_id.is_nil() {
            return Err(anyhow!("ListReadingSourcesInWindow requires user_id"));
        }
        if from > to {
            return Err(anyhow!(
                "ListReadingSourcesInWindow requires from <= to ({from} > {to})"
            ));
        }

        let url = self
            .base_url
            .join("/services.sovereign.v1.KnowledgeSovereignService/ListReadingSourcesInWindow")
            .context("failed to build ListReadingSourcesInWindow URL")?;

        let body = ListReadingSourcesInWindowRequest {
            user_id: user_id.to_string(),
            from: from.to_rfc3339(),
            to: to.to_rfc3339(),
        };

        let mut request = self
            .client
            .post(url)
            .header("Content-Type", "application/json");
        if let Some(ref token) = self.token {
            request = request.header("Authorization", format!("Bearer {token}"));
        }

        let response = request
            .json(&body)
            .send()
            .await
            .context("ListReadingSourcesInWindow request failed")?;

        if !response.status().is_success() {
            let status = response.status();
            let err_body = response.text().await.unwrap_or_default();
            return Err(anyhow!(
                "ListReadingSourcesInWindow returned status {status}: {err_body}"
            ));
        }

        let wire: ListReadingSourcesInWindowResponse = response
            .json()
            .await
            .context("failed to deserialize ListReadingSourcesInWindow response")?;

        let mut out = Vec::with_capacity(wire.entries.len());
        for entry in wire.entries {
            let article_id = Uuid::parse_str(&entry.article_id).with_context(|| {
                format!(
                    "invalid articleId '{}' in ListReadingSourcesInWindow response",
                    entry.article_id
                )
            })?;
            let first_read_at = DateTime::parse_from_rfc3339(&entry.first_read_at)
                .with_context(|| {
                    format!(
                        "invalid firstReadAt '{}' in ListReadingSourcesInWindow response",
                        entry.first_read_at
                    )
                })?
                .with_timezone(&Utc);
            out.push(ReadingSourceEntry {
                article_id,
                first_read_at,
                read_rule_version: entry.read_rule_version,
            });
        }
        Ok(out)
    }

    async fn send_append_knowledge_event(&self, body: &AppendKnowledgeEventRequest) -> Result<()> {
        let url = self
            .base_url
            .join("/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent")
            .context("failed to build AppendKnowledgeEvent URL")?;

        let mut request = self
            .client
            .post(url)
            .header("Content-Type", "application/json");

        if let Some(ref token) = self.token {
            request = request.header("Authorization", format!("Bearer {token}"));
        }

        let response = request
            .json(body)
            .send()
            .await
            .context("AppendKnowledgeEvent request failed")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            if status == reqwest::StatusCode::UNAUTHORIZED
                || status == reqwest::StatusCode::FORBIDDEN
            {
                error!(
                    status = %status,
                    body = %body,
                    "AppendKnowledgeEvent unauthenticated or forbidden; check RECAP_KNOWLEDGE_SOVEREIGN_TOKEN_FILE"
                );
            } else {
                warn!(
                    status = %status,
                    body = %body,
                    "AppendKnowledgeEvent returned non-2xx (continuing)"
                );
            }
            return Err(anyhow!(
                "AppendKnowledgeEvent returned status {status}: {body}"
            ));
        }

        Ok(())
    }
}

#[derive(Debug, Serialize)]
struct ThemeVersionCreatedPayload<'a> {
    theme_id: Uuid,
    version: i32,
    title: &'a str,
    member_article_ids: &'a [Uuid],
    article_count: usize,
    created_at: &'a str,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct ListReadingSourcesInWindowRequest {
    user_id: String,
    from: String,
    to: String,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ListReadingSourcesInWindowResponse {
    #[serde(default)]
    entries: Vec<ReadingSourceEntryWire>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ReadingSourceEntryWire {
    article_id: String,
    first_read_at: String,
    #[serde(default)]
    read_rule_version: i32,
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

    #[tokio::test]
    async fn emits_with_authorization_header() {
        use wiremock::matchers::{header, method, path};
        use wiremock::{Mock, MockServer, ResponseTemplate};

        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path(
                "/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent",
            ))
            .and(header(
                "Authorization",
                "Bearer test-sovereign-token-wire-test-1234",
            ))
            .respond_with(
                ResponseTemplate::new(200)
                    .insert_header("Content-Type", "application/json")
                    .set_body_string(r#"{"eventSeq":"123"}"#),
            )
            .expect(1)
            .mount(&server)
            .await;

        let client = KnowledgeSovereignClient::new_for_test_with_token(
            server.uri(),
            Some("test-sovereign-token-wire-test-1234".to_string()),
        );

        let input = TopicSnapshottedInput {
            user_id: Uuid::new_v4(),
            tenant_id: Uuid::new_v4(),
            recap_topic_snapshot_id: Uuid::new_v4(),
            aggregate_id: "agg".into(),
            top_terms: vec!["a".into()],
            cluster_id: 0,
            snapshot_window_start: Utc.with_ymd_and_hms(2026, 4, 26, 0, 0, 0).unwrap(),
            snapshot_window_end: Utc.with_ymd_and_hms(2026, 4, 26, 1, 0, 0).unwrap(),
        };

        let result = client.emit_recap_topic_snapshotted(&input).await;
        assert!(result.is_ok(), "emit with auth header should succeed");
    }

    #[tokio::test]
    async fn list_reading_sources_rejects_nil_user_and_inverted_window() {
        let client = KnowledgeSovereignClient::new_for_test("http://localhost:1");
        let t1 = Utc.with_ymd_and_hms(2026, 11, 18, 0, 0, 0).unwrap();
        let t2 = Utc.with_ymd_and_hms(2026, 11, 19, 0, 0, 0).unwrap();

        let err_nil = client
            .list_reading_sources_in_window(Uuid::nil(), t1, t2)
            .await
            .expect_err("nil user_id must fail");
        assert!(err_nil.to_string().contains("user_id"));

        let err_window = client
            .list_reading_sources_in_window(Uuid::new_v4(), t2, t1)
            .await
            .expect_err("from > to must fail");
        assert!(err_window.to_string().contains("from <= to"));
    }

    #[tokio::test]
    async fn list_reading_sources_parses_entries_and_empty_response() {
        use wiremock::matchers::{body_json, header, method, path};
        use wiremock::{Mock, MockServer, ResponseTemplate};

        let server = MockServer::start().await;
        let user_id = Uuid::new_v4();
        let article_id = Uuid::new_v4();
        let from = Utc.with_ymd_and_hms(2026, 11, 1, 0, 0, 0).unwrap();
        let to = Utc.with_ymd_and_hms(2026, 11, 19, 0, 0, 0).unwrap();
        let first_read = Utc.with_ymd_and_hms(2026, 11, 10, 8, 30, 0).unwrap();

        Mock::given(method("POST"))
            .and(path(
                "/services.sovereign.v1.KnowledgeSovereignService/ListReadingSourcesInWindow",
            ))
            .and(header("Authorization", "Bearer tok"))
            .and(body_json(serde_json::json!({
                "userId": user_id.to_string(),
                "from": from.to_rfc3339(),
                "to": to.to_rfc3339(),
            })))
            .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
                "entries": [{
                    "articleId": article_id.to_string(),
                    "firstReadAt": first_read.to_rfc3339(),
                    "readRuleVersion": 2
                }]
            })))
            .expect(1)
            .mount(&server)
            .await;

        let client =
            KnowledgeSovereignClient::new_for_test_with_token(server.uri(), Some("tok".into()));
        let entries = client
            .list_reading_sources_in_window(user_id, from, to)
            .await
            .expect("list should succeed");
        assert_eq!(entries.len(), 1);
        assert_eq!(entries[0].article_id, article_id);
        assert_eq!(entries[0].first_read_at, first_read);
        assert_eq!(entries[0].read_rule_version, 2);

        let empty_server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path(
                "/services.sovereign.v1.KnowledgeSovereignService/ListReadingSourcesInWindow",
            ))
            .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({})))
            .mount(&empty_server)
            .await;
        let empty_client = KnowledgeSovereignClient::new_for_test(empty_server.uri());
        let empty = empty_client
            .list_reading_sources_in_window(user_id, from, to)
            .await
            .expect("empty protojson response must succeed");
        assert!(empty.is_empty());
    }

    #[tokio::test]
    async fn emit_theme_version_created_is_byte_stable_across_retries() {
        use std::sync::{Arc, Mutex};
        use wiremock::matchers::{method, path};
        use wiremock::{Mock, MockServer, Request, ResponseTemplate};

        let captured: Arc<Mutex<Vec<serde_json::Value>>> = Arc::new(Mutex::new(Vec::new()));
        let captured_clone = Arc::clone(&captured);

        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path(
                "/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent",
            ))
            .respond_with(move |req: &Request| {
                let body: serde_json::Value = req.body_json().unwrap();
                captured_clone.lock().unwrap().push(body);
                ResponseTemplate::new(200).set_body_json(serde_json::json!({"eventSeq": "10"}))
            })
            .expect(2)
            .mount(&server)
            .await;

        let client = KnowledgeSovereignClient::new_for_test(server.uri());
        let input = ThemeVersionCreatedInput {
            user_id: Uuid::new_v4(),
            tenant_id: Uuid::new_v4(),
            theme_id: Uuid::new_v4(),
            version: 1,
            title: "Theme One".to_string(),
            member_article_ids: vec![Uuid::new_v4()],
            created_at: Utc.with_ymd_and_hms(2026, 11, 18, 2, 0, 0).unwrap(),
        };

        client.emit_theme_version_created(&input).await.unwrap();
        client.emit_theme_version_created(&input).await.unwrap();

        let bodies = captured.lock().unwrap();
        assert_eq!(bodies.len(), 2);
        assert_eq!(
            bodies[0]["event"]["occurredAt"],
            bodies[1]["event"]["occurredAt"]
        );
        assert_eq!(
            bodies[0]["event"]["dedupeKey"],
            bodies[1]["event"]["dedupeKey"]
        );
        assert_eq!(bodies[0]["event"]["payload"], bodies[1]["event"]["payload"]);
    }
}
