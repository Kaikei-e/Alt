use crate::buffer::queue::LogBuffer;
use crate::buffer::lockfree::LogBuffer as LockfreeLogBuffer;
use crate::parser::{NginxLogEntry, EnrichedLogEntry};
use std::sync::Arc;

#[test]
fn test_buffer_capacity_validation() {
    // Test with zero capacity - should fail
    let result = LogBuffer::new(0);
    assert!(result.is_err());
    
    // Test with reasonable capacity - should succeed
    let result = LogBuffer::new(1000);
    assert!(result.is_ok());
    
    // Test with maximum safe capacity - should succeed
    let result = LogBuffer::new(100_000_000);
    assert!(result.is_ok());
    
    // Test with excessive capacity - should fail
    let result = LogBuffer::new(100_000_001);
    assert!(result.is_err());
}

#[test]
fn test_lockfree_buffer_capacity_validation() {
    // Test with zero capacity - should fail
    let result = LockfreeLogBuffer::new(0);
    assert!(result.is_err());
    
    // Test with reasonable capacity - should succeed
    let result = LockfreeLogBuffer::new(1000);
    assert!(result.is_ok());
}

#[test]
fn test_memory_usage_calculation_safety() {
    let buffer = LogBuffer::new(10000).unwrap();
    
    // Add some entries
    for i in 0..1000 {
        let entry = Arc::new(NginxLogEntry::default());
        let _ = buffer.push(entry);
    }
    
    // Memory usage calculation should not overflow
    let metrics = buffer.metrics();
    assert!(metrics.memory_usage_bytes > 0);
    assert!(metrics.memory_usage_bytes < usize::MAX);
    
    // Verify the calculation is reasonable
    let expected_size = metrics.len * std::mem::size_of::<Arc<NginxLogEntry>>();
    assert_eq!(metrics.memory_usage_bytes, expected_size);
}

#[test]
fn test_lockfree_memory_usage_calculation_safety() {
    let buffer = LockfreeLogBuffer::new(10000).unwrap();
    
    // Add some entries
    for i in 0..1000 {
        let entry = EnrichedLogEntry::default();
        let _ = buffer.push(entry);
    }
    
    // Memory usage calculation should not overflow
    let metrics = buffer.detailed_metrics();
    assert!(metrics.memory_usage_bytes > 0);
    assert!(metrics.memory_usage_bytes < usize::MAX);
}

#[test]
fn test_batch_operations_with_large_sizes() {
    let buffer = LogBuffer::new(100_000).unwrap();
    
    // Create a large batch
    let mut entries = Vec::new();
    for i in 0..50_000 {
        entries.push(Arc::new(NginxLogEntry::default()));
    }
    
    // Push batch should not overflow
    let result = buffer.push_batch(entries);
    assert!(result.is_ok());
    
    let pushed_count = result.unwrap();
    assert!(pushed_count > 0);
    assert!(pushed_count <= 50_000);
}

#[test]
fn test_pop_batch_with_large_max_size() {
    let mut buffer = LogBuffer::new(100_000).unwrap();
    
    // Add some entries first
    for i in 0..10_000 {
        let entry = Arc::new(NginxLogEntry::default());
        let _ = buffer.push(entry);
    }
    
    // Try to pop a large batch
    let batch = buffer.pop_batch(50_000);
    assert!(batch.len() <= 10_000); // Should not exceed available entries
    assert!(batch.len() > 0);
}

#[test]
fn test_fill_ratio_calculation_safety() {
    let buffer = LogBuffer::new(10000).unwrap();
    
    // Test with empty buffer
    let ratio = buffer.fill_ratio();
    assert!(ratio >= 0.0);
    assert!(ratio <= 1.0);
    
    // Add some entries
    for i in 0..5000 {
        let entry = Arc::new(NginxLogEntry::default());
        let _ = buffer.push(entry);
    }
    
    // Test with partially filled buffer
    let ratio = buffer.fill_ratio();
    assert!(ratio >= 0.0);
    assert!(ratio <= 1.0);
    assert!(ratio > 0.0); // Should be positive
}

#[test]
fn test_throughput_calculation_safety() {
    let buffer = LogBuffer::new(10000).unwrap();
    
    // Add some entries
    for i in 0..1000 {
        let entry = Arc::new(NginxLogEntry::default());
        let _ = buffer.push(entry);
    }
    
    // Wait a bit to ensure time passes
    std::thread::sleep(std::time::Duration::from_millis(10));
    
    // Throughput calculation should not overflow or panic
    let metrics = buffer.detailed_metrics();
    assert!(metrics.throughput_per_second >= 0.0);
    assert!(metrics.throughput_per_second < f64::MAX);
}

#[test]
fn test_vec_with_capacity_safety() {
    // Test that Vec::with_capacity doesn't panic with large but reasonable sizes
    let capacity = 1_000_000;
    let vec: Vec<u8> = Vec::with_capacity(capacity);
    assert_eq!(vec.capacity(), capacity);
    assert!(vec.is_empty());
    
    // Test with maximum safe capacity
    let max_safe_capacity = usize::MAX / std::mem::size_of::<u8>() / 2;
    let vec: Vec<u8> = Vec::with_capacity(max_safe_capacity);
    assert!(vec.capacity() > 0);
}

#[test]
fn test_arithmetic_operations_safety() {
    // Test multiplication that could overflow
    let large_number = usize::MAX / 2;
    
    // Use checked arithmetic
    let result = large_number.checked_mul(2);
    assert!(result.is_some());
    
    let result = large_number.checked_mul(3);
    assert!(result.is_none()); // Should overflow
    
    // Test addition that could overflow
    let result = large_number.checked_add(large_number);
    assert!(result.is_some());
    
    let result = large_number.checked_add(large_number + 1);
    assert!(result.is_none()); // Should overflow
}