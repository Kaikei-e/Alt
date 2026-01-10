pub mod discovery;
pub mod docker;

use thiserror::Error;
use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;
// Ensure zero-copy processing by using Bytes throughout
use bytes::Bytes;

pub type LogBytes = Bytes;
pub use discovery::{ContainerInfo, DiscoveryError, ServiceDiscovery, ServiceDiscoveryTrait};
pub use docker::{
    CollectorError as DockerError, ContainerInfo as DockerContainerInfo, DockerCollector,
    LogStreamOptions,
};

#[derive(Error, Debug)]
pub enum CollectorError {
    #[error("Discovery error: {0}")]
    DiscoveryError(#[from] discovery::DiscoveryError),
    #[error("Docker error: {0}")]
    DockerError(#[from] docker::CollectorError),
    #[error("Collection stopped")]
    CollectionStopped,
}

#[derive(Debug, Clone)]
pub struct LogEntry {
    pub log: String,
    pub stream: String,
    pub time: String,
    pub id: String,
    pub container_id: String,
    pub raw_bytes: Vec<u8>,
}

#[derive(Debug)]
pub struct CollectorConfig {
    pub auto_discover: bool,
    pub target_service: Option<String>,
    pub follow_rotations: bool,
    pub buffer_size: usize,
}

impl Default for CollectorConfig {
    fn default() -> Self {
        Self {
            auto_discover: true,
            target_service: None,
            follow_rotations: true,
            buffer_size: 8192,
        }
    }
}

#[derive(Debug)]
pub struct LogCollector {
    #[allow(dead_code)]
    config: CollectorConfig,
    discovery: discovery::ServiceDiscovery,
    container_info: Option<discovery::ContainerInfo>,
    target_service: String,
}

impl LogCollector {
    pub async fn new(config: CollectorConfig) -> Result<Self, CollectorError> {
        let discovery = discovery::ServiceDiscovery::new().await?;

        let target_service = if let Some(ref service) = config.target_service {
            service.clone()
        } else if config.auto_discover {
            discovery.get_target_service()?
        } else {
            return Err(CollectorError::DiscoveryError(
                discovery::DiscoveryError::NoTargetService,
            ));
        };

        Ok(Self {
            config,
            discovery,
            container_info: None,
            target_service,
        })
    }

    pub fn get_target_service(&self) -> &str {
        &self.target_service
    }

    pub async fn start_collection(
        &mut self,
        tx: mpsc::UnboundedSender<LogEntry>,
        cancel_token: CancellationToken,
    ) -> Result<(), CollectorError> {
        // Discover container
        let container_info = self
            .discovery
            .find_container_by_service(&self.target_service)
            .await?;

        tracing::info!(
            "Starting log collection for service '{}' (container: {})",
            self.target_service,
            container_info.id
        );

        // Use Docker API instead of file tailing
        let docker_collector = DockerCollector::new()
            .await
            .map_err(CollectorError::DockerError)?;

        self.container_info = Some(container_info.clone());

        // Start Docker API log streaming
        self.start_docker_api_streaming(docker_collector, &container_info.id, tx, cancel_token)
            .await
    }

    async fn start_docker_api_streaming(
        &self,
        _docker_collector: DockerCollector,
        container_id: &str,
        tx: mpsc::UnboundedSender<LogEntry>,
        cancel_token: CancellationToken,
    ) -> Result<(), CollectorError> {
        use bollard::Docker;
        use bollard::query_parameters::LogsOptions;
        use futures::StreamExt;

        // Create new Docker client since DockerCollector's docker field is private
        let docker = Docker::connect_with_unix_defaults().map_err(|e| {
            CollectorError::DiscoveryError(discovery::DiscoveryError::DockerError(e))
        })?;

        // IMPORTANT: Set timestamps to false to avoid Docker adding timestamps to log messages
        let options = LogsOptions {
            follow: true,
            stdout: true,
            stderr: true,
            timestamps: false, // Changed from true to false
            tail: "all".to_string(),
            ..Default::default()
        };

        let mut stream = docker.logs(container_id, Some(options));

        loop {
            tokio::select! {
                // Check for cancellation signal first
                _ = cancel_token.cancelled() => {
                    tracing::info!("Collector received cancellation signal, stopping log collection");
                    break;
                }
                // Process log stream
                log_output = stream.next() => {
                    match log_output {
                        Some(Ok(log_chunk)) => {
                            // Convert Docker log output to LogEntry
                            let log_bytes = log_chunk.into_bytes();

                            // Create LogEntry with raw bytes - let the parser handle the actual parsing
                            let entry = LogEntry {
                                log: String::new(),                    // Will be filled by the parser
                                stream: "stdout".to_string(), // Default, will be overridden by parser
                                time: chrono::Utc::now().to_rfc3339(), // Default timestamp
                                id: container_id.to_string(),
                                container_id: container_id.to_string(),
                                raw_bytes: log_bytes.to_vec(),
                            };

                            if tx.send(entry).is_err() {
                                return Err(CollectorError::CollectionStopped);
                            }
                        }
                        Some(Err(e)) => {
                            tracing::error!("Docker log stream error: {e}");
                            return Err(CollectorError::DiscoveryError(
                                discovery::DiscoveryError::DockerError(e),
                            ));
                        }
                        None => {
                            // Stream ended
                            break;
                        }
                    }
                }
            }
        }

        Ok(())
    }

    pub fn get_container_info(&self) -> Option<&discovery::ContainerInfo> {
        self.container_info.as_ref()
    }
}
