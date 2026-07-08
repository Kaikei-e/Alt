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
            info!("Successfully exported {log_count} log entries to ClickHouse");
            (StatusCode::OK, "OK")
        }
        Err(e) => {
            error!("Failed to export logs to ClickHouse: {e}");
            (StatusCode::INTERNAL_SERVER_ERROR, "Export failed")
        }
    }
}
