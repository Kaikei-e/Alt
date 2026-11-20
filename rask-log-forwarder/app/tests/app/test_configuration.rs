use super::fixture::{create_token_in_temp_dir, with_isolated_env};
use rask_log_forwarder::app::config::{DiskFallbackConfig, MetricsConfig, Protocol, RetryConfig};
use rask_log_forwarder::app::{Config, LogLevel};
use serial_test::serial;
use std::{env, path::PathBuf, time::Duration};
use tempfile::TempDir;

// Helper function to create test config with temporary disk fallback directory.
// The token file is written inside temp_dir so it persists for the lifetime of
// the returned TempDir (callers must keep the TempDir alive).
fn create_test_config_with_temp_dir() -> Result<(Config, TempDir), std::io::Error> {
    let temp_dir = TempDir::new()?;
    let token_path = temp_dir.path().join("ingest_token");
    std::fs::write(&token_path, "test-token")?;
    let config = Config {
        ingest_token_file: Some(token_path),
        ingest_token: "test-token".to_string(),
        ..Default::default()
    };

    Ok((config, temp_dir))
}

#[test]
#[serial]
fn test_config_from_args() {
    let (token_path, _token_dir) = create_token_in_temp_dir("test-token").unwrap();

    let args = vec![
        "rask-log-forwarder",
        "--target-service",
        "nginx",
        "--endpoint",
        "http://custom-aggregator:9000/v1/aggregate",
        "--batch-size",
        "5000",
        "--log-level",
        "debug",
        "--metrics-port",
        "9091",
        "--ingest-token-file",
        token_path.to_str().unwrap(),
    ];

    let config = with_isolated_env(&[], || Config::from_args_and_env(args).unwrap());

    assert_eq!(config.target_service, Some("nginx".to_string()));
    assert_eq!(
        config.endpoint,
        "http://custom-aggregator:9000/v1/aggregate"
    );
    assert_eq!(config.batch_size, 5000);
    assert_eq!(config.log_level, LogLevel::Debug);
    assert_eq!(config.metrics_port, 9091);
}

#[test]
#[serial]
fn test_config_from_environment() {
    let (token_path, _token_dir) = create_token_in_temp_dir("valid-token-for-tests").unwrap();
    let token_path_str = token_path.to_string_lossy();

    let config = with_isolated_env(
        &[
            ("RASK_INGEST_TOKEN_FILE", Some(&token_path_str)),
            ("TARGET_SERVICE", Some("alt-backend")),
            (
                "RASK_ENDPOINT",
                Some("http://test-aggregator:9600/v1/aggregate"),
            ),
            ("BATCH_SIZE", Some("15000")),
            ("LOG_LEVEL", Some("warn")),
            ("ENABLE_DISK_FALLBACK", Some("false")),
        ],
        || Config::from_env().unwrap(),
    );

    assert_eq!(config.target_service, Some("alt-backend".to_string()));
    assert_eq!(config.endpoint, "http://test-aggregator:9600/v1/aggregate");
    assert_eq!(config.batch_size, 15000);
    assert!(matches!(config.log_level, LogLevel::Warn));
    assert!(!config.enable_disk_fallback);
}

#[test]
fn test_config_validation() {
    let (mut config, _temp_dir) = create_test_config_with_temp_dir().unwrap();

    // Valid config should pass
    config.validate().unwrap();

    // Invalid endpoint should fail
    config.endpoint = "invalid-url".to_string();
    assert!(config.validate().is_err());

    // Invalid batch size should fail
    config.endpoint = "http://valid:9600/v1/aggregate".to_string();
    config.batch_size = 0;
    assert!(config.validate().is_err());

    // Invalid buffer capacity should fail
    config.batch_size = 1000;
    config.buffer_capacity = 100; // Smaller than batch size
    assert!(config.validate().is_err());
}

#[test]
#[serial]
fn test_config_file_loading() {
    let (token_path, _token_dir) = create_token_in_temp_dir("test-token").unwrap();
    let token_path_str = token_path.to_str().unwrap().replace('\\', "/");

    let temp_dir = TempDir::new().unwrap();
    let config_file = temp_dir.path().join("config.toml");

    let config_content = format!(
        r#"
target_service = "meilisearch"
endpoint = "http://aggregator:9600/v1/aggregate"
batch_size = 8000
flush_interval_ms = 1000
buffer_capacity = 50000
log_level = "info"
enable_metrics = true
metrics_port = 9090
enable_disk_fallback = false
disk_fallback_path = "/tmp/test-fallback"
max_disk_usage_mb = 500
connection_timeout_secs = 20
max_connections = 5
enable_compression = true
protocol = "ndjson"
otlp_endpoint = "http://rask-log-aggregator:4318/v1/logs"
ingest_token_file = "{token_path_str}"
"#
    );

    std::fs::write(&config_file, config_content).unwrap();

    let config = with_isolated_env(&[], || Config::from_file(&config_file).unwrap());

    assert_eq!(config.target_service, Some("meilisearch".to_string()));
    assert_eq!(config.batch_size, 8000);
    assert_eq!(config.flush_interval_ms, 1000);
}

