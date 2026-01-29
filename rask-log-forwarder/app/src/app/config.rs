use clap::{Parser, ValueEnum};
use serde::{Deserialize, Serialize};
use std::path::{Path, PathBuf};
use std::time::Duration;
use thiserror::Error;
use url::Url;

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

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RetryConfig {
    pub max_attempts: u32,
    #[serde(with = "duration_serde")]
    pub base_delay: Duration,
    #[serde(with = "duration_serde")]
    pub max_delay: Duration,
    pub jitter: bool,
}

impl Default for RetryConfig {
    fn default() -> Self {
        Self {
            max_attempts: 5,
            base_delay: Duration::from_millis(500),
            max_delay: Duration::from_secs(60),
            jitter: true,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DiskFallbackConfig {
    pub enabled: bool,
    pub storage_path: PathBuf,
    pub max_disk_usage_mb: u64,
    pub retention_hours: u64,
    pub compression: bool,
}

impl Default for DiskFallbackConfig {
    fn default() -> Self {
        Self {
            enabled: true,
            storage_path: PathBuf::from("/tmp/rask-log-forwarder/fallback"),
            max_disk_usage_mb: 1000, // 1GB
            retention_hours: 24,
            compression: true,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MetricsConfig {
    pub enabled: bool,
    pub port: u16,
    pub path: String,
}

impl Default for MetricsConfig {
    fn default() -> Self {
        Self {
            enabled: true,
            port: 9090,
            path: "/metrics".to_string(),
        }
    }
}

#[derive(Parser, Debug, Clone, Serialize, Deserialize)]
#[command(author, version, about, long_about = None)]
pub struct Config {
    /// Target service name (auto-detected from hostname if not provided)
    #[arg(long, env = "TARGET_SERVICE")]
    pub target_service: Option<String>,

    /// Rask aggregator endpoint URL
    #[arg(
        long,
        env = "RASK_ENDPOINT",
        default_value = "http://rask-aggregator:9600/v1/aggregate"
    )]
    pub endpoint: String,

    /// Number of log entries per batch
    #[arg(long, env = "BATCH_SIZE", default_value = "10000")]
    pub batch_size: usize,

    /// Flush interval in milliseconds
    #[arg(long, env = "FLUSH_INTERVAL_MS", default_value = "500")]
    pub flush_interval_ms: u64,

    /// Buffer capacity for queuing log entries
    #[arg(long, env = "BUFFER_CAPACITY", default_value = "100000")]
    pub buffer_capacity: usize,

    /// Log level
    #[arg(long, env = "LOG_LEVEL", default_value = "info")]
    pub log_level: LogLevel,

    /// Enable metrics export
    #[arg(long, env = "ENABLE_METRICS")]
    pub enable_metrics: bool,

    /// Metrics export port
    #[arg(long, env = "METRICS_PORT", default_value = "9090")]
    pub metrics_port: u16,

    /// Enable disk fallback for failed transmissions
    #[arg(long, env = "ENABLE_DISK_FALLBACK")]
    pub enable_disk_fallback: bool,

    /// Disk fallback storage path
    #[arg(
        long,
        env = "DISK_FALLBACK_PATH",
        default_value = "/tmp/rask-log-forwarder/fallback"
    )]
    pub disk_fallback_path: PathBuf,

    /// Maximum disk usage for fallback in MB
    #[arg(long, env = "MAX_DISK_USAGE_MB", default_value = "1000")]
    pub max_disk_usage_mb: u64,

    /// Connection timeout in seconds
    #[arg(long, env = "CONNECTION_TIMEOUT_SECS", default_value = "30")]
    pub connection_timeout_secs: u64,

    /// Maximum HTTP connections
    #[arg(long, env = "MAX_CONNECTIONS", default_value = "10")]
    pub max_connections: usize,

    /// Configuration file path (optional)
    #[arg(long, env = "CONFIG_FILE")]
    pub config_file: Option<PathBuf>,

    /// Enable compression for HTTP requests
    #[arg(long, env = "ENABLE_COMPRESSION")]
    pub enable_compression: bool,

    /// Protocol for sending logs (ndjson or otlp)
    #[arg(long, env = "PROTOCOL", default_value = "ndjson")]
    pub protocol: Protocol,

    /// OTLP endpoint URL (used when protocol=otlp)
    #[arg(
        long,
        env = "OTLP_ENDPOINT",
        default_value = "http://rask-log-aggregator:4318/v1/logs"
    )]
    pub otlp_endpoint: String,

