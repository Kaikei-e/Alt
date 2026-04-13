use std::{collections::HashSet, time::Instant};

use axum::{
    Json, extract::Path, extract::State, http::HeaderMap, http::StatusCode, response::IntoResponse,
};
use serde::Serialize;
use serde_json::{Map, Value};
use tracing::{error, info, warn};

use crate::app::AppState;
use crate::store::models::MorningLetterContent;

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

// =============================================================================
// Search recaps by term
// =============================================================================

#[derive(Debug, serde::Deserialize)]
pub(crate) struct SearchRecapsQuery {
    term: String,
    limit: Option<i32>,
}

#[derive(Debug, Serialize)]
pub(crate) struct RecapSearchHitResponse {
    job_id: String,
    executed_at: String,
    window_days: i32,
    genre: String,
    summary: String,
    top_terms: Vec<String>,
    tags: Vec<String>,
    bullets: Vec<String>,
}

#[derive(Debug, Serialize)]
pub(crate) struct SearchRecapsResponse {
    results: Vec<RecapSearchHitResponse>,
}

/// GET /v1/recaps/search?term=LLM&limit=50
/// Search across all completed recap jobs for genres matching the given term in top_terms.
pub(crate) async fn search_recaps(
    State(state): State<AppState>,
    axum::extract::Query(query): axum::extract::Query<SearchRecapsQuery>,
) -> impl IntoResponse {
    let term = query.term.trim();
    if term.is_empty() {
        return (
            StatusCode::BAD_REQUEST,
            Json(ErrorResponse {
                error: "term parameter is required".to_string(),
            }),
        )
            .into_response();
    }

    let limit = query.limit.unwrap_or(50).min(200);
    let dao = state.dao();

    match dao.search_recaps_by_term(term, limit).await {
        Ok(hits) => {
            let results: Vec<RecapSearchHitResponse> = hits
                .into_iter()
                .map(|hit| {
                    let summary =
                        normalize_summary_text(hit.summary_ja.as_deref().unwrap_or_default());
                    let bullets =
                        extract_bullets_from_payload(hit.summary_ja.as_deref().unwrap_or_default());
                    RecapSearchHitResponse {
                        job_id: hit.job_id.to_string(),
                        executed_at: hit.executed_at.to_rfc3339(),
                        window_days: hit.window_days,
                        genre: hit.genre,
                        summary,
                        top_terms: hit.top_terms,
                        tags: hit.tags,
                        bullets,
                    }
                })
                .collect();

            info!(
                "Search recaps: term='{}', found {} results",
                term,
                results.len()
            );
            (StatusCode::OK, Json(SearchRecapsResponse { results })).into_response()
        }
        Err(e) => {
            error!("Failed to search recaps by term '{}': {}", term, e);
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(ErrorResponse {
                    error: "Failed to search recaps".to_string(),
                }),
            )
                .into_response()
        }
    }
}

// =============================================================================
// Indexable genres (for search-indexer Meilisearch indexing)
// =============================================================================

#[derive(Debug, serde::Deserialize)]
pub(crate) struct IndexableGenresQuery {
    since: Option<String>,
    limit: Option<i32>,
}

#[derive(Debug, Serialize)]
pub(crate) struct IndexableGenresResponse {
    results: Vec<RecapSearchHitResponse>,
    has_more: bool,
}

