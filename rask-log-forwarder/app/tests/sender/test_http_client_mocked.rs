use rask_log_forwarder::sender::{ClientError, NewConnectionStats as ConnectionStats};
use std::time::Duration;

// 簡単なモック構造体を手動で作成
#[derive(Debug)]
pub struct MockHttpClient {
    pub should_fail: bool,
    pub error_type: Option<ClientError>,
    pub stats: ConnectionStats,
    pub endpoint: String,
}

impl MockHttpClient {
    pub fn new() -> Self {
        Self {
            should_fail: false,
            error_type: None,
            stats: ConnectionStats {
                max_connections: 10,
                active_connections: 0,
                total_requests: 0,
                successful_requests: 0,
                failed_requests: 0,
                average_response_time: Duration::ZERO,
            },
            endpoint: "http://localhost:9600/ingest".to_string(),
        }
    }

    pub fn with_error(mut self, error: ClientError) -> Self {
        self.should_fail = true;
        self.error_type = Some(error);
        self
    }

    pub fn with_stats(mut self, stats: ConnectionStats) -> Self {
        self.stats = stats;
        self
    }

    pub fn with_endpoint(mut self, endpoint: &str) -> Self {
        self.endpoint = endpoint.to_string();
        self
    }

    pub async fn health_check(&self) -> Result<(), ClientError> {
        if self.should_fail {
            if let Some(ref error) = self.error_type {
                match error {
                    ClientError::HttpError { status, message } => {
                        return Err(ClientError::HttpError {
                            status: *status,
                            message: message.clone()
                        });
                    }
                    ClientError::RequestTimeout(msg) => {
                        return Err(ClientError::RequestTimeout(msg.clone()));
                    }
                    _ => return Err(ClientError::ConnectionFailed("Mock error".to_string())),
                }
            }
        }
        Ok(())
    }

    pub fn connection_stats(&self) -> ConnectionStats {
        self.stats.clone()
    }

    pub fn endpoint(&self) -> &str {
        &self.endpoint
    }
}

#[tokio::test]
async fn test_mock_http_client_successful_health_check() {
    let mock_client = MockHttpClient::new()
        .with_stats(ConnectionStats {
            max_connections: 10,
            active_connections: 1,
            total_requests: 1,
            successful_requests: 1,
            failed_requests: 0,
            average_response_time: Duration::from_millis(50),
        })
        .with_endpoint("http://mock-server:9600/ingest");

    // Test the mock
    let result = mock_client.health_check().await;
    assert!(result.is_ok());

    let stats = mock_client.connection_stats();
    assert_eq!(stats.total_requests, 1);
    assert_eq!(stats.successful_requests, 1);
    assert_eq!(stats.failed_requests, 0);

    let endpoint = mock_client.endpoint();
    assert_eq!(endpoint, "http://mock-server:9600/ingest");
}

#[tokio::test]
async fn test_mock_http_client_failed_health_check() {
    let mock_client = MockHttpClient::new()
        .with_error(ClientError::HttpError {
            status: 503,
            message: "Service Unavailable".to_string()
        })
        .with_stats(ConnectionStats {
            max_connections: 10,
            active_connections: 0,
            total_requests: 1,
            successful_requests: 0,
            failed_requests: 1,
            average_response_time: Duration::from_millis(100),
        });

    // Test the mock
    let result = mock_client.health_check().await;
    assert!(result.is_err());

    match result.unwrap_err() {
        ClientError::HttpError { status, message } => {
            assert_eq!(status, 503);
            assert!(message.contains("Service Unavailable"));
        }
        _ => panic!("Expected HttpError"),
    }

    let stats = mock_client.connection_stats();
    assert_eq!(stats.failed_requests, 1);
}

#[tokio::test]
async fn test_mock_http_client_timeout() {
    let mock_client = MockHttpClient::new()
        .with_error(ClientError::RequestTimeout("Health check timeout".to_string()));

    // Test the mock
    let result = mock_client.health_check().await;
    assert!(result.is_err());

    match result.unwrap_err() {
        ClientError::RequestTimeout(msg) => {
            assert!(msg.contains("timeout"));
        }
        _ => panic!("Expected RequestTimeout"),
    }
}

#[tokio::test]
async fn test_mock_http_client_multiple_scenarios() {
    // Test successful scenario
    let success_client = MockHttpClient::new()
        .with_stats(ConnectionStats {
            max_connections: 20,
            active_connections: 2,
            total_requests: 3,
            successful_requests: 3,
            failed_requests: 0,
            average_response_time: Duration::from_millis(75),
        });

    let result = success_client.health_check().await;
    assert!(result.is_ok());

    let stats = success_client.connection_stats();
    assert_eq!(stats.total_requests, 3);
    assert_eq!(stats.successful_requests, 3);
    assert_eq!(stats.max_connections, 20);

    // Test different error types
    let timeout_client = MockHttpClient::new()
        .with_error(ClientError::RequestTimeout("Connection timeout".to_string()));

    let timeout_result = timeout_client.health_check().await;
    assert!(timeout_result.is_err());

    let http_error_client = MockHttpClient::new()
        .with_error(ClientError::HttpError {
            status: 500,
            message: "Internal Server Error".to_string()
        });

    let http_result = http_error_client.health_check().await;
    assert!(http_result.is_err());
}