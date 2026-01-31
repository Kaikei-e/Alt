//! Evening Pulse API handler
//!
//! Provides the `/v1/pulse/latest` endpoint for retrieving the latest
//! Evening Pulse data.

use axum::{
    Json,
    extract::{Query, State},
    http::StatusCode,
    response::IntoResponse,
};
use chrono::NaiveDate;
use serde::{Deserialize, Serialize};
use tracing::{error, info};

use crate::app::AppState;
use crate::pipeline::pulse::{PulseDiagnostics, PulseResult, PulseTopic, TopicRole};

/// Query parameters for GET /v1/pulse/latest
#[derive(Debug, Deserialize)]
pub(crate) struct PulseLatestQuery {
    /// Optional date in YYYY-MM-DD format. If not provided, returns the latest pulse.
    date: Option<String>,
}

/// API response for pulse endpoint
#[derive(Debug, Serialize)]
pub(crate) struct PulseLatestResponse {
    job_id: String,
    version: String,
    date: String,
    generated_at: String,
    status: String,
    topics: Vec<PulseTopicResponse>,
    quiet_day: Option<QuietDayResponse>,
    diagnostics: Option<PulseDiagnosticsResponse>,
}

/// Topic in the API response
#[derive(Debug, Serialize)]
pub(crate) struct PulseTopicResponse {
    cluster_id: i64,
    role: String,
    title: String,
    rationale: PulseRationaleResponse,
    article_count: i32,
    source_count: i32,
    tier1_count: Option<i32>,
    time_ago: String,
    trend_multiplier: Option<f64>,
    genre: Option<String>,
    article_ids: Vec<String>,
}

/// Rationale for topic selection
#[derive(Debug, Serialize)]
pub(crate) struct PulseRationaleResponse {
    text: String,
    confidence: String,
}

/// Quiet day response when no significant news
#[derive(Debug, Serialize)]
pub(crate) struct QuietDayResponse {
    message: String,
    weekly_highlights: Vec<WeeklyHighlightResponse>,
}

/// Weekly highlight for quiet days
#[derive(Debug, Serialize)]
pub(crate) struct WeeklyHighlightResponse {
    id: String,
    title: String,
    date: String,
    role: String,
}

/// Diagnostics information
#[derive(Debug, Serialize)]
pub(crate) struct PulseDiagnosticsResponse {
    syndication_removed: i32,
    clusters_evaluated: i32,
    fallback_level: i32,
    duration_ms: i64,
}

/// Error response
#[derive(Debug, Serialize)]
struct ErrorResponse {
    error: String,
}

/// GET /v1/pulse/latest
///
/// Returns the latest Evening Pulse data.
/// Optionally accepts a `date` query parameter in YYYY-MM-DD format.
pub(crate) async fn get_latest(
    State(state): State<AppState>,
    Query(query): Query<PulseLatestQuery>,
) -> impl IntoResponse {
    info!("Fetching latest evening pulse, date={:?}", query.date);

    let dao = state.dao();

    let result = match &query.date {
        Some(date_str) => {
            let Ok(date) = NaiveDate::parse_from_str(date_str, "%Y-%m-%d") else {
                return (
                    StatusCode::BAD_REQUEST,
                    Json(ErrorResponse {
                        error: format!("Invalid date format: {date_str}. Expected YYYY-MM-DD"),
                    }),
                )
                    .into_response();
            };
            dao.get_pulse_by_date(date).await
        }
        None => dao.get_latest_pulse().await,
    };

    match result {
        Ok(Some(row)) => match build_response(&row) {
            Ok(response) => {
                info!("Successfully fetched evening pulse for job {}", row.job_id);
                (StatusCode::OK, Json(response)).into_response()
            }
            Err(e) => {
                error!("Failed to build pulse response: {}", e);
                (
                    StatusCode::INTERNAL_SERVER_ERROR,
                    Json(ErrorResponse {
                        error: "Failed to build response".to_string(),
                    }),
                )
                    .into_response()
            }
        },
        Ok(None) => {
            let date_msg = query
                .date
                .as_ref()
                .map_or_else(|| "today".to_string(), Clone::clone);
            info!("No evening pulse found for date {}", date_msg);
            (
                StatusCode::NOT_FOUND,
                Json(ErrorResponse {
                    error: format!("No evening pulse found for date {date_msg}"),
                }),
            )
                .into_response()
        }
        Err(e) => {
            error!("Failed to fetch pulse: {}", e);
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(ErrorResponse {
                    error: "Failed to fetch pulse data".to_string(),
                }),
            )
                .into_response()
        }
    }
}

