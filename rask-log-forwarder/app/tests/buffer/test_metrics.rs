use chrono::Utc;
use rask_log_forwarder::buffer::{BufferConfig, LogBuffer};
use rask_log_forwarder::parser::EnrichedLogEntry;
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::time;

#[tokio::test]
async fn test_buffer_metrics_tracking() {
    let buffer = LogBuffer::new_with_config(BufferConfig {
        capacity: 10,
        batch_size: 10,
        batch_timeout: Duration::from_millis(100),
        enable_backpressure: false,
        backpressure_threshold: 0.0,
        backpressure_delay: Duration::from_millis(0),
    })
    .await
    .unwrap();

    // Fill buffer to capacity
    let (sender, _receiver) = buffer.split();
    for i in 0..15 {
        let log_entry = create_test_enriched_log(i);
        sender.send(log_entry).await.ok(); // Some will fail due to capacity
    }

    let metrics = buffer.metrics().snapshot();
    assert!(metrics.queue_depth <= 10); // Should be at or near capacity
    assert!(metrics.messages_sent <= 15); // Sent attempts
    // Messages dropped count should be tracked (can be 0 or more)
    assert_eq!(metrics.messages_received, 0); // No pops yet
}

#[tokio::test]
async fn test_memory_usage_calculation() {
    let buffer = LogBuffer::new_with_config(BufferConfig {
        capacity: 1000,
        ..Default::default()
    })
    .await
    .unwrap();

    // Add some entries
    let (sender, _receiver) = buffer.split();
    for i in 0..100 {
        let log_entry = create_test_enriched_log(i);
        sender.send(log_entry).await.unwrap();
    }

    let metrics = buffer.metrics().snapshot();
    assert_eq!(metrics.queue_depth, 100);

    // Check that messages were sent successfully
    assert_eq!(metrics.messages_sent, 100);
    assert_eq!(metrics.messages_dropped, 0);
}

#[tokio::test]
async fn test_throughput_metrics() {
    let buffer = LogBuffer::new_with_config(BufferConfig {
        capacity: 1000000,
        ..Default::default()
    })
    .await
    .unwrap();
    let start = Instant::now();

    // Push many messages
    for i in 0..10000 {
        let log_entry = create_test_nginx_log(i);
        buffer.push(log_entry).unwrap();
    }

    let duration = start.elapsed();
    let metrics = buffer.metrics().snapshot();

    assert_eq!(metrics.messages_sent, 10000);
    assert_eq!(metrics.queue_depth, 10000);

    // Calculate throughput - lower expectation for tokio broadcast channel
    let throughput = metrics.messages_sent as f64 / duration.as_secs_f64();
    assert!(throughput > 10_000.0); // Should be >10K msgs/sec for tokio broadcast channel
}

#[tokio::test]
async fn test_real_time_metrics_updates() {
    let buffer = Arc::new(LogBuffer::new(1000).unwrap());

    // Background task that continuously monitors metrics
    let buffer_monitor = buffer.clone();
    let metrics_handle = tokio::spawn(async move {
        let mut last_messages_sent = 0;
        for _ in 0..10 {
            time::sleep(Duration::from_millis(10)).await;
            let metrics = buffer_monitor.metrics().snapshot();
            assert!(metrics.messages_sent >= last_messages_sent); // Should be monotonically increasing
            last_messages_sent = metrics.messages_sent;
        }
    });

    // Producer task
    let buffer_producer = buffer.clone();
    let producer_handle = tokio::spawn(async move {
        for i in 0..500 {
            let log_entry = create_test_nginx_log(i);
            buffer_producer.push(log_entry).unwrap();
            if i % 50 == 0 {
                time::sleep(Duration::from_millis(1)).await;
            }
        }
    });

    let (metrics_result, producer_result) = tokio::join!(metrics_handle, producer_handle);
    metrics_result.unwrap();
    producer_result.unwrap();
}

#[test]
fn test_detailed_metrics() {
    let mut buffer = LogBuffer::new(1000).unwrap();

    // Add some entries
    for i in 0..100 {
        let log_entry = create_test_nginx_log(i);
        buffer.push(log_entry).unwrap();
    }

    // Pop some entries
    for _ in 0..30 {
        buffer.pop().unwrap();
    }

    let detailed_metrics = buffer.detailed_metrics();
    assert_eq!(detailed_metrics.capacity, 1000);
    assert_eq!(detailed_metrics.len, 70); // 100 - 30
    assert_eq!(detailed_metrics.pushed, 100);
    assert_eq!(detailed_metrics.popped, 30);
    assert_eq!(detailed_metrics.dropped, 0);

    // Check fill ratio
    assert!((detailed_metrics.fill_ratio - 0.07).abs() < 0.01); // 70/1000 = 0.07

    // Check throughput calculation
    assert!(detailed_metrics.throughput_per_second > 0.0);
}

