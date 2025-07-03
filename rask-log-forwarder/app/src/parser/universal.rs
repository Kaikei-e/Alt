use super::{
    docker::{DockerJsonParser, ParseError},
    schema::NginxLogEntry,
    services::{
        GoStructuredParser, LogLevel, NginxParser, ParsedLogEntry, PostgresParser, ServiceParser,
    },
};
use crate::collector::ContainerInfo;
use bytes::Bytes;
use lazy_static::lazy_static;
use regex::Regex;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;

lazy_static! {
    static ref DOCKER_NATIVE_TIMESTAMP_PATTERN: Regex =
        Regex::new(r#"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+[Z]?\s"#).unwrap();
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct EnrichedLogEntry {
    // From parsed log
    pub service_type: String,
    pub log_type: String,
    pub message: String,
    pub level: Option<LogLevel>,
    pub timestamp: String, // From Docker log
    pub stream: String,

    // HTTP fields
    pub method: Option<String>,
    pub path: Option<String>,
    pub status_code: Option<u16>,
    pub response_size: Option<u64>,
    pub ip_address: Option<String>,
    pub user_agent: Option<String>,

    // Container metadata
    pub container_id: String,
    pub service_name: String,
    pub service_group: Option<String>,

    // Additional fields
    pub fields: HashMap<String, String>,
}

impl From<NginxLogEntry> for EnrichedLogEntry {
    fn from(nginx_entry: NginxLogEntry) -> Self {
        Self {
            service_type: nginx_entry.service_type,
            log_type: nginx_entry.log_type,
            message: nginx_entry.message,
            level: nginx_entry
                .level
                .as_ref()
                .map(|level| match level.to_lowercase().as_str() {
                    "error" => LogLevel::Error,
                    "warn" | "warning" => LogLevel::Warn,
                    "debug" => LogLevel::Debug,
                    "fatal" => LogLevel::Fatal,
                    _ => LogLevel::Info,
                }),
            timestamp: nginx_entry.timestamp.to_rfc3339(),
            stream: nginx_entry.stream,
            method: nginx_entry.method,
            path: nginx_entry.path,
            status_code: nginx_entry.status_code,
            response_size: nginx_entry.response_size,
            ip_address: nginx_entry.ip_address,
            user_agent: nginx_entry.user_agent,
            container_id: nginx_entry
                .container_id
                .unwrap_or_else(|| "unknown".to_string()),
            service_name: "nginx".to_string(),
            service_group: Some("alt-frontend".to_string()),
            fields: HashMap::new(),
        }
    }
}

impl From<std::sync::Arc<NginxLogEntry>> for EnrichedLogEntry {
    fn from(nginx_entry: std::sync::Arc<NginxLogEntry>) -> Self {
        // Dereference the Arc and convert
        EnrichedLogEntry::from((*nginx_entry).clone())
    }
}

impl From<std::sync::Arc<dyn std::any::Any + Send + Sync>> for EnrichedLogEntry {
    fn from(any_entry: std::sync::Arc<dyn std::any::Any + Send + Sync>) -> Self {
        // Try to downcast to known types
        if let Some(nginx_entry) = any_entry.downcast_ref::<NginxLogEntry>() {
            EnrichedLogEntry::from(nginx_entry.clone())
        } else if let Some(enriched_entry) = any_entry.downcast_ref::<EnrichedLogEntry>() {
            enriched_entry.clone()
        } else {
            // Fallback to a dummy entry for unknown types
            Self {
                service_type: "unknown".to_string(),
                log_type: "unknown".to_string(),
                message: "Unknown log entry type".to_string(),
                level: Some(LogLevel::Info),
                timestamp: chrono::Utc::now().to_rfc3339(),
                stream: "stdout".to_string(),
                method: None,
                path: None,
                status_code: None,
                response_size: None,
                ip_address: None,
                user_agent: None,
                container_id: "unknown".to_string(),
                service_name: "unknown".to_string(),
                service_group: None,
                fields: HashMap::new(),
            }
        }
    }
}

pub struct UniversalParser {
    docker_parser: DockerJsonParser,
    nginx_parser: NginxParser,
    go_parser: GoStructuredParser,
    postgres_parser: PostgresParser,
    native_timestamp_pattern: Regex,
}

impl Default for UniversalParser {
    fn default() -> Self {
        Self::new()
    }
}

impl UniversalParser {
    pub fn new() -> Self {
        Self {
            docker_parser: DockerJsonParser::new(),
            nginx_parser: NginxParser::new(),
            go_parser: GoStructuredParser::new(),
            postgres_parser: PostgresParser::new(),
            native_timestamp_pattern: DOCKER_NATIVE_TIMESTAMP_PATTERN.clone(),
        }
    }

    pub async fn parse_docker_log(
        &self,
        log_bytes: &[u8],
        container_info: &ContainerInfo,
    ) -> Result<EnrichedLogEntry, ParseError> {
        // First, parse Docker JSON format
        let bytes = Bytes::copy_from_slice(log_bytes);
        let docker_entry = self.docker_parser.parse(bytes)?;

        // Remove native timestamp
        let log_content = self.trim_docker_native_timestamp(docker_entry.log.clone());

        // Remove trailing newline from log message
        let log_content = log_content.trim_end_matches('\n');

        // Determine service-specific parser
        let parsed_entry = self.parse_service_log(log_content, &container_info.service_name)?;

        // Enrich with container metadata
        Ok(EnrichedLogEntry {
            service_type: parsed_entry.service_type,
            log_type: parsed_entry.log_type,
            message: parsed_entry.message,
            level: parsed_entry.level,
            timestamp: docker_entry.time,
            stream: docker_entry.stream,
            method: parsed_entry.method,
            path: parsed_entry.path,
            status_code: parsed_entry.status_code,
            response_size: parsed_entry.response_size,
            ip_address: parsed_entry.ip_address,
            user_agent: parsed_entry.user_agent,
            container_id: container_info.id.clone(),
            service_name: container_info.service_name.clone(),
            service_group: container_info.group.clone(),
            fields: parsed_entry.fields,
        })
    }

    fn parse_service_log(
        &self,
        log_content: &str,
        service_name: &str,
    ) -> Result<ParsedLogEntry, ParseError> {
        let parser: &dyn ServiceParser = match service_name {
            "nginx" => &self.nginx_parser,
            "alt-backend" | "alt-frontend" | "pre-processor" | "search-indexer"
            | "tag-generator" => &self.go_parser,
            "db" | "postgres" | "postgresql" => &self.postgres_parser,
            _ => {
                // Try to auto-detect format
                return self.auto_detect_format(log_content, service_name);
            }
        };

        parser.parse_log(log_content)
    }

    fn auto_detect_format(
        &self,
        log_content: &str,
        service_name: &str,
    ) -> Result<ParsedLogEntry, ParseError> {
        // Try JSON first (common for Go services)
        if log_content.trim_start().starts_with('{') {
            if let Ok(entry) = self.go_parser.parse_log(log_content) {
                return Ok(entry);
            }
        }

        // Try nginx format
        if log_content.contains("HTTP/") && log_content.contains("\"") {
            if let Ok(entry) = self.nginx_parser.parse_log(log_content) {
                return Ok(entry);
            }
        }

        // Try postgres format
        if log_content.contains("LOG:") || log_content.contains("ERROR:") {
            if let Ok(entry) = self.postgres_parser.parse_log(log_content) {
                return Ok(entry);
            }
        }

        // Fallback to plain text
        Ok(ParsedLogEntry {
            service_type: service_name.to_string(),
            log_type: "plain".to_string(),
            message: log_content.to_string(),
            level: Some(LogLevel::Info),
            timestamp: None,
            stream: "stdout".to_string(),
            method: None,
            path: None,
            status_code: None,
            response_size: None,
            ip_address: None,
            user_agent: None,
            fields: HashMap::new(),
        })
    }

    pub async fn parse_batch(
        &self,
        log_lines: Vec<&[u8]>,
        container_info: &ContainerInfo,
    ) -> Vec<Result<EnrichedLogEntry, ParseError>> {
        let mut results = Vec::with_capacity(log_lines.len());

        for log_line in log_lines {
            results.push(self.parse_docker_log(log_line, container_info).await);
        }

        results
    }

    // AS-IS: 2025-07-03T17:35:01.856438308Z {"char_count": 6281, "level": "info", "logger": "tag_extractor.extract", "msg": "Processing text", "service": "tag-generator", "taskName": null, "timestamp": "iso"}
    // TO-BE: {"char_count": 6281, "level": "info", "logger": "tag_extractor.extract", "msg": "Processing text", "service": "tag-generator", "taskName": null, "timestamp": "iso"}
    pub fn trim_docker_native_timestamp(&self, native_log: String) -> String {
        // trim the timestamp
        self.native_timestamp_pattern
            .replace(&native_log, "")
            .to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

    fn create_nginx_container_info() -> ContainerInfo {
        let mut labels = HashMap::new();
        labels.insert("rask.group".to_string(), "alt-frontend".to_string());

        ContainerInfo {
            id: "nginx123".to_string(),
            service_name: "nginx".to_string(),
            labels,
            group: Some("alt-frontend".to_string()),
        }
    }

    fn create_go_backend_container_info() -> ContainerInfo {
        let mut labels = HashMap::new();
        labels.insert("rask.group".to_string(), "alt-backend".to_string());

        ContainerInfo {
            id: "backend456".to_string(),
            service_name: "alt-backend".to_string(),
            labels,
            group: Some("alt-backend".to_string()),
        }
    }

    #[tokio::test]
    async fn test_universal_parser_with_nginx() {
        let container_info = create_nginx_container_info();
        let parser = UniversalParser::new();

        let docker_log = r#"{"log":"192.168.1.1 - - [01/Jan/2024:00:00:00 +0000] \"GET /health HTTP/1.1\" 200 2 \"-\" \"curl/7.68.0\"\n","stream":"stdout","time":"2024-01-01T00:00:00Z"}"#;

        let entry = parser
            .parse_docker_log(docker_log.as_bytes(), &container_info)
            .await
            .unwrap();

        assert_eq!(entry.service_type, "nginx");
        assert_eq!(entry.log_type, "access");
        assert_eq!(entry.status_code, Some(200));
        assert_eq!(entry.container_id, "nginx123");
        assert_eq!(entry.service_group, Some("alt-frontend".to_string()));
    }

    #[tokio::test]
    async fn test_universal_parser_with_go_backend() {
        let container_info = create_go_backend_container_info();
        let parser = UniversalParser::new();

        let docker_log = r#"{"log":"{\"level\":\"info\",\"msg\":\"Processing request\",\"method\":\"GET\"}\n","stream":"stdout","time":"2024-01-01T00:00:00Z"}"#;

        let entry = parser
            .parse_docker_log(docker_log.as_bytes(), &container_info)
            .await
            .unwrap();

        assert_eq!(entry.service_type, "go");
        assert_eq!(entry.log_type, "structured");
        assert_eq!(entry.level, Some(LogLevel::Info));
        assert_eq!(entry.container_id, "backend456");
    }

    #[tokio::test]
    async fn test_universal_parser_with_unknown_service() {
        let container_info = ContainerInfo {
            id: "unknown789".to_string(),
            service_name: "unknown-service".to_string(),
            labels: HashMap::new(),
            group: None,
        };

        let parser = UniversalParser::new();

        let docker_log = r#"{"log":"Some random log message\n","stream":"stdout","time":"2024-01-01T00:00:00Z"}"#;

        let entry = parser
            .parse_docker_log(docker_log.as_bytes(), &container_info)
            .await
            .unwrap();

        assert_eq!(entry.service_type, "unknown-service");
        assert_eq!(entry.log_type, "plain");
        assert!(entry.message.contains("Some random log message"));
    }

    #[test]
    fn test_trim_docker_native_timestamp() {
        let parser = UniversalParser::new();
        let native_log = "2025-07-03T17:35:01.856438308Z {\"char_count\": 6281, \"level\": \"info\", \"logger\": \"tag_extractor.extract\", \"msg\": \"Processing text\", \"service\": \"tag-generator\", \"taskName\": null, \"timestamp\": \"iso\"}";
        let trimmed_log = parser.trim_docker_native_timestamp(native_log.to_string());
        assert_eq!(
            trimmed_log,
            "{\"char_count\": 6281, \"level\": \"info\", \"logger\": \"tag_extractor.extract\", \"msg\": \"Processing text\", \"service\": \"tag-generator\", \"taskName\": null, \"timestamp\": \"iso\"}"
        );
    }
}
