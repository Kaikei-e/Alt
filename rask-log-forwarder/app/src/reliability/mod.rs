pub mod disk;
pub mod health;
pub mod metrics;
pub mod retry;

pub use disk::{DiskConfig, DiskError, DiskFallback};
pub use health::{ComponentHealth, HealthConfig, HealthMonitor, HealthReport, HealthStatus};
pub use metrics::{
    MetricsCollector, MetricsConfig, MetricsError, MetricsSnapshot, PrometheusExporter,
};
pub use retry::{RetryConfig, RetryError, RetryManager, RetryStrategy};

use crate::buffer::Batch;
use crate::sender::{LogSender, TransmissionError};
use std::sync::Arc;
use std::time::Instant;
use tokio::sync::Mutex;

pub struct ReliabilityManager {
    retry_manager: Arc<Mutex<RetryManager>>,
    disk_fallback: Arc<Mutex<DiskFallback>>,
    metrics_collector: Arc<Mutex<MetricsCollector>>,
    health_monitor: Arc<HealthMonitor>,
    log_sender: LogSender,
    start_time: Instant,
}

impl ReliabilityManager {
    pub async fn new(
        retry_config: RetryConfig,
        disk_config: DiskConfig,
        metrics_config: MetricsConfig,
        health_config: HealthConfig,
        log_sender: LogSender,
    ) -> Result<Self, Box<dyn std::error::Error + Send + Sync>> {
        let retry_manager = Arc::new(Mutex::new(RetryManager::new(retry_config)));
        let disk_fallback = Arc::new(Mutex::new(DiskFallback::new(disk_config).await?));
        let metrics_collector = Arc::new(Mutex::new(MetricsCollector::new(metrics_config)));
        let health_monitor = Arc::new(HealthMonitor::new(health_config));

        Ok(Self {
            retry_manager,
            disk_fallback,
            metrics_collector,
            health_monitor,
            log_sender,
            start_time: Instant::now(),
        })
    }

    pub async fn send_batch_with_reliability(&self, batch: Batch) -> Result<(), TransmissionError> {
        let batch_id = batch.id().to_string();
        let entry_count = batch.size();

        // Start retry tracking
        {
            let mut retry_manager = self.retry_manager.lock().await;
            retry_manager.start_retry(&batch_id);
        }

        // Attempt transmission with retries
        let result = self.attempt_transmission_with_retries(batch).await;

        // Record metrics
        {
            let mut metrics = self.metrics_collector.lock().await;
            match &result {
                Ok(_) => {
                    metrics
                        .record_batch_sent(entry_count, true, std::time::Duration::from_millis(0))
                        .await;
                    self.health_monitor
                        .record_health_check("transmission", true)
                        .await;
                }
                Err(_) => {
                    metrics
                        .record_batch_sent(entry_count, false, std::time::Duration::from_millis(0))
                        .await;
                    self.health_monitor
                        .record_health_check("transmission", false)
                        .await;
                }
            }
        }

        // Clean up retry tracking
        {
            let mut retry_manager = self.retry_manager.lock().await;
            retry_manager.remove_retry(&batch_id);
        }

        result
    }

    async fn attempt_transmission_with_retries(
        &self,
        batch: Batch,
    ) -> Result<(), TransmissionError> {
        let batch_id = batch.id().to_string();

        loop {
            // Check if we should give up
            {
                let retry_manager = self.retry_manager.lock().await;
                if retry_manager.should_give_up(&batch_id) {
                    // Store to disk fallback as last resort
                    return self.store_to_disk_fallback(batch).await;
                }
            }

            // Attempt transmission
            match self.log_sender.send_batch(batch.clone()).await {
                Ok(result) => {
                    if result.success {
                        tracing::info!(
                            "Successfully transmitted batch {} on attempt {}",
                            batch_id,
                            self.get_attempt_count(&batch_id).await
                        );
                        return Ok(());
                    } else {
                        tracing::warn!(
                            "Transmission failed with HTTP {}: {}",
                            result.status_code,
                            batch_id
                        );
                    }
                }
                Err(e) => {
                    tracing::error!("Transmission error for batch {}: {}", batch_id, e);
                }
            }

            // Increment retry attempt
            {
                let mut retry_manager = self.retry_manager.lock().await;
                retry_manager.increment_attempt(&batch_id);

                let mut metrics = self.metrics_collector.lock().await;
                let attempt_count = retry_manager.get_attempt_count(&batch_id);
                metrics.record_retry_attempt(&batch_id, attempt_count).await;

                // Wait for next retry
                if !retry_manager.should_give_up(&batch_id) {
                    let delay = retry_manager.calculate_delay(attempt_count);
                    tracing::info!(
                        "Retrying batch {} in {:?} (attempt {})",
                        batch_id,
                        delay,
                        attempt_count + 1
                    );
                    tokio::time::sleep(delay).await;
                }
            }
        }
    }

