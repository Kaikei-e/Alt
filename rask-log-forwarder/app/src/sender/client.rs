use reqwest::{Client, ClientBuilder};
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, AtomicUsize, Ordering};
use std::time::Duration;
use thiserror::Error;
use tokio::time::timeout;
use url::Url;

#[cfg(test)]
use mockall::automock;

#[derive(Error, Debug)]
pub enum ClientError {
    #[error("Invalid configuration: {0}")]
    InvalidConfiguration(String),
    #[error("Connection failed: {0}")]
    ConnectionFailed(String),
    #[error("Request timeout: {0}")]
    RequestTimeout(String),
    #[error("HTTP error: {status} - {message}")]
    HttpError { status: u16, message: String },
    #[error("Serialization error: {0}")]
    SerializationError(String),
    #[error("Network error: {0}")]
    NetworkError(#[from] reqwest::Error),
}

#[derive(Debug, Clone)]
pub struct ClientConfig {
    pub endpoint: String,
    pub timeout: Duration,
    pub connection_timeout: Duration,
    pub max_connections: usize,
    pub keep_alive_timeout: Duration,
    pub user_agent: String,
    pub enable_compression: bool,
    pub retry_attempts: u32,
}

impl Default for ClientConfig {
    fn default() -> Self {
        Self {
            endpoint: "http://rask-aggregator:9600/v1/aggregate".to_string(),
            timeout: Duration::from_secs(30),
            connection_timeout: Duration::from_secs(10),
            max_connections: 20,
            keep_alive_timeout: Duration::from_secs(60),
            user_agent: "rask-log-forwarder/0.1.0".to_string(),
            enable_compression: true,
            retry_attempts: 3,
        }
    }
}

#[derive(Debug, Clone)]
pub struct ConnectionStats {
    pub max_connections: usize,
    pub active_connections: usize,
    pub total_requests: u64,
    pub successful_requests: u64,
    pub failed_requests: u64,
    pub average_response_time: Duration,
}

// Trait for mocking HTTP operations
#[cfg_attr(test, automock)]
pub trait HttpClientTrait: Send + Sync {
    fn health_check(&self) -> impl std::future::Future<Output = Result<(), ClientError>> + Send;
    fn connection_stats(&self) -> ConnectionStats;
    fn endpoint(&self) -> &str;
}

#[derive(Debug, Clone)]
pub struct HttpClient {
    pub client: Client,
    pub config: ClientConfig,
    endpoint_url: Url,
    pub aggregate_url: Url,
    pub stats: Arc<ClientStats>,
}

#[derive(Debug)]
pub struct ClientStats {
    total_requests: AtomicU64,
    successful_requests: AtomicU64,
    failed_requests: AtomicU64,
    active_connections: AtomicUsize,
    total_response_time: AtomicU64,
}

impl ClientStats {
    fn new() -> Self {
        Self {
            total_requests: AtomicU64::new(0),
            successful_requests: AtomicU64::new(0),
            failed_requests: AtomicU64::new(0),
            active_connections: AtomicUsize::new(0),
            total_response_time: AtomicU64::new(0),
        }
    }

    pub fn record_request(&self, success: bool, response_time: Duration) {
        self.total_requests.fetch_add(1, Ordering::Relaxed);
        self.total_response_time.fetch_add(response_time.as_millis() as u64, Ordering::Relaxed);

        if success {
            self.successful_requests.fetch_add(1, Ordering::Relaxed);
        } else {
            self.failed_requests.fetch_add(1, Ordering::Relaxed);
        }
    }
}

impl HttpClient {
    pub async fn new(config: ClientConfig) -> Result<Self, ClientError> {
        // Validate endpoint URL
        let endpoint_url: Url = config.endpoint.parse()
            .map_err(|e| ClientError::InvalidConfiguration(format!("Invalid endpoint URL: {}", e)))?;

        // Construct aggregate URL
        let aggregate_url = if config.endpoint.ends_with("/v1/aggregate") {
            endpoint_url.clone()
        } else {
            let mut url = endpoint_url.clone();
            if !url.path().ends_with('/') {
                url.set_path(&format!("{}/v1/aggregate", url.path()));
            } else {
                url.set_path(&format!("{}v1/aggregate", url.path()));
            }
            url
        };

        // Configure HTTP client with connection pooling
        let mut client_builder = ClientBuilder::new()
            .timeout(config.timeout)
            .connect_timeout(config.connection_timeout)
            .pool_max_idle_per_host(config.max_connections)
            .pool_idle_timeout(config.keep_alive_timeout)
            .user_agent(&config.user_agent);

        if config.enable_compression {
            client_builder = client_builder.gzip(true);
        }

        let client = client_builder.build()
            .map_err(|e| ClientError::InvalidConfiguration(format!("Failed to build HTTP client: {}", e)))?;

        let stats = Arc::new(ClientStats::new());

        Ok(Self {
            client,
            config,
            endpoint_url,
            aggregate_url,
            stats,
        })
    }

