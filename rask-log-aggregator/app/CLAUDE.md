# rask-log-aggregator/CLAUDE.md

## Overview

High-performance log aggregation service for the Alt platform. Built with **Rust 1.87+ (2024 Edition)**, **Axum**, and **ClickHouse**. Designed for extreme throughput and low-latency processing.

> For ClickHouse wiring and ingestion details, see `docs/services/rask-log-aggregator.md`.

## Quick Start

```bash
# Run tests
cargo test

# Run benchmarks
cargo bench

# Start service
cargo run --release
```

## Core Capabilities

- Zero-copy parsing with `bytes` and `nom`
- Lock-free data structures (`crossbeam`, `dashmap`)
- Vectorized processing with `rayon`
- Structured storage in ClickHouse

## TDD Workflow

**IMPORTANT**: Always write failing tests BEFORE implementation.

Testing layers:
- **Unit**: Use cases as plain Rust structs, mock traits with `mockall`
- **Integration**: Use `axum-test` for in-memory handler testing
- **Performance**: Use `criterion` for benchmarking critical paths

## Critical Guidelines

1. **TDD First**: No implementation without failing tests
2. **Rust 2024 Edition**: Use `async fn` in traits directly (no `async_trait`)
3. **No `static mut`**: Use `OnceCell` or `Mutex` instead
4. **Edition Hygiene**: Enforce `#![deny(warnings, rust_2024_idioms)]`
5. **Zero-Copy**: Prefer `bytes::Bytes` over owned allocations

## Testing with axum-test

```rust
use axum_test::TestServer;

#[tokio::test]
async fn test_ingest_handler() {
    let app = Router::new().route("/logs", post(handler));
    let server = TestServer::new(app).unwrap();
    let response = server.post("/logs").json(&payload).await;
    response.assert_status(StatusCode::ACCEPTED);
}
```

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| Async trait errors | Use Rust 2024 native async traits |
| Lock contention | Use lock-free structures (dashmap) |
| Memory bloat | Check batch sizes, use zero-copy |
| Benchmark regression | Review criterion results |

## Appendix: References

### Official Documentation
- [Axum Documentation](https://docs.rs/axum/latest/axum/)
- [axum-test Crate](https://crates.io/crates/axum-test)

### Best Practices
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
- [The Rust Performance Book](https://nnethercote.github.io/perf-book/)

### Testing
- [mockall Crate](https://crates.io/crates/mockall)
- [criterion Benchmarking](https://crates.io/crates/criterion)
