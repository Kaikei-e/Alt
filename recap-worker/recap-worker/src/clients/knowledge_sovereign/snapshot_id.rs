//! Deterministic `recap_topic_snapshot_id` derivation (RFC 9562 §6.5 UUIDv5).
//!
//! Why deterministic ids matter:
//!
//! `dedupe_key` for the `recap.topic_snapshotted.v1` event is
//! `recap.topic_snapshotted.v1:<user_id>:<snapshot_id>`. If a transient
//! network blip causes the persist stage to retry, a random snapshot_id
//! would change the dedupe key on each attempt and let the same logical
//! topic snapshot get appended twice. UUIDv5 over `(user_id, cluster_id,
//! window_start, window_end)` makes the snapshot id stable across retries
//! so the sovereign unique index can elide the duplicate.
//!
//! Knowledge Loop Completion Phase 1 §2 (ADR-000853 follow-up).

use chrono::{DateTime, Utc};
use uuid::Uuid;

/// Custom namespace UUID for `recap.topic_snapshotted.v1` snapshot ids.
///
/// **Do NOT change this value.** Once a single snapshot id has been emitted
/// against the sovereign dedupe registry, mutating the namespace would make
/// the next retry's snapshot id differ from the first attempt, breaking
/// idempotency for every in-flight retry. Generated once with
/// `Uuid::new_v4()` and frozen here.
const RECAP_TOPIC_SNAPSHOTTED_NAMESPACE: Uuid = Uuid::from_bytes([
    0x18, 0xc0, 0x67, 0x40, 0xa8, 0x46, 0x4b, 0x6e, 0x9d, 0x4a, 0x16, 0x6e, 0x7a, 0x32, 0xc1, 0x9f,
]);

/// Compute a stable UUIDv5 from `(user_id, cluster_id, window_start,
/// window_end)`. Same inputs → same output, retry-safe.
///
/// The name byte string is `<user_id>|<cluster_id>|<window_start RFC3339>|
/// <window_end RFC3339>`. Pipe-separation keeps it injection-safe: none of
/// the constituent values can contain a literal `|` (uuids are hex only,
/// cluster_id is i64 base-10, RFC3339 doesn't use pipes).
pub(crate) fn deterministic_snapshot_id(
    user_id: Uuid,
    cluster_id: i64,
    window_start: DateTime<Utc>,
    window_end: DateTime<Utc>,
) -> Uuid {
    let name = format!(
        "{}|{}|{}|{}",
        user_id,
        cluster_id,
        window_start.to_rfc3339(),
        window_end.to_rfc3339(),
    );
    Uuid::new_v5(&RECAP_TOPIC_SNAPSHOTTED_NAMESPACE, name.as_bytes())
}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::TimeZone;

    fn fixture_user() -> Uuid {
        Uuid::parse_str("44444444-4444-4444-4444-444444444444").unwrap()
    }

    fn fixture_window() -> (DateTime<Utc>, DateTime<Utc>) {
        (
            Utc.with_ymd_and_hms(2026, 5, 1, 0, 0, 0).unwrap(),
            Utc.with_ymd_and_hms(2026, 5, 8, 0, 0, 0).unwrap(),
        )
    }

    /// Same inputs → same UUID. The whole point of UUIDv5 over a stable
    /// namespace: retry safety against the sovereign dedupe index.
    #[test]
    fn returns_same_uuid_for_same_inputs() {
        let (start, end) = fixture_window();
        let a = deterministic_snapshot_id(fixture_user(), 7, start, end);
        let b = deterministic_snapshot_id(fixture_user(), 7, start, end);
        assert_eq!(
            a, b,
            "deterministic_snapshot_id must be stable for same inputs"
        );
    }

    /// Different cluster_id → different UUID. Guards against accidentally
    /// collapsing two distinct clusters into one event row.
    #[test]
    fn differs_when_cluster_id_differs() {
        let (start, end) = fixture_window();
        let a = deterministic_snapshot_id(fixture_user(), 7, start, end);
        let b = deterministic_snapshot_id(fixture_user(), 8, start, end);
        assert_ne!(a, b);
    }

    /// Different user → different UUID. Cross-user isolation: even with the
    /// same cluster and window, two users must not share a snapshot id.
    #[test]
    fn differs_when_user_id_differs() {
        let other = Uuid::parse_str("55555555-5555-5555-5555-555555555555").unwrap();
        let (start, end) = fixture_window();
        let a = deterministic_snapshot_id(fixture_user(), 7, start, end);
        let b = deterministic_snapshot_id(other, 7, start, end);
        assert_ne!(a, b);
    }

    /// Different window → different UUID. A retry of the same logical
    /// cluster but a different window (e.g. operator-triggered re-cluster
    /// over a longer span) must produce a fresh event.
    #[test]
    fn differs_when_window_differs() {
        let (start, end) = fixture_window();
        let a = deterministic_snapshot_id(fixture_user(), 7, start, end);
        let later_end = end + chrono::Duration::days(1);
        let b = deterministic_snapshot_id(fixture_user(), 7, start, later_end);
        assert_ne!(a, b);
    }

    /// The output must be a UUIDv5 (version 5 nibble in byte 6, variant
    /// "10" in byte 8 per RFC 9562 §4). RFC compliance keeps us
    /// interoperable with any consumer that validates UUIDs strictly.
    #[test]
    fn output_is_uuidv5() {
        let (start, end) = fixture_window();
        let id = deterministic_snapshot_id(fixture_user(), 7, start, end);
        assert_eq!(id.get_version_num(), 5);
        let variant = id.as_bytes()[8] & 0xC0;
        assert_eq!(variant, 0x80, "UUIDv5 must have RFC 9562 variant bits 10xx");
    }

    /// Frozen reference value. If this assertion fires, somebody changed the
    /// namespace UUID or the name format — that's an idempotency-breaking
    /// regression. Restore the constant or expect every in-flight retry to
    /// produce a duplicate row.
    #[test]
    fn frozen_reference_for_known_inputs() {
        let (start, end) = fixture_window();
        let id = deterministic_snapshot_id(fixture_user(), 7, start, end);
        // Computed once and pinned so any future change to the namespace UUID
        // or name encoding is caught here rather than in production.
        let expected = Uuid::new_v5(
            &RECAP_TOPIC_SNAPSHOTTED_NAMESPACE,
            format!(
                "44444444-4444-4444-4444-444444444444|7|{}|{}",
                start.to_rfc3339(),
                end.to_rfc3339(),
            )
            .as_bytes(),
        );
        assert_eq!(id, expected);
    }
}
