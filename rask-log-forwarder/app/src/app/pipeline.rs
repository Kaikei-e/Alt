use super::protocol::SenderConfig;
use crate::{
    buffer::{Batch, BatchType},
    collector::LogCollector,
    domain::EnrichedLogEntry,
    parser::UniversalParser,
    reliability::ReliabilityManager,
    sender::LogSender,
};
use std::sync::Arc;
use std::time::Duration;
use tokio::sync::{RwLock, mpsc};
use tokio_util::sync::CancellationToken;
use tracing::{error, info};

/// Bundled parameters for the main processing loop.
pub struct ProcessingLoopParams {
    pub collector: Arc<tokio::sync::Mutex<LogCollector>>,
    pub parser: Arc<UniversalParser>,
    pub reliability_manager: Arc<ReliabilityManager>,
    pub sender: Arc<LogSender>,
    pub running: Arc<RwLock<bool>>,
    pub shutdown_rx: mpsc::UnboundedReceiver<()>,
    pub target_service: String,
    pub sender_config: SenderConfig,
    pub batch_size: usize,
    pub flush_interval: Duration,
    /// Upper bound on the number of raw log entries buffered between the
    /// Docker collector and the batching/send loop. Keeps memory bounded when
    /// the aggregator is slow or unreachable, instead of growing forever.
    pub channel_capacity: usize,
    /// Shared shutdown signal: cancelled once the loop starts shutting down,
    /// so sibling background tasks spawned by the composition root (e.g. the
    /// disk-fallback replay task) stop at the same time as the collector.
    pub cancel_token: CancellationToken,
}

/// Run the main processing pipeline: collect → parse → batch → send.
pub async fn run_processing_loop(params: ProcessingLoopParams) {
    let ProcessingLoopParams {
        collector,
        parser,
        reliability_manager,
        sender,
        running,
        mut shutdown_rx,
        target_service,
        sender_config,
        batch_size,
        flush_interval,
        channel_capacity,
        cancel_token,
    } = params;

    info!(
        "Starting main processing loop (batch_size={}, flush_interval={:?}, channel_capacity={})",
        batch_size, flush_interval, channel_capacity
    );

    // Bounded log collection channel. Backpressure policy: once full, the
    // collector awaits (blocks) on send rather than growing memory without
    // bound or silently dropping entries; it still stays responsive to
    // cancellation while backpressured (see collector::start_docker_api_streaming).
    let (log_tx, mut log_rx) = tokio::sync::mpsc::channel(channel_capacity.max(1));

    // Start log collection in background with cancellation support
    let collector_clone = collector.clone();
    let collector_cancel_token = cancel_token.clone();
    tokio::spawn(async move {
        let mut collector_guard = collector_clone.lock().await;
        if let Err(e) = collector_guard
            .start_collection(log_tx, collector_cancel_token)
            .await
        {
            error!("Log collection failed: {}", e);
        }
    });

    // Buffer for batching logs - directly collect EnrichedLogEntry (no double conversion)
    let mut log_batch: Vec<EnrichedLogEntry> = Vec::with_capacity(batch_size);

    // Fire flush ticks on a fixed cadence, created once outside the loop so a
    // steady stream of incoming logs (faster than flush_interval) can never
    // starve the periodic-flush branch of `select!`.
    let mut flush_ticker = tokio::time::interval(flush_interval);
    flush_ticker.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Delay);

    // Main processing loop
    loop {
        tokio::select! {
            // Process incoming log entries
            Some(log_entry) = log_rx.recv() => {
                log_batch.push(enrich_log_entry(&parser, log_entry, &target_service).await);

                // Send batch when it reaches the configured size
                if log_batch.len() >= batch_size {
                    flush_batch(&mut log_batch, &reliability_manager, &sender, &sender_config).await;
                }
            }

            // Periodic flush for any remaining logs
            _ = flush_ticker.tick() => {
                if !log_batch.is_empty() {
                    flush_batch(&mut log_batch, &reliability_manager, &sender, &sender_config).await;
                }
            }

            // Handle shutdown signal
            _ = shutdown_rx.recv() => {
                info!("Received shutdown signal, stopping processing loop");
                // Cancel the collector to stop log streaming
                cancel_token.cancel();

                // Drain any entries already buffered in the channel before the
                // final flush - otherwise logs in flight at shutdown are lost.
                log_rx.close();
                while let Ok(log_entry) = log_rx.try_recv() {
                    log_batch.push(enrich_log_entry(&parser, log_entry, &target_service).await);
                }

                // Flush any remaining logs before shutting down
                if !log_batch.is_empty() {
                    flush_batch(&mut log_batch, &reliability_manager, &sender, &sender_config).await;
                }
                break;
            }
        }
    }

    *running.write().await = false;
    info!("Main processing loop stopped");
}

