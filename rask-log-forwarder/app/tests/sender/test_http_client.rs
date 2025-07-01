use futures::future;
use rask_log_forwarder::sender::{BatchSender, SenderConfig, SenderError};
use tokio::time::Duration;

#[tokio::test]
#[ignore = "requires running server"]
async fn test_http_client_setup() {
    let config = SenderConfig {
        endpoint: "http://localhost:9600/v1/aggregate".to_string(),
        timeout: Duration::from_secs(30),
        max_connections: 10,
        keep_alive: true,
        ..Default::default()
    };

    let sender = BatchSender::new(config).await.unwrap();
    assert!(
        sender.can_connect().await,
        "Should be able to connect to endpoint"
    );
}

#[tokio::test]
#[ignore = "requires running server"]
async fn test_connection_pooling() {
    let config = SenderConfig {
        endpoint: "http://localhost:9600/v1/aggregate".to_string(),
        max_connections: 5,
        keep_alive: true,
        ..Default::default()
    };

    let sender = BatchSender::new(config).await.unwrap();

    // Make multiple concurrent requests to test pooling
    let futures: Vec<_> = (0..10)
        .map(|_| {
            let sender = sender.clone();
            async move { sender.health_check().await }
        })
        .collect();

    let results = future::join_all(futures).await;

    // All should succeed with connection reuse
    for result in results {
        assert!(result.is_ok(), "Health check should succeed");
    }

    let stats = sender.connection_stats().await;
    assert!(
        stats.active_connections <= 5,
        "Should not exceed max connections"
    );
    assert!(stats.reused_connections > 0, "Should reuse connections");
}

#[tokio::test]
async fn test_connection_failure_handling() {
    let config = SenderConfig {
        endpoint: "http://nonexistent-host:9600/v1/aggregate".to_string(),
        timeout: Duration::from_millis(100),
        ..Default::default()
    };

    let result = BatchSender::new(config).await;
    assert!(
        result.is_err(),
        "Should fail to connect to invalid endpoint"
    );

    let error = result.unwrap_err();
    assert!(matches!(error, SenderError::ConnectionFailed(_)));
}

#[tokio::test]
#[ignore = "requires running server"]
async fn test_keep_alive_connections() {
    let config = SenderConfig {
        endpoint: "http://localhost:9600/v1/aggregate".to_string(),
        keep_alive: true,
        keep_alive_timeout: Duration::from_secs(60),
        ..Default::default()
    };

    let sender = BatchSender::new(config).await.unwrap();

    // Make two requests and verify connection reuse
    let _response1 = sender.health_check().await.unwrap();
    let _response2 = sender.health_check().await.unwrap();

    let stats = sender.connection_stats().await;
    assert_eq!(stats.total_requests, 2);
    assert!(
        stats.reused_connections >= 1,
        "Should reuse connection with keep-alive"
    );
}
