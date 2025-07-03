use super::docker::ParseError;
use chrono::{DateTime, Utc};
use lazy_static::lazy_static;
use regex::Regex;
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

pub trait ServiceParser {
    fn service_type(&self) -> &str;
    fn parse_log(&self, log: &str) -> Result<ParsedLogEntry, ParseError>;
}

// Nginx Access Log Parser
pub struct NginxParser {
    access_regex: Regex,
    error_regex: Regex,
}

lazy_static! {
    static ref NGINX_ACCESS_PATTERN: Regex = Regex::new(
        r#"^(\S+) \S+ \S+ \[([^\]]+)\] "([A-Z]+) ([^"]*) HTTP/[^"]*" (\d+) (\d+|-)(?: "([^"]*)" "([^"]*)")?.*$"#
    ).unwrap();

    static ref NGINX_ERROR_PATTERN: Regex = Regex::new(
        r#"^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}) \[(\w+)\] \d+#\d+: (.+)"#
    ).unwrap();
}

impl Default for NginxParser {
    fn default() -> Self {
        Self::new()
    }
}

impl NginxParser {
    pub fn new() -> Self {
        Self {
            access_regex: NGINX_ACCESS_PATTERN.clone(),
            error_regex: NGINX_ERROR_PATTERN.clone(),
        }
    }
}

impl ServiceParser for NginxParser {
    fn service_type(&self) -> &str {
        "nginx"
    }

