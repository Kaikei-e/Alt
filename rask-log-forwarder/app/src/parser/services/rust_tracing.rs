//! Rust tracing_subscriber fmt().json() log parser.
//!
//! Handles the specific format produced by `tracing_subscriber::fmt::layer().json()`:
//! ```json
//! {"timestamp":"...","level":"INFO","fields":{"message":"...","alt.job.id":"..."}}
//! ```
//! Key difference from Go slog: message and business context fields are nested
//! inside a `"fields"` object rather than at the top level.

use super::{LogLevel, ParsedLogEntry, ServiceParser};
use crate::parser::docker::ParseError;
use serde_json::Value;

/// Parser for Rust tracing_subscriber fmt().json() logs.
pub struct RustTracingParser;

impl Default for RustTracingParser {
    fn default() -> Self {
        Self::new()
    }
}

impl RustTracingParser {
    pub fn new() -> Self {
        Self
    }

    /// Convert JSON value to string without quotes for primitives.
    fn json_value_to_string(value: &Value) -> String {
        match value {
            Value::String(s) => s.clone(),
            Value::Number(n) => n.to_string(),
            Value::Bool(b) => b.to_string(),
            Value::Null => "null".to_string(),
            _ => value.to_string(),
        }
    }
}

impl ServiceParser for RustTracingParser {
    fn service_type(&self) -> &str {
        "rust-tracing"
    }

    fn detection_priority(&self) -> u8 {
        // Higher than Go (60) to ensure Rust tracing format is tried first
        65
    }

    fn can_parse(&self, log: &str) -> bool {
        let trimmed = log.trim();

        if let Some(start) = trimmed.find('{') {
            let json_part = &trimmed[start..];
            // Rust tracing fmt().json() has "fields":{ AND "timestamp" at top level
            // but does NOT have top-level "msg" or "message" (those are inside "fields")
            json_part.contains("\"fields\":{")
                && json_part.contains("\"timestamp\"")
                && !json_part.contains("\"msg\"")
                && serde_json::from_str::<Value>(json_part).is_ok()
        } else {
            false
        }
    }

