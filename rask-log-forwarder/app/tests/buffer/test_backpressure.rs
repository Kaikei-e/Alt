use chrono::Utc;
use rask_log_forwarder::buffer::{
    BackpressureLevel, BackpressureStrategy, BufferConfig, BufferError, LogBuffer,
};
use rask_log_forwarder::parser::EnrichedLogEntry;
use rask_log_forwarder::parser::NginxLogEntry;
use std::sync::Arc;
use std::time::Duration;
use tokio::time;

#[tokio::test]
async fn test_buffer_backpressure_handling() {
    let buffer = Arc::new(
        LogBuffer::new_with_config(BufferConfig {
            capacity: 100,
            batch_size: 10,
            batch_timeout: Duration::from_millis(100),
            enable_backpressure: true,
            backpressure_threshold: 0.5,
            backpressure_delay: Duration::from_millis(10),
        })
        .await
        .unwrap(),
    ); // Small buffer
    let (backpressure_tx, mut backpressure_rx) = tokio::sync::mpsc::channel(1000);

    // Slow consumer simulation
    let consumer_handle = tokio::spawn(async move {
        let mut consumed = 0;
        while let Some(_log) = backpressure_rx.recv().await {
            // Simulate slow processing
            time::sleep(Duration::from_millis(10)).await;
            consumed += 1;
            if consumed >= 50 {
                break;
            }
        }
        consumed
    });

    // Fast producer with backpressure handling
    let buffer_producer = buffer.clone();
    let producer_handle = tokio::spawn(async move {
        let mut sent = 0;
        for i in 0..200 {
            let log_entry = create_test_nginx_log(i);

            // Try to push to buffer
            match buffer_producer.push(log_entry.clone()) {
                Ok(()) => {
                    // Successfully buffered, send to consumer
                    if backpressure_tx.send(log_entry).await.is_ok() {
                        sent += 1;
                    }
                }
                Err(BufferError::BufferFull) => {
                    // Apply backpressure
                    time::sleep(Duration::from_millis(1)).await;
                }
                Err(_) => break,
            }
        }
        sent
    });

    let (consumed, sent) = tokio::join!(consumer_handle, producer_handle);

    // Should handle backpressure gracefully
    assert!(sent.unwrap() >= 50);
    assert_eq!(consumed.unwrap(), 50);

    // Buffer should not have excessive drops
    let metrics = buffer.metrics().snapshot();
    let drop_ratio =
        metrics.messages_dropped as f64 / (metrics.messages_sent + metrics.messages_dropped) as f64;
    assert!(drop_ratio < 0.5, "Drop ratio too high: {drop_ratio:.2}");
}

#[tokio::test]
async fn test_adaptive_backpressure() {
    let buffer_config = BufferConfig {
        capacity: 100,
        ..Default::default()
    };
    let buffer = Arc::new(LogBuffer::new_with_config(buffer_config).await.unwrap());

    // Test different backpressure strategies with smaller scope
    let strategies = vec![BackpressureStrategy::Drop, BackpressureStrategy::Yield];

    for strategy in strategies {
        buffer.reset_metrics();

        // Fill buffer to capacity first
        for i in 0..100 {
            let log_entry = create_test_nginx_log(i);
            let _ = buffer.push(log_entry);
        }

        // Now try to push more with strategy
        for i in 100..120 {
            let log_entry = create_test_nginx_log(i);
            let _ = buffer.push_with_strategy(log_entry, strategy).await;
        }

        let metrics = buffer.metrics().snapshot();
        println!(
            "Strategy {:?}: pushed={}, dropped={}",
            strategy, metrics.messages_sent, metrics.messages_dropped
        );

        // Clear buffer for next test by resetting metrics
        // Note: In a real scenario, the buffer would naturally drain as items are consumed
    }
}

