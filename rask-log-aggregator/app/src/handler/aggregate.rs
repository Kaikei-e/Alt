use crate::domain::EnrichedLogEntry;
use crate::port::LogExporter;
use axum::extract::State;
use axum::http::StatusCode;
use axum::response::IntoResponse;
use std::sync::Arc;
use tracing::{debug, error, info};

/// Max bytes of a malformed line to include in an error log - a full 10MB
/// line dumped into the log on every parse failure would itself become a
/// throughput/storage problem.
const MAX_LOGGED_LINE_LEN: usize = 256;

/// Truncates `s` to at most `max_len` bytes, on a UTF-8 char boundary.
fn truncate_for_log(s: &str, max_len: usize) -> &str {
    if s.len() <= max_len {
        return s;
    }
    let mut end = max_len;
    while !s.is_char_boundary(end) {
        end -= 1;
    }
    &s[..end]
}

/// Handler for POST /v1/aggregate (legacy NDJSON logs from rask-log-forwarder)
///
/// The 2xx returned here is a durability ack, not a receipt: the forwarder
/// `mem::take`s its buffer and drops the batch the moment it sees one, so
/// after a 200 this process holds the only copy. That makes the response the
/// HTTP edition of CLAUDE.md rule 10 - ack only once the side effect is
/// durable. Two obligations follow, and only the first is enforceable here:
///
/// 1. Never answer before `LogExporter::export_batch` has resolved, and
///    answer 5xx when it fails, so the forwarder retries from its own copy.
/// 2. `export_batch` returning `Ok` must mean "durably written", not
///    "buffered". `BatchWriter` currently resolves `Ok` on channel send and
///    flushes up to `FLUSH_INTERVAL_SECS` later, so a SIGKILL/OOM between
///    the two loses the batch. Fixing that needs a durability ack on the
///    port itself and belongs in `port::LogExporter` /
///    `adapter::clickhouse::BatchWriter`, not in this layer.
pub async fn aggregate_handler(
    State(exporter): State<Arc<dyn LogExporter>>,
    body: String,
) -> impl IntoResponse {
    debug!(
        "Received aggregate request with body length: {}",
        body.len()
    );

    // Handle empty body
    if body.is_empty() {
        return (StatusCode::OK, "No logs to process");
    }

    let logs: Vec<EnrichedLogEntry> = body
        .lines()
        .filter_map(|line| match serde_json::from_str(line) {
            Ok(entry) => Some(entry),
            Err(e) => {
                error!(
                    "Failed to parse log entry: {e} - Line: {}",
                    truncate_for_log(line, MAX_LOGGED_LINE_LEN)
                );
                None
            }
        })
        .collect();

    debug!("Parsed {} log entries from request", logs.len());

    // Handle case where no valid logs were parsed
    if logs.is_empty() {
        return (StatusCode::OK, "No valid logs to export");
    }

    let log_count = logs.len();

    match exporter.export_batch(logs).await {
        Ok(()) => {
            // Report what this layer actually observed - the export port
            // took the batch - rather than a completed ClickHouse write it
            // has no way to confirm. A log that claims success for rows a
            // SIGKILL is about to discard is worse than no log at all.
            info!(log_count, "Accepted log batch for export");
            (StatusCode::OK, "OK")
        }
        Err(e) => {
            error!(log_count, error = %e, "Export port rejected log batch");
            (StatusCode::INTERNAL_SERVER_ERROR, "Export failed")
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::error::AggregatorError;
    use crate::port::BoxFuture;
    use crate::test_support::MockExporter;
    use std::sync::atomic::{AtomicUsize, Ordering};
    use std::time::Duration;
    use tokio::sync::Notify;
    use tracing_test::traced_test;

    const VALID_LINE: &str = r#"{"service_type":"http","log_type":"access","message":"hello","timestamp":"2025-01-10T12:00:00Z","stream":"stdout","container_id":"abc123","service_name":"test-svc","fields":{}}"#;

    async fn post(exporter: Arc<dyn LogExporter>, body: &str) -> StatusCode {
        aggregate_handler(State(exporter), body.to_string())
            .await
            .into_response()
            .status()
    }

    /// Exporter whose `export_batch` future stays pending until `release` is
    /// signalled - stands in for a backend that has taken the rows but has
    /// not yet made them durable.
    struct GatedExporter {
        release: Notify,
        accepted: AtomicUsize,
    }

    impl GatedExporter {
        fn new() -> Self {
            Self {
                release: Notify::new(),
                accepted: AtomicUsize::new(0),
            }
        }
    }

    impl LogExporter for GatedExporter {
        fn export_batch(
            &self,
            logs: Vec<EnrichedLogEntry>,
        ) -> BoxFuture<'_, Result<(), AggregatorError>> {
            Box::pin(async move {
                self.accepted.fetch_add(logs.len(), Ordering::SeqCst);
                self.release.notified().await;
                Ok(())
            })
        }
    }

    /// The 200 this handler returns is a durability ack: rask-log-forwarder
    /// drops its only copy of the batch on any 2xx. Claiming a completed
    /// ClickHouse write for rows the export port has merely taken makes an
    /// unrecoverable loss (SIGKILL before the batch is flushed) look like a
    /// clean success in the log.
    #[tokio::test]
    #[traced_test]
    async fn success_log_does_not_claim_a_write_the_handler_never_observed() {
        let exporter: Arc<dyn LogExporter> = Arc::new(MockExporter::new());

        let status = post(exporter, VALID_LINE).await;

        assert_eq!(status, StatusCode::OK);
        assert!(
            !logs_contain("Successfully exported"),
            "the handler only knows the export port returned Ok; it must not \
             report a completed backend write it never observed"
        );
        assert!(
            !logs_contain("to ClickHouse"),
            "the handler talks to a LogExporter port, not to ClickHouse - \
             naming the driver here is both a layer leak and a false claim \
             when the batch is still buffered"
        );
        assert!(
            logs_contain("Accepted log batch for export"),
            "the accepted batch must still be recorded, just described \
             honestly"
        );
    }

    /// Regression guard for the other half of the ack contract: a failing
    /// export must be answered 5xx so the forwarder retries instead of
    /// discarding the batch.
    #[tokio::test]
    async fn export_failure_is_answered_5xx_so_the_forwarder_keeps_the_batch() {
        let mock = Arc::new(MockExporter::new());
        mock.set_should_fail(true);
        let exporter: Arc<dyn LogExporter> = mock;

        let status = post(exporter, VALID_LINE).await;

        assert_eq!(status, StatusCode::INTERNAL_SERVER_ERROR);
    }

    /// Regression guard: the response must not race ahead of the export
    /// port. Answering 200 while `export_batch` is still pending would let
    /// the forwarder drop the batch before anything downstream owns it.
    #[tokio::test(start_paused = true)]
    async fn no_2xx_is_sent_while_the_export_port_is_still_pending() {
        let gate = Arc::new(GatedExporter::new());
        let exporter: Arc<dyn LogExporter> = gate.clone();
        let body = VALID_LINE.to_string();

        let mut handle = tokio::spawn(async move {
            aggregate_handler(State(exporter), body)
                .await
                .into_response()
                .status()
        });

        assert!(
            tokio::time::timeout(Duration::from_secs(30), &mut handle)
                .await
                .is_err(),
            "handler answered before the export port resolved"
        );
        assert_eq!(gate.accepted.load(Ordering::SeqCst), 1);

        gate.release.notify_one();

        let status = tokio::time::timeout(Duration::from_secs(5), handle)
            .await
            .expect("handler must answer once the export port resolves")
            .unwrap();
        assert_eq!(status, StatusCode::OK);
    }
}