fn build_response(
    row: &crate::store::models::PulseGenerationRow,
) -> anyhow::Result<PulseLatestResponse> {
    // Deserialize result_payload to PulseResult
    let pulse_result: PulseResult = serde_json::from_value(
        row.result_payload
            .clone()
            .ok_or_else(|| anyhow::anyhow!("No result payload"))?,
    )?;

    let topics = convert_topics(&pulse_result.topics);
    let status = determine_status(&pulse_result);
    let quiet_day = build_quiet_day(&pulse_result);
    let diagnostics = build_diagnostics(&pulse_result.diagnostics);

    Ok(PulseLatestResponse {
        job_id: row.job_id.to_string(),
        version: row.version.clone(),
        date: row.target_date.to_string(),
        generated_at: pulse_result.generated_at.to_rfc3339(),
        status,
        topics,
        quiet_day,
        diagnostics: Some(diagnostics),
    })
}

fn convert_topics(topics: &[PulseTopic]) -> Vec<PulseTopicResponse> {
    topics
        .iter()
        .map(|topic| {
            let article_count = i32::try_from(topic.articles.len()).unwrap_or(i32::MAX);
            // Estimate source count from unique article prefixes (simplified)
            let source_count = estimate_source_count(&topic.articles);

            PulseTopicResponse {
                cluster_id: topic.cluster_id,
                role: topic.role.to_string(),
                title: generate_topic_title(topic),
                rationale: PulseRationaleResponse {
                    text: topic.rationale.clone(),
                    confidence: tier_to_confidence(topic.quality_metrics.tier),
                },
                article_count,
                source_count,
                tier1_count: None,       // Would require additional data
                time_ago: String::new(), // Would require published_at from articles
                trend_multiplier: if topic.role == TopicRole::Trend {
                    Some(f64::from(topic.score_breakdown.burst_score))
                } else {
                    None
                },
                genre: None, // Would require genre mapping
                article_ids: topic.articles.clone(),
            }
        })
        .collect()
}

fn generate_topic_title(topic: &PulseTopic) -> String {
    // In a real implementation, this would use the cluster label or generate from articles
    // For now, use a placeholder based on role
    match topic.role {
        TopicRole::NeedToKnow => format!("重要ニュース (クラスタ {})", topic.cluster_id),
        TopicRole::Trend => format!("トレンドトピック (クラスタ {})", topic.cluster_id),
        TopicRole::Serendipity => format!("注目の発見 (クラスタ {})", topic.cluster_id),
    }
}

fn estimate_source_count(article_ids: &[String]) -> i32 {
    // Simplified: count unique article ID prefixes
    // In production, this would query article metadata for unique sources
    use std::collections::HashSet;
    let prefixes: HashSet<_> = article_ids
        .iter()
        .filter_map(|id| id.split('-').next())
        .collect();
    i32::try_from(prefixes.len().max(1)).unwrap_or(1)
}

fn tier_to_confidence(tier: crate::pipeline::pulse::QualityTier) -> String {
    match tier {
        crate::pipeline::pulse::QualityTier::Ok => "high".to_string(),
        crate::pipeline::pulse::QualityTier::Caution => "medium".to_string(),
        crate::pipeline::pulse::QualityTier::Ng => "low".to_string(),
    }
}

fn determine_status(result: &PulseResult) -> String {
    if result.topics.is_empty() {
        "quiet_day".to_string()
    } else if result.topics.len() < 3 {
        "partial".to_string()
    } else {
        "normal".to_string()
    }
}

fn build_quiet_day(result: &PulseResult) -> Option<QuietDayResponse> {
    if result.topics.is_empty() {
        Some(QuietDayResponse {
            message: "今日は静かな一日でした。特筆すべきニュースは見つかりませんでした。"
                .to_string(),
            weekly_highlights: Vec::new(), // Would be populated from 7days recap
        })
    } else {
        None
    }
}

