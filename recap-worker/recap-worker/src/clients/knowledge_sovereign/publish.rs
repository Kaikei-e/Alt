//! `recap.topic_snapshotted.v1` publish helper.
//!
//! The persist stage hands this helper a list of confirmed clusters per
//! recap cycle. The helper:
//!
//!   - derives a stable snapshot id (UUIDv5 via [`deterministic_snapshot_id`])
//!     so retries are idempotent against the sovereign dedupe index;
//!   - composes a [`TopicSnapshottedInput`] per cluster;
//!   - calls [`KnowledgeSovereignClient::emit_recap_topic_snapshotted`] best
//!     effort: emit failure must NOT fail the recap pipeline (warn-and-continue);
//!   - skips clusters whose `top_terms` are empty (no-op signal — would only
//!     contribute zeros to Surface Planner v2's `topic_overlap_count`).
//!
//! Knowledge Loop Completion Phase 1 §2 (ADR-000853 follow-up).

use chrono::{DateTime, Utc};
use tracing::{debug, warn};
use uuid::Uuid;

use super::snapshot_id::deterministic_snapshot_id;
use super::{KnowledgeSovereignClient, TopicSnapshottedInput};

/// One cluster the persist stage has confirmed during a recap cycle. Held by
/// reference so the helper can stay independent of the heavyweight
/// `DispatchResult` type — easier to test, easier to evolve when more
/// signals get added.
#[derive(Debug, Clone)]
#[allow(dead_code)]
pub(crate) struct ConfirmedCluster {
    pub(crate) cluster_id: i64,
    pub(crate) top_terms: Vec<String>,
}

/// Outcome of a publish pass. `attempted` is the count after empty-terms
/// filtering; `succeeded` is how many emits completed without RPC error;
/// `failed` is the count of warn-and-continue rejections.
#[derive(Debug, Default, Clone, Copy, PartialEq, Eq)]
#[allow(dead_code)]
pub(crate) struct PublishOutcome {
    pub(crate) attempted: usize,
    pub(crate) succeeded: usize,
    pub(crate) failed: usize,
    pub(crate) skipped_empty_terms: usize,
}

