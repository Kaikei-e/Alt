//! Nginx log parser for access and error logs.

use super::{LogLevel, ParsedLogEntry, ServiceParser};
use crate::parser::docker::ParseError;
use crate::parser::generated::{pattern_index, VALIDATED_PATTERNS};
use crate::parser::regex_patterns::SimplePatternParser;

/// Parser for Nginx access and error logs.
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
        if log.contains("HTTP/")
            && (log.contains("\"GET")
                || log.contains("\"POST")
                || log.contains("\"PUT")
                || log.contains("\"DELETE")
                || log.contains("\"PATCH")
                || log.contains("\"HEAD")
                || log.contains("\"OPTIONS"))
        {
            return true;
        }
        // Check for nginx error log pattern
        if log.contains("[error]")
            || log.contains("[warn]")
            || log.contains("[info]")
            || log.contains("[notice]")
            || log.contains("[crit]")
        {
            return true;
        }
        false
    }
}