    fn parse_log(&self, log: &str) -> Result<ParsedLogEntry, ParseError> {
        let trimmed_log = log.trim();

        if let Some(start_brace_pos) = trimmed_log.find('{') {
            let potential_json_slice = &trimmed_log[start_brace_pos..];

            match serde_json::from_str::<Value>(potential_json_slice) {
                Ok(json) => {
                    if let Some(obj) = json.as_object() {
                        // Extract level from top-level "level" field
                        let level_str =
                            obj.get("level").and_then(|v| v.as_str()).unwrap_or("INFO");

                        let level = match level_str {
                            "DEBUG" | "debug" => LogLevel::Debug,
                            "INFO" | "info" => LogLevel::Info,
                            "WARN" | "warn" | "WARNING" | "warning" => LogLevel::Warn,
                            "ERROR" | "error" => LogLevel::Error,
                            "FATAL" | "fatal" => LogLevel::Fatal,
                            _ => LogLevel::Info,
                        };

                        // Extract fields from nested "fields" object
                        let mut message = String::new();
                        let mut fields = std::collections::HashMap::new();

                        if let Some(fields_obj) = obj.get("fields").and_then(|v| v.as_object()) {
                            // Extract message from fields.message
                            message = fields_obj
                                .get("message")
                                .and_then(|v| v.as_str())
                                .unwrap_or("")
                                .to_string();

                            // Flatten all other fields from the "fields" object
                            for (key, value) in fields_obj {
                                if key != "message" {
                                    fields.insert(key.clone(), Self::json_value_to_string(value));
                                }
                            }
                        }

                        // Also collect top-level fields (except level, timestamp, fields, target, span, spans)
                        for (key, value) in obj {
                            if !["level", "timestamp", "fields", "target", "span", "spans"]
                                .contains(&key.as_str())
                            {
                                fields.insert(key.clone(), Self::json_value_to_string(value));
                            }
                        }

                        return Ok(ParsedLogEntry {
                            service_type: "rust-tracing".to_string(),
                            log_type: "structured".to_string(),
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
                            fields,
                        });
                    }
                }
                Err(e) => {
                    tracing::error!("RustTracingParser: Failed to parse JSON: {e:?}");
                }
            }
        }

        // Fallback for non-JSON input
        Ok(ParsedLogEntry {
            service_type: "rust-tracing".to_string(),
            log_type: "plain".to_string(),
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
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_rust_tracing_fmt_json() {
        let parser = RustTracingParser::new();

        let log = r#"{"timestamp":"2026-03-04T10:00:00Z","level":"INFO","fields":{"message":"Processing recap job","alt.job.id":"abc-123","alt.processing.stage":"clustering"}}"#;

        let entry = parser.parse_log(log).unwrap();

        assert_eq!(entry.service_type, "rust-tracing");
        assert_eq!(entry.log_type, "structured");
        assert_eq!(entry.message, "Processing recap job");
        assert_eq!(entry.level, Some(LogLevel::Info));

        // Business context fields should be flattened from fields object
        assert_eq!(
            entry.fields.get("alt.job.id"),
            Some(&"abc-123".to_string())
        );
        assert_eq!(
            entry.fields.get("alt.processing.stage"),
            Some(&"clustering".to_string())
        );
    }

    #[test]
    fn test_parse_fields_flattened_to_entry_fields() {
        let parser = RustTracingParser::new();

        let log = r#"{"timestamp":"2026-03-04T10:00:00Z","level":"INFO","fields":{"message":"test msg","alt.article.id":"art-456","custom_key":"custom_value"}}"#;

        let entry = parser.parse_log(log).unwrap();

        assert_eq!(entry.message, "test msg");
        assert_eq!(
            entry.fields.get("alt.article.id"),
            Some(&"art-456".to_string())
        );
        assert_eq!(
            entry.fields.get("custom_key"),
            Some(&"custom_value".to_string())
        );
        // "message" should NOT be in fields
        assert!(!entry.fields.contains_key("message"));
    }

    #[test]
    fn test_level_extraction() {
        let parser = RustTracingParser::new();

        for (level_str, expected) in [
            ("DEBUG", LogLevel::Debug),
            ("INFO", LogLevel::Info),
            ("WARN", LogLevel::Warn),
            ("ERROR", LogLevel::Error),
        ] {
            let log = format!(
                r#"{{"timestamp":"2026-03-04T10:00:00Z","level":"{}","fields":{{"message":"test"}}}}"#,
                level_str
            );
            let entry = parser.parse_log(&log).unwrap();
            assert_eq!(entry.level, Some(expected), "Failed for level {level_str}");
        }
    }

    #[test]
    fn test_can_parse_distinguishes_from_go_slog() {
        let parser = RustTracingParser::new();

        // Rust tracing fmt().json() — should match
        let rust_log = r#"{"timestamp":"2026-03-04T10:00:00Z","level":"INFO","fields":{"message":"Processing recap job","alt.job.id":"abc-123"}}"#;
        assert!(parser.can_parse(rust_log), "Should detect Rust tracing format");

        // Go slog JSON — should NOT match (has top-level "msg")
        let go_log = r#"{"level":"info","msg":"Processing request","service":"alt-backend"}"#;
        assert!(
            !parser.can_parse(go_log),
            "Should not match Go slog format"
        );

        // Go slog JSON with "message" — should NOT match
        let go_log2 = r#"{"level":"info","message":"Processing request","service":"alt-backend"}"#;
        assert!(
            !parser.can_parse(go_log2),
            "Should not match Go slog with 'message' at top level"
        );
    }

    #[test]
    fn test_non_json_fallback() {
        let parser = RustTracingParser::new();

        let plain = "This is plain text without any JSON";
        let entry = parser.parse_log(plain).unwrap();

        assert_eq!(entry.service_type, "rust-tracing");
        assert_eq!(entry.log_type, "plain");
        assert_eq!(entry.message, plain);
        assert_eq!(entry.level, Some(LogLevel::Info));
    }

    #[test]
    fn test_parse_with_docker_timestamp_prefix() {
        let parser = RustTracingParser::new();

        // Docker may prepend a native timestamp before the JSON
        let log = r#"2026-03-04T10:00:00.123456789Z {"timestamp":"2026-03-04T10:00:00Z","level":"INFO","fields":{"message":"Processing recap job","alt.job.id":"abc-123"}}"#;

        let entry = parser.parse_log(log).unwrap();

        assert_eq!(entry.log_type, "structured");
        assert_eq!(entry.message, "Processing recap job");
        assert_eq!(
            entry.fields.get("alt.job.id"),
            Some(&"abc-123".to_string())
        );
    }

    #[test]
    fn test_parse_with_target_and_span() {
        let parser = RustTracingParser::new();

        // tracing_subscriber can include target and span info
        let log = r#"{"timestamp":"2026-03-04T10:00:00Z","level":"INFO","target":"recap_worker::handler","span":{"name":"process_job"},"spans":[{"name":"process_job"}],"fields":{"message":"Job started","alt.job.id":"job-789"}}"#;

        let entry = parser.parse_log(log).unwrap();

        assert_eq!(entry.message, "Job started");
        assert_eq!(
            entry.fields.get("alt.job.id"),
            Some(&"job-789".to_string())
        );
        // target, span, spans should NOT be in fields
        assert!(!entry.fields.contains_key("target"));
        assert!(!entry.fields.contains_key("span"));
        assert!(!entry.fields.contains_key("spans"));
    }

    #[test]
    fn test_parse_with_numeric_and_bool_fields() {
        let parser = RustTracingParser::new();

        let log = r#"{"timestamp":"2026-03-04T10:00:00Z","level":"INFO","fields":{"message":"Stats","count":42,"enabled":true,"ratio":3.14}}"#;

        let entry = parser.parse_log(log).unwrap();

        assert_eq!(entry.fields.get("count"), Some(&"42".to_string()));
        assert_eq!(entry.fields.get("enabled"), Some(&"true".to_string()));
        assert_eq!(entry.fields.get("ratio"), Some(&"3.14".to_string()));
    }

    #[test]
    fn test_detection_priority_higher_than_go() {
        let rust_parser = RustTracingParser::new();
        let go_parser = super::super::GoStructuredParser::new();

        assert!(
            rust_parser.detection_priority() > go_parser.detection_priority(),
            "RustTracingParser priority ({}) should be higher than GoStructuredParser ({})",
            rust_parser.detection_priority(),
            go_parser.detection_priority()
        );
    }
}
