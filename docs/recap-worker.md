# Recap Worker

_Last reviewed: December 2024_

**Location:** `recap-worker/recap-worker`

## Role
- Rust 2024 batch processor that turns the last seven days of articles into curated Japanese recaps.
- Orchestrates the full pipeline—fetch, preprocess, deduplicate, genre-tag, select, evidence build, ML clustering (recap-subworker), LLM summarization (news-creator), and persistence into recap-db—while shipping metrics and admin APIs via Axum.
- Runs two parallel pipelines: **7-day recap generation** (daily at 04:00 JST) and **morning update** (article deduplication grouping).

## Service Snapshot
| Layer | Highlights |
| --- | --- |
| Control Plane | Axum router exposing `/health/live`, `/health/ready`, `/metrics`, `/v1/generate/recaps/7days`, `/v1/recaps/7days`, `/v1/morning/updates`, `/v1/evaluation/genres`, `/admin/jobs/retry`, `/admin/genre-learning`. |
| Pipeline (`src/pipeline/`) | Stages: fetch → preprocess → dedup → genre → select → evidence → dispatch → persist. Morning pipeline: fetch → preprocess → dedup → group persistence. |
| Clients (`src/clients/`) | Typed HTTP clients for alt-backend, tag-generator, recap-subworker (`/v1/runs`), and news-creator (LLM summaries) with JSON Schema validation. |
| Classifier (`src/classifier/`) | Hybrid classification: **Centroid-based (Rocchio)** with multi-centroid support and temperature scaling, plus **Graph Label Propagation** for rescue pass. Golden Dataset training. |
| Store (`src/store/`) | SQLx DAO with advisory locks, recap job metadata, JSONB outputs, the `recap_cluster_evidence` table for pre-deduplicated links, `recap_genre_learning_results`, cached `tag_label_graph` priors, and `morning_article_groups` for deduplication tracking. |
| Observability (`src/observability/`) | Tracing, Prometheus exporter, OTLP wiring plus counters for genre refine rollout gating (`recap_genre_refine_rollout_enabled_total` / `_skipped_total`), graph boosts, fallbacks, and LLM latency. |

## Code Status
- `src/app.rs` assembles config, DAO, telemetry, scheduler, HTTP clients, and two pipeline orchestrators (7-day recap + morning update), then launches both the control plane and pipeline runners.
- Scheduler defaults to a JST-tuned cron (04:00 UTC+9) for 7-day recaps and morning updates, but manual runs are supported via `POST /v1/generate/recaps/7days`.
- Pipeline stages:
  1. **Fetch:** Pulls articles from alt-backend for `RECAP_WINDOW_DAYS`, optionally enriches with tag-generator tags, backs up raw HTML to `recap_job_articles`, and records job metadata.
  2. **Preprocess:** Cleans HTML (ammonia/html2text), normalizes Unicode, detects language (`whatlang`), tokenizes (Lindera IPADIC), and extracts tag signals. CPU-heavy work offloaded via `spawn_blocking` with semaphore-constrained concurrency.
  3. **Dedup:** XXH3 hashing + sentence filters to drop near-duplicates. Per-article stats tracked.
  4. **Genre:** Hybrid two-stage classifier:
     - **Coarse Pass:** Centroid-based similarity (Rocchio) with multi-centroid support, temperature scaling, and adaptive thresholds. Uses Golden Dataset for training. Falls back to keyword-based classification if centroids unavailable.
     - **Refine Pass (optional):** Graph label propagation for articles that failed coarse pass. Uses `tag_label_graph` from recap-db (cached with `TAG_LABEL_GRAPH_WINDOW` and `TAG_LABEL_GRAPH_TTL_SECONDS`). Graph override settings loaded from `recap_worker_config` table (latest by `created_at DESC`) with YAML fallback via `GRAPH_CONFIG` environment variable. Rollout controlled by `RECAP_GENRE_REFINE_ROLLOUT_PERCENT`.
  5. **Select:** Trims articles per genre (max 20, adjusted for `min_documents_per_genre`), filters outliers using embedding-based coherence similarity (optional `EmbeddingService`), and ensures minimum document counts.
  6. **Evidence:** Bundles articles per genre, capturing language mix + metadata; enforces per-genre article uniqueness before dispatch so the downstream evidence payload already reflects the final cap.
  7. **Dispatch:** Sends corpora to recap-subworker (clustering) in parallel per genre, then generates summaries via news-creator sequentially (queue-based to respect LLM context limits). Only top 40 clusters (by size) sent to news-creator. Responses validated against JSON Schema before returning. Representatives persisted to `recap_cluster_evidence` once and later reused by the API.
  8. **Persist:** Writes recap sections + evidence to `recap_outputs`/`recap_jobs` tables inside recap-db.
