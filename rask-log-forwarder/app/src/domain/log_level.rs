use serde::{Deserialize, Serialize};

/// Domain log level representing the severity of a log entry.
///
/// This is distinct from `TracingLevel` (used for configuring tracing/logging infrastructure).
/// `LogLevel` represents the semantic level parsed from application logs.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub enum LogLevel {
    Debug,
    Info,
    Warn,
    Error,
    Fatal,
}
