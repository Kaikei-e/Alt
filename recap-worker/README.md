# Recap Worker

A production-ready Rust 2024 edition batch worker for generating 7-day recaps of articles from the Alt platform. This service orchestrates the entire pipeline from article fetching to final Japanese summary generation using ML clustering and LLM summarization.

## Overview

The Recap Worker is responsible for:

1. **Fetching articles** from `alt-backend` (last 7 days) and backing up raw data to `recap-db`
2. **CPU-intensive preprocessing** (Unicode normalization, HTML sanitization, sentence segmentation, deduplication)
3. **Genre classification** using keyword-based heuristics (10 genres)
4. **Evidence corpus construction** per genre for ML processing
5. **ML clustering** via `recap-subworker` (Python) with JSON Schema validation
6. **LLM summarization** via `news-creator` (Gemma 3:4B) for Japanese summaries
7. **Persistence** of final sections to PostgreSQL with JSONB storage

## Architecture

```
┌─────────────┐
│  Scheduler  │ (Cron/K8s/Compose)
│  (04:00 JST)│
└──────┬──────┘
       │
       ▼
┌─────────────────────────────────────────────────────────────┐
│                    Recap Worker (Rust)                       │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │
│  │  Fetch   │→ │ Preprocess│→ │  Dedup   │→ │  Genre   │    │
│  │  Stage   │  │   Stage   │  │  Stage   │  │  Stage   │    │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘    │
│       │             │              │              │          │
│       ▼             ▼              ▼              ▼          │
│  ┌──────────────────────────────────────────────────────┐  │
│  │         Evidence Corpus Construction                 │  │
│  └────────────────────┬─────────────────────────────────┘  │
│                       │                                      │
│                       ▼                                      │
│  ┌──────────────────────────────────────────────────────┐  │
│  │              Dispatch Stage (Parallel)                │  │
│  │  ┌──────────────┐         ┌──────────────┐           │  │
│  │  │  Subworker   │────────▶│ News-Creator │           │  │
│  │  │ (Clustering) │         │ (Summarize)  │           │  │
│  │  └──────────────┘         └──────────────┘           │  │
│  └────────────────────┬─────────────────────────────────┘  │
│                       │                                      │
│                       ▼                                      │
│  ┌──────────────────────────────────────────────────────┐  │
│  │              Persist Stage (JSONB)                    │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
       │                    │                    │
       ▼                    ▼                    ▼
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│ alt-backend │    │ recap-db    │    │ Subworker   │
│   (HTTP)    │    │ (PostgreSQL)│    │  (Python)   │
└─────────────┘    └─────────────┘    └─────────────┘
```

## Features

### ✅ Core Pipeline
- **Fetch Stage**: Paginated article retrieval from alt-backend with retry logic
- **Preprocess Stage**: Unicode normalization, HTML sanitization, sentence segmentation
- **Dedup Stage**: XXH3 hash-based article and sentence-level deduplication
- **Genre Stage**: Keyword-based heuristics for 10-genre classification
- **Evidence Corpus**: Genre-grouped article corpus construction
- **Dispatch Stage**: Parallel ML clustering and LLM summarization
- **Persist Stage**: Final section storage with JSONB fields

### ✅ Infrastructure
- **PostgreSQL Advisory Locks**: Duplicate execution prevention
- **OpenTelemetry Integration**: Distributed tracing with OTLP exporter
- **JSON Schema 2020-12**: Strict contract validation for Subworker/News-Creator
- **JSONB GIN Indexes**: Fast queries on JSON data
- **Exponential Backoff Retry**: Network failure resilience
- **Idempotency-Key Headers**: Idempotent HTTP requests

### ✅ Observability
- **Prometheus Metrics**: Counters, histograms, and gauges
- **Structured JSON Logging**: Important events in JSON format
- **Error Classification**: Retryable/Non-retryable/Fatal error handling
- **Partial Success Handling**: Genre-level failure detection and retry logic

## Quick Start

### Prerequisites

- Rust 1.87+ (2024 edition)
- PostgreSQL 16+
- Docker & Docker Compose (for local development)

### Environment Variables

See [ENVIRONMENT.md](./ENVIRONMENT.md) for complete environment variable documentation.

**Required:**
```bash
RECAP_DB_DSN=postgresql://user:pass@localhost:5432/recap_db
ALT_BACKEND_BASE_URL=http://alt-backend:9000
SUBWORKER_BASE_URL=http://recap-subworker:8080
NEWS_CREATOR_BASE_URL=http://news-creator:8000
```

**Optional (with defaults):**
```bash
RECAP_WORKER_HTTP_BIND=0.0.0.0:9005
RECAP_WINDOW_DAYS=7
RECAP_GENRES=tech,economy,ai,policy,security,science,product,design,devops,culture
HTTP_MAX_RETRIES=3
HTTP_BACKOFF_BASE_MS=250
HTTP_BACKOFF_CAP_MS=10000
OTEL_EXPORTER_ENDPOINT=http://otel-collector:4317
OTEL_SAMPLING_RATIO=1.0
```

### Building

```bash
cd recap-worker/recap-worker
cargo build --release
```

### Running

```bash
# Set environment variables
export RECAP_DB_DSN="postgresql://..."
export ALT_BACKEND_BASE_URL="http://..."
# ... other required vars

# Run the worker
cargo run --release
```