/// Parse a raw collected log entry into an `EnrichedLogEntry`, falling back to
/// a plain-text entry if parsing fails. Shared by the main select loop and the
/// shutdown drain path so both apply identical enrichment.
async fn enrich_log_entry(
    parser: &UniversalParser,
    log_entry: crate::collector::LogEntry,
    target_service: &str,
) -> EnrichedLogEntry {
    let container_info = crate::collector::ContainerInfo {
        id: log_entry.id.clone(),
        service_name: target_service.to_string(),
        group: None,
        labels: std::collections::HashMap::new(),
    };

    match parser
        .parse_docker_log(log_entry.raw_bytes.as_ref(), &container_info)
        .await
    {
        Ok(enriched_entry) => enriched_entry,
        Err(e) => {
            error!("Failed to parse log entry: {}", e);
            // Fallback: create a plain text EnrichedLogEntry directly
            EnrichedLogEntry {
                service_type: target_service.to_string(),
                log_type: "plain".to_string(),
                message: String::from_utf8_lossy(&log_entry.raw_bytes).to_string(),
                level: Some(crate::domain::LogLevel::Info),
                timestamp: chrono::Utc::now().to_rfc3339(),
                stream: "stdout".to_string(),
                method: None,
                path: None,
                status_code: None,
                response_size: None,
                ip_address: None,
                user_agent: None,
                container_id: log_entry.container_id.clone(),
                service_name: target_service.to_string(),
                service_group: None,
                trace_id: None,
                span_id: None,
                fields: std::collections::HashMap::new(),
            }
        }
    }
}

/// Flush the accumulated batch through the `ReliabilityManager` (retry +
/// disk-fallback), instead of a bare single-attempt send. Only truly
/// unrecoverable failures (transmission AND disk fallback both failed) reach
/// the `error!` log below - everything else is retried or durably persisted.
async fn flush_batch(
    log_batch: &mut Vec<EnrichedLogEntry>,
    reliability_manager: &Arc<ReliabilityManager>,
    _sender: &Arc<LogSender>,
    _sender_config: &SenderConfig,
) {
    if log_batch.is_empty() {
        return;
    }

    let entries = std::mem::take(log_batch);
    let entry_count = entries.len();

    info!("Flushing batch of {} log entries", entry_count);

    // Choose protocol based on configuration
    #[cfg(feature = "otlp")]
    {
        use super::config::Protocol;
        if matches!(_sender_config.protocol, Protocol::Otlp) {
            flush_otlp_batch(entries, _sender, _sender_config).await;
            return;
        }
    }

    // Default: NDJSON protocol, routed through the reliability manager so
    // transient failures are retried and, failing that, durably persisted to
    // disk instead of being dropped on the floor.
    let batch = Batch::new(entries, BatchType::SizeBased);
    match reliability_manager.send_batch_with_reliability(batch).await {
        Ok(()) => {
            info!(
                "Successfully delivered batch of {} logs (direct send or disk fallback)",
                entry_count
            );
        }
        Err(e) => {
            error!(
                "Batch of {} logs lost: transmission retries exhausted AND disk fallback failed: {e}",
                entry_count
            );
        }
    }
}

