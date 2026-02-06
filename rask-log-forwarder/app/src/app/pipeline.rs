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
use std::time::{Duration, Instant};
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
}

/// Run the main processing pipeline: collect → parse → batch → send.
pub async fn run_processing_loop(params: ProcessingLoopParams) {
    let ProcessingLoopParams {
        collector,
        parser,
        reliability_manager: _reliability_manager,
        sender,
        running,
        mut shutdown_rx,
        target_service,
        sender_config,
        batch_size,
        flush_interval,
    } = params;

    info!(
        "Starting main processing loop (batch_size={}, flush_interval={:?})",
        batch_size, flush_interval
    );

    // Create cancellation token for graceful shutdown of collector
    let cancel_token = CancellationToken::new();

    // Create log collection channel
    let (log_tx, mut log_rx) = tokio::sync::mpsc::unbounded_channel();

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
    let mut last_flush = Instant::now();

    // Main processing loop
    loop {
        tokio::select! {
            // Process incoming log entries
            Some(log_entry) = log_rx.recv() => {
                // Create container info for the parser
                let container_info = crate::collector::ContainerInfo {
                    id: log_entry.id.clone(),
                    service_name: target_service.clone(),
                    group: None,
                    labels: std::collections::HashMap::new(),
                };

                // Parse directly to EnrichedLogEntry - no intermediate conversion
                match parser.parse_docker_log(log_entry.raw_bytes.as_ref(), &container_info).await {
                    Ok(enriched_entry) => {
                        log_batch.push(enriched_entry);
                    }
                    Err(e) => {
                        error!("Failed to parse log entry: {}", e);
                        // Fallback: create a plain text EnrichedLogEntry directly
                        let fallback_entry = EnrichedLogEntry {
                            service_type: target_service.clone(),
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
                            service_name: target_service.clone(),
                            service_group: None,
                            trace_id: None,
                            span_id: None,
                            fields: std::collections::HashMap::new(),
                        };
                        log_batch.push(fallback_entry);
                    }
                }

                // Send batch when it reaches the configured size
                if log_batch.len() >= batch_size {
                    flush_batch(&mut log_batch, &sender, &sender_config).await;
                    last_flush = Instant::now();
                }
            }

            // Periodic flush for any remaining logs
            _ = tokio::time::sleep(flush_interval) => {
                if !log_batch.is_empty() && last_flush.elapsed() >= flush_interval {
                    flush_batch(&mut log_batch, &sender, &sender_config).await;
                    last_flush = Instant::now();
                }
            }

            // Handle shutdown signal
            _ = shutdown_rx.recv() => {
                info!("Received shutdown signal, stopping processing loop");
                // Cancel the collector to stop log streaming
                cancel_token.cancel();
                // Flush any remaining logs before shutting down
                if !log_batch.is_empty() {
                    flush_batch(&mut log_batch, &sender, &sender_config).await;
                }
                break;
            }
        }
    }

    *running.write().await = false;
    info!("Main processing loop stopped");
}

/// Flush the accumulated batch using the initialized LogSender (connection pooling).
async fn flush_batch(
    log_batch: &mut Vec<EnrichedLogEntry>,
    sender: &Arc<LogSender>,
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
            flush_otlp_batch(entries, sender, _sender_config).await;
            return;
        }
    }

    // Default: NDJSON protocol via LogSender (reuses connection pool)
    let batch = Batch::new(entries, BatchType::SizeBased);
    match sender.send_batch(batch).await {
        Ok(result) => {
            if result.success {
                info!(
                    "Successfully sent NDJSON batch of {} logs in {:?}",
                    entry_count, result.latency
                );
            } else {
                error!(
                    "Failed to send NDJSON logs, HTTP status: {}",
                    result.status_code
                );
            }
        }
        Err(e) => {
            error!("Failed to send NDJSON logs: {e}");
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
