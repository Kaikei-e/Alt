use chrono::Utc;
use rask_log_forwarder::parser::{EnrichedLogEntry, LogLevel, ParseError, ParsedLogEntry};
use std::collections::HashMap;

// 簡単なモックパーサー実装
#[derive(Debug)]
pub struct MockLogParser {
    pub should_fail: bool,
    pub detected_format: String,
    pub can_parse_services: Vec<String>,
}

impl MockLogParser {
    pub fn new() -> Self {
        Self {
            should_fail: false,
            detected_format: "json".to_string(),
            can_parse_services: vec!["nginx".to_string(), "go-service".to_string()],
        }
    }

    pub fn with_failure(mut self) -> Self {
        self.should_fail = true;
        self
    }

    pub fn with_format(mut self, format: &str) -> Self {
        self.detected_format = format.to_string();
        self
    }

    pub fn with_supported_services(mut self, services: Vec<String>) -> Self {
        self.can_parse_services = services;
        self
    }

    pub fn parse_log(&self, log: &str) -> Result<ParsedLogEntry, ParseError> {
        if self.should_fail {
            return Err(ParseError::InvalidFormat("Mock parser failure".to_string()));
        }

        // Simple JSON parsing simulation
        if log.contains("\"level\":\"info\"") && log.contains("\"msg\":") {
            let mut fields = HashMap::new();

            // Extract latency if present
            if log.contains("\"latency\":123") {
                fields.insert("latency".to_string(), "123".to_string());
            }

            let message = if log.contains("Request processed") {
                "Request processed"
            } else if log.contains("Starting server") {
                "Starting server"
            } else if log.contains("Connection failed") {
                "Connection failed"
            } else if log.contains("Processing request") {
                "Processing request"
            } else {
                "Test message"
            };

            Ok(ParsedLogEntry {
                service_type: "go-service".to_string(),
                log_type: "structured".to_string(),
                message: message.to_string(),
                level: Some(LogLevel::Info),
                timestamp: Some(Utc::now()),
                stream: "stdout".to_string(),
                method: None,
                path: None,
                status_code: None,
                response_size: None,
                ip_address: None,
                user_agent: None,
                fields,
            })
        } else {
            Err(ParseError::InvalidFormat("Mock parser failure".to_string()))
        }
    }

    pub fn detect_format(&self, log: &str) -> String {
        if log.starts_with('{') && log.contains("level") {
            "json".to_string()
        } else if log.contains(" INFO ") || log.contains(" ERROR ") {
            "plain".to_string()
        } else if log.contains("GET ") && log.contains("HTTP/1.1") {
            "nginx".to_string()
        } else {
            self.detected_format.clone()
        }
    }

    pub fn can_parse(&self, service_type: &str) -> bool {
        self.can_parse_services.contains(&service_type.to_string())
    }
}

#[test]
fn test_mock_parser_successful_parsing() {
    let mock_parser = MockLogParser::new();

    // Test parsing
    let log = r#"{"level":"info","msg":"Request processed","latency":123}"#;
    let result = mock_parser.parse_log(log);

    assert!(result.is_ok());
    let entry = result.unwrap();
    assert_eq!(entry.message, "Request processed");
    assert_eq!(entry.level, Some(LogLevel::Info));
    assert_eq!(entry.fields.get("latency"), Some(&"123".to_string()));
}

#[test]
fn test_mock_parser_parsing_error() {
    let mock_parser = MockLogParser::new().with_failure();

    // Test parsing invalid log
    let result = mock_parser.parse_log("invalid json {");

    assert!(result.is_err());
    match result.unwrap_err() {
        ParseError::InvalidFormat(_) => {
            // Expected error
        }
        _ => panic!("Expected InvalidFormat error"),
    }
}

