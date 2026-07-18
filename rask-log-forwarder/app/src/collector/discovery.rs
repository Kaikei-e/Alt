use bollard::Docker;
use std::collections::HashMap;
use std::env;
use std::sync::Arc;
use thiserror::Error;

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
}

impl From<bollard::errors::Error> for DiscoveryError {
    fn from(error: bollard::errors::Error) -> Self {
        Self::DockerError(Arc::new(error))
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

#[allow(async_fn_in_trait)]
pub trait ServiceDiscoveryTrait: Send + Sync {
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

impl ServiceDiscoveryTrait for ServiceDiscovery {
    fn get_target_service(&self) -> Result<String, DiscoveryError> {
        // Check environment variable first
        if let Ok(service) = env::var("TARGET_SERVICE") {
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

#[cfg(test)]
mod tests {
    use super::container_name_matches_service;

    #[test]
    fn exact_and_replica_names_match() {
        assert!(container_name_matches_service("db", "db"));
        assert!(container_name_matches_service("db-1", "db"));
        assert!(container_name_matches_service("alt-backend", "alt-backend"));
        assert!(container_name_matches_service("alt-backend-1", "alt-backend"));
    }

    #[test]
    fn compose_project_prefix_matches_multi_segment_service() {
        assert!(container_name_matches_service("alt-alt-backend", "alt-backend"));
        assert!(container_name_matches_service("alt-alt-backend-1", "alt-backend"));
        assert!(container_name_matches_service("alt-kratos-db-1", "kratos-db"));
    }

    #[test]
    fn short_service_name_does_not_match_longer_hyphenated_name() {
        // Regression: parts.contains("db") previously matched "kratos-db-1".
        assert!(!container_name_matches_service("kratos-db", "db"));
        assert!(!container_name_matches_service("kratos-db-1", "db"));
        assert!(!container_name_matches_service("alt-kratos-db-1", "db"));
    }
}
