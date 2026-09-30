# Recap Worker

Recap Worker is Alt's Rust 2024 batch processor that turns articles into curated Japanese summaries. It processes 3-day recap windows (`/v1/generate/recaps/3days` is the main production batch; the 7-day recap auto-batch is retired). It orchestrates every hop—from fetching source material from `alt-data-hub` over mTLS to clustering, LLM summarization, and JSONB persistence—while exposing an Axum control plane for health probes, manual runs, metrics, and admin tooling.

Reference architecture, control plane APIs, configuration, and operational runbooks are documented in [docs/services/recap-worker.md](../docs/services/recap-worker.md).

For a detailed visual walkthrough of pipeline stages and data flow, see [PIPELINE_FLOW.md](./PIPELINE_FLOW.md).

## At a Glance

- **End-to-end pipeline**: Fetch (from `alt-data-hub` over mTLS) → Preprocess → Dedup → Genre tagging → Select → Evidence building → ML clustering (`recap-subworker`) → LLM summarization (`news-creator`) → Persistence (`recap-db`).
- **Morning update pipeline**: Fetch → Preprocess → Dedup → Article grouping (deduplication tracking) → Morning Letter generation.
- **Rust async stack**: Axum, Tokio, sqlx, reqwest, tracing, Prometheus metrics.
- **Strict contracts**: JSON Schema validation for recap-subworker (ML) and news-creator (LLM) responses.
- **Compose-first**: Runs under Docker Compose wired via `compose/recap.yaml` (`include:` in `compose/compose.yaml`).

## Development Workflow

### Prerequisites

- Rust 1.94+ (`rustup`)
- `cargo`
- PostgreSQL 18 (`recap-db` running via Docker Compose)
- Access to `alt-data-hub`, `recap-subworker`, and `news-creator`

### Build & Check

```bash
cd recap-worker/recap-worker

# Format and lint
cargo fmt
cargo clippy --all-targets --all-features

# Build
cargo build --release
```

### Testing

```bash
cd recap-worker/recap-worker

# Unit + doc tests
cargo test

# Genre classification evaluation
cargo test --test evaluation

# Provider verification tests (ignored by default)
cargo test --test provider_verification -- --ignored

# Performance benchmarks
cargo bench -p recap-worker --bench performance
```

### Running Locally

```bash
cd recap-worker/recap-worker

export RECAP_DB_DSN=postgres://recap:recap@localhost:5432/recap
export ALT_BACKEND_MTLS_URL=https://localhost:9443
export SUBWORKER_BASE_URL=http://localhost:8002/
export NEWS_CREATOR_BASE_URL=http://localhost:11434/

cargo run --release
```

### Health Probes & Reference Commands

```bash
# Health check (readiness)
curl http://localhost:9005/health/ready

# Health check (liveness)
curl http://localhost:9005/health/live

# Metrics
curl http://localhost:9005/metrics

# Trigger 3-day recap (production batch)
curl -X POST http://localhost:9005/v1/generate/recaps/3days \
  -H "Content-Type: application/json" \
  -d '{"genres":["tech","ai"]}'

# Trigger manual 7-day recap (retired from auto-batch)
curl -X POST http://localhost:9005/v1/generate/recaps/7days \
  -H "Content-Type: application/json" \
  -d '{"genres":["tech","ai"]}'
```

## Related Documentation

- [Pipeline Flow](./PIPELINE_FLOW.md)
- [Architecture & Reference Documentation](../docs/services/recap-worker.md)
- [Project CLAUDE.md](../CLAUDE.md)
