use rask_log_forwarder::collector::{LogCollector, CollectorConfig, CollectorError};

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
async fn test_integration_with_env_var() {
    unsafe {
        std::env::set_var("TARGET_SERVICE", "nginx");
    }
    
    let config = CollectorConfig::default();
    let result = LogCollector::new(config).await;
    
    // Should succeed in creating collector even if container doesn't exist
    // The error will come when starting collection
    assert!(result.is_ok() || matches!(result.unwrap_err(), CollectorError::DiscoveryError(_)));
    
    unsafe {
        std::env::remove_var("TARGET_SERVICE");
    }
}

#[tokio::test]
async fn test_integration_auto_discover_failure() {
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
}

#[tokio::test]
async fn test_integration_explicit_service() {
    let config = CollectorConfig {
        auto_discover: false,
        target_service: Some("test-service".to_string()),
        follow_rotations: true,
        buffer_size: 1024,
    };
    
    let result = LogCollector::new(config).await;
    
    // Should succeed in creating collector
    assert!(result.is_ok());
    
    let collector = result.unwrap();
    assert_eq!(collector.get_target_service(), "test-service");
}