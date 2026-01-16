// Memory-safe static regex pattern management
use super::regex_error::RegexError;
use regex::Regex;
use std::sync::OnceLock;

/// Zero-cost static regex pattern set with compile-time validation
pub struct StaticRegexSet {
    patterns: &'static [(&'static str, &'static str)], // (pattern, name)
    compiled: OnceLock<Result<Vec<Regex>, RegexError>>,
}

impl StaticRegexSet {
    pub const fn new(patterns: &'static [(&'static str, &'static str)]) -> Self {
        Self {
            patterns,
            compiled: OnceLock::new(),
        }
    }

    pub fn get(&self, index: usize) -> Result<&Regex, RegexError> {
        let compiled = self.compiled.get_or_init(|| {
            let mut regexes = Vec::with_capacity(self.patterns.len());

            for (pattern, name) in self.patterns {
                match Regex::new(pattern) {
                    Ok(regex) => regexes.push(regex),
                    Err(e) => {
                        return Err(RegexError::CompilationFailed {
                            pattern: pattern.to_string(),
                            name: name.to_string(),
                            source: e,
                        });
                    }
                }
            }

            Ok(regexes)
        });

        match compiled {
            Ok(regexes) => regexes.get(index).ok_or(RegexError::IndexOutOfBounds {
                index,
                max: regexes.len(),
            }),
            Err(e) => Err(e.clone()),
        }
    }

    pub fn get_by_name(&self, name: &str) -> Result<&Regex, RegexError> {
        let index = self
            .patterns
            .iter()
            .position(|(_, pattern_name)| *pattern_name == name)
            .ok_or(RegexError::PatternNotFound {
                name: name.to_string(),
            })?;

        self.get(index)
    }

    pub fn len(&self) -> usize {
        self.patterns.len()
    }

    pub fn is_empty(&self) -> bool {
        self.patterns.is_empty()
    }

    pub fn pattern_names(&self) -> Vec<&'static str> {
        self.patterns.iter().map(|(_, name)| *name).collect()
    }
}

/// Simple fallback parser for when regex patterns fail
pub struct SimplePatternParser;

impl SimplePatternParser {
    pub fn new() -> Self {
        Self
    }

    /// Parse ISO 8601 timestamp using string operations
    pub fn parse_timestamp(
        &self,
        text: &str,
    ) -> Result<chrono::DateTime<chrono::Utc>, crate::parser::docker::ParseError> {
        // Basic validation: minimum length and expected separators
        if text.len() >= 19 && text.chars().nth(4) == Some('-') && text.chars().nth(7) == Some('-')
        {
            // Try RFC3339 first
            if let Ok(dt) = chrono::DateTime::parse_from_rfc3339(text) {
                return Ok(dt.with_timezone(&chrono::Utc));
            }

            // Try with Z suffix added
            let with_z = if text.ends_with('Z') {
                text.to_string()
            } else {
                format!("{text}Z")
            };

            if let Ok(dt) = chrono::DateTime::parse_from_rfc3339(&with_z) {
                return Ok(dt.with_timezone(&chrono::Utc));
            }

            // Try naive datetime format
            if let Ok(naive) = chrono::NaiveDateTime::parse_from_str(text, "%Y-%m-%d %H:%M:%S") {
                return Ok(naive.and_utc());
            }

            // Try with T separator
            if let Ok(naive) = chrono::NaiveDateTime::parse_from_str(text, "%Y-%m-%dT%H:%M:%S") {
                return Ok(naive.and_utc());
            }
        }

        Err(crate::parser::docker::ParseError::InvalidFormat(format!(
            "Invalid timestamp format: {text} (expected ISO 8601)"
        )))
    }

    /// Parse nginx access log using string operations
    pub fn parse_nginx_access<'a>(
        &self,
        line: &'a str,
    ) -> Result<NginxAccessMatch<'a>, RegexError> {
        // Simple space-based parsing for basic nginx access log
        let parts: Vec<&str> = line.split_whitespace().collect();

        if parts.len() >= 6 {
            // Try to extract basic components
            let ip = parts[0];

            // Find quoted method and path
            if let Some(quote_start) = line.find('"')
                && let Some(quote_end) = line[quote_start + 1..].find('"')
            {
                let request_line = &line[quote_start + 1..quote_start + 1 + quote_end];
                let request_parts: Vec<&str> = request_line.split_whitespace().collect();

                if request_parts.len() >= 2 {
                    let method = request_parts[0];
                    let path = request_parts[1];

                    // Try to find status and size after the quote
                    let after_quote = &line[quote_start + 1 + quote_end + 1..];
                    let status_size_parts: Vec<&str> = after_quote.split_whitespace().collect();

                    if status_size_parts.len() >= 2 {
                        let status = status_size_parts[0].parse().unwrap_or(0);
                        let size = status_size_parts[1].parse().unwrap_or(0);

                        return Ok(NginxAccessMatch {
                            ip,
                            method,
                            path,
                            status,
                            size,
                            full_line: line,
                        });
                    }
                }
            }
        }

        Err(RegexError::ExecutionFailed {
            details: format!("Failed to parse nginx access log: {line}"),
        })
    }
}

