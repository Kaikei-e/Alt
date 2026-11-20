use super::{
    generated::{VALIDATED_PATTERNS, pattern_index},
    regex_patterns::SimplePatternParser,
    schema::{LogEntry, NginxLogEntry, ParseError},
    zero_alloc_parser::ImprovedNginxParser,
};
use bytes::Bytes;
use chrono::{DateTime, Utc};
use simd_json::prelude::{ValueAsObject, ValueAsScalar};
use simd_json::{OwnedValue, from_slice};

pub struct SimdParser {}

impl Default for SimdParser {
    fn default() -> Self {
        Self::new()
    }
}

impl SimdParser {
    #[must_use]
    pub fn new() -> Self {
        Self {}
    }

    /// Parses a Docker JSON log line into a [`LogEntry`].
    ///
    /// # Errors
    ///
    /// Returns [`ParseError::InvalidJson`] if `bytes` is not valid JSON, [`ParseError::InvalidFormat`]
    /// if the JSON root is not an object, [`ParseError::MissingField`] if `"log"` or `"time"` is
    /// missing, or [`ParseError::InvalidTimestamp`] if `"time"` is not a valid RFC 3339 timestamp.
    pub fn parse_docker_log(&self, bytes: &Bytes) -> Result<LogEntry, ParseError> {
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

    /// Parses a Docker JSON log line whose message is an nginx access or error log.
    ///
    /// # Errors
    ///
    /// Propagates any [`ParseError`] from [`SimdParser::parse_docker_log`], or returns
    /// [`ParseError::InvalidFormat`] if the message does not match a recognized nginx access or
    /// error format.
    pub fn parse_nginx_log(&self, bytes: &Bytes) -> Result<NginxLogEntry, ParseError> {
        let log_entry = self.parse_docker_log(bytes)?;

        // Detect if this is an nginx access log
        if Self::is_nginx_access_log(&log_entry.message) {
            Self::parse_nginx_access_log(log_entry)
        } else if Self::is_nginx_error_log(&log_entry.message) {
            Ok(Self::parse_nginx_error_log(log_entry))
        } else {
            Err(ParseError::InvalidFormat(
                "Not a recognized nginx log format".to_string(),
            ))
        }
    }

    fn is_nginx_access_log(message: &str) -> bool {
        // Try SIMD nginx access patterns
        if let Ok(regex) = VALIDATED_PATTERNS.get(pattern_index::SIMD_NGINX_ACCESS)
            && regex.is_match(message)
        {
            return true;
        }
        if let Ok(regex) = VALIDATED_PATTERNS.get(pattern_index::SIMD_NGINX_COMBINED)
            && regex.is_match(message)
        {
            return true;
        }
        // Try fallback patterns
        if let Ok(regex) = VALIDATED_PATTERNS.get(pattern_index::SIMD_NGINX_ACCESS_FALLBACK)
            && regex.is_match(message)
        {
            return true;
        }
        if let Ok(regex) = VALIDATED_PATTERNS.get(pattern_index::SIMD_NGINX_COMBINED_FALLBACK)
            && regex.is_match(message)
        {
            return true;
        }

        // Simple heuristic as final fallback
        message.contains("HTTP/") && message.contains('"')
    }

    fn is_nginx_error_log(message: &str) -> bool {
        // Try SIMD nginx error patterns
        if let Ok(regex) = VALIDATED_PATTERNS.get(pattern_index::SIMD_NGINX_ERROR)
            && regex.is_match(message)
        {
            return true;
        }
        if let Ok(regex) = VALIDATED_PATTERNS.get(pattern_index::SIMD_NGINX_ERROR_FALLBACK)
            && regex.is_match(message)
        {
            return true;
        }

        // Simple heuristic as final fallback
        message.contains("[error]") || message.contains("[warn]") || message.contains("[info]")
    }

    fn nginx_access_from_captures(
        log_entry: LogEntry,
        captures: &regex::Captures<'_>,
        method_idx: usize,
        path_idx: usize,
        status_idx: usize,
        size_idx: usize,
        ua_idx: Option<usize>,
    ) -> NginxLogEntry {
        let (status_code, _) = ImprovedNginxParser::parse_status_and_size_safe(
            captures.get(status_idx).map(|m| m.as_str()),
            None,
        );
        let (_, response_size) = ImprovedNginxParser::parse_status_and_size_safe(
            None,
            captures.get(size_idx).map(|m| m.as_str()),
        );
        NginxLogEntry {
            service_type: "nginx".to_string(),
            log_type: "access".to_string(),
            message: log_entry.message,
            stream: log_entry.stream,
            timestamp: log_entry.timestamp,
            container_id: log_entry.container_id,
            ip_address: captures.get(1).map(|m| m.as_str().to_string()),
            method: captures.get(method_idx).map(|m| m.as_str().to_string()),
            path: captures.get(path_idx).map(|m| m.as_str().to_string()),
            status_code,
            response_size,
            user_agent: ua_idx.and_then(|i| captures.get(i).map(|m| m.as_str().to_string())),
            level: None,
        }
    }

    fn parse_nginx_access_log(log_entry: LogEntry) -> Result<NginxLogEntry, ParseError> {
        let message_clone = log_entry.message.clone();

        // Try combined format first (more fields)
        if let Ok(regex) = VALIDATED_PATTERNS.get(pattern_index::SIMD_NGINX_COMBINED)
            && let Some(captures) = regex.captures(&message_clone)
        {
            return Ok(Self::nginx_access_from_captures(
                log_entry,
                &captures,
                3,
                4,
                5,
                6,
                Some(8),
            ));
        }

        // Try regular access format
        if let Ok(regex) = VALIDATED_PATTERNS.get(pattern_index::SIMD_NGINX_ACCESS)
            && let Some(captures) = regex.captures(&message_clone)
        {
            return Ok(Self::nginx_access_from_captures(
                log_entry, &captures, 3, 4, 5, 6, None,
            ));
        }

        // Try fallback patterns
        if let Ok(regex) = VALIDATED_PATTERNS.get(pattern_index::SIMD_NGINX_COMBINED_FALLBACK)
            && let Some(captures) = regex.captures(&message_clone)
        {
            return Ok(Self::nginx_access_from_captures(
                log_entry,
                &captures,
                2,
                3,
                4,
                5,
                Some(7),
            ));
        }

        if let Ok(regex) = VALIDATED_PATTERNS.get(pattern_index::SIMD_NGINX_ACCESS_FALLBACK)
            && let Some(captures) = regex.captures(&message_clone)
        {
            return Ok(Self::nginx_access_from_captures(
                log_entry, &captures, 2, 3, 4, 5, None,
            ));
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

    fn parse_nginx_error_log(log_entry: LogEntry) -> NginxLogEntry {
        let message_clone = log_entry.message.clone();

        // Try SIMD nginx error pattern
        if let Ok(regex) = VALIDATED_PATTERNS.get(pattern_index::SIMD_NGINX_ERROR)
            && let Some(captures) = regex.captures(&message_clone)
        {
            return NginxLogEntry {
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
            };
        }

        // Try fallback error pattern
        if let Ok(regex) = VALIDATED_PATTERNS.get(pattern_index::SIMD_NGINX_ERROR_FALLBACK)
            && let Some(captures) = regex.captures(&message_clone)
        {
            return NginxLogEntry {
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
            };
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

        NginxLogEntry {
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
        }
    }
}