    /// Derived fields (not CLI arguments)
    #[serde(skip)]
    #[arg(skip)]
    pub flush_interval: Duration,

    #[serde(skip)]
    #[arg(skip)]
    pub connection_timeout: Duration,

    /// Retry configuration (not exposed as CLI args)
    #[serde(skip)]
    #[arg(skip)]
    pub retry_config: RetryConfig,

    /// Disk fallback configuration (not exposed as CLI args)
    #[serde(skip)]
    #[arg(skip)]
    pub disk_fallback_config: DiskFallbackConfig,

    /// Metrics configuration (not exposed as CLI args)
    #[serde(skip)]
    #[arg(skip)]
    pub metrics_config: MetricsConfig,
}

impl Default for Config {
    fn default() -> Self {
        Self {
            target_service: None,
            endpoint: "http://rask-log-aggregator:9600/v1/aggregate".to_string(),
            batch_size: 10000,
            flush_interval_ms: 500,
            buffer_capacity: 100_000,
            log_level: LogLevel::Info,
            enable_metrics: false,
            metrics_port: 9090,
            enable_disk_fallback: false,
            disk_fallback_path: PathBuf::from("/tmp/rask-log-forwarder/fallback"),
            max_disk_usage_mb: 1000,
            connection_timeout_secs: 30,
            max_connections: 10,
            config_file: None,
            enable_compression: false,
            protocol: Protocol::Ndjson,
            otlp_endpoint: "http://rask-log-aggregator:4318/v1/logs".to_string(),
            flush_interval: Duration::from_millis(500),
            connection_timeout: Duration::from_secs(30),
            retry_config: RetryConfig::default(),
            disk_fallback_config: DiskFallbackConfig::default(),
            metrics_config: MetricsConfig::default(),
        }
    }
}

impl Config {
    pub fn from_args<I, T>(args: I) -> Result<Self, ConfigError>
    where
        I: IntoIterator<Item = T>,
        T: Into<std::ffi::OsString> + Clone,
    {
        let mut config = Config::parse_from(args);
        config.post_process()?;
        config.validate()?;
        Ok(config)
    }

    pub fn from_env() -> Result<Self, ConfigError> {
        // First, try to load from RASK_CONFIG environment variable if it exists
        if let Ok(rask_config) = std::env::var("RASK_CONFIG") {
            return Self::from_rask_config_env(&rask_config);
        }

        let mut config = Config::default();

        // Load from individual environment variables using helpers
        load_env_string_opt("TARGET_SERVICE", &mut config.target_service);
        load_env_string("RASK_ENDPOINT", &mut config.endpoint);
        load_env_var("BATCH_SIZE", &mut config.batch_size)?;
        load_env_var("FLUSH_INTERVAL_MS", &mut config.flush_interval_ms)?;
        load_env_var("BUFFER_CAPACITY", &mut config.buffer_capacity)?;

        // LogLevel requires special handling for case-insensitive parsing
        if let Ok(log_level) = std::env::var("LOG_LEVEL") {
            config.log_level = match log_level.to_lowercase().as_str() {
                "error" => LogLevel::Error,
                "warn" => LogLevel::Warn,
                "info" => LogLevel::Info,
                "debug" => LogLevel::Debug,
                "trace" => LogLevel::Trace,
                _ => {
                    return Err(ConfigError::EnvError(format!(
                        "Invalid LOG_LEVEL: {log_level}"
                    )));
                }
            };
        }

        load_env_var("ENABLE_METRICS", &mut config.enable_metrics)?;
        load_env_var("METRICS_PORT", &mut config.metrics_port)?;
        load_env_var("ENABLE_DISK_FALLBACK", &mut config.enable_disk_fallback)?;
        load_env_path("DISK_FALLBACK_PATH", &mut config.disk_fallback_path);
        load_env_var("MAX_DISK_USAGE_MB", &mut config.max_disk_usage_mb)?;
        load_env_var("CONNECTION_TIMEOUT_SECS", &mut config.connection_timeout_secs)?;
        load_env_var("MAX_CONNECTIONS", &mut config.max_connections)?;
        load_env_path_opt("CONFIG_FILE", &mut config.config_file);
        load_env_var("ENABLE_COMPRESSION", &mut config.enable_compression)?;

        // Protocol requires special handling
        if let Ok(protocol) = std::env::var("PROTOCOL") {
            config.protocol = match protocol.to_lowercase().as_str() {
                "ndjson" => Protocol::Ndjson,
                "otlp" => Protocol::Otlp,
                _ => {
                    return Err(ConfigError::EnvError(format!(
                        "Invalid PROTOCOL: {protocol}. Valid values: ndjson, otlp"
                    )));
                }
            };
        }
        load_env_string("OTLP_ENDPOINT", &mut config.otlp_endpoint);

        config.post_process()?;
        config.validate()?;
        Ok(config)
    }

