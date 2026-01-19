// Nanosecond timestamps (u64) intentionally cast to i64 for ClickHouse.
// This won't overflow until year 2262.
#![allow(clippy::cast_possible_wrap)]

use crate::domain::{EnrichedLogEntry, LogLevel, OTelLog, OTelTrace};
use crate::error::AggregatorError;
use chrono::{DateTime, Utc};
use clickhouse::Client;
use clickhouse::serde::chrono::datetime64::millis;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::time::Duration;
use tracing::error;

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

pub struct ClickHouseExporter {
    client: Client,
}

impl ClickHouseExporter {
    #[must_use]
    pub fn new(client: Client) -> Self {
        Self { client }
    }
}

impl super::LogExporter for ClickHouseExporter {
    fn export_batch(
        &self,
        logs: Vec<EnrichedLogEntry>,
    ) -> std::pin::Pin<Box<dyn std::future::Future<Output = Result<(), AggregatorError>> + Send + '_>>
    {
        Box::pin(async move {
            // 変換時に所有権を奪い clone を削減
            let rows: Vec<LogRow> = logs.into_iter().map(LogRow::from).collect();

            // バッチを 1000 行単位で送信
            let mut inserter = self
                .client
                .inserter::<LogRow>("logs")?
                .with_timeouts(Some(Duration::from_secs(10)), Some(Duration::from_secs(10)))
                .with_max_bytes(50_000_000)
                .with_max_rows(1000);

            for row in &rows {
                match inserter.write(row) {
                    Ok(()) => (),
                    Err(e) => {
                        error!("Failed to write row to ClickHouse: {e}");
                    }
                }
            }
            inserter.end().await?; // commit 相当

            Ok(())
        })
    }
}

// =============================================================================
// OpenTelemetry Log/Trace Export
// =============================================================================

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
    // Note: Events and Links String columns removed to avoid conflict with Events.* nested arrays
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

/// Convert string to fixed-size byte array for FixedString columns
/// Pads with zeros if shorter, truncates if longer
fn string_to_fixed_bytes<const N: usize>(s: &str) -> [u8; N] {
    let mut result = [0u8; N];
    let bytes = s.as_bytes();
    let len = bytes.len().min(N);
    result[..len].copy_from_slice(&bytes[..len]);
    result
}

/// Convert HashMap to Vec for ClickHouse Map type
fn hashmap_to_vec(map: HashMap<String, String>) -> Vec<(String, String)> {
    map.into_iter().collect()
}

impl ClickHouseExporter {
    /// Export OpenTelemetry logs to ClickHouse
    pub async fn export_otel_logs(&self, logs: Vec<OTelLog>) -> Result<(), AggregatorError> {
        if logs.is_empty() {
            return Ok(());
        }

        let rows: Vec<OTelLogRow> = logs.into_iter().map(OTelLogRow::from).collect();

        let mut inserter = self
            .client
            .inserter::<OTelLogRow>("otel_logs")?
            .with_timeouts(Some(Duration::from_secs(10)), Some(Duration::from_secs(10)))
            .with_max_bytes(50_000_000)
            .with_max_rows(1000);

        for row in &rows {
            if let Err(e) = inserter.write(row) {
                error!("Failed to write OTel log row to ClickHouse: {e}");
            }
        }

        inserter.end().await?;
        Ok(())
    }

    /// Export OpenTelemetry traces to ClickHouse
    pub async fn export_otel_traces(&self, traces: Vec<OTelTrace>) -> Result<(), AggregatorError> {
        if traces.is_empty() {
            return Ok(());
        }

        let rows: Vec<OTelTraceRow> = traces.into_iter().map(OTelTraceRow::from).collect();

        let mut inserter = self
            .client
            .inserter::<OTelTraceRow>("otel_traces")?
            .with_timeouts(Some(Duration::from_secs(10)), Some(Duration::from_secs(10)))
            .with_max_bytes(50_000_000)
            .with_max_rows(1000);

        for row in &rows {
            if let Err(e) = inserter.write(row) {
                error!("Failed to write OTel trace row to ClickHouse: {e}");
            }
        }

        inserter.end().await?;
        Ok(())
    }
}

impl super::OTelExporter for ClickHouseExporter {
    fn export_otel_logs(
        &self,
        logs: Vec<OTelLog>,
    ) -> std::pin::Pin<Box<dyn std::future::Future<Output = Result<(), AggregatorError>> + Send + '_>>
    {
        Box::pin(self.export_otel_logs(logs))
    }

