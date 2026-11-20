use axum::{Json, extract::State, http::StatusCode, response::IntoResponse};
use serde::{Deserialize, Serialize};
use tracing::{error, info};

use crate::app::AppState;

#[derive(Debug, Deserialize)]
pub(crate) struct GenreLearningRequest {
    summary: LearningSummary,
    graph_override: Option<GraphOverridePayload>,
    metadata: Option<serde_json::Value>,
}

#[derive(Debug, Deserialize)]
struct LearningSummary {
    graph_margin_reference: Option<f32>,
    boost_threshold_reference: Option<f32>,
    tag_count_threshold_reference: Option<i32>,
    total_records: Option<i64>,
    accuracy_estimate: Option<f64>,
}

#[derive(Debug, Deserialize)]
struct GraphOverridePayload {
    graph_margin: Option<f32>,
    weighted_tie_break_margin: Option<f32>,
    tag_confidence_gate: Option<f32>,
    boost_threshold: Option<f32>,
    tag_count_threshold: Option<usize>,
}

#[derive(Debug, Serialize)]
pub(crate) struct GenreLearningResponse {
    status: String,
    config_saved: bool,
    message: String,
}

fn f32_to_json_number(val: f32) -> serde_json::Value {
    serde_json::Value::Number(
        serde_json::Number::from_f64(f64::from(val)).unwrap_or_else(|| 0.into()),
    )
}

fn build_config_from_override(
    graph_override: &GraphOverridePayload,
    config_payload: &mut serde_json::Map<String, serde_json::Value>,
) {
    if let Some(graph_margin) = graph_override.graph_margin {
        config_payload.insert("graph_margin".to_string(), f32_to_json_number(graph_margin));
    }
    if let Some(weighted_tie_break_margin) = graph_override.weighted_tie_break_margin {
        config_payload.insert(
            "weighted_tie_break_margin".to_string(),
            f32_to_json_number(weighted_tie_break_margin),
        );
    }
    if let Some(tag_confidence_gate) = graph_override.tag_confidence_gate {
        config_payload.insert(
            "tag_confidence_gate".to_string(),
            f32_to_json_number(tag_confidence_gate),
        );
    }
    if let Some(boost_threshold) = graph_override.boost_threshold {
        config_payload.insert(
            "boost_threshold".to_string(),
            f32_to_json_number(boost_threshold),
        );
    }
    if let Some(tag_count_threshold) = graph_override.tag_count_threshold {
        config_payload.insert(
            "tag_count_threshold".to_string(),
            serde_json::Value::Number(tag_count_threshold.into()),
        );
    }
}

fn apply_summary_fallback(
    summary: &LearningSummary,
    config_payload: &mut serde_json::Map<String, serde_json::Value>,
) {
    if let Some(graph_margin) = summary.graph_margin_reference {
        config_payload.insert("graph_margin".to_string(), f32_to_json_number(graph_margin));
    }
    if let Some(boost_threshold) = summary.boost_threshold_reference {
        config_payload.insert(
            "boost_threshold".to_string(),
            f32_to_json_number(boost_threshold),
        );
    }
    if let Some(tag_count_threshold) = summary.tag_count_threshold_reference {
        config_payload.insert(
            "tag_count_threshold".to_string(),
            serde_json::Value::Number(tag_count_threshold.into()),
        );
    }
}

fn build_learning_config_payload(
    payload: &GenreLearningRequest,
) -> serde_json::Map<String, serde_json::Value> {
    let mut config_payload = serde_json::Map::new();

    if let Some(ref graph_override) = payload.graph_override {
        build_config_from_override(graph_override, &mut config_payload);
    }

    // Fallback to summary values if graph_override is missing
    if config_payload.is_empty() {
        apply_summary_fallback(&payload.summary, &mut config_payload);
    }

    config_payload
}

async fn persist_genre_learning_config(
    state: &AppState,
    config_json: &serde_json::Value,
    metadata: &serde_json::Value,
) -> axum::response::Response {
    match state
        .dao()
        .insert_worker_config(
            "graph_override",
            config_json,
            "genre_learning",
            Some(metadata),
        )
        .await
    {
        Ok(()) => {
            info!("saved genre learning config to database");
            (
                StatusCode::OK,
                Json(GenreLearningResponse {
                    status: "success".to_string(),
                    config_saved: true,
                    message: "configuration saved successfully".to_string(),
                }),
            )
                .into_response()
        }
        Err(e) => {
            error!(error = %e, "failed to save genre learning config");
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(GenreLearningResponse {
                    status: "error".to_string(),
                    config_saved: false,
                    message: format!("failed to save config: {e}"),
                }),
            )
                .into_response()
        }
    }
}

