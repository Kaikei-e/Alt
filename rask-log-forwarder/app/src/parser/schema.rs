use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use thiserror::Error;

#[derive(Error, Debug)]
pub enum ParseError {
    #[error("Invalid JSON format: {0}")]
    InvalidJson(#[from] simd_json::Error),
    #[error("Missing required field: {0}")]
    MissingField(&'static str),
    #[error("Invalid timestamp format: {0}")]
    InvalidTimestamp(#[from] chrono::ParseError),
    #[error("Invalid format: {0}")]
    InvalidFormat(String),
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LogEntry {
    pub message: String,
    pub stream: String,
    pub timestamp: DateTime<Utc>,
    pub service_name: Option<String>,
    pub container_id: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct NginxLogEntry {
    pub service_type: String,
    pub log_type: String, // "access" or "error"
    pub message: String,
    pub stream: String,
    pub timestamp: DateTime<Utc>,
    pub container_id: Option<String>,
    // Nginx-specific fields
    pub ip_address: Option<String>,
    pub method: Option<String>,
    pub path: Option<String>,
    pub status_code: Option<u16>,
    pub response_size: Option<u64>,
    pub user_agent: Option<String>,
    pub level: Option<String>, // For error logs
}