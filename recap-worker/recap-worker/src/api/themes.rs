//! Theme page endpoints (`GET /v1/themes/{theme_id}`).

use axum::{
    Json,
    extract::{Path, Query, State, rejection::QueryRejection},
    http::StatusCode,
    response::IntoResponse,
};
use chrono::SecondsFormat;
use serde::{Deserialize, Serialize};
use sqlx::PgPool;
use tracing::error;
use uuid::Uuid;

use crate::app::AppState;
use crate::store::dao::themes::ThemesDaoOps;
pub use crate::store::dao::themes::{
    ThemeBody, ThemeClaim, ThemeSource, ThemeTimelineEntry, ThemeVersionDetail, ThemeVersionSummary,
};

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ThemeVersionHistoryResponse {
    pub version: i32,
    pub created_at: String,
    pub article_count: i32,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct GetThemePageResponse {
    pub theme_id: Uuid,
    pub title: String,
    pub version: i32,
    pub latest_version: i32,
    pub created_at: String,
    pub key_points: Vec<ThemeClaim>,
    pub timeline: Vec<ThemeTimelineEntry>,
    pub what_changed: Vec<ThemeClaim>,
    pub open_questions: Vec<ThemeClaim>,
    pub sources: Vec<ThemeSource>,
    pub versions: Vec<ThemeVersionHistoryResponse>,
}

#[derive(Debug, Serialize)]
pub(crate) struct ErrorResponse {
    pub error: String,
}

#[derive(Debug, Deserialize)]
pub struct GetThemeRawQuery {
    pub user_id: Option<String>,
    pub version: Option<String>,
}

#[async_trait::async_trait]
pub trait ThemeReadDao: Send + Sync {
    async fn get_theme_version(
        &self,
        user_id: Uuid,
        theme_id: Uuid,
        version: Option<i32>,
    ) -> Result<Option<ThemeVersionDetail>, String>;

    async fn list_theme_versions(
        &self,
        user_id: Uuid,
        theme_id: Uuid,
    ) -> Result<Vec<ThemeVersionSummary>, String>;
}

#[async_trait::async_trait]
impl ThemeReadDao for PgPool {
    async fn get_theme_version(
        &self,
        user_id: Uuid,
        theme_id: Uuid,
        version: Option<i32>,
    ) -> Result<Option<ThemeVersionDetail>, String> {
        ThemesDaoOps::get_theme_version(self, user_id, theme_id, version)
            .await
            .map_err(|e| e.to_string())
    }

    async fn list_theme_versions(
        &self,
        user_id: Uuid,
        theme_id: Uuid,
    ) -> Result<Vec<ThemeVersionSummary>, String> {
        ThemesDaoOps::list_theme_versions(self, user_id, theme_id)
            .await
            .map_err(|e| e.to_string())
    }
}

fn bad_request(message: impl Into<String>) -> axum::response::Response {
    (
        StatusCode::BAD_REQUEST,
        Json(ErrorResponse {
            error: message.into(),
        }),
    )
        .into_response()
}

pub async fn get_theme_page_impl(
    dao: &impl ThemeReadDao,
    raw_theme_id: &str,
    raw_query: Result<Query<GetThemeRawQuery>, QueryRejection>,
) -> axum::response::Response {
    let Ok(theme_id) = Uuid::parse_str(raw_theme_id.trim()) else {
        return bad_request("invalid theme_id");
    };

    let Ok(Query(query)) = raw_query else {
        return bad_request("invalid query parameters");
    };

    let Some(raw_user_id) = query
        .user_id
        .as_deref()
        .map(str::trim)
        .filter(|s| !s.is_empty())
    else {
        return bad_request("user_id is required");
    };

    let Ok(user_id) = Uuid::parse_str(raw_user_id) else {
        return bad_request("invalid user_id");
    };
    if user_id.is_nil() {
        return bad_request("user_id must not be nil");
    }

    let version = match query.version.as_deref().map(str::trim) {
        Some("") => return bad_request("invalid version"),
        Some(raw_ver) => {
            let Ok(parsed) = raw_ver.parse::<i32>() else {
                return bad_request("invalid version");
            };
            if parsed < 1 {
                return bad_request("version must be >= 1");
            }
            Some(parsed)
        }
        None => None,
    };

    let detail = match dao.get_theme_version(user_id, theme_id, version).await {
        Ok(Some(detail)) => detail,
        Ok(None) => {
            return (
                StatusCode::NOT_FOUND,
                Json(ErrorResponse {
                    error: "theme not found".to_string(),
                }),
            )
                .into_response();
        }
        Err(err) => {
            error!(%theme_id, %user_id, error = %err, "failed to fetch theme version");
            return (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(ErrorResponse {
                    error: "failed to fetch theme".to_string(),
                }),
            )
                .into_response();
        }
    };

    let mut summaries = match dao.list_theme_versions(user_id, theme_id).await {
        Ok(list) => list,
        Err(err) => {
            error!(%theme_id, %user_id, error = %err, "failed to list theme versions");
            return (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(ErrorResponse {
                    error: "failed to fetch theme versions".to_string(),
                }),
            )
                .into_response();
        }
    };
    summaries.sort_by_key(|a| std::cmp::Reverse(a.version));

    let versions = summaries
        .into_iter()
        .map(|s| ThemeVersionHistoryResponse {
            version: s.version,
            created_at: s.created_at.to_rfc3339_opts(SecondsFormat::Secs, true),
            article_count: s.article_count,
        })
        .collect();

    (
        StatusCode::OK,
        Json(GetThemePageResponse {
            theme_id: detail.theme_id,
            title: detail.title,
            version: detail.version,
            latest_version: detail.latest_version,
            created_at: detail.created_at.to_rfc3339_opts(SecondsFormat::Secs, true),
            key_points: detail.body.key_points,
            timeline: detail.body.timeline,
            what_changed: detail.body.what_changed,
            open_questions: detail.body.open_questions,
            sources: detail.body.sources,
            versions,
        }),
    )
        .into_response()
}

pub(crate) async fn get_theme_page(
    State(state): State<AppState>,
    Path(theme_id): Path<String>,
    query: Result<Query<GetThemeRawQuery>, QueryRejection>,
) -> impl IntoResponse {
    get_theme_page_impl(state.pool(), &theme_id, query).await
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::store::dao::themes::ThemeBody;
    use axum::extract::FromRequestParts;
    use axum::http::Request;
    use chrono::TimeZone;
    use serde_json::Value;
    use std::sync::Mutex;

    #[derive(Default)]
    struct MockThemeReadDao {
        versions: Mutex<Vec<(Uuid, ThemeVersionDetail, i32)>>,
    }

    impl MockThemeReadDao {
        fn insert(&self, user_id: Uuid, detail: ThemeVersionDetail, article_count: i32) {
            self.versions
                .lock()
                .unwrap()
                .push((user_id, detail, article_count));
        }
    }

    #[async_trait::async_trait]
    impl ThemeReadDao for MockThemeReadDao {
        async fn get_theme_version(
            &self,
            user_id: Uuid,
            theme_id: Uuid,
            version: Option<i32>,
        ) -> Result<Option<ThemeVersionDetail>, String> {
            let lock = self.versions.lock().unwrap();
            let matching: Vec<&ThemeVersionDetail> = lock
                .iter()
                .filter(|(u, d, _)| *u == user_id && d.theme_id == theme_id)
                .map(|(_, d, _)| d)
                .collect();
            let Some(latest) = matching.iter().map(|d| d.version).max() else {
                return Ok(None);
            };
            let target = match version {
                Some(v) => matching.into_iter().find(|d| d.version == v),
                None => matching.into_iter().max_by_key(|d| d.version),
            };
            Ok(target.map(|d| {
                let mut cloned = d.clone();
                cloned.latest_version = latest;
                cloned
            }))
        }

        async fn list_theme_versions(
            &self,
            user_id: Uuid,
            theme_id: Uuid,
        ) -> Result<Vec<ThemeVersionSummary>, String> {
            let lock = self.versions.lock().unwrap();
            let mut out: Vec<ThemeVersionSummary> = lock
                .iter()
                .filter(|(u, d, _)| *u == user_id && d.theme_id == theme_id)
                .map(|(_, d, count)| ThemeVersionSummary {
                    version_id: d.version_id,
                    version: d.version,
                    title: d.title.clone(),
                    created_at: d.created_at,
                    article_count: *count,
                })
                .collect();
            out.sort_by_key(|a| std::cmp::Reverse(a.version));
            Ok(out)
        }
    }

    async fn extract_query(uri: &str) -> Result<Query<GetThemeRawQuery>, QueryRejection> {
        let req = Request::builder().uri(uri).body(()).unwrap();
        let (mut parts, ()) = req.into_parts();
        Query::<GetThemeRawQuery>::from_request_parts(&mut parts, &()).await
    }

    fn sample_detail(
        theme_id: Uuid,
        version: i32,
        ts: chrono::DateTime<chrono::Utc>,
    ) -> ThemeVersionDetail {
        let art = Uuid::parse_str("33333333-3333-3333-3333-333333333333").unwrap();
        ThemeVersionDetail {
            theme_id,
            version_id: Uuid::new_v4(),
            version,
            latest_version: version,
            title: format!("Systems Architecture v{version}"),
            body: ThemeBody {
                key_points: vec![ThemeClaim {
                    text: "Event logs keep projections rebuildable.".to_string(),
                    source_article_ids: vec![art],
                }],
                timeline: vec![ThemeTimelineEntry {
                    date: "2026-11-15".to_string(),
                    text: "Initial event schema adopted.".to_string(),
                    source_article_ids: vec![art],
                }],
                what_changed: vec![ThemeClaim {
                    text: "Added versioned projection notes.".to_string(),
                    source_article_ids: vec![art],
                }],
                open_questions: vec![ThemeClaim {
                    text: "How should backfill windows be bounded?".to_string(),
                    source_article_ids: vec![art],
                }],
                sources: vec![ThemeSource {
                    article_id: art,
                    title: "Event Sourcing Notes".to_string(),
                    url: "https://example.com/event-sourcing".to_string(),
                }],
            },
            member_article_ids: vec![art],
            created_at: ts,
        }
    }

    #[tokio::test]
    async fn returns_200_for_latest_and_specific_version_with_newest_first_history() {
        let dao = MockThemeReadDao::default();
        let user_id = Uuid::parse_str("22222222-2222-2222-2222-222222222222").unwrap();
        let theme_id = Uuid::parse_str("44444444-4444-4444-4444-444444444444").unwrap();
        let t1 = chrono::Utc.with_ymd_and_hms(2026, 11, 17, 8, 0, 0).unwrap();
        let t2 = chrono::Utc.with_ymd_and_hms(2026, 11, 18, 8, 0, 0).unwrap();

        dao.insert(user_id, sample_detail(theme_id, 1, t1), 3);
        dao.insert(user_id, sample_detail(theme_id, 2, t2), 5);

        let q = extract_query(&format!("/v1/themes/{theme_id}?user_id={user_id}")).await;
        let resp = get_theme_page_impl(&dao, &theme_id.to_string(), q).await;
        assert_eq!(resp.status(), StatusCode::OK);

        let bytes = axum::body::to_bytes(resp.into_body(), 1024 * 1024)
            .await
            .unwrap();
        let body: Value = serde_json::from_slice(&bytes).unwrap();
        assert_eq!(body["theme_id"], theme_id.to_string());
        assert_eq!(body["version"], 2);
        assert_eq!(body["latest_version"], 2);
        assert_eq!(body["created_at"], "2026-11-18T08:00:00Z");
        assert_eq!(body["versions"][0]["version"], 2);
        assert_eq!(body["versions"][0]["article_count"], 5);
        assert_eq!(body["versions"][1]["version"], 1);
        assert_eq!(body["versions"][1]["article_count"], 3);

        let q_v1 = extract_query(&format!(
            "/v1/themes/{theme_id}?user_id={user_id}&version=1"
        ))
        .await;
        let resp_v1 = get_theme_page_impl(&dao, &theme_id.to_string(), q_v1).await;
        assert_eq!(resp_v1.status(), StatusCode::OK);
        let bytes_v1 = axum::body::to_bytes(resp_v1.into_body(), 1024 * 1024)
            .await
            .unwrap();
        let body_v1: Value = serde_json::from_slice(&bytes_v1).unwrap();
        assert_eq!(body_v1["version"], 1);
        assert_eq!(body_v1["latest_version"], 2);
    }

    #[tokio::test]
    async fn returns_404_for_missing_theme_missing_version_and_other_user() {
        let dao = MockThemeReadDao::default();
        let owner_id = Uuid::new_v4();
        let other_user = Uuid::new_v4();
        let theme_id = Uuid::new_v4();
        let t1 = chrono::Utc.with_ymd_and_hms(2026, 11, 18, 8, 0, 0).unwrap();
        dao.insert(owner_id, sample_detail(theme_id, 1, t1), 3);

        let missing_theme = Uuid::new_v4();
        let q1 = extract_query(&format!("/v1/themes/{missing_theme}?user_id={owner_id}")).await;
        let r1 = get_theme_page_impl(&dao, &missing_theme.to_string(), q1).await;
        assert_eq!(r1.status(), StatusCode::NOT_FOUND);

        let q2 = extract_query(&format!(
            "/v1/themes/{theme_id}?user_id={owner_id}&version=99"
        ))
        .await;
        let r2 = get_theme_page_impl(&dao, &theme_id.to_string(), q2).await;
        assert_eq!(r2.status(), StatusCode::NOT_FOUND);

        let q3 = extract_query(&format!("/v1/themes/{theme_id}?user_id={other_user}")).await;
        let r3 = get_theme_page_impl(&dao, &theme_id.to_string(), q3).await;
        assert_eq!(r3.status(), StatusCode::NOT_FOUND);
        let bytes = axum::body::to_bytes(r3.into_body(), 1024 * 1024)
            .await
            .unwrap();
        let body: Value = serde_json::from_slice(&bytes).unwrap();
        assert!(body["error"].is_string());
    }

    #[tokio::test]
    async fn returns_400_json_for_malformed_inputs() {
        let dao = MockThemeReadDao::default();
        let valid_id = Uuid::new_v4();

        let q_ok = extract_query(&format!("/v1/themes/bad?user_id={valid_id}")).await;
        let r_bad_theme = get_theme_page_impl(&dao, "not-a-uuid", q_ok).await;
        assert_eq!(r_bad_theme.status(), StatusCode::BAD_REQUEST);
        let b1: Value = serde_json::from_slice(
            &axum::body::to_bytes(r_bad_theme.into_body(), 1024 * 1024)
                .await
                .unwrap(),
        )
        .unwrap();
        assert!(b1["error"].is_string());

        for uri in [
            format!("/v1/themes/{valid_id}"),
            format!("/v1/themes/{valid_id}?user_id=bad-uuid"),
            format!("/v1/themes/{valid_id}?user_id={}", Uuid::nil()),
            format!("/v1/themes/{valid_id}?user_id={valid_id}&version=0"),
            format!("/v1/themes/{valid_id}?user_id={valid_id}&version=-2"),
            format!("/v1/themes/{valid_id}?user_id={valid_id}&version=abc"),
        ] {
            let q = extract_query(&uri).await;
            let resp = get_theme_page_impl(&dao, &valid_id.to_string(), q).await;
            assert_eq!(
                resp.status(),
                StatusCode::BAD_REQUEST,
                "expected 400 for {uri}"
            );
            let b: Value = serde_json::from_slice(
                &axum::body::to_bytes(resp.into_body(), 1024 * 1024)
                    .await
                    .unwrap(),
            )
            .unwrap();
            assert!(b["error"].is_string(), "expected error field for {uri}");
        }
    }
}
