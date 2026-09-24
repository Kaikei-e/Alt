use std::collections::HashSet;
use std::sync::Arc;

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
    trigger_recap(state, payload, 7, "7days")
}

pub(crate) async fn trigger_3days(
    State(state): State<AppState>,
    Json(payload): Json<GenerateRecapRequest>,
) -> impl IntoResponse {
    trigger_recap(state, payload, 3, "3days")
}

pub(crate) async fn trigger_3days_cards(
    State(state): State<AppState>,
    body: axum::body::Bytes,
) -> axum::response::Response {
    if !body.is_empty() && serde_json::from_slice::<serde_json::Value>(&body).is_err() {
        let body = Json(ErrorResponse {
            error: "invalid json".into(),
        });
        return (StatusCode::BAD_REQUEST, body).into_response();
    }

    state.telemetry().record_manual_generate_invocation();

    if state.config().cards_user_id().is_none() {
        error!("cards trigger not configured");
        let body = Json(ErrorResponse {
            error: "cards trigger not configured".into(),
        });
        return (StatusCode::SERVICE_UNAVAILABLE, body).into_response();
    }

    // recap-worker runs as a single instance, so an in-process guard is sufficient to prevent overlapping runs.
    let in_flight = state.cards_run_in_flight();
    {
        let lock = in_flight
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        if let Some(running_job_id) = *lock {
            let body = Json(ErrorResponse {
                error: format!("cards job is already running: {running_job_id}"),
            });
            return (StatusCode::CONFLICT, body).into_response();
        }
    }

    match state.dao().find_running_cards_job().await {
        Ok(Some(running_job_id)) => {
            let body = Json(ErrorResponse {
                error: format!("cards job is already running: {running_job_id}"),
            });
            return (StatusCode::CONFLICT, body).into_response();
        }
        Ok(None) => {}
        Err(e) => {
            error!(error = %e, "failed to query running cards job");
            let body = Json(ErrorResponse {
                error: "failed to query running cards job".into(),
            });
            return (StatusCode::INTERNAL_SERVER_ERROR, body).into_response();
        }
    }

    let job_id = Uuid::new_v4();

    {
        let mut lock = in_flight
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        if let Some(running_job_id) = *lock {
            let body = Json(ErrorResponse {
                error: format!("cards job is already running: {running_job_id}"),
            });
            return (StatusCode::CONFLICT, body).into_response();
        }
        *lock = Some(job_id);
    }

    let to = chrono::Utc::now();
    let from = to - chrono::Duration::days(3);
    let runner = state.cards_runner();
    let in_flight_task = Arc::clone(&in_flight);

    tokio::spawn(async move {
        struct CardsInFlightGuard(Arc<std::sync::Mutex<Option<Uuid>>>);
        impl Drop for CardsInFlightGuard {
            fn drop(&mut self) {
                *self
                    .0
                    .lock()
                    .unwrap_or_else(std::sync::PoisonError::into_inner) = None;
            }
        }

        let _guard = CardsInFlightGuard(in_flight_task);
        match runner.run_cards(job_id, from, to).await {
            Ok(()) => info!(%job_id, "cards job completed"),
            Err(err) => error!(%job_id, error = %err, "cards job failed"),
        }
    });

    let body = Json(GenerateRecapResponse {
        job_id,
        genres: vec![],
        status: "accepted",
    });

    (StatusCode::ACCEPTED, body).into_response()
}

