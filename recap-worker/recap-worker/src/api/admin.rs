use axum::{Json, extract::State, http::StatusCode, response::IntoResponse};
use serde::Serialize;
use tracing::warn;
use uuid::Uuid;

use crate::{app::AppState, scheduler::RetryOutcome};

#[derive(Debug, Serialize)]
struct RetryAcceptedResponse {
    job_id: Uuid,
    retried_failed_job_id: Uuid,
    status: &'static str,
}

#[derive(Debug, Serialize)]
struct RetryErrorResponse {
    error: String,
}

/// Maps a `RetryOutcome` to its HTTP status code. Split out from the handler
/// so the "no-op must never look like success" invariant (CLAUDE.md rule 8)
/// is directly unit-testable without spinning up axum or a DB.
fn retry_outcome_status(outcome: &RetryOutcome) -> StatusCode {
    match outcome {
        RetryOutcome::Started { .. } => StatusCode::ACCEPTED,
        RetryOutcome::NoFailedJob => StatusCode::NOT_FOUND,
        RetryOutcome::AlreadyRunning => StatusCode::CONFLICT,
    }
}

pub(crate) async fn retry_jobs(State(state): State<AppState>) -> impl IntoResponse {
    state.telemetry().record_admin_retry_invocation();

    match state.scheduler().retry_most_recent_failed_job().await {
        Ok(outcome) => {
            let status = retry_outcome_status(&outcome);
            match outcome {
                RetryOutcome::Started {
                    job_id,
                    retried_failed_job_id,
                } => (
                    status,
                    Json(RetryAcceptedResponse {
                        job_id,
                        retried_failed_job_id,
                        status: "accepted",
                    }),
                )
                    .into_response(),
                RetryOutcome::NoFailedJob => (
                    status,
                    Json(RetryErrorResponse {
                        error: "no failed recap job to retry".to_string(),
                    }),
                )
                    .into_response(),
                RetryOutcome::AlreadyRunning => (
                    status,
                    Json(RetryErrorResponse {
                        error: "another recap pipeline run is already in progress".to_string(),
                    }),
                )
                    .into_response(),
            }
        }
        Err(error) => {
            warn!(error = %error, "failed to look up most recent failed recap job for retry");
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(RetryErrorResponse {
                    error: error.to_string(),
                }),
            )
                .into_response()
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// RED→GREEN regression (rule 8): the old `/admin/jobs/retry` built an
    /// empty-genres `JobContext` unconditionally and reported
    /// `202 Accepted` as long as `run_job` returned `Ok`, regardless of
    /// whether there was anything to retry — a silent no-op success. This
    /// pins that `NoFailedJob` maps to `404`, never `202`.
    #[test]
    fn no_failed_job_maps_to_not_found_not_accepted() {
        let status = retry_outcome_status(&RetryOutcome::NoFailedJob);
        assert_eq!(status, StatusCode::NOT_FOUND);
        assert_ne!(status, StatusCode::ACCEPTED);
    }

    #[test]
    fn already_running_maps_to_conflict_not_accepted() {
        let status = retry_outcome_status(&RetryOutcome::AlreadyRunning);
        assert_eq!(status, StatusCode::CONFLICT);
        assert_ne!(status, StatusCode::ACCEPTED);
    }

    #[test]
    fn started_maps_to_accepted() {
        let outcome = RetryOutcome::Started {
            job_id: Uuid::new_v4(),
            retried_failed_job_id: Uuid::new_v4(),
        };
        assert_eq!(retry_outcome_status(&outcome), StatusCode::ACCEPTED);
    }
}
