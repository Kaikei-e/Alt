use super::Config;
use crate::{
    collector::{LogCollector, CollectorConfig},
    parser::UniversalParser,
    buffer::{BufferManager, BufferConfig, BatchConfig, MemoryConfig},
    sender::{LogSender, ClientConfig},
    reliability::{ReliabilityManager, HealthReport},
};
use std::sync::Arc;
use std::time::{Duration, Instant};
use thiserror::Error;
use tokio::signal;
use tokio::sync::{mpsc, RwLock};
use tracing::{info, warn, error};

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

        info!("Initializing rask-log-forwarder for service: {}", target_service);

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
        let parser = self.parser.clone();
        let reliability_manager = Arc::new(self.reliability_manager.take().unwrap());

        tokio::spawn(async move {
            Self::main_processing_loop(
                collector,
                parser,
                reliability_manager,
                running,
                shutdown_rx,
            ).await;
        });

        // Start background tasks
        if let Some(reliability_manager) = &self.reliability_manager {
            reliability_manager.start_background_tasks().await;
        }

        info!("rask-log-forwarder started successfully for service: {}", self.target_service);

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

        self.buffer_manager = Some(BufferManager::new(
            buffer_config,
            batch_config,
            memory_config,
        ).await?);

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
                self.config.disk_fallback_config.retention_hours * 3600
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

        self.reliability_manager = Some(ReliabilityManager::new(
            retry_config,
            disk_config,
            metrics_config,
            health_config,
            (*self.sender.as_ref().unwrap()).clone(),
        ).await.map_err(|e| ServiceError::SenderError(
            crate::sender::ClientError::InvalidConfiguration(e.to_string())
        ))?);

        Ok(())
    }

    async fn main_processing_loop(
        collector: Arc<tokio::sync::Mutex<LogCollector>>,
        _parser: Arc<UniversalParser>,
        _reliability_manager: Arc<ReliabilityManager>,
        running: Arc<RwLock<bool>>,
        mut shutdown_rx: mpsc::UnboundedReceiver<()>,
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

        // Main processing loop
        loop {
            tokio::select! {
                // Process incoming log entries
                Some(log_entry) = log_rx.recv() => {
                    // TODO: Add parsing and batching logic here
                    // For now, just log that we received an entry
                    tracing::trace!("Received log entry: {:?}", log_entry);
                }

                // Handle shutdown signal
                _ = shutdown_rx.recv() => {
                    info!("Received shutdown signal, stopping processing loop");
                    break;
                }
            }
        }

        *running.write().await = false;
        info!("Main processing loop stopped");
    }

    pub async fn setup_signal_handlers(&self) -> Result<SignalHandler, ServiceError> {
        let shutdown_tx = self.shutdown_tx.as_ref()
            .ok_or(ServiceError::NotInitialized)?
            .clone();

        Ok(SignalHandler::new(shutdown_tx).await)
    }

    pub fn is_initialized(&self) -> bool {
        self.collector.is_some() &&
        self.buffer_manager.is_some() &&
        self.sender.is_some() &&
        self.reliability_manager.is_some()
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
        if let Err(_) = self.shutdown_tx.send(()) {
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
            loop {
                if !*active.read().await {
                    break;
                }

                match signal::ctrl_c().await {
                    Ok(()) => {
                        info!("Received SIGINT (Ctrl+C), initiating graceful shutdown");
                        if let Err(_) = shutdown_tx.send(()) {
                            error!("Failed to send shutdown signal");
                        }
                        break;
                    }
                    Err(err) => {
                        error!("Failed to listen for SIGINT: {}", err);
                        break;
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