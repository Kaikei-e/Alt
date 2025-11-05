use axum::{Json, extract::State, http::StatusCode};
use serde::Serialize;
use tracing::error;

use crate::app::AppState;

#[derive(Debug, Serialize)]
#[serde(rename_all = "snake_case")]
pub(crate) struct HealthReport {
    status: &'static str,
    #[serde(skip_serializing_if = "Option::is_none")]
    detail: Option<String>,
}

impl HealthReport {
    fn ready() -> Self {
        Self {
            status: "ready",
            detail: None,
        }
    }

    fn degraded(detail: impl Into<String>) -> Self {
        Self {
            status: "degraded",
            detail: Some(detail.into()),
        }
    }
}

pub(crate) async fn ready(
    State(state): State<AppState>,
) -> Result<Json<HealthReport>, (StatusCode, Json<HealthReport>)> {
    state.telemetry().record_ready_probe();

    if let Err(error) = state.subworker_client().ping().await {
        error!(%error, "subworker readiness check failed");
        return Err((
            StatusCode::SERVICE_UNAVAILABLE,
            Json(HealthReport::degraded(format!("subworker: {error:#}"))),
        ));
    }

    if let Err(error) = state.news_creator_client().health_check().await {
        error!(%error, "news-creator readiness check failed");
        return Err((
            StatusCode::SERVICE_UNAVAILABLE,
            Json(HealthReport::degraded(format!("news_creator: {error:#}"))),
        ));
    }

    Ok(Json(HealthReport::ready()))
}

pub(crate) async fn live(State(state): State<AppState>) -> Json<HealthReport> {
    state.telemetry().record_live_probe();
    Json(HealthReport {
        status: "live",
        detail: None,
    })
}
