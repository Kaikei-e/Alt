use chrono::{DateTime, Utc};
use rask::domain::{EnrichedLogEntry, LogLevel};
use std::collections::HashMap;

#[test]
fn test_enriched_log_entry_to_log_row_conversion() {
    let mut fields = HashMap::new();
    fields.insert("key1".to_string(), "value1".to_string());
    fields.insert("key2".to_string(), "value2".to_string());

    let enriched_log = EnrichedLogEntry {
        service_type: "test_service_type".to_string(),
        log_type: "test_log_type".to_string(),
        message: "test_message".to_string(),
        level: Some(LogLevel::Info),
        timestamp: "2023-01-01T12:34:56.789Z".to_string(),
        stream: "test_stream".to_string(),
        container_id: "test_container_id".to_string(),
        service_name: "test_service_name".to_string(),
        service_group: Some("test_service_group".to_string()),
        fields,
    };

    let log_row = rask::log_exporter::clickhouse_exporter::LogRow::from(enriched_log.clone());

    assert_eq!(log_row.service_type, enriched_log.service_type);
    assert_eq!(log_row.log_type, enriched_log.log_type);
    assert_eq!(log_row.message, enriched_log.message);
    assert_eq!(log_row.level, 1); // LogLevel::Info maps to 1
                                  // Verify timestamp is correctly parsed
    let expected_timestamp: DateTime<Utc> = "2023-01-01T12:34:56.789Z".parse().unwrap();
    assert_eq!(log_row.timestamp, expected_timestamp);
    assert_eq!(log_row.stream, enriched_log.stream);
    assert_eq!(log_row.container_id, enriched_log.container_id);
    assert_eq!(log_row.service_name, enriched_log.service_name);
    assert_eq!(log_row.service_group, enriched_log.service_group.unwrap());
    // Convert HashMap to Vec for comparison
    let mut expected_fields: Vec<(String, String)> = enriched_log.fields.into_iter().collect();
    expected_fields.sort();
    let mut actual_fields = log_row.fields.clone();
    actual_fields.sort();
    assert_eq!(actual_fields, expected_fields);
}

#[test]
fn test_enriched_log_entry_to_log_row_conversion_no_level() {
    let enriched_log = EnrichedLogEntry {
        service_type: "test_service_type".to_string(),
        log_type: "test_log_type".to_string(),
        message: "test_message".to_string(),
        level: None,
        timestamp: "2023-01-01T12:34:56.789Z".to_string(),
        stream: "test_stream".to_string(),
        container_id: "test_container_id".to_string(),
        service_name: "test_service_name".to_string(),
        service_group: None,
        fields: HashMap::new(),
    };

    let log_row = rask::log_exporter::clickhouse_exporter::LogRow::from(enriched_log.clone());

    assert_eq!(log_row.level, 1); // Default to Info (1) if level is None
    assert_eq!(log_row.service_group, "unknown"); // Default to "unknown" if service_group is None
}
