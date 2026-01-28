//! Meilisearch log parser with ANSI escape code stripping.

use super::{LogLevel, ParsedLogEntry, ServiceParser};
use crate::parser::docker::ParseError;

/// Parser for Meilisearch logs.
/// Strips ANSI escape codes from log messages.
pub struct MeilisearchParser;

impl Default for MeilisearchParser {
    fn default() -> Self {
        Self::new()
    }
}

impl MeilisearchParser {
    pub fn new() -> Self {
        Self
    }

    /// Strip ANSI escape codes from log messages.
    /// Handles sequences like: \x1b[2m, \x1b[0m, \x1b[32m, etc.
    pub fn strip_ansi(input: &str) -> String {
        let mut result = String::with_capacity(input.len());
        let mut chars = input.chars().peekable();

        while let Some(c) = chars.next() {
            if c == '\x1b' {
                // Skip the escape sequence
                if chars.peek() == Some(&'[') {
                    chars.next(); // consume '['
                    // Skip until we hit a letter (end of escape sequence)
                    while let Some(&next_c) = chars.peek() {
                        chars.next();
                        if next_c.is_ascii_alphabetic() {
                            break;
                        }
                    }
                }
            } else {
                result.push(c);
            }
        }

        result
    }

    /// Extract log level from meilisearch format
    fn extract_level(log: &str) -> LogLevel {
        if log.contains(" ERROR ") || log.contains("[ERROR]") {
            LogLevel::Error
        } else if log.contains(" WARN ") || log.contains("[WARN]") {
            LogLevel::Warn
        } else if log.contains(" DEBUG ") || log.contains("[DEBUG]") {
            LogLevel::Debug
        } else {
            LogLevel::Info
        }
    }
}

impl ServiceParser for MeilisearchParser {
    fn service_type(&self) -> &str {
        "meilisearch"
    }

    fn parse_log(&self, log: &str) -> Result<ParsedLogEntry, ParseError> {
        // Strip ANSI escape codes first
        let cleaned_log = Self::strip_ansi(log);
        let level = Self::extract_level(&cleaned_log);

        Ok(ParsedLogEntry {
            service_type: "meilisearch".to_string(),
            log_type: "search".to_string(),
            message: cleaned_log,
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
        })
    }

    fn detection_priority(&self) -> u8 {
        // Medium priority - specific to meilisearch
        65
    }

    fn can_parse(&self, log: &str) -> bool {
        // Meilisearch logs often contain ANSI codes or specific patterns
        log.contains("meilisearch")
            || log.contains("HTTP request{")
            || (log.contains("\x1b[")
                && (log.contains("INFO") || log.contains("WARN") || log.contains("ERROR")))
    }
}
