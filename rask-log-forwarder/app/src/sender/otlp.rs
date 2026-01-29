//! OTLP (OpenTelemetry Protocol) serialization for log batches.
//!
//! This module converts EnrichedLogEntry batches to OTLP protobuf format
//! for transmission to OTLP-compatible log receivers.

use crate::buffer::Batch;
use crate::parser::{EnrichedLogEntry, LogLevel};
use crate::sender::SerializationError;

use opentelemetry_proto::tonic::collector::logs::v1::ExportLogsServiceRequest;
use opentelemetry_proto::tonic::common::v1::{any_value, AnyValue, KeyValue};
use opentelemetry_proto::tonic::logs::v1::{LogRecord, ResourceLogs, ScopeLogs};
use opentelemetry_proto::tonic::resource::v1::Resource;

use prost::Message;
use std::collections::HashMap;

/// Instrumentation scope name for rask-log-forwarder
const SCOPE_NAME: &str = "rask-log-forwarder";

/// OTLP serializer that converts EnrichedLogEntry batches to protobuf format.
#[derive(Clone)]
pub struct OtlpSerializer {
    forwarder_version: String,
}

impl OtlpSerializer {
    /// Creates a new OtlpSerializer.
    pub fn new() -> Self {
        Self {
            forwarder_version: env!("CARGO_PKG_VERSION").to_string(),
        }
    }

    /// Serializes a batch of log entries to OTLP protobuf format.
    ///
    /// Returns the protobuf-encoded bytes ready for HTTP transmission.
    pub fn serialize_batch(&self, batch: &Batch) -> Result<Vec<u8>, SerializationError> {
        if batch.is_empty() {
            return Err(SerializationError::EmptyBatch);
        }

        // Group entries by service_name to create ResourceLogs
        let grouped = self.group_by_service(batch.entries());

        // Convert to OTLP format
        let resource_logs: Vec<ResourceLogs> = grouped
            .into_iter()
            .map(|(service_name, entries)| self.create_resource_logs(&service_name, entries))
            .collect();

        let request = ExportLogsServiceRequest { resource_logs };

        // Encode to protobuf bytes
        let mut buf = Vec::with_capacity(request.encoded_len());
        request
            .encode(&mut buf)
            .map_err(|e| SerializationError::IoError(std::io::Error::other(e)))?;

        Ok(buf)
    }

