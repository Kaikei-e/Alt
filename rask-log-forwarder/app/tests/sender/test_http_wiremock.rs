use rask_log_forwarder::sender::{ClientConfig, ClientError, HttpClient};
use std::time::Duration;
use wiremock::{
    Mock, MockServer, ResponseTemplate,
    matchers::{header, method, path},
};

#[tokio::test]
async fn test_http_client_with_wiremock_success() {
    // Start a mock server
    let mock_server = MockServer::start().await;

    // Setup mock endpoint
    Mock::given(method("GET"))
        .and(path("/v1/health"))
        .respond_with(ResponseTemplate::new(200).set_body_string("OK"))
        .mount(&mock_server)
        .await;

    // Create HTTP client with mock server URL
    let config = ClientConfig {
        endpoint: mock_server.uri(),
        timeout: Duration::from_secs(5),
        connection_timeout: Duration::from_secs(2),
        max_connections: 10,
        user_agent: "test-client/1.0".to_string(),
        ..Default::default()
    };

    let client = HttpClient::new(config).await.unwrap();

    // Test health check
    let result = client.health_check().await;
    assert!(result.is_ok());

    // Verify stats
    let stats = client.connection_stats();
    assert_eq!(stats.total_requests, 1);
    assert_eq!(stats.successful_requests, 1);
    assert_eq!(stats.failed_requests, 0);
}

#[tokio::test]
async fn test_http_client_with_wiremock_server_error() {
    let mock_server = MockServer::start().await;

    // Setup mock to return 500 error
    Mock::given(method("GET"))
        .and(path("/v1/health"))
        .respond_with(ResponseTemplate::new(500).set_body_string("Internal Server Error"))
        .mount(&mock_server)
        .await;

    let config = ClientConfig {
        endpoint: mock_server.uri(),
        timeout: Duration::from_secs(5),
        ..Default::default()
    };

    let client = HttpClient::new(config).await.unwrap();

    // Test health check should fail
    let result = client.health_check().await;
    assert!(result.is_err());

    match result.unwrap_err() {
        ClientError::HttpError { status, .. } => {
            assert_eq!(status, 500);
        }
        _ => panic!("Expected HttpError"),
    }
}

#[tokio::test]
async fn test_http_client_with_wiremock_timeout() {
    let mock_server = MockServer::start().await;

    // Setup mock with delay longer than client timeout
    Mock::given(method("GET"))
        .and(path("/v1/health"))
        .respond_with(
            ResponseTemplate::new(200).set_delay(Duration::from_secs(10)), // Delay longer than client timeout
        )
        .mount(&mock_server)
        .await;

    let config = ClientConfig {
        endpoint: mock_server.uri(),
        timeout: Duration::from_millis(100), // Very short timeout
        ..Default::default()
    };

    let client = HttpClient::new(config).await.unwrap();

    // Test should timeout
    let result = client.health_check().await;
    assert!(result.is_err());

    let error = result.unwrap_err();

    match error {
        ClientError::RequestTimeout(_) => {
            // Expected timeout
        }
        ClientError::ConnectionFailed(msg)
            if msg.contains("timeout") || msg.contains("Timeout") =>
        {
            // Also acceptable - different timeout error type
        }
        ClientError::NetworkError(ref err) if err.is_timeout() => {
            // reqwest timeout error
        }
        _ => panic!("Expected timeout-related error, got: {error:?}"),
    }
}

#[tokio::test]
async fn test_http_client_with_wiremock_multiple_endpoints() {
    let mock_server = MockServer::start().await;

    // Setup multiple mock endpoints
    Mock::given(method("GET"))
        .and(path("/v1/health"))
        .respond_with(ResponseTemplate::new(200).set_body_string("Healthy"))
        .mount(&mock_server)
        .await;

    Mock::given(method("POST"))
        .and(path("/v1/aggregate"))
        .and(header("content-type", "application/x-ndjson"))
        .respond_with(ResponseTemplate::new(202).set_body_string("Accepted"))
        .mount(&mock_server)
        .await;

    let config = ClientConfig {
        endpoint: mock_server.uri(),
        ..Default::default()
    };

    let client = HttpClient::new(config).await.unwrap();

    // Test health check
    let health_result = client.health_check().await;
    assert!(health_result.is_ok());

    // Test multiple health checks for connection reuse
    for _ in 0..3 {
        let result = client.health_check().await;
        assert!(result.is_ok());
    }

    let stats = client.connection_stats();
    assert_eq!(stats.total_requests, 4); // 1 from client creation + 3 from loop
}

