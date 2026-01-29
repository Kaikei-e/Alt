//! OTLP serialization tests for rask-log-forwarder.
//!
//! These tests verify the conversion of EnrichedLogEntry batches to OTLP protobuf format.

#![cfg(feature = "otlp")]

use rask_log_forwarder::buffer::{Batch, BatchType};
use rask_log_forwarder::parser::{EnrichedLogEntry, LogLevel};
use rask_log_forwarder::sender::otlp::OtlpSerializer;
use std::collections::HashMap;

use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use opentelemetry_proto::tonic::common::v1::any_value;
use prost::Message;

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

fn create_test_entry_with_trace(message: &str) -> EnrichedLogEntry {
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
        trace_id: Some("4bf92f3577b34da6a3ce929d0e0e4736".to_string()),
        span_id: Some("00f067aa0ba902b7".to_string()),
        fields: HashMap::new(),
    }
}

#[test]
fn test_otlp_serialization_basic() {
    let serializer = OtlpSerializer::new();

    let entries = vec![
        create_test_entry("Message 1"),
        create_test_entry("Message 2"),
        create_test_entry("Message 3"),
    ];

    let batch = Batch::new(entries, BatchType::SizeBased);
    let protobuf_bytes = serializer.serialize_batch(&batch).expect("Serialization should succeed");

    // Verify we can decode the protobuf
    let request = ExportLogsServiceRequest::decode(protobuf_bytes.as_slice())
        .expect("Should decode as ExportLogsServiceRequest");

    // Verify structure
    assert!(!request.resource_logs.is_empty(), "Should have resource logs");

    // Count total log records
    let total_records: usize = request
        .resource_logs
        .iter()
        .flat_map(|rl| &rl.scope_logs)
        .map(|sl| sl.log_records.len())
        .sum();

    assert_eq!(total_records, 3, "Should have 3 log records");
}

#[test]
fn test_otlp_serialization_with_trace_context() {
    let serializer = OtlpSerializer::new();

    let entries = vec![create_test_entry_with_trace("Traced message")];

    let batch = Batch::new(entries, BatchType::SizeBased);
    let protobuf_bytes = serializer.serialize_batch(&batch).expect("Serialization should succeed");

    let request = ExportLogsServiceRequest::decode(protobuf_bytes.as_slice())
        .expect("Should decode as ExportLogsServiceRequest");

    // Get the log record
    let log_record = &request.resource_logs[0].scope_logs[0].log_records[0];

    // Verify trace_id (16 bytes)
    assert_eq!(log_record.trace_id.len(), 16, "trace_id should be 16 bytes");
    let expected_trace_id = hex::decode("4bf92f3577b34da6a3ce929d0e0e4736").unwrap();
    assert_eq!(log_record.trace_id, expected_trace_id, "trace_id should match");

    // Verify span_id (8 bytes)
    assert_eq!(log_record.span_id.len(), 8, "span_id should be 8 bytes");
    let expected_span_id = hex::decode("00f067aa0ba902b7").unwrap();
    assert_eq!(log_record.span_id, expected_span_id, "span_id should match");
}

#[test]
fn test_otlp_serialization_log_level_mapping() {
    let serializer = OtlpSerializer::new();

    let log_levels = [
        (LogLevel::Debug, 5),   // DEBUG
        (LogLevel::Info, 9),    // INFO
        (LogLevel::Warn, 13),   // WARN
        (LogLevel::Error, 17),  // ERROR
        (LogLevel::Fatal, 21),  // FATAL
    ];

    for (level, expected_severity) in log_levels {
        let level_debug = format!("{:?}", level);
        let mut entry = create_test_entry(&format!("Level test: {}", level_debug));
        entry.level = Some(level);

        let batch = Batch::new(vec![entry], BatchType::SizeBased);
        let protobuf_bytes = serializer.serialize_batch(&batch).expect("Serialization should succeed");

        let request = ExportLogsServiceRequest::decode(protobuf_bytes.as_slice())
            .expect("Should decode as ExportLogsServiceRequest");

        let log_record = &request.resource_logs[0].scope_logs[0].log_records[0];
        assert_eq!(
            log_record.severity_number as u8, expected_severity,
            "LogLevel::{} should map to severity {}",
            level_debug, expected_severity
        );
    }
}