#[test]
#[serial]
fn test_hostname_based_service_detection() {
    let (token_path, _token_dir) = create_token_in_temp_dir("valid-token-for-tests").unwrap();
    let token_path_str = token_path.to_string_lossy();

    with_isolated_env(&[("RASK_INGEST_TOKEN_FILE", Some(&token_path_str))], || {
        // Test hostname pattern
        let config = Config::detect_service_from_hostname("nginx-logs").unwrap();
        assert_eq!(config.target_service, Some("nginx".to_string()));

        let config = Config::detect_service_from_hostname("alt-backend-logs").unwrap();
        assert_eq!(config.target_service, Some("alt-backend".to_string()));

        let config = Config::detect_service_from_hostname("db-logs").unwrap();
        assert_eq!(config.target_service, Some("db".to_string()));

        // Invalid hostname should fail
        let result = Config::detect_service_from_hostname("invalid-hostname");
        assert!(result.is_err());
    });
}

#[test]
#[serial]
fn test_config_auto_detect_service() {
    let (token_path, _token_dir) = create_token_in_temp_dir("valid-token-for-tests").unwrap();

    with_isolated_env(&[], || {
        // Verify environment is clean
        assert!(
            env::var("TARGET_SERVICE").is_err(),
            "TARGET_SERVICE should not be set before test"
        );

        let mut config = Config {
            ingest_token_file: Some(token_path),
            ingest_token: "test-token".to_string(),
            ..Default::default()
        };

        // Set environment variable and test auto-detection
        temp_env::with_var("TARGET_SERVICE", Some("test-service"), || {
            assert_eq!(
                env::var("TARGET_SERVICE").as_deref(),
                Ok("test-service"),
                "TARGET_SERVICE should be set"
            );

            config.auto_detect_service().unwrap();
            assert_eq!(config.target_service, Some("test-service".to_string()));
        });

        // Verify cleanup
        assert!(
            env::var("TARGET_SERVICE").is_err(),
            "TARGET_SERVICE should be cleaned up after test"
        );

        // Test auto-detection without environment variable (should fail or use hostname)
        let mut config2 = Config::default();
        let result = config2.auto_detect_service();

        // In CI environment, this might fail if hostname doesn't match pattern
        // We'll accept either success (if hostname matches) or failure (if it doesn't)
        match result {
            Ok(()) => {
                // Auto-detection succeeded (probably from hostname)
                assert!(
                    config2.target_service.is_some(),
                    "Target service should be set after successful auto-detection"
                );
            }
            Err(e) => {
                // Auto-detection failed (expected in some environments)
                assert!(
                    e.to_string()
                        .contains("Could not auto-detect target service"),
                    "Error should be about auto-detection failure: {e}"
                );
            }
        }
    });
}

#[test]
fn test_config_post_process() {
    let (token_path, _token_dir) = create_token_in_temp_dir("valid-token-for-tests").unwrap();
    let mut config = Config {
        ingest_token_file: Some(token_path),
        ingest_token: "test-token".to_string(),
        connection_timeout_secs: 20,
        ..Default::default()
    };

    config.post_process().unwrap();

    assert_eq!(config.flush_interval.as_millis(), 500);
    assert_eq!(config.connection_timeout.as_secs(), 20);
}

#[test]
#[serial]
fn test_config_defaults() {
    let (token_path, _token_dir) = create_token_in_temp_dir("valid-token-for-tests").unwrap();
    let token_path_str = token_path.to_string_lossy();

    let config = with_isolated_env(&[("RASK_INGEST_TOKEN_FILE", Some(&token_path_str))], || {
        // Double check that critical variables are actually unset
        assert!(
            env::var("BATCH_SIZE").is_err(),
            "BATCH_SIZE should not be set"
        );
        assert!(
            env::var("TARGET_SERVICE").is_err(),
            "TARGET_SERVICE should not be set"
        );

        Config::from_env().unwrap()
    });

    // Test defaults - check the actual value first to debug
    println!("Actual batch_size: {}", config.batch_size);
    assert_eq!(config.batch_size, 10000); // Default batch size
    assert!(matches!(config.log_level, LogLevel::Info)); // Default log level
    assert!(!config.enable_disk_fallback); // Default should be false
    assert!(!config.enable_metrics); // Default should be false
    assert!(!config.enable_compression); // Default should be false
}

#[test]
#[serial]
fn test_invalid_log_level() {
    let (token_path, _token_dir) = create_token_in_temp_dir("valid-token-for-tests").unwrap();
    let token_path_str = token_path.to_string_lossy();

    let result = with_isolated_env(
        &[
            ("RASK_INGEST_TOKEN_FILE", Some(&token_path_str)),
            ("LOG_LEVEL", Some("invalid_level")),
        ],
        Config::from_env,
    );
    // Invalid log level should cause parsing to fail
    assert!(result.is_err());
}