/// GET /v1/recaps/genres/indexable?since=<RFC3339>&limit=200
/// Returns all completed recap genres suitable for Meilisearch indexing.
pub(crate) async fn get_indexable_genres(
    State(state): State<AppState>,
    axum::extract::Query(query): axum::extract::Query<IndexableGenresQuery>,
) -> impl IntoResponse {
    let limit = query.limit.unwrap_or(200).min(1000);
    let since = query.since.as_deref().and_then(|s| {
        chrono::DateTime::parse_from_rfc3339(s)
            .ok()
            .map(|dt| dt.with_timezone(&chrono::Utc))
    });

    let pool = state.pool();

    match crate::store::dao::output::RecapDao::fetch_indexable_genres(pool, since, limit).await {
        Ok(hits) => {
            let has_more = hits.len() == usize::try_from(limit).unwrap_or(0);
            let results: Vec<RecapSearchHitResponse> = hits
                .into_iter()
                .map(|hit| {
                    let summary =
                        normalize_summary_text(hit.summary_ja.as_deref().unwrap_or_default());
                    let bullets =
                        extract_bullets_from_payload(hit.summary_ja.as_deref().unwrap_or_default());
                    RecapSearchHitResponse {
                        job_id: hit.job_id.to_string(),
                        executed_at: hit.executed_at.to_rfc3339(),
                        window_days: hit.window_days,
                        genre: hit.genre,
                        summary,
                        top_terms: hit.top_terms,
                        tags: hit.tags,
                        bullets,
                    }
                })
                .collect();

            info!(
                "Indexable genres: since={:?}, returned {} results, has_more={}",
                query.since,
                results.len(),
                has_more
            );
            (
                StatusCode::OK,
                Json(IndexableGenresResponse { results, has_more }),
            )
                .into_response()
        }
        Err(e) => {
            error!("Failed to fetch indexable genres: {}", e);
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(ErrorResponse {
                    error: "Failed to fetch indexable genres".to_string(),
                }),
            )
                .into_response()
        }
    }
}

/// GET /v1/recaps/7days
/// 最新の7日間Recapデータを取得する
pub(crate) async fn get_7days_recap(State(state): State<AppState>) -> impl IntoResponse {
    get_recap_by_window(state, 7, "7-day").await
}

/// GET /v1/recaps/3days
/// 最新の3日間Recapデータを取得する
pub(crate) async fn get_3days_recap(State(state): State<AppState>) -> impl IntoResponse {
    get_recap_by_window(state, 3, "3-day").await
}

