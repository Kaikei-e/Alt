use serde::{Deserialize, Serialize};
use std::path::PathBuf;
use std::time::Duration;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RetryConfig {
    pub max_attempts: u32,
    #[serde(with = "super::serde_helpers")]
    pub base_delay: Duration,
    #[serde(with = "super::serde_helpers")]
    pub max_delay: Duration,
    pub jitter: bool,
}

impl Default for RetryConfig {
    fn default() -> Self {
        Self {
            max_attempts: 5,
            base_delay: Duration::from_millis(500),
            max_delay: Duration::from_secs(60),
            jitter: true,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DiskFallbackConfig {
    pub enabled: bool,
    pub storage_path: PathBuf,
    pub max_disk_usage_mb: u64,
    pub retention_hours: u64,
    pub compression: bool,
}

impl Default for DiskFallbackConfig {
    fn default() -> Self {
        Self {
            enabled: true,
            storage_path: PathBuf::from("/tmp/rask-log-forwarder/fallback"),
            max_disk_usage_mb: 1000, // 1GB
            retention_hours: 24,
            compression: true,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MetricsConfig {
    pub enabled: bool,
    pub port: u16,
    pub path: String,
}

impl Default for MetricsConfig {
    fn default() -> Self {
        Self {
            enabled: true,
            port: 9090,
            path: "/metrics".to_string(),
        }
    }
}
