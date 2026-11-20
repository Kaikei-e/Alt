use std::collections::HashMap;
use std::sync::Arc;

use bollard::Docker;
use thiserror::Error;

use super::docker::{DockerConnectError, DockerEndpointError, connect_docker_with};

#[derive(Error, Debug, Clone)]
pub enum DiscoveryError {
    #[error("Container not found for service: {0}")]
    ContainerNotFound(String),
    #[error("No target service specified")]
    NoTargetService,
    #[error("Docker API error: {0}")]
    DockerError(Arc<bollard::errors::Error>),
    #[error("Invalid hostname format: {0}")]
    InvalidHostname(String),
    #[error("Docker endpoint configuration error: {0}")]
    EndpointError(#[from] DockerEndpointError),
}

impl From<bollard::errors::Error> for DiscoveryError {
    fn from(error: bollard::errors::Error) -> Self {
        Self::DockerError(Arc::new(error))
    }
}

impl From<DockerConnectError> for DiscoveryError {
    fn from(err: DockerConnectError) -> Self {
        match err {
            DockerConnectError::Endpoint(e) => Self::EndpointError(e),
            DockerConnectError::Bollard(e) => Self::DockerError(Arc::new(e)),
        }
    }
}

/// Returns true when `clean_name` refers to Compose service `service_name`.
///
/// Matches:
/// - exact name / `{service}-{replica}`
/// - `{project}-{service}[-{replica}]` only when `service_name` itself contains
///   a hyphen (multi-segment services like `alt-backend`), so a short name like
///   `"db"` cannot falsely match `"kratos-db-1"` via a fake project prefix.
///
/// Loose token `contains` matching is intentionally not used. Single-segment
/// services under a Compose project should match via `com.docker.compose.service`.
pub(crate) fn container_name_matches_service(clean_name: &str, service_name: &str) -> bool {
    if clean_name == service_name {
        return true;
    }

    let name_tokens: Vec<&str> = clean_name.split('-').collect();
    let service_tokens: Vec<&str> = service_name.split('-').collect();
    if service_tokens.is_empty() || name_tokens.is_empty() {
        return false;
    }

    // Strip trailing numeric replica (e.g. "...-1")
    let core = match name_tokens.split_last() {
        Some((last, rest))
            if !rest.is_empty() && !last.is_empty() && last.chars().all(|c| c.is_ascii_digit()) =>
        {
            rest
        }
        _ => name_tokens.as_slice(),
    };

    if core == service_tokens.as_slice() {
        return true;
    }

    // `{project}-{service}` for multi-segment services only (unambiguous).
    if service_tokens.len() > 1
        && core.len() == service_tokens.len() + 1
        && core[1..] == service_tokens[..]
    {
        return true;
    }

    false
}

#[derive(Debug, Clone)]
pub struct ContainerInfo {
    pub id: String,
    pub service_name: String,
    pub labels: HashMap<String, String>,
    pub group: Option<String>,
}

pub trait ServiceDiscoveryTrait: Send + Sync {
    /// Finds a running container matching `service_name`.
    ///
    /// # Errors
    ///
    /// Returns [`DiscoveryError::DockerError`] if listing containers fails, or
    /// [`DiscoveryError::ContainerNotFound`] if no matching container exists.
    fn find_container_by_service(
        &self,
        service_name: &str,
    ) -> impl std::future::Future<Output = Result<ContainerInfo, DiscoveryError>> + Send;

    /// Determines the target service name from environment or hostname.
    ///
    /// # Errors
    ///
    /// Returns [`DiscoveryError::NoTargetService`] if the hostname cannot be
    /// read or [`DiscoveryError::InvalidHostname`] if it does not match
    /// `*-logs`.
    fn get_target_service(&self) -> Result<String, DiscoveryError>;

    /// Extracts the service name from a `*-logs` hostname.
    ///
    /// # Errors
    ///
    /// Returns [`DiscoveryError::InvalidHostname`] if `hostname` does not end
    /// with `"-logs"` or has an empty service prefix.
    fn detect_target_service_from_hostname(&self, hostname: &str)
    -> Result<String, DiscoveryError>;
}

type EnvLookup = Arc<dyn Fn(&str) -> Option<String> + Send + Sync>;

pub struct ServiceDiscovery {
    docker: Docker,
    env_lookup: EnvLookup,
}

impl ServiceDiscovery {
    /// Connects to the Docker daemon using the process environment.
    ///
    /// # Errors
    ///
    /// Returns [`DiscoveryError::EndpointError`] if `DOCKER_HOST` is missing or
    /// invalid, or [`DiscoveryError::DockerError`] if creating the Docker
    /// client fails.
    pub fn new() -> Result<Self, DiscoveryError> {
        Self::new_with_env(|k| std::env::var(k).ok())
    }