/// 指定されたウィンドウ日数のRecapデータを取得する
#[allow(clippy::too_many_lines)]
async fn get_recap_by_window(
    state: AppState,
    window_days: i32,
    label: &str,
) -> axum::response::Response {
    info!("Fetching latest {} recap", label);
    let metrics = state.telemetry().metrics();
    let handler_start = Instant::now();

    // 最新のジョブを取得
    let dao = state.dao();

    let job = match dao.get_latest_completed_job(window_days).await {
        Ok(Some(job)) => job,
        Ok(None) => {
            info!("No completed {} recap found", label);
            return (
                StatusCode::NOT_FOUND,
                Json(ErrorResponse {
                    error: format!("No {} recap found", label),
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

    info!(
        "Successfully fetched {} recap for job {}",
        label, job.job_id
    );
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

// =============================================================================
// Morning Letter Read Endpoints
// =============================================================================

#[derive(Debug, Serialize)]
pub(crate) struct MorningLetterResponse {
    id: String,
    target_date: String,
    edition_timezone: String,
    is_degraded: bool,
    schema_version: i32,
    generation_revision: i32,
    model: Option<String>,
    created_at: String,
    etag: String,
    body: MorningLetterBodyResponse,
}

#[derive(Debug, Serialize)]
pub(crate) struct MorningLetterBodyResponse {
    lead: String,
    sections: Vec<MorningLetterSectionResponse>,
    generated_at: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    source_recap_window_days: Option<u32>,
    #[serde(skip_serializing_if = "String::is_empty", default)]
    through_line: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    previous_letter_ref: Option<PreviousLetterRefResponse>,
}

#[derive(Debug, Serialize)]
pub(crate) struct MorningLetterSectionResponse {
    key: String,
    title: String,
    bullets: Vec<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    genre: Option<String>,
    #[serde(skip_serializing_if = "String::is_empty", default)]
    narrative: String,
    #[serde(skip_serializing_if = "Vec::is_empty", default)]
    why_reasons: Vec<WhyReasonResponse>,
}

#[derive(Debug, Serialize)]
pub(crate) struct PreviousLetterRefResponse {
    id: String,
    target_date: String,
    through_line: String,
}

#[derive(Debug, Serialize)]
pub(crate) struct WhyReasonResponse {
    code: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    ref_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    tag: Option<String>,
}

#[derive(Debug, Serialize)]
pub(crate) struct MorningLetterSourceResponse {
    letter_id: String,
    section_key: String,
    article_id: String,
    source_type: String,
    position: i32,
}

/// Parse `result_jsonb` into a typed body. Fails closed on malformed payloads.
fn parse_morning_letter_body(
    result_jsonb: &Value,
    letter_id: &str,
    schema_version: i32,
    model: Option<&String>,
    generation_revision: i32,
) -> Result<MorningLetterBodyResponse, StatusCode> {
    if schema_version != 1 {
        warn!(
            letter_id = letter_id,
            schema_version = schema_version,
            "unsupported morning letter schema_version"
        );
        return Err(StatusCode::INTERNAL_SERVER_ERROR);
    }

    match serde_json::from_value::<MorningLetterContent>(result_jsonb.clone()) {
        Ok(content) => Ok(MorningLetterBodyResponse {
            lead: content.lead,
            sections: content
                .sections
                .into_iter()
                .map(|s| MorningLetterSectionResponse {
                    key: s.key,
                    title: s.title,
                    bullets: s.bullets,
                    genre: s.genre,
                    narrative: s.narrative.unwrap_or_default(),
                    why_reasons: s
                        .why_reasons
                        .into_iter()
                        .map(|w| WhyReasonResponse {
                            code: w.code,
                            ref_id: w.ref_id,
                            tag: w.tag,
                        })
                        .collect(),
                })
                .collect(),
            generated_at: content.generated_at,
            source_recap_window_days: content.source_recap_window_days,
            through_line: content.through_line.unwrap_or_default(),
            previous_letter_ref: content.previous_letter_ref.map(|p| {
                PreviousLetterRefResponse {
                    id: p.id,
                    target_date: p.target_date,
                    through_line: p.through_line,
                }
            }),
        }),
        Err(e) => {
            error!(
                letter_id = letter_id,
                schema_version = schema_version,
                model = model.map_or("unknown", String::as_str),
                generation_revision = generation_revision,
                error = %e,
                "failed to parse morning letter result_jsonb"
            );
            Err(StatusCode::INTERNAL_SERVER_ERROR)
        }
    }
}

/// Map a MorningLetter model to the REST response, including ETag/Last-Modified headers.
fn map_morning_letter_response(
    letter: &crate::store::models::MorningLetter,
) -> Result<(HeaderMap, Json<MorningLetterResponse>), (StatusCode, Json<ErrorResponse>)> {
    let letter_id_str = letter.id.to_string();
    let body = parse_morning_letter_body(
        &letter.result_jsonb,
        &letter_id_str,
        letter.schema_version,
        letter.model.as_ref(),
        letter.generation_revision,
    )
    .map_err(|status| {
        (
            status,
            Json(ErrorResponse {
                error: "Failed to parse morning letter content".to_string(),
            }),
        )
    })?;

    let etag = format!("\"{}:{}\"", letter.id, letter.generation_revision);

    let mut headers = HeaderMap::new();
    if let Ok(val) = etag.parse() {
        headers.insert("ETag", val);
    }
    if let Ok(val) = letter
        .created_at
        .format("%a, %d %b %Y %H:%M:%S GMT")
        .to_string()
        .parse()
    {
        headers.insert("Last-Modified", val);
    }

    Ok((
        headers,
        Json(MorningLetterResponse {
            id: letter_id_str,
            target_date: letter.target_date.to_string(),
            edition_timezone: letter.edition_timezone.clone(),
            is_degraded: letter.is_degraded,
            schema_version: letter.schema_version,
            generation_revision: letter.generation_revision,
            model: letter.model.clone(),
            created_at: letter.created_at.to_rfc3339(),
            etag: etag.clone(),
            body,
        }),
    ))
}

/// GET /v1/morning/letters/latest
/// Edition timezone 基準での最新の Morning Letter を返す。
pub(crate) async fn get_latest_morning_letter(State(state): State<AppState>) -> impl IntoResponse {
    let dao = state.dao();

    match dao.get_latest_morning_letter().await {
        Ok(Some(letter)) => match map_morning_letter_response(&letter) {
            Ok((headers, json)) => (StatusCode::OK, headers, json).into_response(),
            Err((status, json)) => (status, json).into_response(),
        },
        Ok(None) => (
            StatusCode::NOT_FOUND,
            Json(ErrorResponse {
                error: "No morning letter found".to_string(),
            }),
        )
            .into_response(),
        Err(e) => {
            error!("Failed to fetch latest morning letter: {}", e);
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(ErrorResponse {
                    error: "Failed to fetch latest morning letter".to_string(),
                }),
            )
                .into_response()
        }
    }
}

/// GET /v1/morning/letters/{target_date}
/// 指定日の Morning Letter を返す。日付形式: YYYY-MM-DD (edition timezone)。
pub(crate) async fn get_morning_letter_by_date(
    State(state): State<AppState>,
    Path(target_date): Path<String>,
) -> impl IntoResponse {
    let Ok(date) = chrono::NaiveDate::parse_from_str(&target_date, "%Y-%m-%d") else {
        return (
            StatusCode::BAD_REQUEST,
            Json(ErrorResponse {
                error: format!(
                    "Invalid date format: '{}'. Expected YYYY-MM-DD",
                    target_date
                ),
            }),
        )
            .into_response();
    };

    let dao = state.dao();

    match dao.get_morning_letter_by_date(date).await {
        Ok(Some(letter)) => match map_morning_letter_response(&letter) {
            Ok((headers, json)) => (StatusCode::OK, headers, json).into_response(),
            Err((status, json)) => (status, json).into_response(),
        },
        Ok(None) => (
            StatusCode::NOT_FOUND,
            Json(ErrorResponse {
                error: format!("No morning letter found for date: {}", target_date),
            }),
        )
            .into_response(),
        Err(e) => {
            error!(
                target_date = %target_date,
                "Failed to fetch morning letter by date: {}", e
            );
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(ErrorResponse {
                    error: "Failed to fetch morning letter".to_string(),
                }),
            )
                .into_response()
        }
    }
}

/// GET /v1/morning/letters/{letter_id}/sources
/// Morning Letter のソース (provenance) を返す。
pub(crate) async fn get_morning_letter_sources(
    State(state): State<AppState>,
    Path(letter_id): Path<String>,
) -> impl IntoResponse {
    let Ok(id) = uuid::Uuid::parse_str(&letter_id) else {
        return (
            StatusCode::BAD_REQUEST,
            Json(ErrorResponse {
                error: format!("Invalid letter_id: '{}'. Expected UUID", letter_id),
            }),
        )
            .into_response();
    };

    let dao = state.dao();

    match dao.get_morning_letter_sources(id).await {
        Ok(sources) => {
            let response: Vec<MorningLetterSourceResponse> = sources
                .into_iter()
                .map(|s| MorningLetterSourceResponse {
                    letter_id: s.letter_id.to_string(),
                    section_key: s.section_key,
                    article_id: s.article_id.to_string(),
                    source_type: s.source_type,
                    position: s.position,
                })
                .collect();
            (StatusCode::OK, Json(response)).into_response()
        }
        Err(e) => {
            error!(
                letter_id = %letter_id,
                "Failed to fetch morning letter sources: {}", e
            );
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(ErrorResponse {
                    error: "Failed to fetch morning letter sources".to_string(),
                }),
            )
                .into_response()
        }
    }
}
