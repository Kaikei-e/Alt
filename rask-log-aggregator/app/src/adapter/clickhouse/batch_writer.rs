//! Channel-based batch buffering for ClickHouse writes.
//!
//! Instead of each HTTP request immediately creating a ClickHouse inserter,
//! rows are sent through bounded `mpsc` channels to a background task that
//! aggregates batches and writes them periodically.

use crate::adapter::clickhouse::otel_row::{OTelLogRow, OTelTraceRow};
use crate::adapter::clickhouse::row::LogRow;
use crate::domain::{EnrichedLogEntry, OTelLog, OTelTrace};
use crate::error::AggregatorError;
use clickhouse::{Client, RowOwned, RowWrite};
use std::future::Future;
use std::pin::Pin;
use std::time::Duration;
use tokio::sync::mpsc;
use tokio::task::JoinHandle;
use tokio_util::sync::CancellationToken;
use tracing::{error, info, warn};

/// Channel capacity for each row type.
const CHANNEL_CAPACITY: usize = 1024;

/// How often the background task flushes to ClickHouse (seconds).
const FLUSH_INTERVAL_SECS: u64 = 5;

/// Maximum rows per batch flush.
const MAX_BATCH_SIZE: usize = 5000;

/// Maximum number of attempts (including the first) to flush a single batch
/// to ClickHouse before handing the batch back to the caller. With
/// `retry_backoff` this spans just over 60s, so a whole ClickHouse restart
/// (10-30s) is ridden out inside a single flush.
const MAX_FLUSH_ATTEMPTS: u32 = 12;

/// Attempts allowed for the final flush on shutdown.
///
/// The shutdown flush has no next round to hand the batch back to, and the
/// process must exit within the container's stop grace period — spending
/// the whole budget on the first buffer would leave the other two
/// unattempted.
const SHUTDOWN_FLUSH_ATTEMPTS: u32 = 2;

/// ClickHouse inserter configuration
const INSERTER_SEND_TIMEOUT: Duration = Duration::from_secs(10);
const INSERTER_END_TIMEOUT: Duration = Duration::from_secs(10);
const INSERTER_MAX_BYTES: u64 = 50_000_000;
const INSERTER_MAX_ROWS: u64 = 10_000;

/// Delay before the second flush attempt; doubles on each further failure.
///
/// Avoids hammering a ClickHouse instance that is already struggling (e.g.
/// mid-restart) with back-to-back retries at the same rate the previous
/// attempt just failed at.
const INITIAL_RETRY_BACKOFF: Duration = Duration::from_millis(250);

/// Ceiling for the exponential backoff, so late attempts still probe often
/// enough to notice ClickHouse coming back.
const MAX_RETRY_BACKOFF: Duration = Duration::from_secs(10);

/// Batch writer that buffers rows through channels before writing to ClickHouse.
///
/// Implements both `LogExporter` and `OTelExporter`. Handlers send rows
/// through bounded channels; a background task drains and flushes them.
pub struct BatchWriter {
    logs: mpsc::Sender<Vec<LogRow>>,
    otel_logs: mpsc::Sender<Vec<OTelLogRow>>,
    otel_traces: mpsc::Sender<Vec<OTelTraceRow>>,
}

impl BatchWriter {
    /// Spawn the background flush task and return the `BatchWriter` handle
    /// together with the flush loop's `JoinHandle`.
    ///
    /// The background task runs until `shutdown_token` is cancelled. Callers
    /// MUST hold onto the returned handle and `.await` it after cancelling
    /// the token — otherwise the runtime can drop the flush task mid-write
    /// on shutdown, silently losing the final batch.
    #[must_use]
    pub fn spawn(client: Client, shutdown_token: CancellationToken) -> (Self, JoinHandle<()>) {
        let (logs_tx, logs_rx) = mpsc::channel::<Vec<LogRow>>(CHANNEL_CAPACITY);
        let (otel_logs_tx, otel_logs_rx) = mpsc::channel::<Vec<OTelLogRow>>(CHANNEL_CAPACITY);
        let (otel_traces_tx, otel_traces_rx) = mpsc::channel::<Vec<OTelTraceRow>>(CHANNEL_CAPACITY);

        let handle = tokio::spawn(flush_loop(
            client,
            logs_rx,
            otel_logs_rx,
            otel_traces_rx,
            shutdown_token,
        ));

        (
            Self {
                logs: logs_tx,
                otel_logs: otel_logs_tx,
                otel_traces: otel_traces_tx,
            },
            handle,
        )
    }
}

