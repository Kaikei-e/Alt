use super::{
    docker::ParseError,
    generated::{VALIDATED_PATTERNS, pattern_index},
    regex_patterns::SimplePatternParser,
};
use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use serde_json::Value;

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub enum LogLevel {
    Debug,
    Info,
    Warn,
    Error,
    Fatal,
}

#[derive(Debug, Clone, Serialize)]
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

/// Trait for service-specific log parsers.
///
/// Implementations handle parsing log messages from specific service types
/// (e.g., nginx, Go applications, PostgreSQL).
pub trait ServiceParser: Send + Sync {
    /// Returns the service type identifier (e.g., "nginx", "go", "postgres").
    fn service_type(&self) -> &str;

    /// Parse a log message and extract structured data.
    fn parse_log(&self, log: &str) -> Result<ParsedLogEntry, ParseError>;

    /// Priority for auto-detection (higher = tried first).
    /// Default is 50. Range: 0-100.
    ///
    /// - 90-100: High priority (specific formats like nginx access logs)
    /// - 50-89: Medium priority (general formats like JSON)
    /// - 0-49: Low priority (fallback parsers)
    fn detection_priority(&self) -> u8 {
        50
    }

    /// Check if this parser can handle the given log format.
    /// Used for auto-detection when service name is unknown.
    fn can_parse(&self, log: &str) -> bool;
}

// Nginx Access Log Parser - Now uses memory-safe patterns
pub struct NginxParser {
    fallback_parser: SimplePatternParser,
}

impl Default for NginxParser {
    fn default() -> Self {
        Self::new()
    }
}

impl NginxParser {
    pub fn new() -> Self {
        Self {
            fallback_parser: SimplePatternParser::new(),
        }
    }

    /// Parse nginx access log with memory-safe patterns
    fn parse_nginx_access(&self, log: &str) -> Result<ParsedLogEntry, ParseError> {
        // Try with the full nginx access pattern first
        match VALIDATED_PATTERNS.get(pattern_index::NGINX_ACCESS_FULL) {
            Ok(regex) => {
                if let Some(captures) = regex.captures(log) {
                    return self.extract_access_log_data(captures, log);
                }
            }
            Err(regex_error) => {
                tracing::debug!(
                    "Full nginx access pattern failed: {}, trying fallback",
                    regex_error
                );
            }
        }

        // Try fallback pattern
        match VALIDATED_PATTERNS.get(pattern_index::NGINX_ACCESS_FALLBACK) {
            Ok(regex) => {
                if let Some(captures) = regex.captures(log) {
                    return self.extract_access_log_data(captures, log);
                }
            }
            Err(regex_error) => {
                tracing::debug!(
                    "Fallback nginx access pattern failed: {}, using simple parser",
                    regex_error
                );

                // Use simple pattern parser as last resort
                return self
                    .fallback_parser
                    .parse_nginx_access(log)
                    .map(|access_match| ParsedLogEntry {
                        service_type: "nginx".to_string(),
                        log_type: "access".to_string(),
                        message: log.to_string(),
                        level: Some(LogLevel::Info),
                        timestamp: None,
                        stream: "stdout".to_string(),
                        method: Some(access_match.method.to_string()),
                        path: Some(access_match.path.to_string()),
                        status_code: Some(access_match.status),
                        response_size: Some(access_match.size),
                        ip_address: Some(access_match.ip.to_string()),
                        user_agent: None,
                        fields: std::collections::HashMap::new(),
                    })
                    .map_err(|_| {
                        ParseError::InvalidFormat("Failed to parse nginx access log".to_string())
                    });
            }
        }

        Err(ParseError::InvalidFormat(
            "Not a valid nginx access log".to_string(),
        ))
    }

