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
    env::var(env_name).map_err(|_| {
        AggregatorError::Config(format!("Missing required environment variable: {env_name}"))
    })
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
    let clickhouse_port = require_env("APP_CLICKHOUSE_PORT")?
        .parse::<u16>()
        .map_err(|e| {
            AggregatorError::Config(format!(
                "Invalid APP_CLICKHOUSE_PORT (must be a valid port): {e}"
            ))
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
    use std::sync::Mutex;

    /// `require_env`/`env_port_or`/`get_env_or_file` all read `std::env`,
    /// which is process-global and not safe to mutate/read concurrently
    /// (`env::set_var`/`remove_var` are `unsafe` as of the 2024-edition std
    /// library precisely because of this). Cargo runs tests in the same
    /// process on multiple threads, so every test below acquires this lock
    /// for its whole body — not just the mutation — to fully serialize
    /// access instead of merely picking unique variable names, which would
    /// avoid *assertion* collisions but not the underlying data race.
    static ENV_LOCK: Mutex<()> = Mutex::new(());

    /// RAII helper: sets an env var for the duration of the guard, restores
    /// the previous state (unset, in every case these tests need) on drop —
    /// including on panic — so a failing assertion can't leak state into
    /// whichever test runs next.
    struct EnvVarGuard {
        name: &'static str,
    }

    impl EnvVarGuard {
        fn set(name: &'static str, value: &str) -> Self {
            // SAFETY: caller holds `ENV_LOCK` for the guard's entire
            // lifetime, so no other thread in this test binary can be
            // reading or writing the environment concurrently.
            unsafe { env::set_var(name, value) };
            Self { name }
        }
    }

    impl Drop for EnvVarGuard {
        fn drop(&mut self) {
            // SAFETY: see `EnvVarGuard::set`.
            unsafe { env::remove_var(self.name) };
        }
    }

    fn locked() -> std::sync::MutexGuard<'static, ()> {
        ENV_LOCK
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
    }

    #[test]
    fn require_env_returns_value_when_set() {
        let _lock = locked();
        let _guard = EnvVarGuard::set("RASK_TEST_REQUIRE_ENV_PRESENT", "hello");

        let result = require_env("RASK_TEST_REQUIRE_ENV_PRESENT");

        assert_eq!(result.unwrap(), "hello");
    }

    #[test]
    fn require_env_errors_and_names_the_variable_when_missing() {
        let _lock = locked();
        // Not set by anything in this process; no guard needed.
        let result = require_env("RASK_TEST_REQUIRE_ENV_DEFINITELY_UNSET");

        let err = result.unwrap_err();
        assert!(
            err.to_string()
                .contains("RASK_TEST_REQUIRE_ENV_DEFINITELY_UNSET"),
            "fail-fast config error must name the missing variable, got: {err}"
        );
    }

    #[test]
    fn env_port_or_returns_default_when_unset() {
        let _lock = locked();
        let result = env_port_or("RASK_TEST_PORT_UNSET", 9600);

        assert_eq!(result.unwrap(), 9600);
    }

    #[test]
    fn env_port_or_returns_configured_value_when_set() {
        let _lock = locked();
        let _guard = EnvVarGuard::set("RASK_TEST_PORT_SET", "1234");

        let result = env_port_or("RASK_TEST_PORT_SET", 9600);

        assert_eq!(result.unwrap(), 1234);
    }

    #[test]
    fn env_port_or_errors_on_non_numeric_value() {
        let _lock = locked();
        let _guard = EnvVarGuard::set("RASK_TEST_PORT_INVALID", "not-a-port");

        let result = env_port_or("RASK_TEST_PORT_INVALID", 9600);

        let err = result.unwrap_err();
        assert!(
            err.to_string().contains("RASK_TEST_PORT_INVALID"),
            "port parse error must name the variable, got: {err}"
        );
    }

    #[test]
    fn env_port_or_errors_on_out_of_range_value() {
        let _lock = locked();
        // u16::MAX + 1 : valid integer, invalid port.
        let _guard = EnvVarGuard::set("RASK_TEST_PORT_OUT_OF_RANGE", "65536");

        let result = env_port_or("RASK_TEST_PORT_OUT_OF_RANGE", 9600);

        assert!(result.is_err());
    }

    #[test]
    fn get_env_or_file_reads_direct_variable() {
        let _lock = locked();
        let _guard = EnvVarGuard::set("RASK_TEST_SECRET_DIRECT", "direct-value");

        let result = get_env_or_file("RASK_TEST_SECRET_DIRECT");

        assert_eq!(result.unwrap(), "direct-value");
    }

    #[test]
    fn get_env_or_file_errors_when_neither_variable_nor_file_is_set() {
        let _lock = locked();
        let result = get_env_or_file("RASK_TEST_SECRET_NEITHER_SET");

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
        let _lock = locked();
        let temp_dir = tempfile::TempDir::new().unwrap();
        let secret_path = temp_dir.path().join("clickhouse_password");
        // Trailing newline mimics how Docker/Kubernetes secret files are
        // typically written; the value must be trimmed, not compared raw.
        std::fs::write(&secret_path, "s3cret\n").unwrap();
        let _guard = EnvVarGuard::set("RASK_TEST_SECRET_FILE_FILE", secret_path.to_str().unwrap());

        let result = get_env_or_file("RASK_TEST_SECRET_FILE");

        assert_eq!(result.unwrap(), "s3cret");
    }

    #[test]
    fn get_env_or_file_errors_when_file_path_does_not_exist() {
        let _lock = locked();
        let _guard = EnvVarGuard::set(
            "RASK_TEST_SECRET_MISSING_FILE_FILE",
            "/nonexistent/path/for/rask/test",
        );

        let result = get_env_or_file("RASK_TEST_SECRET_MISSING_FILE");

        let err = result.unwrap_err();
        assert!(
            err.to_string()
                .contains("RASK_TEST_SECRET_MISSING_FILE_FILE"),
            "must name the _FILE variable when the referenced file can't be read, got: {err}"
        );
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