    pub fn from_args_and_env<I, T>(args: I) -> Result<Self, ConfigError>
    where
        I: IntoIterator<Item = T>,
        T: Into<std::ffi::OsString> + Clone,
    {
        // Start with RASK_CONFIG if available, then override with CLI args
        let base_config = if let Ok(rask_config) = std::env::var("RASK_CONFIG") {
            Self::from_rask_config_env(&rask_config)?
        } else {
            Config::default()
        };

        // Parse CLI args (which automatically includes env vars due to clap's env feature)
        let mut config = Config::parse_from(args);

        // Merge base_config values for fields that weren't explicitly set via CLI
        if config.target_service.is_none() && base_config.target_service.is_some() {
            config.target_service = base_config.target_service;
        }
        if config.endpoint == Config::default().endpoint
            && base_config.endpoint != Config::default().endpoint
        {
            config.endpoint = base_config.endpoint;
        }
        if config.batch_size == Config::default().batch_size
            && base_config.batch_size != Config::default().batch_size
        {
            config.batch_size = base_config.batch_size;
        }
        if config.flush_interval_ms == Config::default().flush_interval_ms
            && base_config.flush_interval_ms != Config::default().flush_interval_ms
        {
            config.flush_interval_ms = base_config.flush_interval_ms;
        }
        if config.buffer_capacity == Config::default().buffer_capacity
            && base_config.buffer_capacity != Config::default().buffer_capacity
        {
            config.buffer_capacity = base_config.buffer_capacity;
        }

        config.post_process()?;
        config.validate()?;
        Ok(config)
    }

    pub fn from_file<P: AsRef<Path>>(path: P) -> Result<Self, ConfigError> {
        let content = std::fs::read_to_string(path)?;
        let mut config: Config = toml::from_str(&content)?;
        config.post_process()?;
        config.validate()?;
        Ok(config)
    }

    pub fn detect_service_from_hostname(hostname: &str) -> Result<Self, ConfigError> {
        let service_name = if hostname.ends_with("-logs") {
            hostname.trim_end_matches("-logs")
        } else {
            return Err(ConfigError::InvalidConfig(format!(
                "Hostname '{hostname}' doesn't match pattern '*-logs'"
            )));
        };

        let mut config = Config {
            target_service: Some(service_name.to_string()),
            ..Config::default()
        };
        config.post_process()?;
        config.validate()?;
        Ok(config)
    }

    pub fn auto_detect_service(&mut self) -> Result<(), ConfigError> {
        if self.target_service.is_some() {
            return Ok(()); // Already configured
        }

        // Try environment variable first
        if let Ok(service) = std::env::var("TARGET_SERVICE") {
            self.target_service = Some(service);
            return Ok(());
        }

        // Try hostname detection
        if let Ok(hostname) = hostname::get()
            && let Some(hostname_str) = hostname.to_str()
            && hostname_str.ends_with("-logs")
        {
            let service_name = hostname_str.trim_end_matches("-logs");
            self.target_service = Some(service_name.to_string());
            return Ok(());
        }

        Err(ConfigError::InvalidConfig(
            "Could not auto-detect target service. Please set TARGET_SERVICE environment variable or use --target-service flag".to_string()
        ))
    }

    pub fn post_process(&mut self) -> Result<(), ConfigError> {
        // Convert milliseconds to Duration
        self.flush_interval = Duration::from_millis(self.flush_interval_ms);
        self.connection_timeout = Duration::from_secs(self.connection_timeout_secs);

        // Update nested configs
        self.disk_fallback_config.enabled = self.enable_disk_fallback;
        self.disk_fallback_config.storage_path = self.disk_fallback_path.clone();
        self.disk_fallback_config.max_disk_usage_mb = self.max_disk_usage_mb;

        self.metrics_config.enabled = self.enable_metrics;
        self.metrics_config.port = self.metrics_port;

        Ok(())
    }

