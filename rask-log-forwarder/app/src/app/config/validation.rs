use super::{Config, ConfigError, Protocol};
use url::Url;

impl Config {
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

        // D-02: Require ingest_token_file and non-empty ingest_token
        if self.ingest_token_file.is_none() {
            return Err(ConfigError::InvalidConfig(
                "ingest_token_file is required: RASK_INGEST_TOKEN_FILE must be configured"
                    .to_string(),
            ));
        }

        if self.ingest_token.is_empty() {
            return Err(ConfigError::InvalidConfig(
                "ingest_token must not be empty".to_string(),
            ));
        }

        Ok(())
    }
}
