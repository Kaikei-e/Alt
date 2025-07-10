use rask_log_forwarder::buffer::{
    BatchConfig, BufferConfig, BufferManager, MemoryConfig, MemoryPressure,
};
use rask_log_forwarder::parser::EnrichedLogEntry;
use tokio::time::Duration;

#[tokio::test]
async fn test_buffer_manager_integration() {
    let buffer_config = BufferConfig {
        capacity: 1000,
        batch_size: 10,
        batch_timeout: Duration::from_millis(100),
        ..Default::default()
    };

    let batch_config = BatchConfig {
        max_size: 5,
        max_wait_time: Duration::from_millis(50),
        max_memory_size: 1024,
    };

    let memory_config = MemoryConfig {
        max_memory: 10 * 1024,
        warning_threshold: 0.7,
        critical_threshold: 0.9,
    };

    let manager = BufferManager::new(buffer_config, batch_config, memory_config)
        .await
        .unwrap();

    // Test memory manager
    assert_eq!(
        manager.memory_manager().current_pressure(),
        MemoryPressure::None
    );

    // Test buffer splitting
    let (_sender, _receiver) = manager.split().expect("Failed to split buffer");

    // Test that components are accessible
    assert!(!manager.batch_former().has_ready_batch().await);
}

#[allow(dead_code)]
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