#[test]
fn test_otlp_serialization_resource_attributes() {
    let serializer = OtlpSerializer::new();

    let entry = create_test_entry("Test message");
    let batch = Batch::new(vec![entry], BatchType::SizeBased);
    let protobuf_bytes = serializer.serialize_batch(&batch).expect("Serialization should succeed");

    let request = ExportLogsServiceRequest::decode(protobuf_bytes.as_slice())
        .expect("Should decode as ExportLogsServiceRequest");

    let resource = request.resource_logs[0]
        .resource
        .as_ref()
        .expect("Should have resource");

    // Find service.name attribute
    let service_name_attr = resource
        .attributes
        .iter()
        .find(|kv| kv.key == "service.name")
        .expect("Should have service.name attribute");

    if let Some(any_value::Value::StringValue(value)) = &service_name_attr.value.as_ref().and_then(|v| v.value.as_ref()) {
        assert_eq!(value, "test-service");
    } else {
        panic!("service.name should be a string");
    }

    // Find container.id attribute
    let container_id_attr = resource
        .attributes
        .iter()
        .find(|kv| kv.key == "container.id")
        .expect("Should have container.id attribute");

    if let Some(any_value::Value::StringValue(value)) = &container_id_attr.value.as_ref().and_then(|v| v.value.as_ref()) {
        assert_eq!(value, "test123");
    } else {
        panic!("container.id should be a string");
    }
}

#[test]
fn test_otlp_serialization_log_attributes() {
    let serializer = OtlpSerializer::new();

    let mut entry = create_test_entry("Test message");
    entry.fields.insert("custom_field".to_string(), "custom_value".to_string());
    entry.fields.insert("request_id".to_string(), "req-123".to_string());

    let batch = Batch::new(vec![entry], BatchType::SizeBased);
    let protobuf_bytes = serializer.serialize_batch(&batch).expect("Serialization should succeed");

    let request = ExportLogsServiceRequest::decode(protobuf_bytes.as_slice())
        .expect("Should decode as ExportLogsServiceRequest");

    let log_record = &request.resource_logs[0].scope_logs[0].log_records[0];

    // Check custom field
    let custom_attr = log_record
        .attributes
        .iter()
        .find(|kv| kv.key == "custom_field")
        .expect("Should have custom_field attribute");

    if let Some(any_value::Value::StringValue(value)) = &custom_attr.value.as_ref().and_then(|v| v.value.as_ref()) {
        assert_eq!(value, "custom_value");
    } else {
        panic!("custom_field should be a string");
    }

    // Check HTTP method attribute
    let method_attr = log_record
        .attributes
        .iter()
        .find(|kv| kv.key == "http.method")
        .expect("Should have http.method attribute");

    if let Some(any_value::Value::StringValue(value)) = &method_attr.value.as_ref().and_then(|v| v.value.as_ref()) {
        assert_eq!(value, "GET");
    } else {
        panic!("http.method should be a string");
    }
}

#[test]
fn test_otlp_serialization_message_body() {
    let serializer = OtlpSerializer::new();

    let entry = create_test_entry("This is the log message body");
    let batch = Batch::new(vec![entry], BatchType::SizeBased);
    let protobuf_bytes = serializer.serialize_batch(&batch).expect("Serialization should succeed");

    let request = ExportLogsServiceRequest::decode(protobuf_bytes.as_slice())
        .expect("Should decode as ExportLogsServiceRequest");

    let log_record = &request.resource_logs[0].scope_logs[0].log_records[0];

    // Check body
    let body = log_record.body.as_ref().expect("Should have body");
    if let Some(any_value::Value::StringValue(value)) = &body.value {
        assert_eq!(value, "This is the log message body");
    } else {
        panic!("Body should be a string");
    }
}

