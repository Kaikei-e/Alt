// Nanosecond timestamps (u64) intentionally cast to i64 for ClickHouse.
// This won't overflow until year 2262.
#![allow(clippy::cast_possible_wrap)]

use crate::adapter::clickhouse::convert::{hashmap_to_vec, string_to_fixed_bytes};
use crate::domain::{OTelLog, OTelTrace};
use serde::{Deserialize, Serialize};

/// ClickHouse row structure for otel_logs table
/// Uses FixedString-compatible byte arrays for TraceId/SpanId
#[derive(clickhouse::Row, Serialize, Deserialize, Clone, Debug)]
pub struct OTelLogRow {
    #[serde(rename = "Timestamp")]
    pub timestamp: i64, // DateTime64(9) - nanoseconds since epoch
    #[serde(rename = "ObservedTimestamp")]
    pub observed_timestamp: i64,
    #[serde(rename = "TraceId")]
    pub trace_id: [u8; 32], // FixedString(32)
    #[serde(rename = "SpanId")]
    pub span_id: [u8; 16], // FixedString(16)
    #[serde(rename = "TraceFlags")]
    pub trace_flags: u8,
    #[serde(rename = "SeverityText")]
    pub severity_text: String,
    #[serde(rename = "SeverityNumber")]
    pub severity_number: u8,
    #[serde(rename = "Body")]
    pub body: String,
    #[serde(rename = "ResourceSchemaUrl")]
    pub resource_schema_url: String,
    #[serde(rename = "ResourceAttributes")]
    pub resource_attributes: Vec<(String, String)>,
    #[serde(rename = "ScopeSchemaUrl")]
    pub scope_schema_url: String,
    #[serde(rename = "ScopeName")]
    pub scope_name: String,
    #[serde(rename = "ScopeVersion")]
    pub scope_version: String,
    #[serde(rename = "ScopeAttributes")]
    pub scope_attributes: Vec<(String, String)>,
    #[serde(rename = "LogAttributes")]
    pub log_attributes: Vec<(String, String)>,
}

impl From<OTelLog> for OTelLogRow {
    fn from(log: OTelLog) -> Self {
        Self {
            timestamp: log.timestamp as i64,
            observed_timestamp: log.observed_timestamp as i64,
            trace_id: string_to_fixed_bytes::<32>(&log.trace_id),
            span_id: string_to_fixed_bytes::<16>(&log.span_id),
            trace_flags: log.trace_flags,
            severity_text: log.severity_text,
            severity_number: log.severity_number,
            body: log.body,
            resource_schema_url: log.resource_schema_url,
            resource_attributes: hashmap_to_vec(log.resource_attributes),
            scope_schema_url: log.scope_schema_url,
            scope_name: log.scope_name,
            scope_version: log.scope_version,
            scope_attributes: hashmap_to_vec(log.scope_attributes),
            log_attributes: hashmap_to_vec(log.log_attributes),
        }
    }
}

