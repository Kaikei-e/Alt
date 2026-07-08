use std::env;
use std::fs;

use crate::error::AggregatorError;

#[derive(Debug)]
pub struct Settings {
    pub clickhouse_host: String,
    pub clickhouse_port: u16,
    pub clickhouse_user: String,
    pub clickhouse_password: String,
    pub clickhouse_database: String,
    /// Main HTTP server port (legacy aggregate + health)
    pub http_port: u16,
    /// OTLP HTTP server port (traces/logs)
    pub otlp_http_port: u16,
}

impl Settings {
    /// Validates the settings and returns an error if invalid.
    pub fn validate(&self) -> Result<(), AggregatorError> {
        validate_host(&self.clickhouse_host)?;
        validate_port(self.clickhouse_port)?;
        validate_port(self.http_port)?;
        validate_port(self.otlp_http_port)?;
        Ok(())
    }
}

/// Validates that the host is not empty or whitespace-only.
fn validate_host(host: &str) -> Result<(), AggregatorError> {
    if host.trim().is_empty() {
        return Err(AggregatorError::Config("Host cannot be empty".into()));
    }
    Ok(())
}

/// Validates that the port is in valid range (1-65535).
fn validate_port(port: u16) -> Result<(), AggregatorError> {
    if port == 0 {
        return Err(AggregatorError::Config("Port cannot be 0".into()));
    }
    Ok(())
}

/// Read a required environment variable, naming it in the error so a
/// missing-config failure is diagnosable instead of a bare "not found".
fn require_env(env_name: &str) -> Result<String, AggregatorError> {
    env::var(env_name)
        .map_err(|_| AggregatorError::Config(format!("Missing required environment variable: {env_name}")))
}

/// Read a required `u16` environment variable, with a default fallback.
fn env_port_or(env_name: &str, default: u16) -> Result<u16, AggregatorError> {
    match env::var(env_name) {
        Ok(value) => value.parse::<u16>().map_err(|e| {
            AggregatorError::Config(format!("Invalid {env_name} (must be a valid port): {e}"))
        }),
        Err(_) => Ok(default),
    }
}

/// Read a value from environment variable, with support for _FILE suffix (Docker Secrets)
fn get_env_or_file(env_name: &str) -> Result<String, AggregatorError> {
    // First check for _FILE suffix (Docker Secrets support)
    let file_env = format!("{env_name}_FILE");
    if let Ok(file_path) = env::var(&file_env) {
        return fs::read_to_string(&file_path)
            .map(|content| content.trim().to_string())
            .map_err(|e| AggregatorError::Config(format!("Failed to read {file_env}: {e}")));
    }

    // Fallback to standard environment variable
    require_env(env_name).map_err(|_| {
        AggregatorError::Config(format!(
            "Missing required environment variable: {env_name} or {file_env}"
        ))
    })
}

pub fn get_configuration() -> Result<Settings, AggregatorError> {
    let clickhouse_host = require_env("APP_CLICKHOUSE_HOST")?;
    let clickhouse_port = require_env("APP_CLICKHOUSE_PORT")?.parse::<u16>().map_err(|e| {
        AggregatorError::Config(format!("Invalid APP_CLICKHOUSE_PORT (must be a valid port): {e}"))
    })?;
    let clickhouse_user = require_env("APP_CLICKHOUSE_USER")?;
    let clickhouse_password = get_env_or_file("APP_CLICKHOUSE_PASSWORD")?;
    let clickhouse_database = require_env("APP_CLICKHOUSE_DATABASE")?;

    // Server ports with defaults
    let http_port = env_port_or("HTTP_PORT", 9600)?;
    let otlp_http_port = env_port_or("OTLP_HTTP_PORT", 4318)?;

    let settings = Settings {
        clickhouse_host,
        clickhouse_port,
        clickhouse_user,
        clickhouse_password,
        clickhouse_database,
        http_port,
        otlp_http_port,
    };

    // Validate settings before returning
    settings.validate()?;

    Ok(settings)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_validate_port_valid() {
        assert!(validate_port(80).is_ok());
        assert!(validate_port(443).is_ok());
        assert!(validate_port(8123).is_ok());
        assert!(validate_port(65535).is_ok());
        assert!(validate_port(1).is_ok());
    }

    #[test]
    fn test_validate_port_zero_fails() {
        let result = validate_port(0);
        assert!(result.is_err());
        let err = result.unwrap_err();
        assert!(err.to_string().contains("Port cannot be 0"));
    }

    #[test]
    fn test_validate_host_valid() {
        assert!(validate_host("localhost").is_ok());
        assert!(validate_host("192.168.1.1").is_ok());
        assert!(validate_host("clickhouse.example.com").is_ok());
        assert!(validate_host("ch").is_ok());
    }

    #[test]
    fn test_validate_host_empty_fails() {
        let result = validate_host("");
        assert!(result.is_err());
        let err = result.unwrap_err();
        assert!(err.to_string().contains("Host cannot be empty"));
    }

    #[test]
    fn test_validate_host_whitespace_fails() {
        let result = validate_host("   ");
        assert!(result.is_err());
        let err = result.unwrap_err();
        assert!(err.to_string().contains("Host cannot be empty"));
    }

    #[test]
    fn test_settings_validate_success() {
        let settings = Settings {
            clickhouse_host: "localhost".into(),
            clickhouse_port: 8123,
            clickhouse_user: "default".into(),
            clickhouse_password: String::new(),
            clickhouse_database: "default".into(),
            http_port: 9600,
            otlp_http_port: 4318,
        };
        assert!(settings.validate().is_ok());
    }

    #[test]
    fn test_settings_validate_empty_host_fails() {
        let settings = Settings {
            clickhouse_host: String::new(),
            clickhouse_port: 8123,
            clickhouse_user: "default".into(),
            clickhouse_password: String::new(),
            clickhouse_database: "default".into(),
            http_port: 9600,
            otlp_http_port: 4318,
        };
        assert!(settings.validate().is_err());
    }

    #[test]
    fn test_settings_validate_zero_port_fails() {
        let settings = Settings {
            clickhouse_host: "localhost".into(),
            clickhouse_port: 0,
            clickhouse_user: "default".into(),
            clickhouse_password: String::new(),
            clickhouse_database: "default".into(),
            http_port: 9600,
            otlp_http_port: 4318,
        };
        assert!(settings.validate().is_err());
    }

    #[test]
    fn test_settings_validate_zero_http_port_fails() {
        let settings = Settings {
            clickhouse_host: "localhost".into(),
            clickhouse_port: 8123,
            clickhouse_user: "default".into(),
            clickhouse_password: String::new(),
            clickhouse_database: "default".into(),
            http_port: 0,
            otlp_http_port: 4318,
        };
        assert!(settings.validate().is_err());
    }

    #[test]
    fn test_settings_validate_zero_otlp_http_port_fails() {
        let settings = Settings {
            clickhouse_host: "localhost".into(),
            clickhouse_port: 8123,
            clickhouse_user: "default".into(),
            clickhouse_password: String::new(),
            clickhouse_database: "default".into(),
            http_port: 9600,
            otlp_http_port: 0,
        };
        assert!(settings.validate().is_err());
    }
}
