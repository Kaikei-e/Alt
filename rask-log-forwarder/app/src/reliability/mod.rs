pub mod disk;
pub mod health;
pub mod metrics;
pub mod retry;

pub use crate::buffer::MetricsError;
pub use disk::{DiskConfig, DiskError, DiskFallback};
pub use health::{ComponentHealth, HealthConfig, HealthMonitor, HealthReport, HealthStatus};
pub use metrics::{MetricsCollector, MetricsConfig, MetricsSnapshot, PrometheusExporter};
pub use retry::{RetryConfig, RetryError, RetryManager, RetryStrategy};

use crate::buffer::Batch;
use crate::sender::{LogSender, TransmissionError};
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::sync::Mutex;
use tokio_util::sync::CancellationToken;

pub struct ReliabilityManager {
    retry_manager: Arc<Mutex<RetryManager>>,
    /// `None` when ENABLE_DISK_FALLBACK is false. Not a "might not be wired
    /// yet" option: the two states are distinguished by a startup log, and the
    /// disabled branch fails loudly rather than pretending the batch was saved.
    disk_fallback: Option<Arc<Mutex<DiskFallback>>>,
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
        // Loud either way, so "operator turned it off" is never confused with
        // "the wiring forgot about it" (CLAUDE.md rule 8).
        let disk_fallback = if disk_config.enabled {
            tracing::info!(
                storage_path = %disk_config.storage_path.display(),
                max_disk_usage = disk_config.max_disk_usage,
                "disk_fallback_enabled"
            );
            Some(Arc::new(Mutex::new(DiskFallback::new(disk_config).await?)))
        } else {
            tracing::warn!(
                "disk_fallback_disabled: batches that exhaust the retry budget will be dropped, \
                 not persisted"
            );
            None
        };
        let metrics_collector = Arc::new(Mutex::new(
            MetricsCollector::new(metrics_config).unwrap_or_else(|e| {
                tracing::error!(
                    "Failed to create metrics collector: {}, using legacy fallback",
                    e
                );
                MetricsCollector::new_legacy(MetricsConfig::default())
            }),
        ));
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
                    }
                    tracing::warn!(
                        "Transmission failed with HTTP {}: {}",
                        result.status_code,
                        batch_id
                    );
                }
                Err(e) => {
                    tracing::error!("Transmission error for batch {}: {}", batch_id, e);
                }
            }

            // Increment retry attempt and compute the backoff delay (if any)
            // while holding the locks, then drop them before sleeping -
            // holding `retry_manager`/`metrics` across the (up to 60s) sleep
            // would block every other batch's retry bookkeeping in the
            // meantime.
            let delay_to_wait = {
                let mut retry_manager = self.retry_manager.lock().await;
                retry_manager.increment_attempt(&batch_id);
                let attempt_count = retry_manager.get_attempt_count(&batch_id);

                {
                    let mut metrics = self.metrics_collector.lock().await;
                    metrics.record_retry_attempt(&batch_id, attempt_count).await;
                }

                if retry_manager.should_give_up(&batch_id) {
                    None
                } else {
                    Some((retry_manager.calculate_delay(attempt_count), attempt_count))
                }
            };

            if let Some((delay, attempt_count)) = delay_to_wait {
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

    async fn store_to_disk_fallback(&self, batch: Batch) -> Result<(), TransmissionError> {
        let batch_id = batch.id().to_string();
        let entry_count = batch.size();

        let Some(store) = self.disk_fallback.as_ref() else {
            // Disabled by config, so this batch is genuinely lost. Say so at
            // error level with the count — silently returning Ok here would
            // report a successful persist that never happened.
            tracing::error!(
                batch_id = %batch_id,
                entry_count,
                "disk_fallback_disabled: dropping batch that exhausted its retry budget"
            );
            return Err(TransmissionError::ClientError(
                crate::sender::ClientError::InvalidConfiguration(
                    "disk fallback is disabled; batch dropped after retry exhaustion".to_string(),
                ),
            ));
        };

        tracing::warn!(
            "Storing batch {} to disk fallback after failed retries",
            batch_id
        );

        {
            let mut disk_fallback = store.lock().await;
            if let Err(e) = disk_fallback.store_batch(batch).await {
                tracing::error!("Failed to store batch {batch_id} to disk: {e}");
                self.health_monitor
                    .update_component_health(
                        "disk_fallback",
                        ComponentHealth::Unhealthy(format!("Disk storage failed: {e}")),
                    )
                    .await;
                return Err(TransmissionError::ClientError(
                    crate::sender::ClientError::InvalidConfiguration(format!(
                        "Disk fallback failed: {e}"
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

    /// Attempts to resend every batch currently sitting in disk-fallback
    /// storage. Batches that resend successfully are deleted from disk;
    /// batches that still fail are left in place for the next replay pass
    /// (and are eventually cleared by `cleanup_old_batches`'s retention
    /// policy). Without this, disk fallback is a write-only sink: it
    /// protects against data loss on the wire, but nothing ever reads the
    /// data back out and forwards it on.
    pub async fn replay_stored_batches(&self) -> Result<usize, DiskError> {
        // Nothing was ever written when the store is off, so there is nothing
        // to replay. Not a silent skip: construction already logged
        // disk_fallback_disabled.
        let Some(store) = self.disk_fallback.as_ref() else {
            return Ok(0);
        };

        let batch_ids = {
            let disk_fallback = store.lock().await;
            disk_fallback.list_stored_batches().await?
        };

        let mut replayed = 0;
        for batch_id in batch_ids {
            let batch = {
                let disk_fallback = store.lock().await;
                match disk_fallback.retrieve_batch(&batch_id).await {
                    Ok(batch) => batch,
                    Err(e) => {
                        tracing::warn!(
                            "Disk fallback replay: failed to read stored batch {batch_id}: {e}"
                        );
                        continue;
                    }
                }
            };

            let sent = match self.log_sender.send_batch(batch).await {
                Ok(result) => result.success,
                Err(e) => {
                    tracing::warn!(
                        "Disk fallback replay: resend of batch {batch_id} failed: {e}; will retry next pass"
                    );
                    false
                }
            };

            if sent {
                let mut disk_fallback = store.lock().await;
                if let Err(e) = disk_fallback.delete_batch(&batch_id).await {
                    tracing::error!(
                        "Disk fallback replay: resent batch {batch_id} but failed to delete it from disk: {e}"
                    );
                } else {
                    replayed += 1;
                    tracing::info!("Disk fallback replay: successfully resent batch {batch_id}");
                }
            }
        }

        Ok(replayed)
    }

    /// Spawns a periodic task that replays disk-fallback batches, running one
    /// pass immediately so batches persisted before a restart aren't stuck
    /// until the first interval tick. Stops once `cancel_token` is cancelled.
    pub fn start_disk_replay_task(
        self: Arc<Self>,
        interval: Duration,
        cancel_token: CancellationToken,
    ) -> tokio::task::JoinHandle<()> {
        tokio::spawn(async move {
            loop {
                match self.replay_stored_batches().await {
                    Ok(0) => {}
                    Ok(n) => tracing::info!("Disk fallback replay: resent {n} batch(es)"),
                    Err(e) => tracing::error!("Disk fallback replay pass failed: {e}"),
                }

                tokio::select! {
                    _ = cancel_token.cancelled() => break,
                    _ = tokio::time::sleep(interval) => {}
                }
            }
        })
    }

    /// Deletes disk-fallback batches older than the configured retention
    /// period. Without a caller running this, batches that permanently fail
    /// to replay (e.g. the aggregator rejects them with a persistent 4xx)
    /// stay on disk forever and eventually trip `DiskSpaceExceeded`.
    pub async fn cleanup_disk_fallback(&self) -> Result<u32, DiskError> {
        // Nothing is stored when the fallback is off, so there is nothing to
        // reap. Construction already logged disk_fallback_disabled.
        let Some(store) = self.disk_fallback.as_ref() else {
            return Ok(0);
        };
        let mut disk_fallback = store.lock().await;
        disk_fallback.cleanup_old_batches().await
    }

    /// Test-only handle on the disk store. Panics when the fallback is
    /// disabled, so a test that means to exercise storage cannot quietly pass
    /// against a manager that has no store at all.
    #[cfg(test)]
    fn disk_store(&self) -> &Arc<Mutex<DiskFallback>> {
        self.disk_fallback
            .as_ref()
            .expect("this test requires DiskConfig::enabled = true")
    }

    /// Spawns a periodic task that deletes disk-fallback batches past their
    /// retention period, so the `store_batch` disk-usage cap can't be
    /// permanently exhausted by batches that keep failing to replay. Stops
    /// once `cancel_token` is cancelled.
    pub fn start_disk_cleanup_task(
        self: Arc<Self>,
        interval: Duration,
        cancel_token: CancellationToken,
    ) -> tokio::task::JoinHandle<()> {
        tokio::spawn(async move {
            loop {
                tokio::select! {
                    _ = cancel_token.cancelled() => break,
                    _ = tokio::time::sleep(interval) => {}
                }

                match self.cleanup_disk_fallback().await {
                    Ok(0) => {}
                    Ok(n) => tracing::info!("Disk fallback cleanup: removed {n} expired batch(es)"),
                    Err(e) => tracing::error!("Disk fallback cleanup pass failed: {e}"),
                }
            }
        })
    }

    pub async fn get_health_report(&self) -> HealthReport {
        HealthReport::generate(&self.health_monitor, self.start_time).await
    }

    pub async fn get_metrics_snapshot(&self) -> MetricsSnapshot {
        let metrics = self.metrics_collector.lock().await;
        metrics.snapshot().await
    }
}

#[cfg(test)]
mod replay_tests {
    use super::*;
    use crate::buffer::BatchType;
    use crate::parser::{EnrichedLogEntry, LogLevel};
    use crate::sender::ClientConfig;
    use wiremock::matchers::method;
    use wiremock::{Mock, MockServer, ResponseTemplate};

    fn test_entry() -> EnrichedLogEntry {
        EnrichedLogEntry {
            service_type: "test".to_string(),
            log_type: "plain".to_string(),
            message: "disk fallback replay test".to_string(),
            level: Some(LogLevel::Info),
            timestamp: "2024-01-01T00:00:00Z".to_string(),
            stream: "stdout".to_string(),
            method: None,
            path: None,
            status_code: None,
            response_size: None,
            ip_address: None,
            user_agent: None,
            container_id: "container-1".to_string(),
            service_name: "test-service".to_string(),
            service_group: None,
            trace_id: None,
            span_id: None,
            fields: std::collections::HashMap::new(),
        }
    }

    async fn build_manager(
        endpoint: String,
        storage_path: std::path::PathBuf,
    ) -> ReliabilityManager {
        build_manager_with_retention(endpoint, storage_path, Duration::from_secs(24 * 3600)).await
    }

    async fn build_manager_with_retention(
        endpoint: String,
        storage_path: std::path::PathBuf,
        retention_period: Duration,
    ) -> ReliabilityManager {
        let log_sender = LogSender::new(ClientConfig {
            endpoint,
            timeout: Duration::from_millis(500),
            connection_timeout: Duration::from_millis(500),
            ..Default::default()
        })
        .await
        .unwrap();

        ReliabilityManager::new(
            RetryConfig::default(),
            DiskConfig {
                storage_path,
                retention_period,
                ..Default::default()
            },
            MetricsConfig::default(),
            HealthConfig::default(),
            log_sender,
        )
        .await
        .unwrap()
    }

    #[tokio::test]
    async fn replay_resends_and_deletes_successfully_sent_batches() {
        let mock_server = MockServer::start().await;
        Mock::given(method("POST"))
            .respond_with(ResponseTemplate::new(200))
            .mount(&mock_server)
            .await;

        let storage_dir = tempfile::tempdir().unwrap();
        let manager = build_manager(mock_server.uri(), storage_dir.path().to_path_buf()).await;

        // Seed disk fallback directly (bypassing the network path) to pin
        // down replay behavior in isolation.
        let batch = Batch::new(vec![test_entry()], BatchType::SizeBased);
        {
            let mut disk_fallback = manager.disk_store().lock().await;
            disk_fallback.store_batch(batch).await.unwrap();
        }
        assert_eq!(storage_dir.path().read_dir().unwrap().count(), 1);

        let replayed = manager.replay_stored_batches().await.unwrap();

        assert_eq!(replayed, 1);
        assert_eq!(
            storage_dir.path().read_dir().unwrap().count(),
            0,
            "a successfully replayed batch must be removed from disk, not left behind forever"
        );
    }

    #[tokio::test]
    async fn replay_leaves_batch_on_disk_when_resend_still_fails() {
        let mock_server = MockServer::start().await;
        Mock::given(method("POST"))
            .respond_with(ResponseTemplate::new(500))
            .mount(&mock_server)
            .await;

        let storage_dir = tempfile::tempdir().unwrap();
        let manager = build_manager(mock_server.uri(), storage_dir.path().to_path_buf()).await;

        let batch = Batch::new(vec![test_entry()], BatchType::SizeBased);
        {
            let mut disk_fallback = manager.disk_store().lock().await;
            disk_fallback.store_batch(batch).await.unwrap();
        }

        let replayed = manager.replay_stored_batches().await.unwrap();

        assert_eq!(replayed, 0);
        assert_eq!(
            storage_dir.path().read_dir().unwrap().count(),
            1,
            "a batch that still fails to send must stay on disk for the next replay pass"
        );
    }

    #[tokio::test]
    async fn start_disk_replay_task_stops_when_cancelled() {
        let mock_server = MockServer::start().await;
        Mock::given(method("POST"))
            .respond_with(ResponseTemplate::new(200))
            .mount(&mock_server)
            .await;

        let storage_dir = tempfile::tempdir().unwrap();
        let manager =
            Arc::new(build_manager(mock_server.uri(), storage_dir.path().to_path_buf()).await);

        let cancel_token = CancellationToken::new();
        let handle = manager
            .clone()
            .start_disk_replay_task(Duration::from_millis(20), cancel_token.clone());

        // Let it run a couple of passes, then cancel and confirm the task
        // actually terminates promptly instead of leaking a background loop
        // that spins forever past shutdown.
        tokio::time::sleep(Duration::from_millis(60)).await;
        cancel_token.cancel();

        tokio::time::timeout(Duration::from_millis(500), handle)
            .await
            .expect("disk replay task must stop once cancelled")
            .expect("disk replay task must not panic");
    }

    #[tokio::test]
    async fn cleanup_disk_fallback_deletes_batches_past_retention() {
        let storage_dir = tempfile::tempdir().unwrap();
        // Retention period shorter than the sleep below, so the stored batch
        // is already expired by the time cleanup runs.
        let manager = build_manager_with_retention(
            "http://127.0.0.1:1".to_string(),
            storage_dir.path().to_path_buf(),
            Duration::from_millis(1),
        )
        .await;

        let batch = Batch::new(vec![test_entry()], BatchType::SizeBased);
        {
            let mut disk_fallback = manager.disk_store().lock().await;
            disk_fallback.store_batch(batch).await.unwrap();
        }
        assert_eq!(storage_dir.path().read_dir().unwrap().count(), 1);

        tokio::time::sleep(Duration::from_millis(1100)).await;

        let deleted = manager.cleanup_disk_fallback().await.unwrap();

        assert_eq!(deleted, 1);
        assert_eq!(
            storage_dir.path().read_dir().unwrap().count(),
            0,
            "a batch older than the retention period must be deleted by cleanup, \
             otherwise permanently-failing batches never leave disk"
        );
    }

    #[tokio::test]
    async fn start_disk_cleanup_task_stops_when_cancelled() {
        let storage_dir = tempfile::tempdir().unwrap();
        let manager = Arc::new(
            build_manager_with_retention(
                "http://127.0.0.1:1".to_string(),
                storage_dir.path().to_path_buf(),
                Duration::from_secs(24 * 3600),
            )
            .await,
        );

        let cancel_token = CancellationToken::new();
        let handle = manager
            .clone()
            .start_disk_cleanup_task(Duration::from_millis(20), cancel_token.clone());

        tokio::time::sleep(Duration::from_millis(60)).await;
        cancel_token.cancel();

        tokio::time::timeout(Duration::from_millis(500), handle)
            .await
            .expect("disk cleanup task must stop once cancelled")
            .expect("disk cleanup task must not panic");
    }

    /// ENABLE_DISK_FALLBACK=false must actually turn the disk store off.
    /// The flag was parsed into Config and then never read, so every forwarder
    /// kept gzipping batches into its fallback directory regardless — up to
    /// max_disk_usage per container, on hosts where an operator had explicitly
    /// said no.
    #[tokio::test]
    async fn disk_fallback_is_not_created_when_disabled_in_config() {
        let temp = tempfile::tempdir().unwrap();
        let storage_path = temp.path().join("fallback");

        let log_sender = LogSender::new(ClientConfig {
            endpoint: "http://127.0.0.1:1".to_string(),
            timeout: Duration::from_millis(500),
            connection_timeout: Duration::from_millis(500),
            ..Default::default()
        })
        .await
        .unwrap();

        let _manager = ReliabilityManager::new(
            RetryConfig::default(),
            DiskConfig {
                storage_path: storage_path.clone(),
                enabled: false,
                ..Default::default()
            },
            MetricsConfig::default(),
            HealthConfig::default(),
            log_sender,
        )
        .await
        .unwrap();

        assert!(
            !storage_path.exists(),
            "ENABLE_DISK_FALLBACK=false must not stand up the disk-fallback store; \
             every forwarder otherwise keeps writing gzip batches an operator turned off"
        );
    }
}
