use std::{collections::HashSet, time::Instant};

use axum::{Json, extract::State, http::StatusCode, response::IntoResponse};
use serde::Serialize;
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
}

#[derive(Debug, Serialize)]
pub(crate) struct EvidenceLinkResponse {
    article_id: String,
    title: String,
    source_url: String,
    published_at: String,
    lang: String,
}

fn dedupe_evidence_links(links: Vec<EvidenceLinkResponse>) -> Vec<EvidenceLinkResponse> {
    let mut seen_ids = HashSet::new();
    let mut unique_links = Vec::with_capacity(links.len());

    for link in links.into_iter() {
        if seen_ids.insert(link.article_id.clone()) {
            unique_links.push(link);
        }
    }

    unique_links
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
            total_article_count += cluster.evidence.len() as i32;

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
        all_top_terms.truncate(5);

        let deduped_links = {
            let before = evidence_links.len();
            let deduped = dedupe_evidence_links(evidence_links);
            let removed = before.saturating_sub(deduped.len());
            if removed > 0 {
                metrics
                    .api_evidence_duplicates
                    .inc_by(removed as u64);
            }
            deduped
        };

        genre_responses.push(RecapGenreResponse {
            genre: genre.genre_name.clone(),
            summary: genre.summary_ja.clone().unwrap_or_default(),
            top_terms: all_top_terms,
            article_count: total_article_count,
            cluster_count: clusters.len() as i32,
            evidence_links: deduped_links,
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
        let article_ids: Vec<_> = deduped.iter().map(|link| link.article_id.as_str()).collect();

        assert_eq!(article_ids, vec!["a", "b", "c"]);
    }
}