/// ClickHouse row structure for otel_traces table
#[derive(clickhouse::Row, Serialize, Deserialize, Clone, Debug)]
pub struct OTelTraceRow {
    #[serde(rename = "Timestamp")]
    pub timestamp: i64, // DateTime64(9) - nanoseconds since epoch
    #[serde(rename = "TraceId")]
    pub trace_id: [u8; 32], // FixedString(32)
    #[serde(rename = "SpanId")]
    pub span_id: [u8; 16], // FixedString(16)
    #[serde(rename = "ParentSpanId")]
    pub parent_span_id: [u8; 16], // FixedString(16)
    #[serde(rename = "TraceState")]
    pub trace_state: String,
    #[serde(rename = "SpanName")]
    pub span_name: String,
    #[serde(rename = "SpanKind")]
    pub span_kind: i8, // Enum8 as numeric value
    #[serde(rename = "ServiceName")]
    pub service_name: String,
    #[serde(rename = "ResourceAttributes")]
    pub resource_attributes: Vec<(String, String)>,
    #[serde(rename = "SpanAttributes")]
    pub span_attributes: Vec<(String, String)>,
    #[serde(rename = "Duration")]
    pub duration: i64,
    #[serde(rename = "StatusCode")]
    pub status_code: i8, // Enum8 as numeric value
    #[serde(rename = "StatusMessage")]
    pub status_message: String,
    // Nested arrays for Grafana ClickHouse datasource compatibility
    #[serde(rename = "Events.Timestamp")]
    pub events_timestamp: Vec<i64>, // Array(DateTime64(9))
    #[serde(rename = "Events.Name")]
    pub events_name: Vec<String>, // Array(LowCardinality(String))
    #[serde(rename = "Events.Attributes")]
    pub events_attributes: Vec<Vec<(String, String)>>, // Array(Map(String, String))
    #[serde(rename = "Links.TraceId")]
    pub links_trace_id: Vec<String>, // Array(String)
    #[serde(rename = "Links.SpanId")]
    pub links_span_id: Vec<String>, // Array(String)
    #[serde(rename = "Links.TraceState")]
    pub links_trace_state: Vec<String>, // Array(String)
    #[serde(rename = "Links.Attributes")]
    pub links_attributes: Vec<Vec<(String, String)>>, // Array(Map(String, String))
}

