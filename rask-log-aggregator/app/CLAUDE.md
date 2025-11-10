# rask/CLAUDE.md

<!-- Model Configuration -->
<!-- ALWAYS use claude-4-sonnet for this project -->

## About Rask

> For the live ingestion/ClickHouse wiring snapshot, see `docs/rask-log-aggregator.md`.

**Rask** (Norwegian for "fast") is a high-performance log aggregation service built with **Rust 1.87+ (2024 Edition)**, the **Axum framework**, and **Clean Architecture** principles. It is designed for extreme throughput and low-latency processing.

**Core Responsibilities:**
-   Real-time, high-throughput log ingestion.
-   Zero-copy parsing and enrichment.
-   Structured log storage and indexing (ClickHouse).
-   Real-time alerting and analytics.

## TDD and Testing Strategy

Development is strictly **Test-Driven**. We use a layered testing strategy that includes unit, integration, and performance tests.

### 1. Unit and Integration Testing

This is the foundation of our TDD workflow. We test individual components in isolation and their interactions.

#### Testing Use Cases (Business Logic)

Use cases are tested as plain Rust structs/functions, completely decoupled from Axum. Dependencies are mocked using traits.

```rust
#[cfg(test)]
mod tests {
    use super::*;
    use mockall::predicate::*;
    use mockall::*;

    // Create a mock using the `automock` attribute
    #[automock]
    trait LogRepository {
        async fn save(&self, log: LogEntry) -> Result<(), String>;
    }

    #[tokio::test]
    async fn test_ingest_log_use_case() {
        // 1. Arrange: Create mock and set expectations
        let mut mock_repo = MockLogRepository::new();
        mock_repo.expect_save()
            .with(eq(LogEntry { ... }))
            .times(1)
            .returning(|_| Ok(()));

        // 2. Act: Execute the use case
        let use_case = IngestLog::new(Arc::new(mock_repo));
        let result = use_case.execute(LogEntry { ... }).await;

        // 3. Assert
        assert!(result.is_ok());
    }
}
```

#### Testing Axum Handlers

We use the `axum-test` crate for in-memory testing of our API handlers, which is fast and reliable.

```rust
#[cfg(test)]
mod tests {
    use super::*;
    use axum::Router;
    use axum_test::TestServer;
    use std::sync::Arc;

    #[tokio::test]
    async fn test_ingest_log_handler() {
        // 1. Arrange: Mock the use case dependency
        let mock_use_case = Arc::new(MockIngestLogUseCase::new());
        // ... set expectations on the mock ...

        // 2. Arrange: Create the Axum router with the mocked dependency
        let app = Router::new()
            .route("/logs", post(ingest_log_handler))
            .with_state(mock_use_case);

        // 3. Arrange: Create the test server
        let server = TestServer::new(app).unwrap();

        // 4. Act: Send a request to the handler
        let response = server
            .post("/logs")
            .json(&serde_json::json!({ "message": "test log" }))
            .await;

        // 5. Assert
        response.assert_status(StatusCode::ACCEPTED);
    }
}
```

### 2. Performance Testing

We use `criterion` to benchmark critical code paths and prevent performance regressions.

```rust
#[cfg(test)]
mod benches {
    use criterion::{black_box, criterion_group, criterion_main, Criterion};

    fn benchmark_log_parsing(c: &mut Criterion) {
        let log_line = b"..."; // Sample log line
        c.bench_function("parse_log", |b| {
            b.iter(|| parse_log_line(black_box(log_line)));
        });
    }
}
```

## High-Performance Design

-   **Zero-Copy Processing**: Use `bytes::Bytes` and `nom` for efficient, zero-copy log parsing.
-   **Lock-Free Data Structures**: Use `crossbeam` channels and `dashmap` for high-throughput, concurrent data handling.
-   **Vectorized Processing**: Use `rayon` for parallel processing of log batches.

## Rust 2024 Edition Best Practices

-   **Use `async fn` and `impl Trait` in traits directly.** Avoid `async_trait`.
-   **Eliminate `static mut`**. Use `OnceCell` or `Mutex` instead.
-   **Enforce edition hygiene** with `#![deny(warnings, rust_2024_idioms)]`.

## References

-   [Testing Axum Applications with `axum-test`](https://crates.io/crates/axum-test)
-   [The Rust Performance Book](https://nnethercote.github.io/perf-book/)
-   [Clean Architecture in Rust](https://kigawas.me/blog/2024-02-18-rust-clean-architecture.html)
