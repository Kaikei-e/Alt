use axum::{extract::State, http::StatusCode, response::IntoResponse};

use crate::app::AppState;

pub(crate) async fn exporter(State(state): State<AppState>) -> impl IntoResponse {
    (StatusCode::OK, state.telemetry().render_prometheus()).into_response()
}