pub(crate) async fn receive_genre_learning(
    State(state): State<AppState>,
    Json(payload): Json<GenreLearningRequest>,
) -> impl IntoResponse {
    info!("received genre learning payload from recap-subworker");

    // Build config payload from request
    let config_payload = build_learning_config_payload(&payload);

    if config_payload.is_empty() {
        return (
            StatusCode::BAD_REQUEST,
            Json(GenreLearningResponse {
                status: "error".to_string(),
                config_saved: false,
                message: "no configuration values provided".to_string(),
            }),
        )
            .into_response();
    }

    // Save to database
    let config_json = serde_json::Value::Object(config_payload);
    let metadata = payload.metadata.clone().unwrap_or_else(|| {
        serde_json::json!({
            "accuracy_estimate": payload.summary.accuracy_estimate,
            "total_records": payload.summary.total_records,
        })
    });

    persist_genre_learning_config(&state, &config_json, &metadata).await
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Arc;

    #[allow(clippy::await_holding_lock)]
    async fn build_learning_test_state(
        mock_dao: Arc<crate::store::dao::mock::MockRecapDao>,
    ) -> AppState {
        let env_lock = crate::config::ENV_MUTEX
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        let registry = temp_env::async_with_vars(
            [
                (
                    "RECAP_DB_DSN",
                    Some("postgres://recap:recap@127.0.0.1:15432/recap"),
                ),
                ("CLASSIFICATION_QUEUE_CONCURRENCY", Some("0")),
                ("NEWS_CREATOR_BASE_URL", Some("http://127.0.0.1:18001/")),
                ("SUBWORKER_BASE_URL", Some("http://127.0.0.1:18002/")),
                ("ALT_BACKEND_BASE_URL", Some("http://127.0.0.1:19000/")),
                ("RECAP_KNOWLEDGE_EMIT", Some("false")),
                ("RECAP_ADMIN_AUTH", Some("disabled")),
                ("MTLS_ALLOWED_PEERS", Some("disabled")),
                ("RECAP_EVAL_LISTENER", Some("disabled")),
                ("RECAP_CARDS_JOB", Some("disabled")),
                ("RECAP_THEMES_ENABLED", Some("false")),
                ("RECAP_FRESHNESS_THREE_DAY_RECAP_HOURS", Some("30")),
                ("RECAP_FRESHNESS_TOPIC_CARDS_HOURS", Some("30")),
                ("RECAP_FRESHNESS_MORNING_LETTER_HOURS", Some("3")),
                ("RECAP_PRE_REFRESH_GRAPH_ENABLED", Some("false")),
                ("RECAP_GENRES", Some("ai,space")),
                ("TOKEN_COUNTER_ALLOW_DUMMY_FALLBACK", Some("true")),
                ("MTLS_ENFORCE", None),
            ],
            async {
                let config = crate::config::Config::from_env().expect("config loads");
                crate::app::ComponentRegistry::build(config)
                    .await
                    .expect("registry builds")
                    .with_recap_dao(mock_dao)
            },
        )
        .await;
        drop(env_lock);
        AppState::new(registry)
    }

    async fn read_response_json(response: axum::response::Response) -> serde_json::Value {
        let bytes = axum::body::to_bytes(response.into_body(), usize::MAX)
            .await
            .expect("body bytes");
        serde_json::from_slice(&bytes).expect("valid json response")
    }

    #[tokio::test]
    async fn receive_genre_learning_returns_400_when_no_values_provided() {
        let mock_dao = Arc::new(crate::store::dao::mock::MockRecapDao::new());
        let state = build_learning_test_state(Arc::clone(&mock_dao)).await;
        let payload: GenreLearningRequest = serde_json::from_value(serde_json::json!({
            "summary": {},
            "graph_override": null,
            "metadata": null
        }))
        .expect("valid request payload");

        let response = receive_genre_learning(State(state), Json(payload))
            .await
            .into_response();

        assert_eq!(response.status(), StatusCode::BAD_REQUEST);
        let body = read_response_json(response).await;
        assert_eq!(
            body,
            serde_json::json!({
                "status": "error",
                "config_saved": false,
                "message": "no configuration values provided"
            })
        );
        assert!(mock_dao.worker_configs().is_empty());
    }

    #[tokio::test]
    async fn receive_genre_learning_prefers_graph_override_and_custom_metadata() {
        let mock_dao = Arc::new(crate::store::dao::mock::MockRecapDao::new());
        let state = build_learning_test_state(Arc::clone(&mock_dao)).await;
        let payload: GenreLearningRequest = serde_json::from_value(serde_json::json!({
            "summary": {
                "graph_margin_reference": 0.5,
                "boost_threshold_reference": 0.5,
                "tag_count_threshold_reference": 99,
                "total_records": 50,
                "accuracy_estimate": 0.75
            },
            "graph_override": {
                "graph_margin": 0.25,
                "weighted_tie_break_margin": 0.125,
                "tag_confidence_gate": 0.5,
                "boost_threshold": 0.75,
                "tag_count_threshold": 3
            },
            "metadata": {
                "run": "custom-1"
            }
        }))
        .expect("valid request payload");

        let response = receive_genre_learning(State(state), Json(payload))
            .await
            .into_response();

        assert_eq!(response.status(), StatusCode::OK);
        let body = read_response_json(response).await;
        assert_eq!(
            body,
            serde_json::json!({
                "status": "success",
                "config_saved": true,
                "message": "configuration saved successfully"
            })
        );

        let saved = mock_dao.worker_configs();
        assert_eq!(saved.len(), 1);
        assert_eq!(saved[0].0, "graph_override");
        assert_eq!(
            saved[0].1,
            serde_json::json!({
                "graph_margin": 0.25,
                "weighted_tie_break_margin": 0.125,
                "tag_confidence_gate": 0.5,
                "boost_threshold": 0.75,
                "tag_count_threshold": 3
            })
        );
        assert_eq!(saved[0].2, "genre_learning");
        assert_eq!(saved[0].3, Some(serde_json::json!({"run": "custom-1"})));
    }

    #[tokio::test]
    async fn receive_genre_learning_falls_back_to_summary_and_synthesizes_metadata() {
        let mock_dao = Arc::new(crate::store::dao::mock::MockRecapDao::new());
        let state = build_learning_test_state(Arc::clone(&mock_dao)).await;
        let payload: GenreLearningRequest = serde_json::from_value(serde_json::json!({
            "summary": {
                "graph_margin_reference": 0.25,
                "boost_threshold_reference": 0.5,
                "tag_count_threshold_reference": 4,
                "total_records": 100,
                "accuracy_estimate": 0.875
            },
            "graph_override": {
                "graph_margin": null,
                "weighted_tie_break_margin": null,
                "tag_confidence_gate": null,
                "boost_threshold": null,
                "tag_count_threshold": null
            },
            "metadata": null
        }))
        .expect("valid request payload");

        let response = receive_genre_learning(State(state), Json(payload))
            .await
            .into_response();

        assert_eq!(response.status(), StatusCode::OK);
        let body = read_response_json(response).await;
        assert_eq!(body["status"], "success");
        assert_eq!(body["config_saved"], true);

        let saved = mock_dao.worker_configs();
        assert_eq!(saved.len(), 1);
        assert_eq!(saved[0].0, "graph_override");
        assert_eq!(
            saved[0].1,
            serde_json::json!({
                "graph_margin": 0.25,
                "boost_threshold": 0.5,
                "tag_count_threshold": 4
            })
        );
        assert_eq!(saved[0].2, "genre_learning");
        assert_eq!(
            saved[0].3,
            Some(serde_json::json!({
                "accuracy_estimate": 0.875,
                "total_records": 100
            }))
        );
    }

    #[tokio::test]
    async fn receive_genre_learning_returns_500_when_dao_insert_fails() {
        let mock_dao = Arc::new(crate::store::dao::mock::MockRecapDao::new());
        mock_dao.set_insert_worker_config_error("simulated insert failure");
        let state = build_learning_test_state(Arc::clone(&mock_dao)).await;
        let payload: GenreLearningRequest = serde_json::from_value(serde_json::json!({
            "summary": {},
            "graph_override": {
                "graph_margin": 0.25
            }
        }))
        .expect("valid request payload");

        let response = receive_genre_learning(State(state), Json(payload))
            .await
            .into_response();

        assert_eq!(response.status(), StatusCode::INTERNAL_SERVER_ERROR);
        let body = read_response_json(response).await;
        assert_eq!(body["status"], "error");
        assert_eq!(body["config_saved"], false);
        assert!(
            body["message"]
                .as_str()
                .is_some_and(|m| m.starts_with("failed to save config: ")
                    && m.contains("simulated insert failure")),
            "unexpected message: {body}"
        );
        assert!(mock_dao.worker_configs().is_empty());
    }
}
