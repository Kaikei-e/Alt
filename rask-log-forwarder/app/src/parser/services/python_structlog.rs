//! Python structlog JSONRenderer() log parser.
//!
//! Handles the specific format produced by `structlog.JSONRenderer()`:
//! ```json
//! {"event":"recap-evaluator started successfully","level":"info","timestamp":"...","service":"recap-evaluator"}
//! ```
//! Key difference from Go slog: message is in the `"event"` key rather than `"msg"`/`"message"`.
//! Services using custom Formatters that rename `event` to `msg` (news-creator, tag-generator)
//! are handled by the Go parser instead.

use super::{LogLevel, ParsedLogEntry, ServiceParser};
use crate::parser::docker::ParseError;
use serde_json::Value;

/// Parser for Python structlog JSONRenderer() logs.
pub struct PythonStructlogParser;

impl Default for PythonStructlogParser {
    fn default() -> Self {
        Self::new()
    }
}

impl PythonStructlogParser {
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

impl ServiceParser for PythonStructlogParser {
    fn service_type(&self) -> &str {
        "python-structlog"
    }

    fn detection_priority(&self) -> u8 {
        // Higher than Go (60), lower than Rust tracing (65)
        63
    }

    fn can_parse(&self, log: &str) -> bool {
        let trimmed = log.trim();

        if let Some(start) = trimmed.find('{') {
            let json_part = &trimmed[start..];
            // Python structlog JSONRenderer() has "event" key at top level
            // but does NOT have "msg" (Go slog) or "fields":{ (Rust tracing)
            json_part.contains("\"event\"")
                && !json_part.contains("\"msg\"")
                && !json_part.contains("\"fields\":{")
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
                        // Extract message from "event" key
                        let message = obj
                            .get("event")
                            .and_then(|v| v.as_str())
                            .unwrap_or("")
                            .to_string();

                        // Extract level
                        let level_str =
                            obj.get("level").and_then(|v| v.as_str()).unwrap_or("info");

                        let level = match level_str {
                            "DEBUG" | "debug" => LogLevel::Debug,
                            "INFO" | "info" => LogLevel::Info,
                            "WARN" | "warn" | "WARNING" | "warning" => LogLevel::Warn,
                            "ERROR" | "error" => LogLevel::Error,
                            "FATAL" | "fatal" | "CRITICAL" | "critical" => LogLevel::Fatal,
                            _ => LogLevel::Info,
                        };

                        // Collect all fields except "event" and "level"
                        let mut fields = std::collections::HashMap::new();
                        for (key, value) in obj {
                            if key != "event" && key != "level" {
                                fields.insert(key.clone(), Self::json_value_to_string(value));
                            }
                        }

                        return Ok(ParsedLogEntry {
                            service_type: "python-structlog".to_string(),
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
                    tracing::error!("PythonStructlogParser: Failed to parse JSON: {e:?}");
                }
            }
        }

        // Fallback for non-JSON input
        Ok(ParsedLogEntry {
            service_type: "python-structlog".to_string(),
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
    fn test_parse_recap_evaluator_real_log() {
        let parser = PythonStructlogParser::new();

        let log = r#"{"event":"recap-evaluator started successfully","alt.ai.pipeline":"recap-evaluation","timestamp":"2026-03-03T17:38:47.998241Z","service":"recap-evaluator","level":"info"}"#;

        let entry = parser.parse_log(log).unwrap();

        assert_eq!(entry.service_type, "python-structlog");
        assert_eq!(entry.log_type, "structured");
        assert_eq!(entry.message, "recap-evaluator started successfully");
        assert_eq!(entry.level, Some(LogLevel::Info));

        assert_eq!(
            entry.fields.get("alt.ai.pipeline"),
            Some(&"recap-evaluation".to_string())
        );
        assert_eq!(
            entry.fields.get("service"),
            Some(&"recap-evaluator".to_string())
        );
        assert_eq!(
            entry.fields.get("timestamp"),
            Some(&"2026-03-03T17:38:47.998241Z".to_string())
        );
        // "event" and "level" should NOT be in fields
        assert!(!entry.fields.contains_key("event"));
        assert!(!entry.fields.contains_key("level"));
    }

    #[test]
    fn test_parse_recap_subworker_real_log() {
        let parser = PythonStructlogParser::new();

        let log = r#"{"event":"Embedding generation completed","alt.ai.pipeline":"recap-subwork","embedding_count":128,"timestamp":"2026-03-03T18:00:00Z","service":"recap-subworker","level":"info"}"#;

        let entry = parser.parse_log(log).unwrap();

        assert_eq!(entry.message, "Embedding generation completed");
        assert_eq!(entry.level, Some(LogLevel::Info));
        assert_eq!(
            entry.fields.get("alt.ai.pipeline"),
            Some(&"recap-subwork".to_string())
        );
        assert_eq!(
            entry.fields.get("embedding_count"),
            Some(&"128".to_string())
        );
    }

    #[test]
    fn test_level_extraction() {
        let parser = PythonStructlogParser::new();

        for (level_str, expected) in [
            ("debug", LogLevel::Debug),
            ("info", LogLevel::Info),
            ("warning", LogLevel::Warn),
            ("error", LogLevel::Error),
            ("critical", LogLevel::Fatal),
        ] {
            let log = format!(
                r#"{{"event":"test","level":"{}","timestamp":"2026-03-03T10:00:00Z"}}"#,
                level_str
            );
            let entry = parser.parse_log(&log).unwrap();
            assert_eq!(
                entry.level,
                Some(expected),
                "Failed for level {level_str}"
            );
        }
    }

    #[test]
    fn test_can_parse_distinguishes_from_go_slog() {
        let parser = PythonStructlogParser::new();

        // Python structlog JSONRenderer() — should match
        let python_log = r#"{"event":"recap-evaluator started","level":"info","timestamp":"2026-03-03T17:38:47Z","service":"recap-evaluator"}"#;
        assert!(
            parser.can_parse(python_log),
            "Should detect Python structlog format"
        );

        // Go slog JSON — should NOT match (has "msg")
        let go_log = r#"{"level":"info","msg":"Processing request","service":"alt-backend"}"#;
        assert!(
            !parser.can_parse(go_log),
            "Should not match Go slog format"
        );
    }

    #[test]
    fn test_can_parse_distinguishes_from_rust_tracing() {
        let parser = PythonStructlogParser::new();

        // Rust tracing fmt().json() — should NOT match (has "fields":{)
        let rust_log = r#"{"timestamp":"2026-03-04T10:00:00Z","level":"INFO","fields":{"message":"Processing recap job","alt.job.id":"abc-123"}}"#;
        assert!(
            !parser.can_parse(rust_log),
            "Should not match Rust tracing format"
        );
    }

    #[test]
    fn test_non_json_fallback() {
        let parser = PythonStructlogParser::new();

        let plain = "This is plain text without any JSON";
        let entry = parser.parse_log(plain).unwrap();

        assert_eq!(entry.service_type, "python-structlog");
        assert_eq!(entry.log_type, "plain");
        assert_eq!(entry.message, plain);
        assert_eq!(entry.level, Some(LogLevel::Info));
    }

    #[test]
    fn test_parse_with_docker_timestamp_prefix() {
        let parser = PythonStructlogParser::new();

        let log = r#"2026-03-03T17:38:47.998241Z {"event":"recap-evaluator started","level":"info","service":"recap-evaluator"}"#;

        let entry = parser.parse_log(log).unwrap();

        assert_eq!(entry.log_type, "structured");
        assert_eq!(entry.message, "recap-evaluator started");
        assert_eq!(
            entry.fields.get("service"),
            Some(&"recap-evaluator".to_string())
        );
    }

    #[test]
    fn test_parse_with_numeric_and_bool_fields() {
        let parser = PythonStructlogParser::new();

        let log = r#"{"event":"Stats","level":"info","count":42,"enabled":true,"ratio":3.14}"#;

        let entry = parser.parse_log(log).unwrap();

        assert_eq!(entry.fields.get("count"), Some(&"42".to_string()));
        assert_eq!(entry.fields.get("enabled"), Some(&"true".to_string()));
        assert_eq!(entry.fields.get("ratio"), Some(&"3.14".to_string()));
    }

    #[test]
    fn test_parse_with_null_field() {
        let parser = PythonStructlogParser::new();

        let log = r#"{"event":"test","level":"info","taskName":null}"#;

        let entry = parser.parse_log(log).unwrap();

        assert_eq!(entry.fields.get("taskName"), Some(&"null".to_string()));
    }

    #[test]
    fn test_detection_priority_between_go_and_rust() {
        let python_parser = PythonStructlogParser::new();
        let go_parser = super::super::GoStructuredParser::new();
        let rust_parser = super::super::RustTracingParser::new();

        assert!(
            python_parser.detection_priority() > go_parser.detection_priority(),
            "PythonStructlogParser priority ({}) should be higher than GoStructuredParser ({})",
            python_parser.detection_priority(),
            go_parser.detection_priority()
        );
        assert!(
            python_parser.detection_priority() < rust_parser.detection_priority(),
            "PythonStructlogParser priority ({}) should be lower than RustTracingParser ({})",
            python_parser.detection_priority(),
            rust_parser.detection_priority()
        );
    }
}
