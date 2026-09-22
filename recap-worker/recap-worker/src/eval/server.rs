//! Eval HTTP Listener router and handlers for topic card evaluation.
//!
//! Provides the evaluation UI and APIs behind admin Bearer auth on port 9006:
//! - GET /eval/
//! - GET /v1/eval/windows
//! - GET /v1/eval/windows/{id}/candidates
//! - POST /v1/eval/judgments (201 / 400)
//! - GET /v1/eval/jobs
//! - GET /v1/eval/jobs/{job_id}/cards
//! - POST /v1/eval/ratings (201 / 400)

use crate::error::Result;
use crate::store::dao::cards::{
    CardsDaoOps, CardsJobSummary, RecapCard, RecapCardCandidate, RecapCardRating, RecapEvalWindow,
    RecapEvalWindowSummary, RecapStoryJudgment,
};
use axum::{
    Json, Router,
    extract::{Path, State},
    http::StatusCode,
    response::Html,
    routing::{get, post},
};
use chrono::Utc;
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use uuid::Uuid;

pub const EVAL_PAGE_HTML: &str = include_str!("../../assets/recap-eval/index.html");

/// Trait abstracting DB operations for the eval router (enables testability without live DB).
#[async_trait::async_trait]
pub(crate) trait EvalDao: Send + Sync {
    async fn get_eval_window(&self, window_id: Uuid) -> Result<Option<RecapEvalWindow>>;
    async fn list_eval_windows(&self) -> Result<Vec<RecapEvalWindowSummary>>;
    async fn get_candidates_for_window(&self, window_id: Uuid) -> Result<Vec<RecapCardCandidate>>;
    async fn latest_judgments_for_window(&self, window_id: Uuid)
    -> Result<Vec<RecapStoryJudgment>>;
    async fn insert_story_judgment(&self, judgment: &RecapStoryJudgment) -> Result<()>;
    async fn list_cards_jobs(&self, limit: i64) -> Result<Vec<CardsJobSummary>>;
    async fn get_cards_for_job(&self, job_id: Uuid) -> Result<Vec<RecapCard>>;
    async fn latest_rating_per_card(&self, job_id: Uuid) -> Result<Vec<RecapCardRating>>;
    async fn insert_card_rating(&self, rating: &RecapCardRating) -> Result<()>;
}

#[async_trait::async_trait]
impl EvalDao for sqlx::PgPool {
    async fn get_eval_window(&self, window_id: Uuid) -> Result<Option<RecapEvalWindow>> {
        CardsDaoOps::get_eval_window(self, window_id).await
    }
    async fn list_eval_windows(&self) -> Result<Vec<RecapEvalWindowSummary>> {
        CardsDaoOps::list_eval_windows(self).await
    }
    async fn get_candidates_for_window(&self, window_id: Uuid) -> Result<Vec<RecapCardCandidate>> {
        CardsDaoOps::get_candidates_for_window(self, window_id).await
    }
    async fn latest_judgments_for_window(
        &self,
        window_id: Uuid,
    ) -> Result<Vec<RecapStoryJudgment>> {
        CardsDaoOps::latest_judgments_for_window(self, window_id).await
    }
    async fn insert_story_judgment(&self, judgment: &RecapStoryJudgment) -> Result<()> {
        CardsDaoOps::insert_story_judgment(self, judgment).await
    }
    async fn list_cards_jobs(&self, limit: i64) -> Result<Vec<CardsJobSummary>> {
        CardsDaoOps::list_cards_jobs(self, limit).await
    }
    async fn get_cards_for_job(&self, job_id: Uuid) -> Result<Vec<RecapCard>> {
        CardsDaoOps::get_cards_for_job(self, job_id).await
    }
    async fn latest_rating_per_card(&self, job_id: Uuid) -> Result<Vec<RecapCardRating>> {
        CardsDaoOps::latest_rating_per_card(self, job_id).await
    }
    async fn insert_card_rating(&self, rating: &RecapCardRating) -> Result<()> {
        CardsDaoOps::insert_card_rating(self, rating).await
    }
}