#[test]
#[serial]
fn test_invalid_batch_size() {
    let (token_path, _token_dir) = create_token_in_temp_dir("valid-token-for-tests").unwrap();
    let token_path_str = token_path.to_string_lossy();

    let result = with_isolated_env(&[("RASK_INGEST_TOKEN_FILE", Some(&token_path_str))], || {
        // Double check that BATCH_SIZE is not set
        assert!(
            env::var("BATCH_SIZE").is_err(),
            "BATCH_SIZE should not be set before test"
        );

        temp_env::with_var("BATCH_SIZE", Some("not_a_number"), || {
            // Verify the environment variable is set
            assert_eq!(env::var("BATCH_SIZE").unwrap(), "not_a_number");

            Config::from_env()
        })
    });
    // Invalid batch size should cause parsing to fail
    assert!(result.is_err());
}

#[test]
fn test_config_serialization() {
    let config = Config {
        target_service: Some("test-service".to_string()),
        endpoint: "http://localhost:9600/v1/aggregate".to_string(),
        batch_size: 5000,
        log_level: LogLevel::Debug,
        enable_disk_fallback: true,
        flush_interval_ms: 1000,
        buffer_capacity: 10000,
        enable_metrics: true,
        metrics_port: 9090,
        disk_fallback_path: PathBuf::from("/tmp/test"),
        max_disk_usage_mb: 1000,
        connection_timeout_secs: 30,
        max_connections: 10,
        config_file: None,
        enable_compression: false,
        protocol: rask_log_forwarder::app::config::Protocol::Ndjson,
        otlp_endpoint: "http://rask-log-aggregator:4318/v1/logs".to_string(),
        flush_interval: Duration::from_millis(1000),
        connection_timeout: Duration::from_secs(30),
        retry_config: RetryConfig::default(),
        disk_fallback_config: DiskFallbackConfig::default(),
        metrics_config: MetricsConfig::default(),
        ingest_token_file: None,
        ingest_token: String::new(),
    };

    // Test that config can be serialized/deserialized
    let json = serde_json::to_string(&config).unwrap();
    assert!(json.contains("test-service"));

    let deserialized: Config = serde_json::from_str(&json).unwrap();
    assert_eq!(deserialized.target_service, config.target_service);
    assert_eq!(deserialized.endpoint, config.endpoint);
}

#[test]
#[serial]
fn test_hostname_detection() {
    let (token_path, _token_dir) = create_token_in_temp_dir("valid-token-for-tests").unwrap();
    let token_path_str = token_path.to_string_lossy();

    let config = with_isolated_env(
        &[
            ("RASK_INGEST_TOKEN_FILE", Some(&token_path_str)),
            ("TARGET_SERVICE", Some("test-service")),
        ],
        || Config::from_env().unwrap(),
    );

    // Should detect hostname automatically if not set
    assert!(config.target_service.is_some());
    assert_eq!(config.target_service, Some("test-service".to_string()));
}

#[test]
#[serial]
fn test_protocol_configuration() {
    let (token_path, _token_dir) = create_token_in_temp_dir("valid-token-for-tests").unwrap();
    let token_path_str = token_path.to_string_lossy();

    with_isolated_env(&[("RASK_INGEST_TOKEN_FILE", Some(&token_path_str))], || {
        // Test default protocol (NDJSON)
        let config = Config::from_env().unwrap();
        assert!(
            matches!(
                config.protocol,
                rask_log_forwarder::app::config::Protocol::Ndjson
            ),
            "Default protocol should be NDJSON"
        );

        // Test OTLP protocol via environment
        temp_env::with_var("PROTOCOL", Some("otlp"), || {
            let config = Config::from_env().unwrap();
            assert!(
                matches!(
                    config.protocol,
                    rask_log_forwarder::app::config::Protocol::Otlp
                ),
                "Protocol should be OTLP"
            );
        });

        // Test invalid protocol
        temp_env::with_var("PROTOCOL", Some("invalid_protocol"), || {
            let result = Config::from_env();
            assert!(result.is_err(), "Invalid protocol should cause error");
        });
    });
}

#[test]
fn test_otlp_endpoint_validation() {
    let (token_path, _token_dir) = create_token_in_temp_dir("valid-token-for-tests").unwrap();

    let mut config = Config {
        protocol: Protocol::Otlp,
        otlp_endpoint: "http://valid-endpoint:4318/v1/logs".to_string(),
        ingest_token_file: Some(token_path),
        ingest_token: "test-token".to_string(),
        ..Default::default()
    };

    // Valid OTLP endpoint should pass
    assert!(
        config.validate().is_ok(),
        "Valid OTLP endpoint should pass validation"
    );

    // Invalid OTLP endpoint should fail when protocol is OTLP
    config.otlp_endpoint = "not-a-valid-url".to_string();
    assert!(
        config.validate().is_err(),
        "Invalid OTLP endpoint should fail validation"
    );

    // Invalid OTLP endpoint should NOT fail when protocol is NDJSON
    config.protocol = Protocol::Ndjson;
    config.otlp_endpoint = "not-a-valid-url".to_string();
    assert!(
        config.validate().is_ok(),
        "OTLP endpoint validation should be skipped for NDJSON protocol"
    );
}