    fn export_otel_traces(
        &self,
        traces: Vec<OTelTrace>,
    ) -> std::pin::Pin<Box<dyn std::future::Future<Output = Result<(), AggregatorError>> + Send + '_>>
    {
        Box::pin(self.export_otel_traces(traces))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::{SpanEvent, SpanKind, SpanLink, StatusCode as DomainStatusCode};

    // =========================================================================
    // string_to_fixed_bytes tests
    // =========================================================================

    #[test]
    fn test_string_to_fixed_bytes_empty_string() {
        let result: [u8; 8] = string_to_fixed_bytes("");
        assert_eq!(result, [0u8; 8]);
    }

    #[test]
    fn test_string_to_fixed_bytes_shorter_than_n() {
        let result: [u8; 8] = string_to_fixed_bytes("abc");
        assert_eq!(result, [b'a', b'b', b'c', 0, 0, 0, 0, 0]);
    }

    #[test]
    fn test_string_to_fixed_bytes_exact_n_length() {
        let result: [u8; 4] = string_to_fixed_bytes("test");
        assert_eq!(result, [b't', b'e', b's', b't']);
    }

    #[test]
    fn test_string_to_fixed_bytes_longer_than_n_truncates() {
        let result: [u8; 4] = string_to_fixed_bytes("hello world");
        assert_eq!(result, [b'h', b'e', b'l', b'l']);
    }

    #[test]
    fn test_string_to_fixed_bytes_utf8_multibyte_boundary() {
        // "あ" is 3 bytes in UTF-8: [0xe3, 0x81, 0x82]
        let result: [u8; 4] = string_to_fixed_bytes("あ");
        assert_eq!(result, [0xe3, 0x81, 0x82, 0]);
    }

    #[test]
    fn test_string_to_fixed_bytes_trace_id_size() {
        // Typical OTel trace ID: 32-char hex string
        let trace_id = "0123456789abcdef0123456789abcdef";
        let result: [u8; 32] = string_to_fixed_bytes(trace_id);
        assert_eq!(&result[..], trace_id.as_bytes());
    }

    #[test]
    fn test_string_to_fixed_bytes_span_id_size() {
        // Typical OTel span ID: 16-char hex string
        let span_id = "0123456789abcdef";
        let result: [u8; 16] = string_to_fixed_bytes(span_id);
        assert_eq!(&result[..], span_id.as_bytes());
    }

    #[test]
    fn test_string_to_fixed_bytes_zero_length_array() {
        let result: [u8; 0] = string_to_fixed_bytes("anything");
        assert_eq!(result.len(), 0);
    }

    // =========================================================================
    // hashmap_to_vec tests
    // =========================================================================

    #[test]
    fn test_hashmap_to_vec_empty() {
        let map: HashMap<String, String> = HashMap::new();
        let result = hashmap_to_vec(map);
        assert!(result.is_empty());
    }

    #[test]
    fn test_hashmap_to_vec_single_entry() {
        let mut map = HashMap::new();
        map.insert("key".to_string(), "value".to_string());
        let result = hashmap_to_vec(map);
        assert_eq!(result.len(), 1);
        assert!(result.contains(&("key".to_string(), "value".to_string())));
    }

    #[test]
    fn test_hashmap_to_vec_multiple_entries() {
        let mut map = HashMap::new();
        map.insert("a".to_string(), "1".to_string());
        map.insert("b".to_string(), "2".to_string());
        map.insert("c".to_string(), "3".to_string());
        let result = hashmap_to_vec(map);
        assert_eq!(result.len(), 3);
        // HashMap order is not deterministic, so we check for presence
        assert!(result.contains(&("a".to_string(), "1".to_string())));
        assert!(result.contains(&("b".to_string(), "2".to_string())));
        assert!(result.contains(&("c".to_string(), "3".to_string())));
    }

    // =========================================================================
    // OTelLogRow::from(OTelLog) tests
    // =========================================================================

    fn create_test_otel_log() -> OTelLog {
        OTelLog {
            timestamp: 1_700_000_000_000_000_000, // nanoseconds
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

    // =========================================================================
    // OTelTraceRow::from(OTelTrace) tests
    // =========================================================================

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
            duration: 1_000_000, // 1ms in nanoseconds
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

    // =========================================================================
    // LogRow::from(EnrichedLogEntry) tests
    // =========================================================================

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
}