    /// Groups log entries by service name.
    fn group_by_service<'a>(
        &self,
        entries: &'a [EnrichedLogEntry],
    ) -> HashMap<String, Vec<&'a EnrichedLogEntry>> {
        let mut grouped: HashMap<String, Vec<&EnrichedLogEntry>> = HashMap::new();

        for entry in entries {
            grouped
                .entry(entry.service_name.clone())
                .or_default()
                .push(entry);
        }

        grouped
    }

    /// Creates a ResourceLogs message for a group of entries from the same service.
    fn create_resource_logs(&self, service_name: &str, entries: Vec<&EnrichedLogEntry>) -> ResourceLogs {
        // Get container_id and service_group from first entry (all entries in group have same service)
        let first_entry = entries.first().expect("entries should not be empty");

        // Create resource attributes
        let mut resource_attributes = vec![
            self.create_string_kv("service.name", service_name),
            self.create_string_kv("container.id", &first_entry.container_id),
            self.create_string_kv("telemetry.sdk.name", "rask-log-forwarder"),
            self.create_string_kv("telemetry.sdk.version", &self.forwarder_version),
        ];

        if let Some(ref group) = first_entry.service_group {
            resource_attributes.push(self.create_string_kv("service.namespace", group));
        }

        let resource = Resource {
            attributes: resource_attributes,
            dropped_attributes_count: 0,
            entity_refs: vec![],
        };

        // Create log records
        let log_records: Vec<LogRecord> = entries
            .into_iter()
            .map(|entry| self.create_log_record(entry))
            .collect();

        // Create scope logs
        let scope_logs = vec![ScopeLogs {
            scope: Some(opentelemetry_proto::tonic::common::v1::InstrumentationScope {
                name: SCOPE_NAME.to_string(),
                version: self.forwarder_version.clone(),
                attributes: vec![],
                dropped_attributes_count: 0,
            }),
            log_records,
            schema_url: String::new(),
        }];

        ResourceLogs {
            resource: Some(resource),
            scope_logs,
            schema_url: String::new(),
        }
    }

    /// Creates an OTLP LogRecord from an EnrichedLogEntry.
    fn create_log_record(&self, entry: &EnrichedLogEntry) -> LogRecord {
        // Parse timestamp to nanoseconds
        let time_unix_nano = self.parse_timestamp_to_nanos(&entry.timestamp);
        let observed_time_unix_nano = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map(|d| d.as_nanos() as u64)
            .unwrap_or(time_unix_nano);

        // Map log level to severity number
        let (severity_number, severity_text) = self.map_log_level(entry.level.clone());

        // Create body
        let body = Some(AnyValue {
            value: Some(any_value::Value::StringValue(entry.message.clone())),
        });

        // Create log attributes
        let mut attributes = self.create_log_attributes(entry);

        // Add extra fields
        for (key, value) in &entry.fields {
            attributes.push(self.create_string_kv(key, value));
        }

        // Parse trace context
        let trace_id = entry
            .trace_id
            .as_ref()
            .and_then(|id| hex::decode(id).ok())
            .unwrap_or_default();

        let span_id = entry
            .span_id
            .as_ref()
            .and_then(|id| hex::decode(id).ok())
            .unwrap_or_default();

        LogRecord {
            time_unix_nano,
            observed_time_unix_nano,
            severity_number: severity_number as i32,
            severity_text,
            body,
            attributes,
            dropped_attributes_count: 0,
            flags: 0,
            trace_id,
            span_id,
            event_name: String::new(),
        }
    }

    /// Creates log attributes from an EnrichedLogEntry.
    fn create_log_attributes(&self, entry: &EnrichedLogEntry) -> Vec<KeyValue> {
        let mut attrs = vec![
            self.create_string_kv("log.type", &entry.log_type),
            self.create_string_kv("stream", &entry.stream),
        ];

        // HTTP attributes (OTel semantic conventions)
        if let Some(ref method) = entry.method {
            attrs.push(self.create_string_kv("http.method", method));
        }
        if let Some(ref path) = entry.path {
            attrs.push(self.create_string_kv("http.target", path));
        }
        if let Some(status_code) = entry.status_code {
            attrs.push(self.create_int_kv("http.status_code", status_code as i64));
        }
        if let Some(response_size) = entry.response_size {
            attrs.push(self.create_int_kv("http.response_content_length", response_size as i64));
        }
        if let Some(ref ip) = entry.ip_address {
            attrs.push(self.create_string_kv("net.peer.ip", ip));
        }
        if let Some(ref ua) = entry.user_agent {
            attrs.push(self.create_string_kv("http.user_agent", ua));
        }

        attrs
    }

    /// Creates a string-valued KeyValue.
    fn create_string_kv(&self, key: &str, value: &str) -> KeyValue {
        KeyValue {
            key: key.to_string(),
            value: Some(AnyValue {
                value: Some(any_value::Value::StringValue(value.to_string())),
            }),
        }
    }

    /// Creates an integer-valued KeyValue.
    fn create_int_kv(&self, key: &str, value: i64) -> KeyValue {
        KeyValue {
            key: key.to_string(),
            value: Some(AnyValue {
                value: Some(any_value::Value::IntValue(value)),
            }),
        }
    }

    /// Maps LogLevel to OTLP severity number and text.
    ///
    /// OTel severity numbers:
    /// - 1-4: TRACE
    /// - 5-8: DEBUG
    /// - 9-12: INFO
    /// - 13-16: WARN
    /// - 17-20: ERROR
    /// - 21-24: FATAL
    fn map_log_level(&self, level: Option<LogLevel>) -> (u8, String) {
        match level {
            Some(LogLevel::Debug) => (5, "DEBUG".to_string()),
            Some(LogLevel::Info) => (9, "INFO".to_string()),
            Some(LogLevel::Warn) => (13, "WARN".to_string()),
            Some(LogLevel::Error) => (17, "ERROR".to_string()),
            Some(LogLevel::Fatal) => (21, "FATAL".to_string()),
            None => (0, "UNSPECIFIED".to_string()),
        }
    }

    /// Parses an RFC3339 timestamp string to nanoseconds since Unix epoch.
    fn parse_timestamp_to_nanos(&self, timestamp: &str) -> u64 {
        chrono::DateTime::parse_from_rfc3339(timestamp)
            .map(|dt| dt.timestamp_nanos_opt().unwrap_or(0) as u64)
            .unwrap_or_else(|_| {
                // Fallback: try parsing without timezone
                chrono::NaiveDateTime::parse_from_str(timestamp, "%Y-%m-%dT%H:%M:%S%.f")
                    .map(|dt| dt.and_utc().timestamp_nanos_opt().unwrap_or(0) as u64)
                    .unwrap_or(0)
            })
    }
}

