use super::Config;
use crate::parser::services::ServiceParser;
use crate::{
    buffer::{BatchConfig, BufferConfig, BufferManager, MemoryConfig},
    collector::{CollectorConfig, LogCollector},
    parser::UniversalParser,
    reliability::{HealthReport, ReliabilityManager},
    sender::{ClientConfig, LogSender},
};
use lazy_static::lazy_static;
use regex::Regex;
use reqwest;
use serde_json;
use std::sync::Arc;
use std::time::{Duration, Instant};
use thiserror::Error;
use tokio::signal;
use tokio::sync::{RwLock, mpsc};
use tracing::{debug, error, info, warn};

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
        let collector = Arc::new(tokio::sync::Mutex::new(self.collector.take().unwrap()));
        let _parser = self.parser.clone();
        let reliability_manager = Arc::new(self.reliability_manager.take().unwrap());
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
                (*self.sender.as_ref().unwrap()).clone(),
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
        _parser: Arc<UniversalParser>,
        reliability_manager: Arc<ReliabilityManager>,
        running: Arc<RwLock<bool>>,
        mut shutdown_rx: mpsc::UnboundedReceiver<()>,
        target_service: String,
    ) {
        info!("Starting main processing loop");

        // Create log collection channel
        let (log_tx, mut log_rx) = tokio::sync::mpsc::unbounded_channel();

        // Start log collection in background
        let _collection_running = running.clone();
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
                    debug!("Received log entry from container");

                    // Step 1: Remove ANSI escape sequences for easier parsing
                    let cleaned_log = strip_ansi_codes(&log_entry.log);

                    // Try parsing strategies in order of confidence
                    let mut parsed_opt: Option<crate::parser::services::ParsedLogEntry> = None;

                    // 1) JSON structured (common in Go, Python structured logging)
                    if cleaned_log.trim_start().starts_with('{') {
                        if let Ok(mut json_parsed) = crate::parser::services::GoStructuredParser::new().parse_log(&cleaned_log) {
                            json_parsed.service_type = target_service.clone();
                            parsed_opt = Some(json_parsed);
                        }
                    }

                    // 2) GIN access logs ("[GIN] yyyy/mm/dd - hh:mm:ss | 200 | ... | ip | METHOD  \"/path\"")
                    if parsed_opt.is_none() && cleaned_log.contains("[GIN]") {
                        let parts: Vec<&str> = cleaned_log.split('|').collect();
                        if parts.len() >= 5 {
                            let status_code_part = parts[1].trim();
                            let ip_part = parts[3].trim();
                            let method_path_part = parts[4].trim();

                            let status_code = status_code_part.parse::<u16>().ok();

                            // Split method and path
                            let mut method: Option<String> = None;
                            let mut path: Option<String> = None;
                            let tokens: Vec<&str> = method_path_part.split_whitespace().collect();
                            if tokens.len() >= 2 {
                                method = Some(tokens[0].to_string());
                                let p = tokens[1].trim_matches('"');
                                path = Some(p.to_string());
                            }

                            parsed_opt = Some(crate::parser::services::ParsedLogEntry {
                                service_type: target_service.clone(),
                                log_type: "gin".to_string(),
                                message: cleaned_log.clone(),
                                level: Some(crate::parser::services::LogLevel::Info),
                                timestamp: None,
                                stream: log_entry.stream.clone(),
                                method,
                                path,
                                status_code,
                                response_size: None,
                                ip_address: if ip_part.is_empty() { None } else { Some(ip_part.to_string()) },
                                user_agent: None,
                                fields: std::collections::HashMap::new(),
                            });
                        }
                    }

                    // 3) key=value scanning fallback (existing logic)
                    if parsed_opt.is_none() {
                        // Try to parse key=value structured log parts (common in Go's slog)
                        let mut fields: std::collections::HashMap<String, String> = std::collections::HashMap::new();
                        let mut message = cleaned_log.clone();
                        let mut level = crate::parser::services::LogLevel::Info;
                        let mut method: Option<String> = None;
                        let mut path: Option<String> = None;
                        let mut status_code: Option<u16> = None;
                        let mut ip_address: Option<String> = None;
                        let mut user_agent: Option<String> = None;

                        for part in cleaned_log.split_whitespace() {
                            if let Some(idx) = part.find('=') {
                                let key = &part[..idx];
                                let value = part[idx + 1..].trim_matches('"').to_string();

                                match key {
                                    "msg" | "message" => message = value.clone(),
                                    "level" => {
                                        level = match value.to_lowercase().as_str() {
                                            "debug" => crate::parser::services::LogLevel::Debug,
                                            "warn" | "warning" => crate::parser::services::LogLevel::Warn,
                                            "error" => crate::parser::services::LogLevel::Error,
                                            "fatal" | "panic" => crate::parser::services::LogLevel::Fatal,
                                            _ => crate::parser::services::LogLevel::Info,
                                        }
                                    },
                                    "method" => method = Some(value.clone()),
                                    "path" => path = Some(value.clone()),
                                    "status" | "status_code" => {
                                        if let Ok(code) = value.parse::<u16>() {
                                            status_code = Some(code);
                                        }
                                    },
                                    "remote_addr" | "ip" => ip_address = Some(value.clone()),
                                    "user_agent" | "agent" => user_agent = Some(value.clone()),
                                    _ => {
                                        fields.insert(key.to_string(), value);
                                    }
                                }
                            }
                        }

                        parsed_opt = Some(crate::parser::services::ParsedLogEntry {
                            service_type: target_service.clone(),
                            log_type: "plain".to_string(),
                            message,
                            level: Some(level),
                            timestamp: None,
                            stream: log_entry.stream.clone(),
                            method,
                            path,
                            status_code,
                            response_size: None,
                            ip_address,
                            user_agent,
                            fields,
                        });
                    }

                    // At this point parsed_opt is guaranteed to be Some
                    let mut parsed_entry = parsed_opt.unwrap();

                    // Parse timestamp from Docker metadata first, fallback to now
                    let timestamp = match chrono::DateTime::parse_from_rfc3339(&log_entry.time) {
                        Ok(dt) => Some(dt.with_timezone(&chrono::Utc)),
                        Err(_) => Some(chrono::Utc::now()),
                    };
                    parsed_entry.timestamp = timestamp;

                    debug!("Successfully created log entry");
                    log_batch.push(parsed_entry);

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
                timestamp: entry.timestamp.map(|dt| dt.to_rfc3339()).unwrap_or_else(|| chrono::Utc::now().to_rfc3339()),
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
                    error!("Failed to serialize log entry: {}", e);
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
                error!("Failed to send logs: {}", e);
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
fn strip_ansi_codes(input: &str) -> String {
    lazy_static! {
        static ref ANSI_RE: Regex = Regex::new(r"\x1B\[[0-9;?]*[ -/]*[@-~]").unwrap();
    }
    ANSI_RE.replace_all(input, "").to_string()
}
