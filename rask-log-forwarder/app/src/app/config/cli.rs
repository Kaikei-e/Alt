use super::groups::{DiskFallbackConfig, MetricsConfig, RetryConfig};
use super::serde_helpers::{
    load_env_path, load_env_path_opt, load_env_string, load_env_string_opt, load_env_var,
};
use super::{ConfigError, LogLevel, Protocol};
use clap::Parser;
use serde::{Deserialize, Serialize};
use std::path::{Path, PathBuf};
use std::time::Duration;

#[derive(Parser, Clone, Serialize, Deserialize)]
#[command(author, version, about, long_about = None)]
#[serde(default)]
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

    /// D-02: Path to the file containing the bearer token for authenticating with aggregator.
    #[arg(long, env = "RASK_INGEST_TOKEN_FILE")]
    pub ingest_token_file: Option<PathBuf>,

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

    /// The loaded D-02 ingest bearer token
    #[serde(skip)]
    #[arg(skip)]
    pub ingest_token: String,
}

impl std::fmt::Debug for Config {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("Config")
            .field("target_service", &self.target_service)
            .field("endpoint", &self.endpoint)
            .field("batch_size", &self.batch_size)
            .field("flush_interval_ms", &self.flush_interval_ms)
            .field("buffer_capacity", &self.buffer_capacity)
            .field("log_level", &self.log_level)
            .field("enable_metrics", &self.enable_metrics)
            .field("metrics_port", &self.metrics_port)
            .field("enable_disk_fallback", &self.enable_disk_fallback)
            .field("disk_fallback_path", &self.disk_fallback_path)
            .field("max_disk_usage_mb", &self.max_disk_usage_mb)
            .field("connection_timeout_secs", &self.connection_timeout_secs)
            .field("max_connections", &self.max_connections)
            .field("config_file", &self.config_file)
            .field("enable_compression", &self.enable_compression)
            .field("protocol", &self.protocol)
            .field("otlp_endpoint", &self.otlp_endpoint)
            .field("ingest_token_file", &self.ingest_token_file)
            .field("flush_interval", &self.flush_interval)
            .field("connection_timeout", &self.connection_timeout)
            .field("retry_config", &self.retry_config)
            .field("disk_fallback_config", &self.disk_fallback_config)
            .field("metrics_config", &self.metrics_config)
            .field("ingest_token", &"[REDACTED]")
            .finish()
    }
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
            ingest_token_file: None,
            flush_interval: Duration::from_millis(500),
            connection_timeout: Duration::from_secs(30),
            retry_config: RetryConfig::default(),
            disk_fallback_config: DiskFallbackConfig::default(),
            metrics_config: MetricsConfig::default(),
            ingest_token: String::new(),
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
            let mut config: Config = toml::from_str(&rask_config)?;
            load_env_path_opt("RASK_INGEST_TOKEN_FILE", &mut config.ingest_token_file);
            config.post_process()?;
            config.validate()?;
            return Ok(config);
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
        load_env_var(
            "CONNECTION_TIMEOUT_SECS",
            &mut config.connection_timeout_secs,
        )?;
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
        load_env_path_opt("RASK_INGEST_TOKEN_FILE", &mut config.ingest_token_file);

        config.post_process()?;
        config.validate()?;
        Ok(config)
    }

    fn parse_unvalidated_from_file<P: AsRef<Path>>(path: P) -> Result<Self, ConfigError> {
        let content = std::fs::read_to_string(path)?;
        let config: Config = toml::from_str(&content)?;
        Ok(config)
    }

    fn parse_unvalidated_from_rask_config_str(rask_config: &str) -> Result<Self, ConfigError> {
        let config: Config = toml::from_str(rask_config)?;
        Ok(config)
    }

    pub fn from_args_and_env<I, T>(args: I) -> Result<Self, ConfigError>
    where
        I: IntoIterator<Item = T>,
        T: Into<std::ffi::OsString> + Clone,
    {
        use clap::{CommandFactory, FromArgMatches, parser::ValueSource};

        let matches = Config::command()
            .try_get_matches_from(args)
            .map_err(|e| ConfigError::InvalidConfig(e.to_string()))?;
        let cli = Config::from_arg_matches(&matches)
            .map_err(|e| ConfigError::InvalidConfig(e.to_string()))?;

        let explicitly_set = |id: &str| -> bool {
            matches!(
                matches.value_source(id),
                Some(ValueSource::CommandLine | ValueSource::EnvVariable)
            )
        };

        // Determine base configuration based on precedence: Env (RASK_CONFIG) > File > Default
        // RASK_CONFIG is evaluated first as per "env > file" precedence document exact existing convention.
        let mut config = if let Ok(rask_config) = std::env::var("RASK_CONFIG") {
            Self::parse_unvalidated_from_rask_config_str(&rask_config)?
        } else if let Some(path) = if explicitly_set("config_file") {
            cli.config_file.as_ref()
        } else {
            None
        } {
            Self::parse_unvalidated_from_file(path)?
        } else {
            Config::default()
        };

        // Overlay explicit CLI arguments and explicitly set Env vars (via clap)
        if explicitly_set("target_service") {
            config.target_service = cli.target_service;
        }
        if explicitly_set("endpoint") {
            config.endpoint = cli.endpoint;
        }
        if explicitly_set("batch_size") {
            config.batch_size = cli.batch_size;
        }
        if explicitly_set("flush_interval_ms") {
            config.flush_interval_ms = cli.flush_interval_ms;
        }
        if explicitly_set("buffer_capacity") {
            config.buffer_capacity = cli.buffer_capacity;
        }
        if explicitly_set("log_level") {
            config.log_level = cli.log_level;
        }
        if explicitly_set("enable_metrics") {
            config.enable_metrics = cli.enable_metrics;
        }
        if explicitly_set("metrics_port") {
            config.metrics_port = cli.metrics_port;
        }
        if explicitly_set("enable_disk_fallback") {
            config.enable_disk_fallback = cli.enable_disk_fallback;
        }
        if explicitly_set("disk_fallback_path") {
            config.disk_fallback_path = cli.disk_fallback_path;
        }
        if explicitly_set("max_disk_usage_mb") {
            config.max_disk_usage_mb = cli.max_disk_usage_mb;
        }
        if explicitly_set("connection_timeout_secs") {
            config.connection_timeout_secs = cli.connection_timeout_secs;
        }
        if explicitly_set("max_connections") {
            config.max_connections = cli.max_connections;
        }
        if explicitly_set("config_file") {
            config.config_file = cli.config_file;
        }
        if explicitly_set("enable_compression") {
            config.enable_compression = cli.enable_compression;
        }
        if explicitly_set("protocol") {
            config.protocol = cli.protocol;
        }
        if explicitly_set("otlp_endpoint") {
            config.otlp_endpoint = cli.otlp_endpoint;
        }
        if explicitly_set("ingest_token_file") {
            config.ingest_token_file = cli.ingest_token_file;
        } else if config.ingest_token_file.is_none() {
            load_env_path_opt("RASK_INGEST_TOKEN_FILE", &mut config.ingest_token_file);
        }

        config.post_process()?;
        config.validate()?;
        Ok(config)
    }

    pub fn from_file<P: AsRef<Path>>(path: P) -> Result<Self, ConfigError> {
        let mut config = Self::parse_unvalidated_from_file(path)?;
        load_env_path_opt("RASK_INGEST_TOKEN_FILE", &mut config.ingest_token_file);
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
        load_env_path_opt("RASK_INGEST_TOKEN_FILE", &mut config.ingest_token_file);
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

        // D-02: Load and validate ingest token
        let path = self.ingest_token_file.as_ref().ok_or_else(|| {
            ConfigError::InvalidConfig(
                "ingest_token_file is required: RASK_INGEST_TOKEN_FILE must be configured"
                    .to_string(),
            )
        })?;
        let raw = std::fs::read_to_string(path).map_err(|e| {
            ConfigError::InvalidConfig(format!(
                "Could not read RASK_INGEST_TOKEN_FILE at {}: {}",
                path.display(),
                e
            ))
        })?;
        let token = raw.trim().to_string();
        if token.is_empty() {
            return Err(ConfigError::InvalidConfig(format!(
                "Ingest token file at {} is empty",
                path.display()
            )));
        }
        if !token.is_ascii() {
            return Err(ConfigError::InvalidConfig(format!(
                "Ingest token at {} contains non-ASCII characters",
                path.display()
            )));
        }
        if token.contains('\r') || token.contains('\n') {
            return Err(ConfigError::InvalidConfig(format!(
                "Ingest token at {} contains embedded CR/LF",
                path.display()
            )));
        }
        if token.bytes().any(|b| b < 0x20 || b == 0x7F) {
            return Err(ConfigError::InvalidConfig(format!(
                "Ingest token at {} contains control characters",
                path.display()
            )));
        }
        if !is_rfc6750_token68(&token) {
            return Err(ConfigError::InvalidConfig(format!(
                "Ingest token at {} does not conform to RFC 6750 token68 syntax",
                path.display()
            )));
        }
        self.ingest_token = token;

        Ok(())
    }

    pub fn get_target_service(&self) -> Result<String, ConfigError> {
        self.target_service
            .clone()
            .ok_or_else(|| ConfigError::InvalidConfig("Target service not configured".to_string()))
    }

    pub fn from_rask_config_env(rask_config: &str) -> Result<Self, ConfigError> {
        let mut config: Config = toml::from_str(rask_config)?;
        load_env_path_opt("RASK_INGEST_TOKEN_FILE", &mut config.ingest_token_file);
        config.post_process()?;
        config.validate()?;
        Ok(config)
    }
}

