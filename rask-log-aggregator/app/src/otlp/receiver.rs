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

use crate::otlp::converter::{convert_log_records, convert_spans};
use crate::port::OTelExporter;

/// Application state for OTLP handlers
#[derive(Clone)]
pub struct OTLPState {
    pub exporter: Arc<dyn OTelExporter>,
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
    use super::*;
    use crate::domain::{OTelLog, OTelTrace};
    use crate::error::AggregatorError;
    use axum_test::TestServer;
    use opentelemetry_proto::tonic::collector::{
        logs::v1::ExportLogsServiceRequest, trace::v1::ExportTraceServiceRequest,
    };
    use opentelemetry_proto::tonic::common::v1::{AnyValue, KeyValue, any_value};
    use opentelemetry_proto::tonic::logs::v1::{LogRecord, ResourceLogs, ScopeLogs};
    use opentelemetry_proto::tonic::resource::v1::Resource;
    use opentelemetry_proto::tonic::trace::v1::{ResourceSpans, ScopeSpans, Span};
    use std::sync::atomic::{AtomicBool, AtomicUsize, Ordering};

    /// Mock OTelExporter for testing
    struct MockOTelExporter {
        logs_count: AtomicUsize,
        traces_count: AtomicUsize,
        should_fail: AtomicBool,
    }

    impl MockOTelExporter {
        fn new() -> Self {
            Self {
                logs_count: AtomicUsize::new(0),
                traces_count: AtomicUsize::new(0),
                should_fail: AtomicBool::new(false),
            }
        }

        fn set_should_fail(&self, fail: bool) {
            self.should_fail.store(fail, Ordering::SeqCst);
        }

        fn logs_exported(&self) -> usize {
            self.logs_count.load(Ordering::SeqCst)
        }

        fn traces_exported(&self) -> usize {
            self.traces_count.load(Ordering::SeqCst)
        }
    }

    impl OTelExporter for MockOTelExporter {
        fn export_otel_logs(
            &self,
            logs: Vec<OTelLog>,
        ) -> std::pin::Pin<
            Box<dyn std::future::Future<Output = Result<(), AggregatorError>> + Send + '_>,
        > {
            let count = logs.len();
            Box::pin(async move {
                if self.should_fail.load(Ordering::SeqCst) {
                    return Err(AggregatorError::Export("Mock export failure".to_string()));
                }
                self.logs_count.fetch_add(count, Ordering::SeqCst);
                Ok(())
            })
        }

        fn export_otel_traces(
            &self,
            traces: Vec<OTelTrace>,
        ) -> std::pin::Pin<
            Box<dyn std::future::Future<Output = Result<(), AggregatorError>> + Send + '_>,
        > {
            let count = traces.len();
            Box::pin(async move {
                if self.should_fail.load(Ordering::SeqCst) {
                    return Err(AggregatorError::Export("Mock export failure".to_string()));
                }
                self.traces_count.fetch_add(count, Ordering::SeqCst);
                Ok(())
            })
        }
    }

    fn create_test_server(exporter: Arc<dyn OTelExporter>) -> TestServer {
        let state = OTLPState { exporter };
        let app = otlp_routes(state);
        TestServer::new(app).expect("Failed to create test server")
    }

    // =========================================================================
    // receive_logs_http tests
    // =========================================================================

    #[tokio::test]
    async fn test_logs_empty_body_returns_ok() {
        let exporter = Arc::new(MockOTelExporter::new());
        let server = create_test_server(exporter.clone());

        // Empty body (empty protobuf message)
        let request = ExportLogsServiceRequest::default();
        let body = request.encode_to_vec();

        let response = server
            .post("/v1/logs")
            .content_type("application/x-protobuf")
            .bytes(body.into())
            .await;

        response.assert_status(StatusCode::OK);
        assert_eq!(exporter.logs_exported(), 0);
    }

    #[tokio::test]
    async fn test_logs_invalid_protobuf_returns_bad_request() {
        let exporter = Arc::new(MockOTelExporter::new());
        let server = create_test_server(exporter);

        // Invalid protobuf bytes
        let invalid_body = vec![0xFF, 0xFF, 0xFF, 0xFF];

        let response = server
            .post("/v1/logs")
            .content_type("application/x-protobuf")
            .bytes(invalid_body.into())
            .await;

        response.assert_status(StatusCode::BAD_REQUEST);
    }

