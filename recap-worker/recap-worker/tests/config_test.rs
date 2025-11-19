/// Tests for config store (database and YAML fallback).
///
/// Note: GraphOverrideSettings tests are in src/pipeline/graph_override.rs
/// since the module is pub(crate) and not accessible from integration tests.
/// This file tests the database operations directly.

use sqlx::{postgres::PgPoolOptions, Executor, Row};
use std::env;

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
async fn test_insert_and_retrieve_config() {
    let pool = setup_test_database().await;
    setup_schema(&pool).await.expect("schema setup");

    // Insert test config
    let config_payload = serde_json::json!({
        "graph_margin": 0.12,
        "weighted_tie_break_margin": 0.03,
        "tag_confidence_gate": 0.65,
        "boost_threshold": 0.05,
        "tag_count_threshold": 3
    });

    sqlx::query(
        r#"
        INSERT INTO recap_worker_config (config_type, config_payload, source, metadata)
        VALUES ('graph_override', $1, 'test', '{"test": true}'::jsonb)
        "#,
    )
    .bind(serde_json::Value::from(config_payload))
    .execute(&pool)
    .await
    .expect("insert config");

    // Retrieve config directly
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

// Note: GraphOverrideSettings::load_with_fallback tests are in
// src/pipeline/graph_override.rs since the module is pub(crate)

#[tokio::test]
async fn test_get_latest_worker_config() {
    let pool = setup_test_database().await;
    setup_schema(&pool).await.expect("schema setup");

    // Insert first config
    let config1 = serde_json::json!({
        "graph_margin": 0.10,
        "boost_threshold": 0.03
    });

    sqlx::query(
        r#"
        INSERT INTO recap_worker_config (config_type, config_payload, source, metadata)
        VALUES ('graph_override', $1, 'test', NULL)
        "#,
    )
    .bind(serde_json::Value::from(config1))
    .execute(&pool)
    .await
    .expect("insert config 1");

    // Wait a bit to ensure different timestamps
    tokio::time::sleep(tokio::time::Duration::from_millis(100)).await;

    // Insert second config (should be latest)
    let config2 = serde_json::json!({
        "graph_margin": 0.12,
        "boost_threshold": 0.05
    });

    sqlx::query(
        r#"
        INSERT INTO recap_worker_config (config_type, config_payload, source, metadata)
        VALUES ('graph_override', $1, 'test', NULL)
        "#,
    )
    .bind(serde_json::Value::from(config2))
    .execute(&pool)
    .await
    .expect("insert config 2");

    // Get latest config (should be config2)
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
    .expect("get latest config");

    let latest = row.expect("config should exist");

    let config_value: serde_json::Value = latest.get("config_payload");
    assert_eq!(config_value["graph_margin"], 0.12);
    assert_eq!(config_value["boost_threshold"], 0.05);
}

#[tokio::test]
async fn test_insert_worker_config() {
    let pool = setup_test_database().await;
    setup_schema(&pool).await.expect("schema setup");

    let config_payload = serde_json::json!({
        "graph_margin": 0.15,
        "weighted_tie_break_margin": 0.04,
        "tag_confidence_gate": 0.70
    });

    let metadata = serde_json::json!({
        "accuracy_estimate": 0.92,
        "total_records": 100
    });

    sqlx::query(
        r#"
        INSERT INTO recap_worker_config (config_type, config_payload, source, metadata)
        VALUES ('graph_override', $1, 'test', $2)
        "#,
    )
    .bind(serde_json::Value::from(config_payload))
    .bind(serde_json::Value::from(metadata))
    .execute(&pool)
    .await
    .expect("insert config");

    // Verify config was inserted
    let row = sqlx::query(
        r#"
        SELECT config_payload, source, metadata
        FROM recap_worker_config
        WHERE config_type = 'graph_override'
        ORDER BY created_at DESC
        LIMIT 1
        "#,
    )
    .fetch_one(&pool)
    .await
    .expect("fetch config");

    let payload: serde_json::Value = row.get("config_payload");
    let source: String = row.get("source");
    let meta: serde_json::Value = row.get("metadata");

    assert_eq!(payload["graph_margin"], 0.15);
    assert_eq!(payload["weighted_tie_break_margin"], 0.04);
    assert_eq!(payload["tag_confidence_gate"], 0.70);
    assert_eq!(source, "test");
    assert_eq!(meta["accuracy_estimate"], 0.92);
    assert_eq!(meta["total_records"], 100);
}

// Note: GraphOverrideSettings::load_from_env and load_from_path tests are in
// src/pipeline/graph_override.rs since the module is pub(crate)