pub(crate) struct EvalServerState {
    pub(crate) dao: Arc<dyn EvalDao>,
}

// Request and response payloads matching assets/recap-eval/index.html

#[derive(Debug, Serialize)]
pub struct WindowsResponse {
    pub windows: Vec<RecapEvalWindowSummary>,
}

#[derive(Debug, Serialize)]
pub struct CandidateWithJudgment {
    #[serde(flatten)]
    pub candidate: RecapCardCandidate,
    pub decision: Option<String>,
}

#[derive(Debug, Serialize)]
pub struct WindowCandidatesResponse {
    pub window: RecapEvalWindow,
    pub candidates: Vec<CandidateWithJudgment>,
}

#[derive(Debug, Deserialize)]
pub struct CreateStoryJudgmentRequest {
    pub id: Option<Uuid>,
    pub window_id: Uuid,
    pub cluster_fingerprint: String,
    pub decision: String,
}

#[derive(Debug, Serialize)]
pub struct JobSummaryItem {
    pub job_id: Uuid,
    pub status: String,
    pub trigger_source: String,
    pub kicked_at: chrono::DateTime<Utc>,
    pub card_count: i64,
    pub rated_count: i64,
}

#[derive(Debug, Serialize)]
pub struct JobsResponse {
    pub jobs: Vec<JobSummaryItem>,
}

#[derive(Debug, Serialize)]
pub struct CardRatingDto {
    pub score: i16,
    pub flags: Vec<String>,
    pub comment: Option<String>,
}

/// Exact contract fields for topic card evaluation without internal pipeline fields.
#[derive(Debug, Serialize)]
pub struct CardItemDto {
    pub id: Uuid,
    pub rank: i32,
    pub headline_ja: String,
    pub what_ja: String,
    pub why_ja: Option<String>,
    pub genre: Option<String>,
    pub continues_card_id: Option<Uuid>,
    pub sources: serde_json::Value,
    pub rating: Option<CardRatingDto>,
}

#[derive(Debug, Serialize)]
pub struct JobCardsResponse {
    pub cards: Vec<CardItemDto>,
}

#[derive(Debug, Deserialize)]
pub struct CreateCardRatingRequest {
    pub id: Option<Uuid>,
    pub card_id: Uuid,
    pub score: i16,
    #[serde(default)]
    pub flags: Vec<String>,
    pub comment: Option<String>,
}

/// Serve static HTML page from memory (embedded at compile-time).
async fn serve_eval_page() -> Html<&'static str> {
    Html(EVAL_PAGE_HTML)
}

/// GET /v1/eval/windows
async fn list_windows(
    State(state): State<Arc<EvalServerState>>,
) -> Result<Json<WindowsResponse>, (StatusCode, String)> {
    let windows = state
        .dao
        .list_eval_windows()
        .await
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;

    Ok(Json(WindowsResponse { windows }))
}

/// GET /v1/eval/windows/{id}/candidates
async fn get_window_candidates(
    State(state): State<Arc<EvalServerState>>,
    Path(window_id): Path<Uuid>,
) -> Result<Json<WindowCandidatesResponse>, (StatusCode, String)> {
    let window = state
        .dao
        .get_eval_window(window_id)
        .await
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?
        .ok_or_else(|| {
            (
                StatusCode::NOT_FOUND,
                format!("eval window '{window_id}' not found"),
            )
        })?;

    let candidates = state
        .dao
        .get_candidates_for_window(window_id)
        .await
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;

    let judgments = state
        .dao
        .latest_judgments_for_window(window_id)
        .await
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;

    let mut judgment_map = std::collections::HashMap::new();
    for j in judgments {
        judgment_map.insert(j.cluster_fingerprint, j.decision);
    }

    let candidates_with_judgments = candidates
        .into_iter()
        .map(|c| {
            let decision = judgment_map.get(&c.cluster_fingerprint).cloned();
            CandidateWithJudgment {
                candidate: c,
                decision,
            }
        })
        .collect();

    Ok(Json(WindowCandidatesResponse {
        window,
        candidates: candidates_with_judgments,
    }))
}

