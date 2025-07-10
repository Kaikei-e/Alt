use rask_log_forwarder::buffer::{BatchConfig, BatchFormer};
use rask_log_forwarder::parser::EnrichedLogEntry;
use tokio::time::{Duration, timeout};

#[tokio::test]
async fn test_batch_formation_by_size() {
    let config = BatchConfig {
        max_size: 10,
        max_wait_time: Duration::from_secs(10), // Long timeout
        max_memory_size: 1024 * 1024,           // 1MB
    };

    let mut former = BatchFormer::new(config);

    // Add 10 entries to trigger size-based batching
    for i in 0..10 {
        let entry = create_test_log_entry(&format!("message-{i}"));
        former.add_entry(entry).await.unwrap();
    }

    // Should have formed a batch
    let batch = timeout(Duration::from_millis(100), former.next_batch())
        .await
        .unwrap()
        .unwrap();

    assert_eq!(batch.size(), 10);
    assert_eq!(
        batch.batch_type(),
        rask_log_forwarder::buffer::BatchType::SizeBased
    );
}

#[tokio::test]
async fn test_batch_formation_by_timeout() {
    let config = BatchConfig {
        max_size: 100,                            // Large size
        max_wait_time: Duration::from_millis(50), // Short timeout
        max_memory_size: 1024 * 1024,
    };

    let mut former = BatchFormer::new(config);

    // Add just a few entries
    for i in 0..3 {
        let entry = create_test_log_entry(&format!("message-{i}"));
        former.add_entry(entry).await.unwrap();
    }

    // Should form batch due to timeout
    let batch = timeout(Duration::from_millis(100), former.next_batch())
        .await
        .unwrap()
        .unwrap();

    assert_eq!(batch.size(), 3);
    assert_eq!(
        batch.batch_type(),
        rask_log_forwarder::buffer::BatchType::TimeBased
    );
}

#[tokio::test]
async fn test_batch_formation_by_memory() {
    let config = BatchConfig {
        max_size: 1000,                         // Large size
        max_wait_time: Duration::from_secs(10), // Long timeout
        max_memory_size: 1024,                  // Small memory limit (1KB)
    };

    let mut former = BatchFormer::new(config);

    // Add entries with large messages to trigger memory-based batching
    for _i in 0..10 {
        let large_message = "x".repeat(200); // 200 bytes each
        let entry = create_test_log_entry(&large_message);
        former.add_entry(entry).await.unwrap();

        // Should trigger memory limit around 5-6 entries
        if former.has_ready_batch().await {
            break;
        }
    }

    let batch = former.next_batch().await.unwrap();
    assert!(batch.size() < 10); // Should be less than all entries
    assert_eq!(
        batch.batch_type(),
        rask_log_forwarder::buffer::BatchType::MemoryBased
    );
}

#[tokio::test]
async fn test_concurrent_batch_formation() {
    let config = BatchConfig {
        max_size: 100,
        max_wait_time: Duration::from_millis(100),
        max_memory_size: 1024 * 1024,
    };

    let mut former = BatchFormer::new(config);

    // Spawn producer task
    let former_clone = former.clone();
    tokio::spawn(async move {
        for i in 0..250 {
            let entry = create_test_log_entry(&format!("message-{i}"));
            former_clone.add_entry(entry).await.unwrap();

            if i % 50 == 0 {
                tokio::task::yield_now().await;
            }
        }
    });

    // Consume batches
    let mut total_processed = 0;
    let mut batch_count = 0;

    while total_processed < 250 {
        if let Ok(Some(batch)) = timeout(Duration::from_millis(500), former.next_batch()).await {
            total_processed += batch.size();
            batch_count += 1;

            // Each batch should be approximately 100 entries (or timeout-based)
            assert!(batch.size() <= 100);
        } else {
            break;
        }
    }

    assert_eq!(total_processed, 250);
    assert!(batch_count >= 3); // Should have multiple batches
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
