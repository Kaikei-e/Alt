use crate::handler::aggregate::aggregate_handler;
use crate::handler::health::health_handler;
use crate::otlp::otlp_routes;
use crate::otlp::receiver::OTLPState;
use crate::port::{LogExporter, OTelExporter};
use axum::Router;
use axum::routing::{get, post};
use std::sync::Arc;

/// Build the main HTTP router (health + legacy aggregate).
pub fn main_router(exporter: Arc<dyn LogExporter>) -> Router {
    let v1_health_router = Router::new().route("/v1/health", get(health_handler));

    let v1_aggregate_router = Router::new()
        .route("/v1/aggregate", post(aggregate_handler))
        .with_state(exporter);

    Router::new()
        .merge(v1_health_router)
        .merge(v1_aggregate_router)
}

/// Build the OTLP HTTP router (logs + traces).
pub fn otlp_router(exporter: Arc<dyn OTelExporter>) -> Router {
    let state = OTLPState { exporter };
    otlp_routes(state)
}
