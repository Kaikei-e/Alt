//! Forwards `notification_outbox` rows to alt-data-hub.
//!
//! The shape is deliberate on two points:
//!
//!   - The claim commits before the outbound call. Holding a transaction open
//!     across a call to another service ties this database's connection
//!     lifetime to that service's latency, and a slow provider becomes a pool
//!     exhaustion in a database it has no business touching.
//!   - Delivery is at-least-once. Nothing here tries for exactly-once:
//!     idempotency is the provider's, keyed on the `dedupe_key` the producer
//!     derived from the business fact.

use std::sync::Arc;
use std::time::Duration;

use rand::RngExt;
use sqlx::PgPool;
use tokio_util::sync::CancellationToken;
use tracing::{debug, error, info, warn};

use crate::clients::datahub::{DataHubClient, NotificationEnqueue};
use crate::error::Result;
use crate::observability::metrics::Metrics;
use crate::store::dao::notification_outbox::{ClaimedNotification, NotificationOutboxDao};

/// Knobs of the relay loop. Defaults are the operating point the outbox was
/// designed around; they are constructor arguments only so tests can drive
/// the loop without waiting out a real backoff.
#[derive(Debug, Clone, Copy)]
pub(crate) struct RelayConfig {
    /// Rows claimed per tick.
    pub(crate) batch_size: i32,
    /// How long a claim holds. Doubles as the crash-reclaim window: an
    /// orphaned row re-enters the claim query once this expires.
    pub(crate) lease: Duration,
    /// Gap between ticks when there was nothing to do.
    pub(crate) poll_interval: Duration,
    /// Full-jitter backoff base.
    pub(crate) backoff_base: Duration,
    /// Full-jitter backoff ceiling.
    pub(crate) backoff_cap: Duration,
    /// Attempts before a row is declared dead.
    pub(crate) max_attempts: i32,
    /// How long after `occurred_at` the notification stops being worth
    /// delivering. A "your recap is ready" ping is stale the next day; the
    /// window is the producer's judgement, and the provider does not guess.
    pub(crate) ttl: Duration,
}

impl Default for RelayConfig {
    fn default() -> Self {
        Self {
            batch_size: 100,
            lease: Duration::from_mins(1),
            poll_interval: Duration::from_secs(5),
            backoff_base: Duration::from_secs(10),
            backoff_cap: Duration::from_hours(1),
            max_attempts: 8,
            ttl: Duration::from_hours(24),
        }
    }
}

impl RelayConfig {
    /// `min(cap, base * 2^attempt)` — the upper bound the jitter samples under.
    pub(crate) fn backoff_ceiling_seconds(&self, attempt: i32) -> f64 {
        let shift = u32::try_from(attempt.max(0)).unwrap_or(u32::MAX).min(32);
        let exponential = self.backoff_base.as_secs_f64() * 2_f64.powi(shift as i32);
        exponential.min(self.backoff_cap.as_secs_f64())
    }

    /// Full jitter: `random(0, ceiling)`. Sampling the whole interval, rather
    /// than jittering around the ceiling, is what stops a batch of rows that
    /// failed together from retrying together.
    pub(crate) fn backoff_delay_seconds(&self, attempt: i32) -> f64 {
        let ceiling = self.backoff_ceiling_seconds(attempt);
        if ceiling <= 0.0 {
            return 0.0;
        }
        rand::rng().random_range(0.0..=ceiling)
    }
}

pub(crate) struct NotificationRelay {
    pool: PgPool,
    client: Arc<DataHubClient>,
    metrics: Arc<Metrics>,
    /// Recorded in `locked_by`. The only forensic link from a stuck row to
    /// the process that holds it.
    locked_by: String,
    config: RelayConfig,
}

impl NotificationRelay {
    pub(crate) fn new(
        pool: PgPool,
        client: Arc<DataHubClient>,
        metrics: Arc<Metrics>,
        locked_by: String,
        config: RelayConfig,
    ) -> Self {
        Self {
            pool,
            client,
            metrics,
            locked_by,
            config,
        }
    }

