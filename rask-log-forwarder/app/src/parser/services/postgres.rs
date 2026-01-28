//! PostgreSQL log parser.

use super::{LogLevel, ParsedLogEntry, ServiceParser};
use crate::parser::docker::ParseError;
use crate::parser::generated::{pattern_index, VALIDATED_PATTERNS};

/// Parser for PostgreSQL logs.
pub struct PostgresParser;

impl Default for PostgresParser {
    fn default() -> Self {
        Self::new()
    }
}

impl PostgresParser {
    pub fn new() -> Self {
        Self
    }

    /// Parse postgres log with memory-safe patterns
    fn parse_postgres_log(&self, log: &str) -> Result<ParsedLogEntry, ParseError> {
        // Try with the validated postgres log pattern
        match VALIDATED_PATTERNS.get(pattern_index::POSTGRES_LOG) {
            Ok(regex) => {
                if let Some(captures) = regex.captures(log) {
                    let level_str = captures.get(2).map(|m| m.as_str()).unwrap_or("LOG");
                    let level = match level_str {
                        "DEBUG" | "DEBUG1" | "DEBUG2" | "DEBUG3" | "DEBUG4" | "DEBUG5" => {
                            LogLevel::Debug
                        }
                        "LOG" | "INFO" => LogLevel::Info,
                        "NOTICE" | "WARNING" => LogLevel::Warn,
                        "ERROR" => LogLevel::Error,
                        "FATAL" | "PANIC" => LogLevel::Fatal,
                        _ => LogLevel::Info,
                    };

                    let message = captures
                        .get(3)
                        .map(|m| m.as_str())
                        .unwrap_or(log)
                        .to_string();

                    return Ok(ParsedLogEntry {
                        service_type: "postgres".to_string(),
                        log_type: "database".to_string(),
                        message,
                        level: Some(level),
                        timestamp: None,
                        stream: "stdout".to_string(),
                        method: None,
                        path: None,
                        status_code: None,
                        response_size: None,
                        ip_address: None,
                        user_agent: None,
                        fields: std::collections::HashMap::new(),
                    });
                }
            }
            Err(regex_error) => {
                tracing::debug!(
                    "Postgres pattern failed: {}, using simple parsing",
                    regex_error
                );

                // Simple parsing for postgres logs
                let level = if log.contains("ERROR") {
                    LogLevel::Error
                } else if log.contains("WARNING") || log.contains("NOTICE") {
                    LogLevel::Warn
                } else if log.contains("DEBUG") {
                    LogLevel::Debug
                } else {
                    LogLevel::Info
                };

                return Ok(ParsedLogEntry {
                    service_type: "postgres".to_string(),
                    log_type: "database".to_string(),
                    message: log.to_string(),
                    level: Some(level),
                    timestamp: None,
                    stream: "stdout".to_string(),
                    method: None,
                    path: None,
                    status_code: None,
                    response_size: None,
                    ip_address: None,
                    user_agent: None,
                    fields: std::collections::HashMap::new(),
                });
            }
        }

        Err(ParseError::InvalidFormat(
            "Not a valid postgres log".to_string(),
        ))
    }
}

impl ServiceParser for PostgresParser {
    fn service_type(&self) -> &str {
        "postgres"
    }

    fn parse_log(&self, log: &str) -> Result<ParsedLogEntry, ParseError> {
        // Use the memory-safe postgres parser
        self.parse_postgres_log(log).or_else(|_| {
            // Final fallback for unrecognized postgres logs
            Ok(ParsedLogEntry {
                service_type: "postgres".to_string(),
                log_type: "unknown".to_string(),
                message: log.to_string(),
                level: Some(LogLevel::Info),
                timestamp: None,
                stream: "stdout".to_string(),
                method: None,
                path: None,
                status_code: None,
                response_size: None,
                ip_address: None,
                user_agent: None,
                fields: std::collections::HashMap::new(),
            })
        })
    }

    fn detection_priority(&self) -> u8 {
        // Medium-high priority - postgres logs have specific format
        70
    }

    fn can_parse(&self, log: &str) -> bool {
        // Check for PostgreSQL log level indicators
        log.contains("LOG:")
            || log.contains("ERROR:")
            || log.contains("WARNING:")
            || log.contains("NOTICE:")
            || log.contains("DEBUG:")
            || log.contains("FATAL:")
            || log.contains("PANIC:")
            || log.contains("HINT:")
    }
}