    pub fn endpoint(&self) -> &str {
        &self.config.endpoint
    }

    pub async fn health_check(&self) -> Result<(), ClientError> {
        let mut health_url = self.endpoint_url.clone();
        health_url.set_path("/v1/health");

        let start = std::time::Instant::now();

        let response = timeout(
            self.config.timeout,
            self.client.get(health_url).send()
        )
        .await
        .map_err(|_| ClientError::RequestTimeout("Health check timeout".to_string()))?
        .map_err(ClientError::NetworkError)?;

        let response_time = start.elapsed();
        let success = response.status().is_success();

        self.stats.record_request(success, response_time);

        if success {
            Ok(())
        } else {
            Err(ClientError::HttpError {
                status: response.status().as_u16(),
                message: format!("Health check failed: {}", response.status()),
            })
        }
    }

    pub fn connection_stats(&self) -> ConnectionStats {
        let total_requests = self.stats.total_requests.load(Ordering::Relaxed);
        let successful_requests = self.stats.successful_requests.load(Ordering::Relaxed);
        let failed_requests = self.stats.failed_requests.load(Ordering::Relaxed);
        let total_response_time = self.stats.total_response_time.load(Ordering::Relaxed);

        let average_response_time = if total_requests > 0 {
            Duration::from_millis(total_response_time / total_requests)
        } else {
            Duration::ZERO
        };

        ConnectionStats {
            max_connections: self.config.max_connections,
            active_connections: self.stats.active_connections.load(Ordering::Relaxed),
            total_requests,
            successful_requests,
            failed_requests,
            average_response_time,
        }
    }
}

impl HttpClientTrait for HttpClient {
    async fn health_check(&self) -> Result<(), ClientError> {
        let mut health_url = self.endpoint_url.clone();
        health_url.set_path("/v1/health");

        let start = std::time::Instant::now();

        let response = timeout(
            self.config.timeout,
            self.client.get(health_url).send()
        )
        .await
        .map_err(|_| ClientError::RequestTimeout("Health check timeout".to_string()))?
        .map_err(ClientError::NetworkError)?;

        let response_time = start.elapsed();
        let success = response.status().is_success();

        self.stats.record_request(success, response_time);

        if success {
            Ok(())
        } else {
            Err(ClientError::HttpError {
                status: response.status().as_u16(),
                message: format!("Health check failed: {}", response.status()),
            })
        }
    }

    fn connection_stats(&self) -> ConnectionStats {
        let total_requests = self.stats.total_requests.load(Ordering::Relaxed);
        let successful_requests = self.stats.successful_requests.load(Ordering::Relaxed);
        let failed_requests = self.stats.failed_requests.load(Ordering::Relaxed);
        let total_response_time = self.stats.total_response_time.load(Ordering::Relaxed);

        let average_response_time = if total_requests > 0 {
            Duration::from_millis(total_response_time / total_requests)
        } else {
            Duration::ZERO
        };

        ConnectionStats {
            max_connections: self.config.max_connections,
            active_connections: self.stats.active_connections.load(Ordering::Relaxed),
            total_requests,
            successful_requests,
            failed_requests,
            average_response_time,
        }
    }

    fn endpoint(&self) -> &str {
        &self.config.endpoint
    }
}