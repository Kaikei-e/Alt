use std::collections::HashSet;

use axum::{Json, extract::State, http::StatusCode, response::IntoResponse};
use serde::{Deserialize, Serialize};
use tracing::{error, info};
use uuid::Uuid;

use crate::{app::AppState, scheduler::JobContext};

#[derive(Debug, Deserialize)]
pub(crate) struct GenerateRecapRequest {
    #[serde(default)]
    genres: Option<Vec<String>>,
}

#[derive(Debug, Serialize)]
struct GenerateRecapResponse {
    job_id: Uuid,
    genres: Vec<String>,
    status: &'static str,
}

#[derive(Debug, Serialize)]
struct ErrorResponse {
    error: String,
}

pub(crate) async fn trigger_7days(
    State(state): State<AppState>,
    Json(payload): Json<GenerateRecapRequest>,
) -> impl IntoResponse {
    state.telemetry().record_manual_generate_invocation();

    let (genres, provided) = match payload.genres {
        Some(raw) => {
            let normalized = normalize_genres(raw);
            if normalized.is_empty() {
                let body = Json(ErrorResponse {
                    error: "genres array must include at least one non-empty value".into(),
                });
                return (StatusCode::BAD_REQUEST, body).into_response();
            }
            (normalized, true)
        }
        None => (state.config().recap_genres().to_vec(), false),
    };

    let job_id = Uuid::new_v4();
    let response_genres = genres.clone();
    let scheduled_genre_count = response_genres.len();
    let job = JobContext::new(job_id, genres);
    let scheduler = state.scheduler().clone();

    tokio::spawn(async move {
        if let Err(error) = scheduler.run_job(job).await {
            error!(%job_id, error = ?error, provided, "manual 7days recap job failed");
        } else {
            info!(%job_id, provided, genres = scheduled_genre_count, "manual 7days recap job scheduled");
        }
    });

    let body = Json(GenerateRecapResponse {
        job_id,
        genres: response_genres,
        status: "accepted",
    });

    (StatusCode::ACCEPTED, body).into_response()
}

fn normalize_genres(raw: Vec<String>) -> Vec<String> {
    let mut seen = HashSet::new();
    let mut result = Vec::new();
    for genre in raw {
        let normalized = genre.trim().to_lowercase();
        if normalized.is_empty() {
            continue;
        }
        if seen.insert(normalized.clone()) {
            result.push(normalized);
        }
    }
    result
}

#[cfg(test)]
mod tests {
    use axum::{body::Body, http::Request, http::StatusCode};
    use tower::ServiceExt;
    use uuid::Uuid;

    use super::normalize_genres;
    use crate::{
        app::{ComponentRegistry, build_router},
        config::{Config, ENV_MUTEX},
    };

    #[test]
    fn normalize_strips_and_deduplicates() {
        let genres = vec![
            " AI ".to_string(),
            "security".to_string(),
            "ai".to_string(),
            String::new(),
        ];
        let normalized = normalize_genres(genres);
        assert_eq!(normalized, vec!["ai".to_string(), "security".to_string()]);
    }

    #[tokio::test]
    async fn trigger_returns_accepted_with_configured_defaults() {
        let config = {
            let _lock = ENV_MUTEX.lock().expect("env mutex");
            unsafe {
                std::env::set_var(
                    "RECAP_DB_DSN",
                    "postgres://recap:recap@localhost:5432/recap",
                );
                std::env::set_var("NEWS_CREATOR_BASE_URL", "http://localhost:18001/");
                std::env::set_var("SUBWORKER_BASE_URL", "http://localhost:18002/");
                std::env::set_var("ALT_BACKEND_BASE_URL", "http://localhost:19000/");
                std::env::set_var("RECAP_GENRES", "ai,space");
                std::env::remove_var("ALT_BACKEND_SERVICE_TOKEN");
                // Set dummy token path for testing (file doesn't need to exist, will fail gracefully)
                std::env::set_var(
                    "HUGGING_FACE_TOKEN_PATH",
                    "/tmp/test-token-which-does-not-exist",
                );
            }
            Config::from_env().expect("config loads")
        };

        let registry = ComponentRegistry::build(config)
            .await
            .expect("registry builds");

        let app = build_router(registry);

        let request = Request::post("/v1/generate/recaps/7days")
            .header("content-type", "application/json")
            .body(Body::from("{}"))
            .expect("request builds");

        let response = app.oneshot(request).await.expect("request succeeds");

        assert_eq!(response.status(), StatusCode::ACCEPTED);

        let body = response.into_body();
        let body_bytes = axum::body::to_bytes(body, usize::MAX)
            .await
            .expect("body bytes");
        let payload: serde_json::Value = serde_json::from_slice(&body_bytes).expect("valid json");

        assert!(
            payload["job_id"]
                .as_str()
                .and_then(|id| Uuid::parse_str(id).ok())
                .is_some()
        );
        let genres = payload["genres"]
            .as_array()
            .expect("genres array")
            .iter()
            .map(|value| value.as_str().expect("genre str").to_string())
            .collect::<Vec<_>>();
        assert_eq!(genres, vec!["ai".to_string(), "space".to_string()]);

        {
            let _lock = ENV_MUTEX.lock().expect("env mutex cleanup");
            unsafe {
                std::env::remove_var("RECAP_DB_DSN");
                std::env::remove_var("NEWS_CREATOR_BASE_URL");
                std::env::remove_var("SUBWORKER_BASE_URL");
                std::env::remove_var("ALT_BACKEND_BASE_URL");
                std::env::remove_var("ALT_BACKEND_SERVICE_TOKEN");
                std::env::remove_var("RECAP_GENRES");
            }
        }
    }
}