/// Send batch using OTLP protobuf format via the initialized sender's HTTP client.
#[cfg(feature = "otlp")]
async fn flush_otlp_batch(
    entries: Vec<EnrichedLogEntry>,
    sender: &Arc<LogSender>,
    sender_config: &SenderConfig,
) {
    use crate::sender::OtlpBatchTransmitter;

    let batch = Batch::new(entries, BatchType::SizeBased);
    let entry_count = batch.size();

    // Create OtlpBatchTransmitter reusing the sender's HTTP client
    let otlp_transmitter = match OtlpBatchTransmitter::new(
        sender.transmitter_client().clone(),
        &sender_config.otlp_endpoint,
    ) {
        Ok(t) => t,
        Err(e) => {
            error!("Failed to create OTLP transmitter: {e}");
            return;
        }
    };

    match otlp_transmitter.send_batch(batch).await {
        Ok(result) => {
            if result.success {
                info!(
                    "Successfully sent OTLP batch of {} logs in {:?}",
                    entry_count, result.latency
                );
            } else {
                error!(
                    "Failed to send OTLP logs, HTTP status: {}",
                    result.status_code
                );
            }
        }
        Err(e) => {
            error!("Failed to send OTLP logs: {e}");
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::LogLevel;
    use crate::reliability::{DiskConfig, HealthConfig, MetricsConfig, RetryConfig, RetryStrategy};
    use crate::sender::ClientConfig;
    use wiremock::matchers::method;
    use wiremock::{Mock, MockServer, ResponseTemplate};

    fn test_sender_config() -> SenderConfig {
        SenderConfig {
            #[cfg(feature = "otlp")]
            protocol: Default::default(),
            #[cfg(feature = "otlp")]
            otlp_endpoint: String::new(),
        }
    }

    fn test_entry(message: &str) -> EnrichedLogEntry {
        EnrichedLogEntry {
            service_type: "test".to_string(),
            log_type: "plain".to_string(),
            message: message.to_string(),
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

    async fn build_reliability_manager(endpoint: String, storage_path: std::path::PathBuf) -> ReliabilityManager {
        let client_config = ClientConfig {
            endpoint,
            timeout: Duration::from_millis(500),
            connection_timeout: Duration::from_millis(500),
            ..Default::default()
        };
        let log_sender = LogSender::new(client_config).await.unwrap();

        let retry_config = RetryConfig {
            max_attempts: 1,
            base_delay: Duration::from_millis(1),
            max_delay: Duration::from_millis(5),
            strategy: RetryStrategy::FixedDelay,
            jitter: false,
        };
        let disk_config = DiskConfig {
            storage_path,
            ..Default::default()
        };

        ReliabilityManager::new(
            retry_config,
            disk_config,
            MetricsConfig::default(),
            HealthConfig::default(),
            log_sender,
        )
        .await
        .unwrap()
    }

    #[tokio::test]
    async fn flush_batch_persists_to_disk_instead_of_dropping_on_failure() {
        // The aggregator is unreachable (every request fails), so a naive
        // single-attempt send would just log-and-drop the batch. This test
        // pins down that flush_batch instead routes through the reliability
        // manager, which retries and then durably persists to disk.
        let mock_server = MockServer::start().await;
        Mock::given(method("POST"))
            .respond_with(ResponseTemplate::new(500))
            .mount(&mock_server)
            .await;

        let storage_dir = tempfile::tempdir().unwrap();
        let reliability_manager = Arc::new(
            build_reliability_manager(mock_server.uri(), storage_dir.path().to_path_buf()).await,
        );
        let sender = Arc::new(
            LogSender::new(ClientConfig {
                endpoint: mock_server.uri(),
                ..Default::default()
            })
            .await
            .unwrap(),
        );

        let mut log_batch = vec![test_entry("unreachable aggregator")];

        flush_batch(
            &mut log_batch,
            &reliability_manager,
            &sender,
            &test_sender_config(),
        )
        .await;

        assert!(log_batch.is_empty(), "flush_batch should always drain the batch it was given");

        let snapshot = reliability_manager.get_metrics_snapshot().await;
        assert_eq!(
            snapshot.disk_fallback_count, 1,
            "flush_batch must fall back to disk (via ReliabilityManager) rather than silently \
             dropping the batch when the aggregator is unreachable"
        );

        let stored = storage_dir.path().read_dir().unwrap().count();
        assert_eq!(
            stored, 1,
            "the failed batch should actually be persisted to disk, not just counted"
        );
    }

    #[tokio::test]
    async fn flush_batch_reports_success_on_healthy_aggregator() {
        let mock_server = MockServer::start().await;
        Mock::given(method("POST"))
            .respond_with(ResponseTemplate::new(200))
            .mount(&mock_server)
            .await;

        let storage_dir = tempfile::tempdir().unwrap();
        let reliability_manager = Arc::new(
            build_reliability_manager(mock_server.uri(), storage_dir.path().to_path_buf()).await,
        );
        let sender = Arc::new(
            LogSender::new(ClientConfig {
                endpoint: mock_server.uri(),
                ..Default::default()
            })
            .await
            .unwrap(),
        );

        let mut log_batch = vec![test_entry("all good")];

        flush_batch(
            &mut log_batch,
            &reliability_manager,
            &sender,
            &test_sender_config(),
        )
        .await;

        let snapshot = reliability_manager.get_metrics_snapshot().await;
        assert_eq!(snapshot.successful_batches, 1);
        assert_eq!(snapshot.disk_fallback_count, 0);
    }
}