### Testing

```bash
# Unit tests
cargo test

# Integration tests (requires Docker)
cargo test --test integration_test

# Performance benchmarks
cargo bench
```

## Pipeline Stages

### 1. Fetch Stage
- Fetches articles from `alt-backend` with pagination
- Implements exponential backoff retry with jitter
- Backs up raw articles to `recap_job_articles` table
- Configurable timeouts (connect: 3s, read: 20s, total: 30s)

### 2. Preprocess Stage
- **Unicode Normalization**: NFC form
- **HTML Sanitization**: `ammonia` + `html2text` for plain text conversion
- **Language Detection**: `whatlang` for language hints
- **CPU Offloading**: `tokio::task::spawn_blocking` with semaphore concurrency control
- **Metrics Collection**: Articles processed, HTML cleaned, language distribution

### 3. Dedup Stage
- **Article-level Deduplication**: XXH3 hash-based exact and near-duplicate detection
- **Sentence-level Deduplication**: Per-article sentence hash deduplication
- **Rolling Window**: Jaccard similarity for near-duplicate detection
- **Statistics**: Tracks unique/duplicate articles and sentences

### 4. Genre Stage
- **Keyword Heuristics**: Multilingual (Japanese/English) keyword matching
- **10 Genres**: ai, tech, business, science, entertainment, sports, politics, health, world, other
- **Multi-label Assignment**: 1-3 genres per article
- **Distribution Tracking**: Genre-level article counts

### 5. Evidence Corpus
- Groups articles by genre
- Constructs evidence corpus with metadata (language distribution, sentence counts)
- Prepares data for Subworker ML processing

### 6. Dispatch Stage
- **Parallel Processing**: One task per genre
- **Subworker Integration**: Sends evidence corpus, receives clustering results
- **News-Creator Integration**: Sends clusters, receives Japanese summaries
- **JSON Schema Validation**: Validates all external service responses
- **Error Handling**: Genre-level failure isolation

### 7. Persist Stage
- Saves final sections to `recap_final_sections` table
- Uses JSONB for flexible storage
- Tracks success/failure per genre

## Database Schema

### Key Tables

- **`recap_jobs`**: Job metadata with advisory lock tracking
- **`recap_job_articles`**: Raw article backups (source of truth)
- **`recap_preprocess_metrics`**: Preprocessing statistics
- **`recap_subworker_runs`**: Subworker execution records
- **`recap_subworker_clusters`**: ML clustering results (JSONB `top_terms`)
- **`recap_subworker_sentences`**: Sentence-level cluster assignments
- **`recap_final_sections`**: Final Japanese summaries (JSONB `bullets_ja`)

### Indexes

- **GIN indexes** on JSONB fields for fast queries:
  - `recap_subworker_clusters.top_terms`
  - `recap_subworker_runs.request_payload`, `response_payload`
  - `recap_outputs.output_json`

## Error Handling

### Error Classification

- **Retryable**: Network timeouts, 5xx errors, database connection issues
- **Non-retryable**: 4xx errors (except auth), validation failures
- **Fatal**: Authentication errors, configuration errors

### Partial Success

- Genre-level failure isolation
- Missing genre detection for retry
- Retryability determination based on error types

## Observability

### Prometheus Metrics

- **Counters**: `recap_articles_fetched_total`, `recap_jobs_completed_total`, `recap_retries_total`
- **Histograms**: `recap_fetch_duration_seconds`, `recap_preprocess_duration_seconds`, `recap_job_duration_seconds`
- **Gauges**: `recap_active_jobs`, `recap_queue_size`

### OpenTelemetry Tracing

- Spans for each pipeline stage
- OTLP exporter to collector
- Configurable sampling ratio

### Structured Logging

- JSON format for important events (ERROR/WARN/INFO)
- Includes timestamp, level, target, message, and fields

## Configuration

See [ENVIRONMENT.md](./ENVIRONMENT.md) for detailed configuration options.

## Troubleshooting

See [TROUBLESHOOTING.md](./TROUBLESHOOTING.md) for common issues and solutions.

## Development

### SQLx Offline Mode

```bash
# Prepare queries for offline compilation
cargo sqlx prepare --check

# Or with database connection
DATABASE_URL=postgresql://... cargo sqlx prepare
```

### Running Tests

```bash
# Unit tests
cargo test

# Integration tests (requires testcontainers)
cargo test --test integration_test -- --ignored

# All tests
cargo test --all-features
```

### Code Quality

```bash
# Format
cargo fmt

# Lint
cargo clippy --all-targets --all-features -- -D warnings
```

## Performance

### Benchmarks

Run performance benchmarks:

```bash
cargo bench
```

Target performance (7 days, ~10k articles):
- Fetch: < 5 minutes
- Preprocess: < 10 minutes
- Dedup: < 2 minutes
- Genre: < 1 minute
- Dispatch (per genre): < 30 seconds
- Total: < 20 minutes

## License

See LICENSE file in the repository root.

## References

- [PostgreSQL Advisory Locks](https://www.postgresql.org/docs/current/functions-admin.html)
- [Tower Retry Policy](https://tower-rs.github.io/tower/tower/retry/trait.Policy.html)
- [JSON Schema Validation](https://docs.rs/jsonschema)
- [OpenTelemetry Rust](https://opentelemetry.io/docs/languages/rust/)
