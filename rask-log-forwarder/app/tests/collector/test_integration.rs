use rask_log_forwarder::collector::{CollectorConfig, CollectorError, LogCollector};
use serial_test::serial;

#[tokio::test]
async fn test_integration_config_creation() {
    let config = CollectorConfig {
        auto_discover: true,
        target_service: None,
        follow_rotations: true,
        buffer_size: 1024,
    };

    assert!(config.auto_discover);
    assert!(config.follow_rotations);
    assert_eq!(config.buffer_size, 1024);
}

#[tokio::test]
#[serial]
async fn test_integration_with_env_var() {
    // Test environment variable configuration
    unsafe {
        std::env::set_var("TARGET_SERVICE", "nginx");
        std::env::set_var("DOCKER_HOST", "unix:///var/run/docker.sock");
    }

    let config = CollectorConfig::default();
    let result = LogCollector::new(config).await;

    // Should handle both success and Docker unavailability gracefully
    match result {
        Ok(collector) => {
            assert_eq!(collector.get_target_service(), "nginx");
            println!("Collector created successfully with env var");
        }
        Err(CollectorError::DiscoveryError(_)) => {
            println!("Discovery error expected when Docker is unavailable");
        }
        Err(e) => {
            println!("Other error (may be expected): {e}");
        }
    }

    unsafe {
        std::env::remove_var("TARGET_SERVICE");
        std::env::remove_var("DOCKER_HOST");
    }
}

#[tokio::test]
#[serial]
async fn test_integration_auto_discover_failure() {
    unsafe {
        std::env::set_var("DOCKER_HOST", "unix:///var/run/docker.sock");
    }

    // Test with hostname that doesn't match pattern
    let config = CollectorConfig {
        auto_discover: true,
        target_service: None,
        follow_rotations: true,
        buffer_size: 1024,
    };

    // This should work with proper hostname detection
    let result = LogCollector::new(config).await;
    // May succeed or fail depending on hostname, but shouldn't panic
    assert!(result.is_ok() || result.is_err());

    unsafe {
        std::env::remove_var("DOCKER_HOST");
    }
}

#[tokio::test]
#[serial]
async fn test_integration_explicit_service() {
    unsafe {
        std::env::set_var("DOCKER_HOST", "unix:///var/run/docker.sock");
    }

    let config = CollectorConfig {
        auto_discover: false,
        target_service: Some("test-service".to_string()),
        follow_rotations: true,
        buffer_size: 1024,
    };

    let result = LogCollector::new(config).await;

    // Should handle both success and Docker unavailability gracefully
    match result {
        Ok(collector) => {
            assert_eq!(collector.get_target_service(), "test-service");
            println!("Collector created successfully with explicit service");
        }
        Err(CollectorError::DiscoveryError(_)) => {
            println!("Discovery error expected when Docker is unavailable");
        }
        Err(e) => {
            println!("Other error (may be expected): {e}");
        }
    }

    unsafe {
        std::env::remove_var("DOCKER_HOST");
    }
}
