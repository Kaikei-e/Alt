/// Tests for the learning API endpoint.

use axum::{
    body::Body,
    http::{Request, StatusCode},
};
use serde_json::json;
use sqlx::{postgres::PgPoolOptions, Executor, Row};
use tower::ServiceExt;

use recap_worker::{
    app::{build_router, ComponentRegistry},
    config::Config,
};

async fn setup_test_state() -> ComponentRegistry {
    // Note: ENV_MUTEX is only available in unit tests, not integration tests
    // We set environment variables directly for integration tests
    let config = {
        // SAFETY: test code adjusts deterministic environment state sequentially.
        unsafe {
            std::env::set_var(
                "RECAP_DB_DSN",
                "postgres://user:pass@localhost:5555/recap_db",
            );
            std::env::set_var("NEWS_CREATOR_BASE_URL", "http://localhost:8001/");
            std::env::set_var("SUBWORKER_BASE_URL", "http://localhost:8002/");
            std::env::set_var("ALT_BACKEND_BASE_URL", "http://localhost:9000/");
            std::env::remove_var("ALT_BACKEND_SERVICE_TOKEN");
        }
        Config::from_env().expect("config loads")
    };
    ComponentRegistry::build(config)
        .await
        .expect("registry builds")
}

async fn setup_test_database() -> sqlx::PgPool {
    let Ok(database_url) = std::env::var("DATABASE_URL") else {
        // Use a dummy pool if DATABASE_URL is not set
        return PgPoolOptions::new()
            .max_connections(1)
            .connect_lazy("postgres://user:pass@localhost:5555/recap_db")
            .expect("lazy connection");
    };
    PgPoolOptions::new()
        .max_connections(1)
        .connect(&database_url)
        .await
        .expect("database connection")
}

async fn setup_schema(pool: &sqlx::PgPool) -> anyhow::Result<()> {
    pool.execute(
        r#"
        CREATE TABLE IF NOT EXISTS recap_worker_config (
            id BIGSERIAL PRIMARY KEY,
            config_type TEXT NOT NULL DEFAULT 'graph_override',
            config_payload JSONB NOT NULL,
            source TEXT NOT NULL DEFAULT 'genre_learning',
            metadata JSONB,
            created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        );
        CREATE INDEX IF NOT EXISTS idx_recap_worker_config_type_created
            ON recap_worker_config(config_type, created_at DESC);
        "#,
    )
    .await?;
    Ok(())
}

#[tokio::test]
async fn test_receive_genre_learning_success() {
    let registry = setup_test_state().await;
    let router = build_router(registry);

    let pool = setup_test_database().await;
    setup_schema(&pool).await.expect("schema setup");

    let payload = json!({
        "summary": {
            "graph_margin_reference": 0.15,
            "boost_threshold_reference": 0.05,
            "tag_count_threshold_reference": 3,
            "total_records": 100,
            "accuracy_estimate": 0.92
        },
        "graph_override": {
            "graph_margin": 0.12,
            "weighted_tie_break_margin": 0.03,
            "tag_confidence_gate": 0.65,
            "boost_threshold": 0.05,
            "tag_count_threshold": 3
        },
        "metadata": {
            "captured_at": "2025-11-20T10:00:00Z",
            "entries_observed": 100
        }
    });

    let request = Request::post("/admin/genre-learning")
        .header("content-type", "application/json")
        .body(Body::from(serde_json::to_string(&payload).unwrap()))
        .expect("request builds");

    let response = router.oneshot(request).await.unwrap();

    assert_eq!(response.status(), StatusCode::OK);

    let body = axum::body::to_bytes(response.into_body(), usize::MAX).await.unwrap();
    let response_json: serde_json::Value = serde_json::from_slice(&body).unwrap();

    assert_eq!(response_json["status"], "success");
    assert_eq!(response_json["config_saved"], true);

    // Verify config was saved to database
    let row = sqlx::query(
        r#"
        SELECT config_payload
        FROM recap_worker_config
        WHERE config_type = 'graph_override'
        ORDER BY created_at DESC
        LIMIT 1
        "#,
    )
    .fetch_optional(&pool)
    .await
    .expect("query config");

    let config = row.expect("config should exist");
    let config_value: serde_json::Value = config.get("config_payload");
    assert_eq!(config_value["graph_margin"], 0.12);
    assert_eq!(config_value["weighted_tie_break_margin"], 0.03);
    assert_eq!(config_value["tag_confidence_gate"], 0.65);
    assert_eq!(config_value["boost_threshold"], 0.05);
    assert_eq!(config_value["tag_count_threshold"], 3);
}