/// Validates that a token adheres to RFC 6750 Section 2.1 token68 syntax:
/// `1*( ALPHA / DIGIT / "-" / "." / "_" / "~" / "+" / "/" ) *"="`
pub fn is_rfc6750_token68(token: &str) -> bool {
    if token.is_empty() {
        return false;
    }
    let mut non_padding_count = 0usize;
    let mut seen_equal = false;
    for b in token.bytes() {
        if b == b'=' {
            seen_equal = true;
        } else if seen_equal {
            // Once '=' padding is encountered, no subsequent non-'=' characters are allowed.
            return false;
        } else if b.is_ascii_alphanumeric()
            || b == b'-'
            || b == b'.'
            || b == b'_'
            || b == b'~'
            || b == b'+'
            || b == b'/'
        {
            non_padding_count += 1;
        } else {
            return false;
        }
    }
    non_padding_count >= 1
}

#[cfg(test)]
mod tests {
    use super::*;
    use serial_test::serial;
    use std::io::Write;
    use tempfile::NamedTempFile;

    fn write_token_file(content: &str) -> NamedTempFile {
        let mut file = NamedTempFile::new().unwrap();
        write!(file, "{content}").unwrap();
        file
    }

    #[test]
    fn test_token68_padding_only_rejected() {
        assert!(!is_rfc6750_token68("="));
        assert!(!is_rfc6750_token68("=="));
        assert!(!is_rfc6750_token68("==="));
        assert!(!is_rfc6750_token68("===="));
    }