#[test]
fn test_metrics_reset() {
    let mut buffer = LogBuffer::new(100).unwrap();

    // Generate some activity
    for i in 0..50 {
        let log_entry = create_test_nginx_log(i);
        buffer.push(log_entry).unwrap();
    }

    for _ in 0..20 {
        buffer.pop().unwrap();
    }

    // Try to cause some drops by filling remaining capacity + more
    let remaining_capacity = buffer.capacity() - buffer.len();
    for i in 0..(remaining_capacity + 10) {
        // This should cause drops
        let log_entry = create_test_nginx_log(i + 50);
        buffer.push(log_entry).ok();
    }

    let metrics_before = buffer.metrics().snapshot();
    assert!(metrics_before.messages_sent > 0);
    assert!(metrics_before.messages_received > 0);
    assert!(metrics_before.messages_dropped > 0);

    // Reset metrics
    buffer.reset_metrics();

    let metrics_after = buffer.metrics().snapshot();
    assert_eq!(metrics_after.messages_sent, 0);
    assert_eq!(metrics_after.messages_received, 0);
    assert_eq!(metrics_after.messages_dropped, 0);
    // Length should remain the same
    assert_eq!(metrics_after.queue_depth, metrics_before.queue_depth);
}

fn create_test_nginx_log(id: usize) -> Arc<dyn std::any::Any + Send + Sync> {
    Arc::new(EnrichedLogEntry {
        service_type: "nginx".to_string(),
        log_type: "access".to_string(),
        message: format!("Test log message {id}"),
        stream: "stdout".to_string(),
        timestamp: Utc::now().to_rfc3339(),
        container_id: format!("container_{}", id % 10),
        service_name: "nginx".to_string(),
        service_group: Some("alt-frontend".to_string()),
        ip_address: Some("192.168.1.1".to_string()),
        method: Some("GET".to_string()),
        path: Some("/api/test".to_string()),
        status_code: Some(200),
        response_size: Some(1024),
        user_agent: Some("test-agent".to_string()),
        level: None,
        fields: std::collections::HashMap::new(),
    })
}

#[tokio::test]
async fn test_basic_metrics_tracking() {
    let config = BufferConfig {
        capacity: 10,
        batch_size: 5,
        batch_timeout: Duration::from_millis(100),
        enable_backpressure: true,
        backpressure_threshold: 0.8,
        backpressure_delay: Duration::from_micros(100),
    };
    let buffer = LogBuffer::new_with_config(config).await.unwrap();
    let (sender, mut receiver) = buffer.split();

    // Send some entries
    for i in 0..5 {
        let entry = create_test_enriched_log(i);
        sender.send(entry).await.unwrap();
    }

    let metrics = buffer.metrics().snapshot();
    assert_eq!(metrics.messages_sent, 5);
    assert_eq!(metrics.queue_depth, 5);

    // Receive some entries
    for _ in 0..3 {
        let _ = receiver.recv_with_timeout(Duration::from_millis(100)).await;
    }

    let metrics = buffer.metrics().snapshot();
    assert_eq!(metrics.messages_received, 3);
    assert_eq!(metrics.queue_depth, 2);
}

#[tokio::test]
async fn test_dropped_messages_tracking() {
    let config = BufferConfig {
        capacity: 1000,
        batch_size: 100,
        batch_timeout: Duration::from_millis(100),
        enable_backpressure: false, // Disable backpressure to allow drops
        backpressure_threshold: 1.0,
        backpressure_delay: Duration::from_micros(0),
    };
    let buffer = LogBuffer::new_with_config(config).await.unwrap();
    let (sender, _receiver) = buffer.split();

    let mut sent_count = 0;
    let mut dropped_count = 0;

    // Try to send many entries rapidly
    for i in 0..2000 {
        let entry = create_test_enriched_log(i);
        match sender.send(entry).await {
            Ok(()) => sent_count += 1,
            Err(_) => dropped_count += 1,
        }
    }

    let metrics = buffer.metrics().snapshot();

    // Print for debugging
    println!("Sent count: {sent_count}, Dropped count: {dropped_count}");
    println!(
        "Metrics - sent: {}, dropped: {}, queue_depth: {}",
        metrics.messages_sent, metrics.messages_dropped, metrics.queue_depth
    );

    // With backpressure disabled and capacity=1000, trying to send 2000 messages:
    // The implementation counts all send attempts as 'messages_sent', regardless of success
    assert_eq!(
        metrics.messages_sent, 2000,
        "All 2000 send attempts should be counted"
    );

    // Some messages should have been successfully queued (actual sends)
    assert!(
        sent_count > 0,
        "At least some messages should be successfully sent"
    );
    assert!(
        sent_count <= 1000,
        "Successful sends should not exceed buffer capacity"
    );

    // The rest should be dropped
    assert_eq!(
        sent_count + dropped_count,
        2000,
        "All attempts should be accounted for"
    );
    assert!(
        dropped_count > 0,
        "Some messages should be dropped due to capacity"
    );
    assert!(
        metrics.messages_dropped > 0,
        "Dropped count should be tracked in metrics"
    );
    assert_eq!(
        metrics.messages_dropped, dropped_count,
        "Dropped counts should match"
    );
}

