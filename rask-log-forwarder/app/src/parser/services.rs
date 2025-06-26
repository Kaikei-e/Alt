use regex::Regex;
use lazy_static::lazy_static;
use serde_json::Value;
use chrono::{DateTime, Utc};
use super::docker::ParseError;
use serde::{Serialize, Deserialize};

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub enum LogLevel {
    Debug,
    Info,
    Warn,
    Error,
    Fatal,
}

#[derive(Debug, Clone)]
pub struct ParsedLogEntry {
    pub service_type: String,
    pub log_type: String,
    pub message: String,
    pub level: Option<LogLevel>,
    pub timestamp: Option<DateTime<Utc>>,
    pub stream: String,

    // HTTP fields
    pub method: Option<String>,
    pub path: Option<String>,
    pub status_code: Option<u16>,
    pub response_size: Option<u64>,
    pub ip_address: Option<String>,
    pub user_agent: Option<String>,

    // Structured fields
    pub fields: std::collections::HashMap<String, String>,
}

pub trait ServiceParser {
    fn service_type(&self) -> &str;
    fn parse_log(&self, log: &str) -> Result<ParsedLogEntry, ParseError>;
}

// Nginx Access Log Parser
pub struct NginxParser {
    access_regex: Regex,
    error_regex: Regex,
}

lazy_static! {
    static ref NGINX_ACCESS_PATTERN: Regex = Regex::new(
        r#"^(\S+) \S+ \S+ \[([^\]]+)\] "([A-Z]+) ([^"]*) HTTP/[^"]*" (\d+) (\d+|-)(?: "([^"]*)" "([^"]*)")?.*$"#
    ).unwrap();

    static ref NGINX_ERROR_PATTERN: Regex = Regex::new(
        r#"^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}) \[(\w+)\] \d+#\d+: (.+)"#
    ).unwrap();
}

impl Default for NginxParser {
    fn default() -> Self {
        Self::new()
    }
}

impl NginxParser {
    pub fn new() -> Self {
        Self {
            access_regex: NGINX_ACCESS_PATTERN.clone(),
            error_regex: NGINX_ERROR_PATTERN.clone(),
        }
    }
}

impl ServiceParser for NginxParser {
    fn service_type(&self) -> &str {
        "nginx"
    }

    fn parse_log(&self, log: &str) -> Result<ParsedLogEntry, ParseError> {
        // Try access log format first
        if let Some(captures) = self.access_regex.captures(log) {
            let ip = captures.get(1).map(|m| m.as_str().to_string());
            let method = captures.get(3).map(|m| m.as_str().to_string());
            let path = captures.get(4).map(|m| m.as_str().to_string());
            let status = captures.get(5).and_then(|m| m.as_str().parse().ok());
            let size = captures.get(6).and_then(|m| {
                if m.as_str() == "-" { None } else { m.as_str().parse().ok() }
            });
            let user_agent = captures.get(8).map(|m| m.as_str().to_string());

            return Ok(ParsedLogEntry {
                service_type: "nginx".to_string(),
                log_type: "access".to_string(),
                message: log.to_string(),
                level: Some(LogLevel::Info),
                timestamp: None, // TODO: parse timestamp
                stream: "stdout".to_string(),
                method,
                path,
                status_code: status,
                response_size: size,
                ip_address: ip,
                user_agent,
                fields: std::collections::HashMap::new(),
            });
        }

        // Try error log format
        if let Some(captures) = self.error_regex.captures(log) {
            let level_str = captures.get(2).map(|m| m.as_str()).unwrap_or("info");
            let level = match level_str {
                "debug" => LogLevel::Debug,
                "info" => LogLevel::Info,
                "warn" | "warning" => LogLevel::Warn,
                "error" => LogLevel::Error,
                "crit" | "critical" => LogLevel::Fatal,
                _ => LogLevel::Info,
            };

            let message = captures.get(3).map(|m| m.as_str()).unwrap_or(log).to_string();

            return Ok(ParsedLogEntry {
                service_type: "nginx".to_string(),
                log_type: "error".to_string(),
                message,
                level: Some(level),
                timestamp: None,
                stream: "stderr".to_string(),
                method: None,
                path: None,
                status_code: None,
                response_size: None,
                ip_address: None,
                user_agent: None,
                fields: std::collections::HashMap::new(),
            });
        }

        // Fallback - but first check if it looks like an access log
        if log.contains("HTTP/") && log.contains("\"") {
            return Ok(ParsedLogEntry {
                service_type: "nginx".to_string(),
                log_type: "access".to_string(),
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
            });
        }

        // Final fallback to plain text
        Ok(ParsedLogEntry {
            service_type: "nginx".to_string(),
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
    }
}

// Go Structured Log Parser
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
}

impl ServiceParser for GoStructuredParser {
    fn service_type(&self) -> &str {
        "go"
    }

