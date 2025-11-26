# Recap Worker

Recap Worker is Alt's Rust 2024 batch processor that turns seven days of raw articles into curated Japanese summaries. It orchestrates every hop—from fetching source material to clustering, LLM summarization, and JSONB persistence—while exposing an Axum control plane for health probes, manual runs, metrics, and admin tooling.

## At a Glance
- **End-to-end pipeline**: Fetch → Preprocess → Dedup → Genre tagging → Evidence building → ML clustering → LLM summarization → Persistence.
- **Rust async stack**: Axum, Tokio, sqlx, reqwest, tracing, Prometheus metrics, Tokio-based schedulers.
- **Strict contracts**: JSON Schema validation for recap-subworker (ML) and news-creator (LLM) responses.
- **Compose-first**: Runs under Docker Compose alongside alt-backend, recap-db (PostgreSQL 16), recap-subworker, and news-creator.
- **Atlas migrations**: Definitive schema lives in `recap-migration-atlas/` (see below).

## System Flow
```mermaid
flowchart TD
    Scheduler[Daily JST Daemon<br/>04:00 @ UTC+9] -->|JobContext + genres| API
    Admin[Manual trigger / retry<br/>POST /v1/generate…] --> API
    subgraph Axum Control Plane
        API((Router)) -->|dispatch job| Pipeline
        API --> Health[/Health + Metrics/]
    end
    Pipeline --> FetchStage[Fetch & backup<br/>alt-backend]
    FetchStage --> PreStage[Preprocess<br/>HTML clean + lang detect]
    PreStage --> DedupStage[Dedup<br/>XXH3 + sentence filters]
    DedupStage --> GenreStage[Genre tagging]
    GenreStage --> Evidence[Evidence bundle
per genre]
    Evidence --> Dispatch[Dispatch Stage]
    Dispatch -->|Evidence corpus| Subworker
    Dispatch -->|Clusters| NewsCreator
    Subworker -->|Clustering JSON| Dispatch
    NewsCreator -->|Summary JSON| Dispatch
    Dispatch --> Persist[Persist Stage<br/>recap_outputs]
    Persist --> RecapDB[(recap-db / PostgreSQL)]
```

## Key Modules & Directories
| Path | Purpose |
| --- | --- |
| [`recap-worker/src/app.rs`](recap-worker/src/app.rs) | Component registry (Config, DAO, telemetry, scheduler, HTTP clients). |
| [`recap-worker/src/api/*.rs`](recap-worker/src/api) | Axum handlers for health, metrics, admin retry, fetch, and manual generation. |
| [`recap-worker/src/pipeline/`](recap-worker/src/pipeline) | Pipeline stages (`fetch`, `preprocess`, `dedup`, `genre`, `select`, `dispatch`, `persist`, `evidence`). |
| [`recap-worker/src/clients/`](recap-worker/src/clients) | HTTP clients for alt-backend, recap-subworker, and news-creator (with JSON Schema validation). |
| [`recap-worker/src/classifier/`](recap-worker/src/classifier) | Centroid-based classification (Rocchio) + Graph label propagation hybrid pipeline. |
| [`recap-worker/src/classification/`](recap-worker/src/classification) | Feature extraction and tokenization utilities for classification. |
| [`recap-worker/src/observability/`](recap-worker/src/observability) | Prometheus metrics, structured logging, tracing bootstrap. |
| [`recap-worker/src/store/`](recap-worker/src/store) | `RecapDao`, data models, advisory-lock helpers, persistence logic. |
| [`recap-migration-atlas/`](../recap-migration-atlas) | Source of truth Atlas migrations, Dockerized runner, and schema definitions. |
| [`docs/`](recap-worker/docs) | Deep dives (`dedup_analysis.md`, `dedup_optimization_design.md`, `subworker_404_investigation.md`). |
| [`docs/recap-worker.md`](../docs/recap-worker.md) | High-level service snapshot, runbook, integrations, and LLM notes (recommended entry point). |

## Control Plane APIs
| Method & Path | Description |
| --- | --- |
| `GET /health/live` | Liveness probe (internal counter + telemetry tick). |
| `GET /health/ready` | Readiness probe; pings recap-subworker + news-creator before returning `200`. |
| `GET /metrics` | Prometheus exposition containing pipeline counters, histograms, and gauges. |
| `POST /v1/generate/recaps/7days` | Manually enqueue a recap job (optionally pass custom `genres`). Returns `202 Accepted` + `job_id`. |
| `GET /v1/recaps/7days` | Fetch latest persisted recap (job metadata + per-genre sections + evidence links). |
| `POST /admin/jobs/retry` | Fire-and-forget retry using default Config genres; useful when a batch degraded. |

