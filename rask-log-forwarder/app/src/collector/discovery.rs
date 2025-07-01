use bollard::Docker;
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
            DiscoveryError::DockerError(e) => {
                DiscoveryError::DockerError(bollard::errors::Error::DockerResponseServerError {
                    status_code: 500,
                    message: format!("Cloned error: {e}"),
                })
            }
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
    async fn find_container_by_service(
        &self,
        service_name: &str,
    ) -> Result<ContainerInfo, DiscoveryError>;
    fn get_target_service(&self) -> Result<String, DiscoveryError>;
    fn detect_target_service_from_hostname(&self, hostname: &str)
    -> Result<String, DiscoveryError>;
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

    fn detect_target_service_from_hostname(
        &self,
        hostname: &str,
    ) -> Result<String, DiscoveryError> {
        // Pattern: "service-logs" → "service"
        if hostname.ends_with("-logs") {
            let service_name = hostname.trim_end_matches("-logs");
            if service_name.is_empty() {
                return Err(DiscoveryError::InvalidHostname(hostname.to_string()));
            }
            Ok(service_name.to_string())
        } else {
            Err(DiscoveryError::InvalidHostname(format!(
                "Hostname '{hostname}' doesn't match pattern '*-logs'"
            )))
        }
    }

    async fn find_container_by_service(
        &self,
        service_name: &str,
    ) -> Result<ContainerInfo, DiscoveryError> {
        let options = bollard::query_parameters::ListContainersOptions {
            all: false, // Only running containers
            ..Default::default()
        };

        let containers = self.docker.list_containers(Some(options)).await?;

        // First, filter out any containers that are for logging
        let filtered_containers: Vec<_> = containers
            .into_iter()
            .filter(|c| {
                if let Some(names) = &c.names {
                    // Keep the container if NONE of its names end with "-logs"
                    !names
                        .iter()
                        .any(|n| n.trim_start_matches('/').ends_with("-logs"))
                } else {
                    // Keep containers with no names (unlikely, but safe)
                    true
                }
            })
            .collect();

        // From the filtered list, find the best match
        let mut best_match: Option<bollard::service::ContainerSummary> = None;

        for container in filtered_containers {
            if let Some(names) = &container.names {
                for name in names {
                    let clean_name = name.trim_start_matches('/');

                    // Exact match is the best
                    if clean_name == service_name {
                        best_match = Some(container);
                        break;
                    }

                    // Docker Compose pattern: project-service-replica (e.g., alt-alt-backend-1)
                    // Check if the name ends with "-service-1" or "-service"
                    if clean_name.ends_with(&format!("-{service_name}-1"))
                        || clean_name.ends_with(&format!("-{service_name}"))
                    {
                        best_match = Some(container.clone());
                        break;
                    }

                    // Also check if any part of the name matches the service exactly
                    let parts: Vec<&str> = clean_name.split('-').collect();
                    if parts.contains(&service_name) {
                        // Make sure it's not a logs container (double check)
                        if !clean_name.contains("-logs") {
                            best_match = Some(container.clone());
                        }
                    }
                }
            }

            if best_match.is_some() {
                // If we found any match, check if it's an exact service name match
                if let Some(ref matched_container) = best_match {
                    if let Some(names) = &matched_container.names {
                        if names.iter().any(|n| {
                            let clean = n.trim_start_matches('/');
                            clean == service_name
                                || clean.ends_with(&format!("-{service_name}-1"))
                                || clean.ends_with(&format!("-{service_name}"))
                        }) {
                            break; // Found a good match, stop searching
                        }
                    }
                }
            }
        }

        if let Some(container) = best_match {
            let id = container.id.ok_or_else(|| {
                DiscoveryError::ContainerNotFound(format!("Container ID missing for {service_name}"))
            })?;

            let labels = container.labels.unwrap_or_default();
            let group = labels.get("rask.group").cloned();

            return Ok(ContainerInfo {
                id,
                service_name: service_name.to_string(),
                labels,
                group,
            });
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
