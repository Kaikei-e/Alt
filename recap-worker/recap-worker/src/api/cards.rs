//! Topic cards endpoint (`GET /v1/recaps/3days/cards`).
//!
//! Serves the latest completed topic cards recap for alt-backend,
//! adhering strictly to the consumer pact.

use axum::{Json, extract::State, http::StatusCode, response::IntoResponse};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use sqlx::PgPool;
use tracing::{error, info};
use uuid::Uuid;

use crate::app::AppState;
use crate::pipeline::cards::CARDS_DEGRADED_MIN_CARDS;
use crate::store::dao::cards::CardsDaoOps;
pub use crate::store::dao::cards::{PreviousCardsJobMeta, RecapCard, RecapCardJobStats};

/// Information about a single source article cited in a topic card.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct CardSourceResponse {
    pub feed_id: Uuid,
    pub host: String,
    pub n: i32,
    pub pub_date: Option<String>,
    pub title: String,
    pub url: String,
}

/// A generated 3-day topic card.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct CardResponse {
    pub continues_card_id: Option<Uuid>,
    pub created_at: String,
    pub genre: Option<String>,
    pub headline_ja: String,
    pub id: Uuid,
    pub rank: i32,
    pub sources: Vec<CardSourceResponse>,
    pub story_id: Uuid,
    pub what_ja: String,
    pub why_ja: Option<String>,
}

/// Metadata about the completed cards job.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct CardsJobResponse {
    pub cards_selected: i32,
    pub degraded: bool,
    pub from: String,
    pub job_id: Uuid,
    pub kicked_at: String,
    pub params_version: String,
    pub to: String,
}

/// Top-level response for `GET /v1/recaps/3days/cards`.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct GetCardsResponse {
    pub cards: Vec<CardResponse>,
    pub job: Option<CardsJobResponse>,
}

#[derive(Debug, Serialize)]
pub(crate) struct ErrorResponse {
    pub error: String,
}

/// Trait abstracting cards DAO read operations for testability.
#[async_trait::async_trait]
pub trait CardsDao: Send + Sync {
    async fn get_latest_completed_cards_job(&self) -> Result<Option<PreviousCardsJobMeta>, String>;
    async fn get_cards_for_job(&self, job_id: Uuid) -> Result<Vec<RecapCard>, String>;
    async fn get_job_stats(&self, job_id: Uuid) -> Result<Option<RecapCardJobStats>, String>;
}

#[async_trait::async_trait]
impl CardsDao for PgPool {
    async fn get_latest_completed_cards_job(&self) -> Result<Option<PreviousCardsJobMeta>, String> {
        CardsDaoOps::get_latest_completed_cards_job_meta(self)
            .await
            .map_err(|e| e.to_string())
    }

    async fn get_cards_for_job(&self, job_id: Uuid) -> Result<Vec<RecapCard>, String> {
        CardsDaoOps::get_cards_for_job(self, job_id)
            .await
            .map_err(|e| e.to_string())
    }

    async fn get_job_stats(&self, job_id: Uuid) -> Result<Option<RecapCardJobStats>, String> {
        CardsDaoOps::get_job_stats(self, job_id)
            .await
            .map_err(|e| e.to_string())
    }
}

/// Deserialize source citations once from JSONB Value with serde.
fn parse_sources(job_id: Uuid, card_id: Uuid, val: &Value) -> Result<Vec<CardSourceResponse>, ()> {
    serde_json::from_value::<Vec<CardSourceResponse>>(val.clone()).map_err(|e| {
        error!(
            job_id = %job_id,
            card_id = %card_id,
            error = %e,
            "failed to deserialize card sources"
        );
    })
}