impl crate::port::LogExporter for BatchWriter {
    fn export_batch(
        &self,
        logs: Vec<EnrichedLogEntry>,
    ) -> Pin<Box<dyn Future<Output = Result<(), AggregatorError>> + Send + '_>> {
        Box::pin(async move {
            if logs.is_empty() {
                return Ok(());
            }
            let rows: Vec<LogRow> = logs.into_iter().map(LogRow::from).collect();
            self.logs
                .send(rows)
                .await
                .map_err(|_| AggregatorError::Export("log batch channel closed".to_string()))
        })
    }
}

impl crate::port::OTelExporter for BatchWriter {
    fn export_otel_logs(
        &self,
        logs: Vec<OTelLog>,
    ) -> Pin<Box<dyn Future<Output = Result<(), AggregatorError>> + Send + '_>> {
        Box::pin(async move {
            if logs.is_empty() {
                return Ok(());
            }
            let rows: Vec<OTelLogRow> = logs.into_iter().map(OTelLogRow::from).collect();
            self.otel_logs
                .send(rows)
                .await
                .map_err(|_| AggregatorError::Export("otel log batch channel closed".to_string()))
        })
    }

    fn export_otel_traces(
        &self,
        traces: Vec<OTelTrace>,
    ) -> Pin<Box<dyn Future<Output = Result<(), AggregatorError>> + Send + '_>> {
        Box::pin(async move {
            if traces.is_empty() {
                return Ok(());
            }
            let rows: Vec<OTelTraceRow> = traces.into_iter().map(OTelTraceRow::from).collect();
            self.otel_traces
                .send(rows)
                .await
                .map_err(|_| AggregatorError::Export("otel trace batch channel closed".to_string()))
        })
    }
}

// =========================================================================
// Background flush loop
// =========================================================================

async fn flush_loop(
    client: Client,
    mut log_rx: mpsc::Receiver<Vec<LogRow>>,
    mut otel_log_rx: mpsc::Receiver<Vec<OTelLogRow>>,
    mut otel_trace_rx: mpsc::Receiver<Vec<OTelTraceRow>>,
    shutdown_token: CancellationToken,
) {
    let mut log_buf: Vec<LogRow> = Vec::new();
    let mut otel_log_buf: Vec<OTelLogRow> = Vec::new();
    let mut otel_trace_buf: Vec<OTelTraceRow> = Vec::new();
    let mut flush_interval = tokio::time::interval(Duration::from_secs(FLUSH_INTERVAL_SECS));

    info!(
        "BatchWriter flush loop started (interval={FLUSH_INTERVAL_SECS}s, capacity={CHANNEL_CAPACITY})"
    );

    loop {
        tokio::select! {
            // Periodic flush
            _ = flush_interval.tick() => {
                flush_all(
                    &client, &mut log_buf, &mut otel_log_buf, &mut otel_trace_buf,
                    MAX_FLUSH_ATTEMPTS,
                ).await;
            }

            // Drain log rows
            Some(rows) = log_rx.recv() => {
                log_buf.extend(rows);
                if log_buf.len() >= MAX_BATCH_SIZE {
                    flush_rows(&client, "logs", &mut log_buf, MAX_FLUSH_ATTEMPTS).await;
                }
            }

            // Drain OTel log rows
            Some(rows) = otel_log_rx.recv() => {
                otel_log_buf.extend(rows);
                if otel_log_buf.len() >= MAX_BATCH_SIZE {
                    flush_rows(&client, "otel_logs", &mut otel_log_buf, MAX_FLUSH_ATTEMPTS).await;
                }
            }

            // Drain OTel trace rows
            Some(rows) = otel_trace_rx.recv() => {
                otel_trace_buf.extend(rows);
                if otel_trace_buf.len() >= MAX_BATCH_SIZE {
                    flush_rows(&client, "otel_traces", &mut otel_trace_buf, MAX_FLUSH_ATTEMPTS).await;
                }
            }

            // Shutdown signal
            () = shutdown_token.cancelled() => {
                info!("BatchWriter shutting down: draining channels before final flush");
                drain_channel(&mut log_rx, &mut log_buf);
                drain_channel(&mut otel_log_rx, &mut otel_log_buf);
                drain_channel(&mut otel_trace_rx, &mut otel_trace_buf);
                flush_all(
                    &client, &mut log_buf, &mut otel_log_buf, &mut otel_trace_buf,
                    SHUTDOWN_FLUSH_ATTEMPTS,
                ).await;
                let unwritten = log_buf.len() + otel_log_buf.len() + otel_trace_buf.len();
                if unwritten > 0 {
                    error!(unwritten, "Exiting with rows ClickHouse never accepted");
                }
                break;
            }
        }
    }

    info!("BatchWriter flush loop stopped");
}

