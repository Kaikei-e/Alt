//! Transactional outbox for user-facing push notifications.
//!
//! The table lives in recap-db beside the recap job state it describes, and
//! that placement is the whole guarantee: the notification row and the
//! completion share one local commit, so the notification is enqueued if and
//! only if the completion is durable. See the header of
//! `recap-migration-atlas/migrations/20260808000100_create_notification_outbox.sql`.
//!
//! Two halves live here. The producer half
//! ([`NotificationOutboxDao::enqueue_recap_ready`]) takes a connection the
//! caller is already holding a transaction on — it must never open its own,
//! which would make it a dual write wearing an outbox's name. The relay half
//! (claim / forward / reschedule) runs on the pool, each statement its own
//! commit, because the relay must not hold a transaction open across a network
//! call to another service.

use chrono::{DateTime, Utc};
use serde_json::Value;
use sqlx::{PgConnection, PgPool, Row};
use uuid::Uuid;

use crate::error::{RecapError, Result};

/// Notification kind for "the recap you asked for is ready".
pub(crate) const RECAP_READY_KIND: &str = "recap_ready";

/// Derived from the business fact, never from the attempt: a retry of the
/// completion at any layer produces this same key, which is what makes a
/// re-enqueue and a re-forward harmless.
pub(crate) fn recap_dedupe_key(job_id: Uuid) -> String {
    format!("recap:{job_id}")
}

/// A row the relay has claimed and now owns.
#[derive(Debug, Clone)]
pub(crate) struct ClaimedNotification {
    pub(crate) id: Uuid,
    pub(crate) dedupe_key: String,
    pub(crate) user_id: Uuid,
    pub(crate) kind: String,
    pub(crate) payload: Value,
    pub(crate) occurred_at: DateTime<Utc>,
    /// Attempt number this claim represents, counting from 1.
    pub(crate) attempts: i32,
}

pub(crate) struct NotificationOutboxDao;

impl NotificationOutboxDao {
    /// Enqueue the `recap_ready` notification for a completed recap job.
    ///
    /// Takes a `&mut PgConnection` rather than a pool so the caller's
    /// transaction is the only unit of commit. `ON CONFLICT DO NOTHING` on the
    /// derived key makes a re-run of the completion a no-op instead of a
    /// second notification.
    ///
    /// `occurred_at` is supplied by the caller and must be the completion time
    /// the job row records — a fresh clock read here would describe when the
    /// outbox row was written, which is a different fact.
    pub(crate) async fn enqueue_recap_ready(
        conn: &mut PgConnection,
        job_id: Uuid,
        user_id: Uuid,
        occurred_at: DateTime<Utc>,
    ) -> Result<()> {
        // A signal to come and look, not a delivery channel: a type
        // discriminator and where to go, and nothing of the recap itself.
        let payload = crate::clients::datahub::recap_ready_payload();

        sqlx::query(
            r"
            INSERT INTO notification_outbox
                (dedupe_key, user_id, kind, payload, occurred_at)
            VALUES ($1, $2, $3, $4, $5)
            ON CONFLICT (dedupe_key) DO NOTHING
            ",
        )
        .bind(recap_dedupe_key(job_id))
        .bind(user_id)
        .bind(RECAP_READY_KIND)
        .bind(payload)
        .bind(occurred_at)
        .execute(&mut *conn)
        .await
        .map_err(|e| RecapError::Db(format!("failed to enqueue notification outbox row: {e}")))?;

        Ok(())
    }

