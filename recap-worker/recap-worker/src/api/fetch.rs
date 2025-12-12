use std::{collections::HashSet, time::Instant};

use axum::{Json, extract::State, http::StatusCode, response::IntoResponse};
use serde::Serialize;
use serde_json::{Map, Value};
use tracing::{error, info};

use crate::app::AppState;

#[derive(Debug, Serialize)]
pub(crate) struct RecapGenreResponse {
    genre: String,
    summary: String,
    top_terms: Vec<String>,
    article_count: i32,
    cluster_count: i32,
    evidence_links: Vec<EvidenceLinkResponse>,
    bullets: Vec<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    references: Option<Vec<ReferenceResponse>>,
}

#[derive(Debug, Serialize)]
pub(crate) struct EvidenceLinkResponse {
    article_id: String,
    title: String,
    source_url: String,
    published_at: String,
    lang: String,
}

#[derive(Debug, Serialize)]
pub(crate) struct ReferenceResponse {
    id: i32,
    url: String,
    domain: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    article_id: Option<String>,
}

fn dedupe_evidence_links(links: Vec<EvidenceLinkResponse>) -> Vec<EvidenceLinkResponse> {
    let mut seen_ids = HashSet::new();
    let mut unique_links = Vec::with_capacity(links.len());

    for link in links {
        if seen_ids.insert(link.article_id.clone()) {
            unique_links.push(link);
        }
    }

    unique_links
}

fn normalize_summary_text(raw: &str) -> String {
    let trimmed = raw.trim();
    if trimmed.is_empty() {
        return String::new();
    }

    flatten_summary_payload(trimmed).unwrap_or_else(|| raw.to_string())
}

fn flatten_summary_payload(raw: &str) -> Option<String> {
    let value: Value = serde_json::from_str(raw).ok()?;
    let object = value.as_object()?;

    let summary_object = object
        .get("summary")
        .and_then(Value::as_object)
        .unwrap_or(object);

    build_summary_lines(summary_object)
}

fn build_summary_lines(summary_object: &Map<String, Value>) -> Option<String> {
    let mut lines = Vec::new();

    if let Some(title) = summary_object
        .get("title")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
    {
        lines.push(title.to_string());
    }

    if let Some(Value::Array(bullets)) = summary_object.get("bullets") {
        for bullet in bullets {
            match bullet {
                Value::String(text) => {
                    let trimmed = text.trim();
                    if !trimmed.is_empty() {
                        lines.push(trimmed.to_string());
                    }
                }
                Value::Object(obj) => {
                    if let Some(text) = obj
                        .get("text")
                        .and_then(Value::as_str)
                        .map(str::trim)
                        .filter(|value| !value.is_empty())
                    {
                        lines.push(text.to_string());
                    }
                }
                Value::Number(num) => lines.push(num.to_string()),
                Value::Bool(flag) => lines.push(flag.to_string()),
                _ => {}
            }
        }
    }

    if lines.is_empty() {
        return None;
    }

    Some(lines.join("\n"))
}

fn extract_bullets(summary_object: &Map<String, Value>) -> Vec<String> {
    let mut bullets_vec = Vec::new();

    if let Some(Value::Array(bullets)) = summary_object.get("bullets") {
        for bullet in bullets {
            match bullet {
                Value::String(text) => {
                    let trimmed = text.trim();
                    if !trimmed.is_empty() {
                        bullets_vec.push(trimmed.to_string());
                    }
                }
                Value::Object(obj) => {
                    if let Some(text) = obj
                        .get("text")
                        .and_then(Value::as_str)
                        .map(str::trim)
                        .filter(|value| !value.is_empty())
                    {
                        bullets_vec.push(text.to_string());
                    }
                }
                Value::Number(num) => bullets_vec.push(num.to_string()),
                Value::Bool(flag) => bullets_vec.push(flag.to_string()),
                _ => {}
            }
        }
    }
    bullets_vec
}

fn extract_bullets_from_payload(raw: &str) -> Vec<String> {
    let value: Value = serde_json::from_str(raw).unwrap_or(Value::Null);
    let object = value.as_object();

    if let Some(obj) = object {
        let summary_object = obj.get("summary").and_then(Value::as_object).unwrap_or(obj);
        extract_bullets(summary_object)
    } else {
        Vec::new()
    }
}

/// Extract references from body_json.
/// Returns None if references are not found or invalid.
fn extract_references_from_body_json(body_json: &Value) -> Option<Vec<ReferenceResponse>> {
    let summary = body_json.get("summary")?.as_object()?;
    let references_array = summary.get("references")?.as_array()?;

    let mut references = Vec::new();
    for ref_value in references_array {
        let ref_obj = ref_value.as_object()?;
        let id = ref_obj.get("id")?.as_i64()? as i32;
        let url = ref_obj.get("url")?.as_str()?.to_string();
        let domain = ref_obj.get("domain")?.as_str()?.to_string();
        let article_id = ref_obj
            .get("article_id")
            .and_then(|v| v.as_str())
            .map(str::to_string);

        references.push(ReferenceResponse {
            id,
            url,
            domain,
            article_id,
        });
    }

    if references.is_empty() {
        None
    } else {
        Some(references)
    }
}

