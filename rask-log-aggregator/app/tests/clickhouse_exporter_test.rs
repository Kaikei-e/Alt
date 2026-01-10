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
    };

    let log_row = rask::log_exporter::clickhouse_exporter::LogRow::from(enriched_log);

    // Verify HTTP fields are added to fields Map
    let fields_map: HashMap<String, String> = log_row.fields.into_iter().collect();
    assert_eq!(fields_map.get("http_method"), Some(&"GET".to_string()));
    assert_eq!(fields_map.get("http_path"), Some(&"/api/health".to_string()));
    assert_eq!(fields_map.get("http_status"), Some(&"200".to_string()));
    assert_eq!(fields_map.get("http_size"), Some(&"1024".to_string()));
    assert_eq!(fields_map.get("http_ip"), Some(&"192.168.1.1".to_string()));
    assert_eq!(fields_map.get("http_ua"), Some(&"curl/7.68.0".to_string()));
}