/// Close `rx` (rejecting any further sends) and drain every row already
/// buffered in the channel into `buf`.
///
/// Called on shutdown, before the final flush: without this, rows sitting
/// in-channel (senders raced the cancellation signal) are silently lost
/// when the flush loop breaks out of `select!`.
fn drain_channel<T>(rx: &mut mpsc::Receiver<Vec<T>>, buf: &mut Vec<T>) {
    rx.close();
    while let Ok(rows) = rx.try_recv() {
        buf.extend(rows);
    }
}

async fn flush_all(
    client: &Client,
    log_buf: &mut Vec<LogRow>,
    otel_log_buf: &mut Vec<OTelLogRow>,
    otel_trace_buf: &mut Vec<OTelTraceRow>,
    max_attempts: u32,
) {
    if !log_buf.is_empty() {
        flush_rows(client, "logs", log_buf, max_attempts).await;
    }
    if !otel_log_buf.is_empty() {
        flush_rows(client, "otel_logs", otel_log_buf, max_attempts).await;
    }
    if !otel_trace_buf.is_empty() {
        flush_rows(client, "otel_traces", otel_trace_buf, max_attempts).await;
    }
}

async fn flush_rows<T: clickhouse::Row + RowOwned + RowWrite + serde::Serialize>(
    client: &Client,
    table: &str,
    buf: &mut Vec<T>,
    max_attempts: u32,
) {
    let sink = ClickHouseSink { client, table };
    flush_with_retry(table, buf, &sink, max_attempts).await;
}

/// Destination for a batch flush. Exists so `flush_with_retry` can be
/// exercised in tests without a real ClickHouse connection.
trait FlushSink<T> {
    async fn write(&self, rows: &[T]) -> Result<(), AggregatorError>;
}

struct ClickHouseSink<'a> {
    client: &'a Client,
    table: &'a str,
}

impl<T: clickhouse::Row + RowOwned + RowWrite + serde::Serialize> FlushSink<T>
    for ClickHouseSink<'_>
{
    async fn write(&self, rows: &[T]) -> Result<(), AggregatorError> {
        write_batch(self.client, self.table, rows).await
    }
}

/// Exponential backoff before the attempt following `attempt`, capped at
/// `MAX_RETRY_BACKOFF`.
fn retry_backoff(attempt: u32) -> Duration {
    INITIAL_RETRY_BACKOFF
        .saturating_mul(1_u32.checked_shl(attempt - 1).unwrap_or(u32::MAX))
        .min(MAX_RETRY_BACKOFF)
}

