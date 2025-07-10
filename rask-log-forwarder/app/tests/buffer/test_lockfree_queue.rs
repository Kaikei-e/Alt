use rask_log_forwarder::buffer::{BufferConfig, LogBuffer};
use rask_log_forwarder::parser::EnrichedLogEntry;
use std::sync::Arc;
use tokio::time::{Duration, timeout};

#[tokio::test]
async fn test_basic_queue_operations() {
    let config = BufferConfig {
        capacity: 1000,
        batch_size: 100,
        batch_timeout: Duration::from_millis(500),
        ..Default::default()
    };

    let buffer = LogBuffer::new_with_config(config).await.unwrap();
    let (sender, mut receiver) = buffer.split().expect("Failed to split buffer");

    // Create test log entry
    let log_entry = create_test_log_entry("test message");

    // Send log entry
    sender.send(log_entry.clone()).await.unwrap();

    // Receive log entry
    let received = timeout(Duration::from_millis(100), receiver.recv())
        .await
        .unwrap()
        .unwrap();

    assert_eq!(received.message, "test message");
}

#[tokio::test]
async fn test_high_throughput_operations() {
    let config = BufferConfig {
        capacity: 100_000,
        batch_size: 1000,
        batch_timeout: Duration::from_secs(1),
        ..Default::default()
    };

    let buffer = LogBuffer::new_with_config(config).await.unwrap();
    let (sender, mut receiver) = buffer.split().expect("Failed to split buffer");

    let sender = Arc::new(sender);

    // Spawn multiple producers
    let mut handles = vec![];
    for i in 0..10 {
        let sender_clone = sender.clone();
        let handle = tokio::spawn(async move {
            for j in 0..1000 {
                let log_entry = create_test_log_entry(&format!("message-{i}-{j}"));
                sender_clone.send(log_entry).await.unwrap();
            }
        });
        handles.push(handle);
    }

    // Wait for all producers to finish
    for handle in handles {
        handle.await.unwrap();
    }

    // Verify all messages were received
    let mut received_count = 0;
    while let Ok(Ok(_)) = timeout(Duration::from_millis(10), receiver.recv()).await {
        received_count += 1;
        if received_count >= 10_000 {
            break;
        }
    }

    assert_eq!(received_count, 10_000);
}

#[tokio::test]
async fn test_buffer_capacity_limits() {
    let config = BufferConfig {
        capacity: 10, // Small capacity
        batch_size: 100,
        batch_timeout: Duration::from_secs(10), // Long timeout
        ..Default::default()
    };

    let buffer = LogBuffer::new_with_config(config).await.unwrap();
    let (sender, _receiver) = buffer.split().expect("Failed to split buffer");

    // Fill buffer to capacity
    for i in 0..10 {
        let log_entry = create_test_log_entry(&format!("message-{i}"));
        sender.send(log_entry).await.unwrap();
    }

    // Next send should fail or apply backpressure
    let log_entry = create_test_log_entry("overflow message");
    let result = timeout(Duration::from_millis(100), sender.send(log_entry)).await;

    // Should either timeout or return an error
    assert!(result.is_err() || result.unwrap().is_err());
}

#[tokio::test]
async fn test_buffer_metrics() {
    let config = BufferConfig {
        capacity: 1000,
        batch_size: 10,
        batch_timeout: Duration::from_millis(100),
        ..Default::default()
    };

    let buffer = LogBuffer::new_with_config(config).await.unwrap();
    let metrics = buffer.metrics();
    let (sender, mut receiver) = buffer.split().expect("Failed to split buffer");

    // Send some messages
    for i in 0..5 {
        let log_entry = create_test_log_entry(&format!("message-{i}"));
        sender.send(log_entry).await.unwrap();
    }

    // Check metrics
    let current_metrics = metrics.snapshot();
    assert_eq!(current_metrics.messages_sent, 5);
    assert!(current_metrics.queue_depth > 0);

    // Receive messages
    for _ in 0..5 {
        receiver.recv().await.unwrap();
    }

    let updated_metrics = metrics.snapshot();
    assert_eq!(updated_metrics.messages_received, 5);
}

fn create_test_log_entry(message: &str) -> EnrichedLogEntry {
    EnrichedLogEntry {
        service_type: "test".to_string(),
        log_type: "test".to_string(),
        message: message.to_string(),
        level: Some(rask_log_forwarder::parser::LogLevel::Info),
        timestamp: "2024-01-01T00:00:00Z".to_string(),
        stream: "stdout".to_string(),
        method: None,
        path: None,
        status_code: None,
        response_size: None,
        ip_address: None,
        user_agent: None,
        container_id: "test123".to_string(),
        service_name: "test-service".to_string(),
        service_group: Some("test-group".to_string()),
        fields: std::collections::HashMap::new(),
    }
}
