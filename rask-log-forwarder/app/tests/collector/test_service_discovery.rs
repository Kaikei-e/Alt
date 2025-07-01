use async_trait::async_trait;
use rask_log_forwarder::collector::{ContainerInfo, DiscoveryError, ServiceDiscoveryTrait};
use std::collections::HashMap;

// Mock implementation for testing
pub struct MockServiceDiscovery {
    pub available_containers: Vec<ContainerInfo>,
    pub target_service_result: Result<String, DiscoveryError>,
}

impl MockServiceDiscovery {
    pub fn new() -> Self {
        Self {
            available_containers: Vec::new(),
            target_service_result: Err(DiscoveryError::NoTargetService),
        }
    }

    pub fn with_containers(mut self, containers: Vec<ContainerInfo>) -> Self {
        self.available_containers = containers;
        self
    }

    pub fn with_target_service(mut self, service: String) -> Self {
        self.target_service_result = Ok(service);
        self
    }
}

#[async_trait]
impl ServiceDiscoveryTrait for MockServiceDiscovery {
    async fn find_container_by_service(
        &self,
        service_name: &str,
    ) -> Result<ContainerInfo, DiscoveryError> {
        self.available_containers
            .iter()
            .find(|container| container.service_name == service_name)
            .cloned()
            .ok_or_else(|| DiscoveryError::ContainerNotFound(service_name.to_string()))
    }

    fn get_target_service(&self) -> Result<String, DiscoveryError> {
        self.target_service_result.clone()
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
}

#[tokio::test]
async fn test_service_discovery_from_hostname() {
    let discovery = MockServiceDiscovery::new();

    // Mock hostname for testing
    let service_name = discovery
        .detect_target_service_from_hostname("nginx-logs")
        .unwrap();
    assert_eq!(service_name, "nginx");

    let service_name = discovery
        .detect_target_service_from_hostname("alt-backend-logs")
        .unwrap();
    assert_eq!(service_name, "alt-backend");

    let service_name = discovery
        .detect_target_service_from_hostname("meilisearch-logs")
        .unwrap();
    assert_eq!(service_name, "meilisearch");
}

#[tokio::test]
async fn test_service_discovery_from_env() {
    let discovery = MockServiceDiscovery::new().with_target_service("pre-processor".to_string());

    let service_name = discovery.get_target_service().unwrap();
    assert_eq!(service_name, "pre-processor");
}

#[tokio::test]
async fn test_container_discovery_by_service_name() {
    // Create mock container info
    let mut labels = HashMap::new();
    labels.insert("rask.group".to_string(), "alt-backend".to_string());

    let nginx_container = ContainerInfo {
        id: "nginx-container-id".to_string(),
        service_name: "nginx".to_string(),
        labels: labels.clone(),
        group: Some("alt-frontend".to_string()),
    };

    let discovery = MockServiceDiscovery::new().with_containers(vec![nginx_container.clone()]);

    // Test successful container discovery
    let result = discovery.find_container_by_service("nginx").await;
    assert!(result.is_ok());
    let container = result.unwrap();
    assert_eq!(container.service_name, "nginx");
    assert_eq!(container.id, "nginx-container-id");

    // Test container not found
    let result = discovery
        .find_container_by_service("nonexistent-service")
        .await;
    assert!(result.is_err());
    assert!(matches!(
        result.unwrap_err(),
        DiscoveryError::ContainerNotFound(_)
    ));
}

#[tokio::test]
async fn test_discovery_error_for_nonexistent_service() {
    let discovery = MockServiceDiscovery::new();

    let result = discovery
        .find_container_by_service("nonexistent-service")
        .await;
    assert!(result.is_err());
    assert!(matches!(
        result.unwrap_err(),
        DiscoveryError::ContainerNotFound(_)
    ));
}

#[tokio::test]
async fn test_invalid_hostname_format() {
    let discovery = MockServiceDiscovery::new();

    let result = discovery.detect_target_service_from_hostname("invalid-hostname");
    assert!(result.is_err());
    assert!(matches!(
        result.unwrap_err(),
        DiscoveryError::InvalidHostname(_)
    ));

    let result = discovery.detect_target_service_from_hostname("-logs");
    assert!(result.is_err());
    assert!(matches!(
        result.unwrap_err(),
        DiscoveryError::InvalidHostname(_)
    ));
}