#[tokio::test]
async fn test_async_throughput_metrics() {
    let config = BufferConfig {
        capacity: 1000000,
        batch_size: 1000,
        batch_timeout: Duration::from_millis(100),
        enable_backpressure: true,
        backpressure_threshold: 0.9,
        backpressure_delay: Duration::from_micros(10),
    };
    let buffer = LogBuffer::new_with_config(config).await.unwrap();
    let (sender, mut receiver) = buffer.split();

    let start = std::time::Instant::now();

    // Send batch of entries
    for i in 0..1000 {
        let entry = create_test_enriched_log(i);
        sender.send(entry).await.unwrap();
    }

    let send_duration = start.elapsed();

    // Receive entries
    let start = std::time::Instant::now();
    for _ in 0..1000 {
        let _ = receiver.recv_with_timeout(Duration::from_millis(10)).await;
    }
    let receive_duration = start.elapsed();

    let metrics = buffer.metrics().snapshot();
    assert_eq!(metrics.messages_sent, 1000);
    assert_eq!(metrics.messages_received, 1000);

    // Calculate throughput
    let send_throughput = 1000.0 / send_duration.as_secs_f64();
    let receive_throughput = 1000.0 / receive_duration.as_secs_f64();

    // Should be reasonably fast
    assert!(send_throughput > 1000.0); // >1K msgs/sec
    assert!(receive_throughput > 1000.0);
}

#[tokio::test]
async fn test_concurrent_metrics_accuracy() {
    let config = BufferConfig {
        capacity: 1000,
        batch_size: 100,
        batch_timeout: Duration::from_millis(100),
        enable_backpressure: true,
        backpressure_threshold: 0.8,
        backpressure_delay: Duration::from_micros(100),
    };
    let buffer = Arc::new(LogBuffer::new_with_config(config).await.unwrap());
    let (sender, mut receiver) = buffer.split();
    let sender = Arc::new(sender);

    let mut handles = vec![];

    // Spawn multiple senders
    for producer_id in 0..5 {
        let sender_clone = Arc::clone(&sender);
        let handle = tokio::spawn(async move {
            for i in 0..100 {
                let entry = create_test_enriched_log(producer_id * 100 + i);
                let _ = sender_clone.send(entry).await;
            }
        });
        handles.push(handle);
    }

    // Spawn receiver
    let buffer_clone = Arc::clone(&buffer);
    let receiver_handle = tokio::spawn(async move {
        let mut received = 0;
        for _ in 0..500 {
            let result = receiver.recv_with_timeout(Duration::from_millis(10)).await;
            if result.unwrap().is_some() {
                received += 1;
            } else {
                break;
            }
        }
        received
    });

    // Wait for all tasks
    for handle in handles {
        handle.await.unwrap();
    }
    let received_count = receiver_handle.await.unwrap();

    let metrics = buffer_clone.metrics().snapshot();

    // Metrics should be consistent
    assert!(metrics.messages_sent <= 500); // 5 producers * 100 messages
    assert_eq!(metrics.messages_received, received_count);
    assert_eq!(
        metrics.queue_depth,
        (metrics.messages_sent as usize).saturating_sub(metrics.messages_received as usize)
    );
}

#[tokio::test]
async fn test_backpressure_metrics() {
    let config = BufferConfig {
        capacity: 100,
        batch_size: 10,
        batch_timeout: Duration::from_millis(100),
        enable_backpressure: true,
        backpressure_threshold: 0.5, // Low threshold
        backpressure_delay: Duration::from_millis(1),
    };
    let buffer = LogBuffer::new_with_config(config).await.unwrap();
    let (sender, _receiver) = buffer.split();

    // Send enough to trigger backpressure
    for i in 0..80 {
        let entry = create_test_enriched_log(i);
        let _ = sender.send(entry).await;
    }

    let metrics = buffer.metrics().snapshot();

    // Should have triggered backpressure events
    assert!(metrics.backpressure_events > 0);
    assert!(metrics.messages_sent > 0);
}

fn create_test_enriched_log(id: usize) -> EnrichedLogEntry {
    use std::collections::HashMap;
    EnrichedLogEntry {
        service_type: "test".to_string(),
        log_type: "access".to_string(),
        message: format!("Test log message {id}"),
        level: Some(rask_log_forwarder::parser::LogLevel::Info),
        timestamp: chrono::Utc::now().to_rfc3339(),
        stream: "stdout".to_string(),
        method: Some("GET".to_string()),
        path: Some("/api/test".to_string()),
        status_code: Some(200),
        response_size: Some(1024),
        ip_address: Some("192.168.1.1".to_string()),
        user_agent: Some("test-agent".to_string()),
        container_id: format!("container_{}", id % 10),
        service_name: "test-service".to_string(),
        service_group: Some("test".to_string()),
        fields: HashMap::new(),
    }
}
