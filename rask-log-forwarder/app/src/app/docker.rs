use super::{Config, ConfigError};
use std::collections::HashMap;
use std::env;

#[derive(Debug, Clone)]
pub struct DockerEnvironment {
    pub hostname: String,
    pub target_service: Option<String>,
    pub rask_endpoint: String,
    pub network_mode: Option<String>,
    pub container_labels: HashMap<String, String>,
}

impl DockerEnvironment {
    pub fn detect() -> Result<Self, ConfigError> {
        let hostname = env::var("HOSTNAME")
            .or_else(|_| {
                hostname::get()
                    .map_err(|e| format!("Failed to get hostname: {e}"))?
                    .to_str()
                    .map(str::to_string)
                    .ok_or_else(|| "Could not convert hostname to string".to_string())
            })
            .map_err(|e| ConfigError::EnvError(format!("Could not determine hostname: {e}")))?;

        let target_service = env::var("TARGET_SERVICE").ok();

        let rask_endpoint = env::var("RASK_ENDPOINT")
            .unwrap_or_else(|_| "http://rask-aggregator:9600/v1/aggregate".to_string());

        let network_mode = env::var("NETWORK_MODE").ok();

        // In a real Docker environment, we'd read container labels from the Docker API
        // For now, we'll simulate with environment variables
        let mut container_labels = HashMap::new();
        if let Ok(rask_group) = env::var("RASK_GROUP") {
            container_labels.insert("rask.group".to_string(), rask_group);
        }
        if let Ok(compose_service) = env::var("COMPOSE_SERVICE") {
            container_labels.insert("com.docker.compose.service".to_string(), compose_service);
        }

        Ok(Self {
            hostname,
            target_service,
            rask_endpoint,
            network_mode,
            container_labels,
        })
    }

    pub fn detect_target_service_from_labels(&self) -> Result<String, ConfigError> {
        // Try Docker Compose service label first
        if let Some(service) = self.container_labels.get("com.docker.compose.service") {
            return Ok(service.clone());
        }

        // Try hostname pattern
        if self.hostname.ends_with("-logs") {
            let service = self.hostname.trim_end_matches("-logs");
            return Ok(service.to_string());
        }

        Err(ConfigError::InvalidConfig(
            "Could not detect target service from Docker environment".to_string(),
        ))
    }

    pub fn is_sidecar_mode(&self) -> bool {
        self.network_mode
            .as_ref()
            .is_some_and(|mode| mode.starts_with("service:"))
    }

    pub fn get_target_service_from_network_mode(&self) -> Option<String> {
        self.network_mode.as_ref().and_then(|mode| {
            if mode.starts_with("service:") {
                Some(mode.trim_start_matches("service:").to_string())
            } else {
                None
            }
        })
    }
}

impl Config {
    pub fn from_docker_environment(docker_env: &DockerEnvironment) -> Result<Self, ConfigError> {
        let mut config = Config {
            target_service: docker_env
                .target_service
                .clone()
                .or_else(|| docker_env.get_target_service_from_network_mode())
                .or_else(|| docker_env.detect_target_service_from_labels().ok()),
            endpoint: docker_env.rask_endpoint.clone(),
            ..Config::default()
        };

        // Docker-specific optimizations
        if docker_env.is_sidecar_mode() {
            // Optimize for sidecar deployment
            config.connection_timeout_secs = 10; // Faster timeout in sidecar mode
            config.max_connections = 5; // Fewer connections needed
            config.enable_compression = true; // Enable compression for network efficiency
        }

        // Set disk fallback path to a Docker-appropriate location
        config.disk_fallback_path = std::path::PathBuf::from("/tmp/rask-fallback");

        config.post_process()?;
        config.validate()?;

        Ok(config)
    }

    pub fn for_docker_compose() -> Result<Self, ConfigError> {
        let docker_env = DockerEnvironment::detect()?;
        Self::from_docker_environment(&docker_env)
    }
}

// Dockerfile configuration validation
pub fn validate_docker_requirements(config: &Config) -> Result<(), ConfigError> {
    // Ensure endpoint points to aggregator service
    if !config.endpoint.contains("rask-aggregator") {
        return Err(ConfigError::InvalidConfig(
            "Endpoint should point to rask-aggregator service in Docker environment".to_string(),
        ));
    }

    // Ensure disk fallback path is writable in container
    if config.enable_disk_fallback {
        let parent = config
            .disk_fallback_path
            .parent()
            .ok_or_else(|| ConfigError::InvalidConfig("Invalid disk fallback path".to_string()))?;

        if !parent.exists() && parent != std::path::Path::new("/tmp") {
            return Err(ConfigError::InvalidConfig(format!(
                "Disk fallback parent directory not accessible: {}",
                parent.display(),
            )));
        }
    }

    Ok(())
}