/// POST /v1/eval/judgments
async fn create_judgment(
    State(state): State<Arc<EvalServerState>>,
    Json(req): Json<CreateStoryJudgmentRequest>,
) -> Result<(StatusCode, Json<RecapStoryJudgment>), (StatusCode, String)> {
    if !crate::store::dao::cards::is_valid_decision(&req.decision) {
        return Err((
            StatusCode::BAD_REQUEST,
            format!(
                "invalid decision '{}': must be 'top', 'not_top', or 'noise'",
                req.decision
            ),
        ));
    }

    let judgment = RecapStoryJudgment {
        id: req.id.unwrap_or_else(Uuid::new_v4),
        window_id: req.window_id,
        cluster_fingerprint: req.cluster_fingerprint,
        decision: req.decision,
        rated_at: Utc::now(),
    };

    state
        .dao
        .insert_story_judgment(&judgment)
        .await
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;

    Ok((StatusCode::CREATED, Json(judgment)))
}

/// GET /v1/eval/jobs
async fn list_jobs(
    State(state): State<Arc<EvalServerState>>,
) -> Result<Json<JobsResponse>, (StatusCode, String)> {
    let raw_jobs = state
        .dao
        .list_cards_jobs(50)
        .await
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;

    let items = raw_jobs
        .into_iter()
        .map(|j| JobSummaryItem {
            job_id: j.job_id,
            status: j.status,
            trigger_source: j.trigger_source,
            kicked_at: j.kicked_at,
            card_count: j.card_count,
            rated_count: j.rated_count,
        })
        .collect();

    Ok(Json(JobsResponse { jobs: items }))
}

/// GET /v1/eval/jobs/{job_id}/cards
async fn get_job_cards(
    State(state): State<Arc<EvalServerState>>,
    Path(job_id): Path<Uuid>,
) -> Result<Json<JobCardsResponse>, (StatusCode, String)> {
    let cards = state
        .dao
        .get_cards_for_job(job_id)
        .await
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;

    let ratings = state
        .dao
        .latest_rating_per_card(job_id)
        .await
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;

    let mut rating_map = std::collections::HashMap::new();
    for r in ratings {
        rating_map.insert(
            r.card_id,
            CardRatingDto {
                score: r.score,
                flags: r.flags,
                comment: r.comment,
            },
        );
    }

    let cards_dto = cards
        .into_iter()
        .map(|card| {
            let rating = rating_map.remove(&card.id);
            CardItemDto {
                id: card.id,
                rank: card.rank,
                headline_ja: card.headline_ja,
                what_ja: card.what_ja,
                why_ja: card.why_ja,
                genre: card.genre,
                continues_card_id: card.continues_card_id,
                sources: card.sources,
                rating,
            }
        })
        .collect();

    Ok(Json(JobCardsResponse { cards: cards_dto }))
}

/// POST /v1/eval/ratings
async fn create_rating(
    State(state): State<Arc<EvalServerState>>,
    Json(req): Json<CreateCardRatingRequest>,
) -> Result<(StatusCode, Json<RecapCardRating>), (StatusCode, String)> {
    if !crate::store::dao::cards::is_valid_score(req.score) {
        return Err((
            StatusCode::BAD_REQUEST,
            format!("invalid score {}: must be between 0 and 2", req.score),
        ));
    }

    let rating = RecapCardRating {
        id: req.id.unwrap_or_else(Uuid::new_v4),
        card_id: req.card_id,
        score: req.score,
        flags: req.flags,
        comment: req.comment,
        rated_at: Utc::now(),
    };

    state
        .dao
        .insert_card_rating(&rating)
        .await
        .map_err(|e| (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()))?;

    Ok((StatusCode::CREATED, Json(rating)))
}

/// Build Axum router for eval listener with database pool.
pub fn build_eval_router(pool: sqlx::PgPool, admin_token: &str) -> Router {
    build_eval_router_with_dao(Arc::new(pool), admin_token)
}

