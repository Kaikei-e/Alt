//! Converter from OpenTelemetry protocol to internal domain models

use std::collections::HashMap;

use opentelemetry_proto::tonic::{
    collector::{
        logs::v1::ExportLogsServiceRequest,
        trace::v1::ExportTraceServiceRequest,
    },
    common::v1::{any_value, AnyValue, KeyValue},
    logs::v1::LogRecord,
};

use crate::domain::{OTelLog, OTelTrace, SpanKind, StatusCode};

/// Convert OTLP log request to internal log structures
pub fn convert_log_records(request: &ExportLogsServiceRequest) -> Vec<OTelLog> {
    let mut logs = Vec::new();

    for resource_logs in &request.resource_logs {
        let resource = resource_logs.resource.as_ref();
        let resource_attrs = resource
            .map(|r| convert_attributes(&r.attributes))
            .unwrap_or_default();
        let resource_schema_url = resource_logs.schema_url.clone();

        for scope_logs in &resource_logs.scope_logs {
            let scope = scope_logs.scope.as_ref();
            let scope_name = scope.map(|s| s.name.clone()).unwrap_or_default();
            let scope_version = scope.map(|s| s.version.clone()).unwrap_or_default();
            let scope_attrs = scope
                .map(|s| convert_attributes(&s.attributes))
                .unwrap_or_default();
            let scope_schema_url = scope_logs.schema_url.clone();

            for log_record in &scope_logs.log_records {
                let log = convert_single_log(
                    log_record,
                    &resource_attrs,
                    &resource_schema_url,
                    &scope_name,
                    &scope_version,
                    &scope_attrs,
                    &scope_schema_url,
                );
                logs.push(log);
            }
        }
    }

    logs
}

fn convert_single_log(
    record: &LogRecord,
    resource_attrs: &HashMap<String, String>,
    resource_schema_url: &str,
    scope_name: &str,
    scope_version: &str,
    scope_attrs: &HashMap<String, String>,
    scope_schema_url: &str,
) -> OTelLog {
    let service_name = resource_attrs
        .get("service.name")
        .cloned()
        .unwrap_or_else(|| "unknown".to_string());

    OTelLog {
        timestamp: record.time_unix_nano,
        observed_timestamp: record.observed_time_unix_nano,
        trace_id: encode_trace_id(&record.trace_id),
        span_id: encode_span_id(&record.span_id),
        trace_flags: record.flags as u8,
        severity_text: record.severity_text.clone(),
        severity_number: record.severity_number as u8,
        body: extract_body(&record.body),
        resource_schema_url: resource_schema_url.to_string(),
        resource_attributes: resource_attrs.clone(),
        scope_schema_url: scope_schema_url.to_string(),
        scope_name: scope_name.to_string(),
        scope_version: scope_version.to_string(),
        scope_attributes: scope_attrs.clone(),
        log_attributes: convert_attributes(&record.attributes),
        service_name,
    }
}

/// Convert OTLP trace request to internal trace structures
pub fn convert_spans(request: &ExportTraceServiceRequest) -> Vec<OTelTrace> {
    let mut traces = Vec::new();

    for resource_spans in &request.resource_spans {
        let resource = resource_spans.resource.as_ref();
        let resource_attrs = resource
            .map(|r| convert_attributes(&r.attributes))
            .unwrap_or_default();

        let service_name = resource_attrs
            .get("service.name")
            .cloned()
            .unwrap_or_else(|| "unknown".to_string());

        for scope_spans in &resource_spans.scope_spans {
            for span in &scope_spans.spans {
                let trace = convert_single_span(span, &resource_attrs, &service_name);
                traces.push(trace);
            }
        }
    }

    traces
}

fn convert_single_span(
    span: &opentelemetry_proto::tonic::trace::v1::Span,
    resource_attrs: &HashMap<String, String>,
    service_name: &str,
) -> OTelTrace {
    OTelTrace {
        timestamp: span.start_time_unix_nano,
        trace_id: encode_trace_id(&span.trace_id),
        span_id: encode_span_id(&span.span_id),
        parent_span_id: encode_span_id(&span.parent_span_id),
        trace_state: span.trace_state.clone(),
        span_name: span.name.clone(),
        span_kind: SpanKind::from(span.kind),
        service_name: service_name.to_string(),
        resource_attributes: resource_attrs.clone(),
        span_attributes: convert_attributes(&span.attributes),
        duration: (span.end_time_unix_nano.saturating_sub(span.start_time_unix_nano)) as i64,
        status_code: span
            .status
            .as_ref()
            .map(|s| StatusCode::from(s.code))
            .unwrap_or(StatusCode::Unset),
        status_message: span
            .status
            .as_ref()
            .map(|s| s.message.clone())
            .unwrap_or_default(),
        events: serde_json::to_string(&span.events).unwrap_or_else(|_| "[]".to_string()),
        links: serde_json::to_string(&span.links).unwrap_or_else(|_| "[]".to_string()),
    }
}

