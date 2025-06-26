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
    #[arg(long, env = "RASK_ENDPOINT", default_value = "http://rask-aggregator:9600/ingest")]
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
    #[arg(long, env = "DISK_FALLBACK_PATH", default_value = "/tmp/rask-log-forwarder/fallback")]
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
            endpoint: "http://rask-aggregator:9600/ingest".to_string(),
            batch_size: 10000,
            flush_interval_ms: 500,
            buffer_capacity: 100000,
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
        let mut config = Config::default();

        // Load from environment variables
        if let Ok(service) = std::env::var("TARGET_SERVICE") {
            config.target_service = Some(service);
        }

        if let Ok(endpoint) = std::env::var("RASK_ENDPOINT") {
            config.endpoint = endpoint;
        }

        if let Ok(batch_size) = std::env::var("BATCH_SIZE") {
            config.batch_size = batch_size.parse()
                .map_err(|e| ConfigError::EnvError(format!("Invalid BATCH_SIZE: {}", e)))?;
        }

        if let Ok(flush_interval_ms) = std::env::var("FLUSH_INTERVAL_MS") {
            config.flush_interval_ms = flush_interval_ms.parse()
                .map_err(|e| ConfigError::EnvError(format!("Invalid FLUSH_INTERVAL_MS: {}", e)))?;
        }

        if let Ok(buffer_capacity) = std::env::var("BUFFER_CAPACITY") {
            config.buffer_capacity = buffer_capacity.parse()
                .map_err(|e| ConfigError::EnvError(format!("Invalid BUFFER_CAPACITY: {}", e)))?;
        }

        if let Ok(log_level) = std::env::var("LOG_LEVEL") {
            config.log_level = match log_level.to_lowercase().as_str() {
                "error" => LogLevel::Error,
                "warn" => LogLevel::Warn,
                "info" => LogLevel::Info,
                "debug" => LogLevel::Debug,
                "trace" => LogLevel::Trace,
                _ => return Err(ConfigError::EnvError(format!("Invalid LOG_LEVEL: {}", log_level))),
            };
        }

        if let Ok(enable_metrics) = std::env::var("ENABLE_METRICS") {
            config.enable_metrics = enable_metrics.parse()
                .map_err(|e| ConfigError::EnvError(format!("Invalid ENABLE_METRICS: {}", e)))?;
        }

        if let Ok(metrics_port) = std::env::var("METRICS_PORT") {
            config.metrics_port = metrics_port.parse()
                .map_err(|e| ConfigError::EnvError(format!("Invalid METRICS_PORT: {}", e)))?;
        }

        if let Ok(enable_disk_fallback) = std::env::var("ENABLE_DISK_FALLBACK") {
            config.enable_disk_fallback = enable_disk_fallback.parse()
                .map_err(|e| ConfigError::EnvError(format!("Invalid ENABLE_DISK_FALLBACK: {}", e)))?;
        }

        if let Ok(disk_fallback_path) = std::env::var("DISK_FALLBACK_PATH") {
            config.disk_fallback_path = PathBuf::from(disk_fallback_path);
        }

        if let Ok(max_disk_usage_mb) = std::env::var("MAX_DISK_USAGE_MB") {
            config.max_disk_usage_mb = max_disk_usage_mb.parse()
                .map_err(|e| ConfigError::EnvError(format!("Invalid MAX_DISK_USAGE_MB: {}", e)))?;
        }

        if let Ok(connection_timeout_secs) = std::env::var("CONNECTION_TIMEOUT_SECS") {
            config.connection_timeout_secs = connection_timeout_secs.parse()
                .map_err(|e| ConfigError::EnvError(format!("Invalid CONNECTION_TIMEOUT_SECS: {}", e)))?;
        }

        if let Ok(max_connections) = std::env::var("MAX_CONNECTIONS") {
            config.max_connections = max_connections.parse()
                .map_err(|e| ConfigError::EnvError(format!("Invalid MAX_CONNECTIONS: {}", e)))?;
        }

        if let Ok(config_file) = std::env::var("CONFIG_FILE") {
            config.config_file = Some(PathBuf::from(config_file));
        }

        if let Ok(enable_compression) = std::env::var("ENABLE_COMPRESSION") {
            config.enable_compression = enable_compression.parse()
                .map_err(|e| ConfigError::EnvError(format!("Invalid ENABLE_COMPRESSION: {}", e)))?;
        }

        config.post_process()?;
        config.validate()?;
        Ok(config)
    }

    pub fn from_args_and_env<I, T>(args: I) -> Result<Self, ConfigError>
    where
        I: IntoIterator<Item = T>,
        T: Into<std::ffi::OsString> + Clone,
    {
        // Parse CLI args (which automatically includes env vars due to clap's env feature)
        let mut config = Config::parse_from(args);
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
            return Err(ConfigError::InvalidConfig(
                format!("Hostname '{}' doesn't match pattern '*-logs'", hostname)
            ));
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
        if let Ok(hostname) = hostname::get() {
            if let Some(hostname_str) = hostname.to_str() {
                if hostname_str.ends_with("-logs") {
                    let service_name = hostname_str.trim_end_matches("-logs");
                    self.target_service = Some(service_name.to_string());
                    return Ok(());
                }
            }
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
        Url::parse(&self.endpoint)
            .map_err(|e| ConfigError::InvalidUrl(format!("Invalid endpoint URL '{}': {}", self.endpoint, e)))?;

        // Validate batch size
        if self.batch_size == 0 {
            return Err(ConfigError::InvalidConfig("Batch size must be greater than 0".to_string()));
        }

        // Validate buffer capacity
        if self.buffer_capacity < self.batch_size {
            return Err(ConfigError::InvalidConfig(
                format!("Buffer capacity ({}) must be at least as large as batch size ({})",
                        self.buffer_capacity, self.batch_size)
            ));
        }

        // Validate disk fallback path if enabled
        if self.enable_disk_fallback {
            if let Some(parent) = self.disk_fallback_path.parent() {
                if !parent.exists() {
                    return Err(ConfigError::InvalidConfig(
                        format!("Disk fallback parent directory does not exist: {}", parent.display())
                    ));
                }
            }
        }

        // Validate timeouts
        if self.connection_timeout_secs == 0 {
            return Err(ConfigError::InvalidConfig("Connection timeout must be greater than 0".to_string()));
        }

        // Validate retry config
        if self.retry_config.max_attempts == 0 {
            return Err(ConfigError::InvalidConfig("Retry max attempts must be greater than 0".to_string()));
        }

        Ok(())
    }

    pub fn get_target_service(&self) -> Result<String, ConfigError> {
        self.target_service.clone().ok_or_else(|| {
            ConfigError::InvalidConfig("Target service not configured".to_string())
        })
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