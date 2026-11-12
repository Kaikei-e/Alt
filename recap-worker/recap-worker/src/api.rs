pub(crate) mod admin;
pub(crate) mod auth;
pub mod cards;
pub(crate) mod dashboard;
pub(crate) mod evaluation;
pub mod fetch;
pub(crate) mod generate;
pub(crate) mod health;
pub(crate) mod learning;
pub(crate) mod metrics;
pub mod pulse;
pub mod surface_status;

use axum::{
    Extension, Router, middleware,
    routing::{get, post},
};

use crate::app::AppState;

macro_rules! with_non_health_routes {
    ($macro:ident) => {
        $macro! {
            admin: [
                (POST, post, "/admin/jobs/retry", "/admin/jobs/retry", admin::retry_jobs),
                (POST, post, "/admin/genre-learning", "/admin/genre-learning", learning::receive_genre_learning),
            ],
            v1: [
                (POST, post, "/v1/generate/recaps/7days", "/v1/generate/recaps/7days", generate::trigger_7days),
                (GET, get, "/v1/recaps/7days", "/v1/recaps/7days", fetch::get_7days_recap),
                (POST, post, "/v1/generate/recaps/3days", "/v1/generate/recaps/3days", generate::trigger_3days),
                (POST, post, "/v1/generate/topic-cards", "/v1/generate/topic-cards", generate::trigger_topic_cards),
                (GET, get, "/v1/recaps/3days", "/v1/recaps/3days", fetch::get_3days_recap),
                (GET, get, "/v1/topic-cards", "/v1/topic-cards", cards::get_topic_cards),
                (GET, get, "/v1/recaps/search", "/v1/recaps/search?q=ai", fetch::search_recaps),
                (GET, get, "/v1/recaps/genres/indexable", "/v1/recaps/genres/indexable", fetch::get_indexable_genres),
                (GET, get, "/v1/morning/updates", "/v1/morning/updates?since=2026-01-26T00:00:00Z", fetch::get_morning_updates),
                (GET, get, "/v1/morning/letters/latest", "/v1/morning/letters/latest", fetch::get_latest_morning_letter),
                (POST, post, "/v1/morning/letters/regenerate", "/v1/morning/letters/regenerate", fetch::regenerate_morning_letter),
                (GET, get, "/v1/morning/letters/{target_date}", "/v1/morning/letters/2026-01-26", fetch::get_morning_letter_by_date),
                (GET, get, "/v1/morning/letters/{letter_id}/sources", "/v1/morning/letters/00000000-0000-0000-0000-000000000001/sources", fetch::get_morning_letter_sources),
                (POST, post, "/v1/evaluation/genres", "/v1/evaluation/genres", evaluation::evaluate_genres),
                (GET, get, "/v1/evaluation/genres/latest", "/v1/evaluation/genres/latest", evaluation::get_latest_evaluation_result),
                (GET, get, "/v1/evaluation/genres/{run_id}", "/v1/evaluation/genres/00000000-0000-0000-0000-000000000001", evaluation::get_evaluation_result),
                (GET, get, "/v1/dashboard/metrics", "/v1/dashboard/metrics", dashboard::get_metrics),
                (GET, get, "/v1/dashboard/overview", "/v1/dashboard/overview", dashboard::get_overview),
                (GET, get, "/v1/dashboard/logs", "/v1/dashboard/logs", dashboard::get_logs),
                (GET, get, "/v1/dashboard/jobs", "/v1/dashboard/jobs", dashboard::get_jobs),
                (GET, get, "/v1/dashboard/recap_jobs", "/v1/dashboard/recap_jobs", dashboard::get_recap_jobs),
                (GET, get, "/v1/dashboard/job-progress", "/v1/dashboard/job-progress", dashboard::get_job_progress),
                (GET, get, "/v1/dashboard/job-stats", "/v1/dashboard/job-stats", dashboard::get_job_stats),
                (GET, get, "/v1/pulse/latest", "/v1/pulse/latest", pulse::get_latest),
            ]
        }
    };
}

macro_rules! build_peer_authenticated_subrouters {
    (
        admin: [ $( ($a_method:ident, $a_verb:ident, $a_route:literal, $a_sample:literal, $a_handler:path) ),* $(,)? ],
        v1: [ $( ($v_method:ident, $v_verb:ident, $v_route:literal, $v_sample:literal, $v_handler:path) ),* $(,)? ]
    ) => {{
        let admin_router = Router::new()
            $( .route($a_route, $a_verb($a_handler)) )*;
        let v1_router = Router::new()
            $( .route($v_route, $v_verb($v_handler)) )*;
        (admin_router, v1_router)
    }};
}