/// Build Axum router for eval listener with generic EvalDao (used for tests).
pub(crate) fn build_eval_router_with_dao(dao: Arc<dyn EvalDao>, admin_token: &str) -> Router {
    let state = Arc::new(EvalServerState { dao });
    let admin_guard = crate::api::auth::AdminAuthGuard::new(Some(admin_token));

    let protected = Router::new()
        .route("/v1/eval/windows", get(list_windows))
        .route(
            "/v1/eval/windows/{id}/candidates",
            get(get_window_candidates),
        )
        .route("/v1/eval/judgments", post(create_judgment))
        .route("/v1/eval/jobs", get(list_jobs))
        .route("/v1/eval/jobs/{job_id}/cards", get(get_job_cards))
        .route("/v1/eval/ratings", post(create_rating))
        .route_layer(axum::middleware::from_fn(
            crate::api::auth::require_admin_token,
        ))
        .layer(axum::Extension(admin_guard));

    Router::new()
        .route("/eval", get(serve_eval_page))
        .route("/eval/", get(serve_eval_page))
        .merge(protected)
        .with_state(state)
}

#[cfg(test)]
mod tests {
    use super::*;
    use axum::body::Body;
    use axum::http::{Request, StatusCode, header};
    use std::sync::Mutex;
    use tower::ServiceExt;

    #[derive(Default)]
    struct MockEvalDao {
        windows: Mutex<Vec<RecapEvalWindowSummary>>,
        candidates: Mutex<Vec<RecapCardCandidate>>,
        judgments: Mutex<Vec<RecapStoryJudgment>>,
        jobs: Mutex<Vec<CardsJobSummary>>,
        cards: Mutex<Vec<RecapCard>>,
        ratings: Mutex<Vec<RecapCardRating>>,
    }

    #[async_trait::async_trait]
    impl EvalDao for MockEvalDao {
        async fn get_eval_window(&self, _window_id: Uuid) -> Result<Option<RecapEvalWindow>> {
            Ok(None)
        }

        async fn list_eval_windows(&self) -> Result<Vec<RecapEvalWindowSummary>> {
            Ok(self.windows.lock().unwrap().clone())
        }

        async fn get_candidates_for_window(
            &self,
            _window_id: Uuid,
        ) -> Result<Vec<RecapCardCandidate>> {
            Ok(self.candidates.lock().unwrap().clone())
        }

        async fn latest_judgments_for_window(
            &self,
            _window_id: Uuid,
        ) -> Result<Vec<RecapStoryJudgment>> {
            Ok(self.judgments.lock().unwrap().clone())
        }

        async fn insert_story_judgment(&self, judgment: &RecapStoryJudgment) -> Result<()> {
            self.judgments.lock().unwrap().push(judgment.clone());
            Ok(())
        }

        async fn list_cards_jobs(&self, _limit: i64) -> Result<Vec<CardsJobSummary>> {
            let jobs = self.jobs.lock().unwrap().clone();
            Ok(jobs
                .into_iter()
                .filter(|j| j.trigger_source == "cards" || j.trigger_source == "cards_replay")
                .collect())
        }

        async fn get_cards_for_job(&self, job_id: Uuid) -> Result<Vec<RecapCard>> {
            let cards = self.cards.lock().unwrap();
            Ok(cards
                .iter()
                .filter(|c| c.job_id == job_id)
                .cloned()
                .collect())
        }

        async fn latest_rating_per_card(&self, job_id: Uuid) -> Result<Vec<RecapCardRating>> {
            let card_ids: Vec<Uuid> = self
                .cards
                .lock()
                .unwrap()
                .iter()
                .filter(|c| c.job_id == job_id)
                .map(|c| c.id)
                .collect();
            let ratings = self.ratings.lock().unwrap().clone();
            Ok(ratings
                .into_iter()
                .filter(|r| card_ids.contains(&r.card_id))
                .collect())
        }

        async fn insert_card_rating(&self, rating: &RecapCardRating) -> Result<()> {
            self.ratings.lock().unwrap().push(rating.clone());
            Ok(())
        }
    }

    const TEST_TOKEN: &str = "test-token-12345678901234567890";

    fn setup_test_app() -> (Router, Arc<MockEvalDao>) {
        let dao = Arc::new(MockEvalDao::default());
        let router = build_eval_router_with_dao(dao.clone(), TEST_TOKEN);

        (router, dao)
    }

