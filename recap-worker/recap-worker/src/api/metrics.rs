use axum::{extract::State, http::StatusCode, response::IntoResponse};

use crate::app::AppState;

pub(crate) async fn exporter(State(state): State<AppState>) -> impl IntoResponse {
    (StatusCode::OK, state.telemetry().render_prometheus()).into_response()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{
        app::ComponentRegistry,
        config::{Config, ENV_MUTEX},
    };

    #[tokio::test]
    #[allow(clippy::await_holding_lock)]
    async fn metrics_endpoint_renders_surface_health_gauges_for_all_surfaces() {
        let env_lock = ENV_MUTEX
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        let registry = temp_env::async_with_vars(
            [
                (
                    "RECAP_DB_DSN",
                    Some("postgres://recap:recap@localhost:5432/recap"),
                ),
                ("CLASSIFICATION_QUEUE_CONCURRENCY", Some("0")),
                ("NEWS_CREATOR_BASE_URL", Some("http://localhost:18001/")),
                ("SUBWORKER_BASE_URL", Some("http://localhost:18002/")),
                ("ALT_BACKEND_BASE_URL", Some("http://localhost:19000/")),
                ("RECAP_KNOWLEDGE_EMIT", Some("false")),
                ("RECAP_ADMIN_AUTH", Some("disabled")),
                ("MTLS_ALLOWED_PEERS", Some("disabled")),
                ("RECAP_EVAL_LISTENER", Some("disabled")),
                ("RECAP_CARDS_JOB", Some("disabled")),
                ("RECAP_FRESHNESS_THREE_DAY_RECAP_HOURS", Some("30")),
                ("RECAP_FRESHNESS_TOPIC_CARDS_HOURS", Some("30")),
                ("RECAP_FRESHNESS_EVENING_PULSE_HOURS", Some("30")),
                ("RECAP_FRESHNESS_MORNING_LETTER_HOURS", Some("3")),
                ("RECAP_PRE_REFRESH_GRAPH_ENABLED", Some("false")),
                ("RECAP_GENRES", Some("ai")),
                (
                    "HUGGING_FACE_TOKEN_PATH",
                    Some("/tmp/test-token-which-does-not-exist"),
                ),
                ("TOKEN_COUNTER_ALLOW_DUMMY_FALLBACK", Some("true")),
                ("MTLS_ENFORCE", None),
            ],
            async {
                let config = Config::from_env().expect("config loads");
                ComponentRegistry::build(config)
                    .await
                    .expect("registry builds")
            },
        )
        .await;
        drop(env_lock);

        let app_state = AppState::new(registry);
        let resp = exporter(State(app_state)).await.into_response();
        assert_eq!(resp.status(), StatusCode::OK);
        let bytes = axum::body::to_bytes(resp.into_body(), 1024 * 1024)
            .await
            .expect("body bytes");
        let text = String::from_utf8(bytes.to_vec()).expect("utf8");

        assert!(text.contains("# TYPE recap_surface_last_success_timestamp_seconds gauge"));
        assert!(text.contains("# TYPE recap_surface_consecutive_failures gauge"));
        for surface in [
            "three_day_recap",
            "topic_cards",
            "evening_pulse",
            "morning_letter",
        ] {
            assert!(
                text.contains(&format!(
                    "recap_surface_last_success_timestamp_seconds{{surface=\"{surface}\"}}"
                )),
                "missing last_success series for {surface}"
            );
            assert!(
                text.contains(&format!(
                    "recap_surface_consecutive_failures{{surface=\"{surface}\"}}"
                )),
                "missing consecutive_failures series for {surface}"
            );
        }
    }
}