#[tokio::test]
async fn test_http_client_with_wiremock_user_agent() {
    let mock_server = MockServer::start().await;

    let custom_user_agent = "rask-log-forwarder/test-1.0";

    // Setup mock that verifies user agent
    Mock::given(method("GET"))
        .and(path("/v1/health"))
        .and(header("user-agent", custom_user_agent))
        .respond_with(ResponseTemplate::new(200))
        .mount(&mock_server)
        .await;

    let config = ClientConfig {
        endpoint: mock_server.uri(),
        user_agent: custom_user_agent.to_string(),
        ..Default::default()
    };

    let client = HttpClient::new(config).await.unwrap();

    // This should succeed if user agent is correctly set
    let result = client.health_check().await;
    assert!(result.is_ok());
}

#[tokio::test]
async fn test_http_client_with_wiremock_auth_header_for_ndjson_and_otlp() {
    use chrono::Utc;
    use rask_log_forwarder::buffer::{Batch, BatchType};
    use rask_log_forwarder::domain::{EnrichedLogEntry, LogLevel};
    use rask_log_forwarder::sender::transmission::BatchTransmitter;
    #[cfg(feature = "otlp")]
    use rask_log_forwarder::sender::transmission::OtlpBatchTransmitter;
    use std::collections::HashMap;

    let mock_server = MockServer::start().await;

    // Generate random runtime credential
    use std::time::{SystemTime, UNIX_EPOCH};
    let nanos = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .subsec_nanos();
    let token = format!("test-tok-{nanos:x}-{:x}", std::process::id());

    // Setup mock that STRICTLY verifies the Authorization header
    Mock::given(method("POST"))
        .and(path("/v1/aggregate"))
        .and(header("authorization", format!("Bearer {token}").as_str()))
        .respond_with(ResponseTemplate::new(200))
        .mount(&mock_server)
        .await;

    Mock::given(method("POST"))
        .and(path("/v1/logs"))
        .and(header("authorization", format!("Bearer {token}").as_str()))
        .respond_with(ResponseTemplate::new(200))
        .mount(&mock_server)
        .await;

    let config = ClientConfig {
        endpoint: mock_server.uri(),
        ingest_token: token.clone(),
        ..Default::default()
    };

    let client = HttpClient::new(config).await.unwrap();

    // Send NDJSON Batch
    let transmitter = BatchTransmitter::new(client.clone());

    let entry = EnrichedLogEntry {
        service_type: "test".into(),
        log_type: "test".into(),
        message: "hello".into(),
        level: Some(LogLevel::Info),
        timestamp: Utc::now().to_rfc3339(),
        stream: "stdout".into(),
        method: None,
        path: None,
        status_code: None,
        response_size: None,
        ip_address: None,
        user_agent: None,
        container_id: "none".into(),
        service_name: "test".into(),
        service_group: None,
        trace_id: None,
        span_id: None,
        fields: HashMap::new(),
    };
    let batch = Batch::new(vec![entry], BatchType::SizeBased);

    // Test the transmission using the mock server — if it lacks the correct Authorization
    // header, the wiremock server returns 404 (because no Mock matches), returning an error.
    let res = transmitter.send_batch(batch.clone()).await;
    assert!(
        res.is_ok(),
        "NDJSON Batch transmission failed: {:?}",
        res.unwrap_err()
    );
    assert_eq!(res.unwrap().status_code, 200);

    // Send OTLP Batch
    #[cfg(feature = "otlp")]
    {
        let otlp_client = client.clone();
        let otlp_url = format!("{}/v1/logs", mock_server.uri());
        let otlp_transmitter = OtlpBatchTransmitter::new(otlp_client, &otlp_url).unwrap();

        let otlp_res: Result<rask_log_forwarder::sender::transmission::TransmissionResult, _> =
            otlp_transmitter.send_batch(batch).await;
        assert!(
            otlp_res.is_ok(),
            "OTLP Batch transmission failed: {:?}",
            otlp_res.unwrap_err()
        );
        assert_eq!(otlp_res.unwrap().status_code, 200);
    }
}
