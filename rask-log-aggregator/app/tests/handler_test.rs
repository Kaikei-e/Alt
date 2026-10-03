use axum_test::TestServer;
use rask::domain::EnrichedLogEntry;
use rask::error::AggregatorError;
use rask::log_exporter::LogExporter;
use std::future::Future;
use std::pin::Pin;
use std::sync::{Arc, Mutex};

/// Mock exporter that captures exported logs for testing
struct MockExporter {
    exported_logs: Arc<Mutex<Vec<EnrichedLogEntry>>>,
}

impl MockExporter {
    fn new() -> Self {
        Self {
            exported_logs: Arc::new(Mutex::new(Vec::new())),
        }
    }

    fn get_exported_logs(&self) -> Vec<EnrichedLogEntry> {
        self.exported_logs.lock().unwrap().clone()
    }
}

impl LogExporter for MockExporter {
    fn export_batch(
        &self,
        logs: Vec<EnrichedLogEntry>,
    ) -> Pin<Box<dyn Future<Output = Result<(), AggregatorError>> + Send + '_>> {
        let exported_logs = self.exported_logs.clone();
        Box::pin(async move {
            let mut guard = exported_logs.lock().unwrap();
            guard.extend(logs);
            Ok(())
        })
    }
}

fn random_test_token() -> String {
    use std::time::{SystemTime, UNIX_EPOCH};
    let nanos = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .subsec_nanos();
    format!("test-tok-{nanos:x}-{:x}", std::process::id())
}

fn test_token_arc(token: &str) -> Arc<rask::auth::IngestToken> {
    Arc::new(rask::auth::IngestToken(token.to_string()))
}

#[tokio::test]
async fn test_health_endpoint_returns_healthy_no_auth_needed() {
    let token = random_test_token();
    let exporter: Arc<dyn LogExporter> = Arc::new(MockExporter::new());
    let app = rask::app::router::main_router(exporter, test_token_arc(&token));
    let server = TestServer::new(app);

    let response = server.get("/v1/health").await;

    response.assert_status_ok();
    response.assert_text("Healthy");
}

#[tokio::test]
async fn test_aggregate_rejects_missing_auth() {
    let token = random_test_token();
    let mock_exporter = Arc::new(MockExporter::new());
    let exporter: Arc<dyn LogExporter> = mock_exporter.clone();
    let app = rask::app::router::main_router(exporter, test_token_arc(&token));
    let server = TestServer::new(app);

    let response = server.post("/v1/aggregate").text("{}").await;
    response.assert_status(axum::http::StatusCode::UNAUTHORIZED);
}

#[tokio::test]
async fn test_aggregate_rejects_wrong_bearer_token() {
    let token = random_test_token();
    let mock_exporter = Arc::new(MockExporter::new());
    let exporter: Arc<dyn LogExporter> = mock_exporter.clone();
    let app = rask::app::router::main_router(exporter, test_token_arc(&token));
    let server = TestServer::new(app);

    let response = server
        .post("/v1/aggregate")
        .add_header("authorization", format!("Bearer wrong-{token}"))
        .text("{}")
        .await;
    response.assert_status(axum::http::StatusCode::UNAUTHORIZED);
}

#[tokio::test]
async fn test_aggregate_rejects_malformed_token_scheme() {
    let token = random_test_token();
    let mock_exporter = Arc::new(MockExporter::new());
    let exporter: Arc<dyn LogExporter> = mock_exporter.clone();
    let app = rask::app::router::main_router(exporter, test_token_arc(&token));
    let server = TestServer::new(app);

    let response = server
        .post("/v1/aggregate")
        .add_header("authorization", "Basic dXNlcjpwYXNz")
        .text("{}")
        .await;
    response.assert_status(axum::http::StatusCode::UNAUTHORIZED);
}