    fn parse_log(&self, log: &str) -> Result<ParsedLogEntry, ParseError> {
        // Try access log format first
        if let Some(captures) = self.access_regex.captures(log) {
            let ip = captures.get(1).map(|m| m.as_str().to_string());
            let method = captures.get(3).map(|m| m.as_str().to_string());
            let path = captures.get(4).map(|m| m.as_str().to_string());
            let status = captures.get(5).and_then(|m| m.as_str().parse().ok());
            let size = captures.get(6).and_then(|m| {
                if m.as_str() == "-" {
                    None
                } else {
                    m.as_str().parse().ok()
                }
            });
            let user_agent = captures.get(8).map(|m| m.as_str().to_string());

            return Ok(ParsedLogEntry {
                service_type: "nginx".to_string(),
                log_type: "access".to_string(),
                message: log.to_string(),
                level: Some(LogLevel::Info),
                timestamp: None, // TODO: parse timestamp
                stream: "stdout".to_string(),
                method,
                path,
                status_code: status,
                response_size: size,
                ip_address: ip,
                user_agent,
                fields: std::collections::HashMap::new(),
            });
        }

        // Try error log format
        if let Some(captures) = self.error_regex.captures(log) {
            let level_str = captures.get(2).map(|m| m.as_str()).unwrap_or("info");
            let level = match level_str {
                "debug" => LogLevel::Debug,
                "info" => LogLevel::Info,
                "warn" | "warning" => LogLevel::Warn,
                "error" => LogLevel::Error,
                "crit" | "critical" => LogLevel::Fatal,
                _ => LogLevel::Info,
            };

            let message = captures
                .get(3)
                .map(|m| m.as_str())
                .unwrap_or(log)
                .to_string();

            return Ok(ParsedLogEntry {
                service_type: "nginx".to_string(),
                log_type: "error".to_string(),
                message,
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

        // Fallback - but first check if it looks like an access log
        if log.contains("HTTP/") && log.contains("\"") {
            return Ok(ParsedLogEntry {
                service_type: "nginx".to_string(),
                log_type: "access".to_string(),
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
            });
        }

        // Final fallback to plain text
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
            if let Ok(json) = serde_json::from_str::<Value>(potential_json_slice) {
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
                        .unwrap_or(log)
                        .to_string();

                    let method = obj
                        .get("method")
                        .and_then(|v| v.as_str())
                        .map(|s| s.to_string());
                    let path = obj
                        .get("path")
                        .and_then(|v| v.as_str())
                        .map(|s| s.to_string());
                    let status_code = obj.get("status").and_then(|v| v.as_u64()).map(|n| n as u16);

                    let mut fields = std::collections::HashMap::new();
                    for (key, value) in obj {
                        if !["level", "msg", "message", "method", "path", "status"]
                            .contains(&key.as_str())
                        {
                            fields.insert(key.clone(), value.to_string());
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
}

// Python Structured Log Parser
// pub struct PythonParser;

// impl Default for PythonParser {
//     fn default() -> Self {
//         Self::new()
//     }
// }

// impl PythonParser {
//     pub fn new() -> Self {
//         Self
//     }
// }

// impl ServiceParser for PythonParser {
//     fn service_type(&self) -> &str {
//         "python"
//     }

//     fn parse_log(&self, log: &str) -> Result<ParsedLogEntry, ParseError> {
//                 // Try to capture `timestamp JSON` or just `JSON`
//         if let Some(caps) = self.log_pattern.captures(log) {
//             // Optional RFC3339 timestamp (convert to UTC if present)
//             let timestamp = caps.get(1).and_then(|m| DateTime::parse_from_rfc3339(m.as_str()).ok()).map(|dt| dt.with_timezone(&Utc));

//             // JSON body is always capture group 2 (or the whole log as fallback)
//             let json_str = caps.get(2).map(|m| m.as_str()).unwrap_or(log);

//             if let Ok(Value::Object(obj)) = serde_json::from_str::<Value>(json_str) {
//                 // Determine log level
//                 let level_str = obj
//                     .get("level")
//                     .and_then(|v| v.as_str())
//                     .unwrap_or("info")
//                     .to_lowercase();
//                 let level = match level_str.as_str() {
//                     "debug" => LogLevel::Debug,
//                     "info" => LogLevel::Info,
//                     "warn" | "warning" => LogLevel::Warn,
//                     "error" => LogLevel::Error,
//                     "fatal" | "panic" => LogLevel::Fatal,
//                     _ => LogLevel::Info,
//                 };

//                 // Extract common fields
//                 let message = obj
//                     .get("msg")
//                     .or_else(|| obj.get("message"))
//                     .and_then(|v| v.as_str())
//                     .unwrap_or(log)
//                     .to_string();

//                 let method = obj
//                     .get("method")
//                     .and_then(|v| v.as_str())
//                     .map(|s| s.to_string());
//                 let path = obj
//                     .get("path")
//                     .and_then(|v| v.as_str())
//                     .map(|s| s.to_string());
//                 let status_code = obj.get("status").and_then(|v| v.as_u64()).map(|n| n as u16);

//                 // Preserve extra fields
//                 let mut fields = std::collections::HashMap::new();
//                 for (k, v) in obj.iter() {
//                     if !["level", "msg", "message", "method", "path", "status"]
//                         .contains(&k.as_str())
//                     {
//                         fields.insert(k.clone(), v.to_string());
//                     }
//                 }

//                 return Ok(ParsedLogEntry {
//                     service_type: "go".to_string(),
//                     log_type: "structured".to_string(),
//                     message,
//                     level: Some(level),
//                     timestamp,
//                     stream: "stdout".to_string(),
//                     method,
//                     path,
//                     status_code,
//                     response_size: None,
//                     ip_address: None,
//                     user_agent: None,
//                     fields,
//                 });
//             }
//         }

//         // --- Plain-text fallback ---
//         Ok(ParsedLogEntry {
//             service_type: "go".to_string(),
//             log_type: "plain".to_string(),
//             message: log.to_string(),
//             level: Some(LogLevel::Info),
//             timestamp: None,
//             stream: "stdout".to_string(),
//             method: None,
//             path: None,
//             status_code: None,
//             response_size: None,
//             ip_address: None,
//             user_agent: None,
//             fields: std::collections::HashMap::new(),
//         })
//     }
// }

// PostgreSQL Log Parser
pub struct PostgresParser {
    log_regex: Regex,
}

lazy_static! {
    static ref POSTGRES_LOG_PATTERN: Regex =
        Regex::new(r#"^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d+) \w+ \[\d+\] (\w+):\s+(.+)"#)
            .unwrap();
}

impl Default for PostgresParser {
    fn default() -> Self {
        Self::new()
    }
}

impl PostgresParser {
    pub fn new() -> Self {
        Self {
            log_regex: POSTGRES_LOG_PATTERN.clone(),
        }
    }
}

impl ServiceParser for PostgresParser {
    fn service_type(&self) -> &str {
        "postgres"
    }

    fn parse_log(&self, log: &str) -> Result<ParsedLogEntry, ParseError> {
        if let Some(captures) = self.log_regex.captures(log) {
            let level_str = captures.get(2).map(|m| m.as_str()).unwrap_or("LOG");
            let level = match level_str {
                "DEBUG" | "DEBUG1" | "DEBUG2" | "DEBUG3" | "DEBUG4" | "DEBUG5" => LogLevel::Debug,
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

            Ok(ParsedLogEntry {
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
            })
        } else {
            // Fallback
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
        }
    }
}

// pub struct PythonParser {
//     log_regex: Regex,
// }

// lazy_static! {
//     static ref PYTHON_LOG_PATTERN: Regex =
//         Regex::new(r#"^(?:(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z)\s)?(\{.*\})$"#).unwrap();
// }

// impl Default for PythonParser {
//     fn default() -> Self {
//         Self::new()
//     }
// }

// impl PythonParser {
//     pub fn new() -> Self {
//         Self {
//             log_regex: PYTHON_LOG_PATTERN.clone(),
//         }
//     }
// }

// impl ServiceParser for PythonParser {
//     fn service_type(&self) -> &str {
//         "python"
//     }

//     // actual log example: 2025-07-03T16:04:53.555968377Z {"keywords": ["\u60c5\u5831", "\u5229\u7528", "\u5c0e\u5165", "\u5165\u529b", "\u78ba\u8a8d", "\u7ba1\u7406", "\u89b3\u70b9", "\u6a5f\u80fd", "\u5224\u65ad", "\u500b\u4eba"], "level": "info", "logger": "tag_extractor.extract", "msg": "Extraction successful", "service": "tag-generator", "taskName": null, "timestamp": "iso"}
//     // "2025-07-03T16:04:51.375962475Z {""lang"": ""en"", ""level"": ""info"", ""logger"": ""tag_extractor.extract"", ""msg"": ""Detected language"", ""service"": ""tag-generator"", ""taskName"": null, ""timestamp"": ""iso""}"
//     // "2025-07-03T16:04:51.372044691Z {""char_count"": 2730, ""level"": ""info"", ""logger"": ""tag_extractor.extract"", ""msg"": ""Processing text"", ""service"": ""tag-generator"", ""taskName"": null, ""timestamp"": ""iso""}"
//     // "2025-07-03T16:04:51.372022662Z {""keywords"": [""watches"", ""watchos"", ""apple"", ""iphone"", ""versions"", ""beta"", ""warning"", ""incompatibility"", ""crashes""], ""level"": ""info"", ""logger"": ""tag_extractor.extract"", ""msg"": ""Extraction successful"", ""service"": ""tag-generator"", ""taskName"": null, ""timestamp"": ""iso""}"
//     // "2025-07-03T16:04:50.915300394Z {""lang"": ""en"", ""level"": ""info"", ""logger"": ""tag_extractor.extract"", ""msg"": ""Detected language"", ""service"": ""tag-generator"", ""taskName"": null, ""timestamp"": ""iso""}"
//     // "2025-07-03T16:04:50.911359550Z {""char_count"": 1771, ""level"": ""info"", ""logger"": ""tag_extractor.extract"", ""msg"": ""Processing text"", ""service"": ""tag-generator"", ""taskName"": null, ""timestamp"": ""iso""}"
//     // "2025-07-03T16:04:50.911309112Z {""level"": ""info"", ""logger"": ""__main__"", ""msg"": ""Extracted tags for 40/75 articles..."", ""service"": ""tag-generator"", ""taskName"": null, ""timestamp"": ""iso""}"
//     // "2025-07-03T16:04:50.911253858Z {""keywords"": [""calendar"", ""icon"", ""overlooked"", ""hidden"", ""android"", ""gemini"", ""app"", ""display"", ""event"", ""exciting""], ""level"": ""info"", ""logger"": ""tag_extractor.extract"", ""msg"": ""Extraction successful"", ""service"": ""tag-generator"", ""taskName"": null, ""timestamp"": ""iso""}"

//     fn parse_log(&self, log: &str) -> Result<ParsedLogEntry, ParseError> {
//         if let Some(captures) = self.log_regex.captures(log) {
//             let timestamp = captures
//                 .get(1)
//                 .and_then(|m| DateTime::parse_from_rfc3339(m.as_str()).ok())
//                 .map(|dt| dt.with_timezone(&Utc));

//             let json_str = captures.get(2).map(|m| m.as_str()).unwrap_or(log);

//             if let Ok(Value::Object(obj)) = serde_json::from_str::<Value>(json_str) {
//                 let level_str = obj.get("level").and_then(|v| v.as_str()).unwrap_or("info");
//                 let level = match level_str {
//                     "debug" => LogLevel::Debug,
//                     "info" => LogLevel::Info,
//                     "warn" | "warning" => LogLevel::Warn,
//                     "error" => LogLevel::Error,
//                     "fatal" | "panic" => LogLevel::Fatal,
//                     _ => LogLevel::Info,
//                 };

//                 let message = obj
//                     .get("msg")
//                     .and_then(|v| v.as_str())
//                     .unwrap_or(log)
//                     .to_string();

//                 let method = obj
//                     .get("method")
//                     .and_then(|v| v.as_str())
//                     .map(|s| s.to_string());
//                 let path = obj
//                     .get("path")
//                     .and_then(|v| v.as_str())
//                     .map(|s| s.to_string());
//                 let status_code = obj.get("status").and_then(|v| v.as_u64()).map(|n| n as u16);

//                 let mut fields = std::collections::HashMap::new();
//                 for (k, v) in obj.iter() {
//                     if !["level", "msg", "message", "method", "path", "status"]
//                         .contains(&k.as_str())
//                     {
//                         fields.insert(k.clone(), v.to_string());
//                     }
//                 }

//                 return Ok(ParsedLogEntry {
//                     service_type: "python".to_string(),
//                     log_type: "structured".to_string(),
//                     message,
//                     level: Some(level),
//                     timestamp,
//                     stream: "stdout".to_string(),
//                     method,
//                     path,
//                     status_code,
//                     response_size: None,
//                     ip_address: None,
//                     user_agent: None,
//                     fields,
//                 });
//             }

//             Ok(ParsedLogEntry {
//                 service_type: "python".to_string(),
//                 log_type: "unknown".to_string(),
//                 message: log.to_string(),
//                 level: Some(LogLevel::Info),
//                 timestamp: None,
//                 stream: "stdout".to_string(),
//                 method: None,
//                 path: None,
//                 status_code: None,
//                 response_size: None,
//                 ip_address: None,
//                 user_agent: None,
//                 fields: std::collections::HashMap::new(),
//             })
//         }
//     }
// }

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

        let nginx_error = r#"2024/01/01 12:00:00 [error] 123#0: *456 connect() failed (111: Connection refused) while connecting to upstream"#;

        let entry = parser.parse_log(nginx_error).unwrap();

        assert_eq!(entry.log_type, "error");
        assert_eq!(entry.level, Some(LogLevel::Error));
        assert!(entry.message.contains("Connection refused"));
    }

    #[test]
    fn test_go_structured_log_with_docker_timestamp_prefix() {
        let parser = GoStructuredParser::new();

        // Dockerが付与したタイムスタンプ付きのログをシミュレート
        let log_with_prefix = r#"2025-07-03T16:27:09.758077205Z {"level":"info","msg":"Got articles for summarization","service":"pre-processor"}"#;

        let entry = parser.parse_log(log_with_prefix).unwrap();

        // log_typeが "structured" として正しくパースされることを確認
        assert_eq!(entry.log_type, "structured");
        assert_eq!(entry.service_type, "go");
        assert_eq!(entry.level, Some(LogLevel::Info));
        assert_eq!(entry.message, "Got articles for summarization");

        // プレフィックスがない純粋なJSONでもパースできることを確認
        let log_without_prefix = r#"{"level":"info","msg":"pure json log"}"#;
        let entry_no_prefix = parser.parse_log(log_without_prefix).unwrap();
        assert_eq!(entry_no_prefix.log_type, "structured");
        assert_eq!(entry_no_prefix.message, "pure json log");
    }

    #[test]
    fn test_postgres_log_parsing() {
        let parser = PostgresParser::new();

        let pg_log = r#"2024-01-01 12:00:00.123 UTC [123] LOG:  statement: SELECT * FROM users WHERE id = $1"#;

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
}