    pub fn validate(&self) -> Result<(), ConfigError> {
        // Validate endpoint URL
        Url::parse(&self.endpoint).map_err(|e| {
            ConfigError::InvalidUrl(format!("Invalid endpoint URL '{}': {}", self.endpoint, e))
        })?;

        // Validate OTLP endpoint URL if protocol is OTLP
        if self.protocol == Protocol::Otlp {
            Url::parse(&self.otlp_endpoint).map_err(|e| {
                ConfigError::InvalidUrl(format!(
                    "Invalid OTLP endpoint URL '{}': {}",
                    self.otlp_endpoint, e
                ))
            })?;
        }

        // Validate batch size
        if self.batch_size == 0 {
            return Err(ConfigError::InvalidConfig(
                "Batch size must be greater than 0".to_string(),
            ));
        }

        // Validate buffer capacity
        if self.buffer_capacity < self.batch_size {
            return Err(ConfigError::InvalidConfig(format!(
                "Buffer capacity ({}) must be at least as large as batch size ({})",
                self.buffer_capacity, self.batch_size
            )));
        }

        // Validate disk fallback path if enabled
        if self.enable_disk_fallback
            && let Some(parent) = self.disk_fallback_path.parent()
            && !parent.exists()
        {
            return Err(ConfigError::InvalidConfig(format!(
                "Disk fallback parent directory does not exist: {}",
                parent.display()
            )));
        }

        // Validate timeouts
        if self.connection_timeout_secs == 0 {
            return Err(ConfigError::InvalidConfig(
                "Connection timeout must be greater than 0".to_string(),
            ));
        }

        // Validate retry config
        if self.retry_config.max_attempts == 0 {
            return Err(ConfigError::InvalidConfig(
                "Retry max attempts must be greater than 0".to_string(),
            ));
        }

        Ok(())
    }

    pub fn get_target_service(&self) -> Result<String, ConfigError> {
        self.target_service
            .clone()
            .ok_or_else(|| ConfigError::InvalidConfig("Target service not configured".to_string()))
    }

    pub fn from_rask_config_env(rask_config: &str) -> Result<Self, ConfigError> {
        let mut config: Config = toml::from_str(rask_config)?;
        config.post_process()?;
        config.validate()?;
        Ok(config)
    }
}

/// Helper function to load and parse an environment variable.
/// Returns Ok(()) if the variable doesn't exist (keeps default).
fn load_env_var<T>(name: &str, target: &mut T) -> Result<(), ConfigError>
where
    T: std::str::FromStr,
    T::Err: std::fmt::Display,
{
    if let Ok(value) = std::env::var(name) {
        *target = value
            .parse()
            .map_err(|e| ConfigError::EnvError(format!("Invalid {name}: {e}")))?;
    }
    Ok(())
}

/// Helper function to load an optional string environment variable.
fn load_env_string_opt(name: &str, target: &mut Option<String>) {
    if let Ok(value) = std::env::var(name) {
        *target = Some(value);
    }
}

/// Helper function to load a string environment variable.
fn load_env_string(name: &str, target: &mut String) {
    if let Ok(value) = std::env::var(name) {
        *target = value;
    }
}

/// Helper function to load a PathBuf environment variable.
fn load_env_path(name: &str, target: &mut PathBuf) {
    if let Ok(value) = std::env::var(name) {
        *target = PathBuf::from(value);
    }
}

/// Helper function to load an optional PathBuf environment variable.
fn load_env_path_opt(name: &str, target: &mut Option<PathBuf>) {
    if let Ok(value) = std::env::var(name) {
        *target = Some(PathBuf::from(value));
    }
}

// Helper module for duration serialization
mod duration_serde {
    use serde::{Deserialize, Deserializer, Serializer};
    use std::time::Duration;

    pub fn serialize<S>(duration: &Duration, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: Serializer,
    {
        serializer.serialize_u64(duration.as_millis() as u64)
    }

    pub fn deserialize<'de, D>(deserializer: D) -> Result<Duration, D::Error>
    where
        D: Deserializer<'de>,
    {
        let millis = u64::deserialize(deserializer)?;
        Ok(Duration::from_millis(millis))
    }
}
