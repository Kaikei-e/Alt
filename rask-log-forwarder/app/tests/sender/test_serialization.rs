use rask_log_forwarder::buffer::{Batch, BatchType};
use rask_log_forwarder::parser::{EnrichedLogEntry, LogLevel};
use rask_log_forwarder::sender::{BatchSerializer, SerializationFormat};
use std::collections::HashMap;

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
        ip_address: Some("192.168.1.100".to_string()),
        user_agent: Some("test-agent/1.0".to_string()),
        container_id: "test123".to_string(),
        service_name: "test-service".to_string(),
        service_group: Some("test-group".to_string()),
        trace_id: None,
        span_id: None,
        fields: HashMap::new(),
    }
}

#[test]
fn test_ndjson_serialization() {
    let serializer = BatchSerializer::new();

    let entries = vec![
        create_test_entry("Message 1"),
        create_test_entry("Message 2"),
        create_test_entry("Message 3"),
    ];

    let batch = Batch::new(entries, BatchType::SizeBased);
    let ndjson = serializer.serialize_ndjson(&batch).unwrap();

    // Verify NDJSON format - each line should be valid JSON
    let lines: Vec<&str> = ndjson.lines().collect();
    assert_eq!(lines.len(), 3);

    for (i, line) in lines.iter().enumerate() {
        let parsed: serde_json::Value =
            serde_json::from_str(line).unwrap_or_else(|_| panic!("Line {i} should be valid JSON"));

        assert!(
            parsed["message"]
                .as_str()
                .unwrap()
                .contains(&format!("Message {}", i + 1)),
        );
        assert_eq!(parsed["service_type"], "test");
        assert_eq!(parsed["container_id"], "test123");
    }
}

#[test]
fn test_json_array_serialization() {
    let serializer = BatchSerializer::new();

    let entries = vec![
        create_test_entry("Message A"),
        create_test_entry("Message B"),
    ];

    let batch = Batch::new(entries, BatchType::TimeBased);
    let json = serializer.serialize_json_array(&batch).unwrap();

    let parsed: serde_json::Value = serde_json::from_str(&json).unwrap();
    let array = parsed.as_array().unwrap();

    assert_eq!(array.len(), 2);
    assert_eq!(array[0]["message"], "Message A");
    assert_eq!(array[1]["message"], "Message B");
}

#[test]
fn test_batch_with_metadata_serialization() {
    let serializer = BatchSerializer::new();

    let entries = vec![create_test_entry("Test message")];
    let batch = Batch::new(entries, BatchType::MemoryBased);

    let ndjson = serializer.serialize_batch_with_metadata(&batch).unwrap();
    let lines: Vec<&str> = ndjson.lines().collect();

    // Should have metadata line + log entry line
    assert_eq!(lines.len(), 2);

    // First line should be batch metadata
    let metadata: serde_json::Value = serde_json::from_str(lines[0]).unwrap();
    assert_eq!(metadata["batch_id"], batch.id());
    assert_eq!(metadata["batch_size"], 1);
    assert_eq!(metadata["batch_type"], "MemoryBased");

    // Second line should be the log entry
    let entry: serde_json::Value = serde_json::from_str(lines[1]).unwrap();
    assert_eq!(entry["message"], "Test message");
}

#[test]
fn test_large_batch_serialization() {
    let serializer = BatchSerializer::new();

    let entries: Vec<_> = (0..10000)
        .map(|i| create_test_entry(&format!("Large batch message {i}")))
        .collect();

    let batch = Batch::new(entries, BatchType::SizeBased);
    let ndjson = serializer.serialize_ndjson(&batch).unwrap();

    let lines: Vec<&str> = ndjson.lines().collect();
    assert_eq!(lines.len(), 10000);

    // Check first and last entries
    let first: serde_json::Value = serde_json::from_str(lines[0]).unwrap();
    let last: serde_json::Value = serde_json::from_str(lines[9999]).unwrap();

    assert!(first["message"].as_str().unwrap().contains("message 0"));
    assert!(last["message"].as_str().unwrap().contains("message 9999"));
}

#[test]
fn test_empty_batch_error() {
    let serializer = BatchSerializer::new();
    let batch = Batch::new(vec![], BatchType::SizeBased);

    let result = serializer.serialize_ndjson(&batch);
    assert!(result.is_err());

    if let Err(e) = result {
        assert!(e.to_string().contains("empty"));
    }
}

#[test]
fn test_compression() {
    let serializer = BatchSerializer::new();

    let entries: Vec<_> = (0..1000)
        .map(|i| {
            create_test_entry(&format!(
                "Compression test message with repeated content {i}"
            ))
        })
        .collect();

    let batch = Batch::new(entries, BatchType::SizeBased);

    let uncompressed = serializer.serialize_ndjson(&batch).unwrap();
    let compressed = serializer
        .serialize_compressed(&batch, SerializationFormat::NDJSON)
        .unwrap();

    // Compressed should be smaller than uncompressed
    assert!(compressed.len() < uncompressed.len());
    assert!(!compressed.is_empty());
}

#[test]
fn test_serialization_estimate() {
    let serializer = BatchSerializer::new();

    let entries = vec![create_test_entry("Test message")];
    let batch = Batch::new(entries, BatchType::SizeBased);

    let estimate = serializer.estimate_serialized_size(&batch);
    let actual = serializer.serialize_ndjson(&batch).unwrap();

    // Estimate should be reasonable (within a reasonable range of actual)
    assert!(estimate > 0, "Estimate should be positive");
    // Allow for a wider range since estimation is approximate
    assert!(
        (estimate as f64) > (actual.len() as f64 * 0.1),
        "Estimate {} should be at least 10% of actual {}",
        estimate,
        actual.len()
    );
    assert!(
        (estimate as f64) < (actual.len() as f64 * 5.0),
        "Estimate {} should be no more than 500% of actual {}",
        estimate,
        actual.len()
    );
}
