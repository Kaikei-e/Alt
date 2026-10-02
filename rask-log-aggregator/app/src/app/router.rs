use crate::auth::{IngestToken, require_ingest_token};
use crate::handler::aggregate::aggregate_handler;
use crate::handler::health::health_handler;
use crate::otlp::otlp_routes;
use crate::otlp::receiver::OTLPState;
use crate::port::{LogExporter, OTelExporter};
use axum::Extension;
use axum::Router;
use axum::extract::DefaultBodyLimit;
use axum::middleware;
use axum::routing::{get, post};
use std::sync::Arc;
use tower_http::decompression::RequestDecompressionLayer;

/// The forwarder's default `batch_size` is 10,000 entries at an estimated
/// ~500 bytes/entry (see rask-log-forwarder `ESTIMATED_ENTRY_SIZE`), i.e.
/// ~5 MB uncompressed per batch. Set the limit well above that so legitimate
/// batches are never rejected with 413, regardless of compression.
const MAX_REQUEST_BODY_BYTES: usize = 20 * 1024 * 1024; // 20 MB

/// Build the main HTTP router (health + legacy aggregate).
///
/// Health endpoint is intentionally exempt from auth (D-02).
/// Aggregate endpoint requires `Authorization: Bearer <token>`.
pub fn main_router(exporter: Arc<dyn LogExporter>, token: Arc<IngestToken>) -> Router {
    // Health: no auth, no body limit concern.
    let v1_health_router = Router::new().route("/v1/health", get(health_handler));

    // Aggregate: requires auth before decompression (lazy decompression to prevent unauth gzip CPU/bomb exhaustion).
    let v1_aggregate_router = Router::new()
        .route("/v1/aggregate", post(aggregate_handler))
        .with_state(exporter)
        .layer(DefaultBodyLimit::max(MAX_REQUEST_BODY_BYTES))
        .layer(RequestDecompressionLayer::new().gzip(true))
        .layer(middleware::from_fn(require_ingest_token))
        .layer(Extension(token));

    Router::new()
        .merge(v1_health_router)
        .merge(v1_aggregate_router)
}

