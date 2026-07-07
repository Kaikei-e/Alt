use crate::handler::aggregate::aggregate_handler;
use crate::handler::health::health_handler;
use crate::otlp::otlp_routes;
use crate::otlp::receiver::OTLPState;
use crate::port::{LogExporter, OTelExporter};
use axum::Router;
use axum::extract::DefaultBodyLimit;
use axum::routing::{get, post};
use std::sync::Arc;
use tower_http::decompression::RequestDecompressionLayer;

/// The forwarder's default `batch_size` is 10,000 entries at an estimated
/// ~500 bytes/entry (see rask-log-forwarder `ESTIMATED_ENTRY_SIZE`), i.e.
/// ~5 MB uncompressed per batch. Set the limit well above that so legitimate
/// batches are never rejected with 413, regardless of compression.
const MAX_REQUEST_BODY_BYTES: usize = 20 * 1024 * 1024; // 20 MB

/// Build the main HTTP router (health + legacy aggregate).
pub fn main_router(exporter: Arc<dyn LogExporter>) -> Router {
    let v1_health_router = Router::new().route("/v1/health", get(health_handler));

    let v1_aggregate_router = Router::new()
        .route("/v1/aggregate", post(aggregate_handler))
        .with_state(exporter);

    Router::new()
        .merge(v1_health_router)
        .merge(v1_aggregate_router)
        .layer(RequestDecompressionLayer::new().gzip(true))
        .layer(DefaultBodyLimit::max(MAX_REQUEST_BODY_BYTES))
}

/// Build the OTLP HTTP router (logs + traces).
pub fn otlp_router(exporter: Arc<dyn OTelExporter>) -> Router {
    let state = OTLPState { exporter };
    otlp_routes(state)
        .layer(RequestDecompressionLayer::new().gzip(true))
        .layer(DefaultBodyLimit::max(MAX_REQUEST_BODY_BYTES))
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

    /// OTel exporter stub — the gzip/body-limit tests only assert on the
    /// HTTP response, not on what reaches ClickHouse.
    struct NoopOTelExporter;

    impl OTelExporter for NoopOTelExporter {
        fn export_otel_logs(
            &self,
            _logs: Vec<OTelLog>,
        ) -> Pin<Box<dyn Future<Output = Result<(), AggregatorError>> + Send + '_>> {
            Box::pin(async { Ok(()) })
        }

        fn export_otel_traces(
            &self,
            _traces: Vec<OTelTrace>,
        ) -> Pin<Box<dyn Future<Output = Result<(), AggregatorError>> + Send + '_>> {
            Box::pin(async { Ok(()) })
        }
    }

    fn gzip(data: &[u8]) -> Vec<u8> {
        let mut encoder = GzEncoder::new(Vec::new(), Compression::default());
        encoder.write_all(data).unwrap();
        encoder.finish().unwrap()
    }

    /// rask-log-forwarder sends `Content-Encoding: gzip` NDJSON for
    /// `/v1/aggregate` once `enable_compression` is on. Without a
    /// `RequestDecompressionLayer`, the `String` extractor rejects the
    /// (non-UTF8) gzip bytes with 400 — this must instead decompress
    /// transparently and export the log.
    #[tokio::test]
    async fn main_router_accepts_gzip_compressed_ndjson_body() {
        let exporter = Arc::new(MockLogExporter::new());
        let app = main_router(exporter.clone());
        let server = TestServer::new(app).unwrap();

        let line = serde_json::json!({
            "service_type": "http",
            "log_type": "access",
            "message": "gzip body test",
            "timestamp": "2025-01-10T12:00:00Z",
            "stream": "stdout",
            "container_id": "abc123",
            "service_name": "test-svc",
            "fields": {}
        })
        .to_string();
        let compressed = gzip(line.as_bytes());

        let response = server
            .post("/v1/aggregate")
            .add_header(CONTENT_ENCODING, "gzip")
            .bytes(compressed.into())
            .await;

        response.assert_status(StatusCode::OK);
        assert_eq!(exporter.exported_count(), 1);
    }

    /// Same compatibility bug on the OTLP side: forwarder gzips OTLP
    /// payloads > 1KB, and protobuf decode fails on the compressed bytes
    /// without decompression middleware.
    #[tokio::test]
    async fn otlp_router_accepts_gzip_compressed_protobuf_body() {
        use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
        use prost::Message;

        let exporter: Arc<dyn OTelExporter> = Arc::new(NoopOTelExporter);
        let app = otlp_router(exporter);
        let server = TestServer::new(app).unwrap();

        let request = ExportLogsServiceRequest::default();
        let compressed = gzip(&request.encode_to_vec());

        let response = server
            .post("/v1/logs")
            .content_type("application/x-protobuf")
            .add_header(CONTENT_ENCODING, "gzip")
            .bytes(compressed.into())
            .await;

        response.assert_status(StatusCode::OK);
    }

    /// axum's built-in default body limit is 2 MiB; the forwarder's default
    /// batch (`batch_size=10000` * ~500B/entry ≈ 5 MB) must not be rejected
    /// with 413.
    #[tokio::test]
    async fn main_router_accepts_body_larger_than_axum_default_limit() {
        let exporter = Arc::new(MockLogExporter::new());
        let app = main_router(exporter);
        let server = TestServer::new(app).unwrap();

        let oversized_body = "x".repeat(3 * 1024 * 1024);

        let response = server.post("/v1/aggregate").text(oversized_body).await;

        response.assert_status(StatusCode::OK);
    }
}
