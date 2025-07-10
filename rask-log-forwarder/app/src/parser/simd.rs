use super::{
    schema::{LogEntry, NginxLogEntry, ParseError},
    generated::{VALIDATED_PATTERNS, pattern_index},
    regex_patterns::SimplePatternParser,
    zero_alloc_parser::ImprovedNginxParser,
};
use bytes::Bytes;
use chrono::{DateTime, Utc};
use simd_json::prelude::{ValueAsObject, ValueAsScalar};
use simd_json::{OwnedValue, from_slice};


pub struct SimdParser {
    #[allow(dead_code)]
    arena: bumpalo::Bump,
}

impl Default for SimdParser {
    fn default() -> Self {
        Self::new()
    }
}

impl SimdParser {
    pub fn new() -> Self {
        Self {
            arena: bumpalo::Bump::new(),
        }
    }

    pub fn parse_docker_log(&self, bytes: Bytes) -> Result<LogEntry, ParseError> {
        // Convert Bytes to mutable Vec<u8> for simd_json
        let mut data = bytes.to_vec();
        let json = from_slice::<OwnedValue>(&mut data)?;

        let obj = json.as_object().ok_or(ParseError::InvalidFormat(
            "Expected JSON object".to_string(),
        ))?;

        let log = obj
            .get("log")
            .and_then(|v| v.as_str())
            .ok_or(ParseError::MissingField("log"))?;

        let stream = obj
            .get("stream")
            .and_then(|v| v.as_str())
            .unwrap_or("stdout");

        let time_str = obj
            .get("time")
            .and_then(|v| v.as_str())
            .ok_or(ParseError::MissingField("time"))?;

        let timestamp = time_str.parse::<DateTime<Utc>>()?;

        Ok(LogEntry {
            message: log.to_string(),
            stream: stream.to_string(),
            timestamp,
            service_name: None,
            container_id: None,
        })
    }

    pub fn parse_nginx_log(&self, bytes: Bytes) -> Result<NginxLogEntry, ParseError> {
        let log_entry = self.parse_docker_log(bytes)?;

        // Detect if this is an nginx access log
        if self.is_nginx_access_log(&log_entry.message) {
            self.parse_nginx_access_log(log_entry)
        } else if self.is_nginx_error_log(&log_entry.message) {
            self.parse_nginx_error_log(log_entry)
        } else {
            Err(ParseError::InvalidFormat(
                "Not a recognized nginx log format".to_string(),
            ))
        }
    }

    fn is_nginx_access_log(&self, message: &str) -> bool {
        // Try SIMD nginx access patterns
        if let Ok(regex) = VALIDATED_PATTERNS.get(pattern_index::SIMD_NGINX_ACCESS) {
            if regex.is_match(message) {
                return true;
            }
        }
        if let Ok(regex) = VALIDATED_PATTERNS.get(pattern_index::SIMD_NGINX_COMBINED) {
            if regex.is_match(message) {
                return true;
            }
        }
        // Try fallback patterns
        if let Ok(regex) = VALIDATED_PATTERNS.get(pattern_index::SIMD_NGINX_ACCESS_FALLBACK) {
            if regex.is_match(message) {
                return true;
            }
        }
        if let Ok(regex) = VALIDATED_PATTERNS.get(pattern_index::SIMD_NGINX_COMBINED_FALLBACK) {
            if regex.is_match(message) {
                return true;
            }
        }
        
        // Simple heuristic as final fallback
        message.contains("HTTP/") && message.contains("\"")
    }

    fn is_nginx_error_log(&self, message: &str) -> bool {
        // Try SIMD nginx error patterns
        if let Ok(regex) = VALIDATED_PATTERNS.get(pattern_index::SIMD_NGINX_ERROR) {
            if regex.is_match(message) {
                return true;
            }
        }
        if let Ok(regex) = VALIDATED_PATTERNS.get(pattern_index::SIMD_NGINX_ERROR_FALLBACK) {
            if regex.is_match(message) {
                return true;
            }
        }
        
        // Simple heuristic as final fallback
        message.contains("[error]") || message.contains("[warn]") || message.contains("[info]")
    }

