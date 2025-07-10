use rask_log_forwarder::reliability::{MetricsCollector, MetricsConfig, PrometheusExporter};
use std::time::Duration;

#[tokio::test]
async fn test_metrics_collection() {
    let config = MetricsConfig {
        enabled: true,
        export_port: 9090,
        export_path: "/metrics".to_string(),
        collection_interval: Duration::from_secs(10),
    };

    let mut collector = MetricsCollector::new(config).expect("Failed to create metrics collector");

    // Record some metrics
    collector
        .record_batch_sent(1000, true, Duration::from_millis(50))
        .await;
    collector
        .record_batch_sent(2000, false, Duration::from_millis(200))
        .await;
    collector.record_disk_fallback(500);
    collector.record_retry_attempt("batch-123", 2).await;

    // Get snapshot
    let snapshot = collector.snapshot().await;

    assert_eq!(snapshot.total_batches_sent, 2);
    assert_eq!(snapshot.successful_batches, 1);
    assert_eq!(snapshot.failed_batches, 1);
    assert_eq!(snapshot.total_entries_sent, 3000);
    assert_eq!(snapshot.disk_fallback_count, 1);
    assert_eq!(snapshot.retry_attempts, 1);
}

#[tokio::test]
async fn test_prometheus_exposition() {
    let config = MetricsConfig {
        enabled: true,
        export_port: 9091,
        export_path: "/metrics".to_string(),
        collection_interval: Duration::from_secs(1),
    };

    let mut collector = MetricsCollector::new(config.clone()).expect("Failed to create metrics collector");
    let exporter = PrometheusExporter::new(config, collector.clone())
        .await
        .unwrap();

    // Record some test metrics
    collector
        .record_batch_sent(100, true, Duration::from_millis(25))
        .await;
    collector.record_connection_stats(5, 3);

    // Test metrics export
    let metrics_text = exporter.export_metrics().unwrap();

    // Verify Prometheus format
    assert!(metrics_text.contains("rask_batches_sent_total"));
    assert!(metrics_text.contains("rask_transmission_latency_seconds"));
}

#[test]
fn test_metrics_labels() {
    let config = MetricsConfig::default();
    let collector = MetricsCollector::new(config).expect("Failed to create metrics collector");

    // Test metric name generation
    assert_eq!(
        collector.get_metric_name("batches_sent"),
        "rask_batches_sent_total"
    );
    assert_eq!(collector.get_metric_name("latency"), "rask_latency_seconds");
}

#[tokio::test]
async fn test_health_check_metrics() {
    let config = MetricsConfig::default();
    let collector = MetricsCollector::new(config).expect("Failed to create metrics collector");

    // Record health check results using async version for tests
    collector.record_health_check_async(true).await;
    collector.record_health_check_async(true).await;
    collector.record_health_check_async(false).await;

    let snapshot = collector.snapshot().await;
    assert_eq!(snapshot.health_check_total, 3);
    assert_eq!(snapshot.health_check_success, 2);
    assert_eq!(snapshot.health_check_failure, 1);
}

#[tokio::test]
async fn test_metrics_reset() {
    let config = MetricsConfig::default();
    let mut collector = MetricsCollector::new(config).expect("Failed to create metrics collector");

    // Record some metrics
    collector
        .record_batch_sent(100, true, Duration::from_millis(25))
        .await;
    collector
        .record_batch_sent(200, false, Duration::from_millis(50))
        .await;

    let snapshot1 = collector.snapshot().await;
    assert_eq!(snapshot1.total_batches_sent, 2);

    // Reset metrics (should return to zero)
    collector.reset_metrics().await;

    let snapshot2 = collector.snapshot().await;
    assert_eq!(snapshot2.total_batches_sent, 0);
    assert_eq!(snapshot2.successful_batches, 0);
    assert_eq!(snapshot2.failed_batches, 0);
}