/// Core implementation for `GET /v1/recaps/3days/cards` over any `CardsDao`.
#[allow(clippy::too_many_lines)]
pub async fn get_3days_cards_impl(dao: &impl CardsDao) -> axum::response::Response {
    info!("Fetching latest 3-day recap cards");

    let latest_job = match dao.get_latest_completed_cards_job().await {
        Ok(job) => job,
        Err(e) => {
            error!("Failed to fetch latest completed cards job: {e}");
            return (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(ErrorResponse {
                    error: "Failed to fetch recap cards".to_string(),
                }),
            )
                .into_response();
        }
    };

    let Some(job_record) = latest_job else {
        info!("No completed cards job found");
        return (
            StatusCode::OK,
            Json(GetCardsResponse {
                cards: Vec::new(),
                job: None,
            }),
        )
            .into_response();
    };

    let mut raw_cards = match dao.get_cards_for_job(job_record.job_id).await {
        Ok(cards) => cards,
        Err(e) => {
            error!(
                job_id = %job_record.job_id,
                error = %e,
                "Failed to fetch cards for job"
            );
            return (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(ErrorResponse {
                    error: "Failed to fetch recap cards".to_string(),
                }),
            )
                .into_response();
        }
    };

    raw_cards.sort_by_key(|c| c.rank);

    let stats = match dao.get_job_stats(job_record.job_id).await {
        Ok(Some(stats)) => stats,
        Ok(None) => {
            error!(
                job_id = %job_record.job_id,
                "missing job stats for completed cards job"
            );
            return (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(ErrorResponse {
                    error: "Missing job stats for completed cards job".to_string(),
                }),
            )
                .into_response();
        }
        Err(e) => {
            error!(
                job_id = %job_record.job_id,
                error = %e,
                "Failed to fetch job stats for job"
            );
            return (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(ErrorResponse {
                    error: "Failed to fetch job stats".to_string(),
                }),
            )
                .into_response();
        }
    };

    let cards_selected = stats.cards_selected;
    let degraded = usize::try_from(cards_selected).map_or(true, |c| c < CARDS_DEGRADED_MIN_CARDS);

    let mut cards = Vec::with_capacity(raw_cards.len());
    for c in raw_cards {
        let Ok(sources) = parse_sources(job_record.job_id, c.id, &c.sources) else {
            return (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(ErrorResponse {
                    error: "Failed to parse card sources".to_string(),
                }),
            )
                .into_response();
        };
        cards.push(CardResponse {
            continues_card_id: c.continues_card_id,
            created_at: c
                .created_at
                .to_rfc3339_opts(chrono::SecondsFormat::Secs, true),
            genre: c.genre,
            headline_ja: c.headline_ja,
            id: c.id,
            rank: c.rank,
            sources,
            story_id: c.story_id,
            what_ja: c.what_ja,
            why_ja: c.why_ja,
        });
    }

    let job = CardsJobResponse {
        cards_selected,
        degraded,
        from: job_record
            .from_ts
            .to_rfc3339_opts(chrono::SecondsFormat::Secs, true),
        job_id: job_record.job_id,
        kicked_at: job_record
            .kicked_at
            .to_rfc3339_opts(chrono::SecondsFormat::Secs, true),
        params_version: job_record.params_version,
        to: job_record
            .to_ts
            .to_rfc3339_opts(chrono::SecondsFormat::Secs, true),
    };

    (
        StatusCode::OK,
        Json(GetCardsResponse {
            cards,
            job: Some(job),
        }),
    )
        .into_response()
}

