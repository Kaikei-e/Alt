use bollard::Docker;
use bollard::container::{ListContainersOptions, LogsOptions};
use bollard::models::ContainerSummary;
use futures::StreamExt;
use bytes::Bytes;
use multiqueue::BroadcastSender;
use tokio::time::Duration;
use std::collections::HashMap;
use thiserror::Error;

#[derive(Error, Debug)]
pub enum CollectorError {
    #[error("Docker connection failed: {0}")]
    ConnectionFailed(#[from] bollard::errors::Error),
    #[error("Container discovery failed: {0}")]
    DiscoveryFailed(String),
}

#[derive(Debug, Clone)]
pub struct ContainerInfo {
    pub id: String,
    pub name: String,
    pub image: String,
    pub labels: HashMap<String, String>,
}

#[derive(Debug, Clone)]
pub struct LogStreamOptions {
    pub follow: bool,
    pub stdout: bool,
    pub stderr: bool,
    pub timestamps: bool,
    pub tail: String,
}

impl Default for LogStreamOptions {
    fn default() -> Self {
        Self {
            follow: true,
            stdout: true,
            stderr: true,
            timestamps: true,
            tail: "0".to_string(), // All logs
        }
    }
}

pub struct DockerCollector {
    docker: Docker,
}

impl DockerCollector {
    pub async fn new() -> Result<Self, CollectorError> {
        let docker = Docker::connect_with_socket_defaults()?;
        Ok(Self { docker })
    }

    pub async fn new_with_socket(socket_path: &str) -> Result<Self, CollectorError> {
        let docker = Docker::connect_with_socket(socket_path, 120, bollard::API_DEFAULT_VERSION)?;
        Ok(Self { docker })
    }

    pub async fn can_connect(&self) -> bool {
        self.docker.ping().await.is_ok()
    }

    pub async fn find_labeled_containers(&self, label_filter: &str) -> Result<Vec<ContainerInfo>, CollectorError> {
        let filters = HashMap::from([
            ("label".to_string(), vec![label_filter.to_string()])
        ]);

        let options = Some(ListContainersOptions {
            all: false, // Only running containers
            filters,
            ..Default::default()
        });

        let containers = self.docker
            .list_containers(options)
            .await
            .map_err(|e| CollectorError::DiscoveryFailed(e.to_string()))?;

        let container_infos = containers
            .into_iter()
            .filter_map(|container| self.container_to_info(container))
            .collect();

        Ok(container_infos)
    }

    fn container_to_info(&self, container: ContainerSummary) -> Option<ContainerInfo> {
        Some(ContainerInfo {
            id: container.id?,
            name: container.names?.first()?.trim_start_matches('/').to_string(),
            image: container.image?,
            labels: container.labels.unwrap_or_default(),
        })
    }

    pub async fn start_tailing_logs(
        &self,
        tx: BroadcastSender<Bytes>,
        label_filter: &str,
    ) -> Result<(), CollectorError> {
        self.start_tailing_logs_with_options(tx, label_filter, LogStreamOptions::default()).await
    }

    pub async fn start_tailing_logs_with_options(
        &self,
        tx: BroadcastSender<Bytes>,
        label_filter: &str,
        options: LogStreamOptions,
    ) -> Result<(), CollectorError> {
        let containers = self.find_labeled_containers(label_filter).await?;

        if containers.is_empty() {
            tracing::warn!("No containers found with label: {}", label_filter);
            return Ok(());
        }

        let log_options = LogsOptions::<String> {
            follow: options.follow,
            stdout: options.stdout,
            stderr: options.stderr,
            timestamps: options.timestamps,
            tail: options.tail,
            ..Default::default()
        };

        for container in containers {
            let docker = self.docker.clone();
            let tx = tx.clone();
            let container_id = container.id.clone();
            let container_name = container.name.clone();
            let log_options = log_options.clone();

            tokio::spawn(async move {
                tracing::info!("Starting to tail logs for container: {} ({})", container_name, container_id);

                let mut stream = docker.logs(&container_id, Some(log_options));

                while let Some(chunk_result) = stream.next().await {
                    match chunk_result {
                        Ok(chunk) => {
                            // Zero-copy: chunk.into_bytes() returns Bytes directly
                            let bytes = chunk.into_bytes();

                            if tx.try_send(bytes).is_err() {
                                // Queue full, apply backpressure
                                tracing::warn!("Log queue full for container {}, applying backpressure", container_name);
                                tokio::time::sleep(Duration::from_micros(100)).await;
                            }
                        }
                        Err(e) => {
                            tracing::error!("Error reading logs from container {}: {}", container_name, e);
                            break;
                        }
                    }
                }

                tracing::info!("Stopped tailing logs for container: {}", container_name);
            });
        }

        Ok(())
    }
}