    /// Parse nginx error log with memory-safe patterns
    fn parse_nginx_error(&self, log: &str) -> Result<ParsedLogEntry, ParseError> {
        // Try with the full nginx error pattern first
        match VALIDATED_PATTERNS.get(pattern_index::NGINX_ERROR_FULL) {
            Ok(regex) => {
                if let Some(captures) = regex.captures(log) {
                    return self.extract_error_log_data(captures, log);
                }
            }
            Err(regex_error) => {
                tracing::debug!(
                    "Full nginx error pattern failed: {}, trying fallback",
                    regex_error
                );
            }
        }

        // Try fallback pattern
        match VALIDATED_PATTERNS.get(pattern_index::NGINX_ERROR_FALLBACK) {
            Ok(regex) => {
                if let Some(captures) = regex.captures(log) {
                    return self.extract_error_log_data(captures, log);
                }
            }
            Err(regex_error) => {
                tracing::debug!(
                    "Fallback nginx error pattern failed: {}, using simple parsing",
                    regex_error
                );

                // Simple parsing for nginx error logs
                if log.contains("[error]") || log.contains("[warn]") || log.contains("[info]") {
                    let level = if log.contains("[error]") {
                        LogLevel::Error
                    } else if log.contains("[warn]") {
                        LogLevel::Warn
                    } else {
                        LogLevel::Info
                    };

                    return Ok(ParsedLogEntry {
                        service_type: "nginx".to_string(),
                        log_type: "error".to_string(),
                        message: log.to_string(),
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
            }
        }

        Err(ParseError::InvalidFormat(
            "Not a valid nginx error log".to_string(),
        ))
    }

    /// Extract data from access log regex captures
    fn extract_access_log_data(
        &self,
        captures: regex::Captures<'_>,
        full_log: &str,
    ) -> Result<ParsedLogEntry, ParseError> {
        let ip = captures.get(1).map_or("", |m| m.as_str());
        let method = captures.get(3).map_or("", |m| m.as_str());
        let path = captures.get(4).map_or("", |m| m.as_str());
        let status = captures
            .get(5)
            .and_then(|m| m.as_str().parse().ok())
            .unwrap_or(0);
        let size_str = captures.get(6).map_or("0", |m| m.as_str());
        let size = if size_str == "-" {
            0
        } else {
            size_str.parse().unwrap_or(0)
        };

        Ok(ParsedLogEntry {
            service_type: "nginx".to_string(),
            log_type: "access".to_string(),
            message: full_log.to_string(),
            level: Some(LogLevel::Info),
            timestamp: None,
            stream: "stdout".to_string(),
            method: Some(method.to_string()),
            path: Some(path.to_string()),
            status_code: Some(status),
            response_size: Some(size),
            ip_address: Some(ip.to_string()),
            user_agent: captures.get(8).map(|m| m.as_str().to_string()),
            fields: std::collections::HashMap::new(),
        })
    }

    /// Extract data from error log regex captures
    fn extract_error_log_data(
        &self,
        captures: regex::Captures<'_>,
        full_log: &str,
    ) -> Result<ParsedLogEntry, ParseError> {
        let level_str = captures.get(2).map_or("info", |m| m.as_str());
        let level = match level_str.to_lowercase().as_str() {
            "error" => LogLevel::Error,
            "warn" | "warning" => LogLevel::Warn,
            "debug" => LogLevel::Debug,
            _ => LogLevel::Info,
        };

        Ok(ParsedLogEntry {
            service_type: "nginx".to_string(),
            log_type: "error".to_string(),
            message: full_log.to_string(),
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
        })
    }
}

impl ServiceParser for NginxParser {
    fn service_type(&self) -> &str {
        "nginx"
    }

    fn parse_log(&self, log: &str) -> Result<ParsedLogEntry, ParseError> {
        // Try access log format first
        if let Ok(result) = self.parse_nginx_access(log) {
            return Ok(result);
        }

        // Try error log format
        if let Ok(result) = self.parse_nginx_error(log) {
            return Ok(result);
        }

        // Final fallback for unrecognized nginx logs
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

    fn detection_priority(&self) -> u8 {
        // High priority - nginx logs have very specific formats
        80
    }

    fn can_parse(&self, log: &str) -> bool {
        // Check for nginx access log pattern (contains HTTP method and "HTTP/")
        if log.contains("HTTP/") && (log.contains("\"GET") || log.contains("\"POST") || log.contains("\"PUT") || log.contains("\"DELETE") || log.contains("\"PATCH") || log.contains("\"HEAD") || log.contains("\"OPTIONS")) {
            return true;
        }
        // Check for nginx error log pattern
        if log.contains("[error]") || log.contains("[warn]") || log.contains("[info]") || log.contains("[notice]") || log.contains("[crit]") {
            return true;
        }
        false
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

    // Helper function to convert JSON value to string without quotes
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
            if json_part.contains("\"level\"") || json_part.contains("\"msg\"") || json_part.contains("\"message\"") {
                // Verify it's valid JSON
                return serde_json::from_str::<Value>(json_part).is_ok();
            }
        }
        false
    }
}

// Meilisearch Log Parser - Strips ANSI escape codes
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

    /// Strip ANSI escape codes from log messages
    /// Handles sequences like: \x1b[2m, \x1b[0m, \x1b[32m, etc.
    fn strip_ansi(input: &str) -> String {
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
        log.contains("meilisearch") || log.contains("HTTP request{") ||
        (log.contains("\x1b[") && (log.contains("INFO") || log.contains("WARN") || log.contains("ERROR")))
    }
}

// PostgreSQL Log Parser - Now uses memory-safe patterns
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
        log.contains("LOG:") || log.contains("ERROR:") || log.contains("WARNING:")
            || log.contains("NOTICE:") || log.contains("DEBUG:") || log.contains("FATAL:")
            || log.contains("PANIC:") || log.contains("HINT:")
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

        let nginx_error = r"2024/01/01 12:00:00 [error] 123#0: *456 connect() failed (111: Connection refused) while connecting to upstream";

        let entry = parser.parse_log(nginx_error).unwrap();

        assert_eq!(entry.log_type, "error");
        assert_eq!(entry.level, Some(LogLevel::Error));
        assert!(entry.message.contains("Connection refused"));
    }