/// Flush `buf` via `sink`, retrying up to `max_attempts` times on failure.
///
/// The buffer is cleared only once a write succeeds. Rows that ClickHouse
/// never accepted stay buffered and are retried on the next flush: the
/// forwarder was already answered 200, so this process holds the only copy
/// of them. Holding them also stalls the flush loop, which is the intended
/// backpressure — the bounded row channels fill up and ingest handlers park
/// on `send`, pushing the backlog back to the forwarder (which has its own
/// retry and disk fallback) instead of acknowledging logs this process is
/// about to discard.
async fn flush_with_retry<T>(
    table: &str,
    buf: &mut Vec<T>,
    sink: &impl FlushSink<T>,
    max_attempts: u32,
) {
    if buf.is_empty() {
        return;
    }
    let count = buf.len();

    for attempt in 1..=max_attempts {
        match sink.write(buf.as_slice()).await {
            Ok(()) => {
                info!(table, count, attempt, "Flushed batch to ClickHouse");
                buf.clear();
                return;
            }
            Err(e) if attempt < max_attempts => {
                warn!(
                    table, count, attempt, error = %e,
                    "Failed to flush batch to ClickHouse, retrying"
                );
                tokio::time::sleep(retry_backoff(attempt)).await;
            }
            Err(e) => {
                error!(
                    table, count, attempts = attempt, error = %e,
                    "Flush retry budget exhausted; retaining batch and applying backpressure"
                );
            }
        }
    }
}