    async fn store_to_disk_fallback(&self, batch: Batch) -> Result<(), TransmissionError> {
        let batch_id = batch.id().to_string();
        let entry_count = batch.size();

        tracing::warn!(
            "Storing batch {} to disk fallback after failed retries",
            batch_id
        );

        {
            let mut disk_fallback = self.disk_fallback.lock().await;
            if let Err(e) = disk_fallback.store_batch(batch).await {
                tracing::error!("Failed to store batch {} to disk: {}", batch_id, e);
                self.health_monitor
                    .update_component_health(
                        "disk_fallback",
                        ComponentHealth::Unhealthy(format!("Disk storage failed: {}", e)),
                    )
                    .await;
                return Err(TransmissionError::ClientError(
                    crate::sender::ClientError::InvalidConfiguration(format!(
                        "Disk fallback failed: {}",
                        e
                    )),
                ));
            }

            let metrics = self.metrics_collector.lock().await;
            metrics.record_disk_fallback(entry_count);
        }

        self.health_monitor
            .update_component_health("disk_fallback", ComponentHealth::Healthy)
            .await;
        Ok(())
    }

    async fn get_attempt_count(&self, batch_id: &str) -> u32 {
        let retry_manager = self.retry_manager.lock().await;
        retry_manager.get_attempt_count(batch_id)
    }

    pub async fn start_background_tasks(&self) {
        // Start metrics collection
        let metrics_collector = self.metrics_collector.clone();
        tokio::spawn(async move {
            let mut interval = tokio::time::interval(std::time::Duration::from_secs(60));
            loop {
                interval.tick().await;
                // Update system metrics
                let memory_usage = Self::get_memory_usage();
                let metrics = metrics_collector.lock().await;
                metrics.update_memory_usage(memory_usage);
            }
        });

        // Start disk cleanup
        let disk_fallback = self.disk_fallback.clone();
        tokio::spawn(async move {
            let mut interval = tokio::time::interval(std::time::Duration::from_secs(3600)); // Every hour
            loop {
                interval.tick().await;
                if let Ok(mut disk) = disk_fallback.try_lock() {
                    if let Err(e) = disk.cleanup_old_batches().await {
                        tracing::error!("Disk cleanup failed: {}", e);
                    }
                }
            }
        });

        // Start health monitoring
        let health_monitor = self.health_monitor.clone();
        tokio::spawn(async move {
            let mut interval = tokio::time::interval(std::time::Duration::from_secs(300)); // Every 5 minutes
            loop {
                interval.tick().await;
                health_monitor
                    .cleanup_stale_components(std::time::Duration::from_secs(1800))
                    .await; // 30 minutes
            }
        });
    }

    pub async fn get_health_report(&self) -> HealthReport {
        HealthReport::generate(&self.health_monitor, self.start_time).await
    }

    pub async fn get_metrics_snapshot(&self) -> MetricsSnapshot {
        let metrics = self.metrics_collector.lock().await;
        metrics.snapshot().await
    }

    fn get_memory_usage() -> u64 {
        // Simple memory usage estimation
        // In production, you'd use a proper memory profiling library
        // For now, return a fixed value for testing
        16 * 1024 * 1024 // 16MB
    }
}
