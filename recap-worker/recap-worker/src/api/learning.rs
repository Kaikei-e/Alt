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

#[allow(clippy::too_many_lines)]
pub(crate) async fn receive_genre_learning(
    State(state): State<AppState>,
    Json(payload): Json<GenreLearningRequest>,
) -> impl IntoResponse {
    info!("received genre learning payload from recap-subworker");

    // Build config payload from request
    let mut config_payload = serde_json::Map::new();

    if let Some(ref graph_override) = payload.graph_override {
        if let Some(graph_margin) = graph_override.graph_margin {
            config_payload.insert(
                "graph_margin".to_string(),
                serde_json::Value::Number(
                    serde_json::Number::from_f64(f64::from(graph_margin)).unwrap(),
                ),
            );
        }
        if let Some(weighted_tie_break_margin) = graph_override.weighted_tie_break_margin {
            config_payload.insert(
                "weighted_tie_break_margin".to_string(),
                serde_json::Value::Number(
                    serde_json::Number::from_f64(f64::from(weighted_tie_break_margin)).unwrap(),
                ),
            );
        }
        if let Some(tag_confidence_gate) = graph_override.tag_confidence_gate {
            config_payload.insert(
                "tag_confidence_gate".to_string(),
                serde_json::Value::Number(
                    serde_json::Number::from_f64(f64::from(tag_confidence_gate)).unwrap(),
                ),
            );
        }
        if let Some(boost_threshold) = graph_override.boost_threshold {
            config_payload.insert(
                "boost_threshold".to_string(),
                serde_json::Value::Number(
                    serde_json::Number::from_f64(f64::from(boost_threshold)).unwrap(),
                ),
            );
        }
        if let Some(tag_count_threshold) = graph_override.tag_count_threshold {
            config_payload.insert(
                "tag_count_threshold".to_string(),
                serde_json::Value::Number(tag_count_threshold.into()),
            );
        }
    }

    // Fallback to summary values if graph_override is missing
    if config_payload.is_empty() {
        if let Some(graph_margin) = payload.summary.graph_margin_reference {
            config_payload.insert(
                "graph_margin".to_string(),
                serde_json::Value::Number(
                    serde_json::Number::from_f64(f64::from(graph_margin)).unwrap(),
                ),
            );
        }
        if let Some(boost_threshold) = payload.summary.boost_threshold_reference {
            config_payload.insert(
                "boost_threshold".to_string(),
                serde_json::Value::Number(
                    serde_json::Number::from_f64(f64::from(boost_threshold)).unwrap(),
                ),
            );
        }
        if let Some(tag_count_threshold) = payload.summary.tag_count_threshold_reference {
            config_payload.insert(
                "tag_count_threshold".to_string(),
                serde_json::Value::Number(tag_count_threshold.into()),
            );
        }
    }

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

    match state
        .dao()
        .insert_worker_config(
            "graph_override",
            &config_json,
            "genre_learning",
            Some(&metadata),
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
