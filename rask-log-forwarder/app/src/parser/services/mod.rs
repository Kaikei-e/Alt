//! Service-specific log parsers.
//!
//! This module provides parsers for different service types:
//! - Nginx (access and error logs)
//! - Go structured logs (JSON format)
//! - PostgreSQL logs
//! - Meilisearch logs

mod go;
mod meilisearch;
mod nginx;
mod postgres;

use super::docker::ParseError;
use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

// Re-export all parsers
pub use go::GoStructuredParser;
pub use meilisearch::MeilisearchParser;
pub use nginx::NginxParser;
pub use postgres::PostgresParser;

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
