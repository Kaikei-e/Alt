use std::time::Duration;

/// Default HTTP port for health checks
const DEFAULT_HTTP_PORT: u16 = 9600;

/// Error type for healthcheck failures
#[derive(Debug)]
pub struct HealthcheckError(String);

impl std::fmt::Display for HealthcheckError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "Healthcheck failed: {}", self.0)
    }
}

impl std::error::Error for HealthcheckError {}

/// Perform a health check against the default port (9600)
pub async fn healthcheck() -> Result<(), HealthcheckError> {
    healthcheck_with_port(DEFAULT_HTTP_PORT).await
}

/// Perform a health check against a specific port
pub async fn healthcheck_with_port(port: u16) -> Result<(), HealthcheckError> {
    let client = reqwest::Client::builder()
        .timeout(Duration::from_secs(2))
        .build()
        .map_err(|e| HealthcheckError(format!("Failed to create HTTP client: {}", e)))?;

    let url = format!("http://127.0.0.1:{}/v1/health", port);

    let resp = client
        .get(&url)
        .send()
        .await
        .map_err(|e| HealthcheckError(format!("Request failed: {}", e)))?;

    if resp.status().is_success() {
        Ok(())
    } else {
        Err(HealthcheckError(format!(
            "Health endpoint returned status: {}",
            resp.status()
        )))
    }
}