pub(crate) fn router(state: AppState) -> Router {
    let admin_guard = auth::AdminAuthGuard::new(state.config().admin_auth_token());
    let peer_guard = auth::PeerIdentityGuard::new(state.config().peer_identity().allowed_peers());
    let (admin_router, v1_router) = with_non_health_routes!(build_peer_authenticated_subrouters);
    let protected = admin_router
        .route_layer(middleware::from_fn(auth::require_admin_token))
        .layer(Extension(admin_guard));

    Router::new()
        .route("/health/ready", get(health::ready))
        .route("/health/live", get(health::live))
        .route("/metrics", get(metrics::exporter))
        .merge(v1_router)
        .merge(protected)
        .layer(middleware::from_fn(auth::require_peer_identity))
        .layer(Extension(peer_guard))
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

    macro_rules! non_health_route_samples {
        (
            admin: [ $( ($a_method:ident, $a_verb:ident, $a_route:literal, $a_sample:literal, $a_handler:path) ),* $(,)? ],
            v1: [ $( ($v_method:ident, $v_verb:ident, $v_route:literal, $v_sample:literal, $v_handler:path) ),* $(,)? ]
        ) => {
            &[
                $( (Method::$a_method, $a_sample), )*
                $( (Method::$v_method, $v_sample), )*
            ]
        };
    }

    const NON_HEALTH_ROUTES: &[(Method, &str)] = with_non_health_routes!(non_health_route_samples);

    #[tokio::test]
    // Lock held across `ComponentRegistry::build` for the same reason as
    // `api::generate::tests` and `app::tests::component_registry_builds`:
    // `build()` re-reads MTLS/admin env directly from the process, so a
    // concurrent test's env mutation must not leak in.
    #[allow(clippy::await_holding_lock)]
    async fn admin_routes_require_bearer_while_health_and_metrics_do_not() {
        let _lock = ENV_MUTEX
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
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
                ("MTLS_ALLOWED_PEERS", Some("disabled")),
                ("RECAP_EVAL_LISTENER", Some("disabled")),
                ("RECAP_CARDS_JOB", Some("disabled")),
                ("RECAP_FRESHNESS_THREE_DAY_RECAP_HOURS", Some("30")),
                ("RECAP_FRESHNESS_TOPIC_CARDS_HOURS", Some("30")),
                ("RECAP_FRESHNESS_EVENING_PULSE_HOURS", Some("30")),
                ("RECAP_FRESHNESS_MORNING_LETTER_HOURS", Some("3")),
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

    #[tokio::test]
    #[allow(clippy::await_holding_lock, clippy::too_many_lines)]
    async fn topic_cards_routes_registered_and_old_routes_return_404() {
        let _lock = ENV_MUTEX
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
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
                ("RECAP_ADMIN_AUTH", Some("disabled")),
                ("MTLS_ALLOWED_PEERS", Some("disabled")),
                ("RECAP_EVAL_LISTENER", Some("disabled")),
                ("RECAP_CARDS_JOB", Some("disabled")),
                ("RECAP_FRESHNESS_THREE_DAY_RECAP_HOURS", Some("30")),
                ("RECAP_FRESHNESS_TOPIC_CARDS_HOURS", Some("30")),
                ("RECAP_FRESHNESS_EVENING_PULSE_HOURS", Some("30")),
                ("RECAP_FRESHNESS_MORNING_LETTER_HOURS", Some("3")),
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
            (Method::GET, "/v1/recaps/3days/cards"),
            (Method::POST, "/v1/generate/recaps/3days/cards"),
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
                StatusCode::NOT_FOUND,
                "old route {path} must return 404"
            );
        }

        let get_req = Request::builder()
            .method(Method::GET)
            .uri("/v1/topic-cards")
            .body(Body::empty())
            .expect("request builds");
        let get_resp = app
            .clone()
            .oneshot(get_req)
            .await
            .expect("request succeeds");
        assert_ne!(
            get_resp.status(),
            StatusCode::NOT_FOUND,
            "/v1/topic-cards must be registered"
        );

        let post_req = Request::builder()
            .method(Method::POST)
            .uri("/v1/generate/topic-cards")
            .body(Body::empty())
            .expect("request builds");
        let post_resp = app
            .clone()
            .oneshot(post_req)
            .await
            .expect("request succeeds");
        assert_ne!(
            post_resp.status(),
            StatusCode::NOT_FOUND,
            "/v1/generate/topic-cards must be registered"
        );

        assert_eq!(NON_HEALTH_ROUTES.len(), 26);
        for (_method, uri) in NON_HEALTH_ROUTES {
            let req = Request::builder()
                .method(Method::DELETE)
                .uri(*uri)
                .body(Body::empty())
                .expect("request builds");
            let resp = app.clone().oneshot(req).await.expect("request succeeds");
            assert_eq!(
                resp.status(),
                StatusCode::METHOD_NOT_ALLOWED,
                "route {uri} must be registered on the router (405 on DELETE, never 404)"
            );
        }
    }

    fn test_peer_cert(name: &str) -> rustls::pki_types::CertificateDer<'static> {
        let params = rcgen::CertificateParams::new(vec![name.to_string()]).expect("cert params");
        let key = rcgen::KeyPair::generate().expect("keypair");
        let cert = params.self_signed(&key).expect("self-signed cert");
        cert.der().clone()
    }

    fn write_test_mtls_files(dir: &std::path::Path) -> (String, String, String) {
        let _ = rustls::crypto::aws_lc_rs::default_provider().install_default();
        let ca_params = rcgen::CertificateParams::new(vec!["alt-ca".to_string()]).expect("ca");
        let ca_key = rcgen::KeyPair::generate().expect("ca key");
        let ca_cert = ca_params.self_signed(&ca_key).expect("ca cert");

        let leaf_params =
            rcgen::CertificateParams::new(vec!["recap-worker".to_string()]).expect("leaf");
        let leaf_key = rcgen::KeyPair::generate().expect("leaf key");
        let leaf_cert = leaf_params.self_signed(&leaf_key).expect("leaf cert");

        let cert_path = dir.join("cert.pem");
        let key_path = dir.join("key.pem");
        let ca_path = dir.join("ca.pem");
        std::fs::write(&cert_path, leaf_cert.pem()).expect("write cert");
        std::fs::write(&key_path, leaf_key.serialize_pem()).expect("write key");
        std::fs::write(&ca_path, ca_cert.pem()).expect("write ca");
        (
            cert_path.to_str().expect("utf8").to_string(),
            key_path.to_str().expect("utf8").to_string(),
            ca_path.to_str().expect("utf8").to_string(),
        )
    }

    #[tokio::test]
    #[allow(clippy::await_holding_lock)]
    async fn disabled_peer_list_lets_guarded_route_reach_handler_without_certificate() {
        let _lock = ENV_MUTEX
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
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
                ("RECAP_ADMIN_AUTH", Some("disabled")),
                ("MTLS_ENFORCE", None),
                ("MTLS_ALLOWED_PEERS", Some("disabled")),
                ("RECAP_EVAL_LISTENER", Some("disabled")),
                ("RECAP_CARDS_JOB", Some("disabled")),
                ("RECAP_FRESHNESS_THREE_DAY_RECAP_HOURS", Some("30")),
                ("RECAP_FRESHNESS_TOPIC_CARDS_HOURS", Some("30")),
                ("RECAP_FRESHNESS_EVENING_PULSE_HOURS", Some("30")),
                ("RECAP_FRESHNESS_MORNING_LETTER_HOURS", Some("3")),
                ("RECAP_GENRES", Some("ai,space")),
                ("RECAP_PRE_REFRESH_GRAPH_ENABLED", Some("false")),
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
        let request = Request::builder()
            .method(Method::POST)
            .uri("/v1/generate/recaps/7days")
            .header("content-type", "application/json")
            .body(Body::from("{}"))
            .expect("request builds");
        let response = app.oneshot(request).await.expect("request succeeds");
        assert_eq!(
            response.status(),
            StatusCode::ACCEPTED,
            "MTLS_ALLOWED_PEERS=disabled must let a certificate-less request reach the handler"
        );
    }

    #[tokio::test]
    #[allow(clippy::await_holding_lock, clippy::too_many_lines)]
    async fn non_health_routes_require_verified_allowed_peer_while_health_and_metrics_stay_open() {
        let _lock = ENV_MUTEX
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        let mtls_dir = tempfile::tempdir().expect("tempdir");
        let (cert_path, key_path, ca_path) = write_test_mtls_files(mtls_dir.path());
        let registry = temp_env::async_with_vars(
            [
                (
                    "RECAP_DB_DSN",
                    Some("postgres://recap:recap@localhost:5432/recap"),
                ),
                ("MTLS_ENFORCE", Some("true")),
                ("MTLS_CERT_FILE", Some(cert_path.as_str())),
                ("MTLS_KEY_FILE", Some(key_path.as_str())),
                ("MTLS_CA_FILE", Some(ca_path.as_str())),
                ("NEWS_CREATOR_BASE_URL", Some("https://localhost:18021/")),
                ("SUBWORKER_BASE_URL", Some("https://localhost:18022/")),
                ("ALT_BACKEND_BASE_URL", Some("http://localhost:19020/")),
                ("RECAP_KNOWLEDGE_EMIT", Some("false")),
                ("RECAP_ADMIN_AUTH", Some("disabled")),
                (
                    "MTLS_ALLOWED_PEERS",
                    Some("alt-backend,alt-frontend-sv,recap-evaluator,recap-subworker"),
                ),
                ("RECAP_EVAL_LISTENER", Some("disabled")),
                ("RECAP_CARDS_JOB", Some("disabled")),
                ("RECAP_FRESHNESS_THREE_DAY_RECAP_HOURS", Some("30")),
                ("RECAP_FRESHNESS_TOPIC_CARDS_HOURS", Some("30")),
                ("RECAP_FRESHNESS_EVENING_PULSE_HOURS", Some("30")),
                ("RECAP_FRESHNESS_MORNING_LETTER_HOURS", Some("3")),
                ("RECAP_GENRES", Some("ai,space")),
                ("RECAP_PRE_REFRESH_GRAPH_ENABLED", Some("false")),
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

        for (method, path) in NON_HEALTH_ROUTES {
            let unauthenticated = Request::builder()
                .method(method.clone())
                .uri(*path)
                .header("content-type", "application/json")
                .header("x-alt-peer-identity", "alt-frontend-sv")
                .body(Body::from("{}"))
                .expect("request builds");
            let response = app
                .clone()
                .oneshot(unauthenticated)
                .await
                .expect("request succeeds");
            assert_eq!(
                response.status(),
                StatusCode::UNAUTHORIZED,
                "{path} without verified peer certificate must return 401"
            );

            let mut untrusted = Request::builder()
                .method(method.clone())
                .uri(*path)
                .header("content-type", "application/json")
                .body(Body::from("{}"))
                .expect("request builds");
            untrusted
                .extensions_mut()
                .insert(test_peer_cert("untrusted-peer"));
            let forbidden = app
                .clone()
                .oneshot(untrusted)
                .await
                .expect("request succeeds");
            assert_eq!(
                forbidden.status(),
                StatusCode::FORBIDDEN,
                "{path} with non-allowlisted peer certificate must return 403"
            );
        }

        for (method, path) in [
            (Method::GET, "/v1/not-a-registered-route"),
            (Method::GET, "/"),
            (Method::DELETE, "/v1/topic-cards"),
            (Method::POST, "/v1/recaps/7days"),
        ] {
            let request = Request::builder()
                .method(method.clone())
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
                "{method} {path} without a client certificate must return 401, not 404/405"
            );
        }

        let mut allowed_generate = Request::builder()
            .method(Method::POST)
            .uri("/v1/generate/recaps/7days")
            .header("content-type", "application/json")
            .body(Body::from("{}"))
            .expect("request builds");
        allowed_generate
            .extensions_mut()
            .insert(test_peer_cert("alt-frontend-sv"));
        let allowed_resp = app
            .clone()
            .oneshot(allowed_generate)
            .await
            .expect("request succeeds");
        assert_eq!(
            allowed_resp.status(),
            StatusCode::ACCEPTED,
            "allowlisted peer must reach /v1/generate/recaps/7days"
        );

        for path in ["/health/live", "/health/ready", "/metrics"] {
            let req = Request::builder()
                .method(Method::GET)
                .uri(path)
                .body(Body::empty())
                .expect("request builds");
            let resp = app.clone().oneshot(req).await.expect("request succeeds");
            assert_ne!(
                resp.status(),
                StatusCode::UNAUTHORIZED,
                "{path} must stay open without peer identity"
            );
            assert_ne!(
                resp.status(),
                StatusCode::FORBIDDEN,
                "{path} must stay open without peer identity"
            );
        }
    }
}