- **Morning Pipeline:** Runs fetch → preprocess → dedup, then groups articles by deduplication relationships into `morning_article_groups` table (primary article + duplicates mapping).
- JSON Schema contracts for recap-subworker/news-creator responses live alongside clients; failed validation short-circuits persistence and surfaces metrics.

## Classification Architecture
The genre classification uses a **hybrid two-stage approach**:

### Stage 1: Coarse Pass (Centroid Classifier)
- **Multi-Centroid Rocchio:** Each genre can have multiple centroid vectors (trained from Golden Dataset).
- **Temperature Scaling:** Calibrates confidence scores using temperature parameter (default 1.0, optimized via validation set).
- **Adaptive Thresholds:** Genre-specific thresholds (default 0.6, 0.75 for `society_justice`).
- **Feature Vector:** Combines TF-IDF, BM25, token counts, and tag overlap into a unified feature vector.
- **Golden Dataset:** Located at `/app/data/golden_classification.json` (production) or `tests/data/golden_classification.json` (development).

### Stage 2: Refine Pass (Graph Label Propagation)
- **Trigger:** Only for articles that failed coarse pass (below threshold).
- **Graph Construction:** Builds undirected graph where edges connect articles with similarity ≥ 0.85 (cosine similarity on combined feature vectors).
- **Label Propagation:** Uses sparse matrix operations (`sprs`) for efficient label spreading from labeled (Golden Dataset) to unlabeled articles.
- **Tag Label Graph Integration:** Uses cached `tag_label_graph` from recap-db to boost confidence for articles with matching tag-genre associations.
- **Fallback:** Unclassified articles default to "other".

### Configuration
- **Rollout Control:** `RECAP_GENRE_REFINE_ENABLED` (master switch), `RECAP_GENRE_REFINE_ROLLOUT_PERCENT` (0-100% of articles).
- **Graph Cache:** `TAG_LABEL_GRAPH_WINDOW` (default "7d"), `TAG_LABEL_GRAPH_TTL_SECONDS` (default 900).
- **Graph Overrides:** Stored in `recap_worker_config` table, loaded on pipeline initialization with YAML fallback.

## Replay & Evaluation
- `scripts/replay_genre_pipeline.rs` (and the `replay` module under `src/replay.rs`) replays the genre refinement stage using JSONL datasets, reloads `tag_label_graph` (honouring `TAG_LABEL_GRAPH_WINDOW` and `TAG_LABEL_GRAPH_TTL_SECONDS`), and persists tightened rows into `recap_genre_learning_results`. Use flags such as `--dataset`, `--dsn`, `--graph-window`, `--graph-ttl`, `--require-tags`, and `--dry-run` to validate revisions safely.
- Summary quality is guarded by the golden dataset evaluation stack in `recap-worker/tests/golden_eval.rs` and `src/evaluation/golden.rs`, which loads `recap-worker/resources/golden_runs.json`, computes ROUGE, and fails the suite if precision dips below the acceptable threshold; run `cargo test -p recap-worker tests::golden_eval` (or rerun `scripts/replay_genre_pipeline.rs` after prompt/model tweaks) whenever you tweak summarization prompts or reference evidence.
- Genre classification evaluation via `POST /v1/evaluation/genres` endpoint, which runs classification on Golden Dataset and stores metrics in `recap_genre_evaluation_runs` and `recap_genre_evaluation_metrics` tables.

## Integrations & Data
- **recap-db (Postgres 18):** Source of truth for jobs, cached articles, `recap_cluster_evidence`, `recap_genre_learning_results`, `tag_label_graph`, `recap_worker_config` (insert-only config storage), `morning_article_groups`, and final recaps. Schema maintained via Atlas migrations in `recap-migration-atlas/` (see `20251112000100_add_cluster_evidence_table.sql`, `20251113000100_create_tag_label_graph.sql`, `20251113093000_add_genre_learning_results.sql`, `20251120000000_create_recap_worker_config.sql`). Refresh the graph using `scripts/replay_genre_pipeline.rs` or `tag-generator/app/scripts/build_label_graph.py` whenever you adjust `TAG_LABEL_GRAPH_WINDOW`/`TAG_LABEL_GRAPH_TTL_SECONDS`. Graph override settings are stored in `recap_worker_config` and loaded on pipeline initialization with YAML fallback.
- **recap-subworker:** Receives evidence corpus, returns clustering JSON with trimmed, per-genre-unique representatives. Supports graph refresh via `/admin/refresh-graph` endpoint.
- **news-creator:** Generates summaries per cluster/genre. Sequential processing to respect LLM context window limits (8k tokens).
- **alt-backend:** Provides raw article feed via authenticated HTTP client (`/v1/recap/articles`).
- **tag-generator:** Optional tag enrichment service. When enabled, fetches tags during fetch stage to improve classification accuracy.