#[derive(Debug, Serialize)]
pub(crate) struct RecapSummaryResponse {
    job_id: String,
    executed_at: String,
    window_start: String,
    window_end: String,
    total_articles: i32,
    genres: Vec<RecapGenreResponse>,
}

#[derive(Debug, Serialize)]
struct ErrorResponse {
    error: String,
}

/// GET /v1/recaps/7days
/// 最新の7日間Recapデータを取得する
#[allow(clippy::too_many_lines)]
pub(crate) async fn get_7days_recap(State(state): State<AppState>) -> impl IntoResponse {
    info!("Fetching latest 7-day recap");
    let metrics = state.telemetry().metrics();
    let handler_start = Instant::now();

    // 最新のジョブを取得
    let dao = state.dao();

    let job = match dao.get_latest_completed_job(7).await {
        Ok(Some(job)) => job,
        Ok(None) => {
            info!("No completed 7-day recap found");
            return (
                StatusCode::NOT_FOUND,
                Json(ErrorResponse {
                    error: "No 7-day recap found".to_string(),
                }),
            )
                .into_response();
        }
        Err(e) => {
            error!("Failed to fetch latest job: {}", e);
            return (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(ErrorResponse {
                    error: "Failed to fetch recap data".to_string(),
                }),
            )
                .into_response();
        }
    };

    // ジャンルデータを取得
    let genres = match dao.get_genres_by_job(job.job_id).await {
        Ok(genres) => genres,
        Err(e) => {
            error!("Failed to fetch genres for job {}: {}", job.job_id, e);
            return (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(ErrorResponse {
                    error: "Failed to fetch genre data".to_string(),
                }),
            )
                .into_response();
        }
    };

    let cluster_query_start = Instant::now();
    let mut clusters_by_genre = match dao.get_clusters_by_job(job.job_id).await {
        Ok(map) => map,
        Err(e) => {
            error!("Failed to fetch clusters for job {}: {}", job.job_id, e);
            return (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(ErrorResponse {
                    error: "Failed to fetch cluster data".to_string(),
                }),
            )
                .into_response();
        }
    };
    metrics
        .api_cluster_query_duration
        .observe(cluster_query_start.elapsed().as_secs_f64());

    // レスポンス構築
    let mut genre_responses = Vec::new();
    for genre in genres {
        let clusters = clusters_by_genre
            .remove(&genre.genre_name)
            .unwrap_or_default();

        // トップターム、記事数、クラスター数を集計
        let mut all_top_terms = Vec::new();
        let mut total_article_count = 0;
        let mut evidence_links = Vec::new();

        for cluster in &clusters {
            // top_termsを収集
            if let Some(terms) = &cluster.top_terms {
                all_top_terms.extend(terms.iter().cloned());
            }

            // 記事数をカウント
            #[allow(clippy::cast_possible_truncation, clippy::cast_possible_wrap)]
            {
                total_article_count += cluster.evidence.len() as i32;
            }

            // Evidence Links（最初の5件程度）
            for evidence in cluster.evidence.iter().take(5) {
                evidence_links.push(EvidenceLinkResponse {
                    article_id: evidence.article_id.clone(),
                    title: evidence.title.clone(),
                    source_url: evidence.source_url.clone(),
                    published_at: evidence.published_at.to_rfc3339(),
                    lang: evidence
                        .lang
                        .clone()
                        .unwrap_or_else(|| "unknown".to_string()),
                });
            }
        }

        // 重複除去して最大5個のトップタームを取得
        all_top_terms.sort();
        all_top_terms.dedup();
        all_top_terms.truncate(10);

        let deduped_links = {
            let before = evidence_links.len();
            let deduped = dedupe_evidence_links(evidence_links);
            let removed = before.saturating_sub(deduped.len());
            if removed > 0 {
                #[allow(clippy::cast_precision_loss)]
                {
                    metrics.api_evidence_duplicates.inc_by(removed as f64);
                }
            }
            deduped
        };

        // Extract references from body_json
        let references = match dao
            .get_recap_output_body_json(job.job_id, &genre.genre_name)
            .await
        {
            Ok(Some(body_json)) => extract_references_from_body_json(&body_json),
            Ok(None) => None,
            Err(e) => {
                // Log warning but don't fail the request
                tracing::warn!(
                    "Failed to fetch body_json for genre {}: {}",
                    genre.genre_name,
                    e
                );
                None
            }
        };

        genre_responses.push(RecapGenreResponse {
            genre: genre.genre_name.clone(),
            summary: normalize_summary_text(genre.summary_ja.as_deref().unwrap_or_default()),
            top_terms: all_top_terms,
            article_count: total_article_count,
            #[allow(clippy::cast_possible_truncation, clippy::cast_possible_wrap)]
            cluster_count: clusters.len() as i32,
            evidence_links: deduped_links,
            bullets: extract_bullets_from_payload(genre.summary_ja.as_deref().unwrap_or_default()),
            references,
        });
    }

    let response = RecapSummaryResponse {
        job_id: job.job_id.to_string(),
        executed_at: job.started_at.to_rfc3339(),
        window_start: job.window_start.to_rfc3339(),
        window_end: job.window_end.to_rfc3339(),
        total_articles: job.total_articles.unwrap_or(0),
        genres: genre_responses,
    };

    info!("Successfully fetched 7-day recap for job {}", job.job_id);
    metrics
        .api_latest_fetch_duration
        .observe(handler_start.elapsed().as_secs_f64());
    (StatusCode::OK, Json(response)).into_response()
}