    #[test]
    fn test_token68_malformed_rejected() {
        assert!(!is_rfc6750_token68(""));
        assert!(!is_rfc6750_token68("=abc"));
        assert!(!is_rfc6750_token68("abc=def"));
        assert!(!is_rfc6750_token68("abc==d"));
        assert!(!is_rfc6750_token68("abc def"));
        assert!(!is_rfc6750_token68("abc\ndef"));
        assert!(!is_rfc6750_token68("abc\rdef"));
        assert!(!is_rfc6750_token68("abc$token"));
    }

    #[test]
    fn test_token68_non_ascii_rejected() {
        assert!(!is_rfc6750_token68("token\u{1f600}"));
        assert!(!is_rfc6750_token68("tokén"));
        assert!(!is_rfc6750_token68("トークン"));
    }

    #[test]
    fn test_token68_valid_accepted() {
        assert!(is_rfc6750_token68("a"));
        assert!(is_rfc6750_token68("valid_token"));
        assert!(is_rfc6750_token68("valid-token.123~_"));
        assert!(is_rfc6750_token68("valid+token/abc="));
        assert!(is_rfc6750_token68("valid+token/abc=="));
    }

    // Config Entry Mode 1: Normal CLI args
    #[test]
    #[serial]
    fn test_entry_mode_cli_valid_file() {
        let file = write_token_file("valid-cli-token\n");
        let path = file.path().to_str().unwrap();
        let config =
            Config::from_args_and_env(["rask-log-forwarder", "--ingest-token-file", path]).unwrap();
        assert_eq!(config.ingest_token, "valid-cli-token");
        assert!(!format!("{config:?}").contains("valid-cli-token"));
        assert!(format!("{config:?}").contains("[REDACTED]"));
    }

