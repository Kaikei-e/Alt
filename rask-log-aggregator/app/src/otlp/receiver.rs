//! OTLP HTTP Receiver implementation
//!
//! Supports:
//! - POST /v1/logs (OTLP HTTP/protobuf)
//! - POST /v1/traces (OTLP HTTP/protobuf)

use std::sync::Arc;

use axum::{
    Router,
    body::Bytes,
    extract::State,
    http::{StatusCode, header},
    response::IntoResponse,
    routing::post,
};
use opentelemetry_proto::tonic::collector::{
    logs::v1::{ExportLogsServiceRequest, ExportLogsServiceResponse},
    trace::v1::{ExportTraceServiceRequest, ExportTraceServiceResponse},
};
use prost::Message;
use tracing::{error, info, instrument};

use crate::log_exporter::clickhouse_exporter::ClickHouseExporter;
use crate::otlp::converter::{convert_log_records, convert_spans};

/// Application state for OTLP handlers
#[derive(Clone)]
pub struct OTLPState {
    pub exporter: Arc<ClickHouseExporter>,
}

/// Create Axum router for OTLP HTTP endpoints
pub fn otlp_routes(state: OTLPState) -> Router {
    Router::new()
        .route("/v1/logs", post(receive_logs_http))
        .route("/v1/traces", post(receive_traces_http))
        .with_state(state)
}

/// OTLP HTTP logs receiver
///
/// Accepts: application/x-protobuf
/// Returns: application/x-protobuf
#[instrument(skip(state, body), fields(body_size = body.len()))]
async fn receive_logs_http(State(state): State<OTLPState>, body: Bytes) -> impl IntoResponse {
    // Decode protobuf request
    let request = match ExportLogsServiceRequest::decode(body) {
        Ok(req) => req,
        Err(e) => {
            error!(error = %e, "Failed to decode OTLP logs request");
            return (
                StatusCode::BAD_REQUEST,
                [(header::CONTENT_TYPE, "application/x-protobuf")],
                Bytes::new(),
            );
        }
    };

    // Convert and export logs
    let logs = convert_log_records(&request);
    let log_count = logs.len();

    if log_count == 0 {
        // Return success for empty request
        let response = ExportLogsServiceResponse::default();
        let mut buf = Vec::with_capacity(response.encoded_len());
        let _ = response.encode(&mut buf);
        return (
            StatusCode::OK,
            [(header::CONTENT_TYPE, "application/x-protobuf")],
            Bytes::from(buf),
        );
    }

    if let Err(e) = state.exporter.export_otel_logs(logs).await {
        error!(error = %e, "Failed to export logs to ClickHouse");
        return (
            StatusCode::INTERNAL_SERVER_ERROR,
            [(header::CONTENT_TYPE, "application/x-protobuf")],
            Bytes::new(),
        );
    }

    info!(count = log_count, "Exported OTLP logs");

    // Return success response
    let response = ExportLogsServiceResponse::default();
    let mut buf = Vec::with_capacity(response.encoded_len());
    let _ = response.encode(&mut buf);

    (
        StatusCode::OK,
        [(header::CONTENT_TYPE, "application/x-protobuf")],
        Bytes::from(buf),
    )
}

/// OTLP HTTP traces receiver
#[instrument(skip(state, body), fields(body_size = body.len()))]
async fn receive_traces_http(State(state): State<OTLPState>, body: Bytes) -> impl IntoResponse {
    let request = match ExportTraceServiceRequest::decode(body) {
        Ok(req) => req,
        Err(e) => {
            error!(error = %e, "Failed to decode OTLP traces request");
            return (
                StatusCode::BAD_REQUEST,
                [(header::CONTENT_TYPE, "application/x-protobuf")],
                Bytes::new(),
            );
        }
    };

    let spans = convert_spans(&request);
    let span_count = spans.len();

    if span_count == 0 {
        let response = ExportTraceServiceResponse::default();
        let mut buf = Vec::with_capacity(response.encoded_len());
        let _ = response.encode(&mut buf);
        return (
            StatusCode::OK,
            [(header::CONTENT_TYPE, "application/x-protobuf")],
            Bytes::from(buf),
        );
    }

    if let Err(e) = state.exporter.export_otel_traces(spans).await {
        error!(error = %e, "Failed to export traces to ClickHouse");
        return (
            StatusCode::INTERNAL_SERVER_ERROR,
            [(header::CONTENT_TYPE, "application/x-protobuf")],
            Bytes::new(),
        );
    }

    info!(count = span_count, "Exported OTLP traces");

    let response = ExportTraceServiceResponse::default();
    let mut buf = Vec::with_capacity(response.encoded_len());
    let _ = response.encode(&mut buf);

    (
        StatusCode::OK,
        [(header::CONTENT_TYPE, "application/x-protobuf")],
        Bytes::from(buf),
    )
}

#[cfg(test)]
mod tests {
    // Note: Full integration tests require a running ClickHouse instance

    #[tokio::test]
    async fn test_empty_logs_request() {
        // This test would require mocking ClickHouseExporter
        // For now, we verify the module compiles correctly
    }
}
