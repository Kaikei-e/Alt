//! Service Parser Registry
//!
//! Provides a plugin-like architecture for service log parsers.
//! Allows dynamic registration of parsers and service-to-parser mappings.

use super::docker::ParseError;
use super::services::{ParsedLogEntry, ServiceParser};
use std::collections::HashMap;
use std::sync::Arc;

/// Registry for service log parsers.
///
/// Manages parser instances and service-to-parser mappings,
/// enabling runtime configuration of log parsing behavior.
pub struct ServiceParserRegistry {
    /// Map of parser_type -> parser instance
    parsers: HashMap<String, Arc<dyn ServiceParser>>,
    /// Map of service_name -> parser_type
    service_mappings: HashMap<String, String>,
    /// Ordered list of parsers for auto-detection (sorted by priority)
    detection_order: Vec<String>,
}

impl Default for ServiceParserRegistry {
    fn default() -> Self {
        Self::new()
    }
}

impl ServiceParserRegistry {
    /// Create a new empty registry.
    pub fn new() -> Self {
        Self {
            parsers: HashMap::new(),
            service_mappings: HashMap::new(),
            detection_order: Vec::new(),
        }
    }

    /// Register a parser for a specific type.
    ///
    /// # Arguments
    /// * `parser_type` - Unique identifier for this parser (e.g., "nginx", "go", "postgres")
    /// * `parser` - The parser implementation
    pub fn register_parser<P: ServiceParser + 'static>(&mut self, parser_type: &str, parser: P) {
        let arc_parser: Arc<dyn ServiceParser> = Arc::new(parser);
        let priority = arc_parser.detection_priority();

        self.parsers
            .insert(parser_type.to_string(), arc_parser.clone());

        // Update detection order based on priority
        self.detection_order.push(parser_type.to_string());
        self.detection_order.sort_by(|a, b| {
            let priority_a = self
                .parsers
                .get(a)
                .map(|p| p.detection_priority())
                .unwrap_or(0);
            let priority_b = self
                .parsers
                .get(b)
                .map(|p| p.detection_priority())
                .unwrap_or(0);
            priority_b.cmp(&priority_a) // Higher priority first
        });

