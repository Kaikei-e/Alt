use super::{
    docker::{DockerJsonParser, ParseError},
    generated::{VALIDATED_PATTERNS, pattern_index},
    schema::NginxLogEntry,
    services::{
        GoStructuredParser, LogLevel, NginxParser, ParsedLogEntry, PostgresParser, ServiceParser,
    },
};
use crate::collector::ContainerInfo;
use bytes::Bytes;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;

// Input validation constants
const MAX_LOG_LINE_SIZE: usize = 10 * 1024 * 1024; // 10MB per log line
const MAX_LOG_LINES_PER_BATCH: usize = 100_000; // Maximum lines per batch
const MAX_FIELD_SIZE: usize = 64 * 1024; // 64KB per field

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
        }
    }

    /// Validate input size to prevent DoS attacks and memory exhaustion
    fn validate_input_size(&self, log_bytes: &[u8]) -> Result<(), ParseError> {
        if log_bytes.len() > MAX_LOG_LINE_SIZE {
            return Err(ParseError::InvalidFormat(format!(
                "Log line too large: {} bytes (max: {})",
                log_bytes.len(),
                MAX_LOG_LINE_SIZE
            )));
        }
        Ok(())
    }

    /// Validate UTF-8 and handle invalid sequences gracefully
    fn validate_utf8(&self, log_bytes: &[u8]) -> Result<String, ParseError> {
        if log_bytes.is_empty() {
            return Err(ParseError::InvalidFormat("Empty log line".to_string()));
        }

        // Use lossy conversion to handle invalid UTF-8 gracefully
        let raw_str = String::from_utf8_lossy(log_bytes);

        // Check for null bytes or control characters that might indicate malformed input
        if raw_str.contains('\0') {
            return Err(ParseError::InvalidFormat(
                "Log contains null bytes".to_string(),
            ));
        }

        Ok(raw_str.into_owned())
    }

    /// Validate field size to prevent excessive memory usage
    fn validate_field_size(&self, field: &str, field_name: &str) -> Result<(), ParseError> {
        if field.len() > MAX_FIELD_SIZE {
            return Err(ParseError::InvalidFormat(format!(
                "Field '{}' too large: {} bytes (max: {})",
                field_name,
                field.len(),
                MAX_FIELD_SIZE
            )));
        }
        Ok(())
    }

    pub async fn parse_docker_log(
        &self,
        log_bytes: &[u8],
        container_info: &ContainerInfo,
    ) -> Result<EnrichedLogEntry, ParseError> {
        // Validate input size first
        self.validate_input_size(log_bytes)?;

        // Validate UTF-8 and get string representation
        let raw_str = self.validate_utf8(log_bytes)?;

        // Validate field sizes
        self.validate_field_size(&raw_str, "log_line")?;

        // Check if this is already a Docker JSON format or raw application log
        let (log_content, stream, timestamp) = if raw_str.trim_start().starts_with("{\"log\":") {
            // This is Docker JSON format
            let bytes = Bytes::from(raw_str);
            let docker_entry = self.docker_parser.parse(bytes)?;

            // Remove trailing newline from log content
            let log = docker_entry.log.trim_end_matches('\n').to_string();

            // Validate extracted fields
            self.validate_field_size(&log, "log_content")?;
            self.validate_field_size(&docker_entry.stream, "stream")?;
            self.validate_field_size(&docker_entry.time, "timestamp")?;

            (log, docker_entry.stream, docker_entry.time)
        } else {
            // This is raw application log (when timestamps: false in Docker API)
            // Remove trailing newline/carriage return
            let log = raw_str
                .trim_end_matches('\n')
                .trim_end_matches('\r')
                .to_string();
            (log, "stdout".to_string(), chrono::Utc::now().to_rfc3339())
        };

        // Now parse the actual log content
        let parsed_entry = self.parse_service_log(&log_content, &container_info.service_name)?;

        // Enrich with container metadata
        Ok(EnrichedLogEntry {
            service_type: parsed_entry.service_type,
            log_type: parsed_entry.log_type,
            message: parsed_entry.message,
            level: parsed_entry.level,
            timestamp,
            stream,
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

        let mut parsed_entry = parser.parse_log(log_content)?;

        // Add service_name to the parsed entry
        parsed_entry.service_type = service_name.to_string();

        Ok(parsed_entry)
    }

    fn auto_detect_format(
        &self,
        log_content: &str,
        service_name: &str,
    ) -> Result<ParsedLogEntry, ParseError> {
        // Try JSON first (common for Go services)
        if log_content.trim_start().starts_with('{')
            && let Ok(entry) = self.go_parser.parse_log(log_content)
        {
            return Ok(entry);
        }

        // Try nginx format
        if log_content.contains("HTTP/") && log_content.contains("\"")
            && let Ok(entry) = self.nginx_parser.parse_log(log_content)
        {
            return Ok(entry);
        }

        // Try postgres format
        if (log_content.contains("LOG:") || log_content.contains("ERROR:"))
            && let Ok(entry) = self.postgres_parser.parse_log(log_content)
        {
            return Ok(entry);
        }

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
        // Validate batch size to prevent memory exhaustion attacks
        if log_lines.len() > MAX_LOG_LINES_PER_BATCH {
            return vec![Err(ParseError::InvalidFormat(format!(
                "Batch too large: {} lines (max: {})",
                log_lines.len(),
                MAX_LOG_LINES_PER_BATCH
            )))];
        }

        let mut results = Vec::with_capacity(log_lines.len());

        for log_line in log_lines {
            results.push(self.parse_docker_log(log_line, container_info).await);
        }

        results
    }

    // Removes Docker native timestamp from log content
    // AS-IS: 2025-07-03T17:35:01.856438308Z {"char_count": 6281, "level": "info", ...}
    // TO-BE: {"char_count": 6281, "level": "info", ...}
    pub fn trim_docker_native_timestamp(&self, native_log: String) -> String {
        // Try using the validated regex patterns first
        match VALIDATED_PATTERNS.get(pattern_index::DOCKER_NATIVE_TIMESTAMP) {
            Ok(regex) => {
                let result = regex.replace(&native_log, "").to_string();
                if result != native_log {
                    return result;
                }
            }
            Err(regex_error) => {
                tracing::debug!(
                    "Docker native timestamp pattern failed: {}, trying fallback",
                    regex_error
                );
            }
        }

        // Try fallback pattern
        match VALIDATED_PATTERNS.get(pattern_index::ISO_TIMESTAMP_FALLBACK) {
            Ok(regex) => {
                let result = regex.replace(&native_log, "").to_string();
                if result != native_log {
                    return result;
                }
            }
            Err(regex_error) => {
                tracing::debug!(
                    "ISO timestamp fallback pattern failed: {}, using simple parsing",
                    regex_error
                );
            }
        }

        // If regex patterns fail, use simple string parsing
        if let Some(pos) = native_log.find('{') {
            // Check if there's a timestamp-like pattern before the '{'
            let prefix = &native_log[..pos];
            if prefix.contains("T") && prefix.contains(":") && prefix.contains("-") {
                return native_log[pos..].to_string();
            }
        }

        // If no JSON braces found, try to find where the timestamp ends
        // Look for common patterns after timestamps
        for &separator in &[" {", " [", " \"", "  "] {
            if let Some(pos) = native_log.find(separator) {
                let prefix = &native_log[..pos];
                // Basic timestamp validation
                if prefix.len() >= 19 && prefix.chars().nth(4) == Some('-') {
                    return native_log[pos + separator.len()..].to_string();
                }
            }
        }

        // Return as-is if no pattern matches
        native_log
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
            .expect("Failed to parse nginx docker log");

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
            .expect("Failed to parse go backend docker log");

        assert_eq!(entry.service_type, "alt-backend");
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
            .expect("Failed to parse unknown service docker log");

        assert_eq!(entry.service_type, "unknown-service");
        assert_eq!(entry.log_type, "plain");
        assert!(entry.message.contains("Some random log message"));
    }

    #[tokio::test]
    async fn test_universal_parser_with_native_timestamp() {
        let container_info = create_go_backend_container_info();
        let parser = UniversalParser::new();

        // Real case with native timestamp in log field
        let docker_log = r#"{"log":"2025-07-03T18:53:46.741706684Z {\"time\":\"2025-07-03T18:53:46.741620506Z\",\"level\":\"info\",\"msg\":\"processing article for quality check\",\"service\":\"pre-processor\",\"version\":\"1.0.0\",\"article_id\":\"9739342c-d38f-469a-b94f-4aa55c58ab5b\"}\n","stream":"stdout","time":"2025-07-03T18:53:46.741Z"}"#;

        let entry = parser
            .parse_docker_log(docker_log.as_bytes(), &container_info)
            .await
            .expect("Failed to parse docker log with native timestamp");

        assert_eq!(entry.service_type, "alt-backend");
        assert_eq!(entry.log_type, "structured");
        assert_eq!(entry.level, Some(LogLevel::Info));
        assert_eq!(entry.message, "processing article for quality check");
        assert_eq!(
            entry.fields.get("article_id"),
            Some(&"9739342c-d38f-469a-b94f-4aa55c58ab5b".to_string())
        );
        assert_eq!(
            entry.fields.get("service"),
            Some(&"pre-processor".to_string())
        );
        assert_eq!(entry.fields.get("version"), Some(&"1.0.0".to_string()));
    }

    #[test]
    fn test_trim_docker_native_timestamp() {
        let parser = UniversalParser::new();

        // Test with Z suffix
        let native_log = "2025-07-03T17:35:01.856438308Z {\"char_count\": 6281, \"level\": \"info\", \"logger\": \"tag_extractor.extract\", \"msg\": \"Processing text\", \"service\": \"tag-generator\", \"taskName\": null, \"timestamp\": \"iso\"}";
        let trimmed_log = parser.trim_docker_native_timestamp(native_log.to_string());
        assert_eq!(
            trimmed_log,
            "{\"char_count\": 6281, \"level\": \"info\", \"logger\": \"tag_extractor.extract\", \"msg\": \"Processing text\", \"service\": \"tag-generator\", \"taskName\": null, \"timestamp\": \"iso\"}"
        );

        // Test without Z suffix
        let native_log_no_z =
            "2025-07-03T17:35:01.856438308 {\"level\": \"info\", \"msg\": \"test\"}";
        let trimmed_log_no_z = parser.trim_docker_native_timestamp(native_log_no_z.to_string());
        assert_eq!(trimmed_log_no_z, "{\"level\": \"info\", \"msg\": \"test\"}");
    }

    #[tokio::test]
    async fn test_parse_batch_size_validation() {
        let container_info = create_go_backend_container_info();
        let parser = UniversalParser::new();

        // Create a batch that exceeds MAX_LOG_LINES_PER_BATCH
        let docker_log = r#"{"log":"test log\n","stream":"stdout","time":"2024-01-01T00:00:00Z"}"#;
        let log_lines: Vec<&[u8]> = vec![docker_log.as_bytes(); MAX_LOG_LINES_PER_BATCH + 1];

        let results = parser.parse_batch(log_lines, &container_info).await;

        // Should return a single error for the entire batch
        assert_eq!(results.len(), 1);
        assert!(results[0].is_err());

        if let Err(ParseError::InvalidFormat(msg)) = &results[0] {
            assert!(msg.contains("Batch too large"));
            assert!(msg.contains("100001 lines"));
            assert!(msg.contains("max: 100000"));
        } else {
            panic!("Expected InvalidFormat error");
        }
    }

    #[tokio::test]
    async fn test_parse_batch_normal_size() {
        let container_info = create_go_backend_container_info();
        let parser = UniversalParser::new();

        // Create a normal-sized batch
        let docker_log = r#"{"log":"test log\n","stream":"stdout","time":"2024-01-01T00:00:00Z"}"#;
        let log_lines: Vec<&[u8]> = vec![docker_log.as_bytes(); 10];

        let results = parser.parse_batch(log_lines, &container_info).await;

        // Should return results for all lines
        assert_eq!(results.len(), 10);
        assert!(results.iter().all(|r| r.is_ok()));
    }
}
