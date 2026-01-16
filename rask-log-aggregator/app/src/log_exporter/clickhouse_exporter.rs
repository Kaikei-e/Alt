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
    #[serde(rename = "Events")]
    pub events: String,
    #[serde(rename = "Links")]
    pub links: String,
}

impl From<OTelTrace> for OTelTraceRow {
    fn from(trace: OTelTrace) -> Self {
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
            events: trace.events,
            links: trace.links,
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
