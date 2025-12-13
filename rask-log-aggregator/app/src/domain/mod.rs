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
}

#[derive(Serialize, Deserialize, Clone, Debug)]
pub enum LogLevel {
    Debug,
    Info,
    Warn,
    Error,
    Fatal,
}
