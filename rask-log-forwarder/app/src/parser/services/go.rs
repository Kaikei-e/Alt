//! Go structured log parser for JSON-formatted logs.

use super::{LogLevel, ParsedLogEntry, ServiceParser};
use crate::parser::docker::ParseError;
use serde_json::Value;

/// Parser for Go structured logs (JSON format).
pub struct GoStructuredParser;

impl Default for GoStructuredParser {
    fn default() -> Self {
        Self::new()
    }
}

impl GoStructuredParser {
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
            _ => value.to_string(), // For arrays and objects, use JSON representation
        }
    }
}

impl ServiceParser for GoStructuredParser {
    fn service_type(&self) -> &str {
        "go"
    }

    fn parse_log(&self, log: &str) -> Result<ParsedLogEntry, ParseError> {
        let trimmed_log = log.trim();

        if let Some(start_brace_pos) = trimmed_log.find('{') {
            let potential_json_slice = &trimmed_log[start_brace_pos..];

            // 切り出したスライスがJSONとしてパース可能か試す
            match serde_json::from_str::<Value>(potential_json_slice) {
                Ok(json) => {
                    if let Some(obj) = json.as_object() {
                        let level_str = obj.get("level").and_then(|v| v.as_str()).unwrap_or("info");

                        let level = match level_str {
                            "debug" | "DEBUG" => LogLevel::Debug,
                            "info" | "INFO" => LogLevel::Info,
                            "warn" | "warning" | "WARN" | "WARNING" => LogLevel::Warn,
                            "error" | "ERROR" => LogLevel::Error,
                            "fatal" | "panic" | "FATAL" | "PANIC" => LogLevel::Fatal,
                            _ => LogLevel::Info,
                        };

                        let message = obj
                            .get("msg")
                            .or_else(|| obj.get("message"))
                            .and_then(|v| v.as_str())
                            .unwrap_or("")
                            .to_string();

                        let method = obj
                            .get("method")
                            .and_then(|v| v.as_str())
                            .map(str::to_string);
                        let path = obj
                            .get("path")
                            .and_then(|v| v.as_str())
                            .map(str::to_string);
                        #[allow(clippy::cast_possible_truncation)]
                        let status_code =
                            obj.get("status").and_then(|v| v.as_u64()).map(|n| n as u16);

                        let mut fields = std::collections::HashMap::new();
                        for (key, value) in obj {
                            if !["level", "msg", "message", "method", "path", "status"]
                                .contains(&key.as_str())
                            {
                                fields.insert(key.clone(), Self::json_value_to_string(value));
                            }
                        }

                        return Ok(ParsedLogEntry {
                            service_type: "go".to_string(),
                            log_type: "structured".to_string(), // 正しく "structured" になる
                            message,
                            level: Some(level),
                            timestamp: None,
                            stream: "stdout".to_string(),
                            method,
                            path,
                            status_code,
                            response_size: None,
                            ip_address: None,
                            user_agent: None,
                            fields,
                        });
                    }
                }
                Err(e) => {
                    tracing::error!("DEBUG GoParser: Failed to parse JSON: {e:?}");
                }
            }
        }

        // JSONとしてパースできなかった場合のフォールバック
        Ok(ParsedLogEntry {
            service_type: "go".to_string(),
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

    fn detection_priority(&self) -> u8 {
        // Medium priority - JSON is common but not as specific as nginx
        60
    }

    fn can_parse(&self, log: &str) -> bool {
        // Check if the log contains JSON with typical Go structured logging fields
        let trimmed = log.trim();

        // Find JSON object
        if let Some(start) = trimmed.find('{') {
            let json_part = &trimmed[start..];
            // Quick check for common Go log fields before full parse
            if json_part.contains("\"level\"")
                || json_part.contains("\"msg\"")
                || json_part.contains("\"message\"")
            {
                // Verify it's valid JSON
                return serde_json::from_str::<Value>(json_part).is_ok();
            }
        }
        false
    }
}