/// Publish each confirmed cluster as `recap.topic_snapshotted.v1` against
/// the supplied client.
///
/// `user_id` and `tenant_id` come from the `JobContext` resolved by the
/// scheduler. The window endpoints come from the recap cycle itself, not
/// `Utc::now()`, so reproject of the resulting events stays event-time pure.
#[allow(dead_code)]
pub(crate) async fn publish_topic_snapshots(
    client: &KnowledgeSovereignClient,
    user_id: Uuid,
    tenant_id: Uuid,
    snapshot_window_start: DateTime<Utc>,
    snapshot_window_end: DateTime<Utc>,
    clusters: &[ConfirmedCluster],
) -> PublishOutcome {
    let mut outcome = PublishOutcome::default();

    if user_id.is_nil() || tenant_id.is_nil() {
        // Defence-in-depth. The client itself rejects nil ids, but bailing
        // out here keeps us from emitting a debug log per cluster for what
        // is fundamentally a JobContext-population bug.
        warn!(
            user_id = %user_id,
            tenant_id = %tenant_id,
            "skipping recap.topic_snapshotted.v1 publish: user/tenant not resolved"
        );
        return outcome;
    }

    for cluster in clusters {
        if cluster.top_terms.is_empty() {
            outcome.skipped_empty_terms += 1;
            continue;
        }
        outcome.attempted += 1;

        let snapshot_id = deterministic_snapshot_id(
            user_id,
            cluster.cluster_id,
            snapshot_window_start,
            snapshot_window_end,
        );
        let aggregate_id = format!(
            "recap-topic-snapshot:{}:{}",
            user_id,
            snapshot_window_start.to_rfc3339()
        );

        let input = TopicSnapshottedInput {
            user_id,
            tenant_id,
            recap_topic_snapshot_id: snapshot_id,
            aggregate_id,
            top_terms: cluster.top_terms.clone(),
            cluster_id: cluster.cluster_id,
            snapshot_window_start,
            snapshot_window_end,
        };

        match client.emit_recap_topic_snapshotted(&input).await {
            Ok(()) => {
                debug!(
                    user_id = %user_id,
                    cluster_id = cluster.cluster_id,
                    snapshot_id = %snapshot_id,
                    "recap.topic_snapshotted.v1 emit ok"
                );
                outcome.succeeded += 1;
            }
            Err(err) => {
                warn!(
                    user_id = %user_id,
                    cluster_id = cluster.cluster_id,
                    snapshot_id = %snapshot_id,
                    error = ?err,
                    "recap.topic_snapshotted.v1 emit failed (non-fatal)"
                );
                outcome.failed += 1;
            }
        }
    }

    outcome
}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::TimeZone;
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    fn fixture_user() -> Uuid {
        Uuid::parse_str("44444444-4444-4444-4444-444444444444").unwrap()
    }

    fn fixture_tenant() -> Uuid {
        Uuid::parse_str("11111111-1111-1111-1111-111111111111").unwrap()
    }

    fn fixture_window() -> (DateTime<Utc>, DateTime<Utc>) {
        (
            Utc.with_ymd_and_hms(2026, 5, 1, 0, 0, 0).unwrap(),
            Utc.with_ymd_and_hms(2026, 5, 8, 0, 0, 0).unwrap(),
        )
    }

    #[tokio::test]
    async fn publishes_one_event_per_non_empty_cluster() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path(
                "/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent",
            ))
            .respond_with(
                ResponseTemplate::new(200)
                    .insert_header("Content-Type", "application/json")
                    .set_body_string(r#"{"success":true,"eventSeq":1}"#),
            )
            .expect(2)
            .mount(&server)
            .await;
        let client = KnowledgeSovereignClient::new_for_test(server.uri());

        let (start, end) = fixture_window();
        let outcome = publish_topic_snapshots(
            &client,
            fixture_user(),
            fixture_tenant(),
            start,
            end,
            &[
                ConfirmedCluster {
                    cluster_id: 1,
                    top_terms: vec!["finance".into(), "energy".into()],
                },
                ConfirmedCluster {
                    cluster_id: 2,
                    top_terms: vec!["medicine".into()],
                },
            ],
        )
        .await;

        assert_eq!(
            outcome,
            PublishOutcome {
                attempted: 2,
                succeeded: 2,
                failed: 0,
                skipped_empty_terms: 0,
            }
        );
    }

    #[tokio::test]
    async fn skips_clusters_with_empty_top_terms() {
        // No mock mounted — wiremock returns 404 for every unmatched call,
        // so if we accidentally emit, we'd see it as a `failed` count, not
        // skipped. The expect(0) below pins zero RPCs for empty-terms
        // clusters: the helper must filter them client-side.
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .respond_with(ResponseTemplate::new(500))
            .expect(0)
            .mount(&server)
            .await;
        let client = KnowledgeSovereignClient::new_for_test(server.uri());

        let (start, end) = fixture_window();
        let outcome = publish_topic_snapshots(
            &client,
            fixture_user(),
            fixture_tenant(),
            start,
            end,
            &[ConfirmedCluster {
                cluster_id: 1,
                top_terms: vec![],
            }],
        )
        .await;

        assert_eq!(
            outcome,
            PublishOutcome {
                attempted: 0,
                succeeded: 0,
                failed: 0,
                skipped_empty_terms: 1,
            }
        );
    }

    #[tokio::test]
    async fn warn_and_continues_on_emit_failure() {
        let server = MockServer::start().await;
        // Every POST returns 500. The helper must still attempt every cluster
        // (warn-and-continue): a single emit failure must not abort the rest.
        Mock::given(method("POST"))
            .and(path(
                "/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent",
            ))
            .respond_with(ResponseTemplate::new(500).set_body_string("internal error"))
            .expect(2)
            .mount(&server)
            .await;
        let client = KnowledgeSovereignClient::new_for_test(server.uri());

        let (start, end) = fixture_window();
        let outcome = publish_topic_snapshots(
            &client,
            fixture_user(),
            fixture_tenant(),
            start,
            end,
            &[
                ConfirmedCluster {
                    cluster_id: 1,
                    top_terms: vec!["a".into()],
                },
                ConfirmedCluster {
                    cluster_id: 2,
                    top_terms: vec!["b".into()],
                },
            ],
        )
        .await;

        assert_eq!(outcome.attempted, 2);
        assert_eq!(outcome.succeeded, 0);
        assert_eq!(outcome.failed, 2);
    }

    #[tokio::test]
    async fn skips_publish_when_user_id_is_nil() {
        let server = MockServer::start().await;
        // ZERO RPC calls expected — defence-in-depth check fails closed.
        Mock::given(method("POST"))
            .respond_with(ResponseTemplate::new(500))
            .expect(0)
            .mount(&server)
            .await;
        let client = KnowledgeSovereignClient::new_for_test(server.uri());

        let (start, end) = fixture_window();
        let outcome = publish_topic_snapshots(
            &client,
            Uuid::nil(),
            fixture_tenant(),
            start,
            end,
            &[ConfirmedCluster {
                cluster_id: 1,
                top_terms: vec!["a".into()],
            }],
        )
        .await;

        assert_eq!(outcome, PublishOutcome::default());
    }

    #[tokio::test]
    async fn skips_publish_when_tenant_id_is_nil() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .respond_with(ResponseTemplate::new(500))
            .expect(0)
            .mount(&server)
            .await;
        let client = KnowledgeSovereignClient::new_for_test(server.uri());

        let (start, end) = fixture_window();
        let outcome = publish_topic_snapshots(
            &client,
            fixture_user(),
            Uuid::nil(),
            start,
            end,
            &[ConfirmedCluster {
                cluster_id: 1,
                top_terms: vec!["a".into()],
            }],
        )
        .await;

        assert_eq!(outcome, PublishOutcome::default());
    }

    /// Replay safety: same inputs → same deterministic snapshot id, and that
    /// id must actually reach the wire as part of `dedupe_key` (per
    /// `emit_recap_topic_snapshotted`'s `"recap.topic_snapshotted.v1:{user_id}:{snapshot_id}"`
    /// format) so the sovereign side's dedupe index can catch a retried emit.
    ///
    /// Before this test asserted anything, both `publish_topic_snapshots`
    /// return values were discarded via `let _ =`, and the only assertion
    /// compared two *independent* calls to the pure `deterministic_snapshot_id`
    /// helper — which is trivially equal to itself and would stay green even
    /// if `publish_topic_snapshots` never called that helper at all, or used
    /// `Uuid::new_v4()` per emit. The mock below pins the exact dedupe_key on
    /// the outgoing request for both calls, so a regression to a
    /// non-deterministic id would surface as an unmatched request /
    /// unmet `.expect(2)` instead of passing silently.
    #[tokio::test]
    async fn retry_yields_same_snapshot_id() {
        let (start, end) = fixture_window();
        let cluster = ConfirmedCluster {
            cluster_id: 7,
            top_terms: vec!["a".into()],
        };
        let expected_snapshot_id = deterministic_snapshot_id(fixture_user(), 7, start, end);
        let expected_dedupe_key = format!(
            "recap.topic_snapshotted.v1:{}:{}",
            fixture_user(),
            expected_snapshot_id
        );

        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path(
                "/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent",
            ))
            .and(wiremock::matchers::body_string_contains(
                expected_dedupe_key,
            ))
            .respond_with(
                ResponseTemplate::new(200)
                    .insert_header("Content-Type", "application/json")
                    .set_body_string(r#"{"success":true,"eventSeq":1}"#),
            )
            .expect(2)
            .mount(&server)
            .await;
        let client = KnowledgeSovereignClient::new_for_test(server.uri());

        let outcome_a = publish_topic_snapshots(
            &client,
            fixture_user(),
            fixture_tenant(),
            start,
            end,
            std::slice::from_ref(&cluster),
        )
        .await;
        let outcome_b = publish_topic_snapshots(
            &client,
            fixture_user(),
            fixture_tenant(),
            start,
            end,
            std::slice::from_ref(&cluster),
        )
        .await;

        assert_eq!(
            outcome_a.succeeded, 1,
            "first publish must reach the mock using the deterministic dedupe_key"
        );
        assert_eq!(
            outcome_b.succeeded, 1,
            "retried publish must reach the mock using the SAME deterministic dedupe_key"
        );

        let snap_a = deterministic_snapshot_id(fixture_user(), 7, start, end);
        let snap_b = deterministic_snapshot_id(fixture_user(), 7, start, end);
        assert_eq!(snap_a, snap_b);
        // `server` drop verifies `.expect(2)`: both calls matched the pinned
        // dedupe_key, not just any POST.
    }
}
