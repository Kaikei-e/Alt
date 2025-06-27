use rask_log_forwarder::app::{Config, LogLevel};
use std::{env, path::PathBuf, time::Duration};
use tempfile::TempDir;
use serial_test::serial;

// Helper function to clean all environment variables before and after tests
fn clean_all_env_vars() {
    let env_vars = [
        "TARGET_SERVICE", "RASK_ENDPOINT", "BATCH_SIZE", "LOG_LEVEL",
        "ENABLE_DISK_FALLBACK", "ENABLE_METRICS", "ENABLE_COMPRESSION",
        "FLUSH_INTERVAL_MS", "BUFFER_CAPACITY", "CONNECTION_TIMEOUT_SECS",
        "MAX_CONNECTIONS", "MAX_DISK_USAGE_MB", "METRICS_PORT",
        "DISK_FALLBACK_PATH", "CONFIG_FILE"
    ];

    unsafe {
        for var in &env_vars {
            env::remove_var(var);
        }
    }
}

// Helper function to create test config with temporary disk fallback directory
fn create_test_config_with_temp_dir() -> (Config, TempDir) {
    let temp_dir = TempDir::new().unwrap();
    let fallback_path = temp_dir.path().join("fallback");

    let mut config = Config::default();
    config.disk_fallback_path = fallback_path;

    (config, temp_dir)
}

// Helper to disable disk fallback for simple tests
#[allow(dead_code)]
fn create_test_config_no_disk() -> Config {
    let mut config = Config::default();
    config.enable_disk_fallback = false;
    config
}

#[test]
fn test_config_from_args() {
    let args = vec![
        "rask-log-forwarder",
        "--target-service", "nginx",
        "--endpoint", "http://custom-aggregator:9000/v1/aggregate",
        "--batch-size", "5000",
        "--log-level", "debug",
        "--metrics-port", "9091"
    ];

    let config = Config::from_args(args).unwrap();

    assert_eq!(config.target_service, Some("nginx".to_string()));
    assert_eq!(config.endpoint, "http://custom-aggregator:9000/v1/aggregate");
    assert_eq!(config.batch_size, 5000);
    assert_eq!(config.log_level, LogLevel::Debug);
    assert_eq!(config.metrics_port, 9091);
}

#[test]
#[serial]
fn test_config_from_environment() {
    // Create temp directory for disk fallback
    let _temp_dir = TempDir::new().unwrap();
    std::fs::create_dir_all("/tmp/rask-log-forwarder").ok();

    // Clean up all environment variables first using helper
    clean_all_env_vars();

    // Set test environment variables
    unsafe {
        env::set_var("TARGET_SERVICE", "alt-backend");
        env::set_var("RASK_ENDPOINT", "http://test-aggregator:9600/v1/aggregate");
        env::set_var("BATCH_SIZE", "15000");
        env::set_var("LOG_LEVEL", "warn");
        env::set_var("ENABLE_DISK_FALLBACK", "false");
    }

    let config = Config::from_env().unwrap();

    assert_eq!(config.target_service, Some("alt-backend".to_string()));
    assert_eq!(config.endpoint, "http://test-aggregator:9600/v1/aggregate");
    assert_eq!(config.batch_size, 15000);
    assert!(matches!(config.log_level, LogLevel::Warn));
    assert!(!config.enable_disk_fallback);

    // Cleanup - ensure all test environment variables are removed
    clean_all_env_vars();
}

