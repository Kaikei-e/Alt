use rask_log_forwarder::app::{Config, docker::{DockerEnvironment, validate_docker_requirements}};
use std::collections::HashMap;
use std::env;

#[test]
fn test_docker_environment_detection() {
    // Simulate Docker environment variables
    unsafe {
        env::set_var("HOSTNAME", "nginx-logs");
        env::set_var("TARGET_SERVICE", "nginx");
        env::set_var("RASK_ENDPOINT", "http://rask-aggregator:9600/v1/aggregate");
    }

    let docker_env = DockerEnvironment::detect().unwrap();

    assert_eq!(docker_env.hostname, "nginx-logs");
    assert_eq!(docker_env.target_service, Some("nginx".to_string()));
    assert_eq!(docker_env.rask_endpoint, "http://rask-aggregator:9600/v1/aggregate");

    // Cleanup
    unsafe {
        env::remove_var("HOSTNAME");
        env::remove_var("TARGET_SERVICE");
        env::remove_var("RASK_ENDPOINT");
    }
}

#[test]
fn test_sidecar_configuration() {
    let docker_env = DockerEnvironment {
        hostname: "alt-backend-logs".to_string(),
        target_service: Some("alt-backend".to_string()),
        rask_endpoint: "http://rask-aggregator:9600/v1/aggregate".to_string(),
        network_mode: Some("service:alt-backend".to_string()),
        container_labels: HashMap::new(),
    };

    let config = Config::from_docker_environment(&docker_env).unwrap();

    assert_eq!(config.target_service, Some("alt-backend".to_string()));
    assert_eq!(config.endpoint, "http://rask-aggregator:9600/v1/aggregate");
}

#[test]
fn test_service_label_detection() {
    let mut labels = HashMap::new();
    labels.insert("rask.group".to_string(), "alt-frontend".to_string());
    labels.insert("com.docker.compose.service".to_string(), "nginx".to_string());

    let docker_env = DockerEnvironment {
        hostname: "nginx-logs".to_string(),
        target_service: None,
        rask_endpoint: "http://rask-aggregator:9600/v1/aggregate".to_string(),
        network_mode: Some("service:nginx".to_string()),
        container_labels: labels,
    };

    let detected_service = docker_env.detect_target_service_from_labels().unwrap();
    assert_eq!(detected_service, "nginx");
}

#[test]
fn test_sidecar_mode_detection() {
    let docker_env = DockerEnvironment {
        hostname: "test-logs".to_string(),
        target_service: None,
        rask_endpoint: "http://rask-aggregator:9600/v1/aggregate".to_string(),
        network_mode: Some("service:test-service".to_string()),
        container_labels: HashMap::new(),
    };

    assert!(docker_env.is_sidecar_mode());

    let target_service = docker_env.get_target_service_from_network_mode().unwrap();
    assert_eq!(target_service, "test-service");
}

#[test]
fn test_docker_compose_integration() {
    // Test configuration that matches the compose.yaml structure
    let config = Config {
        target_service: Some("nginx".to_string()),
        endpoint: "http://rask-aggregator:9600/v1/aggregate".to_string(),
        batch_size: 10000,
        buffer_capacity: 100000,
        enable_disk_fallback: true,
        disk_fallback_path: std::path::PathBuf::from("/tmp/rask-fallback"),
        enable_metrics: true,
        metrics_port: 9090,
        enable_compression: true,
        ..Default::default()
    };

    // Validate Docker-specific requirements
    config.validate().unwrap();
    assert!(config.endpoint.contains("rask-aggregator"));
    assert_eq!(config.batch_size, 10000);
}

#[test]
fn test_docker_requirements_validation() {
    let config = Config {
        endpoint: "http://rask-aggregator:9600/v1/aggregate".to_string(),
        enable_disk_fallback: true,
        disk_fallback_path: std::path::PathBuf::from("/tmp/rask-fallback"),
        ..Default::default()
    };

    validate_docker_requirements(&config).unwrap();
}

#[test]
fn test_docker_requirements_validation_fails() {
    let config = Config {
        endpoint: "http://wrong-endpoint:9600/v1/aggregate".to_string(),
        ..Default::default()
    };

    assert!(validate_docker_requirements(&config).is_err());
}

#[tokio::test]
async fn test_docker_environment_config() {
    unsafe {
        env::set_var("HOSTNAME", "nginx-logs");
        env::set_var("TARGET_SERVICE", "nginx");
        env::set_var("RASK_ENDPOINT", "http://rask-aggregator:9600/v1/aggregate");
    }

    let config = Config::from_env().unwrap();

    assert_eq!(config.target_service, Some("nginx".to_string()));
    assert_eq!(config.endpoint, "http://rask-aggregator:9600/v1/aggregate");

    // Cleanup
    unsafe {
        env::remove_var("HOSTNAME");
        env::remove_var("TARGET_SERVICE");
        env::remove_var("RASK_ENDPOINT");
    }
}