    #[tokio::test]
    async fn test_logs_empty_log_records_returns_ok() {
        let exporter = Arc::new(MockOTelExporter::new());
        let server = create_test_server(exporter.clone());

        // Request with resource logs but no actual log records
        let request = ExportLogsServiceRequest {
            resource_logs: vec![ResourceLogs {
                resource: Some(Resource::default()),
                scope_logs: vec![ScopeLogs {
                    scope: None,
                    log_records: vec![], // Empty log records
                    schema_url: String::new(),
                }],
                schema_url: String::new(),
            }],
        };
        let body = request.encode_to_vec();

        let response = server
            .post("/v1/logs")
            .content_type("application/x-protobuf")
            .bytes(body.into())
            .await;

        response.assert_status(StatusCode::OK);
        assert_eq!(exporter.logs_exported(), 0);
    }

    #[tokio::test]
    async fn test_logs_valid_request_exports_logs() {
        let exporter = Arc::new(MockOTelExporter::new());
        let server = create_test_server(exporter.clone());

        // Create a valid log record
        let request = ExportLogsServiceRequest {
            resource_logs: vec![ResourceLogs {
                resource: Some(Resource {
                    attributes: vec![KeyValue {
                        key: "service.name".to_string(),
                        value: Some(AnyValue {
                            value: Some(any_value::Value::StringValue("test-service".to_string())),
                        }),
                    }],
                    dropped_attributes_count: 0,
                    entity_refs: vec![],
                }),
                scope_logs: vec![ScopeLogs {
                    scope: None,
                    log_records: vec![LogRecord {
                        time_unix_nano: 1700000000000000000,
                        observed_time_unix_nano: 1700000000000000000,
                        severity_number: 9, // INFO
                        severity_text: "INFO".to_string(),
                        body: Some(AnyValue {
                            value: Some(any_value::Value::StringValue(
                                "Test log message".to_string(),
                            )),
                        }),
                        attributes: vec![],
                        dropped_attributes_count: 0,
                        flags: 0,
                        trace_id: vec![0; 16],
                        span_id: vec![0; 8],
                        event_name: String::new(),
                    }],
                    schema_url: String::new(),
                }],
                schema_url: String::new(),
            }],
        };
        let body = request.encode_to_vec();

        let response = server
            .post("/v1/logs")
            .content_type("application/x-protobuf")
            .bytes(body.into())
            .await;

        response.assert_status(StatusCode::OK);
        assert_eq!(exporter.logs_exported(), 1);
    }

    #[tokio::test]
    async fn test_logs_export_failure_returns_internal_error() {
        let exporter = Arc::new(MockOTelExporter::new());
        exporter.set_should_fail(true);
        let server = create_test_server(exporter);

        // Create a valid log record that will fail to export
        let request = ExportLogsServiceRequest {
            resource_logs: vec![ResourceLogs {
                resource: Some(Resource::default()),
                scope_logs: vec![ScopeLogs {
                    scope: None,
                    log_records: vec![LogRecord {
                        time_unix_nano: 1700000000000000000,
                        observed_time_unix_nano: 1700000000000000000,
                        severity_number: 9,
                        severity_text: "INFO".to_string(),
                        body: Some(AnyValue {
                            value: Some(any_value::Value::StringValue("Test".to_string())),
                        }),
                        attributes: vec![],
                        dropped_attributes_count: 0,
                        flags: 0,
                        trace_id: vec![0; 16],
                        span_id: vec![0; 8],
                        event_name: String::new(),
                    }],
                    schema_url: String::new(),
                }],
                schema_url: String::new(),
            }],
        };
        let body = request.encode_to_vec();

        let response = server
            .post("/v1/logs")
            .content_type("application/x-protobuf")
            .bytes(body.into())
            .await;

        response.assert_status(StatusCode::INTERNAL_SERVER_ERROR);
    }

    // =========================================================================
    // receive_traces_http tests
    // =========================================================================

    #[tokio::test]
    async fn test_traces_empty_body_returns_ok() {
        let exporter = Arc::new(MockOTelExporter::new());
        let server = create_test_server(exporter.clone());

        let request = ExportTraceServiceRequest::default();
        let body = request.encode_to_vec();

        let response = server
            .post("/v1/traces")
            .content_type("application/x-protobuf")
            .bytes(body.into())
            .await;

        response.assert_status(StatusCode::OK);
        assert_eq!(exporter.traces_exported(), 0);
    }