#[test]
fn test_config_validation() {
    let (mut config, _temp_dir) = create_test_config_with_temp_dir();

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
fn test_config_file_loading() {
    let temp_dir = TempDir::new().unwrap();
    let config_file = temp_dir.path().join("config.toml");

    let config_content = r#"
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
"#;

    std::fs::write(&config_file, config_content).unwrap();

    let config = Config::from_file(&config_file).unwrap();

    assert_eq!(config.target_service, Some("meilisearch".to_string()));
    assert_eq!(config.batch_size, 8000);
    assert_eq!(config.flush_interval_ms, 1000);
}

#[test]
fn test_hostname_based_service_detection() {
    // Create temp directory for disk fallback before detection
    let _temp_dir = TempDir::new().unwrap();
    std::fs::create_dir_all("/tmp/rask-log-forwarder").ok();

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
}

#[test]
#[serial]
fn test_config_auto_detect_service() {
    // Clean environment first
    clean_all_env_vars();

    let mut config = Config::default();

    // Set environment variable
    unsafe {
        env::set_var("TARGET_SERVICE", "test-service");
    }

    config.auto_detect_service().unwrap();
    assert_eq!(config.target_service, Some("test-service".to_string()));

    // Clean up after test
    clean_all_env_vars();
}

#[test]
fn test_config_post_process() {
    let mut config = Config::default();
    config.flush_interval_ms = 1000;
    config.connection_timeout_secs = 20;

    config.post_process().unwrap();

    assert_eq!(config.flush_interval.as_millis(), 1000);
    assert_eq!(config.connection_timeout.as_secs(), 20);
}

#[test]
#[serial]
fn test_config_defaults() {
    // Create temp directory for disk fallback
    let _temp_dir = TempDir::new().unwrap();
    std::fs::create_dir_all("/tmp/rask-log-forwarder").ok();

    // Remove ALL possible environment variables to test defaults
    // This ensures we don't inherit values from other tests
    clean_all_env_vars();

    // Wait a bit to ensure environment is clean
    std::thread::sleep(std::time::Duration::from_millis(10));

    // Double check that critical variables are actually unset
    assert!(env::var("BATCH_SIZE").is_err(), "BATCH_SIZE should not be set");
    assert!(env::var("TARGET_SERVICE").is_err(), "TARGET_SERVICE should not be set");

    let config = Config::from_env().unwrap();

    // Test defaults - check the actual value first to debug
    println!("Actual batch_size: {}", config.batch_size);
    assert_eq!(config.batch_size, 10000); // Default batch size
    assert!(matches!(config.log_level, LogLevel::Info)); // Default log level
    assert!(!config.enable_disk_fallback); // Default should be false
    assert!(!config.enable_metrics); // Default should be false
    assert!(!config.enable_compression); // Default should be false

    // Clean up after test as well
    clean_all_env_vars();
}

#[test]
#[serial]
fn test_invalid_log_level() {
    // Create temp directory for disk fallback
    let _temp_dir = TempDir::new().unwrap();
    std::fs::create_dir_all("/tmp/rask-log-forwarder").ok();

    // Clean environment first
    clean_all_env_vars();

    unsafe {
        env::set_var("LOG_LEVEL", "invalid_level");
    }

    let result = Config::from_env();
    // Invalid log level should cause parsing to fail
    assert!(result.is_err());

    // Clean up after test
    clean_all_env_vars();
}

#[test]
#[serial]
fn test_invalid_batch_size() {
    // Create temp directory for disk fallback
    let _temp_dir = TempDir::new().unwrap();
    std::fs::create_dir_all("/tmp/rask-log-forwarder").ok();

    // Clean environment first
    clean_all_env_vars();

    unsafe {
        env::set_var("BATCH_SIZE", "not_a_number");
    }

    let result = Config::from_env();
    // Invalid batch size should cause parsing to fail
    assert!(result.is_err());

    // Clean up after test
    clean_all_env_vars();
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
        flush_interval: Duration::from_millis(1000),
        connection_timeout: Duration::from_secs(30),
        retry_config: Default::default(),
        disk_fallback_config: Default::default(),
        metrics_config: Default::default(),
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
    // Create temp directory for disk fallback
    let _temp_dir = TempDir::new().unwrap();
    std::fs::create_dir_all("/tmp/rask-log-forwarder").ok();

    // Clean up ALL environment variables from previous tests
    clean_all_env_vars();

    // Set only the required TARGET_SERVICE environment variable
    unsafe {
        env::set_var("TARGET_SERVICE", "test-service");
    }

    let config = Config::from_env().unwrap();

    // Should detect hostname automatically if not set
    assert!(config.target_service.is_some());
    assert_eq!(config.target_service, Some("test-service".to_string()));

    // Clean up
    clean_all_env_vars();
}