    #[test]
    fn test_go_structured_log_with_docker_timestamp_prefix() {
        let parser = GoStructuredParser::new();

        // Dockerが付与したタイムスタンプ付きのログをシミュレート
        let log_with_prefix = r#"2025-07-03T16:27:09.758077205Z {"level":"info","msg":"Got articles for summarization","service":"pre-processor","article_id":"test-123"}"#;

        let entry = parser.parse_log(log_with_prefix).unwrap();

        // log_typeが "structured" として正しくパースされることを確認
        assert_eq!(entry.log_type, "structured");
        assert_eq!(entry.service_type, "go");
        assert_eq!(entry.level, Some(LogLevel::Info));
        assert_eq!(entry.message, "Got articles for summarization");

        // fieldsが正しく抽出されることを確認（引用符なし）
        assert_eq!(
            entry.fields.get("service"),
            Some(&"pre-processor".to_string())
        );
        assert_eq!(
            entry.fields.get("article_id"),
            Some(&"test-123".to_string())
        );

        // プレフィックスがない純粋なJSONでもパースできることを確認
        let log_without_prefix =
            r#"{"level":"info","msg":"pure json log","count":42,"enabled":true}"#;
        let entry_no_prefix = parser.parse_log(log_without_prefix).unwrap();
        assert_eq!(entry_no_prefix.log_type, "structured");
        assert_eq!(entry_no_prefix.message, "pure json log");
        assert_eq!(entry_no_prefix.fields.get("count"), Some(&"42".to_string()));
        assert_eq!(
            entry_no_prefix.fields.get("enabled"),
            Some(&"true".to_string())
        );
    }

    #[test]
    fn test_go_structured_log_real_world_example() {
        let parser = GoStructuredParser::new();

        // 実際の問題のあるログ
        let real_log = r#"2025-07-03T18:53:46.741706684Z {"time":"2025-07-03T18:53:46.741620506Z","level":"info","msg":"processing article for quality check","service":"pre-processor","version":"1.0.0","article_id":"9739342c-d38f-469a-b94f-4aa55c58ab5b"}"#;

        let entry = parser.parse_log(real_log).unwrap();

        assert_eq!(entry.log_type, "structured");
        assert_eq!(entry.service_type, "go");
        assert_eq!(entry.level, Some(LogLevel::Info));
        assert_eq!(entry.message, "processing article for quality check");

        // fieldsの検証
        assert_eq!(
            entry.fields.get("service"),
            Some(&"pre-processor".to_string())
        );
        assert_eq!(entry.fields.get("version"), Some(&"1.0.0".to_string()));
        assert_eq!(
            entry.fields.get("article_id"),
            Some(&"9739342c-d38f-469a-b94f-4aa55c58ab5b".to_string())
        );
        assert_eq!(
            entry.fields.get("time"),
            Some(&"2025-07-03T18:53:46.741620506Z".to_string())
        );
    }

    #[test]
    fn test_postgres_log_parsing() {
        let parser = PostgresParser::new();

        let pg_log = r"2024-01-01 12:00:00.123 UTC [123] LOG:  statement: SELECT * FROM users WHERE id = $1";

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

    #[test]
    fn test_meilisearch_ansi_stripping() {
        let parser = MeilisearchParser::new();

        // Real meilisearch log with ANSI escape codes
        let ansi_log = "\x1b[2m2026-01-14T16:16:10.670962Z\x1b[0m \x1b[32m INFO\x1b[0m \x1b[1mHTTP request\x1b[0m\x1b[1m{\x1b[0m\x1b[3mmethod\x1b[0m\x1b[2m=\x1b[0mGET \x1b[3mhost\x1b[0m\x1b[2m=\x1b[0m\"meilisearch:7700\" \x1b[3mroute\x1b[0m\x1b[2m=\x1b[0m/tasks/463358\x1b[1m}\x1b[0m";

        let entry = parser.parse_log(ansi_log).unwrap();

        assert_eq!(entry.service_type, "meilisearch");
        assert_eq!(entry.log_type, "search");
        assert_eq!(entry.level, Some(LogLevel::Info));
        // Verify ANSI codes are stripped
        assert!(!entry.message.contains("\x1b["));
        assert!(entry.message.contains("HTTP request"));
        assert!(entry.message.contains("meilisearch:7700"));
    }

    #[test]
    fn test_meilisearch_error_level() {
        let parser = MeilisearchParser::new();

        let error_log = "2026-01-14T16:16:10.670962Z ERROR meilisearch: index not found";

        let entry = parser.parse_log(error_log).unwrap();

        assert_eq!(entry.level, Some(LogLevel::Error));
    }

    #[test]
    fn test_strip_ansi_function() {
        // Test the strip_ansi function directly
        let input = "\x1b[32mgreen\x1b[0m \x1b[1mbold\x1b[0m normal";
        let output = MeilisearchParser::strip_ansi(input);
        assert_eq!(output, "green bold normal");

        // Test with no ANSI codes
        let plain = "plain text without codes";
        assert_eq!(MeilisearchParser::strip_ansi(plain), plain);
    }
}