async fn write_batch<T: clickhouse::Row + RowOwned + RowWrite + serde::Serialize>(
    client: &Client,
    table: &str,
    rows: &[T],
) -> Result<(), AggregatorError> {
    let mut inserter = client
        .inserter::<T>(table)
        .with_timeouts(Some(INSERTER_SEND_TIMEOUT), Some(INSERTER_END_TIMEOUT))
        .with_max_bytes(INSERTER_MAX_BYTES)
        .with_max_rows(INSERTER_MAX_ROWS);

    // Individual row failures here are per-row (de)serialization problems,
    // not transient network errors - retrying the batch wouldn't help them.
    // Count and report them as a single aggregated error (rather than one
    // `error!` per row, and rather than swallowing them with no signal at
    // all) so a persistently non-zero drop count is visible.
    let mut dropped = 0usize;
    for row in rows {
        if let Err(e) = inserter.write(row).await {
            warn!(table, error = %e, "Failed to write row to ClickHouse inserter, dropping row");
            dropped += 1;
        }
    }
    if dropped > 0 {
        error!(
            table,
            dropped,
            total = rows.len(),
            "Dropped rows while writing batch to ClickHouse inserter"
        );
    }

    inserter.end().await?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::{LogLevel, OTelLog, OTelTrace, SpanKind, StatusCode};
    use crate::port::{LogExporter, OTelExporter};
    use std::collections::HashMap;

    fn make_enriched_log() -> EnrichedLogEntry {
        EnrichedLogEntry {
            service_type: "http".to_string(),
            log_type: "access".to_string(),
            message: "test message".to_string(),
            level: Some(LogLevel::Info),
            timestamp: "2024-01-15T10:30:00.000Z".to_string(),
            stream: "stdout".to_string(),
            container_id: "abc123".to_string(),
            service_name: "test-svc".to_string(),
            service_group: None,
            fields: HashMap::new(),
            method: None,
            path: None,
            status_code: None,
            response_size: None,
            ip_address: None,
            user_agent: None,
            trace_id: None,
            span_id: None,
        }
    }

    fn make_otel_log() -> OTelLog {
        OTelLog {
            timestamp: 1_700_000_000_000_000_000,
            observed_timestamp: 1_700_000_000_000_000_000,
            trace_id: "0".repeat(32),
            span_id: "0".repeat(16),
            trace_flags: 0,
            severity_text: "INFO".to_string(),
            severity_number: 9,
            body: "test".to_string(),
            resource_schema_url: String::new(),
            resource_attributes: std::sync::Arc::new(HashMap::new()),
            scope_schema_url: String::new(),
            scope_name: String::new(),
            scope_version: String::new(),
            scope_attributes: std::sync::Arc::new(HashMap::new()),
            log_attributes: HashMap::new(),
            service_name: "test-svc".to_string(),
        }
    }

    fn make_otel_trace() -> OTelTrace {
        OTelTrace {
            timestamp: 1_700_000_000_000_000_000,
            trace_id: "0".repeat(32),
            span_id: "0".repeat(16),
            parent_span_id: String::new(),
            trace_state: String::new(),
            span_name: "test-span".to_string(),
            span_kind: SpanKind::Server,
            service_name: "test-svc".to_string(),
            resource_attributes: HashMap::new(),
            span_attributes: HashMap::new(),
            duration: 1000,
            status_code: StatusCode::Ok,
            status_message: String::new(),
            events_nested: vec![],
            links_nested: vec![],
        }
    }

    // =========================================================================
    // Channel behavior tests (no ClickHouse needed)
    // =========================================================================

    type WriterChannels = (
        BatchWriter,
        mpsc::Receiver<Vec<LogRow>>,
        mpsc::Receiver<Vec<OTelLogRow>>,
        mpsc::Receiver<Vec<OTelTraceRow>>,
    );

    fn make_writer() -> WriterChannels {
        let (logs_tx, logs_rx) = mpsc::channel(16);
        let (otel_logs_tx, otel_logs_rx) = mpsc::channel(16);
        let (otel_traces_tx, otel_traces_rx) = mpsc::channel(16);
        let writer = BatchWriter {
            logs: logs_tx,
            otel_logs: otel_logs_tx,
            otel_traces: otel_traces_tx,
        };
        (writer, logs_rx, otel_logs_rx, otel_traces_rx)
    }

    #[tokio::test]
    async fn export_batch_empty_returns_ok() {
        let (writer, _, _, _) = make_writer();
        let result = writer.export_batch(vec![]).await;
        assert!(result.is_ok());
    }

    #[tokio::test]
    async fn export_otel_logs_empty_returns_ok() {
        let (writer, _, _, _) = make_writer();
        let result = writer.export_otel_logs(vec![]).await;
        assert!(result.is_ok());
    }

    #[tokio::test]
    async fn export_otel_traces_empty_returns_ok() {
        let (writer, _, _, _) = make_writer();
        let result = writer.export_otel_traces(vec![]).await;
        assert!(result.is_ok());
    }

    #[tokio::test]
    async fn export_batch_sends_rows_to_channel() {
        let (writer, mut logs_rx, _, _) = make_writer();

        let entries = vec![make_enriched_log(), make_enriched_log()];
        writer.export_batch(entries).await.unwrap();

        let received = logs_rx.recv().await.unwrap();
        assert_eq!(received.len(), 2);
    }

    #[tokio::test]
    async fn export_otel_logs_sends_rows_to_channel() {
        let (writer, _, mut otel_logs_rx, _) = make_writer();

        let logs = vec![make_otel_log()];
        writer.export_otel_logs(logs).await.unwrap();

        let received = otel_logs_rx.recv().await.unwrap();
        assert_eq!(received.len(), 1);
    }

    #[tokio::test]
    async fn export_otel_traces_sends_rows_to_channel() {
        let (writer, _, _, mut otel_traces_rx) = make_writer();

        let traces = vec![make_otel_trace(), make_otel_trace(), make_otel_trace()];
        writer.export_otel_traces(traces).await.unwrap();

        let received = otel_traces_rx.recv().await.unwrap();
        assert_eq!(received.len(), 3);
    }

    #[tokio::test]
    async fn export_batch_errors_when_channel_closed() {
        let (writer, logs_rx, _, _) = make_writer();
        drop(logs_rx);

        let result = writer.export_batch(vec![make_enriched_log()]).await;
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("channel closed"));
    }

    #[tokio::test]
    async fn export_otel_logs_errors_when_channel_closed() {
        let (writer, _, otel_logs_rx, _) = make_writer();
        drop(otel_logs_rx);

        let result = writer.export_otel_logs(vec![make_otel_log()]).await;
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("channel closed"));
    }

    #[tokio::test]
    async fn export_otel_traces_errors_when_channel_closed() {
        let (writer, _, _, otel_traces_rx) = make_writer();
        drop(otel_traces_rx);

        let result = writer.export_otel_traces(vec![make_otel_trace()]).await;
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("channel closed"));
    }

    // =========================================================================
    // Shutdown: JoinHandle is stored and awaited (finding #1)
    // =========================================================================

    #[tokio::test]
    async fn spawn_returns_join_handle_that_completes_after_cancel() {
        // No real ClickHouse needed: buffers are empty, so `flush_all` on the
        // shutdown branch never attempts a network write.
        let client = Client::default().with_url("http://127.0.0.1:1");
        let token = CancellationToken::new();
        let (_writer, handle) = BatchWriter::spawn(client, token.clone());

        token.cancel();

        let result = tokio::time::timeout(Duration::from_secs(5), handle).await;
        assert!(
            result.is_ok(),
            "flush loop task should complete promptly once cancelled and awaited"
        );
    }

    // =========================================================================
    // Shutdown: in-channel rows are drained before the final flush (finding #2)
    // =========================================================================

    #[test]
    fn drain_channel_collects_buffered_rows_and_closes_receiver() {
        let (tx, mut rx) = mpsc::channel::<Vec<u32>>(4);
        tx.try_send(vec![1, 2]).unwrap();
        tx.try_send(vec![3]).unwrap();

        let mut buf = Vec::new();
        drain_channel(&mut rx, &mut buf);

        assert_eq!(buf, vec![1, 2, 3]);
        // Closed: senders can no longer enqueue further rows.
        assert!(tx.try_send(vec![4]).is_err());
    }

    #[test]
    fn drain_channel_on_empty_channel_leaves_buffer_untouched() {
        let (_tx, mut rx) = mpsc::channel::<Vec<u32>>(4);

        let mut buf = vec![9];
        drain_channel(&mut rx, &mut buf);

        assert_eq!(buf, vec![9]);
    }

    // =========================================================================
    // flush_with_retry: failed writes must not lose the batch (finding #3)
    // =========================================================================

    /// Test double for `FlushSink`, driven by a plain synchronous closure so
    /// tests can inject failures without a real ClickHouse connection.
    struct MockSink<F> {
        write_fn: F,
    }

    impl<F> FlushSink<i32> for MockSink<F>
    where
        F: Fn(&[i32]) -> Result<(), AggregatorError>,
    {
        async fn write(&self, rows: &[i32]) -> Result<(), AggregatorError> {
            (self.write_fn)(rows)
        }
    }

    #[tokio::test]
    async fn flush_with_retry_clears_buffer_and_calls_once_on_success() {
        let mut buf = vec![1_i32, 2, 3];
        let calls = std::sync::atomic::AtomicU32::new(0);

        let sink = MockSink {
            write_fn: |rows: &[i32]| {
                calls.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
                assert_eq!(rows, [1, 2, 3]);
                Ok(())
            },
        };
        flush_with_retry("t", &mut buf, &sink, MAX_FLUSH_ATTEMPTS).await;

        assert!(buf.is_empty());
        assert_eq!(calls.load(std::sync::atomic::Ordering::SeqCst), 1);
    }

    #[tokio::test]
    async fn flush_with_retry_is_a_noop_on_empty_buffer() {
        let mut buf: Vec<i32> = Vec::new();
        let calls = std::sync::atomic::AtomicU32::new(0);

        let sink = MockSink {
            write_fn: |_rows: &[i32]| {
                calls.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
                Ok(())
            },
        };
        flush_with_retry("t", &mut buf, &sink, MAX_FLUSH_ATTEMPTS).await;

        assert_eq!(calls.load(std::sync::atomic::Ordering::SeqCst), 0);
    }

    #[tokio::test]
    async fn flush_with_retry_retains_rows_across_failed_attempt_then_succeeds() {
        let mut buf = vec![1_i32, 2, 3];
        let attempt = std::sync::atomic::AtomicU32::new(0);

        let sink = MockSink {
            write_fn: |rows: &[i32]| {
                let n = attempt.fetch_add(1, std::sync::atomic::Ordering::SeqCst) + 1;
                assert_eq!(
                    rows,
                    [1, 2, 3],
                    "rows must still be present on retry {n} — a failed attempt must not have dropped them"
                );
                if n < 2 {
                    Err(AggregatorError::ClickHouse(
                        clickhouse::error::Error::Custom("boom".to_string()),
                    ))
                } else {
                    Ok(())
                }
            },
        };
        flush_with_retry("t", &mut buf, &sink, MAX_FLUSH_ATTEMPTS).await;

        assert!(buf.is_empty(), "buffer clears once the retry succeeds");
        assert_eq!(attempt.load(std::sync::atomic::Ordering::SeqCst), 2);
    }

    #[tokio::test(start_paused = true)]
    async fn flush_with_retry_backs_off_between_failed_attempts() {
        let mut buf = vec![1_i32, 2, 3];
        let attempt = std::sync::atomic::AtomicU32::new(0);
        let timestamps = std::sync::Mutex::new(Vec::new());

        let sink = MockSink {
            write_fn: |rows: &[i32]| {
                timestamps.lock().unwrap().push(tokio::time::Instant::now());
                let n = attempt.fetch_add(1, std::sync::atomic::Ordering::SeqCst) + 1;
                assert_eq!(rows, [1, 2, 3]);
                if n < MAX_FLUSH_ATTEMPTS {
                    Err(AggregatorError::ClickHouse(
                        clickhouse::error::Error::Custom("boom".to_string()),
                    ))
                } else {
                    Ok(())
                }
            },
        };
        flush_with_retry("t", &mut buf, &sink, MAX_FLUSH_ATTEMPTS).await;

        let ts = timestamps.into_inner().unwrap();
        assert_eq!(ts.len(), MAX_FLUSH_ATTEMPTS as usize);
        for (i, pair) in ts.windows(2).enumerate() {
            let attempt = u32::try_from(i).unwrap() + 1;
            assert!(
                pair[1] - pair[0] >= retry_backoff(attempt),
                "must wait the backoff for attempt {attempt} before the next one"
            );
        }
    }

    #[tokio::test(start_paused = true)]
    async fn flush_with_retry_retains_batch_after_retry_budget_exhausted() {
        let mut buf = vec![1_i32, 2, 3];
        let attempts = std::sync::atomic::AtomicU32::new(0);

        let sink = MockSink {
            write_fn: |_rows: &[i32]| {
                attempts.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
                Err(AggregatorError::ClickHouse(
                    clickhouse::error::Error::Custom("still down".to_string()),
                ))
            },
        };
        flush_with_retry("t", &mut buf, &sink, MAX_FLUSH_ATTEMPTS).await;

        assert_eq!(
            buf,
            vec![1, 2, 3],
            "batch must be retained for the next flush (backpressure), not discarded: \
             the forwarder was already answered 200, so no other copy exists"
        );
        assert_eq!(
            attempts.load(std::sync::atomic::Ordering::SeqCst),
            MAX_FLUSH_ATTEMPTS
        );
    }

    /// A ClickHouse restart takes 10-30s. A retry budget that expires in
    /// under a second turns every restart into total log loss for the whole
    /// downtime window.
    #[tokio::test(start_paused = true)]
    async fn flush_with_retry_budget_outlasts_a_clickhouse_restart() {
        let mut buf = vec![1_i32];

        let sink = MockSink {
            write_fn: |_rows: &[i32]| {
                Err(AggregatorError::ClickHouse(
                    clickhouse::error::Error::Custom("connection refused".to_string()),
                ))
            },
        };
        let start = tokio::time::Instant::now();
        flush_with_retry("t", &mut buf, &sink, MAX_FLUSH_ATTEMPTS).await;
        let spent = start.elapsed();

        assert!(
            spent >= Duration::from_mins(1),
            "retry budget must keep trying for at least 60s across a ClickHouse \
             restart, but gave up after {spent:?}"
        );
    }

    #[tokio::test(start_paused = true)]
    async fn flush_with_retry_backoff_grows_exponentially() {
        let mut buf = vec![1_i32];
        let timestamps = std::sync::Mutex::new(Vec::new());

        let sink = MockSink {
            write_fn: |_rows: &[i32]| {
                timestamps.lock().unwrap().push(tokio::time::Instant::now());
                Err(AggregatorError::ClickHouse(
                    clickhouse::error::Error::Custom("still down".to_string()),
                ))
            },
        };
        flush_with_retry("t", &mut buf, &sink, MAX_FLUSH_ATTEMPTS).await;

        let ts = timestamps.into_inner().unwrap();
        let gaps: Vec<Duration> = ts.windows(2).map(|p| p[1] - p[0]).collect();
        assert!(gaps.len() >= 4, "budget too small to observe growth");
        for pair in gaps.windows(2) {
            assert!(
                pair[1] >= pair[0],
                "backoff must never shrink between attempts: {gaps:?}"
            );
        }
        assert!(
            gaps[3] >= gaps[0] * 8,
            "backoff must grow exponentially, not linearly: {gaps:?}"
        );
    }
}