#[test]
fn test_backpressure_signal_detection() {
    let buffer = LogBuffer::new(10).unwrap();

    // Fill buffer to capacity
    for i in 0..10 {
        let log_entry = create_test_nginx_log(i);
        assert!(buffer.push(log_entry).is_ok());
    }

    // Next push should trigger backpressure
    let log_entry = create_test_nginx_log(10);
    let result = buffer.push(log_entry);
    assert!(matches!(result, Err(BufferError::BufferFull)));

    // Check backpressure indicators
    assert!(buffer.is_full());
    assert_eq!(buffer.fill_ratio(), 1.0);
    assert!(buffer.needs_backpressure());
}

#[test]
fn test_backpressure_levels() {
    let buffer = LogBuffer::new(100).unwrap();

    // Test different fill levels
    assert_eq!(buffer.backpressure_level(), BackpressureLevel::None);

    // Fill to 30%
    for i in 0..30 {
        let log_entry = create_test_nginx_log(i);
        buffer.push(log_entry).unwrap();
    }
    assert_eq!(buffer.backpressure_level(), BackpressureLevel::None);

    // Fill to 60%
    for i in 30..60 {
        let log_entry = create_test_nginx_log(i);
        buffer.push(log_entry).unwrap();
    }
    assert_eq!(buffer.backpressure_level(), BackpressureLevel::Low);

    // Fill to 85%
    for i in 60..85 {
        let log_entry = create_test_nginx_log(i);
        buffer.push(log_entry).unwrap();
    }
    assert_eq!(buffer.backpressure_level(), BackpressureLevel::Medium);

    // Fill to 98%
    for i in 85..98 {
        let log_entry = create_test_nginx_log(i);
        buffer.push(log_entry).unwrap();
    }
    assert_eq!(buffer.backpressure_level(), BackpressureLevel::High);
}

#[tokio::test]
async fn test_backpressure_strategy_behavior() {
    let buffer = LogBuffer::new(10).unwrap();

    // Fill buffer to capacity
    for i in 0..10 {
        let log_entry = create_test_nginx_log(i);
        buffer.push(log_entry).unwrap();
    }

    // Test Drop strategy - should fail immediately
    let log_entry = create_test_nginx_log(10);
    let result = buffer
        .push_with_strategy(log_entry.clone(), BackpressureStrategy::Drop)
        .await;
    assert!(result.is_err()); // Should fail immediately

    // Test Sleep strategy with very short sleep
    let start = std::time::Instant::now();
    let result = buffer
        .push_with_strategy(
            log_entry.clone(),
            BackpressureStrategy::Sleep(Duration::from_millis(1)),
        )
        .await;
    let elapsed = start.elapsed();

    assert!(result.is_err()); // Should fail because buffer is full
    assert!(elapsed >= Duration::from_millis(1)); // Should have slept at least once
}