#[tokio::test]
async fn test_receive_genre_learning_fallback_to_summary() {
    let registry = setup_test_state().await;
    let router = build_router(registry);

    let pool = setup_test_database().await;
    setup_schema(&pool).await.expect("schema setup");

    // Payload without graph_override, should fallback to summary
    let payload = json!({
        "summary": {
            "graph_margin_reference": 0.15,
            "boost_threshold_reference": 0.05,
            "tag_count_threshold_reference": 3,
            "total_records": 100,
            "accuracy_estimate": 0.92
        },
        "metadata": {
            "captured_at": "2025-11-20T10:00:00Z"
        }
    });

    let request = Request::post("/admin/genre-learning")
        .header("content-type", "application/json")
        .body(Body::from(serde_json::to_string(&payload).unwrap()))
        .expect("request builds");

    let response = router.oneshot(request).await.unwrap();

    assert_eq!(response.status(), StatusCode::OK);

    let body = axum::body::to_bytes(response.into_body(), usize::MAX).await.unwrap();
    let response_json: serde_json::Value = serde_json::from_slice(&body).unwrap();

    assert_eq!(response_json["status"], "success");

    // Verify config was saved with summary values
    let row = sqlx::query(
        r#"
        SELECT config_payload
        FROM recap_worker_config
        WHERE config_type = 'graph_override'
        ORDER BY created_at DESC
        LIMIT 1
        "#,
    )
    .fetch_optional(&pool)
    .await
    .expect("query config");

    let config = row.expect("config should exist");
    let config_value: serde_json::Value = config.get("config_payload");
    assert_eq!(config_value["graph_margin"], 0.15);
    assert_eq!(config_value["boost_threshold"], 0.05);
    assert_eq!(config_value["tag_count_threshold"], 3);
}

#[tokio::test]
async fn test_receive_genre_learning_no_config_values() {
    let registry = setup_test_state().await;
    let router = build_router(registry);

    // Payload with no config values
    let payload = json!({
        "summary": {
            "total_records": 100
        },
        "metadata": {
            "captured_at": "2025-11-20T10:00:00Z"
        }
    });

    let request = Request::post("/admin/genre-learning")
        .header("content-type", "application/json")
        .body(Body::from(serde_json::to_string(&payload).unwrap()))
        .expect("request builds");

    let response = router.oneshot(request).await.unwrap();

    assert_eq!(response.status(), StatusCode::BAD_REQUEST);

    let body = axum::body::to_bytes(response.into_body(), usize::MAX).await.unwrap();
    let response_json: serde_json::Value = serde_json::from_slice(&body).unwrap();

    assert_eq!(response_json["status"], "error");
    assert_eq!(response_json["config_saved"], false);
    assert!(response_json["message"]
        .as_str()
        .unwrap()
        .contains("no configuration values"));
}

#[tokio::test]
async fn test_receive_genre_learning_invalid_json() {
    let registry = setup_test_state().await;
    let router = build_router(registry);

    let request = Request::builder()
        .method("POST")
        .uri("/admin/genre-learning")
        .header("content-type", "application/json")
        .body(Body::from("invalid json"))
        .unwrap();

    let response = router.oneshot(request).await.unwrap();

    // Axum returns 400 for invalid JSON
    assert_eq!(response.status(), StatusCode::BAD_REQUEST);
}