    fn parse_log(&self, log: &str) -> Result<ParsedLogEntry, ParseError> {
        // Try to parse as JSON first
        if let Ok(json) = serde_json::from_str::<Value>(log) {
            if let Some(obj) = json.as_object() {
                let level_str = obj.get("level")
                    .and_then(|v| v.as_str())
                    .unwrap_or("info");

                let level = match level_str {
                    "debug" => LogLevel::Debug,
                    "info" => LogLevel::Info,
                    "warn" | "warning" => LogLevel::Warn,
                    "error" => LogLevel::Error,
                    "fatal" | "panic" => LogLevel::Fatal,
                    _ => LogLevel::Info,
                };

                let message = obj.get("msg")
                    .or_else(|| obj.get("message"))
                    .and_then(|v| v.as_str())
                    .unwrap_or(log)
                    .to_string();

                let method = obj.get("method").and_then(|v| v.as_str()).map(|s| s.to_string());
                let path = obj.get("path").and_then(|v| v.as_str()).map(|s| s.to_string());
                let status_code = obj.get("status").and_then(|v| v.as_u64()).map(|n| n as u16);

                // Extract additional fields
                let mut fields = std::collections::HashMap::new();
                for (key, value) in obj {
                    if !["level", "msg", "message", "method", "path", "status"].contains(&key.as_str()) {
                        fields.insert(key.clone(), value.to_string());
                    }
                }

                return Ok(ParsedLogEntry {
                    service_type: "go".to_string(),
                    log_type: "structured".to_string(),
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

        // Fallback to plain text
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
}

// PostgreSQL Log Parser
pub struct PostgresParser {
    log_regex: Regex,
}

lazy_static! {
    static ref POSTGRES_LOG_PATTERN: Regex = Regex::new(
        r#"^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d+) \w+ \[\d+\] (\w+):\s+(.+)"#
    ).unwrap();
}

impl Default for PostgresParser {
    fn default() -> Self {
        Self::new()
    }
}

impl PostgresParser {
    pub fn new() -> Self {
        Self {
            log_regex: POSTGRES_LOG_PATTERN.clone(),
        }
    }
}

impl ServiceParser for PostgresParser {
    fn service_type(&self) -> &str {
        "postgres"
    }

    fn parse_log(&self, log: &str) -> Result<ParsedLogEntry, ParseError> {
        if let Some(captures) = self.log_regex.captures(log) {
            let level_str = captures.get(2).map(|m| m.as_str()).unwrap_or("LOG");
            let level = match level_str {
                "DEBUG" | "DEBUG1" | "DEBUG2" | "DEBUG3" | "DEBUG4" | "DEBUG5" => LogLevel::Debug,
                "LOG" | "INFO" => LogLevel::Info,
                "NOTICE" | "WARNING" => LogLevel::Warn,
                "ERROR" => LogLevel::Error,
                "FATAL" | "PANIC" => LogLevel::Fatal,
                _ => LogLevel::Info,
            };

            let message = captures.get(3).map(|m| m.as_str()).unwrap_or(log).to_string();

            Ok(ParsedLogEntry {
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
            })
        } else {
            // Fallback
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
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_nginx_access_log_parsing() {
        let parser = NginxParser::new();

        let nginx_log = r#"192.168.1.100 - - [01/Jan/2024:12:00:00 +0000] "GET /api/health HTTP/1.1" 200 1024 "-" "curl/7.68.0""#;

        let entry = parser.parse_log(nginx_log).unwrap();

        assert_eq!(entry.service_type, "nginx");
        assert_eq!(entry.log_type, "access");
        assert_eq!(entry.ip_address, Some("192.168.1.100".to_string()));
        assert_eq!(entry.method, Some("GET".to_string()));
        assert_eq!(entry.path, Some("/api/health".to_string()));
        assert_eq!(entry.status_code, Some(200));
        assert_eq!(entry.response_size, Some(1024));
    }

    #[test]
    fn test_nginx_error_log_parsing() {
        let parser = NginxParser::new();

        let nginx_error = r#"2024/01/01 12:00:00 [error] 123#0: *456 connect() failed (111: Connection refused) while connecting to upstream"#;

        let entry = parser.parse_log(nginx_error).unwrap();

        assert_eq!(entry.log_type, "error");
        assert_eq!(entry.level, Some(LogLevel::Error));
        assert!(entry.message.contains("Connection refused"));
    }

    #[test]
    fn test_go_structured_log_parsing() {
        let parser = GoStructuredParser::new();

        let go_log = r#"{"level":"info","ts":"2024-01-01T12:00:00.123Z","caller":"main.go:42","msg":"Request processed","method":"GET","path":"/api/users","status":200,"duration":"15ms"}"#;

        let entry = parser.parse_log(go_log).unwrap();

        assert_eq!(entry.service_type, "go");
        assert_eq!(entry.level, Some(LogLevel::Info));
        assert_eq!(entry.message, "Request processed");
        assert_eq!(entry.method, Some("GET".to_string()));
        assert_eq!(entry.path, Some("/api/users".to_string()));
        assert_eq!(entry.status_code, Some(200));
    }

    #[test]
    fn test_postgres_log_parsing() {
        let parser = PostgresParser::new();

        let pg_log = r#"2024-01-01 12:00:00.123 UTC [123] LOG:  statement: SELECT * FROM users WHERE id = $1"#;

        let entry = parser.parse_log(pg_log).unwrap();

        assert_eq!(entry.service_type, "postgres");
        assert_eq!(entry.level, Some(LogLevel::Info));
        assert!(entry.message.contains("SELECT * FROM users"));
    }

    #[test]
    fn test_unknown_format_fallback() {
        let parser = GoStructuredParser::new();

        let unknown_log = "This is just plain text without structure";

        let entry = parser.parse_log(unknown_log).unwrap();

        assert_eq!(entry.service_type, "go");
        assert_eq!(entry.message, unknown_log);
        assert_eq!(entry.level, Some(LogLevel::Info)); // Default level
    }
}