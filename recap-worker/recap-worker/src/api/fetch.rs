use std::{collections::HashSet, time::Instant};

use axum::{
    Json, extract::Path, extract::State, http::HeaderMap, http::StatusCode, response::IntoResponse,
};
use serde::Serialize;
use serde_json::{Map, Value};
use tracing::{error, info, warn};

use crate::api::surface_status::{
    SurfaceRun, SurfaceStatus, build_surface_status, is_body_json_degraded, morning_letter_reasons,
    three_day_recap_reasons,
};
use crate::app::AppState;
use crate::store::dao::surface::SurfaceDaoOps;
use crate::store::models::MorningLetterContent;
pub use crate::store::models::{
    ClusterEvidence, ClusterWithEvidence, GenreWithSummary, MorningLetter, RecapJob,
};

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
    #[serde(skip_serializing_if = "Option::is_none")]
    surface_status: Option<SurfaceStatus>,
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
    #[serde(default)]
    max_results: Option<i32>,
    #[serde(default)]
    offset: Option<i32>,
}

const MAX_SEARCH_SCAN_ROWS: i32 = 1000;

pub(crate) fn clamp_search_limit(limit: Option<i32>) -> i32 {
    limit.unwrap_or(50).clamp(1, 200)
}

pub(crate) fn clamp_indexable_limit(limit: Option<i32>) -> i32 {
    limit.unwrap_or(200).clamp(1, 1000)
}