/// Convert OTLP attributes to HashMap
fn convert_attributes(attrs: &[KeyValue]) -> HashMap<String, String> {
    attrs
        .iter()
        .filter_map(|kv| {
            let value = kv.value.as_ref()?;
            let string_value = extract_string_value(value)?;
            Some((kv.key.clone(), string_value))
        })
        .collect()
}

fn extract_string_value(value: &AnyValue) -> Option<String> {
    match &value.value {
        Some(any_value::Value::StringValue(s)) => Some(s.clone()),
        Some(any_value::Value::IntValue(i)) => Some(i.to_string()),
        Some(any_value::Value::DoubleValue(d)) => Some(d.to_string()),
        Some(any_value::Value::BoolValue(b)) => Some(b.to_string()),
        Some(any_value::Value::BytesValue(b)) => Some(hex::encode(b)),
        Some(any_value::Value::ArrayValue(arr)) => {
            let items: Vec<String> = arr
                .values
                .iter()
                .filter_map(extract_string_value)
                .collect();
            Some(format!("[{}]", items.join(", ")))
        }
        Some(any_value::Value::KvlistValue(kv)) => {
            let items: Vec<String> = kv
                .values
                .iter()
                .filter_map(|kv| {
                    let val = kv.value.as_ref().and_then(extract_string_value)?;
                    Some(format!("{}={}", kv.key, val))
                })
                .collect();
            Some(format!("{{{}}}", items.join(", ")))
        }
        None => None,
    }
}

fn extract_body(body: &Option<AnyValue>) -> String {
    body.as_ref()
        .and_then(extract_string_value)
        .unwrap_or_default()
}

/// Encode trace_id bytes to 32-char hex string
fn encode_trace_id(bytes: &[u8]) -> String {
    if bytes.is_empty() || bytes.iter().all(|&b| b == 0) {
        return "0".repeat(32);
    }
    // Ensure exactly 16 bytes (128 bits) -> 32 hex chars
    let mut padded = vec![0u8; 16];
    let start = 16_usize.saturating_sub(bytes.len());
    let copy_len = bytes.len().min(16);
    padded[start..start + copy_len].copy_from_slice(&bytes[..copy_len]);
    hex::encode(padded)
}

/// Encode span_id bytes to 16-char hex string
fn encode_span_id(bytes: &[u8]) -> String {
    if bytes.is_empty() || bytes.iter().all(|&b| b == 0) {
        return "0".repeat(16);
    }
    // Ensure exactly 8 bytes (64 bits) -> 16 hex chars
    let mut padded = vec![0u8; 8];
    let start = 8_usize.saturating_sub(bytes.len());
    let copy_len = bytes.len().min(8);
    padded[start..start + copy_len].copy_from_slice(&bytes[..copy_len]);
    hex::encode(padded)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_encode_trace_id() {
        let bytes = vec![
            0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
            0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
        ];
        assert_eq!(
            encode_trace_id(&bytes),
            "0102030405060708090a0b0c0d0e0f10"
        );
    }

    #[test]
    fn test_encode_empty_trace_id() {
        assert_eq!(encode_trace_id(&[]), "00000000000000000000000000000000");
        assert_eq!(
            encode_trace_id(&[0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]),
            "00000000000000000000000000000000"
        );
    }

    #[test]
    fn test_encode_span_id() {
        let bytes = vec![0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08];
        assert_eq!(encode_span_id(&bytes), "0102030405060708");
    }

    #[test]
    fn test_encode_empty_span_id() {
        assert_eq!(encode_span_id(&[]), "0000000000000000");
    }

    #[test]
    fn test_convert_attributes() {
        let attrs = vec![
            KeyValue {
                key: "string_key".to_string(),
                value: Some(AnyValue {
                    value: Some(any_value::Value::StringValue("hello".to_string())),
                }),
            },
            KeyValue {
                key: "int_key".to_string(),
                value: Some(AnyValue {
                    value: Some(any_value::Value::IntValue(42)),
                }),
            },
            KeyValue {
                key: "bool_key".to_string(),
                value: Some(AnyValue {
                    value: Some(any_value::Value::BoolValue(true)),
                }),
            },
        ];

        let result = convert_attributes(&attrs);
        assert_eq!(result.get("string_key"), Some(&"hello".to_string()));
        assert_eq!(result.get("int_key"), Some(&"42".to_string()));
        assert_eq!(result.get("bool_key"), Some(&"true".to_string()));
    }
}
