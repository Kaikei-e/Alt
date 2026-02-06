use crate::domain::EnrichedLogEntry;
use crate::port::LogExporter;
use axum::extract::State;
use axum::http::StatusCode;
use axum::response::IntoResponse;
use std::sync::Arc;
use tracing::{error, info};

/// Handler for POST /v1/aggregate (legacy NDJSON logs from rask-log-forwarder)
pub async fn aggregate_handler(
    State(exporter): State<Arc<dyn LogExporter>>,
    body: String,
) -> impl IntoResponse {
    info!(
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
                error!("Failed to parse log entry: {e} - Line: {line}");
                None
            }
        })
        .collect();

    info!("Parsed {} log entries from request", logs.len());

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
