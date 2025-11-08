pub(crate) mod admin;
pub(crate) mod fetch;
pub(crate) mod generate;
pub(crate) mod health;
pub(crate) mod metrics;

use axum::{
    routing::{get, post},
    Router,
};

use crate::app::AppState;

pub(crate) fn router(state: AppState) -> Router {
    Router::new()
        .route("/health/ready", get(health::ready))
        .route("/health/live", get(health::live))
        .route("/metrics", get(metrics::exporter))
        .route("/admin/jobs/retry", post(admin::retry_jobs))
        .route("/v1/generate/recaps/7days", post(generate::trigger_7days))
        .route("/v1/recaps/7days", get(fetch::get_7days_recap))
        .with_state(state)
}
