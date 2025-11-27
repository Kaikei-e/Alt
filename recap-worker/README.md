# Recap Worker

Recap Worker is Alt's Rust 2024 batch processor that turns seven days of raw articles into curated Japanese summaries. It orchestrates every hop—from fetching source material to clustering, LLM summarization, and JSONB persistence—while exposing an Axum control plane for health probes, manual runs, metrics, and admin tooling.

## At a Glance
- **End-to-end pipeline**: Fetch → Preprocess → Dedup → Genre tagging → Select → Evidence building → ML clustering → LLM summarization → Persistence.
- **Morning update pipeline**: Fetch → Preprocess → Dedup → Article grouping (deduplication tracking).
- **Rust async stack**: Axum, Tokio, sqlx, reqwest, tracing, Prometheus metrics, Tokio-based schedulers.
- **Hybrid classification**: Centroid-based (Rocchio) + Graph Label Propagation with Golden Dataset training.
- **Strict contracts**: JSON Schema validation for recap-subworker (ML) and news-creator (LLM) responses.
- **Compose-first**: Runs under Docker Compose alongside alt-backend, recap-db (PostgreSQL 18), recap-subworker, and news-creator.
- **Atlas migrations**: Definitive schema lives in `recap-migration-atlas/` (see below).

## System Flow

**詳細なパイプラインフロー図**: 実装に基づいた詳細なフロー図は [`PIPELINE_FLOW.md`](recap-worker/PIPELINE_FLOW.md) を参照してください。

```mermaid
flowchart TD
    Scheduler[Daily JST Daemon<br/>04:00 @ UTC+9] -->|JobContext + genres| API
    Admin[Manual trigger / retry<br/>POST /v1/generate…] --> API
    subgraph Axum Control Plane
        API((Router)) -->|dispatch job| Pipeline
        API --> Health[/Health + Metrics/]
    end
    Pipeline --> FetchStage[Fetch & backup<br/>alt-backend + tag-generator]
    FetchStage --> PreStage[Preprocess<br/>HTML clean + lang detect + tokenize]
    PreStage --> DedupStage[Dedup<br/>XXH3 + sentence filters]
    DedupStage --> GenreStage[Genre tagging<br/>Coarse + Refine]
    GenreStage --> SelectStage[Select<br/>Trim + outlier filter]
    SelectStage --> Evidence[Evidence bundle<br/>per genre]
    Evidence --> Dispatch[Dispatch Stage]
    Dispatch -->|Evidence corpus| Subworker
    Dispatch -->|Clusters| NewsCreator
    Subworker -->|Clustering JSON| Dispatch
    NewsCreator -->|Summary JSON| Dispatch
    Dispatch --> Persist[Persist Stage<br/>recap_outputs]
    Persist --> RecapDB[(recap-db / PostgreSQL)]

    MorningScheduler[Morning Update Daemon] --> MorningPipeline[Morning Pipeline]
    MorningPipeline --> MorningFetch[Fetch 1-day]
    MorningFetch --> MorningPre[Preprocess]
    MorningPre --> MorningDedup[Dedup]
    MorningDedup --> MorningGroup[Group Articles]
    MorningGroup --> MorningDB[(morning_article_groups)]
```

## Key Modules & Directories
| Path | Purpose |
| --- | --- |
| [`recap-worker/src/app.rs`](recap-worker/src/app.rs) | Component registry (Config, DAO, telemetry, scheduler, HTTP clients, two pipeline orchestrators). |
| [`recap-worker/src/api/*.rs`](recap-worker/src/api) | Axum handlers for health, metrics, admin retry, fetch, manual generation, evaluation, and learning. |
| [`recap-worker/src/pipeline/`](recap-worker/src/pipeline) | Pipeline stages (`fetch`, `preprocess`, `dedup`, `genre`, `select`, `dispatch`, `persist`, `evidence`, `morning`). |
| [`recap-worker/src/clients/`](recap-worker/src/clients) | HTTP clients for alt-backend, tag-generator, recap-subworker, and news-creator (with JSON Schema validation). |
| [`recap-worker/src/classifier/`](recap-worker/src/classifier) | Centroid-based classification (Rocchio with multi-centroid, temperature scaling) + Graph label propagation hybrid pipeline. |
| [`recap-worker/src/classification/`](recap-worker/src/classification) | Feature extraction, tokenization utilities, and feature vector construction for classification. |
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
| `GET /v1/morning/updates` | Fetch morning article groups (deduplication relationships). |
| `POST /v1/evaluation/genres` | Run genre classification evaluation on Golden Dataset. Returns `run_id`. |
| `GET /v1/evaluation/genres/latest` | Get latest evaluation run results. |
| `GET /v1/evaluation/genres/{run_id}` | Get specific evaluation run results. |
| `POST /admin/jobs/retry` | Fire-and-forget retry using default Config genres; useful when a batch degraded. |
| `POST /admin/genre-learning` | Receive optimized genre thresholds from recap-subworker and store in `recap_worker_config`. |