    #[test]
    #[serial]
    fn test_entry_mode_cli_missing_file() {
        let err = Config::from_args_and_env([
            "rask-log-forwarder",
            "--ingest-token-file",
            "/nonexistent/path/to/token",
        ])
        .unwrap_err();
        assert!(
            err.to_string()
                .contains("Could not read RASK_INGEST_TOKEN_FILE")
        );
    }

    #[test]
    #[serial]
    fn test_entry_mode_cli_empty_file() {
        let file = write_token_file("   \n");
        let path = file.path().to_str().unwrap();
        let err = Config::from_args_and_env(["rask-log-forwarder", "--ingest-token-file", path])
            .unwrap_err();
        assert!(err.to_string().contains("is empty"));
    }

    #[test]
    #[serial]
    fn test_entry_mode_cli_bad_token() {
        let file = write_token_file("bad$token\n");
        let path = file.path().to_str().unwrap();
        let err = Config::from_args_and_env(["rask-log-forwarder", "--ingest-token-file", path])
            .unwrap_err();
        assert!(err.to_string().contains("token68"));
        assert!(!err.to_string().contains("bad$token"));
    }

    // Config Entry Mode 2: Environment variables
    #[test]
    #[serial]
    fn test_entry_mode_env_valid_file() {
        let file = write_token_file("valid-env-token\n");
        unsafe {
            std::env::set_var("RASK_INGEST_TOKEN_FILE", file.path());
        }
        let res = Config::from_env();
        unsafe {
            std::env::remove_var("RASK_INGEST_TOKEN_FILE");
        }
        let config = res.unwrap();
        assert_eq!(config.ingest_token, "valid-env-token");
    }

    #[test]
    #[serial]
    fn test_entry_mode_env_missing_file() {
        unsafe {
            std::env::set_var("RASK_INGEST_TOKEN_FILE", "/nonexistent/token/env");
        }
        let res = Config::from_env();
        unsafe {
            std::env::remove_var("RASK_INGEST_TOKEN_FILE");
        }
        assert!(res.is_err());
    }

    #[test]
    #[serial]
    fn test_entry_mode_env_empty_file() {
        let file = write_token_file("   ");
        unsafe {
            std::env::set_var("RASK_INGEST_TOKEN_FILE", file.path());
        }
        let res = Config::from_env();
        unsafe {
            std::env::remove_var("RASK_INGEST_TOKEN_FILE");
        }
        let err = res.unwrap_err();
        assert!(err.to_string().contains("is empty"));
    }

    #[test]
    #[serial]
    fn test_entry_mode_env_bad_token() {
        let file = write_token_file("bad\x01token");
        unsafe {
            std::env::set_var("RASK_INGEST_TOKEN_FILE", file.path());
        }
        let res = Config::from_env();
        unsafe {
            std::env::remove_var("RASK_INGEST_TOKEN_FILE");
        }
        let err = res.unwrap_err();
        assert!(err.to_string().contains("control characters"));
    }

    fn base_json_config(token_file: Option<&Path>) -> String {
        let mut config = Config::default();
        config.ingest_token_file = token_file.map(|p| p.to_path_buf());
        serde_json::to_string(&config).unwrap()
    }

    // Config Entry Mode 3: JSON deserialization
    #[test]
    fn test_entry_mode_json_valid_file() {
        let file = write_token_file("valid-json-token");
        let json = base_json_config(Some(file.path()));
        let mut config: Config = serde_json::from_str(&json).unwrap();
        config.post_process().unwrap();
        config.validate().unwrap();
        assert_eq!(config.ingest_token, "valid-json-token");
    }

    #[test]
    fn test_entry_mode_json_missing_file_fails_validation() {
        let json = base_json_config(None);
        let mut config: Config = serde_json::from_str(&json).unwrap();
        assert!(config.post_process().is_err());
        assert!(config.validate().is_err());
    }

    #[test]
    fn test_entry_mode_json_empty_file() {
        let file = write_token_file("");
        let json = base_json_config(Some(file.path()));
        let mut config: Config = serde_json::from_str(&json).unwrap();
        let err = config.post_process().unwrap_err();
        assert!(err.to_string().contains("is empty"));
    }