## Pipeline Stages
1. **Fetch (`AltBackendFetchStage`)**
   - Pulls paginated articles from alt-backend for the configured `RECAP_WINDOW_DAYS` backfill.
   - Wraps requests in exponential backoff (`util::retry`), backs up raw HTML to `recap_job_articles`, and acquires advisory locks to avoid duplicate runs.
2. **Preprocess (`TextPreprocessStage`)**
   - Offloads CPU-heavy normalization via `tokio::task::spawn_blocking` plus semaphore-constrained concurrency.
   - Ammonia cleans HTML, `html2text` strips markup, `whatlang` provides language hints, and tokens are generated for downstream classifiers.
   - Metrics persisted in `recap_preprocess_metrics` (totals, dropped counts, language distribution).
3. **Dedup (`HashDedupStage`)**
   - Exact + near-duplicate detection using XXH3 hashes and rolling-window similarity; performs sentence-level dedup with per-article stats.
4. **Genre (`KeywordGenreStage`)**
   - Hybrid classification pipeline: **Fast Pass** (Centroid-based similarity) → **Rescue Pass** (Graph label propagation) → **Fallback** (defaults to "other"). Uses Golden Dataset for training centroid vectors. Classifier implementation lives in `classifier/`.
5. **Evidence (`evidence.rs`)**
   - Groups deduplicated articles per genre, filters short sentences, and tracks metadata (language mix, counts). Output feeds ML dispatch.
6. **Dispatch (`MlLlmDispatchStage`)**
   - Parallel per-genre flow: send evidence corpus to recap-subworker (`/v1/runs`), poll until success, then build summary requests for news-creator (`/v1/summary/generate`). Responses pass JSON Schema verification before returning.
7. **Persist (`FinalSectionPersistStage`)**
   - Serializes LLM output to `recap_outputs` (JSONB) and `recap_sections`, storing human-readable summary text, bullets, and the full struct for downstream consumers.

## Scheduling & Job Lifecycle
- **Automatic daily batch**: `scheduler::daemon::spawn_jst_batch_daemon` fires at 04:00 JST using `RECAP_GENRES` defaults. Missing defaults result in a warning and skip auto runs.
- **Manual generation**: Invoke `POST /v1/generate/recaps/7days` with optional `{"genres": ["ai", ...]}`; the handler dedupes/normalizes input before queueing the job.
- **Admin retries**: `POST /admin/jobs/retry` triggers a best-effort rerun using the existing scheduler (useful after fixing upstream outages).
- **Job context**: `JobContext` carries `job_id` + genre list throughout the pipeline for logging and persistence.

## External Services & Contracts
| Service | Purpose | Contract Notes |
| --- | --- | --- |
| `alt-backend` | Article source (`/v1/recap/articles`). | Auth via `X-Service-Token` (optional). Paginated; Recap Worker backs up every article into PostgreSQL. |
| `recap-subworker` | ML clustering + representative selection. | REST interface at `/v1/runs`. Requests derived from evidence corpus; responses validated against `schema::subworker::CLUSTERING_RESPONSE_SCHEMA`. Includes retry/poll loop with idempotency headers. |
| `news-creator` | Japanese bullet summaries via LLM (Gemma 3:4B). | `/v1/summary/generate` with optional `SummaryOptions`. Only top 40 clusters (by size) are sent to respect LLM context window limits (8k tokens). Responses validated against `schema::news_creator::SUMMARY_RESPONSE_SCHEMA` before persistence. |

## Configuration & Environment
Full reference lives in [ENVIRONMENT.md](./ENVIRONMENT.md). Highlights:
- **Database**: `RECAP_DB_DSN` (PostgreSQL 16). Connection pooling via `sqlx::postgres::PgPoolOptions`.
- **External endpoints**: `ALT_BACKEND_BASE_URL`, `SUBWORKER_BASE_URL`, `NEWS_CREATOR_BASE_URL`.
- **Batch parameters**: `RECAP_WINDOW_DAYS`, `RECAP_GENRES`, `LLM_MAX_CONCURRENCY`, `LLM_PROMPT_VERSION`.
- **HTTP resiliency**: `HTTP_MAX_RETRIES`, `HTTP_BACKOFF_BASE_MS`, `HTTP_BACKOFF_CAP_MS`, per-client timeout overrides.
- **Observability**: `OTEL_EXPORTER_ENDPOINT`, `OTEL_SAMPLING_RATIO`, `RUST_LOG`.

### Golden Dataset for Classification
The classification pipeline uses a hybrid approach combining **Centroid-based Classification (Rocchio)** and **Graph Label Propagation**:

1. **Fast Pass**: Centroid classifier computes cosine similarity against genre centroids (trained from Golden Dataset). Default threshold: 0.6 (0.75 for `society_justice`).
2. **Rescue Pass**: Graph propagator applies label propagation for articles that failed Fast Pass. Uses similarity threshold 0.85 for edge creation.
3. **Fallback**: Unclassified articles default to "other".

**Golden Dataset location**:
- **Production/Docker**: `/app/data/golden_classification.json`
- **Development**: `tests/data/golden_classification.json` (relative to `recap-worker/recap-worker/`)

The Dockerfile automatically copies the golden dataset during build. If missing, the system falls back to the legacy `GenreClassifier`.

**Dependencies**: `ndarray` (vector operations), `petgraph` (graph algorithms).

See [IMPLEMENTATION.md](./IMPLEMENTATION.md) for detailed architecture and implementation notes.

## Observability
- **Metrics**: `GET /metrics` surfaces counters (`recap_articles_fetched_total`, `recap_clusters_created_total`, `recap_jobs_failed_total`), histograms (`recap_fetch_duration_seconds`, `recap_job_duration_seconds`, etc.), and gauges (`recap_active_jobs`, `recap_queue_size`). See `observability/metrics.rs`.
- **Tracing**: `observability::Telemetry` initializes JSON-formatted tracing logs. OTLP export can be enabled once collector availability is confirmed (see `observability/tracing.rs`).
- **Structured logging**: Important events (WARN/ERROR/INFO) emit JSON via `StructuredLogLayer`; redaction helpers live in `util::redact`.
- **Health probes**: `GET /health/live` + `GET /health/ready` integrate with Compose/Infra load balancers.

## Database & Migrations
- **Authoritative migrations**: `recap-migration-atlas/migrations/*.sql` with `atlas.hcl` + `schema.hcl`. Use the Docker helper in `recap-migration-atlas/docker/` (`docker build -t recap-db-migrator …`).
- **Legacy init scripts**: `recap-worker/recap-db/init/*.sql` remain for local bootstraps but defer to Atlas in CI/CD.
- **Recent change (2025-11-08)**: `20251108000100_update_recap_preprocess_metrics.sql` migrated legacy metric kv-pairs into typed columns expected by `RecapDao::save_preprocess_metrics`.

## Development Workflow
1. **Prerequisites**: Rust 1.87+, `cargo`, PostgreSQL 16, Docker/Compose for integration tests.
2. **Bootstrap**:
   ```bash
   cd recap-worker/recap-worker
   cargo fmt && cargo clippy --all-targets --all-features
   ```
3. **Run locally**:
   ```bash
   export RECAP_DB_DSN=postgres://recap:recap@localhost:5432/recap
   export ALT_BACKEND_BASE_URL=http://localhost:9000/
   export SUBWORKER_BASE_URL=http://localhost:18002/
   export NEWS_CREATOR_BASE_URL=http://localhost:18003/
   cargo run --release
   ```
4. **Testing & Benchmarks**:
   - Unit + doc tests: `cargo test`
   - Golden-set classifier regression: `cargo test --test classification_metrics`
   - Integration skeleton (requires Testcontainers): `cargo test --test integration_test -- --ignored`
   - Performance bench (preprocessing + keyword scoring): `cargo bench -p recap-worker --bench performance`
5. **Scripts**:
   - Retrain genre classifier weights from labelled data: `python scripts/retrain_genre_classifier.py tests/data/golden_classification.json new_weights.json`

## Troubleshooting & Maintenance
- **Common issues**: See [TROUBLESHOOTING.md](./TROUBLESHOOTING.md) for DB locks, HTTP timeouts, schema validation failures, and observability tips.
- **Dedup performance**: `docs/dedup_analysis.md` and `docs/dedup_optimization_design.md` outline current hotspots and planned improvements.
- **Service contract drift**: `docs/subworker_404_investigation.md` documents the `/v1/runs` endpoint switch and headers required to avoid `404` responses.
- **Atlas hashes**: Update `atlas.sum` via `recap-migration-atlas/docker/scripts/hash.sh` whenever migrations change.

## Reference Commands
```bash
# Health & metrics
curl http://localhost:9005/health/ready
curl http://localhost:9005/metrics | grep recap_jobs

# Inspect latest job metadata (psql)
psql $RECAP_DB_DSN -c "SELECT * FROM recap_jobs ORDER BY kicked_at DESC LIMIT 5;"

# Tail worker logs
docker compose logs -f recap-worker
```

Stay aligned with the Compose-first workflow, keep tests green, and run only the minimal set of stages necessary for safe, fast iterations.