## Pipeline Stages
1. **Fetch (`AltBackendFetchStage`)**
   - Pulls paginated articles from alt-backend for the configured `RECAP_WINDOW_DAYS` backfill.
   - Optionally enriches articles with tags from tag-generator service.
   - Wraps requests in exponential backoff (`util::retry`), backs up raw HTML to `recap_job_articles`, and acquires advisory locks to avoid duplicate runs.

2. **Preprocess (`TextPreprocessStage`)**
   - Offloads CPU-heavy normalization via `tokio::task::spawn_blocking` plus semaphore-constrained concurrency.
   - Ammonia cleans HTML, `html2text` strips markup, `whatlang` provides language hints, and Lindera IPADIC tokenizes for downstream classifiers.
   - Extracts tag signals from article metadata.
   - Metrics persisted in `recap_preprocess_metrics` (totals, dropped counts, language distribution).

3. **Dedup (`HashDedupStage`)**
   - Exact + near-duplicate detection using XXH3 hashes and rolling-window similarity; performs sentence-level dedup with per-article stats.
   - Tracks duplicate relationships for morning update pipeline.

4. **Genre (`TwoStageGenreStage`)**
   - **Coarse Pass:** Centroid-based similarity (Rocchio) with multi-centroid support, temperature scaling, and adaptive thresholds. Uses Golden Dataset for training. Falls back to keyword-based classification if centroids unavailable.
   - **Refine Pass (optional):** Graph label propagation for articles that failed coarse pass. Uses `tag_label_graph` from recap-db (cached with TTL). Graph override settings loaded from `recap_worker_config` with YAML fallback.
   - Rollout controlled by `RECAP_GENRE_REFINE_ROLLOUT_PERCENT` (0-100%).
   - Classifier implementation lives in `classifier/` (centroid.rs, graph.rs).

5. **Select (`SummarySelectStage`)**
   - Trims articles per genre (max 20, adjusted for `min_documents_per_genre`).
   - Filters outliers using embedding-based coherence similarity (optional `EmbeddingService`).
   - Ensures minimum document counts per genre.

6. **Evidence (`evidence.rs`)**
   - Groups deduplicated articles per genre, filters short sentences, and tracks metadata (language mix, counts). Output feeds ML dispatch.
   - Enforces per-genre article uniqueness before dispatch.

7. **Dispatch (`MlLlmDispatchStage`)**
   - **Phase 1:** Parallel per-genre flow: send evidence corpus to recap-subworker (`/v1/runs`), poll until success.
   - **Phase 2:** Sequential summary generation: build summary requests for news-creator (`/v1/summary/generate`), process one at a time (queue-based) to respect LLM context limits. Only top 40 clusters (by size) sent to news-creator.
   - Responses pass JSON Schema verification before returning.
   - Representatives persisted to `recap_cluster_evidence` once and later reused by the API.

8. **Persist (`FinalSectionPersistStage`)**
   - Serializes LLM output to `recap_outputs` (JSONB) and `recap_sections`, storing human-readable summary text, bullets, and the full struct for downstream consumers.

## Morning Update Pipeline
- **Purpose:** Tracks article deduplication relationships for frontend grouping.
- **Stages:** Fetch (1-day window) → Preprocess → Dedup → Group persistence.
- **Output:** `morning_article_groups` table with `(group_id, article_id, is_primary)` tuples.
- **Schedule:** Runs via `spawn_morning_update_daemon` (JST-tuned cron).
- **API:** `GET /v1/morning/updates` to fetch latest groups.

## Scheduling & Job Lifecycle
- **Automatic daily batch**: `scheduler::daemon::spawn_jst_batch_daemon` fires at 04:00 JST using `RECAP_GENRES` defaults. Missing defaults result in a warning and skip auto runs.
- **Morning update daemon**: `scheduler::daemon::spawn_morning_update_daemon` runs independently for article grouping.
- **Manual generation**: Invoke `POST /v1/generate/recaps/7days` with optional `{"genres": ["ai", ...]}`; the handler dedupes/normalizes input before queueing the job.
- **Admin retries**: `POST /admin/jobs/retry` triggers a best-effort rerun using the existing scheduler (useful after fixing upstream outages).
- **Job context**: `JobContext` carries `job_id` + genre list throughout the pipeline for logging and persistence.

