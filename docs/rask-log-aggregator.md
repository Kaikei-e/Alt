# Rask Log Aggregator

_Last reviewed: November 10, 2025_

**Location:** `rask-log-aggregator/app`

## Role
- Rust 1.87 Axum API that receives batched logs from forwarders and writes them into ClickHouse for observability analytics.
- Surface area kept intentionally small: `/v1/aggregate` for ingestion, `/v1/health` for readiness.

## Service Snapshot
| Component | Description |
| --- | --- |
| `src/config.rs` | Loads `APP_CLICKHOUSE_*` env vars, errors fast when missing. |
| `log_exporter` | Trait + implementations (default `ClickHouseExporter`) for writing batches. |
| `domain::EnrichedLogEntry` | Data model for normalized log entries. |
| `log_exporter/clickhouse_exporter.rs` | Converts entries to ClickHouse insert operations. |
| `src/main.rs` | Wires tracing, loads config, builds router, binds to `0.0.0.0:9600`. |

## Code Status
- `aggregate_handler` reads NDJSON body, parses each line via `serde_json::from_str`, filters invalid entries with logging, and sends remaining entries to exporter.
- Exporter interface is async, enabling ClickHouse HTTP writes; implement alternate exporters (e.g., S3) by satisfying `LogExporter`.
- Health route logs each access and responds `"Healthy"`; consider adding rate limiting if probes become noisy.

## Integrations & Data
- **ClickHouse:** Uses HTTP transport. Required env vars: `APP_CLICKHOUSE_HOST`, `APP_CLICKHOUSE_PORT`, `APP_CLICKHOUSE_USER`, `APP_CLICKHOUSE_PASSWORD`, `APP_CLICKHOUSE_DATABASE`.
- **Forwarder contract:** Payload is newline-delimited JSON; keep batches <1 MB (configurable) to avoid ClickHouse HTTP limits.
- **Observability:** Tracing via `tracing_subscriber` with env filter; adjust `RUST_LOG` to control verbosity.
- **Recap metrics:** Ingest `recap_genre_refine_*`, `recap_api_evidence_duplicates_total`, and related counters so ClickHouse dashboards can surface dedup, rollout, and golden dataset guardrails after each recap job.

## Testing & Tooling
- `cargo test` for unit coverage; integration tests (see `tests/`) can start a ClickHouse test container or mock exporter.
- Run `cargo fmt` + `cargo clippy -- -D warnings` before committing.
- Extend tests when adding exporter variants to verify error propagation + retry policies.

## Operational Runbook
1. Set ClickHouse env vars, then `cargo run --release`.
2. Smoke test ingestion: `printf '{"message":"hi"}\n' | curl -X POST --data-binary @- localhost:9600/v1/aggregate`.
3. Monitor logs for `Failed to export logs` entries; they indicate ClickHouse errors. Consider adding retries/backoffs if errors are transient.
4. Health endpoint: `curl localhost:9600/v1/health`.

## LLM Notes
- When requesting changes, specify whether to modify `aggregate_handler`, `LogExporter` implementations, or config parsing so boundaries stay intact.
- Provide schema expectations for `EnrichedLogEntry` if you ask to add new fields; ensure ClickHouse exporter updates column mapping accordingly.
