# Recap Worker

_Last reviewed: November 12, 2025_

**Location:** `recap-worker/recap-worker`

## Role
- Rust 2024 batch processor that turns the last seven days of articles into curated Japanese recaps.
- Orchestrates the full pipeline—fetch, preprocess, deduplicate, genre-tag, evidence build, ML clustering (recap-subworker), LLM summarization (news-creator), and persistence into recap-db—while shipping metrics and admin APIs via Axum.

## Service Snapshot
| Layer | Highlights |
| --- | --- |
| Control Plane | Axum router exposing `/health/live`, `/health/ready`, `/metrics`, `/v1/generate/recaps/7days`, `/admin/jobs/retry`. |
| Pipeline (`src/pipeline/`) | Stages: fetch → preprocess → dedup → genre → evidence → dispatch → persist. |
| Clients (`src/clients/`) | Typed HTTP clients for alt-backend, recap-subworker (`/v1/runs`), and news-creator (LLM summaries) with JSON Schema validation. |
| Store (`src/store/`) | SQLx DAO with advisory locks, recap job metadata, JSONB outputs, and the new `recap_cluster_evidence` table for pre-deduplicated links. |
| Observability (`src/observability/`) | Tracing, Prometheus exporter, OTLP wiring. |

## Code Status
- `src/app.rs` assembles config, DAO, telemetry, scheduler, and HTTP clients, then launches both the control plane and pipeline runner.
- Scheduler defaults to a JST-tuned cron (04:00 UTC+9) but manual runs are supported via `POST /v1/generate/recaps/7days`.
- Pipeline stages:
  1. **Fetch:** Pulls articles from alt-backend for `RECAP_WINDOW_DAYS`, backs up raw HTML, and records job metadata.
  2. **Preprocess:** Cleans HTML (ammonia/html2text), normalizes Unicode, detects language (`whatlang`), and tokenizes.
  3. **Dedup:** XXH3 hashing + sentence filters to drop near-duplicates.
  4. **Genre:** Hybrid heuristic classifier assigns up to 3 genres per article.
 5. **Evidence:** Bundles articles per genre, capturing language mix + metadata; now enforces per-genre article uniqueness before dispatch so the downstream evidence payload already reflects the final cap.
 6. **Dispatch:** Sends corpora to recap-subworker (clustering) and news-creator (LLM summary) with strict schema validation + retries; the returned representatives are persisted to `recap_cluster_evidence` once and later reused by the API.
  7. **Persist:** Writes recap sections + evidence to `recap_outputs`/`recap_jobs` tables inside recap-db.
- JSON Schema contracts for recap-subworker/news-creator responses live alongside clients; failed validation short-circuits persistence and surfaces metrics.

## Integrations & Data
- **recap-db (Postgres 16):** Source of truth for jobs, cached articles, `recap_cluster_evidence`, and final recaps. Schema maintained via Atlas migrations in `recap-migration-atlas/` (see `20251112000100_add_cluster_evidence_table.sql`).
- **recap-subworker:** Receives evidence corpus, returns clustering JSON with trimmed, per-genre-unique representatives.
- **news-creator:** Generates summaries per cluster/genre.
- **alt-backend:** Provides raw article feed via authenticated HTTP client.

## Testing & Tooling
- `cargo test -p recap-worker` for unit/integration suites (Axum handlers, pipeline stages, DAO, clients).
- `cargo bench -p recap-worker --bench performance` to profile preprocessing + keyword scoring.
- Health scripts:
  - `curl http://localhost:9005/health/ready`
- `curl http://localhost:9005/metrics | egrep 'recap_api_(latest_fetch|cluster_query)_duration_seconds'`
- DB inspection: `psql $RECAP_DB_DSN -c "SELECT * FROM recap_jobs ORDER BY kicked_at DESC LIMIT 5;"`.
- Troubleshooting references:
  - `recap-worker/TROUBLESHOOTING.md`
  - `recap-worker/docs/dedup_analysis.md`
  - `recap-worker/docs/subworker_404_investigation.md`

## Operational Notes
- Compose profile includes recap-db and recap-subworker; run `docker compose --profile logging --profile ollama up recap-worker recap-db recap-subworker`.
- Manual trigger: `curl -X POST http://localhost:9005/v1/generate/recaps/7days -H "Content-Type: application/json" -d '{"genres":["tech","finance"]}'`.
- Jobs acquire advisory locks per window to prevent overlaps; clear stuck locks via `SELECT pg_advisory_unlock_all();` when safe.
- Run the Atlas migration that creates `recap_cluster_evidence` before deploying; verify population with `SELECT COUNT(*) FROM recap_cluster_evidence;` after the first recap completes.
- Monitor GET `/v1/recaps/7days` latency via `recap_api_latest_fetch_duration_seconds` and the new duplicate counter `recap_api_evidence_duplicates_total` to confirm dedup is happening before DTO assembly.
- Keep JSON Schema versions in sync with downstream services before deploying new payload fields.

## LLM Tips
- Specify stage/module when asking for changes (e.g., “update `src/pipeline/dedup.rs` to tweak XXH3 threshold”).
- Mention corresponding schema files if altering recap outputs to ensure DAO + JSON validators are updated together.
