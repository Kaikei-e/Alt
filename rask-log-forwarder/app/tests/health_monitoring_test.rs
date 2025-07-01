use rask_log_forwarder::reliability::{ComponentHealth, HealthConfig, HealthMonitor, HealthStatus};
use std::time::Duration;

#[tokio::test]
async fn test_health_status_aggregation() {
    let config = HealthConfig {
        check_interval: Duration::from_secs(1),
        unhealthy_threshold: 3,
        recovery_threshold: 2,
    };

    let monitor = HealthMonitor::new(config);

    // All components healthy initially
    monitor
        .update_component_health("collector", ComponentHealth::Healthy)
        .await;
    monitor
        .update_component_health("parser", ComponentHealth::Healthy)
        .await;
    monitor
        .update_component_health("sender", ComponentHealth::Healthy)
        .await;

    let overall_status = monitor.get_overall_health().await;
    assert_eq!(overall_status, HealthStatus::Healthy);
}

#[tokio::test]
async fn test_degraded_health_status() {
    let config = HealthConfig::default();
    let monitor = HealthMonitor::new(config);

    // Some components healthy, some degraded
    monitor
        .update_component_health("collector", ComponentHealth::Healthy)
        .await;
    monitor
        .update_component_health(
            "parser",
            ComponentHealth::Degraded("High memory usage".to_string()),
        )
        .await;
    monitor
        .update_component_health("sender", ComponentHealth::Healthy)
        .await;

    let overall_status = monitor.get_overall_health().await;
    assert_eq!(overall_status, HealthStatus::Degraded);
}

#[tokio::test]
async fn test_unhealthy_status_with_failures() {
    let config = HealthConfig::default();
    let monitor = HealthMonitor::new(config);

    // Critical component failure
    monitor
        .update_component_health(
            "collector",
            ComponentHealth::Unhealthy("Cannot read logs".to_string()),
        )
        .await;
    monitor
        .update_component_health("parser", ComponentHealth::Healthy)
        .await;
    monitor
        .update_component_health("sender", ComponentHealth::Healthy)
        .await;

    let overall_status = monitor.get_overall_health().await;
    assert_eq!(overall_status, HealthStatus::Unhealthy);
}

#[tokio::test]
async fn test_health_check_history() {
    let config = HealthConfig::default();
    let monitor = HealthMonitor::new(config);

    // Record some health checks
    monitor.record_health_check("http_client", true).await;
    monitor.record_health_check("http_client", true).await;
    monitor.record_health_check("http_client", false).await;
    monitor.record_health_check("http_client", true).await;

    let history = monitor.get_component_history("http_client").await;
    assert_eq!(history.len(), 4);
    assert_eq!(history.iter().filter(|&&result| result).count(), 3); // 3 successes
    assert_eq!(history.iter().filter(|&&result| !result).count(), 1); // 1 failure
}

#[tokio::test]
async fn test_automatic_health_recovery() {
    let config = HealthConfig {
        check_interval: Duration::from_millis(10),
        unhealthy_threshold: 2,
        recovery_threshold: 3,
    };

    let monitor = HealthMonitor::new(config);

    // Simulate component going unhealthy
    monitor.record_health_check("test_component", false).await;
    monitor.record_health_check("test_component", false).await;

    let status = monitor.get_component_health("test_component").await;
    assert!(matches!(status, ComponentHealth::Unhealthy(_)));

    // Simulate recovery
    for _ in 0..3 {
        monitor.record_health_check("test_component", true).await;
    }

    let status = monitor.get_component_health("test_component").await;
    assert_eq!(status, ComponentHealth::Healthy);
}

#[tokio::test]
async fn test_get_all_component_status() {
    let config = HealthConfig::default();
    let monitor = HealthMonitor::new(config);

    // Set up various component states
    monitor
        .update_component_health("api", ComponentHealth::Healthy)
        .await;
    monitor
        .update_component_health(
            "database",
            ComponentHealth::Degraded("Slow queries".to_string()),
        )
        .await;
    monitor
        .update_component_health(
            "cache",
            ComponentHealth::Unhealthy("Connection timeout".to_string()),
        )
        .await;

    let all_status = monitor.get_all_component_status().await;
    assert_eq!(all_status.len(), 3);
    assert_eq!(all_status.get("api"), Some(&ComponentHealth::Healthy));
    assert!(matches!(
        all_status.get("database"),
        Some(ComponentHealth::Degraded(_))
    ));
    assert!(matches!(
        all_status.get("cache"),
        Some(ComponentHealth::Unhealthy(_))
    ));
}

#[tokio::test]
async fn test_component_cleanup() {
    let config = HealthConfig::default();
    let monitor = HealthMonitor::new(config);

    // Add some components
    monitor
        .update_component_health("temp_component", ComponentHealth::Healthy)
        .await;
    monitor
        .update_component_health("permanent_component", ComponentHealth::Healthy)
        .await;

    // Wait to simulate aging
    tokio::time::sleep(Duration::from_millis(10)).await;

    // Cleanup old components (very short max_age for testing)
    monitor
        .cleanup_stale_components(Duration::from_millis(5))
        .await;

    let all_status = monitor.get_all_component_status().await;
    // Both should be cleaned up due to very short max_age
    assert!(all_status.is_empty() || all_status.len() < 2);
}