fn build_diagnostics(diag: &PulseDiagnostics) -> PulseDiagnosticsResponse {
    PulseDiagnosticsResponse {
        syndication_removed: i32::try_from(diag.syndication_removed).unwrap_or(i32::MAX),
        clusters_evaluated: i32::try_from(diag.clusters_evaluated).unwrap_or(i32::MAX),
        fallback_level: i32::from(diag.fallback_level),
        duration_ms: i64::try_from(diag.duration_ms).unwrap_or(i64::MAX),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::pipeline::pulse::{
        ClusterQualityMetrics, PulseDiagnostics, PulseResult, PulseTopic, PulseVersion,
        QualityTier, ScoreBreakdown,
    };
    use chrono::Utc;
    use uuid::Uuid;

    fn create_test_topic(role: TopicRole) -> PulseTopic {
        PulseTopic {
            cluster_id: 42,
            role,
            score: 0.85,
            rationale: "High impact news with multiple sources".to_string(),
            articles: vec![
                "art-001".to_string(),
                "art-002".to_string(),
                "art-003".to_string(),
            ],
            quality_metrics: ClusterQualityMetrics {
                cohesion: 0.8,
                ambiguity: 0.2,
                entity_consistency: 0.9,
                tier: QualityTier::Ok,
            },
            score_breakdown: ScoreBreakdown {
                impact_score: 0.5,
                burst_score: 0.15,
                novelty_score: 0.1,
                recency_score: 0.1,
            },
        }
    }

    fn create_test_pulse_result() -> PulseResult {
        PulseResult {
            job_id: Uuid::new_v4(),
            version: PulseVersion::V4,
            topics: vec![
                create_test_topic(TopicRole::NeedToKnow),
                create_test_topic(TopicRole::Trend),
                create_test_topic(TopicRole::Serendipity),
            ],
            generated_at: Utc::now(),
            diagnostics: PulseDiagnostics {
                syndication_removed: 15,
                clusters_evaluated: 42,
                fallback_level: 0,
                duration_ms: 1250,
                ..Default::default()
            },
        }
    }

    #[test]
    fn test_determine_status_normal() {
        let result = create_test_pulse_result();
        assert_eq!(determine_status(&result), "normal");
    }

    #[test]
    fn test_determine_status_partial() {
        let mut result = create_test_pulse_result();
        result.topics = vec![create_test_topic(TopicRole::NeedToKnow)];
        assert_eq!(determine_status(&result), "partial");
    }

    #[test]
    fn test_determine_status_quiet_day() {
        let mut result = create_test_pulse_result();
        result.topics = vec![];
        assert_eq!(determine_status(&result), "quiet_day");
    }

    #[test]
    fn test_tier_to_confidence() {
        assert_eq!(tier_to_confidence(QualityTier::Ok), "high");
        assert_eq!(tier_to_confidence(QualityTier::Caution), "medium");
        assert_eq!(tier_to_confidence(QualityTier::Ng), "low");
    }

    #[test]
    fn test_estimate_source_count() {
        let articles = vec![
            "src1-001".to_string(),
            "src1-002".to_string(),
            "src2-001".to_string(),
        ];
        assert_eq!(estimate_source_count(&articles), 2);
    }

    #[test]
    fn test_convert_topics() {
        let topics = vec![create_test_topic(TopicRole::NeedToKnow)];
        let converted = convert_topics(&topics);

        assert_eq!(converted.len(), 1);
        assert_eq!(converted[0].cluster_id, 42);
        assert_eq!(converted[0].role, "need_to_know");
        assert_eq!(converted[0].article_count, 3);
        assert_eq!(converted[0].rationale.confidence, "high");
    }

    #[test]
    fn test_build_quiet_day_returns_some_for_empty_topics() {
        let mut result = create_test_pulse_result();
        result.topics = vec![];

        let quiet = build_quiet_day(&result);
        assert!(quiet.is_some());
        assert!(quiet.unwrap().message.contains("静かな一日"));
    }

    #[test]
    fn test_build_quiet_day_returns_none_for_topics() {
        let result = create_test_pulse_result();
        let quiet = build_quiet_day(&result);
        assert!(quiet.is_none());
    }

    #[test]
    fn test_build_diagnostics() {
        let diag = PulseDiagnostics {
            syndication_removed: 15,
            clusters_evaluated: 42,
            fallback_level: 1,
            duration_ms: 1250,
            ..Default::default()
        };

        let response = build_diagnostics(&diag);
        assert_eq!(response.syndication_removed, 15);
        assert_eq!(response.clusters_evaluated, 42);
        assert_eq!(response.fallback_level, 1);
        assert_eq!(response.duration_ms, 1250);
    }
}
