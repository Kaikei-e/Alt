use bollard::{Docker, container::ListContainersOptions};
use std::collections::HashMap;
use std::env;
use thiserror::Error;

#[derive(Error, Debug)]
pub enum DiscoveryError {
    #[error("Container not found for service: {0}")]
    ContainerNotFound(String),
    #[error("No target service specified")]
    NoTargetService,
    #[error("Docker API error: {0}")]
    DockerError(#[from] bollard::errors::Error),
    #[error("Invalid hostname format: {0}")]
    InvalidHostname(String),
}

impl Clone for DiscoveryError {
    fn clone(&self) -> Self {
        match self {
            DiscoveryError::ContainerNotFound(s) => DiscoveryError::ContainerNotFound(s.clone()),
            DiscoveryError::NoTargetService => DiscoveryError::NoTargetService,
            DiscoveryError::DockerError(e) => DiscoveryError::DockerError(
                bollard::errors::Error::DockerResponseServerError {
                    status_code: 500,
                    message: format!("Cloned error: {}", e),
                }
            ),
            DiscoveryError::InvalidHostname(s) => DiscoveryError::InvalidHostname(s.clone()),
        }
    }
}

#[derive(Debug, Clone)]
pub struct ContainerInfo {
    pub id: String,
    pub service_name: String,
    pub labels: HashMap<String, String>,
    pub group: Option<String>,
}

#[async_trait::async_trait]
pub trait ServiceDiscoveryTrait {
    async fn find_container_by_service(&self, service_name: &str) -> Result<ContainerInfo, DiscoveryError>;
    fn get_target_service(&self) -> Result<String, DiscoveryError>;
    fn detect_target_service_from_hostname(&self, hostname: &str) -> Result<String, DiscoveryError>;
}

pub struct ServiceDiscovery {
    docker: Docker,
}

impl ServiceDiscovery {
    pub async fn new() -> Result<Self, DiscoveryError> {
        let docker = Docker::connect_with_unix_defaults()?;
        Ok(Self { docker })
    }
}

#[async_trait::async_trait]
impl ServiceDiscoveryTrait for ServiceDiscovery {
    fn get_target_service(&self) -> Result<String, DiscoveryError> {
        // Check environment variable first
        if let Ok(service) = env::var("TARGET_SERVICE") {
            return Ok(service);
        }

        // Auto-detect from hostname
        let hostname = hostname::get()
            .ok()
            .and_then(|h| h.to_str().map(|s| s.to_string()))
            .ok_or(DiscoveryError::NoTargetService)?;

        self.detect_target_service_from_hostname(&hostname)
    }

    fn detect_target_service_from_hostname(&self, hostname: &str) -> Result<String, DiscoveryError> {
        // Pattern: "service-logs" → "service"
        if hostname.ends_with("-logs") {
            let service_name = hostname.trim_end_matches("-logs");
            if service_name.is_empty() {
                return Err(DiscoveryError::InvalidHostname(hostname.to_string()));
            }
            Ok(service_name.to_string())
        } else {
            Err(DiscoveryError::InvalidHostname(format!(
                "Hostname '{}' doesn't match pattern '*-logs'", hostname
            )))
        }
    }

    async fn find_container_by_service(&self, service_name: &str) -> Result<ContainerInfo, DiscoveryError> {
        let options = ListContainersOptions::<String> {
            all: false, // Only running containers
            ..Default::default()
        };

        let containers = self.docker.list_containers(Some(options)).await?;

        // Find container with matching service name
        for container in containers {
            if let Some(names) = &container.names {
                // Container names start with '/' so we need to handle that
                let container_name = names.iter()
                    .find(|name| {
                        let clean_name = name.trim_start_matches('/');

                        // Match exact service name
                        let exact = clean_name == service_name;
                        // Match with underscore separator
                        let underscore = clean_name.starts_with(&format!("{}_", service_name));
                        // Match with dash separator
                        let dash = clean_name.starts_with(&format!("{}-", service_name));

                        // Docker Compose patterns
                        // Match pattern: project-service-replica (e.g., alt-alt-frontend-1)
                        let compose_pattern = clean_name.ends_with(&format!("-{}-1", service_name));
                        // Match pattern: project-service (e.g., alt-alt-frontend)
                        let compose_no_replica = clean_name.ends_with(&format!("-{}", service_name));
                        // Match pattern where service name appears after project prefix
                        let contains_dash = clean_name.contains(&format!("-{}-", service_name));

                        // Additional patterns for Docker Compose
                        // Match pattern: *-service-* (more flexible)
                        let flexible_pattern = clean_name.contains(&format!("-{}-", service_name)) ||
                                               clean_name.contains(&format!("-{}_", service_name));

                        // Match pattern: service appears in name with separators
                        let service_in_name = clean_name.split(&['-', '_'][..])
                            .any(|part| part == service_name);

                        exact || underscore || dash || compose_pattern || compose_no_replica || contains_dash || flexible_pattern || service_in_name
                    });

                if container_name.is_some() {
                    let id = container.id.ok_or_else(||
                        DiscoveryError::ContainerNotFound(format!("Container ID missing for {}", service_name))
                    )?;

                    let labels = container.labels.unwrap_or_default();
                    let group = labels.get("rask.group").cloned();

                    return Ok(ContainerInfo {
                        id,
                        service_name: service_name.to_string(),
                        labels,
                        group,
                    });
                }
            }
        }

        Err(DiscoveryError::ContainerNotFound(service_name.to_string()))
    }
}

impl std::fmt::Debug for ServiceDiscovery {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("ServiceDiscovery")
            .field("docker", &"Docker { ... }")
            .finish()
    }
}