## Testing & Tooling
- `cargo test -p recap-worker` for unit/integration suites (Axum handlers, pipeline stages, DAO, clients, classifier).
- `cargo bench -p recap-worker --bench performance` to profile preprocessing + keyword scoring.
- Health scripts:
  - `curl http://localhost:9005/health/ready`
  - `curl http://localhost:9005/metrics | egrep 'recap_api_(latest_fetch|cluster_query)_duration_seconds'`
- DB inspection: `psql $RECAP_DB_DSN -c "SELECT * FROM recap_jobs ORDER BY kicked_at DESC LIMIT 5;"`.
- Troubleshooting references:
  - `recap-worker/TROUBLESHOOTING.md`
  - `recap-worker/docs/dedup_analysis.md`
  - `recap-worker/docs/subworker_404_investigation.md`
- Golden dataset evaluation: `recap-worker/tests/golden_eval.rs` exercises `resources/golden_runs.json` and the ROUGE helpers in `src/evaluation/golden.rs`; run `cargo test -p recap-worker tests::golden_eval` and/or `scripts/replay_genre_pipeline.rs` anytime you change prompts, clustering, or tag priors.

## Operational Notes
- Compose profile includes recap-db and recap-subworker; run `docker compose --profile logging --profile ollama up recap-worker recap-db recap-subworker`.
- Manual trigger: `curl -X POST http://localhost:9005/v1/generate/recaps/7days -H "Content-Type: application/json" -d '{"genres":["tech","finance"]}'`.
- Genre learning endpoint: `POST /admin/genre-learning` receives optimized thresholds from recap-subworker and stores them in `recap_worker_config` table. Settings are loaded on pipeline initialization with YAML fallback (via `GRAPH_CONFIG` env var).
- Jobs acquire advisory locks per window to prevent overlaps; clear stuck locks via `SELECT pg_advisory_unlock_all();` when safe.
- Run the Atlas migration that creates `recap_cluster_evidence` before deploying; verify population with `SELECT COUNT(*) FROM recap_cluster_evidence;` after the first recap completes.
- Monitor GET `/v1/recaps/7days` latency via `recap_api_latest_fetch_duration_seconds` and the new duplicate counter `recap_api_evidence_duplicates_total` to confirm dedup is happening before DTO assembly.
- Keep JSON Schema versions in sync with downstream services before deploying new payload fields.
- Grafana: import `observability/grafana/recap-genre-dashboard.json` to surface `genre_tag_agreement_rate`, `recap_genre_tag_missing_ratio`, and `recap_genre_graph_hits_total`. Alertmanager rules live in `observability/alerts/recap-genre-rules.yaml`.
- Rollout controls: use `RECAP_GENRE_REFINE_ENABLED` plus the new `RECAP_GENRE_REFINE_ROLLOUT_PERCENT` (10/50/100) to gate the corpus. The new counters `recap_genre_refine_rollout_enabled_total` and `_skipped_total` plus `recap_genre_refine_graph_hits_total`/`recap_genre_refine_fallback_total`/`recap_genre_refine_llm_latency_seconds` reflect deployment coverage, Graph boosts, fallback hits, and LLM latency respectively. See `docs/recap-genre-rollout-runbook.md` for the Phase 5 playbook.
- Replay helper: run `cargo run --bin replay_genre_pipeline -- --dataset path/to/dataset.json --dsn $RECAP_DB_DSN --graph-window 7d --graph-ttl 900` (or use the script alias) to re-run the genre pipeline offline, refresh `recap_genre_learning_results`, and verify `tag_label_graph` outputs when you adjust TTLs or priors. Ensure `TAG_LABEL_GRAPH_WINDOW`/`TAG_LABEL_GRAPH_TTL_SECONDS` stay in sync across `.env`, `tag-generator`, and the running worker.
- Graph pre-refresh: `RECAP_PRE_REFRESH_GRAPH_ENABLED` (default true) triggers graph refresh before pipeline execution via recap-subworker's `/admin/refresh-graph` endpoint. Timeout controlled by `RECAP_PRE_REFRESH_TIMEOUT_SECS` (default 300).
- Morning update pipeline: Runs independently via `spawn_morning_update_daemon`, processes 1-day window, and persists article groups to `morning_article_groups` table. Accessible via `GET /v1/morning/updates` API.

## LLM Tips
- Specify stage/module when asking for changes (e.g., "update `src/pipeline/dedup.rs` to tweak XXH3 threshold").
- Mention corresponding schema files if altering recap outputs to ensure DAO + JSON validators are updated together.
- Classification changes: update Golden Dataset at `tests/data/golden_classification.json` and retrain centroids if modifying feature extraction or thresholds.
