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

// Re-export all public items for backward compatibility
pub use cli::Config;
pub use groups::{DiskFallbackConfig, MetricsConfig, RetryConfig};