#[test]
fn test_mock_parser_format_detection() {
    let mock_parser = MockLogParser::new();

    // Test format detection
    assert_eq!(
        mock_parser.detect_format(r#"{"level":"info","msg":"test"}"#),
        "json"
    );
    assert_eq!(
        mock_parser.detect_format("2024-01-01 12:00:00 INFO Test message"),
        "plain"
    );
    assert_eq!(
        mock_parser.detect_format(
            r#"192.168.1.1 - - [01/Jan/2024:00:00:00 +0000] "GET /api/health HTTP/1.1" 200 2"#
        ),
        "nginx"
    );
}

#[test]
fn test_mock_parser_service_compatibility() {
    let mock_parser = MockLogParser::new();

    // Test service compatibility
    assert!(mock_parser.can_parse("nginx"));
    assert!(mock_parser.can_parse("go-service"));
    assert!(!mock_parser.can_parse("postgresql"));
}

#[test]
fn test_mock_parser_multiple_logs() {
    let mock_parser = MockLogParser::new();

    // Setup expectations for multiple different log formats
    let test_logs = vec![
        (
            r#"{"level":"info","msg":"Starting server"}"#,
            "Starting server",
        ),
        (
            r#"{"level":"info","msg":"Connection failed"}"#,
            "Connection failed",
        ),
        (
            r#"{"level":"info","msg":"Processing request"}"#,
            "Processing request",
        ),
    ];

    // Test parsing multiple logs
    for (log, expected_msg) in test_logs {
        let result = mock_parser.parse_log(log);
        assert!(result.is_ok());
        let entry = result.unwrap();
        assert_eq!(entry.message, expected_msg);
    }
}

// Mock for enrichment functionality
#[derive(Debug, Clone)]
#[allow(dead_code)]
pub struct ContainerInfo {
    pub id: String,
    pub name: String,
    pub labels: HashMap<String, String>,
    pub service_group: Option<String>,
}

#[allow(dead_code)]
#[derive(Debug, Clone)]
pub struct ServiceInfo {
    pub service_name: String,
    pub service_group: Option<String>,
    pub service_type: String,
}

#[derive(Debug)]
pub struct MockLogEnricher {
    pub service_info: ServiceInfo,
}

#[allow(dead_code)]
impl MockLogEnricher {
    pub fn new() -> Self {
        Self {
            service_info: ServiceInfo {
                service_name: "nginx".to_string(),
                service_group: Some("alt-frontend".to_string()),
                service_type: "nginx".to_string(),
            },
        }
    }

    pub fn with_service_info(mut self, service_info: ServiceInfo) -> Self {
        self.service_info = service_info;
        self
    }

    pub fn enrich_log(
        &self,
        entry: ParsedLogEntry,
        container_info: &ContainerInfo,
    ) -> EnrichedLogEntry {
        EnrichedLogEntry {
            service_type: entry.service_type,
            log_type: entry.log_type,
            message: entry.message,
            level: entry.level,
            timestamp: entry
                .timestamp
                .map(|dt| dt.to_rfc3339())
                .unwrap_or_else(|| "2024-01-01T00:00:00Z".to_string()),
            stream: entry.stream,
            method: entry.method,
            path: entry.path,
            status_code: entry.status_code,
            response_size: entry.response_size,
            ip_address: entry.ip_address,
            user_agent: entry.user_agent,
            container_id: container_info.id.clone(),
            service_name: container_info.name.clone(),
            service_group: container_info.service_group.clone(),
            trace_id: None,
        span_id: None,
        fields: entry.fields,
        }
    }

    pub fn extract_service_info(&self, _container_info: &ContainerInfo) -> ServiceInfo {
        self.service_info.clone()
    }
}

#[test]
fn test_mock_log_enricher() {
    let mock_enricher = MockLogEnricher::new();

    let container_info = ContainerInfo {
        id: "container123".to_string(),
        name: "nginx".to_string(),
        labels: {
            let mut labels = HashMap::new();
            labels.insert("rask.group".to_string(), "alt-frontend".to_string());
            labels
        },
        service_group: Some("alt-frontend".to_string()),
    };

    let parsed_entry = ParsedLogEntry {
        service_type: "nginx".to_string(),
        log_type: "access".to_string(),
        message: "GET /api/health 200".to_string(),
        level: Some(LogLevel::Info),
        timestamp: Some(Utc::now()),
        stream: "stdout".to_string(),
        method: Some("GET".to_string()),
        path: Some("/api/health".to_string()),
        status_code: Some(200),
        response_size: Some(2),
        ip_address: Some("192.168.1.1".to_string()),
        user_agent: None,
        fields: HashMap::new(),
    };

    // Test enrichment
    let enriched = mock_enricher.enrich_log(parsed_entry, &container_info);

    assert_eq!(enriched.service_name, "nginx");
    assert_eq!(enriched.service_group, Some("alt-frontend".to_string()));
    assert_eq!(enriched.container_id, "container123");
    assert_eq!(enriched.message, "GET /api/health 200");
}

#[test]
fn test_mock_parser_custom_formats() {
    let mock_parser = MockLogParser::new()
        .with_format("custom")
        .with_supported_services(vec!["redis".to_string(), "postgres".to_string()]);

    // Test custom format detection
    assert_eq!(mock_parser.detect_format("unknown format"), "custom");

    // Test custom service support
    assert!(mock_parser.can_parse("redis"));
    assert!(mock_parser.can_parse("postgres"));
    assert!(!mock_parser.can_parse("nginx"));
}
