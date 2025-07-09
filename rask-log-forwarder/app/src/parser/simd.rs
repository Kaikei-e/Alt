use super::schema::{LogEntry, NginxLogEntry, ParseError};
use bytes::Bytes;
use chrono::{DateTime, Utc};
use lazy_static::lazy_static;
use regex::Regex;
use simd_json::prelude::{ValueAsObject, ValueAsScalar};
use simd_json::{OwnedValue, from_slice};

lazy_static! {
    // Common Log Format: IP - user [timestamp] "METHOD path HTTP/version" status size
    static ref NGINX_ACCESS_REGEX: Regex = {
        match Regex::new(r#"^(\S+) \S+ \S+ \[([^\]]+)\] "(\S+) ([^"]*) HTTP/[^"]*" (\d+) (\d+)"#) {
            Ok(regex) => regex,
            Err(_) => {
                // Fallback to a simpler pattern for nginx access logs
                Regex::new(r#"^(\S+) .+ "(\S+) ([^"]*)" (\d+) (\d+)"#)
                    .expect("Fallback nginx access regex pattern is invalid")
            }
        }
    };

    // Combined Log Format includes referer and user-agent
    static ref NGINX_ACCESS_COMBINED_REGEX: Regex = {
        match Regex::new(r#"^(\S+) \S+ \S+ \[([^\]]+)\] "(\S+) ([^"]*) HTTP/[^"]*" (\d+) (\d+) "([^"]*)" "([^"]*)""#) {
            Ok(regex) => regex,
            Err(_) => {
                // Fallback to a simpler pattern for nginx combined logs
                Regex::new(r#"^(\S+) .+ "(\S+) ([^"]*)" (\d+) (\d+) "([^"]*)" "([^"]*)""#)
                    .expect("Fallback nginx combined regex pattern is invalid")
            }
        }
    };

    // Nginx Error Log Format: timestamp [level] pid#tid: *cid message
    static ref NGINX_ERROR_REGEX: Regex = {
        match Regex::new(r#"^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}) \[(\w+)\] (\d+)#(\d+): (.*?)(?:\n)?$"#) {
            Ok(regex) => regex,
            Err(_) => {
                // Fallback to a simpler pattern for nginx error logs
                Regex::new(r#"^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}) \[(\w+)\] (.*)$"#)
                    .expect("Fallback nginx error regex pattern is invalid")
            }
        }
    };
}

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
        NGINX_ACCESS_REGEX.is_match(message) || NGINX_ACCESS_COMBINED_REGEX.is_match(message)
    }

    fn is_nginx_error_log(&self, message: &str) -> bool {
        NGINX_ERROR_REGEX.is_match(message)
    }

    fn parse_nginx_access_log(&self, log_entry: LogEntry) -> Result<NginxLogEntry, ParseError> {
        let message_clone = log_entry.message.clone();

        // Try combined format first (more fields)
        if let Some(captures) = NGINX_ACCESS_COMBINED_REGEX.captures(&message_clone) {
            Ok(NginxLogEntry {
                service_type: "nginx".to_string(),
                log_type: "access".to_string(),
                message: log_entry.message,
                stream: log_entry.stream,
                timestamp: log_entry.timestamp,
                container_id: log_entry.container_id,
                ip_address: Some(captures[1].to_string()),
                method: Some(captures[3].to_string()),
                path: Some(captures[4].to_string()),
                status_code: Some(captures[5].parse().unwrap_or(0)),
                response_size: Some(captures[6].parse().unwrap_or(0)),
                user_agent: Some(captures[8].to_string()),
                level: None,
            })
        } else if let Some(captures) = NGINX_ACCESS_REGEX.captures(&message_clone) {
            Ok(NginxLogEntry {
                service_type: "nginx".to_string(),
                log_type: "access".to_string(),
                message: log_entry.message,
                stream: log_entry.stream,
                timestamp: log_entry.timestamp,
                container_id: log_entry.container_id,
                ip_address: Some(captures[1].to_string()),
                method: Some(captures[3].to_string()),
                path: Some(captures[4].to_string()),
                status_code: Some(captures[5].parse().unwrap_or(0)),
                response_size: Some(captures[6].parse().unwrap_or(0)),
                user_agent: None,
                level: None,
            })
        } else {
            Err(ParseError::InvalidFormat(
                "Could not parse nginx access log".to_string(),
            ))
        }
    }

    fn parse_nginx_error_log(&self, log_entry: LogEntry) -> Result<NginxLogEntry, ParseError> {
        let message_clone = log_entry.message.clone();

        if let Some(captures) = NGINX_ERROR_REGEX.captures(&message_clone) {
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
                level: Some(captures[2].to_string()),
            })
        } else {
            Err(ParseError::InvalidFormat(
                "Could not parse nginx error log".to_string(),
            ))
        }
    }
}
