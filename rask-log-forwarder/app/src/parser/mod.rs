pub mod docker;
pub mod generated;
pub mod regex_error;
pub mod regex_patterns;
pub mod schema;
pub mod services;
pub mod simd;
pub mod universal;
pub mod zero_alloc_parser;

// Legacy exports (keep for compatibility)
pub use schema::{LogEntry, NginxLogEntry};
pub use simd::SimdParser;

// New TASK2 exports
pub use docker::{DockerJsonParser, DockerLogEntry, ParseError};
pub use services::{
    GoStructuredParser, LogLevel, NginxParser, ParsedLogEntry, PostgresParser, ServiceParser,
};
pub use universal::{EnrichedLogEntry, UniversalParser};

// TASK3 exports - Memory-safe regex patterns
pub use regex_error::{RegexError, FallbackStrategy};
pub use regex_patterns::{StaticRegexSet, SimplePatternParser, NginxAccessMatch};

// TASK5 exports - Zero-allocation parsing
pub use zero_alloc_parser::{ZeroAllocParser, ImprovedNginxParser};

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_module_exports() {
        let _docker_parser = DockerJsonParser::new();
        let _nginx_parser = NginxParser::new();
        let _go_parser = GoStructuredParser::new();
        let _postgres_parser = PostgresParser::new();
        let _universal_parser = UniversalParser::new();
    }
}
