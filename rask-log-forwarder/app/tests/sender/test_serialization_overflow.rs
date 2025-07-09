use crate::buffer::Batch;
use crate::sender::serialization::{BatchSerializer, SerializationError};
use crate::parser::EnrichedLogEntry;

#[test]
fn test_estimate_serialized_size_overflow() {
    let serializer = BatchSerializer::new();
    
    // Test with reasonable batch size
    let mut batch = Batch::new("test".to_string(), 1000);
    for _ in 0..1000 {
        batch.push(EnrichedLogEntry::default());
    }
    
    let estimated_size = serializer.estimate_serialized_size(&batch);
    assert!(estimated_size > 0);
    assert!(estimated_size < usize::MAX);
}

#[test]
fn test_large_batch_size_handling() {
    let serializer = BatchSerializer::new();
    
    // Create a batch with a large number of entries
    let mut batch = Batch::new("large_test".to_string(), 100_000);
    for i in 0..100_000 {
        let mut entry = EnrichedLogEntry::default();
        entry.set_message(format!("Test log entry {}", i));
        batch.push(entry);
    }
    
    // This should not panic or overflow
    let estimated_size = serializer.estimate_serialized_size(&batch);
    assert!(estimated_size > 0);
    
    // Verify serialization works with large batches
    let result = serializer.serialize_ndjson(&batch);
    match result {
        Ok(serialized) => {
            assert!(!serialized.is_empty());
            // Verify it contains newlines (NDJSON format)
            assert!(serialized.contains('\n'));
        }
        Err(e) => {
            // Should not fail due to overflow
            panic!("Serialization failed unexpectedly: {}", e);
        }
    }
}

#[test]
fn test_capacity_validation() {
    let serializer = BatchSerializer::new();
    
    // Test with maximum safe batch size
    let max_safe_size = usize::MAX / 1000; // Leave room for the multiplication
    let mut batch = Batch::new("max_test".to_string(), max_safe_size);
    
    // Add a few entries (not the full amount to avoid test timeout)
    for i in 0..100 {
        let mut entry = EnrichedLogEntry::default();
        entry.set_message(format!("Test entry {}", i));
        batch.push(entry);
    }
    
    // This should not overflow
    let estimated_size = serializer.estimate_serialized_size(&batch);
    assert!(estimated_size > 0);
}

#[test]
fn test_empty_batch_handling() {
    let serializer = BatchSerializer::new();
    let batch = Batch::new("empty_test".to_string(), 0);
    
    // Empty batch should be handled gracefully
    let estimated_size = serializer.estimate_serialized_size(&batch);
    assert_eq!(estimated_size, 1024); // Just the metadata overhead
    
    // Serialization should return appropriate error
    let result = serializer.serialize_ndjson(&batch);
    assert!(matches!(result, Err(SerializationError::EmptyBatch)));
}

#[test]
fn test_vec_with_capacity_safety() {
    let serializer = BatchSerializer::new();
    let mut batch = Batch::new("capacity_test".to_string(), 1000);
    
    for i in 0..1000 {
        let mut entry = EnrichedLogEntry::default();
        entry.set_message(format!("Entry {}", i));
        batch.push(entry);
    }
    
    // Test that Vec::with_capacity doesn't panic with large estimates
    let estimated_size = serializer.estimate_serialized_size(&batch);
    
    // This should not panic
    let mut buffer = Vec::with_capacity(estimated_size);
    assert_eq!(buffer.capacity(), estimated_size);
    assert!(buffer.is_empty());
    
    // Verify we can actually write to the buffer
    buffer.extend_from_slice(b"test data");
    assert_eq!(buffer.len(), 9);
}