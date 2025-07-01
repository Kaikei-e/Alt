pub mod docker;
pub mod schema;
pub mod services;
pub mod simd;
pub mod universal;

// Legacy exports (keep for compatibility)
pub use schema::{LogEntry, NginxLogEntry};
pub use simd::SimdParser;

// New TASK2 exports
pub use docker::{DockerJsonParser, DockerLogEntry, ParseError};
pub use services::{
    GoStructuredParser, LogLevel, NginxParser, ParsedLogEntry, PostgresParser, ServiceParser,
};
pub use universal::{EnrichedLogEntry, UniversalParser};

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