/// Build the OTLP HTTP router (logs + traces).
///
/// All OTLP endpoints require `Authorization: Bearer <token>` (D-02).
pub fn otlp_router(exporter: Arc<dyn OTelExporter>, token: Arc<IngestToken>) -> Router {
    let state = OTLPState { exporter };
    otlp_routes(state)
        .layer(DefaultBodyLimit::max(MAX_REQUEST_BODY_BYTES))
        .layer(RequestDecompressionLayer::new().gzip(true))
        .layer(middleware::from_fn(require_ingest_token))
        .layer(Extension(token))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::{EnrichedLogEntry, OTelLog, OTelTrace};
    use crate::error::AggregatorError;
    use axum::http::StatusCode;
    use axum::http::header::CONTENT_ENCODING;
    use axum_test::TestServer;
    use flate2::Compression;
    use flate2::write::GzEncoder;
    use std::future::Future;
    use std::io::Write;
    use std::pin::Pin;
    use std::sync::Mutex;

    struct MockLogExporter {
        exported: Mutex<Vec<EnrichedLogEntry>>,
    }

    impl MockLogExporter {
        fn new() -> Self {
            Self {
                exported: Mutex::new(Vec::new()),
            }
        }

        fn exported_count(&self) -> usize {
            self.exported.lock().unwrap().len()
        }
    }

    impl LogExporter for MockLogExporter {
        fn export_batch(
            &self,
            logs: Vec<EnrichedLogEntry>,
        ) -> Pin<Box<dyn Future<Output = Result<(), AggregatorError>> + Send + '_>> {
            Box::pin(async move {
                self.exported.lock().unwrap().extend(logs);
                Ok(())
            })
        }
    }

    /// Recording OTel exporter that stores received logs and traces and counts them.
    struct RecordingOTelExporter {
        exported_logs: Mutex<Vec<OTelLog>>,
        exported_traces: Mutex<Vec<OTelTrace>>,
    }

    impl RecordingOTelExporter {
        fn new() -> Self {
            Self {
                exported_logs: Mutex::new(Vec::new()),
                exported_traces: Mutex::new(Vec::new()),
            }
        }

        fn logs_count(&self) -> usize {
            self.exported_logs.lock().unwrap().len()
        }

        fn traces_count(&self) -> usize {
            self.exported_traces.lock().unwrap().len()
        }
    }

    impl OTelExporter for RecordingOTelExporter {
        fn export_otel_logs(
            &self,
            logs: Vec<OTelLog>,
        ) -> Pin<Box<dyn Future<Output = Result<(), AggregatorError>> + Send + '_>> {
            Box::pin(async move {
                self.exported_logs.lock().unwrap().extend(logs);
                Ok(())
            })
        }

        fn export_otel_traces(
            &self,
            traces: Vec<OTelTrace>,
        ) -> Pin<Box<dyn Future<Output = Result<(), AggregatorError>> + Send + '_>> {
            Box::pin(async move {
                self.exported_traces.lock().unwrap().extend(traces);
                Ok(())
            })
        }
    }

    fn gzip(data: &[u8]) -> Vec<u8> {
        let mut encoder = GzEncoder::new(Vec::new(), Compression::default());
        encoder.write_all(data).unwrap();
        encoder.finish().unwrap()
    }

    /// Generate a random test token at runtime (avoids fixed credential
    /// literals that trigger secret-scan).
    fn random_test_token() -> String {
        use std::time::{SystemTime, UNIX_EPOCH};
        let nanos = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .subsec_nanos();
        format!("test-tok-{nanos:x}-{:x}", std::process::id())
    }

    fn test_token_arc(token: &str) -> Arc<IngestToken> {
        Arc::new(IngestToken(token.to_string()))
    }

    // =========================================================================
    // D-02 auth tests — RED first: requests without / with wrong token rejected
    // =========================================================================

    #[tokio::test]
    async fn aggregate_rejects_request_without_authorization_header() {
        let token = random_test_token();
        let exporter = Arc::new(MockLogExporter::new());
        let app = main_router(exporter, test_token_arc(&token));
        let server = TestServer::new(app);

        let response = server.post("/v1/aggregate").text("{}").await;
        response.assert_status(StatusCode::UNAUTHORIZED);
    }

    #[tokio::test]
    async fn aggregate_rejects_wrong_bearer_token() {
        let token = random_test_token();
        let exporter = Arc::new(MockLogExporter::new());
        let app = main_router(exporter, test_token_arc(&token));
        let server = TestServer::new(app);

        let response = server
            .post("/v1/aggregate")
            .add_header("authorization", format!("Bearer wrong-{token}"))
            .text("{}")
            .await;
        response.assert_status(StatusCode::UNAUTHORIZED);
    }

    #[tokio::test]
    async fn aggregate_rejects_basic_auth_scheme() {
        let token = random_test_token();
        let exporter = Arc::new(MockLogExporter::new());
        let app = main_router(exporter, test_token_arc(&token));
        let server = TestServer::new(app);

        let response = server
            .post("/v1/aggregate")
            .add_header("authorization", "Basic dXNlcjpwYXNz")
            .text("{}")
            .await;
        response.assert_status(StatusCode::UNAUTHORIZED);
    }

    #[tokio::test]
    async fn aggregate_accepts_correct_bearer_token() {
        let token = random_test_token();
        let exporter = Arc::new(MockLogExporter::new());
        let app = main_router(exporter.clone(), test_token_arc(&token));
        let server = TestServer::new(app);

        let log_json = serde_json::json!({
            "service_type": "test", "log_type": "app", "message": "auth test",
            "timestamp": "2025-01-10T12:00:00Z", "stream": "stdout",
            "container_id": "abc123", "service_name": "test-svc", "fields": {}
        });

        let response = server
            .post("/v1/aggregate")
            .add_header("authorization", format!("Bearer {token}"))
            .text(log_json.to_string())
            .await;
        response.assert_status(StatusCode::OK);
        assert_eq!(exporter.exported_count(), 1);
    }

    #[tokio::test]
    async fn health_endpoint_is_exempt_from_auth() {
        let token = random_test_token();
        let exporter = Arc::new(MockLogExporter::new());
        let app = main_router(exporter, test_token_arc(&token));
        let server = TestServer::new(app);

        // No Authorization header — health must still return 200.
        let response = server.get("/v1/health").await;
        response.assert_status(StatusCode::OK);
    }

    fn create_nonempty_logs_request()
    -> opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest {
        use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
        use opentelemetry_proto::tonic::common::v1::{AnyValue, any_value::Value};
        use opentelemetry_proto::tonic::logs::v1::{LogRecord, ResourceLogs, ScopeLogs};

        ExportLogsServiceRequest {
            resource_logs: vec![ResourceLogs {
                resource: None,
                scope_logs: vec![ScopeLogs {
                    scope: None,
                    log_records: vec![LogRecord {
                        time_unix_nano: 1_700_000_000_000_000_000,
                        observed_time_unix_nano: 1_700_000_000_000_000_000,
                        severity_number: 9,
                        severity_text: "INFO".to_string(),
                        body: Some(AnyValue {
                            value: Some(Value::StringValue("test log message".to_string())),
                        }),
                        attributes: vec![],
                        dropped_attributes_count: 0,
                        flags: 0,
                        trace_id: vec![1; 16],
                        span_id: vec![2; 8],
                        event_name: "".to_string(),
                    }],
                    schema_url: "".to_string(),
                }],
                schema_url: "".to_string(),
            }],
        }
    }

    fn create_nonempty_trace_request()
    -> opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest {
        use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
        use opentelemetry_proto::tonic::trace::v1::{ResourceSpans, ScopeSpans, Span};

        ExportTraceServiceRequest {
            resource_spans: vec![ResourceSpans {
                resource: None,
                scope_spans: vec![ScopeSpans {
                    scope: None,
                    spans: vec![Span {
                        trace_id: vec![1; 16],
                        span_id: vec![2; 8],
                        trace_state: "".to_string(),
                        parent_span_id: vec![],
                        name: "test-span".to_string(),
                        kind: 1,
                        start_time_unix_nano: 1_700_000_000_000_000_000,
                        end_time_unix_nano: 1_700_000_001_000_000_000,
                        attributes: vec![],
                        dropped_attributes_count: 0,
                        events: vec![],
                        dropped_events_count: 0,
                        links: vec![],
                        dropped_links_count: 0,
                        status: None,
                        flags: 0,
                    }],
                    schema_url: "".to_string(),
                }],
                schema_url: "".to_string(),
            }],
        }
    }

    #[tokio::test]
    async fn otlp_logs_rejects_request_without_auth() {
        use prost::Message;

        let token = random_test_token();
        let exporter: Arc<dyn OTelExporter> = Arc::new(RecordingOTelExporter::new());
        let app = otlp_router(exporter, test_token_arc(&token));
        let server = TestServer::new(app);

        let request = create_nonempty_logs_request();
        let response = server
            .post("/v1/logs")
            .content_type("application/x-protobuf")
            .bytes(request.encode_to_vec().into())
            .await;
        response.assert_status(StatusCode::UNAUTHORIZED);
    }

    #[tokio::test]
    async fn otlp_logs_rejects_bad_bearer() {
        use prost::Message;

        let token = random_test_token();
        let exporter: Arc<dyn OTelExporter> = Arc::new(RecordingOTelExporter::new());
        let app = otlp_router(exporter, test_token_arc(&token));
        let server = TestServer::new(app);

        let request = create_nonempty_logs_request();
        let response = server
            .post("/v1/logs")
            .content_type("application/x-protobuf")
            .add_header("authorization", format!("Bearer wrong-{token}"))
            .bytes(request.encode_to_vec().into())
            .await;
        response.assert_status(StatusCode::UNAUTHORIZED);
    }

    #[tokio::test]
    async fn otlp_traces_rejects_request_without_auth() {
        use prost::Message;

        let token = random_test_token();
        let exporter: Arc<dyn OTelExporter> = Arc::new(RecordingOTelExporter::new());
        let app = otlp_router(exporter, test_token_arc(&token));
        let server = TestServer::new(app);

        let request = create_nonempty_trace_request();
        let response = server
            .post("/v1/traces")
            .content_type("application/x-protobuf")
            .bytes(request.encode_to_vec().into())
            .await;
        response.assert_status(StatusCode::UNAUTHORIZED);
    }

    #[tokio::test]
    async fn otlp_traces_rejects_bad_bearer() {
        use prost::Message;

        let token = random_test_token();
        let exporter: Arc<dyn OTelExporter> = Arc::new(RecordingOTelExporter::new());
        let app = otlp_router(exporter, test_token_arc(&token));
        let server = TestServer::new(app);

        let request = create_nonempty_trace_request();
        let response = server
            .post("/v1/traces")
            .content_type("application/x-protobuf")
            .add_header("authorization", format!("Bearer wrong-{token}"))
            .bytes(request.encode_to_vec().into())
            .await;
        response.assert_status(StatusCode::UNAUTHORIZED);
    }

    #[tokio::test]
    async fn otlp_logs_accepts_valid_nonempty_protobuf_reaches_recording_storage() {
        use prost::Message;

        let token = random_test_token();
        let exporter = Arc::new(RecordingOTelExporter::new());
        let app = otlp_router(exporter.clone(), test_token_arc(&token));
        let server = TestServer::new(app);

        let request = create_nonempty_logs_request();
        let response = server
            .post("/v1/logs")
            .content_type("application/x-protobuf")
            .add_header("authorization", format!("Bearer {token}"))
            .bytes(request.encode_to_vec().into())
            .await;
        response.assert_status(StatusCode::OK);
        assert_eq!(
            exporter.logs_count(),
            1,
            "nonempty valid log records must reach recording storage"
        );
    }

    #[tokio::test]
    async fn otlp_traces_accepts_valid_nonempty_protobuf_reaches_recording_storage() {
        use prost::Message;

        let token = random_test_token();
        let exporter = Arc::new(RecordingOTelExporter::new());
        let app = otlp_router(exporter.clone(), test_token_arc(&token));
        let server = TestServer::new(app);

        let request = create_nonempty_trace_request();
        let response = server
            .post("/v1/traces")
            .content_type("application/x-protobuf")
            .add_header("authorization", format!("Bearer {token}"))
            .bytes(request.encode_to_vec().into())
            .await;
        response.assert_status(StatusCode::OK);
        assert_eq!(
            exporter.traces_count(),
            1,
            "nonempty valid trace spans must reach recording storage"
        );
    }

    #[tokio::test]
    async fn main_router_lazy_decompression_unauthenticated_request_rejected_without_decompressing()
    {
        let token = random_test_token();
        let exporter = Arc::new(MockLogExporter::new());
        let app = main_router(exporter, test_token_arc(&token));
        let server = TestServer::new(app);

        // Corrupt/invalid gzip bytes sent without Authorization header.
        // If decompression were eager, it would fail decompression.
        // With lazy decompression after auth, it immediately returns 401 UNAUTHORIZED without decompressing.
        let corrupt_gzip = vec![0x1f, 0x8b, 0xff, 0xff, 0x00, 0x01];
        let response = server
            .post("/v1/aggregate")
            .add_header(CONTENT_ENCODING, "gzip")
            .bytes(corrupt_gzip.into())
            .await;

        response.assert_status(StatusCode::UNAUTHORIZED);
    }

    #[tokio::test]
    async fn otlp_router_lazy_decompression_unauthenticated_request_rejected_without_decompressing()
    {
        let token = random_test_token();
        let exporter: Arc<dyn OTelExporter> = Arc::new(RecordingOTelExporter::new());
        let app = otlp_router(exporter, test_token_arc(&token));
        let server = TestServer::new(app);

        let corrupt_gzip = vec![0x1f, 0x8b, 0xff, 0xff, 0x00, 0x01];
        let response = server
            .post("/v1/logs")
            .content_type("application/x-protobuf")
            .add_header(CONTENT_ENCODING, "gzip")
            .bytes(corrupt_gzip.into())
            .await;

        response.assert_status(StatusCode::UNAUTHORIZED);
    }

    #[tokio::test]
    async fn main_router_rejects_decompressed_body_exceeding_20mib() {
        let token = random_test_token();
        let exporter = Arc::new(MockLogExporter::new());
        let app = main_router(exporter, test_token_arc(&token));
        let server = TestServer::new(app);

        // 21 MiB decompresses to > 20 MiB limit
        let large_data = vec![0u8; 21 * 1024 * 1024];
        let compressed = gzip(&large_data);

        let response = server
            .post("/v1/aggregate")
            .add_header(CONTENT_ENCODING, "gzip")
            .add_header("authorization", format!("Bearer {token}"))
            .bytes(compressed.into())
            .await;

        response.assert_status(StatusCode::PAYLOAD_TOO_LARGE);
    }

    #[tokio::test]
    async fn otlp_router_rejects_decompressed_body_exceeding_20mib() {
        let token = random_test_token();
        let exporter: Arc<dyn OTelExporter> = Arc::new(RecordingOTelExporter::new());
        let app = otlp_router(exporter, test_token_arc(&token));
        let server = TestServer::new(app);

        let large_data = vec![0u8; 21 * 1024 * 1024];
        let compressed = gzip(&large_data);

        let response = server
            .post("/v1/logs")
            .content_type("application/x-protobuf")
            .add_header(CONTENT_ENCODING, "gzip")
            .add_header("authorization", format!("Bearer {token}"))
            .bytes(compressed.into())
            .await;

        response.assert_status(StatusCode::PAYLOAD_TOO_LARGE);
    }

    // =========================================================================
    // Pre-existing functional tests — updated to pass auth
    // =========================================================================

    #[tokio::test]
    async fn main_router_accepts_gzip_compressed_ndjson_body() {
        let token = random_test_token();
        let exporter = Arc::new(MockLogExporter::new());
        let app = main_router(exporter.clone(), test_token_arc(&token));
        let server = TestServer::new(app);

        let line = serde_json::json!({
            "service_type": "http", "log_type": "access", "message": "gzip body test",
            "timestamp": "2025-01-10T12:00:00Z", "stream": "stdout",
            "container_id": "abc123", "service_name": "test-svc", "fields": {}
        })
        .to_string();
        let compressed = gzip(line.as_bytes());

        let response = server
            .post("/v1/aggregate")
            .add_header(CONTENT_ENCODING, "gzip")
            .add_header("authorization", format!("Bearer {token}"))
            .bytes(compressed.into())
            .await;

        response.assert_status(StatusCode::OK);
        assert_eq!(exporter.exported_count(), 1);
    }

    #[tokio::test]
    async fn otlp_router_accepts_gzip_compressed_protobuf_body() {
        use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
        use prost::Message;

        let token = random_test_token();
        let exporter: Arc<dyn OTelExporter> = Arc::new(RecordingOTelExporter::new());
        let app = otlp_router(exporter, test_token_arc(&token));
        let server = TestServer::new(app);

        let request = ExportLogsServiceRequest::default();
        let compressed = gzip(&request.encode_to_vec());

        let response = server
            .post("/v1/logs")
            .content_type("application/x-protobuf")
            .add_header(CONTENT_ENCODING, "gzip")
            .add_header("authorization", format!("Bearer {token}"))
            .bytes(compressed.into())
            .await;

        response.assert_status(StatusCode::OK);
    }

    #[tokio::test]
    async fn main_router_accepts_body_larger_than_axum_default_limit() {
        let token = random_test_token();
        let exporter = Arc::new(MockLogExporter::new());
        let app = main_router(exporter, test_token_arc(&token));
        let server = TestServer::new(app);

        let oversized_body = "x".repeat(3 * 1024 * 1024);

        let response = server
            .post("/v1/aggregate")
            .add_header("authorization", format!("Bearer {token}"))
            .text(oversized_body)
            .await;

        response.assert_status(StatusCode::OK);
    }
}
