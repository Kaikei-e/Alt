use std::env;
use std::fs;

use crate::error::AggregatorError;

#[derive(Clone)]
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
    /// Ingest bearer token for authenticating forwarders/OTLP clients (D-02).
    pub ingest_token: String,
}

impl std::fmt::Debug for Settings {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("Settings")
            .field("clickhouse_host", &self.clickhouse_host)
            .field("clickhouse_port", &self.clickhouse_port)
            .field("clickhouse_user", &self.clickhouse_user)
            .field("clickhouse_password", &"[REDACTED]")
            .field("clickhouse_database", &self.clickhouse_database)
            .field("http_port", &self.http_port)
            .field("otlp_http_port", &self.otlp_http_port)
            .field("ingest_token", &"[REDACTED]")
            .finish()
    }
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

/// Read a required environment variable via `lookup`, naming it in the error
/// so a missing-config failure is diagnosable instead of a bare "not found".
fn require_env_with<F>(env_name: &str, lookup: &F) -> Result<String, AggregatorError>
where
    F: Fn(&str) -> Result<String, env::VarError>,
{
    lookup(env_name).map_err(|_| {
        AggregatorError::Config(format!("Missing required environment variable: {env_name}"))
    })
}

/// Read an optional `u16` environment variable via `lookup`, with a default fallback.
fn env_port_or_with<F>(env_name: &str, default: u16, lookup: &F) -> Result<u16, AggregatorError>
where
    F: Fn(&str) -> Result<String, env::VarError>,
{
    match lookup(env_name) {
        Ok(value) => value.parse::<u16>().map_err(|e| {
            AggregatorError::Config(format!("Invalid {env_name} (must be a valid port): {e}"))
        }),
        Err(_) => Ok(default),
    }
}

/// Read a value from environment variable via `lookup`, with support for `_FILE` suffix (Docker Secrets).
fn get_env_or_file_with<F>(env_name: &str, lookup: &F) -> Result<String, AggregatorError>
where
    F: Fn(&str) -> Result<String, env::VarError>,
{
    // First check for _FILE suffix (Docker Secrets support)
    let file_env = format!("{env_name}_FILE");
    if let Ok(file_path) = lookup(&file_env) {
        return fs::read_to_string(&file_path)
            .map(|content| content.trim().to_string())
            .map_err(|e| AggregatorError::Config(format!("Failed to read {file_env}: {e}")));
    }

    // Fallback to standard environment variable
    require_env_with(env_name, lookup).map_err(|_| {
        AggregatorError::Config(format!(
            "Missing required environment variable: {env_name} or {file_env}"
        ))
    })
}

/// Load configuration using the provided environment variable lookup function.
pub fn get_configuration_from_lookup<F>(lookup: F) -> Result<Settings, AggregatorError>
where
    F: Fn(&str) -> Result<String, env::VarError>,
{
    let clickhouse_host = require_env_with("APP_CLICKHOUSE_HOST", &lookup)?;
    let clickhouse_port = require_env_with("APP_CLICKHOUSE_PORT", &lookup)?
        .parse::<u16>()
        .map_err(|e| {
            AggregatorError::Config(format!(
                "Invalid APP_CLICKHOUSE_PORT (must be a valid port): {e}"
            ))
        })?;
    let clickhouse_user = require_env_with("APP_CLICKHOUSE_USER", &lookup)?;
    let clickhouse_password = get_env_or_file_with("APP_CLICKHOUSE_PASSWORD", &lookup)?;
    let clickhouse_database = require_env_with("APP_CLICKHOUSE_DATABASE", &lookup)?;

    // Server ports with defaults
    let http_port = env_port_or_with("HTTP_PORT", 9600, &lookup)?;
    let otlp_http_port = env_port_or_with("OTLP_HTTP_PORT", 4318, &lookup)?;

    // D-02: Load ingest bearer token from file. Fail-fast at startup.
    let ingest_token_file = lookup("RASK_INGEST_TOKEN_FILE")
        .unwrap_or_else(|_| "/run/secrets/rask_ingest_token".to_string());
    let ingest_token = crate::auth::load_ingest_token(&ingest_token_file)?;

    let settings = Settings {
        clickhouse_host,
        clickhouse_port,
        clickhouse_user,
        clickhouse_password,
        clickhouse_database,
        http_port,
        otlp_http_port,
        ingest_token,
    };

    // Validate settings before returning
    settings.validate()?;

    Ok(settings)
}

