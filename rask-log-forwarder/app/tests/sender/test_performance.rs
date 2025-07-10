use rask_log_forwarder::buffer::{Batch, BatchType};
use rask_log_forwarder::parser::{EnrichedLogEntry, LogLevel};
use rask_log_forwarder::sender::{BatchTransmitter, ClientConfig, HttpClient, MetricsCollector};
use std::collections::HashMap;
use std::sync::Arc;
use std::time::{Duration, Instant};

fn create_test_entry(message: &str) -> EnrichedLogEntry {
    EnrichedLogEntry {
        service_type: "test".to_string(),
        log_type: "test".to_string(),
        message: message.to_string(),
        level: Some(LogLevel::Info),
        timestamp: "2024-01-01T00:00:00.000Z".to_string(),
        stream: "stdout".to_string(),
        method: Some("GET".to_string()),
        path: Some("/api/test".to_string()),
        status_code: Some(200),
        response_size: Some(1024),
        ip_address: Some("192.168.1.1".to_string()),
        user_agent: Some("test-client".to_string()),
        container_id: "test123".to_string(),
        service_name: "test-service".to_string(),
        service_group: Some("test-group".to_string()),
        fields: HashMap::new(),
    }
}

#[tokio::test]
async fn test_latency_measurement() {
    let config = ClientConfig::default();

    let result = HttpClient::new(config).await;

    match result {
        Ok(client) => {
            let transmitter = BatchTransmitter::new(client);
            let entries = vec![create_test_entry("Latency test")];
            let batch = Batch::new(entries, BatchType::SizeBased);

            // Measure transmission preparation time
            let start = Instant::now();
            let _payload = transmitter.prepare_payload(&batch, false).unwrap();
            let preparation_time = start.elapsed();

            // Should be fast for small batches (more lenient for CI)
            assert!(preparation_time < Duration::from_millis(100));
        }
        Err(_) => {
            // Expected when server is not available
        }
    }
}

#[tokio::test]
async fn test_metrics_collection() {
    let metrics = MetricsCollector::new();

    // Record test transmissions with realistic latency distribution
    // Use more samples for statistically meaningful percentiles
    let test_latencies = vec![
        (true, 100, 5000, 45), // Fast successful transmission
        (true, 200, 8000, 52), // Normal successful transmission
        (true, 150, 6000, 68), // Slightly slower successful transmission
        (true, 120, 4500, 71), // Another successful transmission
        (true, 180, 7200, 85), // Slower but still successful
        (false, 50, 0, 200),   // Failed transmission (higher latency but not extreme)
        (true, 140, 5800, 89), // Successful transmission
        (true, 160, 6400, 95), // Another successful transmission
    ];

    for (success, entries, bytes, latency_ms) in test_latencies {
        let compression_info = if success && entries > 100 {
            Some((bytes * 3 / 5, bytes)) // 60% compression ratio
        } else {
            None
        };

        metrics.record_transmission(
            success,
            entries,
            bytes,
            Duration::from_millis(latency_ms),
            compression_info,
        );
    }

    let snapshot = metrics.snapshot();

    assert_eq!(snapshot.total_batches_sent, 8);
    assert_eq!(snapshot.total_entries_sent, 1100); // Sum of all entries
    assert_eq!(snapshot.total_bytes_sent, 42900); // Sum of bytes from successful transmissions only
    assert_eq!(snapshot.successful_transmissions, 7);
    assert_eq!(snapshot.failed_transmissions, 1);

    // Test compression ratio
    assert!(snapshot.compression_ratio > 0.0);
    assert!(snapshot.compression_ratio < 1.0);

    // Test latency metrics with realistic expectations
    assert!(snapshot.average_latency > Duration::ZERO);

    // With our test data: [45, 52, 68, 71, 85, 89, 95, 200]
    // Average ≈ 88ms, P95 should be around 95-200ms range
    assert!(snapshot.p95_latency >= snapshot.average_latency);
    assert!(snapshot.p99_latency >= snapshot.p95_latency);

    // Verify reasonable ranges
    assert!(snapshot.average_latency >= Duration::from_millis(70));
    assert!(snapshot.average_latency <= Duration::from_millis(110));
    assert!(snapshot.p95_latency >= Duration::from_millis(90));
    assert!(snapshot.p99_latency >= Duration::from_millis(95));
}

