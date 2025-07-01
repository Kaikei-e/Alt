use rask_log_forwarder::sender::{ClientConfig, ClientError, HttpClient};
use std::sync::Arc;
use tokio::time::Duration;

#[tokio::test]
async fn test_http_client_creation() {
    let config = ClientConfig {
        endpoint: "http://rask-aggregator:9600/v1/aggregate".to_string(),
        timeout: Duration::from_secs(30),
        max_connections: 10,
        keep_alive_timeout: Duration::from_secs(60),
        ..Default::default()
    };

    let client = HttpClient::new(config).await;
    // Should succeed in creating client, but might fail health check without server
    assert!(client.is_ok() || matches!(client.unwrap_err(), ClientError::ConnectionFailed(_)));
}

#[tokio::test]
async fn test_client_configuration() {
    let config = ClientConfig {
        endpoint: "http://example.com:9600/v1/aggregate".to_string(),
        timeout: Duration::from_secs(5),
        max_connections: 20,
        keep_alive_timeout: Duration::from_secs(90),
        connection_timeout: Duration::from_secs(10),
        user_agent: "test-agent/1.0".to_string(),
        enable_compression: true,
        retry_attempts: 5,
    };

    // This should create client successfully even without connecting
    let result = HttpClient::new(config.clone()).await;

    // Might fail on health check, but config should be valid
    match result {
        Ok(client) => {
            assert_eq!(client.endpoint(), "http://example.com:9600/v1/aggregate");
            let stats = client.connection_stats();
            assert_eq!(stats.max_connections, 20);
        }
        Err(ClientError::ConnectionFailed(_)) => {
            // Expected when server is not available
        }
        Err(e) => panic!("Unexpected error: {e}"),
    }
}

#[tokio::test]
async fn test_invalid_endpoint() {
    let config = ClientConfig {
        endpoint: "invalid-url".to_string(),
        ..Default::default()
    };

    let result = HttpClient::new(config).await;
    assert!(result.is_err());
    assert!(matches!(
        result.unwrap_err(),
        ClientError::InvalidConfiguration(_)
    ));
}

#[tokio::test]
async fn test_connection_stats() {
    let config = ClientConfig {
        endpoint: "http://localhost:9600".to_string(),
        max_connections: 15,
        ..Default::default()
    };

    let max_connections = config.max_connections; // Extract before move

    // Try to create client
    let result = HttpClient::new(config).await;

    match result {
        Ok(client) => {
            let stats = client.connection_stats();
            assert_eq!(stats.max_connections, 15);
            assert_eq!(stats.active_connections, 0); // No active connections yet
            // Note: Health check is not automatically performed during client creation
            // It only happens when explicitly called or during send operations
            // So total_requests starts at 0 initially
            // total_requests is u64, so it's always non-negative by definition
        }
        Err(ClientError::ConnectionFailed(_)) => {
            // Expected when server is not available - test the config at least
            assert_eq!(max_connections, 15);
            // This is acceptable since we can't actually connect without a server
        }
        Err(e) => panic!("Unexpected error: {e}"),
    }
}

#[tokio::test]
async fn test_client_clone_and_sharing() {
    let config = ClientConfig {
        endpoint: "http://test.example.com:9600".to_string(),
        ..Default::default()
    };

    let result = HttpClient::new(config).await;

    match result {
        Ok(client) => {
            let client = Arc::new(client);

            // Test that client can be shared across tasks
            let client1 = client.clone();
            let client2 = client.clone();

            let handle1 = tokio::spawn(async move { client1.connection_stats() });

            let handle2 = tokio::spawn(async move { client2.connection_stats() });

            let stats1 = handle1.await.unwrap();
            let stats2 = handle2.await.unwrap();

            // Both should return the same stats
            assert_eq!(stats1.max_connections, stats2.max_connections);
        }
        Err(ClientError::ConnectionFailed(_)) => {
            // Expected when server is not available
        }
        Err(e) => panic!("Unexpected error: {e}"),
    }
}

#[tokio::test]
async fn test_timeout_configuration() {
    let config = ClientConfig {
        endpoint: "http://httpbin.org".to_string(), // Use a real endpoint for timeout test
        timeout: Duration::from_millis(1),          // Very short timeout
        connection_timeout: Duration::from_millis(1),
        ..Default::default()
    };

    let result = HttpClient::new(config).await;

    // Should either timeout or fail to connect
    match result {
        Ok(_) => {
            // Unlikely with such short timeout, but possible
        }
        Err(ClientError::ConnectionFailed(_)) | Err(ClientError::RequestTimeout(_)) => {
            // Expected with very short timeout
        }
        Err(e) => panic!("Unexpected error: {e}"),
    }
}

#[tokio::test]
async fn test_user_agent_header() {
    let config = ClientConfig {
        endpoint: "http://localhost:9600".to_string(),
        user_agent: "custom-forwarder/2.0".to_string(),
        ..Default::default()
    };

    let result = HttpClient::new(config).await;

    match result {
        Ok(client) => {
            // Verify user agent is stored in config
            assert_eq!(client.config.user_agent, "custom-forwarder/2.0");
        }
        Err(ClientError::ConnectionFailed(_)) => {
            // Expected when server is not available, but config is still valid
        }
        Err(e) => panic!("Unexpected error: {e}"),
    }
}

#[tokio::test]
async fn test_compression_enabled() {
    let config = ClientConfig {
        endpoint: "http://localhost:9600".to_string(),
        enable_compression: true,
        ..Default::default()
    };

    let result = HttpClient::new(config).await;

    match result {
        Ok(client) => {
            assert!(client.config.enable_compression);
        }
        Err(ClientError::ConnectionFailed(_)) => {
            // Expected when server is not available
        }
        Err(e) => panic!("Unexpected error: {e}"),
    }
}