    /// Connects to the Docker daemon using `lookup` as the sole source of
    /// environment variables (both `DOCKER_HOST` and `TARGET_SERVICE`).
    ///
    /// # Errors
    ///
    /// Returns [`DiscoveryError::EndpointError`] if `DOCKER_HOST` is missing or
    /// invalid, or [`DiscoveryError::DockerError`] if creating the Docker
    /// client fails.
    pub fn new_with_env<F>(lookup: F) -> Result<Self, DiscoveryError>
    where
        F: Fn(&str) -> Option<String> + Send + Sync + 'static,
    {
        let env_lookup: EnvLookup = Arc::new(lookup);
        let docker = connect_docker_with(|k| env_lookup(k))?;
        Ok(Self { docker, env_lookup })
    }

    /// Connects to the Docker daemon at an explicit `endpoint` URL.
    ///
    /// # Errors
    ///
    /// Returns [`DiscoveryError::EndpointError`] if `endpoint` fails
    /// validation, or [`DiscoveryError::DockerError`] if creating the Docker
    /// client fails.
    pub fn from_endpoint(endpoint: &str) -> Result<Self, DiscoveryError> {
        let endpoint = endpoint.to_string();
        Self::new_with_env(move |k| (k == "DOCKER_HOST").then(|| endpoint.clone()))
    }
}

impl ServiceDiscoveryTrait for ServiceDiscovery {
    fn get_target_service(&self) -> Result<String, DiscoveryError> {
        // Check injected environment lookup first
        if let Some(service) = (self.env_lookup)("TARGET_SERVICE") {
            return Ok(service);
        }

        // Auto-detect from hostname
        let hostname = hostname::get()
            .ok()
            .and_then(|h| h.to_str().map(str::to_string))
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
        let service_name = service_name.to_string();
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
            // Prefer the Compose service label when present (unambiguous).
            if let Some(labels) = &container.labels
                && labels
                    .get("com.docker.compose.service")
                    .is_some_and(|s| s == &service_name)
            {
                best_match = Some(container);
                break;
            }

            if let Some(names) = &container.names {
                for name in names {
                    let clean_name = name.trim_start_matches('/');
                    if container_name_matches_service(clean_name, &service_name) {
                        best_match = Some(container.clone());
                        break;
                    }
                }
            }

            if best_match.is_some() {
                break;
            }
        }

        if let Some(container) = best_match {
            let id = container.id.ok_or_else(|| {
                DiscoveryError::ContainerNotFound(format!(
                    "Container ID missing for {service_name}"
                ))
            })?;

            let labels = container.labels.unwrap_or_default();
            let group = labels.get("rask.group").cloned();

            return Ok(ContainerInfo {
                id,
                service_name: service_name.clone(),
                labels,
                group,
            });
        }

        Err(DiscoveryError::ContainerNotFound(service_name.clone()))
    }
}

impl std::fmt::Debug for ServiceDiscovery {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("ServiceDiscovery")
            .field("docker", &"Docker { ... }")
            .finish()
    }
}

#[cfg(test)]
mod tests {
    use super::container_name_matches_service;

    #[test]
    fn exact_and_replica_names_match() {
        assert!(container_name_matches_service("db", "db"));
        assert!(container_name_matches_service("db-1", "db"));
        assert!(container_name_matches_service("alt-backend", "alt-backend"));
        assert!(container_name_matches_service(
            "alt-backend-1",
            "alt-backend"
        ));
    }

    #[test]
    fn compose_project_prefix_matches_multi_segment_service() {
        assert!(container_name_matches_service(
            "alt-alt-backend",
            "alt-backend"
        ));
        assert!(container_name_matches_service(
            "alt-alt-backend-1",
            "alt-backend"
        ));
        assert!(container_name_matches_service(
            "alt-kratos-db-1",
            "kratos-db"
        ));
    }

    #[test]
    fn short_service_name_does_not_match_longer_hyphenated_name() {
        // Regression: parts.contains("db") previously matched "kratos-db-1".
        assert!(!container_name_matches_service("kratos-db", "db"));
        assert!(!container_name_matches_service("kratos-db-1", "db"));
        assert!(!container_name_matches_service("alt-kratos-db-1", "db"));
    }
}