impl Default for OtlpSerializer {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

    fn create_test_entry() -> EnrichedLogEntry {
        EnrichedLogEntry {
            service_type: "test".to_string(),
            log_type: "test".to_string(),
            message: "Test message".to_string(),
            level: Some(LogLevel::Info),
            timestamp: "2024-01-01T00:00:00.000Z".to_string(),
            stream: "stdout".to_string(),
            method: None,
            path: None,
            status_code: None,
            response_size: None,
            ip_address: None,
            user_agent: None,
            container_id: "test123".to_string(),
            service_name: "test-service".to_string(),
            service_group: None,
            trace_id: None,
            span_id: None,
            fields: HashMap::new(),
        }
    }

    #[test]
    fn test_log_level_mapping() {
        let serializer = OtlpSerializer::new();

        assert_eq!(serializer.map_log_level(Some(LogLevel::Debug)), (5, "DEBUG".to_string()));
        assert_eq!(serializer.map_log_level(Some(LogLevel::Info)), (9, "INFO".to_string()));
        assert_eq!(serializer.map_log_level(Some(LogLevel::Warn)), (13, "WARN".to_string()));
        assert_eq!(serializer.map_log_level(Some(LogLevel::Error)), (17, "ERROR".to_string()));
        assert_eq!(serializer.map_log_level(Some(LogLevel::Fatal)), (21, "FATAL".to_string()));
        assert_eq!(serializer.map_log_level(None), (0, "UNSPECIFIED".to_string()));
    }

    #[test]
    fn test_timestamp_parsing() {
        let serializer = OtlpSerializer::new();

        // RFC3339 with Z suffix
        let nanos = serializer.parse_timestamp_to_nanos("2024-01-01T00:00:00.000Z");
        assert!(nanos > 0);

        // RFC3339 with offset
        let nanos2 = serializer.parse_timestamp_to_nanos("2024-01-01T00:00:00.000+00:00");
        assert_eq!(nanos, nanos2);
    }

    #[test]
    fn test_group_by_service() {
        let serializer = OtlpSerializer::new();

        let mut entry1 = create_test_entry();
        entry1.service_name = "service-a".to_string();

        let mut entry2 = create_test_entry();
        entry2.service_name = "service-b".to_string();

        let mut entry3 = create_test_entry();
        entry3.service_name = "service-a".to_string();

        let entries = vec![entry1, entry2, entry3];
        let grouped = serializer.group_by_service(&entries);

        assert_eq!(grouped.len(), 2);
        assert_eq!(grouped.get("service-a").map(|v| v.len()), Some(2));
        assert_eq!(grouped.get("service-b").map(|v| v.len()), Some(1));
    }
}