pub(crate) fn clamp_offset(offset: Option<i32>) -> i32 {
    offset.unwrap_or(0).clamp(0, MAX_SEARCH_SCAN_ROWS)
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

    let limit = clamp_search_limit(query.limit.or(query.max_results));
    let offset = clamp_offset(query.offset);
    let dao = state.dao();

    match dao
        .search_recaps_by_term(term, limit.saturating_add(offset).min(MAX_SEARCH_SCAN_ROWS))
        .await
    {
        Ok(hits) => {
            let offset_usize = usize::try_from(offset).unwrap_or(0);
            let limit_usize = usize::try_from(limit).unwrap_or(0);
            let results: Vec<RecapSearchHitResponse> = hits
                .into_iter()
                .skip(offset_usize)
                .take(limit_usize)
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
    let limit = clamp_indexable_limit(query.limit);
    let since = match query.since.as_deref() {
        Some(s) => match chrono::DateTime::parse_from_rfc3339(s) {
            Ok(dt) => Some(dt.with_timezone(&chrono::Utc)),
            Err(_) => {
                return (
                    StatusCode::BAD_REQUEST,
                    Json(ErrorResponse {
                        error: format!("Invalid since timestamp: '{s}'. Expected RFC3339"),
                    }),
                )
                    .into_response();
            }
        },
        None => None,
    };

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

#[async_trait::async_trait]
pub trait RecapReadDao: Send + Sync {
    async fn get_latest_completed_job(&self, window_days: i32) -> Result<Option<RecapJob>, String>;
    async fn get_genres_by_job(&self, job_id: uuid::Uuid) -> Result<Vec<GenreWithSummary>, String>;
    async fn get_clusters_by_job(
        &self,
        job_id: uuid::Uuid,
    ) -> Result<std::collections::HashMap<String, Vec<ClusterWithEvidence>>, String>;
    async fn get_recap_output_body_json(
        &self,
        job_id: uuid::Uuid,
        genre: &str,
    ) -> Result<Option<Value>, String>;
    async fn get_latest_three_day_recap_run(&self) -> Result<Option<SurfaceRun>, String>;
    fn observe_cluster_query_duration(&self, duration_secs: f64);
    fn inc_evidence_duplicates(&self, removed: usize);
    fn observe_latest_fetch_duration(&self, duration_secs: f64);
}

#[async_trait::async_trait]
impl RecapReadDao for AppState {
    async fn get_latest_completed_job(&self, window_days: i32) -> Result<Option<RecapJob>, String> {
        self.dao()
            .get_latest_completed_job(window_days)
            .await
            .map_err(|e| e.to_string())
    }

    async fn get_genres_by_job(&self, job_id: uuid::Uuid) -> Result<Vec<GenreWithSummary>, String> {
        self.dao()
            .get_genres_by_job(job_id)
            .await
            .map_err(|e| e.to_string())
    }

    async fn get_clusters_by_job(
        &self,
        job_id: uuid::Uuid,
    ) -> Result<std::collections::HashMap<String, Vec<ClusterWithEvidence>>, String> {
        self.dao()
            .get_clusters_by_job(job_id)
            .await
            .map_err(|e| e.to_string())
    }

    async fn get_recap_output_body_json(
        &self,
        job_id: uuid::Uuid,
        genre: &str,
    ) -> Result<Option<Value>, String> {
        self.dao()
            .get_recap_output_body_json(job_id, genre)
            .await
            .map_err(|e| e.to_string())
    }

    async fn get_latest_three_day_recap_run(&self) -> Result<Option<SurfaceRun>, String> {
        SurfaceDaoOps::get_latest_three_day_recap_run(self.pool())
            .await
            .map(|opt| opt.map(|r| SurfaceRun::new(r.job_id, &r.status, r.kicked_at, r.updated_at)))
            .map_err(|e| e.to_string())
    }

    fn observe_cluster_query_duration(&self, duration_secs: f64) {
        self.telemetry()
            .metrics()
            .api_cluster_query_duration
            .observe(duration_secs);
    }

    fn inc_evidence_duplicates(&self, removed: usize) {
        #[allow(clippy::cast_precision_loss)]
        {
            self.telemetry()
                .metrics()
                .api_evidence_duplicates
                .inc_by(removed as f64);
        }
    }

    fn observe_latest_fetch_duration(&self, duration_secs: f64) {
        self.telemetry()
            .metrics()
            .api_latest_fetch_duration
            .observe(duration_secs);
    }
}

pub async fn get_3days_recap_impl(
    dao: &impl RecapReadDao,
    freshness_budget: chrono::Duration,
    now: chrono::DateTime<chrono::Utc>,
) -> axum::response::Response {
    get_recap_by_window(dao, 3, "3-day", Some((freshness_budget, now))).await
}

/// GET /v1/recaps/7days
/// 最新の7日間Recapデータを取得する
pub(crate) async fn get_7days_recap(State(state): State<AppState>) -> impl IntoResponse {
    get_recap_by_window(&state, 7, "7-day", None).await
}

/// GET /v1/recaps/3days
/// 最新の3日間Recapデータを取得する
pub(crate) async fn get_3days_recap(State(state): State<AppState>) -> impl IntoResponse {
    let budget = state.config().surface_freshness().three_day_recap_budget();
    get_3days_recap_impl(&state, budget, chrono::Utc::now()).await
}

/// 指定されたウィンドウ日数のRecapデータを取得する
#[allow(clippy::too_many_lines)]
async fn get_recap_by_window(
    dao: &impl RecapReadDao,
    window_days: i32,
    label: &str,
    surface_clock: Option<(chrono::Duration, chrono::DateTime<chrono::Utc>)>,
) -> axum::response::Response {
    info!("Fetching latest {} recap", label);
    let handler_start = Instant::now();

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
    dao.observe_cluster_query_duration(cluster_query_start.elapsed().as_secs_f64());

    // レスポンス構築
    let total_genres = genres.len();
    let mut degraded_genres = 0usize;
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
                dao.inc_evidence_duplicates(removed);
            }
            deduped
        };

        // Extract references from body_json
        let references = match dao
            .get_recap_output_body_json(job.job_id, &genre.genre_name)
            .await
        {
            Ok(Some(body_json)) => {
                if is_body_json_degraded(&body_json) {
                    degraded_genres += 1;
                }
                extract_references_from_body_json(&body_json)
            }
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

    let surface_status = if let Some((budget, now)) = surface_clock {
        let latest_run = match dao.get_latest_three_day_recap_run().await {
            Ok(run) => run,
            Err(e) => {
                error!("Failed to fetch latest 3-day recap run: {}", e);
                return (
                    StatusCode::INTERNAL_SERVER_ERROR,
                    Json(ErrorResponse {
                        error: "Failed to fetch recap data".to_string(),
                    }),
                )
                    .into_response();
            }
        };
        Some(build_surface_status(
            job.window_end,
            Some((job.window_start, job.window_end)),
            latest_run,
            three_day_recap_reasons(degraded_genres, total_genres),
            budget,
            now,
        ))
    } else {
        None
    };

    let response = RecapSummaryResponse {
        job_id: job.job_id.to_string(),
        executed_at: job.started_at.to_rfc3339(),
        window_start: job.window_start.to_rfc3339(),
        window_end: job.window_end.to_rfc3339(),
        total_articles: job.total_articles.unwrap_or(0),
        genres: genre_responses,
        surface_status,
    };

    info!(
        "Successfully fetched {} recap for job {}",
        label, job.job_id
    );
    dao.observe_latest_fetch_duration(handler_start.elapsed().as_secs_f64());
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

    #[test]
    fn clamp_helpers_enforce_bounds() {
        assert_eq!(clamp_search_limit(None), 50);
        assert_eq!(clamp_search_limit(Some(0)), 1);
        assert_eq!(clamp_search_limit(Some(-50)), 1);
        assert_eq!(clamp_search_limit(Some(500)), 200);

        assert_eq!(clamp_indexable_limit(None), 200);
        assert_eq!(clamp_indexable_limit(Some(0)), 1);
        assert_eq!(clamp_indexable_limit(Some(-10)), 1);
        assert_eq!(clamp_indexable_limit(Some(5000)), 1000);

        assert_eq!(clamp_offset(None), 0);
        assert_eq!(clamp_offset(Some(-10)), 0);
        assert_eq!(clamp_offset(Some(25_000)), 1000);
    }

    #[tokio::test]
    #[allow(clippy::await_holding_lock, clippy::too_many_lines)]
    async fn search_recaps_applies_offset_and_limit() {
        use std::sync::Arc;

        use axum::{body::Body, http::Request};
        use chrono::{DateTime, Utc};
        use tower::ServiceExt;
        use uuid::Uuid;

        use crate::{
            app::{ComponentRegistry, build_router},
            config::{Config, ENV_MUTEX},
            store::{dao::mock::MockRecapDao, models::RecapSearchHit},
        };

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

        let ts = DateTime::parse_from_rfc3339("2026-04-02T00:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let mock_dao = Arc::new(MockRecapDao::new());
        mock_dao.set_search_recaps_hits(
            (1..=1000_u128)
                .map(|i| RecapSearchHit {
                    job_id: Uuid::from_u128(i),
                    executed_at: ts,
                    window_days: 7,
                    genre: format!("genre-{i}"),
                    summary_ja: Some(format!("Hit {i}")),
                    top_terms: vec!["LLM".to_string()],
                    tags: vec!["ai".to_string()],
                })
                .collect(),
        );

        let app = build_router(registry.with_recap_dao(mock_dao));
        let request = Request::builder()
            .uri("/v1/recaps/search?term=LLM&limit=1&offset=1")
            .body(Body::empty())
            .expect("request builds");

        let response = app
            .clone()
            .oneshot(request)
            .await
            .expect("request succeeds");
        assert_eq!(response.status(), StatusCode::OK);
        let body_bytes = axum::body::to_bytes(response.into_body(), usize::MAX)
            .await
            .expect("body bytes");
        let payload: Value = serde_json::from_slice(&body_bytes).expect("valid json");
        let results = payload["results"].as_array().expect("results array");
        assert_eq!(results.len(), 1);
        assert_eq!(results[0]["job_id"], Uuid::from_u128(2).to_string());
        assert_eq!(results[0]["genre"], "genre-2");

        let tail_req = Request::builder()
            .uri("/v1/recaps/search?term=LLM&limit=50&offset=960")
            .body(Body::empty())
            .expect("request builds");
        let tail_resp = app
            .clone()
            .oneshot(tail_req)
            .await
            .expect("request succeeds");
        assert_eq!(tail_resp.status(), StatusCode::OK);
        let tail_bytes = axum::body::to_bytes(tail_resp.into_body(), usize::MAX)
            .await
            .expect("body bytes");
        let tail_json: Value = serde_json::from_slice(&tail_bytes).expect("valid json");
        let tail_results = tail_json["results"].as_array().expect("results array");
        assert_eq!(tail_results.len(), 40);
        assert_eq!(tail_results[0]["job_id"], Uuid::from_u128(961).to_string());
        assert_eq!(
            tail_results[39]["job_id"],
            Uuid::from_u128(1000).to_string()
        );

        let capped_req = Request::builder()
            .uri("/v1/recaps/search?term=LLM&limit=50&offset=1000")
            .body(Body::empty())
            .expect("request builds");
        let capped_resp = app.oneshot(capped_req).await.expect("request succeeds");
        assert_eq!(capped_resp.status(), StatusCode::OK);
        let capped_bytes = axum::body::to_bytes(capped_resp.into_body(), usize::MAX)
            .await
            .expect("body bytes");
        let capped_json: Value = serde_json::from_slice(&capped_bytes).expect("valid json");
        let capped_results = capped_json["results"].as_array().expect("results array");
        assert_eq!(capped_results.len(), 0);
    }

    #[tokio::test]
    #[allow(clippy::await_holding_lock)]
    async fn get_indexable_genres_returns_400_on_invalid_or_offsetless_since() {
        use crate::{
            app::{ComponentRegistry, build_router},
            config::{Config, ENV_MUTEX},
        };
        use axum::{body::Body, http::Request};
        use tower::ServiceExt;

        let _lock = ENV_MUTEX
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        let registry = temp_env::async_with_vars(
            [
                (
                    "RECAP_DB_DSN",
                    Some("postgres://recap:recap@127.0.0.1:15432/recap"),
                ),
                ("RECAP_DB_ACQUIRE_TIMEOUT_SECS", Some("1")),
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

        let app = build_router(registry);

        for invalid_since in ["not-a-date", "2026-11-08T10:00:00"] {
            let uri = format!("/v1/recaps/genres/indexable?since={invalid_since}");
            let request = Request::get(&uri)
                .body(Body::empty())
                .expect("request builds");
            let response = app
                .clone()
                .oneshot(request)
                .await
                .expect("request succeeds");
            assert_eq!(
                response.status(),
                StatusCode::BAD_REQUEST,
                "expected 400 for since={invalid_since}"
            );

            let body_bytes = axum::body::to_bytes(response.into_body(), usize::MAX)
                .await
                .expect("body bytes");
            let payload: Value = serde_json::from_slice(&body_bytes).expect("valid json");
            assert!(
                payload["error"].as_str().is_some_and(|s| !s.is_empty()),
                "error field must be non-empty string for since={invalid_since}"
            );
        }
    }

    struct MockRecapReadDao {
        job: Option<crate::store::models::RecapJob>,
        genres: Vec<crate::store::models::GenreWithSummary>,
        body_jsons: std::collections::HashMap<String, Value>,
        latest_run: Option<crate::api::surface_status::SurfaceRun>,
    }

    #[async_trait::async_trait]
    impl RecapReadDao for MockRecapReadDao {
        async fn get_latest_completed_job(
            &self,
            _window_days: i32,
        ) -> Result<Option<crate::store::models::RecapJob>, String> {
            Ok(self.job.clone())
        }

        async fn get_genres_by_job(
            &self,
            _job_id: uuid::Uuid,
        ) -> Result<Vec<crate::store::models::GenreWithSummary>, String> {
            Ok(self.genres.clone())
        }

        async fn get_clusters_by_job(
            &self,
            _job_id: uuid::Uuid,
        ) -> Result<
            std::collections::HashMap<String, Vec<crate::store::models::ClusterWithEvidence>>,
            String,
        > {
            Ok(std::collections::HashMap::new())
        }

        async fn get_recap_output_body_json(
            &self,
            _job_id: uuid::Uuid,
            genre: &str,
        ) -> Result<Option<Value>, String> {
            Ok(self.body_jsons.get(genre).cloned())
        }

        async fn get_latest_three_day_recap_run(
            &self,
        ) -> Result<Option<crate::api::surface_status::SurfaceRun>, String> {
            Ok(self.latest_run.clone())
        }

        fn observe_cluster_query_duration(&self, _duration_secs: f64) {}

        fn inc_evidence_duplicates(&self, _removed: usize) {}

        fn observe_latest_fetch_duration(&self, _duration_secs: f64) {}
    }

    #[tokio::test]
    async fn get_3days_recap_serves_surface_status_and_extractive_genres_reason() {
        use chrono::{DateTime, Utc};
        use serde_json::json;
        use uuid::Uuid;

        let job_id = Uuid::parse_str("11111111-2222-3333-4444-555555555555").unwrap();
        let started_at = DateTime::parse_from_rfc3339("2026-11-08T12:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let window_start = DateTime::parse_from_rfc3339("2026-11-05T12:05:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let window_end = DateTime::parse_from_rfc3339("2026-11-08T12:05:00Z")
            .unwrap()
            .with_timezone(&Utc);

        let mut body_jsons = std::collections::HashMap::new();
        body_jsons.insert(
            "ai_data".to_string(),
            json!({
                "summary": { "title": "AI", "bullets": ["a"] },
                "metadata": { "is_degraded": true }
            }),
        );
        body_jsons.insert(
            "space_astronomy".to_string(),
            json!({
                "summary": { "title": "Space", "bullets": ["b"] },
                "metadata": { "is_degraded": false }
            }),
        );

        let dao = MockRecapReadDao {
            job: Some(crate::store::models::RecapJob {
                job_id,
                started_at,
                window_start,
                window_end,
                total_articles: Some(12),
            }),
            genres: vec![
                crate::store::models::GenreWithSummary {
                    genre_name: "ai_data".to_string(),
                    summary_ja: Some("AI summary".to_string()),
                    title_ja: None,
                    bullets_ja: None,
                },
                crate::store::models::GenreWithSummary {
                    genre_name: "space_astronomy".to_string(),
                    summary_ja: Some("Space summary".to_string()),
                    title_ja: None,
                    bullets_ja: None,
                },
            ],
            body_jsons,
            latest_run: Some(crate::api::surface_status::SurfaceRun {
                job_id: job_id.to_string(),
                status: "completed".to_string(),
                kicked_at: "2026-11-08T12:00:00Z".to_string(),
                updated_at: "2026-11-08T12:05:00Z".to_string(),
            }),
        };

        let now = DateTime::parse_from_rfc3339("2026-11-09T00:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let response = get_3days_recap_impl(&dao, chrono::Duration::hours(30), now).await;
        assert_eq!(response.status(), StatusCode::OK);

        let body_bytes = axum::body::to_bytes(response.into_body(), usize::MAX)
            .await
            .expect("body bytes");
        let body: Value = serde_json::from_slice(&body_bytes).expect("valid json");

        assert_eq!(
            body["surface_status"]["generated_at"],
            "2026-11-08T12:05:00Z"
        );
        assert_eq!(
            body["surface_status"]["window_start"],
            "2026-11-05T12:05:00Z"
        );
        assert_eq!(body["surface_status"]["window_end"], "2026-11-08T12:05:00Z");
        assert_eq!(body["surface_status"]["is_stale"], false);
        assert_eq!(
            body["surface_status"]["stale_after"],
            "2026-11-09T18:05:00Z"
        );
        assert_eq!(body["surface_status"]["latest_run"]["status"], "completed");
        assert_eq!(
            body["surface_status"]["degradation_reasons"],
            json!([
                {
                    "code": "extractive_genres",
                    "detail": "1 of 2 genres are extractive excerpts, not AI summaries"
                }
            ])
        );
    }

    #[tokio::test]
    async fn get_7days_recap_omits_surface_status_key() {
        use chrono::{DateTime, Utc};
        use uuid::Uuid;

        let job_id = Uuid::parse_str("11111111-2222-3333-4444-555555555555").unwrap();
        let started_at = DateTime::parse_from_rfc3339("2026-11-08T12:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let window_start = DateTime::parse_from_rfc3339("2026-11-01T12:05:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let window_end = DateTime::parse_from_rfc3339("2026-11-08T12:05:00Z")
            .unwrap()
            .with_timezone(&Utc);

        let dao = MockRecapReadDao {
            job: Some(crate::store::models::RecapJob {
                job_id,
                started_at,
                window_start,
                window_end,
                total_articles: Some(25),
            }),
            genres: vec![crate::store::models::GenreWithSummary {
                genre_name: "ai_data".to_string(),
                summary_ja: Some("7-day AI summary".to_string()),
                title_ja: None,
                bullets_ja: None,
            }],
            body_jsons: std::collections::HashMap::new(),
            latest_run: None,
        };

        let response = get_recap_by_window(&dao, 7, "7-day", None).await;
        assert_eq!(response.status(), StatusCode::OK);

        let body_bytes = axum::body::to_bytes(response.into_body(), usize::MAX)
            .await
            .expect("body bytes");
        let body: Value = serde_json::from_slice(&body_bytes).expect("valid json");

        assert!(
            !body
                .as_object()
                .expect("response is object")
                .contains_key("surface_status")
        );
    }

    struct MockMorningDao {
        letter: Option<crate::store::models::MorningLetter>,
        latest_run: Option<crate::api::surface_status::SurfaceRun>,
        window: Option<(chrono::DateTime<chrono::Utc>, chrono::DateTime<chrono::Utc>)>,
    }

    #[async_trait::async_trait]
    impl MorningLetterReadDao for MockMorningDao {
        async fn get_latest_morning_letter(
            &self,
        ) -> Result<Option<crate::store::models::MorningLetter>, String> {
            Ok(self.letter.clone())
        }

        async fn get_morning_letter_by_id(
            &self,
            id: uuid::Uuid,
        ) -> Result<Option<crate::store::models::MorningLetter>, String> {
            Ok(self.letter.as_ref().filter(|l| l.id == id).cloned())
        }

        async fn get_morning_letter_by_date(
            &self,
            _date: chrono::NaiveDate,
        ) -> Result<Option<crate::store::models::MorningLetter>, String> {
            Ok(self.letter.clone())
        }

        async fn get_latest_morning_run(
            &self,
        ) -> Result<Option<crate::api::surface_status::SurfaceRun>, String> {
            Ok(self.latest_run.clone())
        }

        async fn get_recap_job_window(
            &self,
            _job_id: uuid::Uuid,
        ) -> Result<Option<(chrono::DateTime<chrono::Utc>, chrono::DateTime<chrono::Utc>)>, String>
        {
            Ok(self.window)
        }
    }

    #[tokio::test]
    async fn morning_letter_serves_surface_status_and_degradation_reason() {
        use chrono::{DateTime, NaiveDate, Utc};
        use serde_json::json;
        use uuid::Uuid;

        let letter_id = Uuid::parse_str("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee").unwrap();
        let recap_job_id = Uuid::parse_str("11111111-2222-3333-4444-555555555555").unwrap();
        let created_at = DateTime::parse_from_rfc3339("2026-11-09T06:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let win_start = DateTime::parse_from_rfc3339("2026-11-06T06:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let win_end = DateTime::parse_from_rfc3339("2026-11-09T06:00:00Z")
            .unwrap()
            .with_timezone(&Utc);

        let letter = crate::store::models::MorningLetter {
            id: letter_id,
            target_date: NaiveDate::from_ymd_opt(2026, 11, 9).unwrap(),
            edition_timezone: "Asia/Tokyo".to_string(),
            source_recap_job_id: Some(recap_job_id),
            is_degraded: true,
            schema_version: 1,
            generation_revision: 1,
            result_jsonb: json!({
                "lead": "Morning lead",
                "sections": [
                    {
                        "key": "top3",
                        "title": "Top Stories",
                        "bullets": ["Bullet 1"]
                    }
                ],
                "generated_at": "2026-11-09T06:00:00Z"
            }),
            model: Some("gemma4-e4b-12k".to_string()),
            generation_metadata_jsonb: json!({
                "degradation_reason": "placeholder_output"
            }),
            created_at,
        };

        let dao = MockMorningDao {
            letter: Some(letter),
            latest_run: Some(crate::api::surface_status::SurfaceRun {
                job_id: recap_job_id.to_string(),
                status: "completed".to_string(),
                kicked_at: "2026-11-09T05:59:00Z".to_string(),
                updated_at: "2026-11-09T06:00:00Z".to_string(),
            }),
            window: Some((win_start, win_end)),
        };

        let now = DateTime::parse_from_rfc3339("2026-11-09T10:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let response = get_latest_morning_letter_impl(&dao, chrono::Duration::hours(3), now).await;
        assert_eq!(response.status(), StatusCode::OK);

        let body_bytes = axum::body::to_bytes(response.into_body(), usize::MAX)
            .await
            .expect("body bytes");
        let body: Value = serde_json::from_slice(&body_bytes).expect("valid json");

        assert_eq!(
            body["surface_status"]["generated_at"],
            "2026-11-09T06:00:00Z"
        );
        assert_eq!(
            body["surface_status"]["window_start"],
            "2026-11-06T06:00:00Z"
        );
        assert_eq!(body["surface_status"]["window_end"], "2026-11-09T06:00:00Z");
        assert_eq!(body["surface_status"]["is_stale"], true);
        assert_eq!(
            body["surface_status"]["stale_after"],
            "2026-11-09T09:00:00Z"
        );
        assert_eq!(body["surface_status"]["latest_run"]["status"], "completed");
        assert_eq!(
            body["surface_status"]["degradation_reasons"],
            json!([
                {
                    "code": "placeholder_output",
                    "detail": ""
                }
            ])
        );
    }

    async fn start_regenerate_mock_server() -> wiremock::MockServer {
        use serde_json::json;
        use wiremock::matchers::{method, path};
        use wiremock::{Mock, MockServer, ResponseTemplate};

        let mock_server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path(
                "/services.datahub.v1.DataHubService/ListRecapArticles",
            ))
            .respond_with(ResponseTemplate::new(200).set_body_json(json!({
                "articles": [],
                "hasMore": false
            })))
            .mount(&mock_server)
            .await;
        Mock::given(method("POST"))
            .and(path("/v1/morning-letter/generate"))
            .respond_with(ResponseTemplate::new(200).set_body_json(json!({
                "target_date": "2026-11-09",
                "edition_timezone": "Asia/Tokyo",
                "content": {
                    "schema_version": 1,
                    "lead": "Regenerated on-demand lead",
                    "sections": [
                        {
                            "key": "top3",
                            "title": "Top Stories",
                            "bullets": ["Regenerated bullet"]
                        }
                    ],
                    "source_recap_window_days": 3
                },
                "metadata": {
                    "model": "gemma4-e4b-12k",
                    "is_degraded": false,
                    "degradation_reason": null,
                    "processing_time_ms": 420
                }
            })))
            .mount(&mock_server)
            .await;
        mock_server
    }

    #[tokio::test]
    #[allow(clippy::await_holding_lock)]
    async fn regenerate_morning_letter_returns_its_own_written_revision_when_newer_revision_exists()
    {
        use crate::{
            app::ComponentRegistry,
            config::{Config, ENV_MUTEX},
            store::dao::mock::MockRecapDao,
        };
        use chrono::{DateTime, NaiveDate, Utc};
        use serde_json::json;
        use std::sync::Arc;
        use uuid::Uuid;

        let mock_server = start_regenerate_mock_server().await;

        let concurrent_letter = crate::store::models::MorningLetter {
            id: Uuid::parse_str("99999999-8888-7777-6666-555555555555").unwrap(),
            target_date: NaiveDate::from_ymd_opt(2026, 11, 9).unwrap(),
            edition_timezone: "Asia/Tokyo".to_string(),
            source_recap_job_id: None,
            is_degraded: false,
            schema_version: 1,
            generation_revision: 2,
            result_jsonb: json!({
                "lead": "Concurrent scheduled lead",
                "sections": [
                    {
                        "key": "top3",
                        "title": "Concurrent Stories",
                        "bullets": ["Concurrent bullet"]
                    }
                ],
                "generated_at": "2026-11-09T06:05:00Z"
            }),
            model: Some("gemma4-e4b-12k".to_string()),
            generation_metadata_jsonb: json!({}),
            created_at: DateTime::parse_from_rfc3339("2026-11-09T06:05:00Z")
                .unwrap()
                .with_timezone(&Utc),
        };

        let dao = Arc::new(MockRecapDao::new());
        dao.set_concurrent_morning_letter_on_save(concurrent_letter);

        let server_uri = mock_server.uri();

        let env_lock = ENV_MUTEX
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        let registry = temp_env::async_with_vars(
            [
                (
                    "RECAP_DB_DSN",
                    Some("postgres://recap:recap@localhost:5432/recap"),
                ),
                ("NEWS_CREATOR_BASE_URL", Some(server_uri.as_str())),
                ("SUBWORKER_BASE_URL", Some(server_uri.as_str())),
                ("ALT_BACKEND_BASE_URL", Some(server_uri.as_str())),
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
                    .with_recap_dao(dao.clone())
            },
        )
        .await;
        drop(env_lock);

        let state = AppState::new(registry);
        let budget = state.config().surface_freshness().morning_letter_budget();
        let response =
            regenerate_morning_letter_impl(state.scheduler(), dao.as_ref(), budget, Utc::now())
                .await;

        assert_eq!(response.status(), StatusCode::OK);
        let body_bytes = axum::body::to_bytes(response.into_body(), usize::MAX)
            .await
            .expect("body bytes");
        let body: Value = serde_json::from_slice(&body_bytes).expect("valid json");

        let persisted = dao.morning_letters();
        assert_eq!(
            persisted.len(),
            2,
            "two revisions must be present for the same date"
        );
        assert_eq!(body["id"], json!(persisted[0].id.to_string()));
        assert_eq!(body["generation_revision"], json!(1));
        assert_eq!(body["body"]["lead"], json!("Regenerated on-demand lead"));
    }

    #[tokio::test]
    #[allow(clippy::await_holding_lock)]
    async fn regenerate_morning_letter_rejects_concurrent_run_and_releases_after_completion_or_failure()
     {
        use crate::{
            app::{AppState, ComponentRegistry},
            config::{Config, ENV_MUTEX},
            store::dao::mock::MockRecapDao,
        };
        use chrono::Utc;
        use serde_json::json;
        use std::sync::{
            Arc,
            atomic::{AtomicBool, AtomicUsize, Ordering},
        };
        use std::time::Duration;
        use wiremock::matchers::{method, path};
        use wiremock::{Mock, MockServer, Request as WireRequest, Respond, ResponseTemplate};

        struct CountingResponder {
            calls: Arc<AtomicUsize>,
            fail_next: Arc<AtomicBool>,
        }

        impl Respond for CountingResponder {
            fn respond(&self, _request: &WireRequest) -> ResponseTemplate {
                self.calls.fetch_add(1, Ordering::SeqCst);
                if self.fail_next.swap(false, Ordering::SeqCst) {
                    return ResponseTemplate::new(500).set_body_string("upstream error");
                }
                ResponseTemplate::new(200)
                    .set_delay(Duration::from_millis(250))
                    .set_body_json(json!({
                        "target_date": "2026-11-09",
                        "edition_timezone": "Asia/Tokyo",
                        "content": {
                            "schema_version": 1,
                            "lead": "Single-flight lead",
                            "sections": [
                                {
                                    "key": "top3",
                                    "title": "Top Stories",
                                    "bullets": ["Single-flight bullet"]
                                }
                            ],
                            "source_recap_window_days": 3
                        },
                        "metadata": {
                            "model": "gemma4-e4b-12k",
                            "is_degraded": false,
                            "degradation_reason": null,
                            "processing_time_ms": 100
                        }
                    }))
            }
        }

        let mock_server = MockServer::start().await;
        let generate_calls = Arc::new(AtomicUsize::new(0));
        let fail_next = Arc::new(AtomicBool::new(false));

        Mock::given(method("POST"))
            .and(path(
                "/services.datahub.v1.DataHubService/ListRecapArticles",
            ))
            .respond_with(ResponseTemplate::new(200).set_body_json(json!({
                "articles": [],
                "hasMore": false
            })))
            .mount(&mock_server)
            .await;
        Mock::given(method("POST"))
            .and(path("/v1/morning-letter/generate"))
            .respond_with(CountingResponder {
                calls: Arc::clone(&generate_calls),
                fail_next: Arc::clone(&fail_next),
            })
            .mount(&mock_server)
            .await;

        let dao = Arc::new(MockRecapDao::new());
        let server_uri = mock_server.uri();

        let env_lock = ENV_MUTEX
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        let registry = temp_env::async_with_vars(
            [
                (
                    "RECAP_DB_DSN",
                    Some("postgres://recap:recap@127.0.0.1:15432/recap"),
                ),
                ("CLASSIFICATION_QUEUE_CONCURRENCY", Some("0")),
                ("NEWS_CREATOR_BASE_URL", Some(server_uri.as_str())),
                ("SUBWORKER_BASE_URL", Some(server_uri.as_str())),
                ("ALT_BACKEND_BASE_URL", Some(server_uri.as_str())),
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
                    .with_recap_dao(dao.clone())
            },
        )
        .await;
        drop(env_lock);

        let state = AppState::new(registry);
        let budget = state.config().surface_freshness().morning_letter_budget();

        let state1 = state.clone();
        let dao1 = Arc::clone(&dao);
        let state2 = state.clone();
        let dao2 = Arc::clone(&dao);
        let (resp1, resp2) = tokio::join!(
            async move {
                regenerate_morning_letter_impl(
                    state1.scheduler(),
                    dao1.as_ref(),
                    budget,
                    Utc::now(),
                )
                .await
            },
            async move {
                tokio::time::sleep(Duration::from_millis(30)).await;
                regenerate_morning_letter_impl(
                    state2.scheduler(),
                    dao2.as_ref(),
                    budget,
                    Utc::now(),
                )
                .await
            }
        );

        assert_eq!(resp1.status(), StatusCode::OK);
        assert_eq!(
            resp2.status(),
            StatusCode::CONFLICT,
            "in-flight morning regeneration must reject concurrent request with 409"
        );
        assert_eq!(
            generate_calls.load(Ordering::SeqCst),
            1,
            "concurrent request must not start a second morning pipeline"
        );

        fail_next.store(true, Ordering::SeqCst);
        let failed_resp =
            regenerate_morning_letter_impl(state.scheduler(), dao.as_ref(), budget, Utc::now())
                .await;
        assert_eq!(failed_resp.status(), StatusCode::INTERNAL_SERVER_ERROR);

        let recovered_resp =
            regenerate_morning_letter_impl(state.scheduler(), dao.as_ref(), budget, Utc::now())
                .await;
        assert_eq!(
            recovered_resp.status(),
            StatusCode::OK,
            "morning regeneration lock must be released after failure"
        );
        assert_eq!(generate_calls.load(Ordering::SeqCst), 3);
    }

    #[allow(clippy::await_holding_lock)]
    async fn build_test_registry_with_dsn(dsn: &str) -> crate::app::ComponentRegistry {
        use crate::{
            app::ComponentRegistry,
            config::{Config, ENV_MUTEX},
        };

        let env_lock = ENV_MUTEX
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        let registry = temp_env::async_with_vars(
            [
                ("RECAP_DB_DSN", Some(dsn)),
                ("RECAP_DB_MAX_CONNECTIONS", Some("1")),
                ("RECAP_DB_MIN_CONNECTIONS", Some("0")),
                ("RECAP_DB_ACQUIRE_TIMEOUT_SECS", Some("1")),
                ("CLASSIFICATION_QUEUE_CONCURRENCY", Some("0")),
                ("NEWS_CREATOR_BASE_URL", Some("http://127.0.0.1:18001/")),
                ("SUBWORKER_BASE_URL", Some("http://127.0.0.1:18002/")),
                ("ALT_BACKEND_BASE_URL", Some("http://127.0.0.1:19000/")),
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
        registry
    }

    async fn create_morning_letter_temp_tables(pool: &sqlx::PgPool) {
        for ddl in [
            r"
            CREATE TEMP TABLE recap_jobs (
                job_id UUID PRIMARY KEY,
                kicked_at TIMESTAMPTZ NOT NULL,
                status TEXT NOT NULL,
                last_stage TEXT,
                window_days INTEGER NOT NULL DEFAULT 3,
                trigger_source TEXT NOT NULL DEFAULT 'system',
                note TEXT,
                updated_at TIMESTAMPTZ NOT NULL
            )
            ",
            r"
            CREATE TEMP TABLE recap_outputs (
                job_id UUID NOT NULL,
                genre TEXT NOT NULL,
                created_at TIMESTAMPTZ NOT NULL
            )
            ",
            r"
            CREATE TEMP TABLE morning_letters (
                id UUID PRIMARY KEY,
                target_date DATE NOT NULL,
                edition_timezone TEXT NOT NULL,
                source_recap_job_id UUID,
                is_degraded BOOLEAN NOT NULL DEFAULT FALSE,
                schema_version INTEGER NOT NULL DEFAULT 1,
                generation_revision INTEGER NOT NULL DEFAULT 1,
                result_jsonb JSONB NOT NULL,
                model TEXT,
                generation_metadata_jsonb JSONB NOT NULL DEFAULT '{}'::jsonb,
                created_at TIMESTAMPTZ NOT NULL
            )
            ",
        ] {
            sqlx::query(ddl)
                .execute(pool)
                .await
                .expect("create temp table");
        }
    }

    #[tokio::test]
    async fn get_latest_morning_letter_reads_surface_data_from_state_pool() {
        use chrono::{DateTime, Duration, Utc};
        use serde_json::json;
        use uuid::Uuid;

        let unreachable_state = AppState::new(
            build_test_registry_with_dsn("postgres://recap:recap@127.0.0.1:1/recap").await,
        );
        let run_res = MorningLetterReadDao::get_latest_morning_run(&unreachable_state).await;
        assert!(
            run_res.is_err(),
            "get_latest_morning_run must query state.pool(), got: {run_res:?}"
        );
        let win_res =
            MorningLetterReadDao::get_recap_job_window(&unreachable_state, Uuid::new_v4()).await;
        assert!(
            win_res.is_err(),
            "get_recap_job_window must query state.pool(), got: {win_res:?}"
        );

        let Ok(database_url) = std::env::var("DATABASE_URL") else {
            return;
        };

        let state = AppState::new(build_test_registry_with_dsn(&database_url).await);
        let pool = state.pool();
        create_morning_letter_temp_tables(pool).await;

        let now = Utc::now();
        let recap_job_id = Uuid::new_v4();
        let morning_job_id = Uuid::new_v4();
        let letter_id = Uuid::new_v4();
        let recap_kicked_at = DateTime::parse_from_rfc3339("2026-01-26T00:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let morning_kicked_at = now - Duration::minutes(10);

        sqlx::query(
            r"
            INSERT INTO recap_jobs (job_id, kicked_at, status, window_days, trigger_source, updated_at)
            VALUES ($1, $2, 'completed', 3, 'system', $2),
                   ($3, $4, 'morning_completed', 1, 'morning', $4)
            ",
        )
        .bind(recap_job_id)
        .bind(recap_kicked_at)
        .bind(morning_job_id)
        .bind(morning_kicked_at)
        .execute(pool)
        .await
        .expect("insert recap_jobs");

        sqlx::query(
            r"
            INSERT INTO morning_letters (
                id, target_date, edition_timezone, source_recap_job_id, is_degraded,
                schema_version, generation_revision, result_jsonb, model,
                generation_metadata_jsonb, created_at
            )
            VALUES ($1, '2026-01-26', 'Asia/Tokyo', $2, FALSE, 1, 1, $3, 'gemma4-e4b-12k', '{}'::jsonb, $4)
            ",
        )
        .bind(letter_id)
        .bind(recap_job_id)
        .bind(json!({
            "lead": "Production DAO morning lead",
            "sections": [{"key": "top3", "title": "Top Stories", "bullets": ["Bullet 1"]}],
            "generated_at": now.to_rfc3339()
        }))
        .bind(now)
        .execute(pool)
        .await
        .expect("insert morning_letter");

        let response = get_latest_morning_letter(State(state))
            .await
            .into_response();
        assert_eq!(response.status(), StatusCode::OK);
        let body_bytes = axum::body::to_bytes(response.into_body(), usize::MAX)
            .await
            .expect("body bytes");
        let body: Value = serde_json::from_slice(&body_bytes).expect("valid json");
        assert_eq!(
            body["surface_status"]["latest_run"]["job_id"],
            json!(morning_job_id.to_string()),
            "latest_run must be read from state.pool()"
        );
        assert_eq!(
            body["surface_status"]["window_end"],
            json!("2026-01-26T00:00:00Z"),
            "recap job window must be read from state.pool()"
        );
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
    surface_status: SurfaceStatus,
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
            previous_letter_ref: content
                .previous_letter_ref
                .map(|p| PreviousLetterRefResponse {
                    id: p.id,
                    target_date: p.target_date,
                    through_line: p.through_line,
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
    surface_status: SurfaceStatus,
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
            surface_status,
        }),
    ))
}

#[async_trait::async_trait]
pub trait MorningLetterReadDao: Send + Sync {
    async fn get_latest_morning_letter(&self) -> Result<Option<MorningLetter>, String>;
    async fn get_morning_letter_by_id(
        &self,
        id: uuid::Uuid,
    ) -> Result<Option<MorningLetter>, String>;
    async fn get_morning_letter_by_date(
        &self,
        date: chrono::NaiveDate,
    ) -> Result<Option<MorningLetter>, String>;
    async fn get_latest_morning_run(&self) -> Result<Option<SurfaceRun>, String>;
    async fn get_recap_job_window(
        &self,
        job_id: uuid::Uuid,
    ) -> Result<Option<(chrono::DateTime<chrono::Utc>, chrono::DateTime<chrono::Utc>)>, String>;
}

#[async_trait::async_trait]
impl MorningLetterReadDao for AppState {
    async fn get_latest_morning_letter(&self) -> Result<Option<MorningLetter>, String> {
        self.dao()
            .get_latest_morning_letter()
            .await
            .map_err(|e| e.to_string())
    }

    async fn get_morning_letter_by_id(
        &self,
        id: uuid::Uuid,
    ) -> Result<Option<MorningLetter>, String> {
        self.dao()
            .get_morning_letter_by_id(id)
            .await
            .map_err(|e| e.to_string())
    }

    async fn get_morning_letter_by_date(
        &self,
        date: chrono::NaiveDate,
    ) -> Result<Option<MorningLetter>, String> {
        self.dao()
            .get_morning_letter_by_date(date)
            .await
            .map_err(|e| e.to_string())
    }

    async fn get_latest_morning_run(&self) -> Result<Option<SurfaceRun>, String> {
        SurfaceDaoOps::get_latest_morning_run(self.pool())
            .await
            .map(|opt| opt.map(|r| SurfaceRun::new(r.job_id, &r.status, r.kicked_at, r.updated_at)))
            .map_err(|e| e.to_string())
    }

    async fn get_recap_job_window(
        &self,
        job_id: uuid::Uuid,
    ) -> Result<Option<(chrono::DateTime<chrono::Utc>, chrono::DateTime<chrono::Utc>)>, String>
    {
        SurfaceDaoOps::get_recap_job_window(self.pool(), job_id)
            .await
            .map_err(|e| e.to_string())
    }
}

#[cfg(test)]
#[async_trait::async_trait]
impl MorningLetterReadDao for crate::store::dao::mock::MockRecapDao {
    async fn get_latest_morning_letter(&self) -> Result<Option<MorningLetter>, String> {
        crate::store::dao::RecapDao::get_latest_morning_letter(self)
            .await
            .map_err(|e| e.to_string())
    }

    async fn get_morning_letter_by_id(
        &self,
        id: uuid::Uuid,
    ) -> Result<Option<MorningLetter>, String> {
        crate::store::dao::RecapDao::get_morning_letter_by_id(self, id)
            .await
            .map_err(|e| e.to_string())
    }

    async fn get_morning_letter_by_date(
        &self,
        date: chrono::NaiveDate,
    ) -> Result<Option<MorningLetter>, String> {
        crate::store::dao::RecapDao::get_morning_letter_by_date(self, date)
            .await
            .map_err(|e| e.to_string())
    }

    async fn get_latest_morning_run(&self) -> Result<Option<SurfaceRun>, String> {
        Ok(None)
    }

    async fn get_recap_job_window(
        &self,
        _job_id: uuid::Uuid,
    ) -> Result<Option<(chrono::DateTime<chrono::Utc>, chrono::DateTime<chrono::Utc>)>, String>
    {
        Ok(None)
    }
}

async fn build_morning_letter_surface_status(
    dao: &impl MorningLetterReadDao,
    letter: &MorningLetter,
    freshness_budget: chrono::Duration,
    now: chrono::DateTime<chrono::Utc>,
) -> Result<SurfaceStatus, String> {
    let latest_run = dao.get_latest_morning_run().await?;
    let window = match letter.source_recap_job_id {
        Some(job_id) => dao.get_recap_job_window(job_id).await?,
        None => None,
    };
    let reasons = morning_letter_reasons(letter.is_degraded, &letter.generation_metadata_jsonb);
    Ok(build_surface_status(
        letter.created_at,
        window,
        latest_run,
        reasons,
        freshness_budget,
        now,
    ))
}

pub async fn get_latest_morning_letter_impl(
    dao: &impl MorningLetterReadDao,
    freshness_budget: chrono::Duration,
    now: chrono::DateTime<chrono::Utc>,
) -> axum::response::Response {
    match dao.get_latest_morning_letter().await {
        Ok(Some(letter)) => {
            let surface_status = match build_morning_letter_surface_status(
                dao,
                &letter,
                freshness_budget,
                now,
            )
            .await
            {
                Ok(status) => status,
                Err(e) => {
                    error!("Failed to build morning letter surface status: {}", e);
                    return (
                        StatusCode::INTERNAL_SERVER_ERROR,
                        Json(ErrorResponse {
                            error: "Failed to fetch latest morning letter".to_string(),
                        }),
                    )
                        .into_response();
                }
            };
            match map_morning_letter_response(&letter, surface_status) {
                Ok((headers, json)) => (StatusCode::OK, headers, json).into_response(),
                Err((status, json)) => (status, json).into_response(),
            }
        }
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

/// GET /v1/morning/letters/latest
/// Edition timezone 基準での最新の Morning Letter を返す。
pub(crate) async fn get_latest_morning_letter(State(state): State<AppState>) -> impl IntoResponse {
    let budget = state.config().surface_freshness().morning_letter_budget();
    get_latest_morning_letter_impl(&state, budget, chrono::Utc::now()).await
}

pub async fn get_morning_letter_by_date_impl(
    dao: &impl MorningLetterReadDao,
    target_date: &str,
    freshness_budget: chrono::Duration,
    now: chrono::DateTime<chrono::Utc>,
) -> axum::response::Response {
    let Ok(date) = chrono::NaiveDate::parse_from_str(target_date, "%Y-%m-%d") else {
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

    match dao.get_morning_letter_by_date(date).await {
        Ok(Some(letter)) => {
            let surface_status = match build_morning_letter_surface_status(
                dao,
                &letter,
                freshness_budget,
                now,
            )
            .await
            {
                Ok(status) => status,
                Err(e) => {
                    error!(
                        target_date = %target_date,
                        "Failed to build morning letter surface status: {}", e
                    );
                    return (
                        StatusCode::INTERNAL_SERVER_ERROR,
                        Json(ErrorResponse {
                            error: "Failed to fetch morning letter".to_string(),
                        }),
                    )
                        .into_response();
                }
            };
            match map_morning_letter_response(&letter, surface_status) {
                Ok((headers, json)) => (StatusCode::OK, headers, json).into_response(),
                Err((status, json)) => (status, json).into_response(),
            }
        }
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

/// GET /v1/morning/letters/{target_date}
/// 指定日の Morning Letter を返す。日付形式: YYYY-MM-DD (edition timezone)。
pub(crate) async fn get_morning_letter_by_date(
    State(state): State<AppState>,
    Path(target_date): Path<String>,
) -> impl IntoResponse {
    let budget = state.config().surface_freshness().morning_letter_budget();
    get_morning_letter_by_date_impl(&state, &target_date, budget, chrono::Utc::now()).await
}

#[derive(Debug, serde::Deserialize, Default)]
pub(crate) struct RegenerateLatestRequest {
    #[serde(default)]
    #[allow(dead_code)]
    pub edition_timezone: Option<String>,
}

pub(crate) async fn regenerate_morning_letter_impl(
    scheduler: &crate::scheduler::Scheduler,
    dao: &impl MorningLetterReadDao,
    freshness_budget: chrono::Duration,
    now: chrono::DateTime<chrono::Utc>,
) -> axum::response::Response {
    use crate::scheduler::JobContext;

    let Some(permit) = scheduler.try_acquire_morning_permit() else {
        return (
            StatusCode::CONFLICT,
            Json(ErrorResponse {
                error: "Morning Letter regeneration is already in progress".to_string(),
            }),
        )
            .into_response();
    };

    let job_id = uuid::Uuid::new_v4();
    // Morning regenerate: tag the row with trigger_source='morning' so a
    // later crash can't cause boot-time find_resumable_job to mis-resume
    // this as a full batch Recap pipeline.
    let context = JobContext::new_morning_update(job_id);

    let letter_id = match scheduler.run_morning_update_locked(context, permit).await {
        Ok(id) => id,
        Err(e) => {
            error!(job_id = %job_id, "morning regenerate pipeline failed: {}", e);
            return (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(ErrorResponse {
                    error: "Morning Letter regeneration failed".to_string(),
                }),
            )
                .into_response();
        }
    };

    match dao.get_morning_letter_by_id(letter_id).await {
        Ok(Some(letter)) => {
            let surface_status = match build_morning_letter_surface_status(
                dao,
                &letter,
                freshness_budget,
                now,
            )
            .await
            {
                Ok(status) => status,
                Err(e) => {
                    error!(
                        "Failed to build regenerated morning letter surface status: {}",
                        e
                    );
                    return (
                        StatusCode::INTERNAL_SERVER_ERROR,
                        Json(ErrorResponse {
                            error: "Failed to fetch regenerated letter".to_string(),
                        }),
                    )
                        .into_response();
                }
            };
            match map_morning_letter_response(&letter, surface_status) {
                Ok((headers, json)) => (StatusCode::OK, headers, json).into_response(),
                Err((status, json)) => (status, json).into_response(),
            }
        }
        Ok(None) => (
            StatusCode::INTERNAL_SERVER_ERROR,
            Json(ErrorResponse {
                error: "Regeneration completed but no letter persisted".to_string(),
            }),
        )
            .into_response(),
        Err(e) => {
            error!("Failed to fetch regenerated letter: {}", e);
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(ErrorResponse {
                    error: "Failed to fetch regenerated letter".to_string(),
                }),
            )
                .into_response()
        }
    }
}

/// POST /v1/morning/letters/regenerate
/// On-demand regeneration. Runs the morning pipeline synchronously and
/// returns the freshly persisted letter.
pub(crate) async fn regenerate_morning_letter(
    State(state): State<AppState>,
    Json(_req): Json<RegenerateLatestRequest>,
) -> impl IntoResponse {
    let budget = state.config().surface_freshness().morning_letter_budget();
    regenerate_morning_letter_impl(state.scheduler(), &state, budget, chrono::Utc::now()).await
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
