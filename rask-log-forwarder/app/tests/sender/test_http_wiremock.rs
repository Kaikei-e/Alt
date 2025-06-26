use rask_log_forwarder::sender::{HttpClient, ClientConfig, ClientError};
use wiremock::{
    matchers::{method, path, header},
    Mock, MockServer, ResponseTemplate,
};
use std::time::Duration;

#[tokio::test]
async fn test_http_client_with_wiremock_success() {
    // Start a mock server
    let mock_server = MockServer::start().await;

    // Setup mock endpoint
    Mock::given(method("GET"))
        .and(path("/health"))
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
        .and(path("/health"))
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
        .and(path("/health"))
        .respond_with(
            ResponseTemplate::new(200)
                .set_delay(Duration::from_secs(10)) // Delay longer than client timeout
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
        ClientError::ConnectionFailed(msg) if msg.contains("timeout") || msg.contains("Timeout") => {
            // Also acceptable - different timeout error type
        }
        ClientError::NetworkError(ref err) if err.is_timeout() => {
            // reqwest timeout error
        }
        _ => panic!("Expected timeout-related error, got: {:?}", error),
    }
}

#[tokio::test]
async fn test_http_client_with_wiremock_multiple_endpoints() {
    let mock_server = MockServer::start().await;

    // Setup multiple mock endpoints
    Mock::given(method("GET"))
        .and(path("/health"))
        .respond_with(ResponseTemplate::new(200).set_body_string("Healthy"))
        .mount(&mock_server)
        .await;

    Mock::given(method("POST"))
        .and(path("/ingest"))
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
        .and(path("/health"))
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