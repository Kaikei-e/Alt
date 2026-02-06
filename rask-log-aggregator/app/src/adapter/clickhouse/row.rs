use crate::adapter::clickhouse::convert::string_to_fixed_bytes;
use crate::domain::{EnrichedLogEntry, LogLevel};
use chrono::{DateTime, Utc};
use clickhouse::serde::chrono::datetime64::millis;
use serde::{Deserialize, Serialize};

#[derive(clickhouse::Row, Serialize, Deserialize, Clone, Debug)]
pub struct LogRow {
    pub service_type: String, // LowCardinality(String)
    pub log_type: String,     // LowCardinality(String)
    pub message: String,      // String
    pub level: i8,            // Enum8 -> underlying UInt8
    #[serde(with = "millis")]
    pub timestamp: DateTime<Utc>, // DateTime64(3,'UTC')
    pub stream: String,       // LowCardinality(String)
    pub container_id: String, // String
    pub service_name: String, // LowCardinality(String)
    pub service_group: String, // LowCardinality(String)
    #[serde(rename = "TraceId")]
    pub trace_id: [u8; 32], // FixedString(32) for trace correlation
    #[serde(rename = "SpanId")]
    pub span_id: [u8; 16], // FixedString(16) for span correlation
    pub fields: Vec<(String, String)>, // Map(String,String)
}