    /// Run until `cancel` fires.
    pub(crate) async fn run(&self, cancel: CancellationToken) {
        info!(
            locked_by = %self.locked_by,
            batch_size = self.config.batch_size,
            max_attempts = self.config.max_attempts,
            "notification_outbox_relay_enabled"
        );

        loop {
            if cancel.is_cancelled() {
                info!("shutdown requested, stopping notification outbox relay");
                return;
            }

            let forwarded = match self.tick().await {
                Ok(forwarded) => forwarded,
                Err(e) => {
                    error!(error = %e, "notification outbox relay tick failed");
                    0
                }
            };

            // A tick that moved a full batch probably has more waiting, so
            // only idle when the batch came back short.
            if forwarded >= usize::try_from(self.config.batch_size).unwrap_or(usize::MAX) {
                continue;
            }

            tokio::select! {
                () = tokio::time::sleep(self.config.poll_interval) => {}
                () = cancel.cancelled() => {
                    info!("shutdown requested, stopping notification outbox relay");
                    return;
                }
            }
        }
    }

    /// Claim one batch, forward it, and publish the relay gauges.
    ///
    /// Returns how many rows the provider accepted.
    pub(crate) async fn tick(&self) -> Result<usize> {
        let result = self.forward_one_batch().await;
        self.publish_gauges().await;
        result
    }

    async fn forward_one_batch(&self) -> Result<usize> {
        let claimed = NotificationOutboxDao::claim_batch(
            &self.pool,
            &self.locked_by,
            self.config.batch_size,
            self.config.lease.as_secs_f64(),
        )
        .await?;

        if claimed.is_empty() {
            return Ok(0);
        }

        debug!(count = claimed.len(), "claimed notification outbox batch");

        let mut forwarded = 0;
        for row in claimed {
            if self.forward_one(&row).await? {
                forwarded += 1;
            }
        }
        Ok(forwarded)
    }

    async fn forward_one(&self, row: &ClaimedNotification) -> Result<bool> {
        // A row can only get here with more attempts than the ceiling by
        // being reclaimed after a crash that followed its last claim. It has
        // no attempts left to spend, so end it rather than calling again.
        if row.attempts > self.config.max_attempts {
            NotificationOutboxDao::mark_dead(
                &self.pool,
                row.id,
                "attempts exhausted before a successful forward",
            )
            .await?;
            warn!(
                notification_id = %row.id,
                dedupe_key = %row.dedupe_key,
                attempts = row.attempts,
                "notification outbox row reclaimed with no attempts left; marking dead"
            );
            return Ok(false);
        }

        let expires_at = row.occurred_at
            + chrono::Duration::from_std(self.config.ttl)
                .unwrap_or_else(|_| chrono::Duration::days(1));

        let outcome = self
            .client
            .enqueue_notification(&NotificationEnqueue {
                dedupe_key: &row.dedupe_key,
                user_id: row.user_id,
                kind: &row.kind,
                payload: &row.payload,
                occurred_at: row.occurred_at,
                expires_at,
            })
            .await;

        match outcome {
            Ok(outcome) => {
                NotificationOutboxDao::mark_forwarded(&self.pool, row.id).await?;
                info!(
                    notification_id = %row.id,
                    dedupe_key = %row.dedupe_key,
                    delivery_count = outcome.delivery_count,
                    superseded_count = outcome.superseded_count,
                    "forwarded notification to alt-data-hub"
                );
                Ok(true)
            }
            Err(e) => {
                let last_error = format!("{e:#}");
                if row.attempts >= self.config.max_attempts {
                    NotificationOutboxDao::mark_dead(&self.pool, row.id, &last_error).await?;
                    error!(
                        notification_id = %row.id,
                        dedupe_key = %row.dedupe_key,
                        attempts = row.attempts,
                        error = %last_error,
                        "notification outbox row exhausted its attempts; marking dead"
                    );
                } else {
                    let delay = self.config.backoff_delay_seconds(row.attempts);
                    NotificationOutboxDao::reschedule(&self.pool, row.id, &last_error, delay)
                        .await?;
                    warn!(
                        notification_id = %row.id,
                        dedupe_key = %row.dedupe_key,
                        attempts = row.attempts,
                        retry_in_seconds = delay,
                        error = %last_error,
                        "failed to forward notification; rescheduled"
                    );
                }
                Ok(false)
            }
        }
    }

