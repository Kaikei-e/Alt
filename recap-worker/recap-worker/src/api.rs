pub(crate) mod admin;
pub(crate) mod evaluation;
pub(crate) mod fetch;
pub(crate) mod generate;
pub(crate) mod health;
pub(crate) mod learning;
pub(crate) mod metrics;

use axum::{
    Router,
    routing::{get, post},
};

use crate::app::AppState;

pub(crate) fn router(state: AppState) -> Router {
    Router::new()
        .route("/health/ready", get(health::ready))
        .route("/health/live", get(health::live))
        .route("/metrics", get(metrics::exporter))
        .route("/admin/jobs/retry", post(admin::retry_jobs))
        .route(
            "/admin/genre-learning",
            post(learning::receive_genre_learning),
        )
        .route("/v1/generate/recaps/7days", post(generate::trigger_7days))
        .route("/v1/recaps/7days", get(fetch::get_7days_recap))
        .route("/v1/morning/updates", get(fetch::get_morning_updates))
        .route("/v1/evaluation/genres", post(evaluation::evaluate_genres))
        .with_state(state)
}