        tracing::debug!(
            parser_type = parser_type,
            priority = priority,
            "Registered parser"
        );
    }

    /// Map a service name to a parser type.
    ///
    /// # Arguments
    /// * `service_name` - The Docker service name (e.g., "nginx", "alt-backend")
    /// * `parser_type` - The parser type to use for this service
    pub fn map_service(&mut self, service_name: &str, parser_type: &str) {
        self.service_mappings
            .insert(service_name.to_string(), parser_type.to_string());
        tracing::debug!(
            service_name = service_name,
            parser_type = parser_type,
            "Mapped service to parser"
        );
    }

    /// Get the parser for a specific service.
    ///
    /// Returns `None` if the service is not mapped to any parser.
    pub fn get_parser_for_service(&self, service_name: &str) -> Option<Arc<dyn ServiceParser>> {
        let parser_type = self.service_mappings.get(service_name)?;
        self.parsers.get(parser_type).cloned()
    }

    /// Get a parser by its type name.
    pub fn get_parser(&self, parser_type: &str) -> Option<Arc<dyn ServiceParser>> {
        self.parsers.get(parser_type).cloned()
    }

    /// Auto-detect the appropriate parser for a log line.
    ///
    /// Tries each registered parser in priority order (highest first)
    /// and returns the first one that can parse the log.
    pub fn detect_parser(&self, log: &str) -> Option<Arc<dyn ServiceParser>> {
        for parser_type in &self.detection_order {
            if let Some(parser) = self.parsers.get(parser_type) {
                if parser.can_parse(log) {
                    tracing::trace!(
                        parser_type = parser_type,
                        "Auto-detected parser for log"
                    );
                    return Some(parser.clone());
                }
            }
        }
        None
    }

    /// Parse a log using the appropriate parser for the service.
    ///
    /// If the service is mapped to a parser, uses that parser.
    /// Otherwise, tries auto-detection.
    pub fn parse_log(
        &self,
        log: &str,
        service_name: &str,
    ) -> Result<ParsedLogEntry, ParseError> {
        // Try mapped parser first
        if let Some(parser) = self.get_parser_for_service(service_name) {
            return parser.parse_log(log);
        }

        // Try auto-detection
        if let Some(parser) = self.detect_parser(log) {
            return parser.parse_log(log);
        }

        Err(ParseError::InvalidFormat(format!(
            "No suitable parser found for service '{}' and log format",
            service_name
        )))
    }

    /// Load service mappings from an environment variable.
    ///
    /// Format: `service1:parser1,service2:parser2,...`
    /// Example: `nginx:nginx,myapp:go,custom:python`
    pub fn load_mappings_from_env(&mut self, env_var: &str) {
        if let Ok(mappings_str) = std::env::var(env_var) {
            for mapping in mappings_str.split(',') {
                let parts: Vec<&str> = mapping.trim().split(':').collect();
                if parts.len() == 2 {
                    self.map_service(parts[0].trim(), parts[1].trim());
                }
            }
        }
    }

    /// Get all registered parser types.
    pub fn parser_types(&self) -> Vec<&str> {
        self.parsers.keys().map(|s| s.as_str()).collect()
    }

    /// Get all service mappings.
    pub fn service_mappings(&self) -> &HashMap<String, String> {
        &self.service_mappings
    }

    /// Check if a parser type is registered.
    pub fn has_parser(&self, parser_type: &str) -> bool {
        self.parsers.contains_key(parser_type)
    }

    /// Check if a service is mapped.
    pub fn has_service_mapping(&self, service_name: &str) -> bool {
        self.service_mappings.contains_key(service_name)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::parser::services::LogLevel;

    // Mock parser for testing
    struct MockParser {
        name: &'static str,
        priority: u8,
        can_parse_fn: fn(&str) -> bool,
    }

    impl ServiceParser for MockParser {
        fn service_type(&self) -> &str {
            self.name
        }

        fn parse_log(&self, log: &str) -> Result<ParsedLogEntry, ParseError> {
            Ok(ParsedLogEntry {
                service_type: self.name.to_string(),
                log_type: "test".to_string(),
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
            self.priority
        }

        fn can_parse(&self, log: &str) -> bool {
            (self.can_parse_fn)(log)
        }
    }

    #[test]
    fn test_register_parser() {
        let mut registry = ServiceParserRegistry::new();

        let parser = MockParser {
            name: "test",
            priority: 50,
            can_parse_fn: |_| true,
        };

        registry.register_parser("test", parser);

        assert!(registry.has_parser("test"));
        assert!(!registry.has_parser("nonexistent"));
    }

    #[test]
    fn test_map_service() {
        let mut registry = ServiceParserRegistry::new();

        let parser = MockParser {
            name: "go",
            priority: 50,
            can_parse_fn: |_| true,
        };

        registry.register_parser("go", parser);
        registry.map_service("alt-backend", "go");

        assert!(registry.has_service_mapping("alt-backend"));
        assert!(!registry.has_service_mapping("unknown"));
    }

    #[test]
    fn test_get_parser_for_service() {
        let mut registry = ServiceParserRegistry::new();

        let parser = MockParser {
            name: "nginx",
            priority: 50,
            can_parse_fn: |_| true,
        };

        registry.register_parser("nginx", parser);
        registry.map_service("nginx", "nginx");

        let retrieved = registry.get_parser_for_service("nginx");
        assert!(retrieved.is_some());
        assert_eq!(retrieved.unwrap().service_type(), "nginx");

        let missing = registry.get_parser_for_service("nonexistent");
        assert!(missing.is_none());
    }

    #[test]
    fn test_auto_detection_priority() {
        let mut registry = ServiceParserRegistry::new();

        // Register parsers with different priorities
        let low_priority = MockParser {
            name: "low",
            priority: 10,
            can_parse_fn: |_| true, // Always returns true
        };

        let high_priority = MockParser {
            name: "high",
            priority: 90,
            can_parse_fn: |_| true, // Always returns true
        };

        let medium_priority = MockParser {
            name: "medium",
            priority: 50,
            can_parse_fn: |_| true, // Always returns true
        };

        registry.register_parser("low", low_priority);
        registry.register_parser("high", high_priority);
        registry.register_parser("medium", medium_priority);

        // High priority parser should be selected
        let detected = registry.detect_parser("any log").unwrap();
        assert_eq!(detected.service_type(), "high");
    }

    #[test]
    fn test_auto_detection_can_parse() {
        let mut registry = ServiceParserRegistry::new();

        // Parser that only handles JSON
        let json_parser = MockParser {
            name: "json",
            priority: 50,
            can_parse_fn: |log| log.trim_start().starts_with('{'),
        };

        // Parser that handles nginx access logs
        let nginx_parser = MockParser {
            name: "nginx",
            priority: 60,
            can_parse_fn: |log| log.contains("HTTP/"),
        };

        registry.register_parser("json", json_parser);
        registry.register_parser("nginx", nginx_parser);

        // JSON log should be detected by json parser
        let json_log = r#"{"level":"info","msg":"test"}"#;
        let detected = registry.detect_parser(json_log).unwrap();
        assert_eq!(detected.service_type(), "json");

        // Nginx log should be detected by nginx parser
        let nginx_log = r#"192.168.1.1 - - [01/Jan/2024:12:00:00 +0000] "GET / HTTP/1.1" 200 1024"#;
        let detected = registry.detect_parser(nginx_log).unwrap();
        assert_eq!(detected.service_type(), "nginx");
    }

    #[test]
    fn test_parse_log_with_service_mapping() {
        let mut registry = ServiceParserRegistry::new();

        let parser = MockParser {
            name: "go",
            priority: 50,
            can_parse_fn: |_| true,
        };

        registry.register_parser("go", parser);
        registry.map_service("alt-backend", "go");

        let result = registry.parse_log("test log", "alt-backend");
        assert!(result.is_ok());
        assert_eq!(result.unwrap().service_type, "go");
    }

    #[test]
    fn test_parse_log_with_auto_detection() {
        let mut registry = ServiceParserRegistry::new();

        let parser = MockParser {
            name: "json",
            priority: 50,
            can_parse_fn: |log| log.starts_with('{'),
        };

        registry.register_parser("json", parser);

        // Unknown service but parseable format
        let result = registry.parse_log(r#"{"msg":"test"}"#, "unknown-service");
        assert!(result.is_ok());
    }

    #[test]
    fn test_load_mappings_from_env() {
        let mut registry = ServiceParserRegistry::new();

        let parser = MockParser {
            name: "go",
            priority: 50,
            can_parse_fn: |_| true,
        };

        registry.register_parser("go", parser);

        // SAFETY: This is a test and we're only modifying a test-specific env var
        // that won't affect other threads in a meaningful way
        unsafe {
            std::env::set_var("TEST_MAPPINGS_REGISTRY", "service1:go,service2:go");
        }
        registry.load_mappings_from_env("TEST_MAPPINGS_REGISTRY");
        unsafe {
            std::env::remove_var("TEST_MAPPINGS_REGISTRY");
        }

        assert!(registry.has_service_mapping("service1"));
        assert!(registry.has_service_mapping("service2"));
    }
}