    #[tokio::test]
    async fn test_serve_eval_page_without_token() {
        let (app, _dao) = setup_test_app();

        for path in ["/eval", "/eval/"] {
            let req = Request::builder().uri(path).body(Body::empty()).unwrap();

            let resp = app.clone().oneshot(req).await.unwrap();
            assert_eq!(
                resp.status(),
                StatusCode::OK,
                "path {path} should return 200 without token"
            );
            assert_eq!(
                resp.headers().get(header::CONTENT_TYPE).unwrap(),
                "text/html; charset=utf-8"
            );
            let body_bytes = axum::body::to_bytes(resp.into_body(), 1024 * 1024)
                .await
                .unwrap();
            assert!(String::from_utf8_lossy(&body_bytes).contains("Alt Recap Evaluation"));
        }
    }

    #[tokio::test]
    async fn test_serve_eval_page_with_valid_token() {
        let (app, _dao) = setup_test_app();

        for path in ["/eval", "/eval/"] {
            let req = Request::builder()
                .uri(path)
                .header(header::AUTHORIZATION, format!("Bearer {TEST_TOKEN}"))
                .body(Body::empty())
                .unwrap();

            let resp = app.clone().oneshot(req).await.unwrap();
            assert_eq!(
                resp.status(),
                StatusCode::OK,
                "path {path} should return 200 with token"
            );
            assert_eq!(
                resp.headers().get(header::CONTENT_TYPE).unwrap(),
                "text/html; charset=utf-8"
            );
            let body_bytes = axum::body::to_bytes(resp.into_body(), 1024 * 1024)
                .await
                .unwrap();
            assert!(String::from_utf8_lossy(&body_bytes).contains("Alt Recap Evaluation"));
        }
    }