    #[test]
    fn test_entry_mode_json_bad_token() {
        let file = write_token_file("===");
        let json = base_json_config(Some(file.path()));
        let mut config: Config = serde_json::from_str(&json).unwrap();
        let err = config.post_process().unwrap_err();
        assert!(err.to_string().contains("token68"));
    }

    // Config Entry Mode 4: RASK_CONFIG (TOML + env overlay)
    fn full_rask_toml(log_level: &str, batch_size: u64, token_file: &str) -> String {
        format!(
            r#"
endpoint = "http://from-rask:9600/v1/aggregate"
batch_size = {batch_size}
flush_interval_ms = 500
buffer_capacity = 100000
log_level = "{log_level}"
enable_metrics = false
metrics_port = 9090
enable_disk_fallback = false
disk_fallback_path = "/tmp/rask-log-forwarder/fallback"
max_disk_usage_mb = 1000
connection_timeout_secs = 30
max_connections = 10
enable_compression = false
protocol = "ndjson"
otlp_endpoint = "http://rask-log-aggregator:4318/v1/logs"
ingest_token_file = "{token_file}"
"#
        )
    }

    #[test]
    #[serial]
    fn test_entry_mode_rask_config_with_valid_token_file() {
        let file = write_token_file("valid-rask-config-token");
        let toml_str = full_rask_toml("info", 10000, file.path().to_str().unwrap());
        let config = Config::from_rask_config_env(&toml_str).unwrap();
        assert_eq!(config.ingest_token, "valid-rask-config-token");
    }

    #[test]
    #[serial]
    fn test_entry_mode_rask_config_env_overlay_mounted_token_file() {
        // RASK_CONFIG has no ingest_token_file, but mounted env var RASK_INGEST_TOKEN_FILE has it
        let file = write_token_file("mounted-token-from-env");
        let toml_without_token =
            full_rask_toml("info", 10000, "").replace("ingest_token_file = \"\"\n", "");
        unsafe {
            std::env::set_var("RASK_CONFIG", &toml_without_token);
            std::env::set_var("RASK_INGEST_TOKEN_FILE", file.path());
        }
        let res = Config::from_env();
        unsafe {
            std::env::remove_var("RASK_CONFIG");
            std::env::remove_var("RASK_INGEST_TOKEN_FILE");
        }
        let config = res.expect("RASK_INGEST_TOKEN_FILE must overlay RASK_CONFIG");
        assert_eq!(config.ingest_token, "mounted-token-from-env");
    }

    #[test]
    #[serial]
    fn test_entry_mode_rask_config_missing_token_file_fails() {
        let toml_without_token =
            full_rask_toml("info", 10000, "").replace("ingest_token_file = \"\"\n", "");
        unsafe {
            std::env::set_var("RASK_CONFIG", &toml_without_token);
            std::env::remove_var("RASK_INGEST_TOKEN_FILE");
        }
        let res = Config::from_env();
        unsafe {
            std::env::remove_var("RASK_CONFIG");
        }
        assert!(res.is_err());
    }

    #[test]
    #[serial]
    fn explicit_cli_default_batch_size_not_overwritten_by_rask_config() {
        let file = write_token_file("test-token");
        unsafe {
            std::env::set_var(
                "RASK_CONFIG",
                full_rask_toml("info", 42, file.path().to_str().unwrap()),
            );
        }

        let config = Config::from_args_and_env(["rask-log-forwarder", "--batch-size", "10000"])
            .expect("config should parse");

        unsafe {
            std::env::remove_var("RASK_CONFIG");
        }

        assert_eq!(config.batch_size, 10000);
        assert_eq!(config.endpoint, "http://from-rask:9600/v1/aggregate");
    }

    #[test]
    #[serial]
    fn rask_config_log_level_preserved_when_cli_omits_it() {
        let file = write_token_file("test-token");
        unsafe {
            std::env::set_var(
                "RASK_CONFIG",
                full_rask_toml("debug", 42, file.path().to_str().unwrap()),
            );
        }

        let config =
            Config::from_args_and_env(["rask-log-forwarder"]).expect("config should parse");

        unsafe {
            std::env::remove_var("RASK_CONFIG");
        }

        assert_eq!(config.log_level, LogLevel::Debug);
    }
}
