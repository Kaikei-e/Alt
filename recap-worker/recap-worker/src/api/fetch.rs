use axum::{Json, extract::State, http::StatusCode, response::IntoResponse};
use serde::Serialize;
use tracing::{error, info};

use crate::app::AppState;
use crate::store::models::{PersistedGenre, PersistedCluster, ClusterEvidence};

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
pub(crate) async fn get_7days_recap(
    State(state): State<AppState>,
) -> impl IntoResponse {
    info!("Fetching latest 7-day recap");

    // 最新のジョブを取得
    let job_result = match state.dao().get_latest_completed_job(7).await {
        Ok(Some(job)) => job,
        Ok(None) => {
            info!("No completed 7-day recap found");
            return (
                StatusCode::NOT_FOUND,
                Json(ErrorResponse {
                    error: "No 7-day recap found".to_string(),
                }),
            ).into_response();
        }
        Err(e) => {
            error!("Failed to fetch latest job: {}", e);
            return (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(ErrorResponse {
                    error: "Failed to fetch recap data".to_string(),
                }),
            ).into_response();
        }
    };

    // ジャンルデータを取得
    let genres = match state.dao().get_genres_by_job(job.job_id).await {
        Ok(genres) => genres,
        Err(e) => {
            error!("Failed to fetch genres for job {}: {}", job.job_id, e);
            return (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(ErrorResponse {
                    error: "Failed to fetch genre data".to_string(),
                }),
            ).into_response();
        }
    };

    // レスポンス構築
    let mut genre_responses = Vec::new();
    for genre in genres {
        // クラスターを取得
        let clusters = match state.dao().get_clusters_by_genre(job.job_id, &genre.genre_name).await {
            Ok(clusters) => clusters,
            Err(e) => {
                error!("Failed to fetch clusters for genre {}: {}", genre.genre_name, e);
                continue;
            }
        };

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
                    lang: evidence.lang.clone().unwrap_or_else(|| "unknown".to_string()),
                });
            }
        }

        // 重複除去して最大5個のトップタームを取得
        all_top_terms.sort();
        all_top_terms.dedup();
        all_top_terms.truncate(5);

        genre_responses.push(RecapGenreResponse {
            genre: genre.genre_name.clone(),
            summary: genre.summary_ja.clone().unwrap_or_default(),
            top_terms: all_top_terms,
            article_count: total_article_count,
            cluster_count: clusters.len() as i32,
            evidence_links,
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
    (StatusCode::OK, Json(response)).into_response()
}