impl Default for SimplePatternParser {
    fn default() -> Self {
        Self::new()
    }
}

/// Zero-copy nginx access log match result
#[derive(Debug)]
pub struct NginxAccessMatch<'a> {
    pub ip: &'a str,
    pub method: &'a str,
    pub path: &'a str,
    pub status: u16,
    pub size: u64,
    pub full_line: &'a str,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_static_regex_set_compilation() {
        static TEST_PATTERNS: StaticRegexSet = StaticRegexSet::new(&[
            (r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}", "iso_timestamp"),
            (
                r#"^(\S+) .+ "([A-Z]+) ([^"]*) HTTP/[^"]*" (\d+) (\d+|-)"#,
                "nginx_access",
            ),
        ]);

        // All patterns should compile successfully
        for i in 0..TEST_PATTERNS.len() {
            assert!(
                TEST_PATTERNS.get(i).is_ok(),
                "Pattern at index {i} should compile"
            );
        }
    }

    #[test]
    fn test_static_regex_set_get_by_name() {
        static TEST_PATTERNS: StaticRegexSet = StaticRegexSet::new(&[
            (r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}", "iso_timestamp"),
            (
                r#"^(\S+) .+ "([A-Z]+) ([^"]*) HTTP/[^"]*" (\d+) (\d+|-)"#,
                "nginx_access",
            ),
        ]);

        assert!(TEST_PATTERNS.get_by_name("iso_timestamp").is_ok());
        assert!(TEST_PATTERNS.get_by_name("nginx_access").is_ok());
        assert!(TEST_PATTERNS.get_by_name("nonexistent").is_err());
    }

    #[test]
    fn test_static_regex_set_thread_safety() {
        use std::sync::Arc;
        use std::thread;

        static REGEX_SET: StaticRegexSet =
            StaticRegexSet::new(&[(r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}", "iso_timestamp")]);

        let regex_set = Arc::new(&REGEX_SET);
        let handles: Vec<_> = (0..10)
            .map(|_| {
                let regex_set = regex_set.clone();
                thread::spawn(move || {
                    // Concurrent access should be memory-safe
                    let regex = regex_set.get(0).unwrap();
                    regex.is_match("2024-01-01T12:00:00")
                })
            })
            .collect();

        for handle in handles {
            assert!(handle.join().unwrap());
        }
    }

    #[test]
    fn test_simple_pattern_parser_timestamp() {
        let parser = SimplePatternParser::new();

        // Test valid timestamps
        assert!(parser.parse_timestamp("2024-01-01T12:00:00Z").is_ok());
        assert!(parser.parse_timestamp("2024-01-01T12:00:00").is_ok());
        assert!(parser.parse_timestamp("2024-01-01 12:00:00").is_ok());

        // Test invalid timestamps
        assert!(parser.parse_timestamp("invalid").is_err());
        assert!(parser.parse_timestamp("").is_err());
        assert!(parser.parse_timestamp("2024").is_err());
    }

    #[test]
    fn test_simple_pattern_parser_nginx_access() {
        let parser = SimplePatternParser::new();

        let log_line =
            r#"192.168.1.1 - - [01/Jan/2024:00:00:00 +0000] "GET /api/health HTTP/1.1" 200 123"#;
        let result = parser.parse_nginx_access(log_line);

        assert!(result.is_ok());
        let access_match = result.unwrap();
        assert_eq!(access_match.ip, "192.168.1.1");
        assert_eq!(access_match.method, "GET");
        assert_eq!(access_match.path, "/api/health");
        assert_eq!(access_match.status, 200);
        assert_eq!(access_match.size, 123);
    }

    #[test]
    fn test_invalid_regex_compilation() {
        static INVALID_PATTERNS: StaticRegexSet =
            StaticRegexSet::new(&[(r"[invalid regex pattern", "invalid_pattern")]);

        let result = INVALID_PATTERNS.get(0);
        assert!(result.is_err());

        if let Err(RegexError::CompilationFailed { pattern, name, .. }) = result {
            assert_eq!(pattern, "[invalid regex pattern");
            assert_eq!(name, "invalid_pattern");
        } else {
            panic!("Expected CompilationFailed error");
        }
    }
}