impl From<OTelTrace> for OTelTraceRow {
    fn from(trace: OTelTrace) -> Self {
        // Extract nested event arrays
        let events_timestamp: Vec<i64> = trace
            .events_nested
            .iter()
            .map(|e| e.timestamp as i64)
            .collect();
        let events_name: Vec<String> = trace.events_nested.iter().map(|e| e.name.clone()).collect();
        let events_attributes: Vec<Vec<(String, String)>> = trace
            .events_nested
            .iter()
            .map(|e| hashmap_to_vec(e.attributes.clone()))
            .collect();

        // Extract nested link arrays
        let links_trace_id: Vec<String> = trace
            .links_nested
            .iter()
            .map(|l| l.trace_id.clone())
            .collect();
        let links_span_id: Vec<String> = trace
            .links_nested
            .iter()
            .map(|l| l.span_id.clone())
            .collect();
        let links_trace_state: Vec<String> = trace
            .links_nested
            .iter()
            .map(|l| l.trace_state.clone())
            .collect();
        let links_attributes: Vec<Vec<(String, String)>> = trace
            .links_nested
            .iter()
            .map(|l| hashmap_to_vec(l.attributes.clone()))
            .collect();

        Self {
            timestamp: trace.timestamp as i64,
            trace_id: string_to_fixed_bytes::<32>(&trace.trace_id),
            span_id: string_to_fixed_bytes::<16>(&trace.span_id),
            parent_span_id: string_to_fixed_bytes::<16>(&trace.parent_span_id),
            trace_state: trace.trace_state,
            span_name: trace.span_name,
            span_kind: trace.span_kind as i8,
            service_name: trace.service_name,
            resource_attributes: hashmap_to_vec(trace.resource_attributes),
            span_attributes: hashmap_to_vec(trace.span_attributes),
            duration: trace.duration,
            status_code: trace.status_code as i8,
            status_message: trace.status_message,
            // Nested arrays for Grafana
            events_timestamp,
            events_name,
            events_attributes,
            links_trace_id,
            links_span_id,
            links_trace_state,
            links_attributes,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::{SpanEvent, SpanKind, SpanLink, StatusCode as DomainStatusCode};
    use std::collections::HashMap;

    fn create_test_otel_log() -> OTelLog {
        OTelLog {
            timestamp: 1_700_000_000_000_000_000,
            observed_timestamp: 1_700_000_000_100_000_000,
            trace_id: "0123456789abcdef0123456789abcdef".to_string(),
            span_id: "0123456789abcdef".to_string(),
            trace_flags: 1,
            severity_text: "INFO".to_string(),
            severity_number: 9,
            body: "Test log message".to_string(),
            resource_schema_url: "https://opentelemetry.io/schemas/1.0.0".to_string(),
            resource_attributes: {
                let mut m = HashMap::new();
                m.insert("service.name".to_string(), "test-service".to_string());
                m
            },
            scope_schema_url: "https://opentelemetry.io/schemas/1.0.0".to_string(),
            scope_name: "test-scope".to_string(),
            scope_version: "1.0.0".to_string(),
            scope_attributes: HashMap::new(),
            log_attributes: {
                let mut m = HashMap::new();
                m.insert("custom.key".to_string(), "custom.value".to_string());
                m
            },
            service_name: "test-service".to_string(),
        }
    }

    #[test]
    fn test_otel_log_row_timestamp_conversion() {
        let log = create_test_otel_log();
        let row = OTelLogRow::from(log);
        assert_eq!(row.timestamp, 1_700_000_000_000_000_000_i64);
        assert_eq!(row.observed_timestamp, 1_700_000_000_100_000_000_i64);
    }

    #[test]
    fn test_otel_log_row_trace_id_fixed_bytes() {
        let log = create_test_otel_log();
        let row = OTelLogRow::from(log);
        let expected: [u8; 32] = string_to_fixed_bytes("0123456789abcdef0123456789abcdef");
        assert_eq!(row.trace_id, expected);
    }

    #[test]
    fn test_otel_log_row_span_id_fixed_bytes() {
        let log = create_test_otel_log();
        let row = OTelLogRow::from(log);
        let expected: [u8; 16] = string_to_fixed_bytes("0123456789abcdef");
        assert_eq!(row.span_id, expected);
    }

    #[test]
    fn test_otel_log_row_attributes_vec_conversion() {
        let mut log = create_test_otel_log();
        log.resource_attributes.clear();
        log.resource_attributes
            .insert("key1".to_string(), "val1".to_string());
        log.resource_attributes
            .insert("key2".to_string(), "val2".to_string());

        let row = OTelLogRow::from(log);
        assert_eq!(row.resource_attributes.len(), 2);
    }

    #[test]
    fn test_otel_log_row_all_fields_mapped() {
        let log = create_test_otel_log();
        let row = OTelLogRow::from(log.clone());

        assert_eq!(row.trace_flags, log.trace_flags);
        assert_eq!(row.severity_text, log.severity_text);
        assert_eq!(row.severity_number, log.severity_number);
        assert_eq!(row.body, log.body);
        assert_eq!(row.resource_schema_url, log.resource_schema_url);
        assert_eq!(row.scope_schema_url, log.scope_schema_url);
        assert_eq!(row.scope_name, log.scope_name);
        assert_eq!(row.scope_version, log.scope_version);
    }

    #[test]
    fn test_otel_log_row_empty_trace_context() {
        let mut log = create_test_otel_log();
        log.trace_id = String::new();
        log.span_id = String::new();

        let row = OTelLogRow::from(log);
        assert_eq!(row.trace_id, [0u8; 32]);
        assert_eq!(row.span_id, [0u8; 16]);
    }

    fn create_test_otel_trace() -> OTelTrace {
        OTelTrace {
            timestamp: 1_700_000_000_000_000_000,
            trace_id: "0123456789abcdef0123456789abcdef".to_string(),
            span_id: "0123456789abcdef".to_string(),
            parent_span_id: "fedcba9876543210".to_string(),
            trace_state: "vendor1=value1".to_string(),
            span_name: "test-span".to_string(),
            span_kind: SpanKind::Server,
            service_name: "test-service".to_string(),
            resource_attributes: {
                let mut m = HashMap::new();
                m.insert("service.name".to_string(), "test-service".to_string());
                m
            },
            span_attributes: {
                let mut m = HashMap::new();
                m.insert("http.method".to_string(), "GET".to_string());
                m
            },
            duration: 1_000_000,
            status_code: DomainStatusCode::Ok,
            status_message: String::new(),
            events_nested: vec![],
            links_nested: vec![],
        }
    }

    #[test]
    fn test_otel_trace_row_basic_fields() {
        let trace = create_test_otel_trace();
        let row = OTelTraceRow::from(trace.clone());

        assert_eq!(row.timestamp, trace.timestamp as i64);
        assert_eq!(row.trace_state, trace.trace_state);
        assert_eq!(row.span_name, trace.span_name);
        assert_eq!(row.span_kind, SpanKind::Server as i8);
        assert_eq!(row.service_name, trace.service_name);
        assert_eq!(row.duration, trace.duration);
        assert_eq!(row.status_code, DomainStatusCode::Ok as i8);
        assert_eq!(row.status_message, trace.status_message);
    }

    #[test]
    fn test_otel_trace_row_fixed_bytes_conversion() {
        let trace = create_test_otel_trace();
        let row = OTelTraceRow::from(trace);

        let expected_trace_id: [u8; 32] = string_to_fixed_bytes("0123456789abcdef0123456789abcdef");
        let expected_span_id: [u8; 16] = string_to_fixed_bytes("0123456789abcdef");
        let expected_parent_id: [u8; 16] = string_to_fixed_bytes("fedcba9876543210");

        assert_eq!(row.trace_id, expected_trace_id);
        assert_eq!(row.span_id, expected_span_id);
        assert_eq!(row.parent_span_id, expected_parent_id);
    }

    #[test]
    fn test_otel_trace_row_nested_events_expansion() {
        let mut trace = create_test_otel_trace();
        trace.events_nested = vec![
            SpanEvent {
                timestamp: 1_700_000_000_001_000_000,
                name: "event1".to_string(),
                attributes: {
                    let mut m = HashMap::new();
                    m.insert("key".to_string(), "value".to_string());
                    m
                },
            },
            SpanEvent {
                timestamp: 1_700_000_000_002_000_000,
                name: "event2".to_string(),
                attributes: HashMap::new(),
            },
        ];

        let row = OTelTraceRow::from(trace);

        assert_eq!(row.events_timestamp.len(), 2);
        assert_eq!(row.events_name.len(), 2);
        assert_eq!(row.events_attributes.len(), 2);

        assert_eq!(row.events_timestamp[0], 1_700_000_000_001_000_000_i64);
        assert_eq!(row.events_timestamp[1], 1_700_000_000_002_000_000_i64);
        assert_eq!(row.events_name[0], "event1");
        assert_eq!(row.events_name[1], "event2");
        assert_eq!(row.events_attributes[0].len(), 1);
        assert_eq!(row.events_attributes[1].len(), 0);
    }

    #[test]
    fn test_otel_trace_row_nested_links_expansion() {
        let mut trace = create_test_otel_trace();
        trace.links_nested = vec![
            SpanLink {
                trace_id: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa".to_string(),
                span_id: "bbbbbbbbbbbbbbbb".to_string(),
                trace_state: "vendor=test".to_string(),
                attributes: {
                    let mut m = HashMap::new();
                    m.insert("link.type".to_string(), "follows".to_string());
                    m
                },
            },
            SpanLink {
                trace_id: "cccccccccccccccccccccccccccccccc".to_string(),
                span_id: "dddddddddddddddd".to_string(),
                trace_state: String::new(),
                attributes: HashMap::new(),
            },
        ];

        let row = OTelTraceRow::from(trace);

        assert_eq!(row.links_trace_id.len(), 2);
        assert_eq!(row.links_span_id.len(), 2);
        assert_eq!(row.links_trace_state.len(), 2);
        assert_eq!(row.links_attributes.len(), 2);

        assert_eq!(row.links_trace_id[0], "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");
        assert_eq!(row.links_span_id[0], "bbbbbbbbbbbbbbbb");
        assert_eq!(row.links_trace_state[0], "vendor=test");
        assert_eq!(row.links_attributes[0].len(), 1);

        assert_eq!(row.links_trace_id[1], "cccccccccccccccccccccccccccccccc");
        assert_eq!(row.links_span_id[1], "dddddddddddddddd");
        assert!(row.links_trace_state[1].is_empty());
        assert_eq!(row.links_attributes[1].len(), 0);
    }

    #[test]
    fn test_otel_trace_row_empty_events_and_links() {
        let trace = create_test_otel_trace();
        let row = OTelTraceRow::from(trace);

        assert!(row.events_timestamp.is_empty());
        assert!(row.events_name.is_empty());
        assert!(row.events_attributes.is_empty());
        assert!(row.links_trace_id.is_empty());
        assert!(row.links_span_id.is_empty());
        assert!(row.links_trace_state.is_empty());
        assert!(row.links_attributes.is_empty());
    }

    #[test]
    fn test_otel_trace_row_span_kind_variants() {
        for (kind, expected_i8) in [
            (SpanKind::Unspecified, 0),
            (SpanKind::Internal, 1),
            (SpanKind::Server, 2),
            (SpanKind::Client, 3),
            (SpanKind::Producer, 4),
            (SpanKind::Consumer, 5),
        ] {
            let mut trace = create_test_otel_trace();
            trace.span_kind = kind;
            let row = OTelTraceRow::from(trace);
            assert_eq!(row.span_kind, expected_i8);
        }
    }

    #[test]
    fn test_otel_trace_row_status_code_variants() {
        for (status, expected_i8) in [
            (DomainStatusCode::Unset, 0),
            (DomainStatusCode::Ok, 1),
            (DomainStatusCode::Error, 2),
        ] {
            let mut trace = create_test_otel_trace();
            trace.status_code = status;
            let row = OTelTraceRow::from(trace);
            assert_eq!(row.status_code, expected_i8);
        }
    }

    #[test]
    fn test_otel_trace_row_attributes_conversion() {
        let trace = create_test_otel_trace();
        let row = OTelTraceRow::from(trace);

        assert!(!row.resource_attributes.is_empty());
        assert!(!row.span_attributes.is_empty());
    }

    #[test]
    fn test_otel_trace_row_empty_parent_span_id() {
        let mut trace = create_test_otel_trace();
        trace.parent_span_id = String::new();

        let row = OTelTraceRow::from(trace);
        assert_eq!(row.parent_span_id, [0u8; 16]);
    }

    #[test]
    fn golden_otel_log_row_from_otel_log() {
        let log = OTelLog {
            timestamp: 1_718_444_400_000_000_000,
            observed_timestamp: 1_718_444_400_100_000_000,
            trace_id: "abcdef0123456789abcdef0123456789".to_string(),
            span_id: "1234567890abcdef".to_string(),
            trace_flags: 1,
            severity_text: "WARN".to_string(),
            severity_number: 13,
            body: "Connection timeout".to_string(),
            resource_schema_url: "https://opentelemetry.io/schemas/1.0.0".to_string(),
            resource_attributes: {
                let mut m = HashMap::new();
                m.insert("service.name".to_string(), "alt-backend".to_string());
                m
            },
            scope_schema_url: String::new(),
            scope_name: "my-scope".to_string(),
            scope_version: "1.2.3".to_string(),
            scope_attributes: HashMap::new(),
            log_attributes: {
                let mut m = HashMap::new();
                m.insert("error.type".to_string(), "timeout".to_string());
                m
            },
            service_name: "alt-backend".to_string(),
        };

        let row = OTelLogRow::from(log);

        assert_eq!(row.timestamp, 1_718_444_400_000_000_000_i64);
        assert_eq!(row.observed_timestamp, 1_718_444_400_100_000_000_i64);
        assert_eq!(
            row.trace_id,
            string_to_fixed_bytes::<32>("abcdef0123456789abcdef0123456789")
        );
        assert_eq!(row.span_id, string_to_fixed_bytes::<16>("1234567890abcdef"));
        assert_eq!(row.trace_flags, 1);
        assert_eq!(row.severity_text, "WARN");
        assert_eq!(row.severity_number, 13);
        assert_eq!(row.body, "Connection timeout");
        assert_eq!(
            row.resource_schema_url,
            "https://opentelemetry.io/schemas/1.0.0"
        );
        assert_eq!(row.scope_name, "my-scope");
        assert_eq!(row.scope_version, "1.2.3");
        assert!(row.scope_attributes.is_empty());
        assert_eq!(row.resource_attributes.len(), 1);
        assert_eq!(row.log_attributes.len(), 1);
    }

    #[test]
    fn golden_otel_trace_row_from_otel_trace() {
        let trace = OTelTrace {
            timestamp: 1_718_444_400_000_000_000,
            trace_id: "abcdef0123456789abcdef0123456789".to_string(),
            span_id: "1234567890abcdef".to_string(),
            parent_span_id: "fedcba9876543210".to_string(),
            trace_state: "rojo=00f067aa0ba902b7".to_string(),
            span_name: "HTTP GET /api/users".to_string(),
            span_kind: SpanKind::Server,
            service_name: "alt-backend".to_string(),
            resource_attributes: {
                let mut m = HashMap::new();
                m.insert("service.name".to_string(), "alt-backend".to_string());
                m
            },
            span_attributes: {
                let mut m = HashMap::new();
                m.insert("http.method".to_string(), "GET".to_string());
                m.insert("http.route".to_string(), "/api/users".to_string());
                m
            },
            duration: 5_000_000,
            status_code: DomainStatusCode::Ok,
            status_message: String::new(),
            events_nested: vec![SpanEvent {
                timestamp: 1_718_444_400_001_000_000,
                name: "exception".to_string(),
                attributes: {
                    let mut m = HashMap::new();
                    m.insert("exception.type".to_string(), "TimeoutError".to_string());
                    m
                },
            }],
            links_nested: vec![SpanLink {
                trace_id: "11111111111111111111111111111111".to_string(),
                span_id: "2222222222222222".to_string(),
                trace_state: String::new(),
                attributes: HashMap::new(),
            }],
        };

        let row = OTelTraceRow::from(trace);

        assert_eq!(row.timestamp, 1_718_444_400_000_000_000_i64);
        assert_eq!(
            row.trace_id,
            string_to_fixed_bytes::<32>("abcdef0123456789abcdef0123456789")
        );
        assert_eq!(row.span_id, string_to_fixed_bytes::<16>("1234567890abcdef"));
        assert_eq!(
            row.parent_span_id,
            string_to_fixed_bytes::<16>("fedcba9876543210")
        );
        assert_eq!(row.trace_state, "rojo=00f067aa0ba902b7");
        assert_eq!(row.span_name, "HTTP GET /api/users");
        assert_eq!(row.span_kind, SpanKind::Server as i8);
        assert_eq!(row.service_name, "alt-backend");
        assert_eq!(row.duration, 5_000_000);
        assert_eq!(row.status_code, DomainStatusCode::Ok as i8);
        assert!(row.status_message.is_empty());

        assert_eq!(row.events_timestamp.len(), 1);
        assert_eq!(row.events_timestamp[0], 1_718_444_400_001_000_000_i64);
        assert_eq!(row.events_name[0], "exception");
        assert_eq!(row.events_attributes[0].len(), 1);

        assert_eq!(row.links_trace_id.len(), 1);
        assert_eq!(row.links_trace_id[0], "11111111111111111111111111111111");
        assert_eq!(row.links_span_id[0], "2222222222222222");
    }
}