    fn parse_nginx_access_log(&self, log_entry: LogEntry) -> Result<NginxLogEntry, ParseError> {
        let message_clone = log_entry.message.clone();

        // Try combined format first (more fields)
        if let Ok(regex) = VALIDATED_PATTERNS.get(pattern_index::SIMD_NGINX_COMBINED) {
            if let Some(captures) = regex.captures(&message_clone) {
                return Ok(NginxLogEntry {
                    service_type: "nginx".to_string(),
                    log_type: "access".to_string(),
                    message: log_entry.message,
                    stream: log_entry.stream,
                    timestamp: log_entry.timestamp,
                    container_id: log_entry.container_id,
                    ip_address: captures.get(1).map(|m| m.as_str().to_string()),
                    method: captures.get(3).map(|m| m.as_str().to_string()),
                    path: captures.get(4).map(|m| m.as_str().to_string()),
                    status_code: {
                        let (status, _) = ImprovedNginxParser::parse_status_and_size_safe(
                            captures.get(5).map(|m| m.as_str()),
                            None
                        );
                        status
                    },
                    response_size: {
                        let (_, size) = ImprovedNginxParser::parse_status_and_size_safe(
                            None,
                            captures.get(6).map(|m| m.as_str())
                        );
                        size
                    },
                    user_agent: captures.get(8).map(|m| m.as_str().to_string()),
                    level: None,
                });
            }
        }
        
        // Try regular access format
        if let Ok(regex) = VALIDATED_PATTERNS.get(pattern_index::SIMD_NGINX_ACCESS) {
            if let Some(captures) = regex.captures(&message_clone) {
                return Ok(NginxLogEntry {
                    service_type: "nginx".to_string(),
                    log_type: "access".to_string(),
                    message: log_entry.message,
                    stream: log_entry.stream,
                    timestamp: log_entry.timestamp,
                    container_id: log_entry.container_id,
                    ip_address: captures.get(1).map(|m| m.as_str().to_string()),
                    method: captures.get(3).map(|m| m.as_str().to_string()),
                    path: captures.get(4).map(|m| m.as_str().to_string()),
                    status_code: {
                        let (status, _) = ImprovedNginxParser::parse_status_and_size_safe(
                            captures.get(5).map(|m| m.as_str()),
                            None
                        );
                        status
                    },
                    response_size: {
                        let (_, size) = ImprovedNginxParser::parse_status_and_size_safe(
                            None,
                            captures.get(6).map(|m| m.as_str())
                        );
                        size
                    },
                    user_agent: None,
                    level: None,
                });
            }
        }
        
        // Try fallback patterns
        if let Ok(regex) = VALIDATED_PATTERNS.get(pattern_index::SIMD_NGINX_COMBINED_FALLBACK) {
            if let Some(captures) = regex.captures(&message_clone) {
                return Ok(NginxLogEntry {
                    service_type: "nginx".to_string(),
                    log_type: "access".to_string(),
                    message: log_entry.message,
                    stream: log_entry.stream,
                    timestamp: log_entry.timestamp,
                    container_id: log_entry.container_id,
                    ip_address: captures.get(1).map(|m| m.as_str().to_string()),
                    method: captures.get(2).map(|m| m.as_str().to_string()),
                    path: captures.get(3).map(|m| m.as_str().to_string()),
                    status_code: {
                        let (status, _) = ImprovedNginxParser::parse_status_and_size_safe(
                            captures.get(4).map(|m| m.as_str()),
                            None
                        );
                        status
                    },
                    response_size: {
                        let (_, size) = ImprovedNginxParser::parse_status_and_size_safe(
                            None,
                            captures.get(5).map(|m| m.as_str())
                        );
                        size
                    },
                    user_agent: captures.get(7).map(|m| m.as_str().to_string()),
                    level: None,
                });
            }
        }
        
        if let Ok(regex) = VALIDATED_PATTERNS.get(pattern_index::SIMD_NGINX_ACCESS_FALLBACK) {
            if let Some(captures) = regex.captures(&message_clone) {
                return Ok(NginxLogEntry {
                    service_type: "nginx".to_string(),
                    log_type: "access".to_string(),
                    message: log_entry.message,
                    stream: log_entry.stream,
                    timestamp: log_entry.timestamp,
                    container_id: log_entry.container_id,
                    ip_address: captures.get(1).map(|m| m.as_str().to_string()),
                    method: captures.get(2).map(|m| m.as_str().to_string()),
                    path: captures.get(3).map(|m| m.as_str().to_string()),
                    status_code: {
                        let (status, _) = ImprovedNginxParser::parse_status_and_size_safe(
                            captures.get(4).map(|m| m.as_str()),
                            None
                        );
                        status
                    },
                    response_size: {
                        let (_, size) = ImprovedNginxParser::parse_status_and_size_safe(
                            None,
                            captures.get(5).map(|m| m.as_str())
                        );
                        size
                    },
                    user_agent: None,
                    level: None,
                });
            }
        }
        
        // Use simple parser as final fallback
        let simple_parser = SimplePatternParser::new();
        if let Ok(access_match) = simple_parser.parse_nginx_access(&message_clone) {
            return Ok(NginxLogEntry {
                service_type: "nginx".to_string(),
                log_type: "access".to_string(),
                message: log_entry.message,
                stream: log_entry.stream,
                timestamp: log_entry.timestamp,
                container_id: log_entry.container_id,
                ip_address: Some(access_match.ip.to_string()),
                method: Some(access_match.method.to_string()),
                path: Some(access_match.path.to_string()),
                status_code: Some(access_match.status),
                response_size: Some(access_match.size),
                user_agent: None,
                level: None,
            });
        }

        Err(ParseError::InvalidFormat(
            "Could not parse nginx access log".to_string(),
        ))
    }

