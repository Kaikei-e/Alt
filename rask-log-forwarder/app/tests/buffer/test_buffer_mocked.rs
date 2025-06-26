use rask_log_forwarder::buffer::{LogBuffer, BufferConfig, MemoryPressure};
use rask_log_forwarder::parser::EnrichedLogEntry;
use std::time::Duration;
use mockall::predicate::*;

#[cfg(test)]
use mockall::automock;

// Mock for MemoryManager
#[automock]
pub trait MemoryManagerTrait {
    async fn allocate(&self, size: u64);
    async fn deallocate(&self, size: u64);
    fn memory_usage(&self) -> u64;
    fn memory_usage_ratio(&self) -> f64;
    fn current_pressure(&self) -> MemoryPressure;
    fn can_allocate(&self, size: u64) -> bool;
}

// Memory Managerの簡単なモック実装
#[derive(Debug)]
pub struct MockMemoryManager {
    pub current_usage: u64,
    pub max_memory: u64,
    pub warning_threshold: f64,
    pub critical_threshold: f64,
}

impl MockMemoryManager {
    pub fn new(max_memory: u64) -> Self {
        Self {
            current_usage: 0,
            max_memory,
            warning_threshold: 0.8,
            critical_threshold: 0.9,
        }
    }

    pub async fn allocate(&mut self, size: u64) {
        self.current_usage += size;
    }

    pub async fn deallocate(&mut self, size: u64) {
        if self.current_usage >= size {
            self.current_usage -= size;
        } else {
            self.current_usage = 0;
        }
    }

    pub fn memory_usage(&self) -> u64 {
        self.current_usage
    }

    pub fn memory_usage_ratio(&self) -> f64 {
        self.current_usage as f64 / self.max_memory as f64
    }

    pub fn current_pressure(&self) -> MemoryPressure {
        let ratio = self.memory_usage_ratio();
        if ratio >= self.critical_threshold {
            MemoryPressure::Critical
        } else if ratio >= self.warning_threshold {
            MemoryPressure::Warning
        } else {
            MemoryPressure::None
        }
    }

    pub fn can_allocate(&self, size: u64) -> bool {
        (self.current_usage + size) < self.max_memory
    }
}

#[tokio::test]
async fn test_mock_memory_manager_allocation() {
    let mut mock_manager = MockMemoryManager::new(10240); // 10KB

    // Test allocation
    mock_manager.allocate(1024).await;

    // Verify state
    let usage = mock_manager.memory_usage();
    assert_eq!(usage, 1024);

    let ratio = mock_manager.memory_usage_ratio();
    assert!((ratio - 0.1).abs() < 0.01); // ~10% usage

    let pressure = mock_manager.current_pressure();
    assert_eq!(pressure, MemoryPressure::None);
}

#[tokio::test]
async fn test_mock_memory_manager_pressure_escalation() {
    let mut mock_manager = MockMemoryManager::new(1000); // 1KB

    // Start with no pressure
    assert_eq!(mock_manager.current_pressure(), MemoryPressure::None);
    assert!(mock_manager.memory_usage_ratio() < 0.8);

    // Allocate to warning level (85%)
    mock_manager.allocate(850).await;
    assert_eq!(mock_manager.current_pressure(), MemoryPressure::Warning);
    assert!(mock_manager.memory_usage_ratio() > 0.8);

    // Allocate to critical level (98%)
    mock_manager.allocate(130).await; // Total: 980 bytes
    assert_eq!(mock_manager.current_pressure(), MemoryPressure::Critical);
    assert!(mock_manager.memory_usage_ratio() > 0.95);
}

#[tokio::test]
async fn test_mock_memory_manager_can_allocate() {
    let mock_manager = MockMemoryManager::new(1024); // 1KB

    // Small allocation should succeed
    assert!(mock_manager.can_allocate(512));

    // Large allocation should fail
    assert!(!mock_manager.can_allocate(2048));
}

#[tokio::test]
async fn test_mock_memory_manager_deallocation() {
    let mut mock_manager = MockMemoryManager::new(4096); // 4KB

    // Test allocation
    mock_manager.allocate(2048).await;
    assert_eq!(mock_manager.memory_usage(), 2048);

    // Test deallocation
    mock_manager.deallocate(1024).await;
    assert_eq!(mock_manager.memory_usage(), 1024);

    // Test over-deallocation (should not go negative)
    mock_manager.deallocate(2048).await;
    assert_eq!(mock_manager.memory_usage(), 0);
}

// Helper function for creating test entries
fn create_test_enriched_log(id: usize) -> EnrichedLogEntry {
    EnrichedLogEntry {
        service_type: "test".to_string(),
        log_type: "info".to_string(),
        message: format!("Test message {}", id),
        level: Some(rask_log_forwarder::parser::LogLevel::Info),
        timestamp: "2024-01-01T00:00:00Z".to_string(),
        stream: "stdout".to_string(),
        method: None,
        path: None,
        status_code: None,
        response_size: None,
        ip_address: None,
        user_agent: None,
        container_id: format!("container-{}", id),
        service_name: "test-service".to_string(),
        service_group: Some("test-group".to_string()),
        fields: std::collections::HashMap::new(),
    }
}

#[tokio::test]
async fn test_buffer_with_mock_memory_pressure() {
    // Create a real buffer
    let config = BufferConfig {
        capacity: 1000,
        batch_size: 100,
        batch_timeout: Duration::from_millis(100),
        enable_backpressure: true,
        backpressure_threshold: 0.8,
        backpressure_delay: Duration::from_micros(100),
    };

    let buffer = LogBuffer::new_with_config(config).await.unwrap();
    let (sender, mut receiver) = buffer.split();

    // Simulate sending logs under various memory pressures
    for i in 0..10 {
        let entry = create_test_enriched_log(i);

        // In a real implementation, we would inject the mocked memory manager
        // For now, we just test that the buffer can handle entries
        match sender.send(entry).await {
            Ok(()) => {
                // Entry accepted
            }
            Err(_) => {
                // Entry rejected due to backpressure
                tokio::time::sleep(Duration::from_millis(10)).await;
            }
        }
    }

    // Verify we can receive some entries
    let entry = tokio::time::timeout(Duration::from_secs(1), receiver.recv())
        .await
        .expect("Should receive entry within timeout")
        .expect("Should have entry available");

    // Check that entry is valid
    assert!(!entry.message.is_empty());
}

#[tokio::test]
async fn test_mock_memory_manager_realistic_scenario() {
    let mut manager = MockMemoryManager::new(1024 * 1024); // 1MB

    // Simulate gradual memory usage
    let allocations = vec![100 * 1024, 200 * 1024, 300 * 1024, 250 * 1024]; // KB allocations

    for (_i, allocation) in allocations.iter().enumerate() {
        manager.allocate(*allocation).await;
    }

    // Should be in warning state (850KB / 1MB = 85%)
    assert_eq!(manager.current_pressure(), MemoryPressure::Warning);

    // Add more to reach critical
    manager.allocate(100 * 1024).await; // Total: 950KB (95%)
    assert_eq!(manager.current_pressure(), MemoryPressure::Critical);

    // Deallocate to reduce pressure
    manager.deallocate(200 * 1024).await; // Down to 750KB (75%)
    assert_eq!(manager.current_pressure(), MemoryPressure::None);
}