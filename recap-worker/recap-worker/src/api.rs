pub(crate) mod admin;
pub(crate) mod auth;
pub(crate) mod dashboard;
pub(crate) mod evaluation;
pub(crate) mod fetch;
pub(crate) mod generate;
pub(crate) mod health;
pub(crate) mod learning;
pub(crate) mod metrics;
pub(crate) mod pulse;

use axum::{
    Extension, Router, middleware,
    routing::{get, post},
};

use crate::app::AppState;

pub(crate) fn router(state: AppState) -> Router {
    // `/admin/jobs/retry` and `/admin/genre-learning` are the only
    // admin/mutating routes on this surface. `/admin/jobs/retry`'s only
    // caller is ops curl; `/admin/genre-learning`'s only caller is
    // recap-subworker's `LearningClient`, which sends this same bearer via
    // `load_admin_auth_config()` (recap-subworker's
    // `app/infra/admin_auth.py`) — see `api/auth.rs` for why the
    // generate/morning-regenerate routes stay open.
    let admin_guard = auth::AdminAuthGuard::new(state.config().admin_auth_token());
    let protected = Router::new()
        .route("/admin/jobs/retry", post(admin::retry_jobs))
        .route(
            "/admin/genre-learning",
            post(learning::receive_genre_learning),
        )
        .route_layer(middleware::from_fn(auth::require_admin_token))
        .layer(Extension(admin_guard));

    Router::new()
        .route("/health/ready", get(health::ready))
        .route("/health/live", get(health::live))
        .route("/metrics", get(metrics::exporter))
        .merge(protected)
        .route("/v1/generate/recaps/7days", post(generate::trigger_7days))
        .route("/v1/recaps/7days", get(fetch::get_7days_recap))
        .route("/v1/generate/recaps/3days", post(generate::trigger_3days))
        .route("/v1/recaps/3days", get(fetch::get_3days_recap))
        .route("/v1/recaps/search", get(fetch::search_recaps))
        .route(
            "/v1/recaps/genres/indexable",
            get(fetch::get_indexable_genres),
        )
        .route("/v1/morning/updates", get(fetch::get_morning_updates))
        .route(
            "/v1/morning/letters/latest",
            get(fetch::get_latest_morning_letter),
        )
        .route(
            "/v1/morning/letters/regenerate",
            post(fetch::regenerate_morning_letter),
        )
        .route(
            "/v1/morning/letters/{target_date}",
            get(fetch::get_morning_letter_by_date),
        )
        .route(
            "/v1/morning/letters/{letter_id}/sources",
            get(fetch::get_morning_letter_sources),
        )
        .route("/v1/evaluation/genres", post(evaluation::evaluate_genres))
        .route(
            "/v1/evaluation/genres/latest",
            get(evaluation::get_latest_evaluation_result),
        )
        .route(
            "/v1/evaluation/genres/{run_id}",
            get(evaluation::get_evaluation_result),
        )
        .route("/v1/dashboard/metrics", get(dashboard::get_metrics))
        .route("/v1/dashboard/overview", get(dashboard::get_overview))
        .route("/v1/dashboard/logs", get(dashboard::get_logs))
        .route("/v1/dashboard/jobs", get(dashboard::get_jobs))
        .route("/v1/dashboard/recap_jobs", get(dashboard::get_recap_jobs))
        .route(
            "/v1/dashboard/job-progress",
            get(dashboard::get_job_progress),
        )
        .route("/v1/dashboard/job-stats", get(dashboard::get_job_stats))
        .route("/v1/pulse/latest", get(pulse::get_latest))
        .with_state(state)
}

#[cfg(test)]
mod tests {
    use axum::{
        body::Body,
        http::{Method, Request, StatusCode},
    };
    use tower::ServiceExt;

    use super::router;
    use crate::{
        app::{AppState, ComponentRegistry},
        config::{Config, ENV_MUTEX},
    };

    #[tokio::test]
    // Lock held across `ComponentRegistry::build` for the same reason as
    // `api::generate::tests` and `app::tests::component_registry_builds`:
    // `build()` re-reads MTLS/admin env directly from the process, so a
    // concurrent test's env mutation must not leak in.
    #[allow(clippy::await_holding_lock)]
    async fn admin_routes_require_bearer_while_health_and_metrics_do_not() {
        let _lock = ENV_MUTEX.lock().expect("env mutex");
        let temp_dir = tempfile::tempdir().expect("tempdir");
        let token_path = temp_dir.path().join("admin_token");
        std::fs::write(&token_path, "test-router-admin-token-1234567890\n").expect("write token");

        let registry = temp_env::async_with_vars(
            [
                (
                    "RECAP_DB_DSN",
                    Some("postgres://recap:recap@localhost:5432/recap"),
                ),
                ("NEWS_CREATOR_BASE_URL", Some("http://localhost:18021/")),
                ("SUBWORKER_BASE_URL", Some("http://localhost:18022/")),
                ("ALT_BACKEND_BASE_URL", Some("http://localhost:19020/")),
                ("RECAP_KNOWLEDGE_EMIT", Some("false")),
                ("RECAP_ADMIN_AUTH", None),
                (
                    "RECAP_ADMIN_TOKEN_FILE",
                    Some(token_path.to_str().expect("utf8 path")),
                ),
                ("RECAP_EVAL_LISTENER", Some("disabled")),
                ("RECAP_CARDS_JOB", Some("disabled")),
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

        let app = router(AppState::new(registry));

        for (method, path) in [
            (Method::POST, "/admin/jobs/retry"),
            (Method::POST, "/admin/genre-learning"),
        ] {
            let request = Request::builder()
                .method(method)
                .uri(path)
                .body(Body::empty())
                .expect("request builds");
            let response = app
                .clone()
                .oneshot(request)
                .await
                .expect("request succeeds");
            assert_eq!(
                response.status(),
                StatusCode::UNAUTHORIZED,
                "{path} must require the admin bearer"
            );
        }

        for path in ["/health/live", "/metrics"] {
            let request = Request::builder()
                .method(Method::GET)
                .uri(path)
                .body(Body::empty())
                .expect("request builds");
            let response = app
                .clone()
                .oneshot(request)
                .await
                .expect("request succeeds");
            assert_ne!(
                response.status(),
                StatusCode::UNAUTHORIZED,
                "{path} must not require the admin bearer"
            );
        }

        // /health/ready reaches out to subworker/news-creator over the
        // network, so it may legitimately answer non-2xx in this
        // environment; the only invariant under test is that it isn't
        // gated by the admin bearer.
        let request = Request::builder()
            .method(Method::GET)
            .uri("/health/ready")
            .body(Body::empty())
            .expect("request builds");
        let response = app
            .clone()
            .oneshot(request)
            .await
            .expect("request succeeds");
        assert_ne!(
            response.status(),
            StatusCode::UNAUTHORIZED,
            "/health/ready must not require the admin bearer"
        );
    }
}