    fn parse_nginx_error_log(&self, log_entry: LogEntry) -> Result<NginxLogEntry, ParseError> {
        let message_clone = log_entry.message.clone();

        // Try SIMD nginx error pattern
        if let Ok(regex) = VALIDATED_PATTERNS.get(pattern_index::SIMD_NGINX_ERROR) {
            if let Some(captures) = regex.captures(&message_clone) {
                return Ok(NginxLogEntry {
                    service_type: "nginx".to_string(),
                    log_type: "error".to_string(),
                    message: log_entry.message,
                    stream: log_entry.stream,
                    timestamp: log_entry.timestamp,
                    container_id: log_entry.container_id,
                    ip_address: None,
                    method: None,
                    path: None,
                    status_code: None,
                    response_size: None,
                    user_agent: None,
                    level: captures.get(2).map(|m| m.as_str().to_string()),
                });
            }
        }
        
        // Try fallback error pattern
        if let Ok(regex) = VALIDATED_PATTERNS.get(pattern_index::SIMD_NGINX_ERROR_FALLBACK) {
            if let Some(captures) = regex.captures(&message_clone) {
                return Ok(NginxLogEntry {
                    service_type: "nginx".to_string(),
                    log_type: "error".to_string(),
                    message: log_entry.message,
                    stream: log_entry.stream,
                    timestamp: log_entry.timestamp,
                    container_id: log_entry.container_id,
                    ip_address: None,
                    method: None,
                    path: None,
                    status_code: None,
                    response_size: None,
                    user_agent: None,
                    level: captures.get(2).map(|m| m.as_str().to_string()),
                });
            }
        }
        
        // Simple heuristic fallback
        let level = if message_clone.contains("[error]") {
            Some("error".to_string())
        } else if message_clone.contains("[warn]") {
            Some("warn".to_string())
        } else if message_clone.contains("[info]") {
            Some("info".to_string())
        } else {
            Some("unknown".to_string())
        };
        
        Ok(NginxLogEntry {
            service_type: "nginx".to_string(),
            log_type: "error".to_string(),
            message: log_entry.message,
            stream: log_entry.stream,
            timestamp: log_entry.timestamp,
            container_id: log_entry.container_id,
            ip_address: None,
            method: None,
            path: None,
            status_code: None,
            response_size: None,
            user_agent: None,
            level,
        })
    }
}
