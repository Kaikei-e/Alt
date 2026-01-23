# rask-log-aggregator/CLAUDE.md

## Overview

High-performance log aggregation service. **Rust 1.87+ (2024 Edition)**, **Axum**, **ClickHouse**.

> Details: `docs/services/rask-log-aggregator.md`

## Commands

```bash
# Test (TDD first)
cargo test

# Benchmarks
cargo bench

# Run
cargo run --release
```

## TDD Workflow

**IMPORTANT**: Write failing tests BEFORE implementation.

- **Unit**: Mock traits with `mockall`
- **Integration**: Use `axum-test` for in-memory handler testing
- **Performance**: Use `criterion` for benchmarking

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Rust 2024 Edition**: Use `async fn` in traits directly (no `async_trait`)
3. **No `static mut`**: Use `OnceCell` or `Mutex` instead
4. **Edition Hygiene**: Enforce `#![deny(warnings, rust_2024_idioms)]`
5. **Zero-Copy**: Prefer `bytes::Bytes` over owned allocations
