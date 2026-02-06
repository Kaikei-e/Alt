pub mod docker;
pub mod generated;
pub mod regex_error;
pub mod regex_patterns;
pub mod registry;
pub mod schema;
pub mod services;
pub mod simd;
pub mod universal;
pub mod zero_alloc_parser;

pub use schema::{LogEntry, NginxLogEntry};
pub use simd::SimdParser;
pub use docker::{DockerJsonParser, DockerLogEntry, ParseError};
pub use services::{
    GoStructuredParser, LogLevel, MeilisearchParser, NginxParser, ParsedLogEntry, PostgresParser,
    ServiceParser,
};
pub use universal::{EnrichedLogEntry, UniversalParser};
pub use regex_error::{FallbackStrategy, RegexError};
pub use regex_patterns::{NginxAccessMatch, SimplePatternParser, StaticRegexSet};
pub use zero_alloc_parser::{ImprovedNginxParser, ZeroAllocParser};
pub use registry::ServiceParserRegistry;

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
