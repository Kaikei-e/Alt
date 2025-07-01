use rask_log_forwarder::buffer::{MemoryConfig, MemoryManager, MemoryPressure};

#[tokio::test]
async fn test_memory_tracking() {
    let config = MemoryConfig {
        max_memory: 1024 * 1024,  // 1MB
        warning_threshold: 0.8,   // 80%
        critical_threshold: 0.95, // 95%
    };

    let manager = MemoryManager::new(config);

    // Initially should have no pressure
    assert_eq!(manager.current_pressure(), MemoryPressure::None);

    // Allocate memory that will exceed warning threshold (need > 80% = 838,861 bytes)
    manager.allocate(850 * 1024).await; // 870,400 bytes (85.0%)

    // Should be in warning state
    assert_eq!(manager.current_pressure(), MemoryPressure::Warning);

    // Allocate more to exceed critical threshold (need > 95% = 996,147 bytes)
    manager.allocate(150 * 1024).await; // Total: 1,024,000 bytes (99.9%)

    // Should be in critical state
    assert_eq!(manager.current_pressure(), MemoryPressure::Critical);

    // Free some memory to go back below critical (need < 95%)
    manager.deallocate(100 * 1024).await; // Total: 921,600 bytes (90.0%)

    // Should be in warning state (between 80% and 95%)
    assert_eq!(manager.current_pressure(), MemoryPressure::Warning);

    // Free more to go below warning threshold (need < 80%)
    manager.deallocate(150 * 1024).await; // Total: 768,000 bytes (75.0%)

    // Should return to none
    assert_eq!(manager.current_pressure(), MemoryPressure::None);
}

#[tokio::test]
async fn test_backpressure_application() {
    let config = MemoryConfig {
        max_memory: 1024,
        warning_threshold: 0.5,
        critical_threshold: 0.9,
    };

    let manager = MemoryManager::new(config);

    // Fill to warning level
    manager.allocate(600).await; // 60%

    let backpressure = manager.calculate_backpressure();
    assert!(backpressure.delay > std::time::Duration::ZERO);
    assert!(!backpressure.should_drop);

    // Fill to critical level
    manager.allocate(350).await; // 95%

    let backpressure = manager.calculate_backpressure();
    assert!(backpressure.delay > std::time::Duration::from_millis(1));
    assert!(backpressure.should_drop);
}
