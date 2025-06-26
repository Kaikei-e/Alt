use rask_log_forwarder::buffer::{LogBuffer, BufferConfig};
use rask_log_forwarder::parser::NginxLogEntry;
use chrono::Utc;
use std::sync::Arc;
use std::time::Duration;

#[tokio::test]
async fn test_log_buffer_creation() {
    let config = BufferConfig {
        capacity: 1000000,
        batch_size: 1000,
        batch_timeout: Duration::from_millis(100),
        enable_backpressure: true,
        backpressure_threshold: 0.8,
        backpressure_delay: Duration::from_micros(100),
    };
    let buffer = LogBuffer::new_with_config(config).await.unwrap();
    let metrics = buffer.metrics().snapshot();
    assert_eq!(metrics.queue_depth, 0);
}

#[tokio::test]
async fn test_buffer_with_zero_capacity() {
    let config = BufferConfig {
        capacity: 0,
        batch_size: 1000,
        batch_timeout: Duration::from_millis(100),
        enable_backpressure: true,
        backpressure_threshold: 0.8,
        backpressure_delay: Duration::from_micros(100),
    };
    let result = LogBuffer::new_with_config(config).await;
    // Zero capacity should still work as it's handled by the underlying queue
    assert!(result.is_ok());
}

#[tokio::test]
async fn test_buffer_with_max_capacity() {
    let config = BufferConfig {
        capacity: 1_000_000,
        batch_size: 1000,
        batch_timeout: Duration::from_millis(100),
        enable_backpressure: true,
        backpressure_threshold: 0.8,
        backpressure_delay: Duration::from_micros(100),
    };
    let result = LogBuffer::new_with_config(config).await;
    // This should succeed as the limit is reasonable
    assert!(result.is_ok());
}

#[tokio::test]
async fn test_buffer_metrics_initialization() {
    let config = BufferConfig {
        capacity: 1000,
        batch_size: 100,
        batch_timeout: Duration::from_millis(100),
        enable_backpressure: true,
        backpressure_threshold: 0.8,
        backpressure_delay: Duration::from_micros(100),
    };
    let buffer = LogBuffer::new_with_config(config).await.unwrap();
    let metrics = buffer.metrics().snapshot();

    assert_eq!(metrics.queue_depth, 0);
    assert_eq!(metrics.messages_sent, 0);
    assert_eq!(metrics.messages_received, 0);
    assert_eq!(metrics.messages_dropped, 0);
}

#[allow(dead_code)]
fn create_test_nginx_log(id: usize) -> Arc<NginxLogEntry> {
    Arc::new(NginxLogEntry {
        service_type: "nginx".to_string(),
        log_type: "access".to_string(),
        message: format!("Test log message {}", id),
        stream: "stdout".to_string(),
        timestamp: Utc::now(),
        container_id: Some(format!("container_{}", id % 10)),
        ip_address: Some("192.168.1.1".to_string()),
        method: Some("GET".to_string()),
        path: Some("/api/test".to_string()),
        status_code: Some(200),
        response_size: Some(1024),
        user_agent: Some("test-agent".to_string()),
        level: None,
    })
}