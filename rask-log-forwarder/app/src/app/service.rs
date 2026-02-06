use super::Config;
use super::pipeline::{ProcessingLoopParams, run_processing_loop};
use super::protocol::SenderConfig;
use super::shutdown::{ShutdownHandle, SignalHandler};
use crate::{
    buffer::{BatchConfig, BufferConfig, BufferManager, MemoryConfig},
    collector::{CollectorConfig, LogCollector},
    parser::UniversalParser,
    reliability::{HealthReport, ReliabilityManager},
    sender::{ClientConfig, LogSender},
};
use std::sync::Arc;
use std::time::{Duration, Instant};
use thiserror::Error;
use tokio::sync::{RwLock, mpsc};
use tracing::{info, warn};

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

    // Sender configuration (for protocol-aware sending)
    sender_config: SenderConfig,

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

        // Create sender configuration for protocol-aware sending
        let sender_config = SenderConfig {
            #[cfg(feature = "otlp")]
            protocol: config.protocol,
            #[cfg(feature = "otlp")]
            otlp_endpoint: config.otlp_endpoint.clone(),
        };

        #[cfg(feature = "otlp")]
        info!(
            "Protocol: {:?}, Endpoint: {}, OTLP Endpoint: {}",
            sender_config.protocol, config.endpoint, sender_config.otlp_endpoint
        );

        Ok(Self {
            config,
            target_service,
            start_time: Instant::now(),
            collector: None,
            parser,
            buffer_manager: None,
            sender: None,
            reliability_manager: None,
            sender_config,
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
        let parser = self.parser.clone();
        let reliability_manager = Arc::new(self.reliability_manager.take().ok_or_else(|| {
            ServiceError::ComponentNotInitialized {
                component: "reliability_manager".to_string(),
            }
        })?);
        let sender = Arc::new(self.sender.take().ok_or_else(|| {
            ServiceError::ComponentNotInitialized {
                component: "sender".to_string(),
            }
        })?);

        let params = ProcessingLoopParams {
            collector,
            parser,
            reliability_manager,
            sender,
            running,
            shutdown_rx,
            target_service: self.target_service.clone(),
            sender_config: self.sender_config.clone(),
            batch_size: self.config.batch_size,
            flush_interval: self.config.flush_interval,
        };

        tokio::spawn(async move {
            run_processing_loop(params).await;
        });

        info!(
            "rask-log-forwarder started successfully for service: {}",
            self.target_service
        );

        Ok(ShutdownHandle::new(
            shutdown_tx,
            signal_handler,
            self.running.clone(),
        ))
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
        if let Some(_reliability_manager) = &self.reliability_manager {
            warn!("Simulating failure for component: {}", component);
        }
    }
}

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
        let config = create_test_config();
        let service = ServiceManager::new(config).await.unwrap();

        assert!(!service.is_initialized());

        let error = ServiceError::ComponentNotInitialized {
            component: "test_component".to_string(),
        };

        assert_eq!(
            error.to_string(),
            "Service component not initialized: test_component"
        );
    }

    #[tokio::test]
    async fn test_service_error_types() {
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
        let config = create_test_config();
        let service = ServiceManager::new(config).await.unwrap();

        assert!(!service.is_initialized());
        assert!(!service.is_running().await);
        assert_eq!(service.get_target_service(), "test-service");

        let health_report = service.get_health_report().await;
        assert_eq!(
            health_report.overall_status,
            crate::reliability::HealthStatus::Unhealthy
        );
    }
}