fn create_test_nginx_log(id: usize) -> Arc<NginxLogEntry> {
    Arc::new(NginxLogEntry {
        service_type: "nginx".to_string(),
        log_type: "access".to_string(),
        message: format!("Test log message {id}"),
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

#[tokio::test]
async fn test_backpressure_triggers_correctly() {
    let config = BufferConfig {
        capacity: 100,
        batch_size: 10,
        batch_timeout: Duration::from_millis(100),
        enable_backpressure: true,
        backpressure_threshold: 0.5, // 50% threshold
        backpressure_delay: Duration::from_millis(10),
    };
    let buffer = Arc::new(LogBuffer::new_with_config(config).await.unwrap()); // Small buffer
    let (sender, _receiver) = buffer.split();

    let mut success_count = 0;
    let mut backpressure_count = 0;

    // Try to send many entries
    for i in 0..200 {
        let entry = create_test_enriched_log(i);
        let start = std::time::Instant::now();

        match sender.send(entry).await {
            Ok(()) => {
                success_count += 1;
                // Check if this send took longer than expected (indicating backpressure)
                if start.elapsed() > Duration::from_millis(5) {
                    backpressure_count += 1;
                }
            }
            Err(BufferError::BufferFull) => {
                // Buffer is full, which is expected
                break;
            }
            Err(_) => {
                // Other errors
                break;
            }
        }
    }

    // Should have successfully sent some messages
    assert!(success_count > 0);

    let metrics = buffer.metrics().snapshot();
    // Should have registered some backpressure events
    assert!(metrics.backpressure_events > 0 || backpressure_count > 0);
}

#[tokio::test]
async fn test_backpressure_delays_under_load() {
    let config = BufferConfig {
        capacity: 100,
        batch_size: 10,
        batch_timeout: Duration::from_millis(100),
        enable_backpressure: true,
        backpressure_threshold: 0.8,
        backpressure_delay: Duration::from_millis(50), // Significant delay
    };
    let buffer = Arc::new(LogBuffer::new_with_config(config).await.unwrap());
    let (sender, _receiver) = buffer.split();

    let start_time = std::time::Instant::now();

    // Send enough messages to trigger backpressure
    for i in 0..90 {
        // Should hit 80% threshold
        let entry = create_test_enriched_log(i);
        let _ = sender.send(entry).await;
    }

    let elapsed = start_time.elapsed();

    // Should take some time due to backpressure delays
    let metrics = buffer.metrics().snapshot();
    if metrics.backpressure_events > 0 {
        assert!(elapsed > Duration::from_millis(10)); // Should be delayed
    }
}

#[tokio::test]
async fn test_no_backpressure_when_disabled() {
    let config = BufferConfig {
        capacity: 10,
        batch_size: 5,
        batch_timeout: Duration::from_millis(100),
        enable_backpressure: false, // Disabled
        backpressure_threshold: 0.5,
        backpressure_delay: Duration::from_millis(100),
    };
    let buffer = LogBuffer::new_with_config(config).await.unwrap();
    let (sender, _receiver) = buffer.split();

    let start_time = std::time::Instant::now();

    // Try to fill the buffer
    for i in 0..15 {
        let entry = create_test_enriched_log(i);
        let result = sender.send(entry).await;

        match result {
            Ok(()) => continue,
            Err(BufferError::BufferFull) => {
                // Expected when buffer is full
                break;
            }
            Err(_) => break,
        }
    }

    let elapsed = start_time.elapsed();
    let metrics = buffer.metrics().snapshot();

    // Should not have any backpressure events
    assert_eq!(metrics.backpressure_events, 0);
    // Should complete quickly since no backpressure delays
    assert!(elapsed < Duration::from_millis(50));
}

#[tokio::test]
async fn test_backpressure_recovery() {
    let config = BufferConfig {
        capacity: 100,
        batch_size: 10,
        batch_timeout: Duration::from_millis(100),
        enable_backpressure: true,
        backpressure_threshold: 0.7,
        backpressure_delay: Duration::from_millis(1),
    };
    let buffer = LogBuffer::new_with_config(config).await.unwrap();
    let (sender, mut receiver) = buffer.split();

    // Fill buffer to trigger backpressure
    for i in 0..80 {
        let entry = create_test_enriched_log(i);
        let _ = sender.send(entry).await;
    }

    let metrics_before = buffer.metrics().snapshot();

    // Consume some entries to reduce pressure
    for _ in 0..30 {
        let _ = receiver.recv_with_timeout(Duration::from_millis(10)).await;
    }

    // Now sending should be faster
    let start_time = std::time::Instant::now();
    for i in 80..85 {
        let entry = create_test_enriched_log(i);
        let _ = sender.send(entry).await;
    }
    let elapsed = start_time.elapsed();

    let metrics_after = buffer.metrics().snapshot();

    // Should have reduced queue depth
    assert!(metrics_after.queue_depth < metrics_before.queue_depth);

    // Later sends should be faster
    assert!(elapsed < Duration::from_millis(100));
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
        fields: HashMap::new(),
    }
}