/// GET /v1/recaps/3days/cards
/// Latest 3-day topic cards recap endpoint.
pub(crate) async fn get_3days_cards(State(state): State<AppState>) -> impl IntoResponse {
    get_3days_cards_impl(state.pool()).await
}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::{DateTime, Utc};
    use std::sync::Mutex;

    #[derive(Default)]
    #[allow(clippy::struct_excessive_bools)]
    struct MockCardsDao {
        latest_job: Mutex<Option<PreviousCardsJobMeta>>,
        cards: Mutex<Vec<RecapCard>>,
        stats: Mutex<Option<RecapCardJobStats>>,
        should_fail_latest: bool,
        should_fail_cards: bool,
        should_fail_stats: bool,
        stats_is_none: bool,
    }

    #[async_trait::async_trait]
    impl CardsDao for MockCardsDao {
        async fn get_latest_completed_cards_job(
            &self,
        ) -> Result<Option<PreviousCardsJobMeta>, String> {
            if self.should_fail_latest {
                return Err("simulated db failure".to_string());
            }
            Ok(self.latest_job.lock().unwrap().clone())
        }

        async fn get_cards_for_job(&self, job_id: Uuid) -> Result<Vec<RecapCard>, String> {
            if self.should_fail_cards {
                return Err("simulated db failure".to_string());
            }
            let cards = self.cards.lock().unwrap();
            Ok(cards
                .iter()
                .filter(|c| c.job_id == job_id)
                .cloned()
                .collect())
        }

        async fn get_job_stats(&self, job_id: Uuid) -> Result<Option<RecapCardJobStats>, String> {
            if self.should_fail_stats {
                return Err("simulated stats db failure".to_string());
            }
            if self.stats_is_none {
                return Ok(None);
            }
            let stats = self.stats.lock().unwrap();
            Ok(stats.as_ref().filter(|s| s.job_id == job_id).cloned())
        }
    }

    #[tokio::test]
    async fn test_empty_case_when_no_job_exists() {
        let dao = MockCardsDao::default();
        let response = get_3days_cards_impl(&dao).await;

        assert_eq!(response.status(), StatusCode::OK);

        let body_bytes = axum::body::to_bytes(response.into_body(), 1024 * 1024)
            .await
            .expect("read body");
        let body: Value = serde_json::from_slice(&body_bytes).expect("parse json");

        assert_eq!(body["cards"], serde_json::json!([]));
        assert!(body["job"].is_null());
    }

    #[tokio::test]
    #[allow(clippy::too_many_lines)]
    async fn test_populated_case_matches_pact_contract() {
        let job_id = Uuid::parse_str("11111111-2222-3333-4444-555555555555").unwrap();
        let card_id = Uuid::parse_str("22222222-3333-4444-5555-666666666666").unwrap();
        let story_id = Uuid::parse_str("33333333-4444-5555-6666-777777777777").unwrap();
        let feed_id = Uuid::parse_str("44444444-5555-6666-7777-888888888888").unwrap();

        let kicked_at = DateTime::parse_from_rfc3339("2026-09-22T17:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let from_ts = DateTime::parse_from_rfc3339("2026-09-19T17:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let to_ts = DateTime::parse_from_rfc3339("2026-09-22T17:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let card_created_at = DateTime::parse_from_rfc3339("2026-09-22T17:05:00Z")
            .unwrap()
            .with_timezone(&Utc);

        let dao = MockCardsDao::default();
        *dao.latest_job.lock().unwrap() = Some(PreviousCardsJobMeta {
            job_id,
            kicked_at,
            from_ts,
            to_ts,
            params_version: "cards-v0.2".to_string(),
        });

        *dao.stats.lock().unwrap() = Some(RecapCardJobStats {
            job_id,
            items_fetched: 100,
            items_after_noise: 80,
            items_after_dedup: 60,
            clusters: 10,
            candidates: 8,
            cards_selected: 8,
            cards_dropped: serde_json::json!({}),
            embed_ms: 10,
            cluster_ms: 20,
            llm_ms: 30,
            total_ms: 60,
            params_version: "cards-v0.2".to_string(),
            embed_cache_hits: 0,
            embed_cache_misses: 0,
            created_at: kicked_at,
        });

        let sources_json = serde_json::json!([
            {
                "feed_id": feed_id.to_string(),
                "host": "example.com",
                "n": 1,
                "pub_date": "2026-09-22T10:00:00Z",
                "title": "新モデル発表のニュース",
                "url": "https://example.com/ai-news"
            }
        ]);

        dao.cards.lock().unwrap().push(RecapCard {
            id: card_id,
            job_id,
            rank: 1,
            story_id,
            continues_card_id: None,
            merged_from: None,
            headline_ja: "日本のAIスタートアップが新モデルを発表".to_string(),
            what_ja: "最新の推論モデルが公開された。[1]ベンチマークで高い性能を示した。[2]"
                .to_string(),
            why_ja: Some("日本語処理の効率化が期待される。[1]".to_string()),
            genre: Some("Technology".to_string()),
            member_feed_ids: vec![feed_id],
            sources: sources_json,
            centroid: None,
            scores: serde_json::json!({}),
            gates: serde_json::json!({}),
            generation: serde_json::json!({}),
            created_at: card_created_at,
        });

        let response = get_3days_cards_impl(&dao).await;
        assert_eq!(response.status(), StatusCode::OK);

        let body_bytes = axum::body::to_bytes(response.into_body(), 1024 * 1024)
            .await
            .expect("read body");
        let body: Value = serde_json::from_slice(&body_bytes).expect("parse json");

        // Verify job structure
        assert_eq!(body["job"]["job_id"], job_id.to_string());
        assert_eq!(body["job"]["kicked_at"], "2026-09-22T17:00:00Z");
        assert_eq!(body["job"]["from"], "2026-09-19T17:00:00Z");
        assert_eq!(body["job"]["to"], "2026-09-22T17:00:00Z");
        assert_eq!(body["job"]["params_version"], "cards-v0.2");
        assert_eq!(body["job"]["cards_selected"], 8);
        assert_eq!(body["job"]["degraded"], false);

        // Verify cards structure
        let cards = body["cards"].as_array().expect("cards is array");
        assert_eq!(cards.len(), 1);
        let card = &cards[0];
        assert_eq!(card["id"], card_id.to_string());
        assert_eq!(card["rank"], 1);
        assert_eq!(card["story_id"], story_id.to_string());
        assert!(card["continues_card_id"].is_null());
        assert_eq!(
            card["headline_ja"],
            "日本のAIスタートアップが新モデルを発表"
        );
        assert_eq!(
            card["what_ja"],
            "最新の推論モデルが公開された。[1]ベンチマークで高い性能を示した。[2]"
        );
        assert_eq!(card["why_ja"], "日本語処理の効率化が期待される。[1]");
        assert_eq!(card["genre"], "Technology");
        assert_eq!(card["created_at"], "2026-09-22T17:05:00Z");

        // Verify source in card
        let sources = card["sources"].as_array().expect("sources is array");
        assert_eq!(sources.len(), 1);
        assert_eq!(sources[0]["n"], 1);
        assert_eq!(sources[0]["feed_id"], feed_id.to_string());
        assert_eq!(sources[0]["host"], "example.com");
        assert_eq!(sources[0]["url"], "https://example.com/ai-news");
        assert_eq!(sources[0]["title"], "新モデル発表のニュース");
        assert_eq!(sources[0]["pub_date"], "2026-09-22T10:00:00Z");
    }

    #[tokio::test]
    async fn test_null_pub_date_passthrough() {
        let job_id = Uuid::new_v4();
        let card_id = Uuid::new_v4();
        let feed_id = Uuid::new_v4();
        let now = Utc::now();

        let dao = MockCardsDao::default();
        *dao.latest_job.lock().unwrap() = Some(PreviousCardsJobMeta {
            job_id,
            kicked_at: now,
            from_ts: now,
            to_ts: now,
            params_version: "cards-v0.2".to_string(),
        });
        *dao.stats.lock().unwrap() = Some(RecapCardJobStats {
            job_id,
            items_fetched: 10,
            items_after_noise: 10,
            items_after_dedup: 10,
            clusters: 5,
            candidates: 5,
            cards_selected: 5,
            cards_dropped: serde_json::json!({}),
            embed_ms: 10,
            cluster_ms: 10,
            llm_ms: 10,
            total_ms: 30,
            params_version: "cards-v0.2".to_string(),
            embed_cache_hits: 0,
            embed_cache_misses: 0,
            created_at: now,
        });

        // Source with null pub_date
        let sources_json = serde_json::json!([
            {
                "feed_id": feed_id.to_string(),
                "host": "example.com",
                "n": 1,
                "pub_date": null,
                "title": "Dateless news",
                "url": "https://example.com/dateless"
            }
        ]);

        dao.cards.lock().unwrap().push(RecapCard {
            id: card_id,
            job_id,
            rank: 1,
            story_id: Uuid::new_v4(),
            continues_card_id: None,
            merged_from: None,
            headline_ja: "日付なし見出し".to_string(),
            what_ja: "内容".to_string(),
            why_ja: None,
            genre: None,
            member_feed_ids: vec![feed_id],
            sources: sources_json,
            centroid: None,
            scores: serde_json::json!({}),
            gates: serde_json::json!({}),
            generation: serde_json::json!({}),
            created_at: now,
        });

        let response = get_3days_cards_impl(&dao).await;
        assert_eq!(response.status(), StatusCode::OK);

        let body_bytes = axum::body::to_bytes(response.into_body(), 1024 * 1024)
            .await
            .expect("read body");
        let body: Value = serde_json::from_slice(&body_bytes).expect("parse json");
        assert!(body["cards"][0]["sources"][0]["pub_date"].is_null());
    }

    #[tokio::test]
    async fn test_malformed_sources_returns_500() {
        let job_id = Uuid::new_v4();
        let now = Utc::now();

        let dao = MockCardsDao::default();
        *dao.latest_job.lock().unwrap() = Some(PreviousCardsJobMeta {
            job_id,
            kicked_at: now,
            from_ts: now,
            to_ts: now,
            params_version: "cards-v0.2".to_string(),
        });
        *dao.stats.lock().unwrap() = Some(RecapCardJobStats {
            job_id,
            items_fetched: 10,
            items_after_noise: 10,
            items_after_dedup: 10,
            clusters: 5,
            candidates: 5,
            cards_selected: 5,
            cards_dropped: serde_json::json!({}),
            embed_ms: 10,
            cluster_ms: 10,
            llm_ms: 10,
            total_ms: 30,
            params_version: "cards-v0.2".to_string(),
            embed_cache_hits: 0,
            embed_cache_misses: 0,
            created_at: now,
        });

        // Malformed sources (not valid CardSourceResponse - invalid UUID)
        let malformed_sources = serde_json::json!([
            {
                "feed_id": "not-a-valid-uuid",
                "host": "example.com",
                "n": 1,
                "pub_date": null,
                "title": "Bad feed_id",
                "url": "https://example.com/bad"
            }
        ]);

        dao.cards.lock().unwrap().push(RecapCard {
            id: Uuid::new_v4(),
            job_id,
            rank: 1,
            story_id: Uuid::new_v4(),
            continues_card_id: None,
            merged_from: None,
            headline_ja: "見出し".to_string(),
            what_ja: "内容".to_string(),
            why_ja: None,
            genre: None,
            member_feed_ids: vec![],
            sources: malformed_sources,
            centroid: None,
            scores: serde_json::json!({}),
            gates: serde_json::json!({}),
            generation: serde_json::json!({}),
            created_at: now,
        });

        let response = get_3days_cards_impl(&dao).await;
        assert_eq!(response.status(), StatusCode::INTERNAL_SERVER_ERROR);
    }

    #[tokio::test]
    async fn test_job_stats_err_returns_500() {
        let job_id = Uuid::new_v4();
        let now = Utc::now();

        let dao = MockCardsDao {
            should_fail_stats: true,
            ..Default::default()
        };
        *dao.latest_job.lock().unwrap() = Some(PreviousCardsJobMeta {
            job_id,
            kicked_at: now,
            from_ts: now,
            to_ts: now,
            params_version: "cards-v0.2".to_string(),
        });

        let response = get_3days_cards_impl(&dao).await;
        assert_eq!(response.status(), StatusCode::INTERNAL_SERVER_ERROR);
    }

    #[tokio::test]
    async fn test_job_stats_none_returns_500() {
        let job_id = Uuid::new_v4();
        let now = Utc::now();

        let dao = MockCardsDao {
            stats_is_none: true,
            ..Default::default()
        };
        *dao.latest_job.lock().unwrap() = Some(PreviousCardsJobMeta {
            job_id,
            kicked_at: now,
            from_ts: now,
            to_ts: now,
            params_version: "cards-v0.2".to_string(),
        });

        let response = get_3days_cards_impl(&dao).await;
        assert_eq!(response.status(), StatusCode::INTERNAL_SERVER_ERROR);
    }

    #[tokio::test]
    async fn test_degraded_threshold_computed_against_shared_constant() {
        let job_id = Uuid::new_v4();
        let dao = MockCardsDao::default();
        let now = Utc::now();
        *dao.latest_job.lock().unwrap() = Some(PreviousCardsJobMeta {
            job_id,
            kicked_at: now,
            from_ts: now,
            to_ts: now,
            params_version: "cards-v0.2".to_string(),
        });

        // Test cards_selected = CARDS_DEGRADED_MIN_CARDS - 1 (4) => degraded: true
        *dao.stats.lock().unwrap() = Some(RecapCardJobStats {
            job_id,
            items_fetched: 100,
            items_after_noise: 80,
            items_after_dedup: 60,
            clusters: 10,
            candidates: 5,
            cards_selected: (CARDS_DEGRADED_MIN_CARDS - 1) as i32,
            cards_dropped: serde_json::json!({}),
            embed_ms: 10,
            cluster_ms: 20,
            llm_ms: 30,
            total_ms: 60,
            params_version: "cards-v0.2".to_string(),
            embed_cache_hits: 0,
            embed_cache_misses: 0,
            created_at: now,
        });

        let response = get_3days_cards_impl(&dao).await;
        assert_eq!(response.status(), StatusCode::OK);
        let body_bytes = axum::body::to_bytes(response.into_body(), 1024 * 1024)
            .await
            .expect("read body");
        let body: Value = serde_json::from_slice(&body_bytes).expect("parse json");
        assert_eq!(body["job"]["degraded"], true);

        // Test cards_selected = CARDS_DEGRADED_MIN_CARDS (5) => degraded: false
        *dao.stats.lock().unwrap() = Some(RecapCardJobStats {
            job_id,
            items_fetched: 100,
            items_after_noise: 80,
            items_after_dedup: 60,
            clusters: 10,
            candidates: 5,
            cards_selected: CARDS_DEGRADED_MIN_CARDS as i32,
            cards_dropped: serde_json::json!({}),
            embed_ms: 10,
            cluster_ms: 20,
            llm_ms: 30,
            total_ms: 60,
            params_version: "cards-v0.2".to_string(),
            embed_cache_hits: 0,
            embed_cache_misses: 0,
            created_at: now,
        });

        let response = get_3days_cards_impl(&dao).await;
        assert_eq!(response.status(), StatusCode::OK);
        let body_bytes = axum::body::to_bytes(response.into_body(), 1024 * 1024)
            .await
            .expect("read body");
        let body: Value = serde_json::from_slice(&body_bytes).expect("parse json");
        assert_eq!(body["job"]["degraded"], false);
    }

    #[tokio::test]
    async fn test_db_failure_returns_500() {
        let dao = MockCardsDao {
            should_fail_latest: true,
            ..Default::default()
        };
        let response = get_3days_cards_impl(&dao).await;
        assert_eq!(response.status(), StatusCode::INTERNAL_SERVER_ERROR);
    }
}