fn trigger_recap(
    state: AppState,
    payload: GenerateRecapRequest,
    window_days: u32,
    label: &'static str,
) -> axum::response::Response {
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
    let job = JobContext::new_manual(job_id, genres, window_days);
    let scheduler = state.scheduler().clone();

    tokio::spawn(async move {
        if let Err(error) = scheduler.run_job(job).await {
            error!(%job_id, error = ?error, provided, label, "manual recap job failed");
        } else {
            info!(%job_id, provided, genres = scheduled_genre_count, label, "manual recap job scheduled");
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
    use std::sync::Arc;
    use std::time::Duration;

    use axum::{body::Body, http::Request, http::StatusCode};
    use tower::ServiceExt;
    use uuid::Uuid;

    use super::normalize_genres;
    use crate::{
        app::{ComponentRegistry, build_router},
        config::{Config, ENV_MUTEX},
        pipeline::cards::fakes::FakeCardsJobRunner,
        store::dao::mock::MockRecapDao,
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
    // `ENV_MUTEX` is `std::sync::Mutex` and must stay held across the
    // `ComponentRegistry::build` await point below (see comment): this is a
    // single-threaded `#[tokio::test]`, never contended by another task
    // within the same test, so there is no deadlock risk — only test-suite
    // env-var serialization.
    #[allow(clippy::await_holding_lock)]
    async fn trigger_returns_accepted_with_configured_defaults() {
        // The lock must stay held across `ComponentRegistry::build` too, not
        // just `Config::from_env` — `build` re-reads `MTLS_ENFORCE` /
        // `MTLS_CERT_FILE` etc. directly from the process env via
        // `MtlsPaths::from_env()`. Releasing the lock right after
        // `Config::from_env` left a window where a concurrent test's
        // `MTLS_ENFORCE=true` could leak into this test's `build()` call.
        let _lock = ENV_MUTEX
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        let registry = temp_env::async_with_vars(
            [
                (
                    "RECAP_DB_DSN",
                    Some("postgres://recap:recap@localhost:5432/recap"),
                ),
                ("NEWS_CREATOR_BASE_URL", Some("http://localhost:18001/")),
                ("SUBWORKER_BASE_URL", Some("http://localhost:18002/")),
                ("ALT_BACKEND_BASE_URL", Some("http://localhost:19000/")),
                ("RECAP_KNOWLEDGE_EMIT", Some("false")),
                ("RECAP_ADMIN_AUTH", Some("disabled")),
                ("RECAP_EVAL_LISTENER", Some("disabled")),
                ("RECAP_CARDS_JOB", Some("disabled")),
                ("RECAP_GENRES", Some("ai,space")),
                (
                    "HUGGING_FACE_TOKEN_PATH",
                    Some("/tmp/test-token-which-does-not-exist"),
                ),
                // This test only exercises the HTTP handler shape, not token
                // accuracy, so opt into the degraded tokenizer instead of
                // failing `ComponentRegistry::build` closed (no real HF
                // token is available in the test environment).
                ("TOKEN_COUNTER_ALLOW_DUMMY_FALLBACK", Some("true")),
            ],
            async {
                let config = Config::from_env().expect("config loads");
                ComponentRegistry::build(config)
                    .await
                    .expect("registry builds")
            },
        )
        .await;

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
    }

    #[tokio::test]
    #[allow(clippy::await_holding_lock)]
    async fn trigger_cards_returns_503_when_unconfigured() {
        let _lock = ENV_MUTEX
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        let registry = temp_env::async_with_vars(
            [
                (
                    "RECAP_DB_DSN",
                    Some("postgres://recap:recap@localhost:5432/recap"),
                ),
                ("NEWS_CREATOR_BASE_URL", Some("http://localhost:18001/")),
                ("SUBWORKER_BASE_URL", Some("http://localhost:18002/")),
                ("ALT_BACKEND_BASE_URL", Some("http://localhost:19000/")),
                ("RECAP_KNOWLEDGE_EMIT", Some("false")),
                ("RECAP_ADMIN_AUTH", Some("disabled")),
                ("RECAP_EVAL_LISTENER", Some("disabled")),
                ("RECAP_CARDS_JOB", Some("disabled")),
                ("RECAP_CARDS_USER_ID", None),
                ("RECAP_GENRES", Some("ai,space")),
                (
                    "HUGGING_FACE_TOKEN_PATH",
                    Some("/tmp/test-token-which-does-not-exist"),
                ),
                ("TOKEN_COUNTER_ALLOW_DUMMY_FALLBACK", Some("true")),
            ],
            async {
                let config = Config::from_env().expect("config loads");
                ComponentRegistry::build(config)
                    .await
                    .expect("registry builds")
            },
        )
        .await;

        let app = build_router(registry);
        let request = Request::post("/v1/generate/recaps/3days/cards")
            .header("content-type", "application/json")
            .body(Body::from("{}"))
            .expect("request builds");

        let response = app.oneshot(request).await.expect("request succeeds");
        assert_eq!(response.status(), StatusCode::SERVICE_UNAVAILABLE);

        let body_bytes = axum::body::to_bytes(response.into_body(), usize::MAX)
            .await
            .expect("body bytes");
        let payload: serde_json::Value = serde_json::from_slice(&body_bytes).expect("valid json");
        assert_eq!(payload["error"], "cards trigger not configured");
    }

    #[tokio::test]
    #[allow(clippy::await_holding_lock)]
    async fn trigger_cards_returns_409_when_job_already_running() {
        let _lock = ENV_MUTEX
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        let user_id = Uuid::new_v4();
        let running_job_id = Uuid::new_v4();
        let user_id_str = user_id.to_string();

        let registry = temp_env::async_with_vars(
            [
                (
                    "RECAP_DB_DSN",
                    Some("postgres://recap:recap@localhost:5432/recap"),
                ),
                ("NEWS_CREATOR_BASE_URL", Some("http://localhost:18001/")),
                ("SUBWORKER_BASE_URL", Some("http://localhost:18002/")),
                ("ALT_BACKEND_BASE_URL", Some("http://localhost:19000/")),
                ("RECAP_KNOWLEDGE_EMIT", Some("false")),
                ("RECAP_ADMIN_AUTH", Some("disabled")),
                ("RECAP_EVAL_LISTENER", Some("disabled")),
                ("RECAP_CARDS_JOB", Some("disabled")),
                ("RECAP_CARDS_USER_ID", Some(user_id_str.as_str())),
                ("RECAP_GENRES", Some("ai,space")),
                (
                    "HUGGING_FACE_TOKEN_PATH",
                    Some("/tmp/test-token-which-does-not-exist"),
                ),
                ("TOKEN_COUNTER_ALLOW_DUMMY_FALLBACK", Some("true")),
            ],
            async {
                let config = Config::from_env().expect("config loads");
                ComponentRegistry::build(config)
                    .await
                    .expect("registry builds")
            },
        )
        .await;

        let mock_dao = MockRecapDao::new();
        mock_dao.set_running_cards_job(Some(running_job_id));
        let registry = registry.with_recap_dao(Arc::new(mock_dao));

        let app = build_router(registry);
        let request = Request::post("/v1/generate/recaps/3days/cards")
            .header("content-type", "application/json")
            .body(Body::from("{}"))
            .expect("request builds");

        let response = app.oneshot(request).await.expect("request succeeds");
        assert_eq!(response.status(), StatusCode::CONFLICT);

        let body_bytes = axum::body::to_bytes(response.into_body(), usize::MAX)
            .await
            .expect("body bytes");
        let payload: serde_json::Value = serde_json::from_slice(&body_bytes).expect("valid json");
        assert!(
            payload["error"]
                .as_str()
                .expect("error string")
                .contains(&running_job_id.to_string()),
            "409 error must name running job id"
        );
    }

    #[tokio::test]
    #[allow(clippy::await_holding_lock)]
    async fn trigger_cards_returns_202_and_invokes_runner_when_idle() {
        let _lock = ENV_MUTEX
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        let user_id = Uuid::new_v4();
        let user_id_str = user_id.to_string();

        let registry = temp_env::async_with_vars(
            [
                (
                    "RECAP_DB_DSN",
                    Some("postgres://recap:recap@localhost:5432/recap"),
                ),
                ("NEWS_CREATOR_BASE_URL", Some("http://localhost:18001/")),
                ("SUBWORKER_BASE_URL", Some("http://localhost:18002/")),
                ("ALT_BACKEND_BASE_URL", Some("http://localhost:19000/")),
                ("RECAP_KNOWLEDGE_EMIT", Some("false")),
                ("RECAP_ADMIN_AUTH", Some("disabled")),
                ("RECAP_EVAL_LISTENER", Some("disabled")),
                ("RECAP_CARDS_JOB", Some("disabled")),
                ("RECAP_CARDS_USER_ID", Some(user_id_str.as_str())),
                ("RECAP_GENRES", Some("ai,space")),
                (
                    "HUGGING_FACE_TOKEN_PATH",
                    Some("/tmp/test-token-which-does-not-exist"),
                ),
                ("TOKEN_COUNTER_ALLOW_DUMMY_FALLBACK", Some("true")),
            ],
            async {
                let config = Config::from_env().expect("config loads");
                ComponentRegistry::build(config)
                    .await
                    .expect("registry builds")
            },
        )
        .await;

        let mock_dao = MockRecapDao::new();
        mock_dao.set_running_cards_job(None);
        let fake_runner = Arc::new(FakeCardsJobRunner::new());

        let registry = registry
            .with_recap_dao(Arc::new(mock_dao))
            .with_cards_runner(fake_runner.clone());

        let app = build_router(registry);
        let request = Request::post("/v1/generate/recaps/3days/cards")
            .header("content-type", "application/json")
            .body(Body::from("{}"))
            .expect("request builds");

        let before = chrono::Utc::now();
        let response = app.oneshot(request).await.expect("request succeeds");
        assert_eq!(response.status(), StatusCode::ACCEPTED);

        let body_bytes = axum::body::to_bytes(response.into_body(), usize::MAX)
            .await
            .expect("body bytes");
        let payload: serde_json::Value = serde_json::from_slice(&body_bytes).expect("valid json");

        let job_id = payload["job_id"]
            .as_str()
            .and_then(|id| Uuid::parse_str(id).ok())
            .expect("job_id in response");
        let genres = payload["genres"].as_array().expect("genres array");
        assert!(
            genres.is_empty(),
            "genres should be empty for cards trigger"
        );
        assert_eq!(payload["status"], "accepted");

        tokio::time::timeout(
            Duration::from_secs(1),
            fake_runner.notify_on_start.notified(),
        )
        .await
        .expect("fake runner should be notified within timeout");

        let invocations = fake_runner.invocations();
        assert_eq!(
            invocations.len(),
            1,
            "runner should be invoked exactly once"
        );
        let (invoked_job_id, from, to, trigger_source) = invocations[0];
        assert_eq!(invoked_job_id, job_id);
        assert_eq!(trigger_source, "cards");
        assert!(to >= before);
        let window = to - from;
        assert_eq!(window.num_days(), 3, "cards job window must be 3 days");
    }

    #[tokio::test]
    #[allow(clippy::await_holding_lock)]
    async fn trigger_cards_returns_409_when_run_is_in_flight() {
        let _lock = ENV_MUTEX
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        let user_id = Uuid::new_v4();
        let user_id_str = user_id.to_string();

        let registry = temp_env::async_with_vars(
            [
                (
                    "RECAP_DB_DSN",
                    Some("postgres://recap:recap@localhost:5432/recap"),
                ),
                ("NEWS_CREATOR_BASE_URL", Some("http://localhost:18001/")),
                ("SUBWORKER_BASE_URL", Some("http://localhost:18002/")),
                ("ALT_BACKEND_BASE_URL", Some("http://localhost:19000/")),
                ("RECAP_KNOWLEDGE_EMIT", Some("false")),
                ("RECAP_ADMIN_AUTH", Some("disabled")),
                ("RECAP_EVAL_LISTENER", Some("disabled")),
                ("RECAP_CARDS_JOB", Some("disabled")),
                ("RECAP_CARDS_USER_ID", Some(user_id_str.as_str())),
                ("RECAP_GENRES", Some("ai,space")),
                (
                    "HUGGING_FACE_TOKEN_PATH",
                    Some("/tmp/test-token-which-does-not-exist"),
                ),
                ("TOKEN_COUNTER_ALLOW_DUMMY_FALLBACK", Some("true")),
            ],
            async {
                let config = Config::from_env().expect("config loads");
                ComponentRegistry::build(config)
                    .await
                    .expect("registry builds")
            },
        )
        .await;

        let mock_dao = MockRecapDao::new();
        mock_dao.set_running_cards_job(None);
        let fake_runner = Arc::new(FakeCardsJobRunner::new());
        fake_runner.set_hold(true);

        let registry = registry
            .with_recap_dao(Arc::new(mock_dao))
            .with_cards_runner(fake_runner.clone());

        let app = build_router(registry);

        // First call starts the run, which is held in-flight by the fake runner
        let req1 = Request::post("/v1/generate/recaps/3days/cards")
            .header("content-type", "application/json")
            .body(Body::from("{}"))
            .expect("request builds");
        let resp1 = app.clone().oneshot(req1).await.expect("request 1 succeeds");
        assert_eq!(resp1.status(), StatusCode::ACCEPTED);

        let body_bytes1 = axum::body::to_bytes(resp1.into_body(), usize::MAX)
            .await
            .expect("body bytes");
        let payload1: serde_json::Value = serde_json::from_slice(&body_bytes1).expect("valid json");
        let first_job_id = payload1["job_id"].as_str().expect("job_id str").to_string();

        // Wait until runner confirms it started the first run
        tokio::time::timeout(
            Duration::from_secs(1),
            fake_runner.notify_on_start.notified(),
        )
        .await
        .expect("first run started");

        // Second call while first is still pending must receive 409 naming the in-flight job
        let req2 = Request::post("/v1/generate/recaps/3days/cards")
            .header("content-type", "application/json")
            .body(Body::from("{}"))
            .expect("request builds");
        let resp2 = app.clone().oneshot(req2).await.expect("request 2 succeeds");
        assert_eq!(resp2.status(), StatusCode::CONFLICT);

        let body_bytes2 = axum::body::to_bytes(resp2.into_body(), usize::MAX)
            .await
            .expect("body bytes");
        let payload2: serde_json::Value = serde_json::from_slice(&body_bytes2).expect("valid json");
        assert!(
            payload2["error"]
                .as_str()
                .expect("error str")
                .contains(&first_job_id),
            "409 response must name the in-flight job_id"
        );

        // Release the first run and wait for it to complete
        fake_runner.set_hold(false);
        fake_runner.release();
        tokio::time::timeout(
            Duration::from_secs(1),
            fake_runner.notify_on_finish.notified(),
        )
        .await
        .expect("first run finished");

        // Third call after completion must return 202 again
        let req3 = Request::post("/v1/generate/recaps/3days/cards")
            .header("content-type", "application/json")
            .body(Body::from("{}"))
            .expect("request builds");
        let resp3 = app.oneshot(req3).await.expect("request 3 succeeds");
        assert_eq!(resp3.status(), StatusCode::ACCEPTED);
    }
}