pub fn get_configuration() -> Result<Settings, AggregatorError> {
    get_configuration_from_lookup(|name| env::var(name))
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

    fn map_lookup<'a>(
        vars: HashMap<&'a str, &'a str>,
    ) -> impl Fn(&str) -> Result<String, env::VarError> + 'a {
        move |name| {
            vars.get(name)
                .map(|v| (*v).to_string())
                .ok_or(env::VarError::NotPresent)
        }
    }

    #[test]
    fn require_env_returns_value_when_set() {
        let lookup = map_lookup(HashMap::from([("RASK_TEST_REQUIRE_ENV_PRESENT", "hello")]));

        let result = require_env_with("RASK_TEST_REQUIRE_ENV_PRESENT", &lookup);

        assert_eq!(result.unwrap(), "hello");
    }

    #[test]
    fn require_env_errors_and_names_the_variable_when_missing() {
        let lookup = map_lookup(HashMap::new());
        let result = require_env_with("RASK_TEST_REQUIRE_ENV_DEFINITELY_UNSET", &lookup);

        let err = result.unwrap_err();
        assert!(
            err.to_string()
                .contains("RASK_TEST_REQUIRE_ENV_DEFINITELY_UNSET"),
            "fail-fast config error must name the missing variable, got: {err}"
        );
    }

    #[test]
    fn env_port_or_returns_default_when_unset() {
        let lookup = map_lookup(HashMap::new());
        let result = env_port_or_with("RASK_TEST_PORT_UNSET", 9600, &lookup);

        assert_eq!(result.unwrap(), 9600);
    }

    #[test]
    fn env_port_or_returns_configured_value_when_set() {
        let lookup = map_lookup(HashMap::from([("RASK_TEST_PORT_SET", "1234")]));

        let result = env_port_or_with("RASK_TEST_PORT_SET", 9600, &lookup);

        assert_eq!(result.unwrap(), 1234);
    }

    #[test]
    fn env_port_or_errors_on_non_numeric_value() {
        let lookup = map_lookup(HashMap::from([("RASK_TEST_PORT_INVALID", "not-a-port")]));

        let result = env_port_or_with("RASK_TEST_PORT_INVALID", 9600, &lookup);

        let err = result.unwrap_err();
        assert!(
            err.to_string().contains("RASK_TEST_PORT_INVALID"),
            "port parse error must name the variable, got: {err}"
        );
    }

    #[test]
    fn env_port_or_errors_on_out_of_range_value() {
        // u16::MAX + 1 : valid integer, invalid port.
        let lookup = map_lookup(HashMap::from([("RASK_TEST_PORT_OUT_OF_RANGE", "65536")]));

        let result = env_port_or_with("RASK_TEST_PORT_OUT_OF_RANGE", 9600, &lookup);

        assert!(result.is_err());
    }

    #[test]
    fn get_env_or_file_reads_direct_variable() {
        let lookup = map_lookup(HashMap::from([("RASK_TEST_SECRET_DIRECT", "direct-value")]));

        let result = get_env_or_file_with("RASK_TEST_SECRET_DIRECT", &lookup);

        assert_eq!(result.unwrap(), "direct-value");
    }

    #[test]
    fn get_env_or_file_errors_when_neither_variable_nor_file_is_set() {
        let lookup = map_lookup(HashMap::new());
        let result = get_env_or_file_with("RASK_TEST_SECRET_NEITHER_SET", &lookup);

        let err = result.unwrap_err();
        let msg = err.to_string();
        assert!(
            msg.contains("RASK_TEST_SECRET_NEITHER_SET")
                && msg.contains("RASK_TEST_SECRET_NEITHER_SET_FILE"),
            "error must name both the direct variable and its _FILE fallback, got: {msg}"
        );
    }

    #[test]
    fn get_env_or_file_reads_from_file_for_docker_secrets_support() {
        let temp_dir = tempfile::TempDir::new().unwrap();
        let secret_path = temp_dir.path().join("clickhouse_password");
        // Trailing newline mimics how Docker/Kubernetes secret files are
        // typically written; the value must be trimmed, not compared raw.
        std::fs::write(&secret_path, "s3cret\n").unwrap();
        let secret_path_str = secret_path.to_str().unwrap();
        let lookup = map_lookup(HashMap::from([(
            "RASK_TEST_SECRET_FILE_FILE",
            secret_path_str,
        )]));

        let result = get_env_or_file_with("RASK_TEST_SECRET_FILE", &lookup);

        assert_eq!(result.unwrap(), "s3cret");
    }

    #[test]
    fn get_env_or_file_errors_when_file_path_does_not_exist() {
        let lookup = map_lookup(HashMap::from([(
            "RASK_TEST_SECRET_MISSING_FILE_FILE",
            "/nonexistent/path/for/rask/test",
        )]));

        let result = get_env_or_file_with("RASK_TEST_SECRET_MISSING_FILE", &lookup);

        let err = result.unwrap_err();
        assert!(
            err.to_string()
                .contains("RASK_TEST_SECRET_MISSING_FILE_FILE"),
            "must name the _FILE variable when the referenced file can't be read, got: {err}"
        );
    }

    #[test]
    fn get_configuration_from_lookup_reads_all_variables_and_defaults() {
        let temp_dir = tempfile::TempDir::new().unwrap();
        let token_path = temp_dir.path().join("ingest_token");
        std::fs::write(&token_path, "valid-test-token_123\n").unwrap();
        let token_path_str = token_path.to_str().unwrap();

        let lookup = map_lookup(HashMap::from([
            ("APP_CLICKHOUSE_HOST", "clickhouse.local"),
            ("APP_CLICKHOUSE_PORT", "8123"),
            ("APP_CLICKHOUSE_USER", "rask"),
            ("APP_CLICKHOUSE_PASSWORD", "pw"),
            ("APP_CLICKHOUSE_DATABASE", "logs"),
            ("RASK_INGEST_TOKEN_FILE", token_path_str),
        ]));

        let settings = get_configuration_from_lookup(lookup).unwrap();
        assert_eq!(settings.clickhouse_host, "clickhouse.local");
        assert_eq!(settings.clickhouse_port, 8123);
        assert_eq!(settings.clickhouse_user, "rask");
        assert_eq!(settings.clickhouse_password, "pw");
        assert_eq!(settings.clickhouse_database, "logs");
        assert_eq!(settings.http_port, 9600);
        assert_eq!(settings.otlp_http_port, 4318);
        assert_eq!(settings.ingest_token, "valid-test-token_123");
    }

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
            ingest_token: String::new(),
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
            ingest_token: String::new(),
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
            ingest_token: String::new(),
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
            ingest_token: String::new(),
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
            ingest_token: String::new(),
        };
        assert!(settings.validate().is_err());
    }

    #[test]
    fn test_settings_debug_redacts_ingest_token_and_password() {
        let unique_token = "unique-secret-token-xyz-12345";
        let unique_pw = "unique-ch-password-98765";
        let settings = Settings {
            clickhouse_host: "localhost".into(),
            clickhouse_port: 8123,
            clickhouse_user: "default".into(),
            clickhouse_password: unique_pw.into(),
            clickhouse_database: "default".into(),
            http_port: 9600,
            otlp_http_port: 4318,
            ingest_token: unique_token.into(),
        };
        let formatted = format!("{settings:?}");
        assert!(
            !formatted.contains(unique_token),
            "ingest_token must not appear in Debug format"
        );
        assert!(
            !formatted.contains(unique_pw),
            "clickhouse_password must not appear in Debug format"
        );
        assert!(formatted.contains("[REDACTED]"));
    }
}