#[tokio::test]
async fn test_aggregate_endpoint_accepts_valid_log_with_auth() {
    let token = random_test_token();
    let mock_exporter = Arc::new(MockExporter::new());
    let exporter: Arc<dyn LogExporter> = mock_exporter.clone();
    let app = rask::app::router::main_router(exporter, test_token_arc(&token));
    let server = TestServer::new(app);

    let log_json = serde_json::json!({
        "service_type": "test",
        "log_type": "app",
        "message": "test message",
        "level": "Info",
        "timestamp": "2025-01-10T12:00:00Z",
        "stream": "stdout",
        "container_id": "abc123",
        "service_name": "test-svc",
        "fields": {}
    });

    let response = server
        .post("/v1/aggregate")
        .add_header("authorization", format!("Bearer {token}"))
        .text(log_json.to_string())
        .await;

    response.assert_status_ok();

    // Verify the log was exported
    let exported = mock_exporter.get_exported_logs();
    assert_eq!(exported.len(), 1);
    assert_eq!(exported[0].service_type, "test");
    assert_eq!(exported[0].message, "test message");
}

#[tokio::test]
async fn test_aggregate_endpoint_handles_multiple_logs_with_auth() {
    let token = random_test_token();
    let mock_exporter = Arc::new(MockExporter::new());
    let exporter: Arc<dyn LogExporter> = mock_exporter.clone();
    let app = rask::app::router::main_router(exporter, test_token_arc(&token));
    let server = TestServer::new(app);

    let logs = [
        serde_json::json!({
            "service_type": "svc1",
            "log_type": "app",
            "message": "message 1",
            "timestamp": "2025-01-10T12:00:00Z",
            "stream": "stdout",
            "container_id": "abc123",
            "service_name": "test-svc",
            "fields": {}
        }),
        serde_json::json!({
            "service_type": "svc2",
            "log_type": "app",
            "message": "message 2",
            "timestamp": "2025-01-10T12:00:01Z",
            "stream": "stderr",
            "container_id": "def456",
            "service_name": "test-svc-2",
            "fields": {}
        }),
    ];

    let body = logs
        .iter()
        .map(|l| l.to_string())
        .collect::<Vec<_>>()
        .join("\n");

    let response = server
        .post("/v1/aggregate")
        .add_header("authorization", format!("Bearer {token}"))
        .text(body)
        .await;

    response.assert_status_ok();

    let exported = mock_exporter.get_exported_logs();
    assert_eq!(exported.len(), 2);
    assert_eq!(exported[0].service_type, "svc1");
    assert_eq!(exported[1].service_type, "svc2");
}

#[tokio::test]
async fn test_aggregate_endpoint_skips_invalid_json_with_auth() {
    let token = random_test_token();
    let mock_exporter = Arc::new(MockExporter::new());
    let exporter: Arc<dyn LogExporter> = mock_exporter.clone();
    let app = rask::app::router::main_router(exporter, test_token_arc(&token));
    let server = TestServer::new(app);

    let body = r#"{"service_type": "valid", "log_type": "app", "message": "valid log", "timestamp": "2025-01-10T12:00:00Z", "stream": "stdout", "container_id": "abc", "service_name": "svc", "fields": {}}
invalid json line
{"service_type": "also_valid", "log_type": "app", "message": "another valid log", "timestamp": "2025-01-10T12:00:01Z", "stream": "stdout", "container_id": "def", "service_name": "svc2", "fields": {}}"#;

    let response = server
        .post("/v1/aggregate")
        .add_header("authorization", format!("Bearer {token}"))
        .text(body)
        .await;

    response.assert_status_ok();

    let exported = mock_exporter.get_exported_logs();
    assert_eq!(exported.len(), 2); // Only valid logs exported
}

#[tokio::test]
async fn test_aggregate_endpoint_handles_empty_body_with_auth() {
    let token = random_test_token();
    let mock_exporter = Arc::new(MockExporter::new());
    let exporter: Arc<dyn LogExporter> = mock_exporter.clone();
    let app = rask::app::router::main_router(exporter, test_token_arc(&token));
    let server = TestServer::new(app);

    let response = server
        .post("/v1/aggregate")
        .add_header("authorization", format!("Bearer {token}"))
        .text("")
        .await;

    response.assert_status_ok();

    let exported = mock_exporter.get_exported_logs();
    assert_eq!(exported.len(), 0);
}
