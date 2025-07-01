use bytes::Bytes;
use http_body_util::Empty;
use hyper::{Method, Request, Uri};
use hyper_util::client::legacy::{Client, connect::HttpConnector};
use std::sync::Arc;
use std::time::Duration;
use thiserror::Error;
use tokio::time::timeout;

#[derive(Error, Debug)]
pub enum SenderError {
    #[error("Connection failed: {0}")]
    ConnectionFailed(String),
    #[error("Request timeout: {0}")]
    Timeout(String),
    #[error("HTTP error: {status}")]
    HttpError { status: u16 },
    #[error("Serialization error: {0}")]
    SerializationError(String),
    #[error("Invalid configuration: {0}")]
    InvalidConfig(String),
    #[error("Network error: {0}")]
    NetworkError(String),
}

#[derive(Debug, Clone)]
pub struct SenderConfig {
    pub endpoint: String,
    pub timeout: Duration,
    pub max_connections: usize,
    pub keep_alive: bool,
    pub keep_alive_timeout: Duration,
    pub user_agent: String,
    pub compression: bool,
}

impl Default for SenderConfig {
    fn default() -> Self {
        Self {
            endpoint: "http://localhost:9600/v1/aggregate".to_string(),
            timeout: Duration::from_secs(30),
            max_connections: 10,
            keep_alive: true,
            keep_alive_timeout: Duration::from_secs(60),
            user_agent: "rask-forwarder/0.1.0".to_string(),
            compression: false,
        }
    }
}

#[derive(Debug, Clone)]
pub struct ConnectionStats {
    pub active_connections: usize,
    pub total_requests: u64,
    pub reused_connections: u64,
    pub failed_requests: u64,
}

#[derive(Clone)]
pub struct BatchSender {
    client: Client<HttpConnector, http_body_util::Empty<Bytes>>,
    config: SenderConfig,
    endpoint_uri: Uri,
    stats: Arc<std::sync::Mutex<ConnectionStats>>,
}

impl BatchSender {
    pub async fn new(config: SenderConfig) -> Result<Self, SenderError> {
        // Validate endpoint URL
        let endpoint_uri = config
            .endpoint
            .parse::<Uri>()
            .map_err(|e| SenderError::InvalidConfig(format!("Invalid endpoint URL: {}", e)))?;

        // Configure HTTP client with connection pooling
        let connector = HttpConnector::new();

        let client = Client::builder(hyper_util::rt::TokioExecutor::new())
            .pool_max_idle_per_host(config.max_connections)
            .pool_idle_timeout(config.keep_alive_timeout)
            .build(connector);

        let sender = Self {
            client,
            config,
            endpoint_uri,
            stats: Arc::new(std::sync::Mutex::new(ConnectionStats {
                active_connections: 0,
                total_requests: 0,
                reused_connections: 0,
                failed_requests: 0,
            })),
        };

        // Test initial connection
        sender.health_check().await.map_err(|e| {
            SenderError::ConnectionFailed(format!("Initial connection test failed: {}", e))
        })?;

        Ok(sender)
    }

    pub async fn can_connect(&self) -> bool {
        self.health_check().await.is_ok()
    }

    pub async fn health_check(&self) -> Result<(), SenderError> {
        let health_uri = format!("{}/v1/health", self.config.endpoint)
            .parse::<Uri>()
            .map_err(|e| SenderError::InvalidConfig(format!("Invalid health check URL: {}", e)))?;

        let request = Request::builder()
            .method(Method::GET)
            .uri(health_uri)
            .header("User-Agent", &self.config.user_agent)
            .body(Empty::<Bytes>::new())
            .map_err(|_| SenderError::InvalidConfig("Failed to build request".to_string()))?;

        let response = timeout(self.config.timeout, self.client.request(request))
            .await
            .map_err(|_| SenderError::Timeout("Health check timeout".to_string()))?
            .map_err(|e| SenderError::NetworkError(e.to_string()))?;

        self.update_stats(|stats| {
            stats.total_requests += 1;
            if response.status().is_success() {
                stats.reused_connections += 1;
            } else {
                stats.failed_requests += 1;
            }
        });

        if response.status().is_success() {
            Ok(())
        } else {
            Err(SenderError::HttpError {
                status: response.status().as_u16(),
            })
        }
    }

    pub async fn connection_stats(&self) -> ConnectionStats {
        self.stats.lock().unwrap().clone()
    }

    fn update_stats<F>(&self, f: F)
    where
        F: FnOnce(&mut ConnectionStats),
    {
        if let Ok(mut stats) = self.stats.lock() {
            f(&mut stats);
        }
    }
}

impl std::fmt::Debug for BatchSender {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("BatchSender")
            .field("config", &self.config)
            .field("endpoint_uri", &self.endpoint_uri)
            .field("stats", &self.stats)
            .finish()
    }
}