    #[tokio::test]
    async fn test_api_routes_require_admin_token() {
        let (app, _dao) = setup_test_app();
        let dummy_id = Uuid::new_v4();

        let endpoints = vec![
            ("GET", "/v1/eval/windows".to_string(), Body::empty()),
            (
                "GET",
                format!("/v1/eval/windows/{dummy_id}/candidates"),
                Body::empty(),
            ),
            (
                "POST",
                "/v1/eval/judgments".to_string(),
                Body::from(
                    r#"{"window_id":"00000000-0000-0000-0000-000000000000","cluster_fingerprint":"fp","decision":"top"}"#,
                ),
            ),
            ("GET", "/v1/eval/jobs".to_string(), Body::empty()),
            (
                "GET",
                format!("/v1/eval/jobs/{dummy_id}/cards"),
                Body::empty(),
            ),
            (
                "POST",
                "/v1/eval/ratings".to_string(),
                Body::from(r#"{"card_id":"00000000-0000-0000-0000-000000000000","score":2}"#),
            ),
        ];

        for (method, uri, body) in endpoints {
            // 1. Without token -> 401 UNAUTHORIZED
            let req_no_token = Request::builder()
                .method(method)
                .uri(&uri)
                .header(header::CONTENT_TYPE, "application/json")
                .body(body)
                .unwrap();

            let resp = app.clone().oneshot(req_no_token).await.unwrap();
            assert_eq!(
                resp.status(),
                StatusCode::UNAUTHORIZED,
                "{method} {uri} must reject unauthenticated requests"
            );

            // 2. With invalid token -> 401 UNAUTHORIZED
            let req_bad_token = Request::builder()
                .method(method)
                .uri(&uri)
                .header(header::AUTHORIZATION, "Bearer invalid-token")
                .header(header::CONTENT_TYPE, "application/json")
                .body(Body::empty())
                .unwrap();

            let resp_bad = app.clone().oneshot(req_bad_token).await.unwrap();
            assert_eq!(
                resp_bad.status(),
                StatusCode::UNAUTHORIZED,
                "{method} {uri} must reject invalid tokens"
            );
        }
    }

    #[tokio::test]
    async fn test_list_windows() {
        let (app, dao) = setup_test_app();

        let window_id = Uuid::new_v4();
        let now = Utc::now();
        dao.windows.lock().unwrap().push(RecapEvalWindowSummary {
            id: window_id,
            from_ts: now,
            to_ts: now,
            snapshot_job_id: Uuid::new_v4(),
            created_at: now,
            candidate_count: 5,
            judged_count: 2,
            params_version: "cards-v0.2".to_string(),
        });

        let req = Request::builder()
            .uri("/v1/eval/windows")
            .header(header::AUTHORIZATION, format!("Bearer {TEST_TOKEN}"))
            .body(Body::empty())
            .unwrap();

        let resp = app.oneshot(req).await.unwrap();
        assert_eq!(resp.status(), StatusCode::OK);
        let body_bytes = axum::body::to_bytes(resp.into_body(), 1024 * 1024)
            .await
            .unwrap();
        let val: serde_json::Value = serde_json::from_slice(&body_bytes).unwrap();
        assert_eq!(val["windows"].as_array().unwrap().len(), 1);
        assert_eq!(val["windows"][0]["id"], window_id.to_string());
        assert_eq!(val["windows"][0]["candidate_count"], 5);
        assert_eq!(val["windows"][0]["judged_count"], 2);
        assert_eq!(val["windows"][0]["params_version"], "cards-v0.2");
        assert!(val["windows"][0]["created_at"].is_string());
    }

    #[tokio::test]
    async fn test_list_windows_newest_first_with_params_version() {
        let (app, dao) = setup_test_app();

        let w1_id = Uuid::new_v4();
        let w2_id = Uuid::new_v4();
        let now = Utc::now();
        let earlier = now - chrono::Duration::hours(1);

        dao.windows.lock().unwrap().extend(vec![
            RecapEvalWindowSummary {
                id: w2_id,
                from_ts: now - chrono::Duration::days(7),
                to_ts: now,
                snapshot_job_id: Uuid::new_v4(),
                created_at: now,
                candidate_count: 10,
                judged_count: 0,
                params_version: "cards-v0.2".to_string(),
            },
            RecapEvalWindowSummary {
                id: w1_id,
                from_ts: now - chrono::Duration::days(7),
                to_ts: now,
                snapshot_job_id: Uuid::new_v4(),
                created_at: earlier,
                candidate_count: 8,
                judged_count: 8,
                params_version: "cards-v0.1".to_string(),
            },
        ]);

        let req = Request::builder()
            .uri("/v1/eval/windows")
            .header(header::AUTHORIZATION, format!("Bearer {TEST_TOKEN}"))
            .body(Body::empty())
            .unwrap();

        let resp = app.oneshot(req).await.unwrap();
        assert_eq!(resp.status(), StatusCode::OK);
        let body_bytes = axum::body::to_bytes(resp.into_body(), 1024 * 1024)
            .await
            .unwrap();
        let val: serde_json::Value = serde_json::from_slice(&body_bytes).unwrap();
        let windows = val["windows"].as_array().unwrap();
        assert_eq!(windows.len(), 2);
        assert_eq!(windows[0]["id"], w2_id.to_string());
        assert_eq!(windows[0]["params_version"], "cards-v0.2");
        assert_eq!(windows[1]["id"], w1_id.to_string());
        assert_eq!(windows[1]["params_version"], "cards-v0.1");
    }

    #[tokio::test]
    async fn test_post_judgment_validation() {
        let (app, _dao) = setup_test_app();

        // 1. Invalid decision should yield 400
        let req_invalid = Request::builder()
            .method("POST")
            .uri("/v1/eval/judgments")
            .header(header::AUTHORIZATION, format!("Bearer {TEST_TOKEN}"))
            .header(header::CONTENT_TYPE, "application/json")
            .body(Body::from(
                serde_json::to_string(&serde_json::json!({
                    "window_id": Uuid::new_v4(),
                    "cluster_fingerprint": "fp1",
                    "decision": "maybe"
                }))
                .unwrap(),
            ))
            .unwrap();

        let resp = app.clone().oneshot(req_invalid).await.unwrap();
        assert_eq!(resp.status(), StatusCode::BAD_REQUEST);

        // 2. Valid decision should yield 201
        let req_valid = Request::builder()
            .method("POST")
            .uri("/v1/eval/judgments")
            .header(header::AUTHORIZATION, format!("Bearer {TEST_TOKEN}"))
            .header(header::CONTENT_TYPE, "application/json")
            .body(Body::from(
                serde_json::to_string(&serde_json::json!({
                    "window_id": Uuid::new_v4(),
                    "cluster_fingerprint": "fp1",
                    "decision": "top"
                }))
                .unwrap(),
            ))
            .unwrap();

        let resp = app.oneshot(req_valid).await.unwrap();
        assert_eq!(resp.status(), StatusCode::CREATED);
    }

    #[tokio::test]
    async fn test_post_rating_validation() {
        let (app, _dao) = setup_test_app();

        // Score 5 is invalid (must be 0..=2) -> 400
        let req_invalid = Request::builder()
            .method("POST")
            .uri("/v1/eval/ratings")
            .header(header::AUTHORIZATION, format!("Bearer {TEST_TOKEN}"))
            .header(header::CONTENT_TYPE, "application/json")
            .body(Body::from(
                serde_json::to_string(&serde_json::json!({
                    "card_id": Uuid::new_v4(),
                    "score": 5,
                    "flags": []
                }))
                .unwrap(),
            ))
            .unwrap();

        let resp = app.clone().oneshot(req_invalid).await.unwrap();
        assert_eq!(resp.status(), StatusCode::BAD_REQUEST);

        // Score 2 is valid -> 201
        let req_valid = Request::builder()
            .method("POST")
            .uri("/v1/eval/ratings")
            .header(header::AUTHORIZATION, format!("Bearer {TEST_TOKEN}"))
            .header(header::CONTENT_TYPE, "application/json")
            .body(Body::from(
                serde_json::to_string(&serde_json::json!({
                    "card_id": Uuid::new_v4(),
                    "score": 2,
                    "flags": ["hallucination"]
                }))
                .unwrap(),
            ))
            .unwrap();

        let resp = app.oneshot(req_valid).await.unwrap();
        assert_eq!(resp.status(), StatusCode::CREATED);
    }

    #[tokio::test]
    async fn test_list_jobs() {
        let (app, dao) = setup_test_app();

        let cards_job_id = Uuid::new_v4();
        let replay_job_id = Uuid::new_v4();
        let other_job_id = Uuid::new_v4();

        // Push a cards job, a cards_replay job, and a non-cards job to test fake filtering
        dao.jobs.lock().unwrap().push(CardsJobSummary {
            job_id: cards_job_id,
            status: "completed".to_string(),
            trigger_source: "cards".to_string(),
            kicked_at: Utc::now(),
            card_count: 10,
            rated_count: 4,
        });
        dao.jobs.lock().unwrap().push(CardsJobSummary {
            job_id: replay_job_id,
            status: "completed".to_string(),
            trigger_source: "cards_replay".to_string(),
            kicked_at: Utc::now(),
            card_count: 8,
            rated_count: 2,
        });
        dao.jobs.lock().unwrap().push(CardsJobSummary {
            job_id: other_job_id,
            status: "completed".to_string(),
            trigger_source: "system".to_string(),
            kicked_at: Utc::now(),
            card_count: 5,
            rated_count: 0,
        });

        let req = Request::builder()
            .uri("/v1/eval/jobs")
            .header(header::AUTHORIZATION, format!("Bearer {TEST_TOKEN}"))
            .body(Body::empty())
            .unwrap();

        let resp = app.oneshot(req).await.unwrap();
        assert_eq!(resp.status(), StatusCode::OK);
        let body_bytes = axum::body::to_bytes(resp.into_body(), 1024 * 1024)
            .await
            .unwrap();
        let val: serde_json::Value = serde_json::from_slice(&body_bytes).unwrap();
        let jobs = val["jobs"].as_array().unwrap();
        assert_eq!(
            jobs.len(),
            2,
            "both trigger_source='cards' and 'cards_replay' jobs should be returned"
        );
        assert_eq!(jobs[0]["job_id"], cards_job_id.to_string());
        assert_eq!(jobs[0]["trigger_source"], "cards");
        assert_eq!(jobs[0]["card_count"], 10);
        assert_eq!(jobs[0]["rated_count"], 4);
        assert_eq!(jobs[1]["job_id"], replay_job_id.to_string());
        assert_eq!(jobs[1]["trigger_source"], "cards_replay");
        assert_eq!(jobs[1]["card_count"], 8);
        assert_eq!(jobs[1]["rated_count"], 2);
    }

    #[tokio::test]
    async fn test_get_job_cards() {
        let (app, dao) = setup_test_app();

        let job_id = Uuid::new_v4();
        let card1_id = Uuid::new_v4();
        let card2_id = Uuid::new_v4();
        let story_id = Uuid::new_v4();

        let card1 = RecapCard {
            id: card1_id,
            job_id,
            rank: 1,
            story_id,
            continues_card_id: None,
            merged_from: None,
            headline_ja: "Headline 1".to_string(),
            what_ja: "What 1".to_string(),
            why_ja: Some("Why 1".to_string()),
            genre: Some("tech".to_string()),
            member_feed_ids: vec![],
            sources: serde_json::json!([{"host": "example.com"}]),
            centroid: None,
            scores: serde_json::json!({}),
            gates: serde_json::json!({}),
            generation: serde_json::json!({}),
            created_at: Utc::now(),
        };

        let card2 = RecapCard {
            id: card2_id,
            job_id,
            rank: 2,
            story_id,
            continues_card_id: Some(card1_id),
            merged_from: None,
            headline_ja: "Headline 2".to_string(),
            what_ja: "What 2".to_string(),
            why_ja: None,
            genre: Some("ai".to_string()),
            member_feed_ids: vec![],
            sources: serde_json::json!([]),
            centroid: None,
            scores: serde_json::json!({}),
            gates: serde_json::json!({}),
            generation: serde_json::json!({}),
            created_at: Utc::now(),
        };

        dao.cards.lock().unwrap().extend(vec![card1, card2]);

        // Rate card2 only
        dao.ratings.lock().unwrap().push(RecapCardRating {
            id: Uuid::new_v4(),
            card_id: card2_id,
            score: 2,
            flags: vec!["accurate".to_string()],
            comment: Some("good card".to_string()),
            rated_at: Utc::now(),
        });

        let req = Request::builder()
            .uri(format!("/v1/eval/jobs/{job_id}/cards"))
            .header(header::AUTHORIZATION, format!("Bearer {TEST_TOKEN}"))
            .body(Body::empty())
            .unwrap();

        let resp = app.oneshot(req).await.unwrap();
        assert_eq!(resp.status(), StatusCode::OK);
        let body_bytes = axum::body::to_bytes(resp.into_body(), 1024 * 1024)
            .await
            .unwrap();
        let val: serde_json::Value = serde_json::from_slice(&body_bytes).unwrap();
        let resp_cards = val["cards"].as_array().unwrap();
        assert_eq!(resp_cards.len(), 2);

        let expected_keys = vec![
            "continues_card_id",
            "genre",
            "headline_ja",
            "id",
            "rank",
            "rating",
            "sources",
            "what_ja",
            "why_ja",
        ];

        for resp_card in resp_cards {
            let obj = resp_card.as_object().unwrap();
            let mut keys: Vec<&str> = obj.keys().map(String::as_str).collect();
            keys.sort_unstable();
            assert_eq!(
                keys, expected_keys,
                "JSON object must have EXACTLY the expected contract keys"
            );
        }

        // card1 is unrated -> rating is null
        assert!(
            resp_cards[0]["rating"].is_null(),
            "unrated card must have rating: null"
        );

        // card2 is rated -> rating is populated
        let rating = &resp_cards[1]["rating"];
        assert!(!rating.is_null(), "rated card must have populated rating");
        assert_eq!(rating["score"], 2);
        assert_eq!(rating["flags"].as_array().unwrap()[0], "accurate");
        assert_eq!(rating["comment"], "good card");
    }
}
