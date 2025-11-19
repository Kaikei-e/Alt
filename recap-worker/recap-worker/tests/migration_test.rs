/// Migration tests for recap_worker_config table.

use sqlx::{postgres::PgPoolOptions, Executor, Row};

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

#[tokio::test]
async fn test_recap_worker_config_table_exists() {
    let pool = setup_test_database().await;

    // Run migration
    let migration_sql = include_str!("../../../recap-migration-atlas/migrations/20251120000000_create_recap_worker_config.sql");
    pool.execute(migration_sql).await.expect("run migration");

    // Verify table exists
    let row = sqlx::query(
        r#"
        SELECT EXISTS (
            SELECT FROM information_schema.tables
            WHERE table_schema = 'public'
            AND table_name = 'recap_worker_config'
        )
        "#,
    )
    .fetch_one(&pool)
    .await
    .expect("check table exists");

    let exists: bool = row.get(0);
    assert!(exists, "recap_worker_config table should exist");
}

#[tokio::test]
async fn test_recap_worker_config_table_structure() {
    let pool = setup_test_database().await;

    // Run migration
    let migration_sql = include_str!("../../../recap-migration-atlas/migrations/20251120000000_create_recap_worker_config.sql");
    pool.execute(migration_sql).await.expect("run migration");

    // Verify columns exist
    let columns = sqlx::query(
        r#"
        SELECT column_name, data_type, is_nullable
        FROM information_schema.columns
        WHERE table_name = 'recap_worker_config'
        ORDER BY ordinal_position
        "#,
    )
    .fetch_all(&pool)
    .await
    .expect("get columns");

    let column_names: Vec<String> = columns
        .iter()
        .map(|row| row.get::<String, _>("column_name"))
        .collect();

    assert!(column_names.contains(&"id".to_string()));
    assert!(column_names.contains(&"config_type".to_string()));
    assert!(column_names.contains(&"config_payload".to_string()));
    assert!(column_names.contains(&"source".to_string()));
    assert!(column_names.contains(&"metadata".to_string()));
    assert!(column_names.contains(&"created_at".to_string()));
}

#[tokio::test]
async fn test_recap_worker_config_index_exists() {
    let pool = setup_test_database().await;

    // Run migration
    let migration_sql = include_str!("../../../recap-migration-atlas/migrations/20251120000000_create_recap_worker_config.sql");
    pool.execute(migration_sql).await.expect("run migration");

    // Verify index exists
    let row = sqlx::query(
        r#"
        SELECT EXISTS (
            SELECT FROM pg_indexes
            WHERE tablename = 'recap_worker_config'
            AND indexname = 'idx_recap_worker_config_type_created'
        )
        "#,
    )
    .fetch_one(&pool)
    .await
    .expect("check index exists");

    let exists: bool = row.get(0);
    assert!(exists, "index should exist");
}

#[tokio::test]
async fn test_recap_worker_config_insert_only_pattern() {
    let pool = setup_test_database().await;

    // Run migration
    let migration_sql = include_str!("../../../recap-migration-atlas/migrations/20251120000000_create_recap_worker_config.sql");
    pool.execute(migration_sql).await.expect("run migration");

    // Insert multiple configs
    let config1 = serde_json::json!({
        "graph_margin": 0.10,
        "boost_threshold": 0.03
    });

    let config2 = serde_json::json!({
        "graph_margin": 0.12,
        "boost_threshold": 0.05
    });

    sqlx::query(
        r#"
        INSERT INTO recap_worker_config (config_type, config_payload, source, metadata)
        VALUES ('graph_override', $1, 'test1', NULL)
        "#,
    )
    .bind(serde_json::Value::from(config1))
    .execute(&pool)
    .await
    .expect("insert config 1");

    tokio::time::sleep(tokio::time::Duration::from_millis(100)).await;

    sqlx::query(
        r#"
        INSERT INTO recap_worker_config (config_type, config_payload, source, metadata)
        VALUES ('graph_override', $1, 'test2', NULL)
        "#,
    )
    .bind(serde_json::Value::from(config2))
    .execute(&pool)
    .await
    .expect("insert config 2");

    // Verify both records exist (insert-only pattern)
    let count: i64 = sqlx::query_scalar(
        r#"
        SELECT COUNT(*) FROM recap_worker_config
        WHERE config_type = 'graph_override'
        "#,
    )
    .fetch_one(&pool)
    .await
    .expect("count configs");

    assert_eq!(count, 2, "both configs should exist");

    // Verify latest is returned
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
    .expect("get latest");

    let latest = row.expect("config should exist");
    let config_value: serde_json::Value = latest.get("config_payload");
    assert_eq!(config_value["graph_margin"], 0.12); // Latest value
}

// Note: GraphOverrideSettings::load_with_fallback test is in config_test.rs
// since graph_override module is pub(crate) and not accessible from integration tests

