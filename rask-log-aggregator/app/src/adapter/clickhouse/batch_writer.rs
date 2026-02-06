//! Channel-based batch buffering for ClickHouse writes.
//!
//! Instead of each HTTP request immediately creating a ClickHouse inserter,
//! rows are sent through bounded `mpsc` channels to a background task that
//! aggregates batches and writes them periodically.

use crate::adapter::clickhouse::otel_row::{OTelLogRow, OTelTraceRow};
use crate::adapter::clickhouse::row::LogRow;
use crate::domain::{EnrichedLogEntry, OTelLog, OTelTrace};
use crate::error::AggregatorError;
use clickhouse::Client;
use std::future::Future;
use std::pin::Pin;
use std::time::Duration;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;
use tracing::{error, info};

/// Channel capacity for each row type.
const CHANNEL_CAPACITY: usize = 1024;

/// How often the background task flushes to ClickHouse (seconds).
const FLUSH_INTERVAL_SECS: u64 = 5;

/// Maximum rows per batch flush.
const MAX_BATCH_SIZE: usize = 5000;

/// ClickHouse inserter configuration
const INSERTER_SEND_TIMEOUT: Duration = Duration::from_secs(10);
const INSERTER_END_TIMEOUT: Duration = Duration::from_secs(10);
const INSERTER_MAX_BYTES: u64 = 50_000_000;
const INSERTER_MAX_ROWS: u64 = 10_000;

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
    /// Spawn the background flush task and return the `BatchWriter` handle.
    ///
    /// The background task runs until `shutdown_token` is cancelled.
    #[must_use]
    pub fn spawn(client: Client, shutdown_token: CancellationToken) -> Self {
        let (logs_tx, logs_rx) = mpsc::channel::<Vec<LogRow>>(CHANNEL_CAPACITY);
        let (otel_logs_tx, otel_logs_rx) = mpsc::channel::<Vec<OTelLogRow>>(CHANNEL_CAPACITY);
        let (otel_traces_tx, otel_traces_rx) = mpsc::channel::<Vec<OTelTraceRow>>(CHANNEL_CAPACITY);

        tokio::spawn(flush_loop(
            client,
            logs_rx,
            otel_logs_rx,
            otel_traces_rx,
            shutdown_token,
        ));

        Self {
            logs: logs_tx,
            otel_logs: otel_logs_tx,
            otel_traces: otel_traces_tx,
        }
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
                flush_all(&client, &mut log_buf, &mut otel_log_buf, &mut otel_trace_buf).await;
            }

            // Drain log rows
            Some(rows) = log_rx.recv() => {
                log_buf.extend(rows);
                if log_buf.len() >= MAX_BATCH_SIZE {
                    flush_rows(&client, "logs", &mut log_buf).await;
                }
            }

            // Drain OTel log rows
            Some(rows) = otel_log_rx.recv() => {
                otel_log_buf.extend(rows);
                if otel_log_buf.len() >= MAX_BATCH_SIZE {
                    flush_rows(&client, "otel_logs", &mut otel_log_buf).await;
                }
            }

            // Drain OTel trace rows
            Some(rows) = otel_trace_rx.recv() => {
                otel_trace_buf.extend(rows);
                if otel_trace_buf.len() >= MAX_BATCH_SIZE {
                    flush_rows(&client, "otel_traces", &mut otel_trace_buf).await;
                }
            }

            // Shutdown signal
            () = shutdown_token.cancelled() => {
                info!("BatchWriter shutting down, flushing remaining rows");
                flush_all(&client, &mut log_buf, &mut otel_log_buf, &mut otel_trace_buf).await;
                break;
            }
        }
    }

    info!("BatchWriter flush loop stopped");
}

async fn flush_all(
    client: &Client,
    log_buf: &mut Vec<LogRow>,
    otel_log_buf: &mut Vec<OTelLogRow>,
    otel_trace_buf: &mut Vec<OTelTraceRow>,
) {
    if !log_buf.is_empty() {
        flush_rows(client, "logs", log_buf).await;
    }
    if !otel_log_buf.is_empty() {
        flush_rows(client, "otel_logs", otel_log_buf).await;
    }
    if !otel_trace_buf.is_empty() {
        flush_rows(client, "otel_traces", otel_trace_buf).await;
    }
}

async fn flush_rows<T: clickhouse::Row + serde::Serialize>(
    client: &Client,
    table: &str,
    buf: &mut Vec<T>,
) {
    let count = buf.len();
    match write_batch(client, table, buf.drain(..)).await {
        Ok(()) => {
            info!(table, count, "Flushed batch to ClickHouse");
        }
        Err(e) => {
            error!(table, count, error = %e, "Failed to flush batch to ClickHouse");
        }
    }
}

async fn write_batch<T: clickhouse::Row + serde::Serialize>(
    client: &Client,
    table: &str,
    rows: impl Iterator<Item = T>,
) -> Result<(), AggregatorError> {
    let mut inserter = client
        .inserter::<T>(table)?
        .with_timeouts(Some(INSERTER_SEND_TIMEOUT), Some(INSERTER_END_TIMEOUT))
        .with_max_bytes(INSERTER_MAX_BYTES)
        .with_max_rows(INSERTER_MAX_ROWS);

    for row in rows {
        if let Err(e) = inserter.write(&row) {
            error!("Failed to write row to ClickHouse inserter: {e}");
        }
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
            resource_attributes: HashMap::new(),
            scope_schema_url: String::new(),
            scope_name: String::new(),
            scope_version: String::new(),
            scope_attributes: HashMap::new(),
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

    fn make_writer() -> (
        BatchWriter,
        mpsc::Receiver<Vec<LogRow>>,
        mpsc::Receiver<Vec<OTelLogRow>>,
        mpsc::Receiver<Vec<OTelTraceRow>>,
    ) {
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
}