#[cfg(test)]
mod tests {
    use super::*;

    fn make_link(article_id: &str, title: &str) -> EvidenceLinkResponse {
        EvidenceLinkResponse {
            article_id: article_id.to_string(),
            title: title.to_string(),
            source_url: format!("https://example.com/{article_id}"),
            published_at: "2025-11-11T00:00:00Z".to_string(),
            lang: "ja".to_string(),
        }
    }

    #[test]
    fn dedupe_evidence_links_removes_duplicates_and_preserves_order() {
        let links = vec![
            make_link("a", "First"),
            make_link("b", "Second"),
            make_link("a", "First Duplicate"),
            make_link("c", "Third"),
        ];

        let deduped = dedupe_evidence_links(links);
        let article_ids: Vec<_> = deduped
            .iter()
            .map(|link| link.article_id.as_str())
            .collect();

        assert_eq!(article_ids, vec!["a", "b", "c"]);
    }

    #[test]
    fn normalize_summary_text_preserves_plain_text() {
        let input = "一行目\n二行目";
        assert_eq!(normalize_summary_text(input), input);
    }

    #[test]
    fn normalize_summary_text_flattens_json_summary() {
        let input = r#"{
            "title": "最新ニュースまとめ",
            "bullets": [
                {"text": "要点1"},
                {"text": "要点2"}
            ]
        }"#;

        let expected = "最新ニュースまとめ\n要点1\n要点2";
        assert_eq!(normalize_summary_text(input), expected);
    }

    #[test]
    fn normalize_summary_text_handles_nested_summary_key() {
        let input = r#"{
            "summary": {
                "title": "ネストタイトル",
                "bullets": ["詳細A", "詳細B"]
            },
            "other": "ignored"
        }"#;

        let expected = "ネストタイトル\n詳細A\n詳細B";
        assert_eq!(normalize_summary_text(input), expected);
    }

    #[test]
    fn normalize_summary_text_falls_back_on_invalid_json() {
        let input = "{ invalid json";
        assert_eq!(normalize_summary_text(input), input);
    }
}

#[derive(Debug, serde::Deserialize)]
pub(crate) struct MorningUpdatesQuery {
    since: Option<chrono::DateTime<chrono::Utc>>,
}

#[derive(Debug, Serialize)]
pub(crate) struct MorningArticleGroupResponse {
    group_id: uuid::Uuid,
    article_id: uuid::Uuid,
    is_primary: bool,
    created_at: chrono::DateTime<chrono::Utc>,
}

/// GET /v1/morning/updates
/// 指定された日時以降のMorning Letter更新分を取得する
pub(crate) async fn get_morning_updates(
    State(state): State<AppState>,
    axum::extract::Query(query): axum::extract::Query<MorningUpdatesQuery>,
) -> impl IntoResponse {
    let since = query
        .since
        .unwrap_or_else(|| chrono::Utc::now() - chrono::Duration::hours(24));
    let dao = state.dao();

    match dao.get_morning_article_groups(since).await {
        Ok(groups) => {
            let response: Vec<MorningArticleGroupResponse> = groups
                .into_iter()
                .map(
                    |(group_id, article_id, is_primary, created_at)| MorningArticleGroupResponse {
                        group_id,
                        article_id,
                        is_primary,
                        created_at,
                    },
                )
                .collect();
            (StatusCode::OK, Json(response)).into_response()
        }
        Err(e) => {
            error!("Failed to fetch morning updates: {}", e);
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(ErrorResponse {
                    error: "Failed to fetch morning updates".to_string(),
                }),
            )
                .into_response()
        }
    }
}
