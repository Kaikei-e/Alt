mod cli;
pub mod groups;
pub mod serde_helpers;
mod validation;

use clap::ValueEnum;
use serde::{Deserialize, Serialize};
use thiserror::Error;

#[derive(Error, Debug)]
pub enum ConfigError {
    #[error("Invalid URL: {0}")]
    InvalidUrl(String),
    #[error("Invalid configuration: {0}")]
    InvalidConfig(String),
    #[error("File error: {0}")]
    FileError(#[from] std::io::Error),
    #[error("Parse error: {0}")]
    ParseError(#[from] toml::de::Error),
    #[error("Environment error: {0}")]
    EnvError(String),
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, ValueEnum, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum LogLevel {
    Error,
    Warn,
    Info,
    Debug,
    Trace,
}

impl From<LogLevel> for tracing::Level {
    fn from(level: LogLevel) -> Self {
        match level {
            LogLevel::Error => tracing::Level::ERROR,
            LogLevel::Warn => tracing::Level::WARN,
            LogLevel::Info => tracing::Level::INFO,
            LogLevel::Debug => tracing::Level::DEBUG,
            LogLevel::Trace => tracing::Level::TRACE,
        }
    }
}

/// Protocol for sending log data to the aggregator.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default, ValueEnum, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum Protocol {
    /// NDJSON format (default, legacy)
    #[default]
    Ndjson,
    /// OpenTelemetry Protocol (OTLP) over HTTP with protobuf encoding
    Otlp,
}

/// Returns a warning message when `protocol`'s batch flush path has no
/// `ReliabilityManager` coverage (retry + disk-fallback) - i.e. a transient
/// aggregator failure silently drops the batch instead of retrying it or
/// persisting it to disk. Callers should log this loudly at startup so the
/// gap is visible before it causes silent log loss in production.
///
/// This is a stopgap: the underlying fix is routing OTLP batches through
/// `ReliabilityManager` like NDJSON already does (see `pipeline::flush_batch`).
pub fn reliability_gap_warning(protocol: Protocol) -> Option<&'static str> {
    match protocol {
        Protocol::Otlp => Some(
            "OTLP protocol sends batches directly, bypassing ReliabilityManager's retry \
             and disk-fallback; transient aggregator failures (5xx, network errors, timeouts) \
             will drop the batch instead of retrying it or persisting it to disk",
        ),
        Protocol::Ndjson => None,
    }
}

// Re-export all public items for backward compatibility
pub use cli::Config;
pub use groups::{DiskFallbackConfig, MetricsConfig, RetryConfig};

#[cfg(test)]
mod reliability_gap_warning_tests {
    use super::*;

    #[test]
    fn otlp_protocol_has_a_reliability_gap_warning() {
        assert!(
            reliability_gap_warning(Protocol::Otlp).is_some(),
            "OTLP batches bypass ReliabilityManager (see pipeline::flush_batch); \
             this must be surfaced, not silent"
        );
    }

    #[test]
    fn ndjson_protocol_has_no_reliability_gap_warning() {
        assert_eq!(
            reliability_gap_warning(Protocol::Ndjson),
            None,
            "NDJSON already goes through ReliabilityManager's retry + disk-fallback"
        );
    }
}
