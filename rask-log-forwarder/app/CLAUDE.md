# rask-log-forwarder/CLAUDE.md

## Overview

Ultra-high-performance log forwarding sidecar for the Alt platform. Built with **Rust 1.87+ (2024 Edition)**. Collects Docker logs with SIMD parsing (>4 GB/s) and guaranteed delivery.

> For buffer tuning and retry strategy, see `docs/services/rask-log-forwarder.md`.

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

- Zero-copy log collection from Docker `json-file` driver
- SIMD-accelerated JSON parsing
- Lock-free buffering with `tokio::sync::broadcast`
- Guaranteed delivery with exponential backoff and `sled` disk fallback

## TDD Workflow

**IMPORTANT**: Always write failing tests BEFORE implementation.

Testing layers:
1. **Unit**: Parser isolation tests for valid/malformed inputs
2. **Component**: Mock Docker client with `mockall`
3. **Integration**: Full pipeline with `wiremock` as aggregator endpoint

## Critical Guidelines

1. **TDD First**: No implementation without failing tests
2. **Rust 2024 Edition**: Use native async traits
3. **No `static mut`**: Use `OnceCell` or `Mutex`
4. **Zero-Copy**: Use `bytes::Bytes` throughout pipeline
5. **Mock Docker API**: Never make real Docker calls in unit tests

## Testing with wiremock

```rust
use wiremock::{MockServer, Mock, ResponseTemplate};
use wiremock::matchers::{method, path};

#[tokio::test]
async fn test_forwarding() {
    let server = MockServer::start().await;
    Mock::given(method("POST")).and(path("/v1/aggregate"))
        .respond_with(ResponseTemplate::new(200))
        .mount(&server).await;
    // Run forwarder with mock endpoint
}
```

## Common Pitfalls

| Issue | Solution |
|-------|----------|
| Docker stream errors | Mock with `mockall`, test error paths |
| Delivery failures | Verify sled fallback and retry logic |
| Parsing errors | Test edge cases with malformed JSON |
| Performance regression | Run criterion benchmarks |

## Appendix: References

### Official Documentation
- [bollard (Docker client)](https://crates.io/crates/bollard)
- [wiremock-rs](https://crates.io/crates/wiremock)

### Best Practices
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
- [The Rust Performance Book](https://nnethercote.github.io/perf-book/)

### Testing
- [mockall Crate](https://crates.io/crates/mockall)
- [criterion Benchmarking](https://crates.io/crates/criterion)