    /// Take ownership of up to `limit` due rows in one statement.
    ///
    /// The claim covers fresh work and crash reclaim together: it matches
    /// `state IN ('pending','forwarding')` and pushes `next_attempt_at`
    /// forward by the lease, so a row orphaned by a relay that died mid-attempt
    /// re-enters this same query once the lease expires. There is no separate
    /// reclaim sweeper to deploy, and none to forget.
    ///
    /// `attempts` increments here rather than on failure, so a crash between
    /// the claim and the outbound call still spends an attempt and a
    /// permanently undeliverable row cannot be reclaimed forever.
    pub(crate) async fn claim_batch(
        pool: &PgPool,
        locked_by: &str,
        limit: i32,
        lease_seconds: f64,
    ) -> Result<Vec<ClaimedNotification>> {
        let rows = sqlx::query(
            r"
            UPDATE notification_outbox
            SET state = 'forwarding',
                attempts = attempts + 1,
                locked_by = $1,
                next_attempt_at = clock_timestamp()
                                  + make_interval(secs => $3::double precision)
            WHERE id IN (
                SELECT id
                FROM notification_outbox
                WHERE state IN ('pending', 'forwarding')
                  AND next_attempt_at <= clock_timestamp()
                ORDER BY next_attempt_at, id
                FOR UPDATE SKIP LOCKED
                LIMIT $2
            )
            RETURNING id, dedupe_key, user_id, kind, payload, occurred_at, attempts
            ",
        )
        .bind(locked_by)
        .bind(limit)
        .bind(lease_seconds)
        .fetch_all(pool)
        .await
        .map_err(|e| RecapError::Db(format!("failed to claim notification outbox batch: {e}")))?;

        rows.into_iter()
            .map(|row| {
                Ok(ClaimedNotification {
                    id: row.try_get("id")?,
                    dedupe_key: row.try_get("dedupe_key")?,
                    user_id: row.try_get("user_id")?,
                    kind: row.try_get("kind")?,
                    payload: row.try_get("payload")?,
                    occurred_at: row.try_get("occurred_at")?,
                    attempts: row.try_get("attempts")?,
                })
            })
            .collect()
    }

    /// Record a forward the provider accepted.
    pub(crate) async fn mark_forwarded(pool: &PgPool, id: Uuid) -> Result<()> {
        sqlx::query(
            r"
            UPDATE notification_outbox
            SET state = 'forwarded',
                forwarded_at = clock_timestamp(),
                locked_by = NULL,
                last_error = NULL
            WHERE id = $1
            ",
        )
        .bind(id)
        .execute(pool)
        .await
        .map_err(|e| RecapError::Db(format!("failed to mark notification forwarded: {e}")))?;

        Ok(())
    }

    /// Return a claimed row to `pending` with the caller's backoff.
    pub(crate) async fn reschedule(
        pool: &PgPool,
        id: Uuid,
        last_error: &str,
        delay_seconds: f64,
    ) -> Result<()> {
        sqlx::query(
            r"
            UPDATE notification_outbox
            SET state = 'pending',
                locked_by = NULL,
                last_error = $2,
                next_attempt_at = clock_timestamp()
                                  + make_interval(secs => $3::double precision)
            WHERE id = $1
            ",
        )
        .bind(id)
        .bind(last_error)
        .bind(delay_seconds)
        .execute(pool)
        .await
        .map_err(|e| RecapError::Db(format!("failed to reschedule notification: {e}")))?;

        Ok(())
    }

    /// End forwarding for a row that has spent its attempts.
    pub(crate) async fn mark_dead(pool: &PgPool, id: Uuid, last_error: &str) -> Result<()> {
        sqlx::query(
            r"
            UPDATE notification_outbox
            SET state = 'dead',
                locked_by = NULL,
                last_error = $2
            WHERE id = $1
            ",
        )
        .bind(id)
        .bind(last_error)
        .execute(pool)
        .await
        .map_err(|e| RecapError::Db(format!("failed to mark notification dead: {e}")))?;

        Ok(())
    }

    /// Age of the oldest row still waiting to be forwarded, in seconds.
    ///
    /// Zero when the backlog is empty — an empty backlog is an answer, and the
    /// caller publishes it on every tick so a gauge that stopped being set
    /// cannot keep reporting a wedged relay's last healthy value.
    pub(crate) async fn oldest_pending_age_seconds(pool: &PgPool) -> Result<f64> {
        let row = sqlx::query(
            r"
            SELECT COALESCE(
                       EXTRACT(EPOCH FROM (clock_timestamp() - MIN(created_at))),
                       0
                   )::double precision AS age_seconds
            FROM notification_outbox
            WHERE state IN ('pending', 'forwarding')
            ",
        )
        .fetch_one(pool)
        .await
        .map_err(|e| {
            RecapError::Db(format!(
                "failed to read notification outbox backlog age: {e}"
            ))
        })?;

        let age: f64 = row.try_get("age_seconds")?;
        Ok(age.max(0.0))
    }
}
