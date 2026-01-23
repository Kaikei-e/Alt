# rask-log-forwarder/CLAUDE.md

## Overview

Ultra-high-performance log forwarding sidecar. **Rust 1.87+ (2024 Edition)**, SIMD parsing (>4 GB/s).

> Details: `docs/services/rask-log-forwarder.md`

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

- **Unit**: Parser isolation tests for valid/malformed inputs
- **Component**: Mock Docker client with `mockall`
- **Integration**: Full pipeline with `wiremock` as aggregator endpoint

## Critical Rules

1. **TDD First**: No implementation without failing tests
2. **Rust 2024 Edition**: Use native async traits
3. **No `static mut`**: Use `OnceCell` or `Mutex`
4. **Zero-Copy**: Use `bytes::Bytes` throughout pipeline
5. **Mock Docker API**: NEVER make real Docker calls in unit tests
