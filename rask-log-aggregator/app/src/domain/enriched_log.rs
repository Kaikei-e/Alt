use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Serialize, Deserialize, Clone, Debug)]
pub struct EnrichedLogEntry {
    pub service_type: String,
    pub log_type: String,
    pub message: String,
    pub level: Option<LogLevel>,
    pub timestamp: String,
    pub stream: String,
    pub container_id: String,
    pub service_name: String,
    pub service_group: Option<String>,
    pub fields: HashMap<String, String>,

    // HTTP-specific fields (populated by rask-log-forwarder for HTTP services)
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub method: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub path: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub status_code: Option<u16>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub response_size: Option<u64>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub ip_address: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub user_agent: Option<String>,

    // Trace context (OpenTelemetry)
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub trace_id: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub span_id: Option<String>,
}

#[derive(Serialize, Deserialize, Clone, Debug)]
pub enum LogLevel {
    Debug,
    Info,
    Warn,
    Error,
    Fatal,
}
