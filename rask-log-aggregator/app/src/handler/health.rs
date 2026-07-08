use tracing::trace;

/// Handler for GET /v1/health
pub async fn health_handler() -> &'static str {
    // Docker healthchecks hit this every ~10s - info! would self-amplify logs.
    trace!("Health check requested");
    "Healthy"
}
