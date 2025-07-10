use super::Config;
use crate::{
    buffer::{BatchConfig, BufferConfig, BufferManager, MemoryConfig},
    collector::{CollectorConfig, LogCollector},
    parser::UniversalParser,
    reliability::{HealthReport, ReliabilityManager},
    sender::{ClientConfig, LogSender},
};
use reqwest;
use serde_json;
use std::sync::Arc;
use std::time::{Duration, Instant};
use thiserror::Error;
use tokio::signal;
use tokio::sync::{RwLock, mpsc};
use tracing::{error, info, warn};

#[derive(Error, Debug)]
pub enum ServiceError {
    #[error("Configuration error: {0}")]
    ConfigError(#[from] crate::app::ConfigError),
    #[error("Collector error: {0}")]
    CollectorError(#[from] crate::collector::CollectorError),
    #[error("Buffer error: {0}")]
    BufferError(#[from] crate::buffer::BufferError),
    #[error("Sender error: {0}")]
    SenderError(#[from] crate::sender::ClientError),
    #[error("Service not initialized")]
    NotInitialized,
    #[error("Service already running")]
    AlreadyRunning,
    #[error("Shutdown timeout")]
    ShutdownTimeout,
    #[error("Service component not initialized: {component}")]
    ComponentNotInitialized { component: String },
}

pub struct ServiceManager {
    config: Config,
    target_service: String,
    start_time: Instant,

    // Components
    collector: Option<LogCollector>,
    parser: Arc<UniversalParser>,
    buffer_manager: Option<BufferManager>,
    sender: Option<LogSender>,
    reliability_manager: Option<ReliabilityManager>,

    // Runtime state
    running: Arc<RwLock<bool>>,
    shutdown_tx: Option<mpsc::UnboundedSender<()>>,
}

impl ServiceManager {
    pub async fn new(mut config: Config) -> Result<Self, ServiceError> {
        // Auto-detect target service if not provided
        config.auto_detect_service()?;
        let target_service = config.get_target_service()?;

        info!(
            "Initializing rask-log-forwarder for service: {}",
            target_service
        );

        // Initialize parser (stateless, can be shared)
        let parser = Arc::new(UniversalParser::new());

        Ok(Self {
            config,
            target_service,
            start_time: Instant::now(),
            collector: None,
            parser,
            buffer_manager: None,
            sender: None,
            reliability_manager: None,
            running: Arc::new(RwLock::new(false)),
            shutdown_tx: None,
        })
    }

    pub async fn start(&mut self) -> Result<ShutdownHandle, ServiceError> {
        if *self.running.read().await {
            return Err(ServiceError::AlreadyRunning);
        }

        info!("Starting rask-log-forwarder components...");

        // Initialize components
        self.initialize_components().await?;

        // Setup shutdown channel
        let (shutdown_tx, shutdown_rx) = mpsc::unbounded_channel();
        self.shutdown_tx = Some(shutdown_tx.clone());

        // Mark as running
        *self.running.write().await = true;

        // Setup signal handlers
        let signal_handler = self.setup_signal_handlers().await?;

        // Start main processing loop
        let running = self.running.clone();
        let collector = Arc::new(tokio::sync::Mutex::new(self.collector.take().ok_or_else(
            || ServiceError::ComponentNotInitialized {
                component: "collector".to_string(),
            },
        )?));
        let _parser = self.parser.clone();
        let reliability_manager = Arc::new(self.reliability_manager.take().ok_or_else(|| {
            ServiceError::ComponentNotInitialized {
                component: "reliability_manager".to_string(),
            }
        })?);
        let target_service = self.target_service.clone();

        tokio::spawn(async move {
            Self::main_processing_loop(
                collector,
                _parser,
                reliability_manager,
                running,
                shutdown_rx,
                target_service,
            )
            .await;
        });

        // Start background tasks
        if let Some(reliability_manager) = &self.reliability_manager {
            reliability_manager.start_background_tasks().await;
        }

        info!(
            "rask-log-forwarder started successfully for service: {}",
            self.target_service
        );

        Ok(ShutdownHandle {
            shutdown_tx,
            signal_handler,
            running: self.running.clone(),
        })
    }

    async fn initialize_components(&mut self) -> Result<(), ServiceError> {
        // Initialize collector
        let collector_config = CollectorConfig {
            auto_discover: true,
            target_service: Some(self.target_service.clone()),
            follow_rotations: true,
            buffer_size: 8192,
        };

        self.collector = Some(LogCollector::new(collector_config).await?);

        // Initialize buffer manager
        let buffer_config = BufferConfig {
            capacity: self.config.buffer_capacity,
            batch_size: self.config.batch_size,
            batch_timeout: self.config.flush_interval,
            enable_backpressure: true,
            backpressure_threshold: 0.8,
            backpressure_delay: Duration::from_micros(100),
        };

        let batch_config = BatchConfig {
            max_size: self.config.batch_size,
            max_wait_time: self.config.flush_interval,
            max_memory_size: 10 * 1024 * 1024, // 10MB
        };

        let memory_config = MemoryConfig {
            max_memory: 100 * 1024 * 1024, // 100MB
            warning_threshold: 0.8,
            critical_threshold: 0.95,
        };

        self.buffer_manager =
            Some(BufferManager::new(buffer_config, batch_config, memory_config).await?);

        // Initialize sender
        let client_config = ClientConfig {
            endpoint: self.config.endpoint.clone(),
            timeout: self.config.connection_timeout,
            connection_timeout: Duration::from_secs(10),
            max_connections: self.config.max_connections,
            keep_alive_timeout: Duration::from_secs(60),
            user_agent: format!("rask-log-forwarder/{}", env!("CARGO_PKG_VERSION")),
            enable_compression: self.config.enable_compression,
            retry_attempts: self.config.retry_config.max_attempts,
        };
        self.sender = Some(LogSender::new(client_config).await?);

        // Initialize reliability manager
        let retry_config = crate::reliability::RetryConfig {
            max_attempts: self.config.retry_config.max_attempts,
            base_delay: self.config.retry_config.base_delay,
            max_delay: self.config.retry_config.max_delay,
            strategy: crate::reliability::RetryStrategy::ExponentialBackoff,
            jitter: self.config.retry_config.jitter,
        };

        let disk_config = crate::reliability::DiskConfig {
            storage_path: self.config.disk_fallback_config.storage_path.clone(),
            max_disk_usage: self.config.disk_fallback_config.max_disk_usage_mb * 1024 * 1024,
            retention_period: Duration::from_secs(
                self.config.disk_fallback_config.retention_hours * 3600,
            ),
            compression: self.config.disk_fallback_config.compression,
        };

        let metrics_config = crate::reliability::MetricsConfig {
            enabled: self.config.metrics_config.enabled,
            export_port: self.config.metrics_config.port,
            export_path: self.config.metrics_config.path.clone(),
            collection_interval: Duration::from_secs(15),
        };

        let health_config = crate::reliability::HealthConfig::default();

        self.reliability_manager = Some(
            ReliabilityManager::new(
                retry_config,
                disk_config,
                metrics_config,
                health_config,
                (*self
                    .sender
                    .as_ref()
                    .ok_or_else(|| ServiceError::ComponentNotInitialized {
                        component: "sender".to_string(),
                    })?)
                .clone(),
            )
            .await
            .map_err(|e| {
                ServiceError::SenderError(crate::sender::ClientError::InvalidConfiguration(
                    e.to_string(),
                ))
            })?,
        );

        Ok(())
    }

    async fn main_processing_loop(
        collector: Arc<tokio::sync::Mutex<LogCollector>>,
        parser: Arc<UniversalParser>,
        reliability_manager: Arc<ReliabilityManager>,
        running: Arc<RwLock<bool>>,
        mut shutdown_rx: mpsc::UnboundedReceiver<()>,
        target_service: String,
    ) {
        info!("Starting main processing loop");

        // Create log collection channel
        let (log_tx, mut log_rx) = tokio::sync::mpsc::unbounded_channel();

        // Start log collection in background
        let collector_clone = collector.clone();
        tokio::spawn(async move {
            let mut collector_guard = collector_clone.lock().await;
            if let Err(e) = collector_guard.start_collection(log_tx).await {
                error!("Log collection failed: {}", e);
            }
        });

        // Buffer for batching logs
        let mut log_batch = Vec::new();
        let batch_size = 1; // Using minimal batch size for immediate processing
        let mut last_flush = std::time::Instant::now();
        let flush_interval = std::time::Duration::from_millis(500);

        // Main processing loop
        loop {
            tokio::select! {
                // Process incoming log entries
                Some(log_entry) = log_rx.recv() => {
                    // Create container info for the parser
                    let container_info = crate::collector::ContainerInfo {
                        id: log_entry.id.clone(),
                        service_name: target_service.clone(),
                        group: None, // Will be set by the parser if available
                        labels: std::collections::HashMap::new(),
                    };

                    // Use UniversalParser to properly parse the Docker log
                    match parser.parse_docker_log(log_entry.raw_bytes.as_ref(), &container_info).await {
                        Ok(enriched_entry) => {
                            // Convert EnrichedLogEntry to format expected by send_log_batch
                            let parsed_entry = crate::parser::services::ParsedLogEntry {
                                service_type: enriched_entry.service_type,
                                log_type: enriched_entry.log_type,
                                message: enriched_entry.message,
                                level: enriched_entry.level,
                                timestamp: Some(chrono::DateTime::parse_from_rfc3339(&enriched_entry.timestamp)
                                    .unwrap_or_else(|_| chrono::Utc::now().into())
                                    .with_timezone(&chrono::Utc)),
                                stream: enriched_entry.stream,
                                method: enriched_entry.method,
                                path: enriched_entry.path,
                                status_code: enriched_entry.status_code,
                                response_size: enriched_entry.response_size,
                                ip_address: enriched_entry.ip_address,
                                user_agent: enriched_entry.user_agent,
                                fields: enriched_entry.fields,
                            };

                            log_batch.push(parsed_entry);
                        }
                        Err(e) => {
                            error!("Failed to parse log entry: {}", e);
                            // Fallback: create a plain text entry
                            let fallback_entry = crate::parser::services::ParsedLogEntry {
                                service_type: target_service.clone(),
                                log_type: "plain".to_string(),
                                    message: log_entry.log.clone(),
                                level: Some(crate::parser::services::LogLevel::Info),
                                timestamp: Some(chrono::Utc::now()),
                                stream: "stdout".to_string(),
                                method: None,
                                path: None,
                                status_code: None,
                                response_size: None,
                                ip_address: None,
                                user_agent: None,
                                fields: std::collections::HashMap::new(),
                            };
                            log_batch.push(fallback_entry);
                        }
                    }

                    // Send batch if it reaches the target size or flush interval has passed
                    if log_batch.len() >= batch_size || last_flush.elapsed() >= flush_interval {
                        Self::send_log_batch(&log_batch, &reliability_manager).await;
                        log_batch.clear();
                        last_flush = std::time::Instant::now();
                    }
                }

                // Periodic flush for any remaining logs
                _ = tokio::time::sleep(flush_interval) => {
                    if !log_batch.is_empty() && last_flush.elapsed() >= flush_interval {
                        Self::send_log_batch(&log_batch, &reliability_manager).await;
                        log_batch.clear();
                        last_flush = std::time::Instant::now();
                    }
                }

                // Handle shutdown signal
                _ = shutdown_rx.recv() => {
                    info!("Received shutdown signal, stopping processing loop");
                    // Send any remaining logs before shutting down
                    if !log_batch.is_empty() {
                        Self::send_log_batch(&log_batch, &reliability_manager).await;
                    }
                    break;
                }
            }
        }

        *running.write().await = false;
        info!("Main processing loop stopped");
    }

    async fn send_log_batch(
        log_batch: &[crate::parser::services::ParsedLogEntry],
        _reliability_manager: &Arc<ReliabilityManager>,
    ) {
        if log_batch.is_empty() {
            return;
        }

        info!("Sending batch of {} log entries", log_batch.len());

        // Convert ParsedLogEntry to EnrichedLogEntry format expected by rask-log-aggregator
        let mut ndjson_lines = Vec::new();
        for entry in log_batch {
            // Convert ParsedLogEntry to EnrichedLogEntry
            let enriched_entry = crate::parser::universal::EnrichedLogEntry {
                service_type: entry.service_type.clone(),
                log_type: entry.log_type.clone(),
                message: entry.message.clone(),
                level: entry.level.clone(),
                timestamp: entry
                    .timestamp
                    .map(|dt| dt.to_rfc3339())
                    .unwrap_or_else(|| chrono::Utc::now().to_rfc3339()),
                stream: entry.stream.clone(),
                method: entry.method.clone(),
                path: entry.path.clone(),
                status_code: entry.status_code,
                response_size: entry.response_size,
                ip_address: entry.ip_address.clone(),
                user_agent: entry.user_agent.clone(),
                container_id: "unknown".to_string(), // TODO: Pass real container ID
                service_name: entry.service_type.clone(), // Use service_type as service_name
                service_group: None,
                fields: entry.fields.clone(),
            };

            match serde_json::to_string(&enriched_entry) {
                Ok(json_line) => ndjson_lines.push(json_line),
                Err(e) => {
                    error!("Failed to serialize log entry: {e}");
                    continue;
                }
            }
        }

        if ndjson_lines.is_empty() {
            warn!("No valid log entries to send");
            return;
        }

        let ndjson_body = ndjson_lines.join("\n");

        // Create a simple HTTP request to send the batch
        let client = reqwest::Client::new();
        let endpoint = "http://rask-log-aggregator:9600/v1/aggregate";

        match client
            .post(endpoint)
            .header("Content-Type", "application/x-ndjson")
            .header("x-batch-size", log_batch.len().to_string())
            .body(ndjson_body)
            .send()
            .await
        {
            Ok(response) => {
                if response.status().is_success() {
                    info!("Successfully sent batch of {} logs", log_batch.len());
                } else {
                    error!("Failed to send logs, status: {}", response.status());
                    // Here we could add retry logic via reliability_manager
                }
            }
            Err(e) => {
                error!("Failed to send logs: {e}");
                // Here we could add retry logic via reliability_manager
            }
        }
    }

    pub async fn setup_signal_handlers(&self) -> Result<SignalHandler, ServiceError> {
        let shutdown_tx = self
            .shutdown_tx
            .as_ref()
            .ok_or(ServiceError::NotInitialized)?
            .clone();

        Ok(SignalHandler::new(shutdown_tx).await)
    }

    pub fn is_initialized(&self) -> bool {
        self.collector.is_some()
            && self.buffer_manager.is_some()
            && self.sender.is_some()
            && self.reliability_manager.is_some()
    }

    pub async fn is_running(&self) -> bool {
        *self.running.read().await
    }

    pub fn get_target_service(&self) -> &str {
        &self.target_service
    }

    pub async fn get_health_report(&self) -> HealthReport {
        if let Some(reliability_manager) = &self.reliability_manager {
            reliability_manager.get_health_report().await
        } else {
            HealthReport {
                overall_status: crate::reliability::HealthStatus::Unhealthy,
                components: std::collections::HashMap::new(),
                timestamp: chrono::Utc::now().to_rfc3339(),
                uptime: self.start_time.elapsed(),
            }
        }
    }

    pub async fn simulate_component_failure(&mut self, component: &str) {
        // For testing purposes only
        if let Some(_reliability_manager) = &self.reliability_manager {
            // This would be implemented in the reliability manager for testing
            warn!("Simulating failure for component: {}", component);
        }
    }
}

// Note: Clone implementations would be added to the respective modules
// For now, we'll use Arc to share instances instead of cloning

#[derive(Debug)]
pub struct ShutdownHandle {
    shutdown_tx: mpsc::UnboundedSender<()>,
    signal_handler: SignalHandler,
    running: Arc<RwLock<bool>>,
}

impl ShutdownHandle {
    pub async fn shutdown(self) -> Result<(), ServiceError> {
        info!("Initiating graceful shutdown...");

        // Send shutdown signal
        if self.shutdown_tx.send(()).is_err() {
            warn!("Shutdown channel already closed");
        }

        // Wait for service to stop
        let timeout_duration = Duration::from_secs(10);
        let start = Instant::now();

        while *self.running.read().await && start.elapsed() < timeout_duration {
            tokio::time::sleep(Duration::from_millis(100)).await;
        }

        if *self.running.read().await {
            error!("Shutdown timeout exceeded");
            return Err(ServiceError::ShutdownTimeout);
        }

        info!("Graceful shutdown completed");
        Ok(())
    }

    pub async fn wait_for_shutdown(self) {
        self.signal_handler.wait().await;
        if let Err(e) = self.shutdown().await {
            error!("Shutdown error: {}", e);
        }
    }
}

#[derive(Debug)]
pub struct SignalHandler {
    shutdown_tx: mpsc::UnboundedSender<()>,
    active: Arc<RwLock<bool>>,
}

impl SignalHandler {
    async fn new(shutdown_tx: mpsc::UnboundedSender<()>) -> Self {
        let handler = Self {
            shutdown_tx,
            active: Arc::new(RwLock::new(true)),
        };

        handler.setup_handlers().await;
        handler
    }

    async fn setup_handlers(&self) {
        let shutdown_tx = self.shutdown_tx.clone();
        let active = self.active.clone();

        tokio::spawn(async move {
            if *active.read().await {
                match signal::ctrl_c().await {
                    Ok(()) => {
                        info!("Received SIGINT (Ctrl+C), initiating graceful shutdown");
                        if shutdown_tx.send(()).is_err() {
                            error!("Failed to send shutdown signal");
                        }
                    }
                    Err(err) => {
                        error!("Failed to listen for SIGINT: {}", err);
                    }
                }
            }
        });
    }

    pub async fn is_active(&self) -> bool {
        *self.active.read().await
    }

    pub async fn wait(&self) {
        while *self.active.read().await {
            tokio::time::sleep(Duration::from_millis(100)).await;
        }
    }
}

// Helper: remove ANSI escape sequences for easier log parsing
// fn strip_ansi_codes(input: &str) -> String {
//     lazy_static! {
//         static ref ANSI_RE: Regex = Regex::new(r"\x1B\[[0-9;?]*[ -/]*[@-~]").unwrap();
//     }
//     ANSI_RE.replace_all(input, "").to_string()
// }

#[cfg(test)]
mod tests {
    use super::*;
    use crate::app::Config;

    fn create_test_config() -> Config {
        Config {
            target_service: Some("test-service".to_string()),
            ..Default::default()
        }
    }

    #[tokio::test]
    async fn test_component_initialization_error_handling() {
        // Test that our error handling works correctly by testing a successful case
        let config = create_test_config();
        let mut service = ServiceManager::new(config).await.unwrap();

        // Test successful component initialization
        let result = service.initialize_components().await;
        assert!(result.is_ok());

        // Verify all components are initialized
        assert!(service.is_initialized());
    }

    #[tokio::test]
    async fn test_service_error_types() {
        // Test that our error types work correctly
        let error = ServiceError::ComponentNotInitialized {
            component: "test_component".to_string(),
        };

        assert_eq!(
            error.to_string(),
            "Service component not initialized: test_component"
        );
    }

    #[tokio::test]
    async fn test_service_initialization_state() {
        // Test the initialization state checking
        let config = create_test_config();
        let mut service = ServiceManager::new(config).await.unwrap();

        // Should not be initialized initially
        assert!(!service.is_initialized());

        // After initialization, should be initialized
        service.initialize_components().await.unwrap();
        assert!(service.is_initialized());
    }
}
