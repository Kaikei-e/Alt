use rask_log_forwarder::buffer::Batch;
use rask_log_forwarder::parser::EnrichedLogEntry;
use rask_log_forwarder::reliability::{DiskConfig, DiskError, DiskFallback};
use std::time::Duration;
use tempfile::TempDir;

#[tokio::test]
async fn test_store_and_retrieve_batch() {
    let temp_dir = TempDir::new().unwrap();
    let config = DiskConfig {
        storage_path: temp_dir.path().to_path_buf(),
        max_disk_usage: 100 * 1024 * 1024,           // 100MB
        retention_period: Duration::from_secs(3600), // 1 hour
        compression: true,
    };

    let mut disk_fallback = DiskFallback::new(config).await.unwrap();

    let batch = create_test_batch_with_entries(100);
    let batch_id = batch.id().to_string();

    // Store batch
    disk_fallback.store_batch(batch).await.unwrap();

    // Verify it exists
    assert!(disk_fallback.has_batch(&batch_id).await);

    // Retrieve batch
    let retrieved_batch = disk_fallback.retrieve_batch(&batch_id).await.unwrap();
    assert_eq!(retrieved_batch.id(), batch_id);
    assert_eq!(retrieved_batch.size(), 100);
}

#[tokio::test]
async fn test_list_stored_batches() {
    let temp_dir = TempDir::new().unwrap();
    let config = DiskConfig {
        storage_path: temp_dir.path().to_path_buf(),
        max_disk_usage: 100 * 1024 * 1024,
        retention_period: Duration::from_secs(3600),
        compression: true,
    };

    let mut disk_fallback = DiskFallback::new(config).await.unwrap();

    // Store multiple batches
    let batch1 = create_test_batch_with_entries(50);
    let batch2 = create_test_batch_with_entries(75);
    let batch3 = create_test_batch_with_entries(100);

    disk_fallback.store_batch(batch1.clone()).await.unwrap();
    disk_fallback.store_batch(batch2.clone()).await.unwrap();
    disk_fallback.store_batch(batch3.clone()).await.unwrap();

    // List all stored batches
    let stored_batches = disk_fallback.list_stored_batches().await.unwrap();
    assert_eq!(stored_batches.len(), 3);

    let stored_ids: std::collections::HashSet<&str> =
        stored_batches.iter().map(|s| s.as_str()).collect();
    assert!(stored_ids.contains(batch1.id()));
    assert!(stored_ids.contains(batch2.id()));
    assert!(stored_ids.contains(batch3.id()));
}

#[tokio::test]
async fn test_disk_usage_limits() {
    let temp_dir = TempDir::new().unwrap();
    let config = DiskConfig {
        storage_path: temp_dir.path().to_path_buf(),
        max_disk_usage: 1024, // Very small limit (1KB)
        retention_period: Duration::from_secs(3600),
        compression: true,
    };

    let mut disk_fallback = DiskFallback::new(config).await.unwrap();

    // Try to store a large batch that exceeds the limit
    let large_batch = create_test_batch_with_entries(1000);

    let result = disk_fallback.store_batch(large_batch).await;
    assert!(result.is_err());
    assert!(matches!(result.unwrap_err(), DiskError::DiskSpaceExceeded));
}

#[tokio::test]
async fn test_batch_cleanup_by_age() {
    let temp_dir = TempDir::new().unwrap();
    let config = DiskConfig {
        storage_path: temp_dir.path().to_path_buf(),
        max_disk_usage: 100 * 1024 * 1024,
        retention_period: Duration::from_secs(1), // 1 second retention for testing
        compression: false,
    };

    let mut disk_fallback = DiskFallback::new(config).await.unwrap();

    let batch = create_test_batch_with_entries(10);
    let batch_id = batch.id().to_string();

    // Store batch
    disk_fallback.store_batch(batch).await.unwrap();
    assert!(disk_fallback.has_batch(&batch_id).await);

    // Wait for retention period to expire with extra buffer
    tokio::time::sleep(Duration::from_secs(2)).await;

    // Run cleanup
    disk_fallback.cleanup_old_batches().await.unwrap();

    // Batch should be removed
    assert!(!disk_fallback.has_batch(&batch_id).await);
}

#[tokio::test]
async fn test_delete_batch() {
    let temp_dir = TempDir::new().unwrap();
    let config = DiskConfig {
        storage_path: temp_dir.path().to_path_buf(),
        max_disk_usage: 100 * 1024 * 1024,
        retention_period: Duration::from_secs(3600),
        compression: false,
    };

    let mut disk_fallback = DiskFallback::new(config).await.unwrap();

    let batch = create_test_batch_with_entries(20);
    let batch_id = batch.id().to_string();

    // Store and verify
    disk_fallback.store_batch(batch).await.unwrap();
    assert!(disk_fallback.has_batch(&batch_id).await);

    // Delete and verify
    disk_fallback.delete_batch(&batch_id).await.unwrap();
    assert!(!disk_fallback.has_batch(&batch_id).await);
}

fn create_test_batch_with_entries(count: usize) -> Batch {
    let entries = (0..count)
        .map(|i| create_test_entry(&format!("Disk fallback test entry {i}")))
        .collect();

    Batch::new(entries, rask_log_forwarder::buffer::BatchType::SizeBased)
}

fn create_test_entry(message: &str) -> EnrichedLogEntry {
    EnrichedLogEntry {
        service_type: "test".to_string(),
        log_type: "test".to_string(),
        message: message.to_string(),
        level: Some(rask_log_forwarder::parser::LogLevel::Info),
        timestamp: "2024-01-01T00:00:00.000Z".to_string(),
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