## External Services & Contracts
| Service | Purpose | Contract Notes |
| --- | --- | --- |
| `alt-backend` | Article source (`/v1/recap/articles`). | Auth via `X-Service-Token` (optional). Paginated; Recap Worker backs up every article into PostgreSQL. |
| `tag-generator` | Optional tag enrichment service. | Base URL configurable via `TAG_GENERATOR_BASE_URL`. When enabled, enriches articles during fetch stage. |
| `recap-subworker` | ML clustering + representative selection. | REST interface at `/v1/runs`. Requests derived from evidence corpus; responses validated against `schema::subworker::CLUSTERING_RESPONSE_SCHEMA`. Includes retry/poll loop with idempotency headers. Supports graph refresh via `/admin/refresh-graph`. |
| `news-creator` | Japanese bullet summaries via LLM (Gemma 3:4B). | `/v1/summary/generate` with optional `SummaryOptions`. Only top 40 clusters (by size) are sent to respect LLM context window limits (8k tokens). Responses validated against `schema::news_creator::SUMMARY_RESPONSE_SCHEMA` before persistence. Sequential processing (queue-based). |

## Configuration & Environment
Full reference lives in [ENVIRONMENT.md](./ENVIRONMENT.md). Highlights:
- **Database**: `RECAP_DB_DSN` (PostgreSQL 18). Connection pooling via `sqlx::postgres::PgPoolOptions`.
- **External endpoints**: `ALT_BACKEND_BASE_URL`, `TAG_GENERATOR_BASE_URL`, `SUBWORKER_BASE_URL`, `NEWS_CREATOR_BASE_URL`.
- **Batch parameters**: `RECAP_WINDOW_DAYS`, `RECAP_GENRES`, `LLM_MAX_CONCURRENCY`, `LLM_PROMPT_VERSION`, `LLM_SUMMARY_TIMEOUT_SECS`.
- **HTTP resiliency**: `HTTP_MAX_RETRIES`, `HTTP_BACKOFF_BASE_MS`, `HTTP_BACKOFF_CAP_MS`, per-client timeout overrides.
- **Classification**: `RECAP_GENRE_MODEL_WEIGHTS`, `RECAP_GENRE_MODEL_THRESHOLD`, `RECAP_GENRE_REFINE_ENABLED`, `RECAP_GENRE_REFINE_ROLLOUT_PERCENT`, `RECAP_GENRE_REFINE_REQUIRE_TAGS`.
- **Graph cache**: `TAG_LABEL_GRAPH_WINDOW`, `TAG_LABEL_GRAPH_TTL_SECONDS`, `RECAP_PRE_REFRESH_GRAPH_ENABLED`, `RECAP_PRE_REFRESH_TIMEOUT_SECS`.
- **Select stage**: `RECAP_MIN_DOCUMENTS_PER_GENRE`, `RECAP_COHERENCE_SIMILARITY_THRESHOLD`.
- **Observability**: `OTEL_EXPORTER_ENDPOINT`, `OTEL_SAMPLING_RATIO`, `RUST_LOG`.

### Golden Dataset for Classification
The classification pipeline uses a hybrid approach combining **Centroid-based Classification (Rocchio)** and **Graph Label Propagation**:

1. **Coarse Pass**: Centroid classifier computes cosine similarity against genre centroids (trained from Golden Dataset). Multi-centroid support allows multiple centroids per genre. Temperature scaling calibrates confidence scores. Default threshold: 0.6 (0.75 for `society_justice`).
2. **Refine Pass (optional)**: Graph propagator applies label propagation for articles that failed coarse pass. Uses similarity threshold 0.85 for edge creation. Integrates `tag_label_graph` for tag-based boosts.
3. **Fallback**: Unclassified articles default to "other".

**Golden Dataset location**:
- **Production/Docker**: `/app/data/golden_classification.json`
- **Development**: `tests/data/golden_classification.json` (relative to `recap-worker/recap-worker/`)

The Dockerfile automatically copies the golden dataset during build. If missing, the system falls back to the legacy `GenreClassifier`.

**Dependencies**: `ndarray` (vector operations), `petgraph` (graph algorithms), `sprs` (sparse matrix operations).

See [IMPLEMENTATION.md](./IMPLEMENTATION.md) for detailed architecture and implementation notes.

