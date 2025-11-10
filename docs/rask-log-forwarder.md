# Rask Log Forwarder

_Last reviewed: November 10, 2025_

**Location:** `rask-log-forwarder/app`

## Role
- Rust 1.87 sidecar that tails Docker logs, parses them with SIMD-aware pipelines, batches data, and ships it to Rask Log Aggregator.
- Designed for zero-copy parsing, backpressure-aware buffering, and resilient delivery with retries + health reports.

## Service Snapshot
| Module | Description |
| --- | --- |
| `src/app/mod.rs` | CLI entrypoint (`clap`), config loading, ServiceManager bootstrap. |
| `src/app/service.rs` | Coordinates collector, parser, buffer manager, sender, and reliability manager. |
| `collector` | Docker log tailing with rotation awareness + optional target service override. |
| `parser::UniversalParser` | SIMD-aware parser normalizing log shape. |
| `buffer` | `BufferManager` w/ capacity, batch size, timeout, memory thresholds. |
| `sender` | HTTP client with compression + retry budgets (default endpoint `/v1/aggregate`). |
| `reliability` | Health reports, retry strategies, optional sled-backed disk fallback. |

## Code Status
- `App::from_args` loads config (supporting `--config-file`), constructs `ServiceManager`, starts processing loop, and exposes `health_check()`.
- `ServiceManager::start` auto-detects target service (via Docker labels) if not provided; backpressure kicks in when buffers reach 80% capacity.
- Signal handling uses Tokio channels to coordinate graceful shutdown (`ShutdownHandle`).
- Config (`src/app/config.rs`) exposes CLI/env overrides: endpoint URL, compression toggle, buffer size, batch size, retry attempts, connection limits.
- Logging uses `tracing` w/ JSON handler (`logging_system.rs`), safe to consume in ClickHouse.

## Integrations & Data
- Input: Docker Engine API via `bollard`; ensure container has read access to `/var/run/docker.sock`.
- Output: HTTP POST to Rask Log Aggregator (`/v1/aggregate`), typically behind the logging profile.
- Optional disk fallback: enable once aggregator enforces durable semantics; requires `sled` volume.

## Testing & Tooling
- `cargo test` for unit/component coverage, `tests/` for integration with `mockall` + `wiremock`.
- `cargo bench` (Criterion) benchmarks parser/buffer hot paths; keep regressions <5% before merging performance changes.
- Use `RUST_LOG=debug` when diagnosing throughput.

## Operational Runbook
1. Launch with `cargo run -- --target-service alt-backend --endpoint http://rask-log-aggregator:9600/v1/aggregate`.
2. Health check: `rask-log-forwarder --health-check` (todo) or query reliability manager via logs.
3. Throughput tuning: adjust `buffer_capacity`, `batch_size`, and `flush_interval` in tandem. Example: double capacity and batch size when shipping high-volume logs.
4. Override auto-detection if container labels missing: set `RASK_TARGET_SERVICE=service-name` or CLI flag.

## LLM Notes
- Mention whether edits should affect collector, parser, buffer, sender, or config modules—the architecture enforces single responsibility per crate.
- Provide desired config keys when requesting new CLI flags to ensure `clap` + env parsing stay consistent.
