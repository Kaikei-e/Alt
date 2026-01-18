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
        method: None,
        path: None,
        status_code: None,
        response_size: None,
        ip_address: None,
        user_agent: None,
        trace_id: Some("4bf92f3577b34da6a3ce929d0e0e4736".to_string()),
        span_id: Some("00f067aa0ba902b7".to_string()),
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
        method: None,
        path: None,
        status_code: None,
        response_size: None,
        ip_address: None,
        user_agent: None,
        trace_id: None,
        span_id: None,
    };

    let log_row = rask::log_exporter::clickhouse_exporter::LogRow::from(enriched_log.clone());

    assert_eq!(log_row.level, 1); // Default to Info (1) if level is None
    assert_eq!(log_row.service_group, "unknown"); // Default to "unknown" if service_group is None
}

#[test]
fn test_http_fields_added_to_fields_map() {
    let enriched_log = EnrichedLogEntry {
        service_type: "nginx".to_string(),
        log_type: "http".to_string(),
        message: "GET /api/health HTTP/1.1".to_string(),
        level: Some(LogLevel::Info),
        timestamp: "2023-01-01T12:34:56.789Z".to_string(),
        stream: "stdout".to_string(),
        container_id: "nginx123".to_string(),
        service_name: "nginx".to_string(),
        service_group: Some("web".to_string()),
        fields: HashMap::new(),
        method: Some("GET".to_string()),
        path: Some("/api/health".to_string()),
        status_code: Some(200),
        response_size: Some(1024),
        ip_address: Some("192.168.1.1".to_string()),
        user_agent: Some("curl/7.68.0".to_string()),
        trace_id: None,
        span_id: None,
    };

    let log_row = rask::log_exporter::clickhouse_exporter::LogRow::from(enriched_log);

    // Verify HTTP fields are added to fields Map
    let fields_map: HashMap<String, String> = log_row.fields.into_iter().collect();
    assert_eq!(fields_map.get("http_method"), Some(&"GET".to_string()));
    assert_eq!(
        fields_map.get("http_path"),
        Some(&"/api/health".to_string())
    );
    assert_eq!(fields_map.get("http_status"), Some(&"200".to_string()));
    assert_eq!(fields_map.get("http_size"), Some(&"1024".to_string()));
    assert_eq!(fields_map.get("http_ip"), Some(&"192.168.1.1".to_string()));
    assert_eq!(fields_map.get("http_ua"), Some(&"curl/7.68.0".to_string()));
}

#[test]
fn test_trace_context_conversion_to_fixed_bytes() {
    let enriched_log = EnrichedLogEntry {
        service_type: "alt-backend".to_string(),
        log_type: "structured".to_string(),
        message: "test message".to_string(),
        level: Some(LogLevel::Info),
        timestamp: "2023-01-01T12:34:56.789Z".to_string(),
        stream: "stdout".to_string(),
        container_id: "container123".to_string(),
        service_name: "alt-backend".to_string(),
        service_group: Some("backend".to_string()),
        fields: HashMap::new(),
        method: None,
        path: None,
        status_code: None,
        response_size: None,
        ip_address: None,
        user_agent: None,
        trace_id: Some("4bf92f3577b34da6a3ce929d0e0e4736".to_string()),
        span_id: Some("00f067aa0ba902b7".to_string()),
    };

    let log_row = rask::log_exporter::clickhouse_exporter::LogRow::from(enriched_log);

    // Verify trace_id is converted to [u8; 32]
    let expected_trace_id: [u8; 32] = *b"4bf92f3577b34da6a3ce929d0e0e4736";
    assert_eq!(log_row.trace_id, expected_trace_id);

    // Verify span_id is converted to [u8; 16]
    let expected_span_id: [u8; 16] = *b"00f067aa0ba902b7";
    assert_eq!(log_row.span_id, expected_span_id);
}

#[test]
fn test_trace_context_empty_when_none() {
    let enriched_log = EnrichedLogEntry {
        service_type: "alt-backend".to_string(),
        log_type: "structured".to_string(),
        message: "test message".to_string(),
        level: Some(LogLevel::Info),
        timestamp: "2023-01-01T12:34:56.789Z".to_string(),
        stream: "stdout".to_string(),
        container_id: "container123".to_string(),
        service_name: "alt-backend".to_string(),
        service_group: Some("backend".to_string()),
        fields: HashMap::new(),
        method: None,
        path: None,
        status_code: None,
        response_size: None,
        ip_address: None,
        user_agent: None,
        trace_id: None,
        span_id: None,
    };

    let log_row = rask::log_exporter::clickhouse_exporter::LogRow::from(enriched_log);

    // Verify trace_id is all zeros when None
    assert_eq!(log_row.trace_id, [0u8; 32]);

    // Verify span_id is all zeros when None
    assert_eq!(log_row.span_id, [0u8; 16]);
}
