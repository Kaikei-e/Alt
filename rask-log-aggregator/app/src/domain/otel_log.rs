//! OpenTelemetry domain models for ClickHouse storage
//!
//! These models represent the OpenTelemetry Log and Trace data models
//! in a format suitable for storage in ClickHouse.

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// OpenTelemetry Log Record
///
/// Represents a single log entry following the OTel Log Data Model.
/// See: https://opentelemetry.io/docs/specs/otel/logs/data-model/
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OTelLog {
    /// Timestamp when the event occurred (nanoseconds since Unix epoch)
    pub timestamp: u64,

    /// Timestamp when the event was observed (nanoseconds since Unix epoch)
    pub observed_timestamp: u64,

    /// Trace ID (32-char hex string)
    pub trace_id: String,

    /// Span ID (16-char hex string)
    pub span_id: String,

    /// Trace flags (W3C Trace Context)
    pub trace_flags: u8,

    /// Severity text (e.g., "INFO", "ERROR")
    pub severity_text: String,

    /// Severity number (1-24, see OTel spec)
    pub severity_number: u8,

    /// Log body (message)
    pub body: String,

    /// Resource schema URL
    pub resource_schema_url: String,

    /// Resource attributes (service.name, service.version, etc.)
    pub resource_attributes: HashMap<String, String>,

    /// Scope schema URL
    pub scope_schema_url: String,

    /// Instrumentation scope name
    pub scope_name: String,

    /// Instrumentation scope version
    pub scope_version: String,

    /// Scope attributes
    pub scope_attributes: HashMap<String, String>,

    /// Log attributes (event-specific key-value pairs)
    pub log_attributes: HashMap<String, String>,

    /// Service name (extracted from resource attributes for convenience)
    pub service_name: String,
}

/// Span Event for nested array storage
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SpanEvent {
    /// Event timestamp (nanoseconds since Unix epoch)
    pub timestamp: u64,
    /// Event name
    pub name: String,
    /// Event attributes
    pub attributes: HashMap<String, String>,
}

/// Span Link for nested array storage
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SpanLink {
    /// Linked trace ID
    pub trace_id: String,
    /// Linked span ID
    pub span_id: String,
    /// Trace state
    pub trace_state: String,
    /// Link attributes
    pub attributes: HashMap<String, String>,
}

/// OpenTelemetry Trace Span
///
/// Represents a single span following the OTel Trace Data Model.
/// See: https://opentelemetry.io/docs/specs/otel/trace/api/#span
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OTelTrace {
    /// Start timestamp (nanoseconds since Unix epoch)
    pub timestamp: u64,

    /// Trace ID (32-char hex string)
    pub trace_id: String,

    /// Span ID (16-char hex string)
    pub span_id: String,

    /// Parent Span ID (empty if root span)
    pub parent_span_id: String,

    /// W3C Trace State
    pub trace_state: String,

    /// Span name/operation name
    pub span_name: String,

    /// Span kind (SERVER, CLIENT, etc.)
    pub span_kind: SpanKind,

    /// Service name
    pub service_name: String,

    /// Resource attributes
    pub resource_attributes: HashMap<String, String>,

    /// Span attributes
    pub span_attributes: HashMap<String, String>,

    /// Duration in nanoseconds
    pub duration: i64,

    /// Status code
    pub status_code: StatusCode,

    /// Status message (for error status)
    pub status_message: String,

    /// Span events (JSON serialized) - kept for backward compatibility
    pub events: String,

    /// Span links (JSON serialized) - kept for backward compatibility
    pub links: String,

    /// Span events (nested array for Grafana compatibility)
    pub events_nested: Vec<SpanEvent>,

    /// Span links (nested array for Grafana compatibility)
    pub links_nested: Vec<SpanLink>,
}

/// OpenTelemetry Span Kind
#[derive(Debug, Clone, Copy, Serialize, Deserialize, Default, PartialEq, Eq)]
#[repr(i8)]
pub enum SpanKind {
    #[default]
    Unspecified = 0,
    Internal = 1,
    Server = 2,
    Client = 3,
    Producer = 4,
    Consumer = 5,
}

impl From<i32> for SpanKind {
    fn from(value: i32) -> Self {
        match value {
            1 => SpanKind::Internal,
            2 => SpanKind::Server,
            3 => SpanKind::Client,
            4 => SpanKind::Producer,
            5 => SpanKind::Consumer,
            _ => SpanKind::Unspecified,
        }
    }
}

/// OpenTelemetry Status Code
#[derive(Debug, Clone, Copy, Serialize, Deserialize, Default, PartialEq, Eq)]
#[repr(i8)]
pub enum StatusCode {
    #[default]
    Unset = 0,
    Ok = 1,
    Error = 2,
}

impl From<i32> for StatusCode {
    fn from(value: i32) -> Self {
        match value {
            1 => StatusCode::Ok,
            2 => StatusCode::Error,
            _ => StatusCode::Unset,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_span_kind_from_i32() {
        assert_eq!(SpanKind::from(0), SpanKind::Unspecified);
        assert_eq!(SpanKind::from(1), SpanKind::Internal);
        assert_eq!(SpanKind::from(2), SpanKind::Server);
        assert_eq!(SpanKind::from(3), SpanKind::Client);
        assert_eq!(SpanKind::from(4), SpanKind::Producer);
        assert_eq!(SpanKind::from(5), SpanKind::Consumer);
        assert_eq!(SpanKind::from(99), SpanKind::Unspecified);
    }

    #[test]
    fn test_status_code_from_i32() {
        assert_eq!(StatusCode::from(0), StatusCode::Unset);
        assert_eq!(StatusCode::from(1), StatusCode::Ok);
        assert_eq!(StatusCode::from(2), StatusCode::Error);
        assert_eq!(StatusCode::from(99), StatusCode::Unset);
    }
}
