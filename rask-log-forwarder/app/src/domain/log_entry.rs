use super::log_level::LogLevel;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// A fully parsed and enriched log entry ready for batching and transmission.
///
/// This is the canonical representation of a log entry throughout the pipeline,
/// from parser output through to sender input.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct EnrichedLogEntry {
    // From parsed log
    pub service_type: String,
    pub log_type: String,
    pub message: String,
    pub level: Option<LogLevel>,
    pub timestamp: String, // From Docker log
    pub stream: String,

    // HTTP fields
    pub method: Option<String>,
    pub path: Option<String>,
    pub status_code: Option<u16>,
    pub response_size: Option<u64>,
    pub ip_address: Option<String>,
    pub user_agent: Option<String>,

    // Container metadata
    pub container_id: String,
    pub service_name: String,
    pub service_group: Option<String>,

    // Trace context (OpenTelemetry)
    // Note: skip_serializing_if is intentionally omitted for bincode compatibility
    #[serde(default)]
    pub trace_id: Option<String>,
    #[serde(default)]
    pub span_id: Option<String>,

    // Additional fields
    pub fields: HashMap<String, String>,
}