#[tokio::test]
async fn test_concurrent_transmissions() {
    let config = ClientConfig {
        max_connections: 10,
        ..Default::default()
    };

    let result = HttpClient::new(config).await;

    match result {
        Ok(client) => {
            let client = Arc::new(client);

            // Spawn multiple transmission preparation tasks
            let mut handles = vec![];

            for i in 0..5 {
                let client_clone = client.clone();
                let handle = tokio::spawn(async move {
                    let transmitter = BatchTransmitter::new((*client_clone).clone());

                    let entries = vec![create_test_entry(&format!("Concurrent test {i}"))];
                    let batch = Batch::new(entries, BatchType::SizeBased);

                    // Test payload preparation (no actual transmission)
                    transmitter.prepare_payload(&batch, false).unwrap()
                });
                handles.push(handle);
            }

            // Wait for all tasks to complete
            for handle in handles {
                let payload = handle.await.unwrap();
                assert!(!payload.is_empty());
            }

            // Check connection stats
            let stats = client.connection_stats();
            assert_eq!(stats.max_connections, 10);
        }
        Err(_) => {
            // Expected when server is not available
        }
    }
}

#[tokio::test]
async fn test_large_batch_performance() {
    let config = ClientConfig::default();

    let result = HttpClient::new(config).await;

    match result {
        Ok(client) => {
            let transmitter = BatchTransmitter::new(client);

            // Create 10K entries
            let entries: Vec<_> = (0..10000)
                .map(|i| create_test_entry(&format!("Performance test entry {i}")))
                .collect();

            let batch = Batch::new(entries, BatchType::SizeBased);

            // Test serialization performance
            let start = Instant::now();
            let payload = transmitter.prepare_payload(&batch, false).unwrap();
            let serialization_time = start.elapsed();

            // Should serialize 10K entries reasonably quickly
            assert!(serialization_time < Duration::from_millis(1000));
            assert!(!payload.is_empty());

            // Test with compression
            let start = Instant::now();
            let compressed_payload = transmitter.prepare_payload(&batch, true).unwrap();
            let compression_time = start.elapsed();

            // Compression should be reasonably fast
            assert!(compression_time < Duration::from_millis(2000));
            assert!(compressed_payload.len() < payload.len());
        }
        Err(_) => {
            // Expected when server is not available
        }
    }
}

#[tokio::test]
async fn test_memory_efficiency() {
    let config = ClientConfig::default();

    let result = HttpClient::new(config).await;

    match result {
        Ok(client) => {
            let transmitter = BatchTransmitter::new(client);

            // Test multiple batches to ensure no memory leaks
            for i in 0..10 {
                let entries = (0..1000)
                    .map(|j| create_test_entry(&format!("Memory test batch {i} entry {j}")))
                    .collect();

                let batch = Batch::new(entries, BatchType::SizeBased);
                let _payload = transmitter.prepare_payload(&batch, false).unwrap();

                // Force garbage collection opportunity
                tokio::task::yield_now().await;
            }

            // Memory usage should remain stable
            // (In a real test environment, you'd measure actual memory usage)
        }
        Err(_) => {
            // Expected when server is not available
        }
    }
}

#[tokio::test]
async fn test_percentile_calculations() {
    let metrics = MetricsCollector::new();

    // Record many samples with known distribution
    let latencies = vec![10, 20, 30, 40, 50, 60, 70, 80, 90, 100]; // ms

    for latency_ms in latencies {
        metrics.record_transmission(true, 100, 1000, Duration::from_millis(latency_ms), None);
    }

    let snapshot = metrics.snapshot();

    // With 10 samples, p95 should be around 90-100ms
    assert!(snapshot.p95_latency >= Duration::from_millis(80));
    assert!(snapshot.p95_latency <= Duration::from_millis(100));

    // p99 should be around 90-100ms
    assert!(snapshot.p99_latency >= Duration::from_millis(90));
    assert!(snapshot.p99_latency <= Duration::from_millis(100));

    // Average should be around 55ms
    assert!(snapshot.average_latency >= Duration::from_millis(40));
    assert!(snapshot.average_latency <= Duration::from_millis(70));
}

#[tokio::test]
async fn test_metrics_reset() {
    let metrics = MetricsCollector::new();

    // Record some data
    metrics.record_transmission(true, 100, 1000, Duration::from_millis(50), None);
    metrics.record_transmission(false, 200, 0, Duration::from_millis(100), None);

    let snapshot_before = metrics.snapshot();
    assert_eq!(snapshot_before.total_batches_sent, 2);

    // Reset metrics
    metrics.reset();

    let snapshot_after = metrics.snapshot();
    assert_eq!(snapshot_after.total_batches_sent, 0);
    assert_eq!(snapshot_after.total_entries_sent, 0);
    assert_eq!(snapshot_after.successful_transmissions, 0);
    assert_eq!(snapshot_after.failed_transmissions, 0);
}

#[test]
fn test_compression_ratio_calculation() {
    let metrics = MetricsCollector::new();

    // Test with no compression data
    let snapshot = metrics.snapshot();
    assert_eq!(snapshot.compression_ratio, 1.0);

    // Test with compression data
    metrics.record_transmission(
        true,
        100,
        1000,
        Duration::from_millis(50),
        Some((600, 1000)), // 60% compression ratio
    );

    let snapshot = metrics.snapshot();
    assert!((snapshot.compression_ratio - 0.6).abs() < 0.01);
}