    #[tokio::test]
    async fn test_traces_invalid_protobuf_returns_bad_request() {
        let exporter = Arc::new(MockOTelExporter::new());
        let server = create_test_server(exporter);

        let invalid_body = vec![0xFF, 0xFF, 0xFF, 0xFF];

        let response = server
            .post("/v1/traces")
            .content_type("application/x-protobuf")
            .bytes(invalid_body.into())
            .await;

        response.assert_status(StatusCode::BAD_REQUEST);
    }

    #[tokio::test]
    async fn test_traces_empty_spans_returns_ok() {
        let exporter = Arc::new(MockOTelExporter::new());
        let server = create_test_server(exporter.clone());

        let request = ExportTraceServiceRequest {
            resource_spans: vec![ResourceSpans {
                resource: Some(Resource::default()),
                scope_spans: vec![ScopeSpans {
                    scope: None,
                    spans: vec![], // Empty spans
                    schema_url: String::new(),
                }],
                schema_url: String::new(),
            }],
        };
        let body = request.encode_to_vec();

        let response = server
            .post("/v1/traces")
            .content_type("application/x-protobuf")
            .bytes(body.into())
            .await;

        response.assert_status(StatusCode::OK);
        assert_eq!(exporter.traces_exported(), 0);
    }

    #[tokio::test]
    async fn test_traces_valid_request_exports_spans() {
        let exporter = Arc::new(MockOTelExporter::new());
        let server = create_test_server(exporter.clone());

        let request = ExportTraceServiceRequest {
            resource_spans: vec![ResourceSpans {
                resource: Some(Resource {
                    attributes: vec![KeyValue {
                        key: "service.name".to_string(),
                        value: Some(AnyValue {
                            value: Some(any_value::Value::StringValue("test-service".to_string())),
                        }),
                    }],
                    dropped_attributes_count: 0,
                    entity_refs: vec![],
                }),
                scope_spans: vec![ScopeSpans {
                    scope: None,
                    spans: vec![Span {
                        trace_id: vec![0; 16],
                        span_id: vec![0; 8],
                        parent_span_id: vec![],
                        name: "test-span".to_string(),
                        kind: 2, // Server
                        start_time_unix_nano: 1700000000000000000,
                        end_time_unix_nano: 1700000001000000000,
                        attributes: vec![],
                        dropped_attributes_count: 0,
                        events: vec![],
                        dropped_events_count: 0,
                        links: vec![],
                        dropped_links_count: 0,
                        status: None,
                        trace_state: String::new(),
                        flags: 0,
                    }],
                    schema_url: String::new(),
                }],
                schema_url: String::new(),
            }],
        };
        let body = request.encode_to_vec();

        let response = server
            .post("/v1/traces")
            .content_type("application/x-protobuf")
            .bytes(body.into())
            .await;

        response.assert_status(StatusCode::OK);
        assert_eq!(exporter.traces_exported(), 1);
    }

    #[tokio::test]
    async fn test_traces_export_failure_returns_internal_error() {
        let exporter = Arc::new(MockOTelExporter::new());
        exporter.set_should_fail(true);
        let server = create_test_server(exporter);

        let request = ExportTraceServiceRequest {
            resource_spans: vec![ResourceSpans {
                resource: Some(Resource::default()),
                scope_spans: vec![ScopeSpans {
                    scope: None,
                    spans: vec![Span {
                        trace_id: vec![0; 16],
                        span_id: vec![0; 8],
                        parent_span_id: vec![],
                        name: "test-span".to_string(),
                        kind: 2,
                        start_time_unix_nano: 1700000000000000000,
                        end_time_unix_nano: 1700000001000000000,
                        attributes: vec![],
                        dropped_attributes_count: 0,
                        events: vec![],
                        dropped_events_count: 0,
                        links: vec![],
                        dropped_links_count: 0,
                        status: None,
                        trace_state: String::new(),
                        flags: 0,
                    }],
                    schema_url: String::new(),
                }],
                schema_url: String::new(),
            }],
        };
        let body = request.encode_to_vec();

        let response = server
            .post("/v1/traces")
            .content_type("application/x-protobuf")
            .bytes(body.into())
            .await;

        response.assert_status(StatusCode::INTERNAL_SERVER_ERROR);
    }
}