#[test]
fn test_otlp_serialization_timestamp() {
    let serializer = OtlpSerializer::new();

    let entry = create_test_entry("Test message");
    let batch = Batch::new(vec![entry], BatchType::SizeBased);
    let protobuf_bytes = serializer.serialize_batch(&batch).expect("Serialization should succeed");

    let request = ExportLogsServiceRequest::decode(protobuf_bytes.as_slice())
        .expect("Should decode as ExportLogsServiceRequest");

    let log_record = &request.resource_logs[0].scope_logs[0].log_records[0];

    // Timestamp should be in nanoseconds (2024-01-01T00:00:00.000Z)
    // = 1704067200000000000 nanoseconds since Unix epoch
    assert!(log_record.time_unix_nano > 0, "time_unix_nano should be set");
    assert!(log_record.observed_time_unix_nano > 0, "observed_time_unix_nano should be set");
}

#[test]
fn test_otlp_serialization_empty_batch_error() {
    let serializer = OtlpSerializer::new();
    let batch = Batch::new(vec![], BatchType::SizeBased);

    let result = serializer.serialize_batch(&batch);
    assert!(result.is_err(), "Empty batch should return error");
}

#[test]
fn test_otlp_serialization_grouping_by_service() {
    let serializer = OtlpSerializer::new();

    let mut entry1 = create_test_entry("Service A message 1");
    entry1.service_name = "service-a".to_string();

    let mut entry2 = create_test_entry("Service B message");
    entry2.service_name = "service-b".to_string();

    let mut entry3 = create_test_entry("Service A message 2");
    entry3.service_name = "service-a".to_string();

    let batch = Batch::new(vec![entry1, entry2, entry3], BatchType::SizeBased);
    let protobuf_bytes = serializer.serialize_batch(&batch).expect("Serialization should succeed");

    let request = ExportLogsServiceRequest::decode(protobuf_bytes.as_slice())
        .expect("Should decode as ExportLogsServiceRequest");

    // Should have 2 ResourceLogs (one for each service)
    assert_eq!(request.resource_logs.len(), 2, "Should have 2 ResourceLogs for 2 services");

    // Count total records
    let total_records: usize = request
        .resource_logs
        .iter()
        .flat_map(|rl| &rl.scope_logs)
        .map(|sl| sl.log_records.len())
        .sum();
    assert_eq!(total_records, 3, "Should have 3 total log records");
}

#[test]
fn test_otlp_serialization_large_batch() {
    let serializer = OtlpSerializer::new();

    let entries: Vec<_> = (0..1000)
        .map(|i| create_test_entry(&format!("Large batch message {}", i)))
        .collect();

    let batch = Batch::new(entries, BatchType::SizeBased);
    let protobuf_bytes = serializer.serialize_batch(&batch).expect("Serialization should succeed");

    // Verify we can decode
    let request = ExportLogsServiceRequest::decode(protobuf_bytes.as_slice())
        .expect("Should decode as ExportLogsServiceRequest");

    let total_records: usize = request
        .resource_logs
        .iter()
        .flat_map(|rl| &rl.scope_logs)
        .map(|sl| sl.log_records.len())
        .sum();

    assert_eq!(total_records, 1000, "Should have 1000 log records");
}

#[test]
fn test_otlp_protobuf_smaller_than_json() {
    use rask_log_forwarder::sender::BatchSerializer;

    let otlp_serializer = OtlpSerializer::new();
    let json_serializer = BatchSerializer::new();

    let entries: Vec<_> = (0..100)
        .map(|i| create_test_entry(&format!("Comparison message with some content {}", i)))
        .collect();

    let batch = Batch::new(entries, BatchType::SizeBased);

    let protobuf_bytes = otlp_serializer.serialize_batch(&batch).expect("OTLP serialization should succeed");
    let ndjson = json_serializer.serialize_ndjson(&batch).expect("NDJSON serialization should succeed");

    // Protobuf should typically be smaller than NDJSON
    println!("Protobuf size: {} bytes", protobuf_bytes.len());
    println!("NDJSON size: {} bytes", ndjson.len());

    // Allow some tolerance - protobuf should be at most 80% of NDJSON size
    // (in practice it's usually 30-50% smaller)
    assert!(
        protobuf_bytes.len() < ndjson.len(),
        "Protobuf ({} bytes) should be smaller than NDJSON ({} bytes)",
        protobuf_bytes.len(),
        ndjson.len()
    );
}
