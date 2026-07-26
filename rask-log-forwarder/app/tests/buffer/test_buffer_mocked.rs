use rask_log_forwarder::buffer::{
    BufferConfig, LogBuffer, MemoryConfig, MemoryManager, MemoryPressure,
};
use rask_log_forwarder::parser::EnrichedLogEntry;
use std::time::Duration;

// These tests exercise the real `rask_log_forwarder::buffer::MemoryManager`
// directly rather than a hand-rolled stand-in. A previous version of this
// file defined a local `MockMemoryManager` that reimplemented the pressure
// math with its own (different) threshold defaults -- it drifted from the
// production `MemoryConfig::default()` (critical at 0.9 instead of the real
// 0.95) and exposed a `can_allocate` method the real type doesn't even have.
// Tests built on that mock could pass or fail independently of whether the
// real `MemoryManager` behaved correctly, so they are rewritten here against
// the production type.

fn memory_manager(max_memory: usize) -> MemoryManager {
    MemoryManager::new(MemoryConfig {
        max_memory,
        ..MemoryConfig::default()
    })
}

#[tokio::test]
async fn test_memory_manager_allocation() {
    let manager = memory_manager(10240); // 10KB

    manager.allocate(1024).await;

    let usage = manager.memory_usage();
    assert_eq!(usage, 1024);

    let ratio = manager.memory_usage_ratio();
    assert!((ratio - 0.1).abs() < 0.01); // ~10% usage

    let pressure = manager.current_pressure();
    assert_eq!(pressure, MemoryPressure::None);
}

#[tokio::test]
async fn test_memory_manager_pressure_escalation() {
    // Uses production defaults: warning_threshold = 0.8, critical_threshold = 0.95.
    let manager = memory_manager(1000);

    // Start with no pressure
    assert_eq!(manager.current_pressure(), MemoryPressure::None);
    assert!(manager.memory_usage_ratio() < 0.8);

    // Allocate to warning level (85%)
    manager.allocate(850).await;
    assert_eq!(manager.current_pressure(), MemoryPressure::Warning);
    assert!(manager.memory_usage_ratio() > 0.8);

    // Allocate to critical level (98%)
    manager.allocate(130).await; // Total: 980 bytes
    assert_eq!(manager.current_pressure(), MemoryPressure::Critical);
    assert!(manager.memory_usage_ratio() > 0.95);
}

#[tokio::test]
async fn test_memory_manager_backpressure_decision() {
    // `calculate_backpressure` (not a `can_allocate` predicate -- the real
    // API has no such method) is what production code actually consults to
    // decide whether to delay or drop an incoming entry.
    let manager = memory_manager(1024); // 1KB

    // Comfortably under the warning threshold: no delay, nothing dropped.
    let decision = manager.calculate_backpressure();
    assert_eq!(decision.delay, Duration::ZERO);
    assert!(!decision.should_drop);

    // Push usage into critical territory (>= 95%).
    manager.allocate(1000).await; // ~97.7%
    let decision = manager.calculate_backpressure();
    assert!(
        decision.should_drop,
        "critical memory pressure must signal should_drop"
    );
    assert!(decision.delay > Duration::ZERO);
}

#[tokio::test]
async fn test_memory_manager_deallocation() {
    let manager = memory_manager(4096); // 4KB

    // Test allocation
    manager.allocate(2048).await;
    assert_eq!(manager.memory_usage(), 2048);

    // Test deallocation
    manager.deallocate(1024).await;
    assert_eq!(manager.memory_usage(), 1024);

    // Over-deallocation must saturate at zero, not underflow. `AtomicUsize`
    // arithmetic is *not* subject to Rust's checked-overflow panics (those
    // only instrument the `+`/`-` operators), so a naive `fetch_sub` wraps
    // silently around to a value near `usize::MAX`. That would leave
    // `memory_usage()` reporting an astronomical figure and
    // `current_pressure()` stuck at `Critical` forever.
    manager.deallocate(2048).await;
    assert_eq!(
        manager.memory_usage(),
        0,
        "deallocating more than is currently tracked must clamp at zero"
    );
    assert_eq!(manager.current_pressure(), MemoryPressure::None);
}

// Helper function for creating test entries
fn create_test_enriched_log(id: usize) -> EnrichedLogEntry {
    EnrichedLogEntry {
        service_type: "test".to_string(),
        log_type: "info".to_string(),
        message: format!("Test message {id}"),
        level: Some(rask_log_forwarder::parser::LogLevel::Info),
        timestamp: "2024-01-01T00:00:00Z".to_string(),
        stream: "stdout".to_string(),
        method: None,
        path: None,
        status_code: None,
        response_size: None,
        ip_address: None,
        user_agent: None,
        container_id: format!("container-{id}"),
        service_name: "test-service".to_string(),
        service_group: Some("test-group".to_string()),
        trace_id: None,
        span_id: None,
        fields: std::collections::HashMap::new(),
    }
}

#[tokio::test]
async fn test_buffer_send_recv_round_trip_under_backpressure() {
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
    let (sender, mut receiver) = buffer.split().expect("Failed to split buffer");

    // Send a batch of distinctly-identifiable entries, retrying briefly on
    // backpressure rejection rather than treating it as a no-op.
    let sent_count = 10;
    for i in 0..sent_count {
        let entry = create_test_enriched_log(i);
        loop {
            match sender.send(entry.clone()).await {
                Ok(()) => break,
                Err(_) => {
                    // Entry rejected due to backpressure; back off briefly and retry.
                    tokio::time::sleep(Duration::from_millis(10)).await;
                }
            }
        }
    }

    // Every entry we sent must be receivable, in order, with its original
    // content intact -- not just "some non-empty message".
    for i in 0..sent_count {
        let entry = tokio::time::timeout(Duration::from_secs(1), receiver.recv())
            .await
            .expect("Should receive entry within timeout")
            .expect("Should have entry available");

        assert_eq!(entry.message, format!("Test message {i}"));
        assert_eq!(entry.container_id, format!("container-{i}"));
    }
}

#[tokio::test]
async fn test_memory_manager_realistic_scenario() {
    // Production defaults: warning_threshold = 0.8, critical_threshold = 0.95.
    let manager = memory_manager(1024 * 1024); // 1MB

    // Simulate gradual memory usage
    let allocations = [100 * 1024, 200 * 1024, 300 * 1024, 250 * 1024]; // 850KB total (~81%)
    for allocation in allocations.iter() {
        manager.allocate(*allocation).await;
    }

    // Should be in warning state (850KB / 1MB ~= 81%)
    assert_eq!(manager.current_pressure(), MemoryPressure::Warning);

    // Add more to reach critical (>= 95%)
    manager.allocate(150 * 1024).await; // Total: 1,000KB (~95.4%)
    assert_eq!(manager.current_pressure(), MemoryPressure::Critical);

    // Deallocate to reduce pressure back below warning (< 80%)
    manager.deallocate(400 * 1024).await; // Down to 600KB (~57.2%)
    assert_eq!(manager.current_pressure(), MemoryPressure::None);
}