## Observability
- **Metrics**: `GET /metrics` surfaces counters (`recap_articles_fetched_total`, `recap_clusters_created_total`, `recap_jobs_failed_total`), histograms (`recap_fetch_duration_seconds`, `recap_job_duration_seconds`, etc.), and gauges (`recap_active_jobs`, `recap_queue_size`). Genre refinement metrics: `recap_genre_refine_rollout_enabled_total`, `recap_genre_refine_rollout_skipped_total`, `recap_genre_refine_graph_hits_total`, `recap_genre_refine_fallback_total`, `recap_genre_refine_llm_latency_seconds`. See `observability/metrics.rs`.
- **Tracing**: `observability::Telemetry` initializes JSON-formatted tracing logs. OTLP export can be enabled once collector availability is confirmed (see `observability/tracing.rs`).
- **Structured logging**: Important events (WARN/ERROR/INFO) emit JSON via `StructuredLogLayer`; redaction helpers live in `util::redact`.
- **Health probes**: `GET /health/live` + `GET /health/ready` integrate with Compose/Infra load balancers.

## Database & Migrations
- **Authoritative migrations**: `recap-migration-atlas/migrations/*.sql` with `atlas.hcl` + `schema.hcl`. Use the Docker helper in `recap-migration-atlas/docker/` (`docker build -t recap-db-migrator …`).
- **Legacy init scripts**: `recap-worker/recap-db/init/*.sql` remain for local bootstraps but defer to Atlas in CI/CD.
- **Recent change (2025-11-08)**: `20251108000100_update_recap_preprocess_metrics.sql` migrated legacy metric kv-pairs into typed columns expected by `RecapDao::save_preprocess_metrics`.
- **Key tables**:
  - `recap_jobs`: Job metadata and status
  - `recap_job_articles`: Raw article backups
  - `recap_outputs`: Final recap JSONB outputs
  - `recap_sections`: Recap section metadata
  - `recap_cluster_evidence`: Pre-deduplicated cluster representatives
  - `recap_genre_learning_results`: Genre refinement learning records
  - `tag_label_graph`: Tag-genre association graph (cached)
  - `recap_worker_config`: Configuration overrides (insert-only)
  - `morning_article_groups`: Article deduplication groups
  - `recap_genre_evaluation_runs`: Evaluation run metadata
  - `recap_genre_evaluation_metrics`: Per-genre evaluation metrics

## Development Workflow
1. **Prerequisites**: Rust 1.87+, `cargo`, PostgreSQL 18, Docker/Compose for integration tests.
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
   - Replay genre pipeline: `cargo run --bin replay_genre_pipeline -- --dataset path/to/dataset.json --dsn $RECAP_DB_DSN`

## Troubleshooting & Maintenance
- **Common issues**: See [TROUBLESHOOTING.md](./TROUBLESHOOTING.md) for DB locks, HTTP timeouts, schema validation failures, and observability tips.
- **Dedup performance**: `docs/dedup_analysis.md` and `docs/dedup_optimization_design.md` outline current hotspots and planned improvements.
- **Service contract drift**: `docs/subworker_404_investigation.md` documents the `/v1/runs` endpoint switch and headers required to avoid `404` responses.
- **Atlas hashes**: Update `atlas.sum` via `recap-migration-atlas/docker/scripts/hash.sh` whenever migrations change.
- **Graph cache refresh**: Ensure `TAG_LABEL_GRAPH_WINDOW` and `TAG_LABEL_GRAPH_TTL_SECONDS` are consistent across services. Refresh graph using `scripts/replay_genre_pipeline.rs` or tag-generator's `build_label_graph.py`.

## Reference Commands
```bash
# Health & metrics
curl http://localhost:9005/health/ready
curl http://localhost:9005/metrics | grep recap_jobs

# Manual trigger
curl -X POST http://localhost:9005/v1/generate/recaps/7days \
  -H "Content-Type: application/json" \
  -d '{"genres":["tech","ai"]}'

# Inspect latest job metadata (psql)
psql $RECAP_DB_DSN -c "SELECT * FROM recap_jobs ORDER BY kicked_at DESC LIMIT 5;"

# Tail worker logs
docker compose logs -f recap-worker

# Run evaluation
curl -X POST http://localhost:9005/v1/evaluation/genres \
  -H "Content-Type: application/json" \
  -d '{"data_path":"/app/data/golden_classification.json"}'
```

Stay aligned with the Compose-first workflow, keep tests green, and run only the minimal set of stages necessary for safe, fast iterations.