    /// Both gauges, every tick. See the comment on their registration.
    async fn publish_gauges(&self) {
        match NotificationOutboxDao::oldest_pending_age_seconds(&self.pool).await {
            Ok(age) => self.metrics.notification_outbox_oldest_pending_age.set(age),
            Err(e) => error!(
                error = %e,
                "failed to read notification outbox backlog age; the age gauge is now stale"
            ),
        }
        self.metrics
            .notification_outbox_last_tick_timestamp
            .set(unix_time_seconds());
    }
}

/// Observability time, not a business fact: this is "when did the loop last
/// run", which is precisely a wall-clock question.
fn unix_time_seconds() -> f64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map_or(0.0, |d| d.as_secs_f64())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::observability::metrics::Metrics;
    use prometheus::Registry;
    use sqlx::postgres::PgPoolOptions;
    use sqlx::{Executor, PgPool, Row};
    use std::sync::Arc;
    use std::time::Duration;
    use uuid::Uuid;
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    const RPC_PATH: &str = "/services.datahub.v1.DataHubService/EnqueueNotification";

    async fn scratch_pool(max_connections: u32) -> Option<PgPool> {
        let database_url = std::env::var("DATABASE_URL").ok()?;
        PgPoolOptions::new()
            .max_connections(max_connections)
            .connect(&database_url)
            .await
            .ok()
    }

    async fn setup_outbox(pool: &PgPool) -> anyhow::Result<()> {
        pool.execute(
            r"
            CREATE TABLE IF NOT EXISTS notification_outbox (
                id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                dedupe_key      TEXT NOT NULL UNIQUE,
                user_id         UUID NOT NULL,
                kind            TEXT NOT NULL,
                payload         JSONB NOT NULL,
                occurred_at     TIMESTAMPTZ NOT NULL,
                created_at      TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
                state           TEXT NOT NULL DEFAULT 'pending'
                                  CHECK (state IN ('pending', 'forwarding', 'forwarded', 'dead')),
                attempts        INT NOT NULL DEFAULT 0,
                next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
                locked_by       TEXT,
                last_error      TEXT,
                forwarded_at    TIMESTAMPTZ
            );
            ",
        )
        .await?;
        pool.execute("TRUNCATE TABLE notification_outbox;").await?;
        Ok(())
    }

    async fn insert_pending(
        pool: &PgPool,
        dedupe_key: &str,
        attempts: i32,
    ) -> anyhow::Result<Uuid> {
        let row = sqlx::query(
            r"
            INSERT INTO notification_outbox
                (dedupe_key, user_id, kind, payload, occurred_at, attempts)
            VALUES ($1, $2, 'recap_ready', $3, clock_timestamp(), $4)
            RETURNING id
            ",
        )
        .bind(dedupe_key)
        .bind(Uuid::new_v4())
        .bind(serde_json::json!({"kind": "recap_ready", "url": "/recap"}))
        .bind(attempts)
        .fetch_one(pool)
        .await?;
        Ok(row.try_get("id")?)
    }

    async fn state_of(pool: &PgPool, id: Uuid) -> anyhow::Result<(String, i32, Option<String>)> {
        let row =
            sqlx::query("SELECT state, attempts, locked_by FROM notification_outbox WHERE id = $1")
                .bind(id)
                .fetch_one(pool)
                .await?;
        Ok((
            row.try_get("state")?,
            row.try_get("attempts")?,
            row.try_get("locked_by")?,
        ))
    }

    fn test_metrics() -> Arc<Metrics> {
        Arc::new(Metrics::new(Arc::new(Registry::new())).expect("metrics register"))
    }

    fn relay(pool: PgPool, base_url: &str, metrics: Arc<Metrics>) -> NotificationRelay {
        NotificationRelay::new(
            pool,
            Arc::new(
                crate::clients::datahub::DataHubClient::new(
                    base_url,
                    Duration::from_secs(2),
                    Duration::from_secs(10),
                )
                .expect("client builds"),
            ),
            metrics,
            "relay-test".to_string(),
            RelayConfig::default(),
        )
    }

    /// Full jitter, per the shape the task pins: `random(0, min(cap, base *
    /// 2^attempt))`. Sampling the ceiling rather than a fixed delay is the
    /// point — a retry storm that all backs off by the same amount is a
    /// thundering herd with extra steps.
    #[test]
    fn backoff_is_full_jitter_under_the_exponential_ceiling() {
        let config = RelayConfig::default();

        assert!(
            (config.backoff_ceiling_seconds(1) - 20.0).abs() < f64::EPSILON,
            "base 10s doubled once"
        );
        assert!(
            (config.backoff_ceiling_seconds(8) - 2560.0).abs() < f64::EPSILON,
            "base 10s doubled eight times, still under the hour cap"
        );
        assert!(
            (config.backoff_ceiling_seconds(40) - 3600.0).abs() < f64::EPSILON,
            "the cap has to bind before the shift overflows"
        );

        for attempt in 1..=8 {
            let ceiling = config.backoff_ceiling_seconds(attempt);
            let mut saw_below_ceiling = false;
            for _ in 0..200 {
                let delay = config.backoff_delay_seconds(attempt);
                assert!(
                    (0.0..=ceiling).contains(&delay),
                    "attempt {attempt}: {delay} outside [0, {ceiling}]"
                );
                if delay < ceiling * 0.9 {
                    saw_below_ceiling = true;
                }
            }
            assert!(
                saw_below_ceiling,
                "attempt {attempt}: every sample sat at the ceiling — that is not jitter"
            );
        }
    }

    #[tokio::test]
    async fn relay_forwards_a_pending_row_and_marks_it_forwarded() -> anyhow::Result<()> {
        let Some(pool) = scratch_pool(2).await else {
            return Ok(());
        };
        setup_outbox(&pool).await?;
        let id = insert_pending(&pool, "recap:forward-ok", 0).await?;

        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path(RPC_PATH))
            .respond_with(
                ResponseTemplate::new(200)
                    .set_body_json(serde_json::json!({"deliveryCount": 1, "supersededCount": 0})),
            )
            .mount(&server)
            .await;

        let forwarded = relay(pool.clone(), &server.uri(), test_metrics())
            .tick()
            .await?;
        assert_eq!(forwarded, 1);

        let (state, attempts, locked_by) = state_of(&pool, id).await?;
        assert_eq!(state, "forwarded");
        assert_eq!(attempts, 1);
        assert_eq!(locked_by, None);
        Ok(())
    }

    /// The claim and the outbound call must not share a transaction. Observing
    /// the claimed state from a *different* connection while the call is still
    /// in flight is what proves it: an uncommitted claim would be invisible
    /// there, and the row would still read `pending`.
    #[tokio::test]
    async fn relay_commits_the_claim_before_the_outbound_call() -> anyhow::Result<()> {
        let Some(pool) = scratch_pool(2).await else {
            return Ok(());
        };
        let Some(observer) = scratch_pool(1).await else {
            return Ok(());
        };
        setup_outbox(&pool).await?;
        let id = insert_pending(&pool, "recap:claim-committed", 0).await?;

        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path(RPC_PATH))
            .respond_with(
                ResponseTemplate::new(200)
                    .set_body_json(serde_json::json!({"deliveryCount": 1}))
                    .set_delay(Duration::from_millis(800)),
            )
            .mount(&server)
            .await;

        let relay = relay(pool.clone(), &server.uri(), test_metrics());
        let task = tokio::spawn(async move { relay.tick().await });

        tokio::time::sleep(Duration::from_millis(250)).await;
        let (state, attempts, locked_by) = state_of(&observer, id).await?;
        assert_eq!(
            state, "forwarding",
            "another connection cannot see the claim, so it was never committed \
             before the network call"
        );
        assert_eq!(attempts, 1);
        assert_eq!(locked_by.as_deref(), Some("relay-test"));

        assert_eq!(task.await??, 1);
        let (state, _, _) = state_of(&pool, id).await?;
        assert_eq!(state, "forwarded");
        Ok(())
    }

    #[tokio::test]
    async fn relay_reschedules_with_backoff_when_the_provider_fails() -> anyhow::Result<()> {
        let Some(pool) = scratch_pool(2).await else {
            return Ok(());
        };
        setup_outbox(&pool).await?;
        let id = insert_pending(&pool, "recap:provider-503", 0).await?;

        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path(RPC_PATH))
            .respond_with(ResponseTemplate::new(503).set_body_string("unavailable"))
            .mount(&server)
            .await;

        let forwarded = relay(pool.clone(), &server.uri(), test_metrics())
            .tick()
            .await?;
        assert_eq!(forwarded, 0);

        let row = sqlx::query(
            "SELECT state, attempts, last_error, next_attempt_at > clock_timestamp() AS deferred
             FROM notification_outbox WHERE id = $1",
        )
        .bind(id)
        .fetch_one(&pool)
        .await?;
        let state: String = row.try_get("state")?;
        let attempts: i32 = row.try_get("attempts")?;
        let last_error: Option<String> = row.try_get("last_error")?;
        let deferred: bool = row.try_get("deferred")?;

        assert_eq!(state, "pending", "a 503 is worth another attempt");
        assert_eq!(attempts, 1);
        assert!(last_error.unwrap_or_default().contains("503"));
        assert!(
            deferred,
            "the row must not be immediately re-claimable, or the backoff is decorative"
        );
        Ok(())
    }

    #[tokio::test]
    async fn relay_marks_dead_once_attempts_are_spent() -> anyhow::Result<()> {
        let Some(pool) = scratch_pool(2).await else {
            return Ok(());
        };
        setup_outbox(&pool).await?;
        // One short of the ceiling, so this tick's claim spends the last one.
        let id = insert_pending(
            &pool,
            "recap:exhausted",
            RelayConfig::default().max_attempts - 1,
        )
        .await?;

        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path(RPC_PATH))
            .respond_with(ResponseTemplate::new(500).set_body_string("boom"))
            .mount(&server)
            .await;

        let forwarded = relay(pool.clone(), &server.uri(), test_metrics())
            .tick()
            .await?;
        assert_eq!(forwarded, 0);

        let (state, attempts, _) = state_of(&pool, id).await?;
        assert_eq!(state, "dead");
        assert_eq!(attempts, RelayConfig::default().max_attempts);
        Ok(())
    }

    /// A gauge that stops being set keeps reporting its last value, so a
    /// wedged relay would look exactly as healthy as an idle one. Both gauges
    /// have to be written on every tick, including the tick that found
    /// nothing.
    #[tokio::test]
    async fn relay_publishes_both_gauges_on_an_empty_tick() -> anyhow::Result<()> {
        let Some(pool) = scratch_pool(2).await else {
            return Ok(());
        };
        setup_outbox(&pool).await?;

        let server = MockServer::start().await;
        let metrics = test_metrics();
        metrics.notification_outbox_oldest_pending_age.set(999.0);

        let forwarded = relay(pool.clone(), &server.uri(), Arc::clone(&metrics))
            .tick()
            .await?;
        assert_eq!(forwarded, 0);

        assert!(
            (metrics.notification_outbox_oldest_pending_age.get() - 0.0).abs() < f64::EPSILON,
            "an empty backlog is zero, not the last value the gauge happened to hold"
        );
        assert!(
            metrics.notification_outbox_last_tick_timestamp.get() > 0.0,
            "the tick timestamp is the only thing that distinguishes a stalled \
             relay from an idle one"
        );
        Ok(())
    }

    #[tokio::test]
    async fn relay_reports_the_backlog_age_of_a_row_it_could_not_forward() -> anyhow::Result<()> {
        let Some(pool) = scratch_pool(2).await else {
            return Ok(());
        };
        setup_outbox(&pool).await?;
        sqlx::query(
            r"
            INSERT INTO notification_outbox
                (dedupe_key, user_id, kind, payload, occurred_at, created_at, next_attempt_at)
            VALUES ($1, $2, 'recap_ready', $3,
                    clock_timestamp() - interval '2 hours',
                    clock_timestamp() - interval '2 hours',
                    clock_timestamp() + interval '1 hour')
            ",
        )
        .bind("recap:stale-backlog")
        .bind(Uuid::new_v4())
        .bind(serde_json::json!({"kind": "recap_ready", "url": "/recap"}))
        .execute(&pool)
        .await?;

        let server = MockServer::start().await;
        let metrics = test_metrics();
        relay(pool.clone(), &server.uri(), Arc::clone(&metrics))
            .tick()
            .await?;

        let age = metrics.notification_outbox_oldest_pending_age.get();
        assert!(
            age > 7000.0,
            "a two-hour-old backlog entry must show up as ~7200s, got {age}"
        );
        Ok(())
    }
}
