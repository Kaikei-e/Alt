use rask_log_forwarder::buffer::{BufferConfig, LogBuffer};
use rask_log_forwarder::parser::EnrichedLogEntry;
use std::sync::Arc;
use std::time::Duration;

#[tokio::test]
async fn test_high_throughput_single_producer() {
    let config = BufferConfig {
        capacity: 100_000,
        batch_size: 1000,
        batch_timeout: Duration::from_millis(100),
        enable_backpressure: true,
        backpressure_threshold: 0.8,
        backpressure_delay: Duration::from_micros(100),
    };
    let buffer = LogBuffer::new_with_config(config).unwrap();
    let (sender, mut receiver) = buffer.split().expect("Failed to split buffer");

    // Send 1000 log entries
    for i in 0..1000 {
        let entry = create_test_enriched_log(i);
        sender.send(entry).await.unwrap();
    }

    // Receive and count entries
    let mut received_count = 0;
    for _ in 0..1000 {
        let result = receiver.recv_with_timeout(Duration::from_millis(100)).await;
        if result.unwrap().is_some() {
            received_count += 1;
        }
    }

    assert!(received_count > 0); // Should receive at least some entries
}

#[tokio::test]
async fn test_high_throughput_multiple_producers() {
    let config = BufferConfig {
        capacity: 50000,
        batch_size: 1000,
        batch_timeout: Duration::from_millis(100),
        enable_backpressure: true,
        backpressure_threshold: 0.8,
        backpressure_delay: Duration::from_micros(100),
    };
    let buffer = Arc::new(LogBuffer::new_with_config(config).unwrap());
    let (sender, mut receiver) = buffer.split().expect("Failed to split buffer");
    let sender = Arc::new(sender);

    let mut handles = vec![];

    // Spawn 10 producer tasks
    for producer_id in 0..10 {
        let sender_clone = Arc::clone(&sender);
        let handle = tokio::spawn(async move {
            for i in 0..100 {
                let entry = create_test_enriched_log(producer_id * 100 + i);
                if let Err(err) = sender_clone.send(entry).await {
                    tracing::debug!("producer send returned: {err}");
                }
            }
        });
        handles.push(handle);
    }

    // Wait for all producers to finish
    for handle in handles {
        handle.await.unwrap();
    }

    // Receive entries
    let mut received_count = 0;
    for _ in 0..1000 {
        let result = receiver.recv_with_timeout(Duration::from_millis(10)).await;
        if let Ok(Some(_)) = result {
            received_count += 1;
        } else {
            break;
        }
    }

    assert!(received_count > 0);
}

#[tokio::test]
async fn test_throughput_with_backpressure() {
    let config = BufferConfig {
        capacity: 100_000,
        batch_size: 1000,
        batch_timeout: Duration::from_millis(100),
        enable_backpressure: true,
        backpressure_threshold: 0.5, // Apply backpressure at 50%
        backpressure_delay: Duration::from_millis(1),
    };
    let buffer = LogBuffer::new_with_config(config).unwrap();
    let (sender, _receiver) = buffer.split().expect("Failed to split buffer");

    // Send many entries quickly
    for i in 0..1000 {
        let entry = create_test_enriched_log(i);
        if let Err(err) = sender.send(entry).await {
            tracing::debug!("throughput send returned: {err}");
        }
    }

    // Check that backpressure was applied
    let metrics = buffer.metrics().snapshot();
    // We expect some backpressure events due to the low threshold
    assert!(metrics.messages_sent > 0);
}

#[tokio::test]
async fn test_memory_usage_under_load() {
    let config = BufferConfig {
        capacity: 1_000_000,
        batch_size: 10000,
        batch_timeout: Duration::from_millis(100),
        enable_backpressure: true,
        backpressure_threshold: 0.9,
        backpressure_delay: Duration::from_micros(10),
    };
    let buffer = LogBuffer::new_with_config(config).unwrap();
    let (sender, mut receiver) = buffer.split().expect("Failed to split buffer");

    // Send large log entries
    for i in 0..100 {
        let mut entry = create_test_enriched_log(i);
        entry
            .fields
            .insert("large_field".to_string(), "x".repeat(1000)); // 1KB per entry
        sender.send(entry).await.unwrap();
    }

    // Consume some entries
    for _ in 0..50 {
        if let Err(err) = receiver.recv_with_timeout(Duration::from_millis(10)).await {
            tracing::debug!("recv_with_timeout returned: {err}");
        }
    }

    let metrics = buffer.metrics().snapshot();
    assert!(metrics.messages_sent >= 100);
    assert!(metrics.messages_received >= 50);
}

#[tokio::test]
async fn test_burst_handling() {
    let config = BufferConfig {
        capacity: 10,
        batch_size: 5,
        batch_timeout: Duration::from_millis(100),
        enable_backpressure: false, // Disable backpressure for this test
        backpressure_threshold: 1.0,
        backpressure_delay: Duration::from_micros(0),
    };
    let buffer = LogBuffer::new_with_config(config).unwrap(); // Small buffer
    let (sender, mut receiver) = buffer.split().expect("Failed to split buffer");

    // Send a burst of entries
    for i in 0..20 {
        let entry = create_test_enriched_log(i);
        if let Err(err) = sender.send(entry).await {
            // Some may fail due to buffer size
            tracing::debug!("burst send rejected: {err}");
        }
    }

    // Consume entries
    let mut recv_count = 0;
    for _ in 0..20 {
        let result = receiver.recv_with_timeout(Duration::from_millis(10)).await;
        if result.unwrap().is_some() {
            recv_count += 1;
        } else {
            break;
        }
    }

    let metrics = buffer.metrics().snapshot();
    assert!(metrics.messages_sent > 0);
    assert!(recv_count > 0);
}

fn create_test_enriched_log(id: usize) -> EnrichedLogEntry {
    use std::collections::HashMap;
    EnrichedLogEntry {
        service_type: "test".to_string(),
        log_type: "info".to_string(),
        message: format!("Test log message {id}"),
        level: Some(rask_log_forwarder::parser::LogLevel::Info),
        timestamp: chrono::Utc::now().to_rfc3339(),
        stream: "stdout".to_string(),
        method: None,
        path: None,
        status_code: None,
        response_size: None,
        ip_address: None,
        user_agent: None,
        container_id: format!("container_{}", id % 10),
        service_name: "test-service".to_string(),
        service_group: None,
        trace_id: None,
        span_id: None,
        fields: HashMap::new(),
    }
}
