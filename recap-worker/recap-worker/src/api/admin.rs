use axum::{extract::State, http::StatusCode, response::IntoResponse};
use tracing::warn;
use uuid::Uuid;

use crate::{app::AppState, scheduler::JobContext};

pub(crate) async fn retry_jobs(State(state): State<AppState>) -> impl IntoResponse {
    state.telemetry().record_admin_retry_invocation();
    let job = JobContext::new(Uuid::new_v4(), Vec::new());
    match state.scheduler().run_job(job).await {
        Ok(()) => StatusCode::ACCEPTED.into_response(),
        Err(error) => {
            warn!(error = %error, "failed to retry recap job");
            (StatusCode::NOT_IMPLEMENTED, error.to_string()).into_response()
        }
    }
}