impl From<EnrichedLogEntry> for LogRow {
    fn from(log: EnrichedLogEntry) -> Self {
        let mut fields: Vec<(String, String)> = log.fields.into_iter().collect();

        // Add HTTP-specific fields to the fields Map (for materialized view extraction)
        if let Some(method) = log.method {
            fields.push(("http_method".to_string(), method));
        }
        if let Some(path) = log.path {
            fields.push(("http_path".to_string(), path));
        }
        if let Some(status) = log.status_code {
            fields.push(("http_status".to_string(), status.to_string()));
        }
        if let Some(size) = log.response_size {
            fields.push(("http_size".to_string(), size.to_string()));
        }
        if let Some(ip) = log.ip_address {
            fields.push(("http_ip".to_string(), ip));
        }
        if let Some(ua) = log.user_agent {
            fields.push(("http_ua".to_string(), ua));
        }

        // Convert trace context to FixedString format
        let trace_id = string_to_fixed_bytes::<32>(log.trace_id.as_deref().unwrap_or(""));
        let span_id = string_to_fixed_bytes::<16>(log.span_id.as_deref().unwrap_or(""));

        Self {
            service_type: log.service_type,
            log_type: log.log_type,
            message: log.message,
            level: match log.level {
                Some(LogLevel::Debug) => 0,
                Some(LogLevel::Info) | None => 1,
                Some(LogLevel::Warn) => 2,
                Some(LogLevel::Error) => 3,
                Some(LogLevel::Fatal) => 4,
            },
            timestamp: log
                .timestamp
                .parse::<DateTime<Utc>>()
                .unwrap_or_else(|_| Utc::now()),
            stream: log.stream,
            container_id: log.container_id,
            service_name: log.service_name,
            service_group: log.service_group.unwrap_or_else(|| "unknown".into()),
            trace_id,
            span_id,
            fields,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

    fn create_test_enriched_log() -> EnrichedLogEntry {
        EnrichedLogEntry {
            service_type: "backend".to_string(),
            log_type: "application".to_string(),
            message: "Test message".to_string(),
            level: Some(LogLevel::Info),
            timestamp: "2024-01-01T00:00:00Z".to_string(),
            stream: "stdout".to_string(),
            container_id: "abc123".to_string(),
            service_name: "test-service".to_string(),
            service_group: Some("core".to_string()),
            fields: HashMap::new(),
            method: None,
            path: None,
            status_code: None,
            response_size: None,
            ip_address: None,
            user_agent: None,
            trace_id: None,
            span_id: None,
        }
    }

    #[test]
    fn test_log_row_level_conversion() {
        for (level, expected) in [
            (Some(LogLevel::Debug), 0),
            (Some(LogLevel::Info), 1),
            (Some(LogLevel::Warn), 2),
            (Some(LogLevel::Error), 3),
            (Some(LogLevel::Fatal), 4),
            (None, 1), // Default to Info
        ] {
            let mut log = create_test_enriched_log();
            log.level = level;
            let row = LogRow::from(log);
            assert_eq!(row.level, expected);
        }
    }

    #[test]
    fn test_log_row_http_fields_added() {
        let mut log = create_test_enriched_log();
        log.method = Some("GET".to_string());
        log.path = Some("/api/test".to_string());
        log.status_code = Some(200);
        log.response_size = Some(1024);
        log.ip_address = Some("127.0.0.1".to_string());
        log.user_agent = Some("test-agent".to_string());

        let row = LogRow::from(log);

        assert!(
            row.fields
                .contains(&("http_method".to_string(), "GET".to_string()))
        );
        assert!(
            row.fields
                .contains(&("http_path".to_string(), "/api/test".to_string()))
        );
        assert!(
            row.fields
                .contains(&("http_status".to_string(), "200".to_string()))
        );
        assert!(
            row.fields
                .contains(&("http_size".to_string(), "1024".to_string()))
        );
        assert!(
            row.fields
                .contains(&("http_ip".to_string(), "127.0.0.1".to_string()))
        );
        assert!(
            row.fields
                .contains(&("http_ua".to_string(), "test-agent".to_string()))
        );
    }

    #[test]
    fn test_log_row_trace_context_conversion() {
        let mut log = create_test_enriched_log();
        log.trace_id = Some("0123456789abcdef0123456789abcdef".to_string());
        log.span_id = Some("0123456789abcdef".to_string());

        let row = LogRow::from(log);

        let expected_trace_id: [u8; 32] = string_to_fixed_bytes("0123456789abcdef0123456789abcdef");
        let expected_span_id: [u8; 16] = string_to_fixed_bytes("0123456789abcdef");

        assert_eq!(row.trace_id, expected_trace_id);
        assert_eq!(row.span_id, expected_span_id);
    }

    #[test]
    fn test_log_row_missing_trace_context() {
        let log = create_test_enriched_log();
        let row = LogRow::from(log);

        assert_eq!(row.trace_id, [0u8; 32]);
        assert_eq!(row.span_id, [0u8; 16]);
    }

    #[test]
    fn test_log_row_service_group_default() {
        let mut log = create_test_enriched_log();
        log.service_group = None;

        let row = LogRow::from(log);
        assert_eq!(row.service_group, "unknown");
    }

    #[test]
    fn test_log_row_invalid_timestamp_fallback() {
        let mut log = create_test_enriched_log();
        log.timestamp = "not-a-valid-timestamp".to_string();

        let row = LogRow::from(log);
        // Should fallback to Utc::now(), just verify it doesn't panic
        // and the timestamp is set to some value
        assert!(row.timestamp.timestamp() > 0);
    }

    #[test]
    fn golden_log_row_from_enriched_log_entry() {
        let mut fields = HashMap::new();
        fields.insert("request_id".to_string(), "abc-123".to_string());

        let log = EnrichedLogEntry {
            service_type: "api".to_string(),
            log_type: "http".to_string(),
            message: "GET /health 200".to_string(),
            level: Some(LogLevel::Warn),
            timestamp: "2024-06-15T10:30:00.123Z".to_string(),
            stream: "stdout".to_string(),
            container_id: "deadbeef1234".to_string(),
            service_name: "alt-backend".to_string(),
            service_group: Some("core".to_string()),
            fields,
            method: Some("GET".to_string()),
            path: Some("/health".to_string()),
            status_code: Some(200),
            response_size: Some(42),
            ip_address: Some("10.0.0.1".to_string()),
            user_agent: Some("curl/8.0".to_string()),
            trace_id: Some("abcdef0123456789abcdef0123456789".to_string()),
            span_id: Some("1234567890abcdef".to_string()),
        };

        let row = LogRow::from(log);

        assert_eq!(row.service_type, "api");
        assert_eq!(row.log_type, "http");
        assert_eq!(row.message, "GET /health 200");
        assert_eq!(row.level, 2); // Warn
        assert_eq!(
            row.timestamp,
            "2024-06-15T10:30:00.123Z".parse::<DateTime<Utc>>().unwrap()
        );
        assert_eq!(row.stream, "stdout");
        assert_eq!(row.container_id, "deadbeef1234");
        assert_eq!(row.service_name, "alt-backend");
        assert_eq!(row.service_group, "core");
        assert_eq!(row.trace_id, *b"abcdef0123456789abcdef0123456789");
        assert_eq!(row.span_id, *b"1234567890abcdef");

        let fields_map: HashMap<String, String> = row.fields.into_iter().collect();
        assert_eq!(fields_map["request_id"], "abc-123");
        assert_eq!(fields_map["http_method"], "GET");
        assert_eq!(fields_map["http_path"], "/health");
        assert_eq!(fields_map["http_status"], "200");
        assert_eq!(fields_map["http_size"], "42");
        assert_eq!(fields_map["http_ip"], "10.0.0.1");
        assert_eq!(fields_map["http_ua"], "curl/8.0");
    }
}
