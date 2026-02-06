use tracing::info;

/// Handler for GET /v1/health
pub async fn health_handler() -> &'static str {
    info!("Health check requested");
    "Healthy"
}
