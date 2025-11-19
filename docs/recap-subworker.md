# Recap Subworker

_Last reviewed: November 12, 2025_

**Location:** `recap-subworker/`

## Role
- FastAPI + Gunicorn service that receives per-genre corpora from Recap Worker and returns clustering metadata + representative sentences for Gemma-based summary generation.
- Provides `/v1/runs` (submission + polling) and `/admin/warmup` APIs, backed by Postgres (`recap-db`) for run state, diagnostics, and evidence storage.
- Runs entirely on CPU (BGE-M3 embeddings + UMAP/HDBSCAN) with optional ONNX/distill backends, and now guarantees each article contributes to at most one cluster per genre.

## Runtime Snapshot
| Layer | Details |
| --- | --- |
| HTTP Edge | `gunicorn` + `uvicorn.workers.UvicornWorker`, config in `recap_subworker/infra/gunicorn_conf.py`. Workers = `RECAP_SUBWORKER_GUNICORN_WORKERS` (default `2*CPU+1`) with `max_requests` recycling + 120s timeout. |
| App Factory | `recap_subworker.app.main:create_app` wires routers (`/health`, `/admin`, `/v1/runs`), telemetry, DI singletons. |
| Pipeline Execution | `PipelineTaskRunner` launches a dedicated `ProcessPoolExecutor` (spawn) when `RECAP_SUBWORKER_PIPELINE_MODE=processpool`. Each worker process loads its own embedder/pipeline so CPU-bound clustering can’t hang ASGI threads. In-process mode falls back to a shared `EvidencePipeline`. |
| Embeddings | `sentence-transformers` (default `BAAI/bge-m3`) with LRU cache + optional distill or ONNX runtime (`RECAP_SUBWORKER_MODEL_BACKEND`). |
| Clustering | `umap-learn` + `hdbscan` + c-TF-IDF topic extraction, dedup via cosine threshold (default 0.92) and MMR selection per cluster. |
| Persistence | Async SQLAlchemy DAO inserts run rows, clusters, diagnostics, and the new `recap_cluster_evidence` records that hold de-duplicated representative links; advisory locking handled upstream by Recap Worker. |

## Key Environment Variables (see `recap_subworker/infra/config.py`)
| Variable | Default | Purpose |
| --- | --- | --- |
| `RECAP_SUBWORKER_DB_URL` | `postgresql+asyncpg://...` | Async connection string for recap-db. |
| `RECAP_SUBWORKER_MODEL_BACKEND` | `sentence-transformers` | `onnx` / `hash` supported for CPU-only workflows. |
| `RECAP_SUBWORKER_PIPELINE_MODE` | `inprocess` | Set to `processpool` (default via Compose) to isolate embeddings/clustering.
| `RECAP_SUBWORKER_PIPELINE_WORKER_PROCESSES` | `2` | Number of pipeline worker processes. |
| `RECAP_SUBWORKER_MAX_BACKGROUND_RUNS` | `2` | Concurrency limit enforced by `RunManager`. |
| `RECAP_SUBWORKER_RUN_EXECUTION_TIMEOUT_SECONDS` | `900` | Hard timeout per genre run. |
| `RECAP_SUBWORKER_QUEUE_WARNING_THRESHOLD` | `25` | Emits warning when queued background runs exceed this value. |
| `RECAP_SUBWORKER_GUNICORN_*` | see config | Tune workers, timeouts, recycling. |
| `RECAP_SUBWORKER_MODEL_ID` / `DISTILL_MODEL_ID` | BGE-M3 / distill | Control embedder weights. |

Compose (`compose.yaml`) enables `RECAP_SUBWORKER_PIPELINE_MODE=processpool` by default so recap-worker can continue dispatching even if one pipeline job wedges.

## API & Flow
1. Recap Worker `POST /v1/runs` (per genre) with headers `X-Alt-Job-Id`, `X-Alt-Genre`, optional `Idempotency-Key` + JSON payload describing article corpus + params.
2. Run Manager stores request, enqueues background task (respecting semaphore), and immediately returns `202 { run_id }`.
3. Background worker executes:
   - Validate schema and sentence budget.
   - Embed sentences (batch size = `RECAP_SUBWORKER_BATCH_SIZE`).
   - Deduplicate, cluster, and select representatives while skipping any article already claimed by an earlier cluster in the same genre.
   - Persist clusters, `recap_cluster_evidence`, and diagnostics; mark status `succeeded` / `partial` / `failed`.
4. Recap Worker polls `GET /v1/runs/{run_id}` until status ready. Any timeout/failure is surfaced via diagnostics and Recap Worker skips that genre.
5. `/admin/warmup` triggers embedder warmup (in worker pool when available) to avoid first-request latency.
6. `/admin/learning` triggers genre learning analysis from `recap_genre_learning_results`, runs Bayes optimization, and sends optimized thresholds to recap-worker's `/admin/genre-learning` endpoint for database storage.

## Observability & Operations
- Metrics: Prometheus via `prometheus-fastapi-instrumentator` (`/metrics`) including embedding seconds, HDBSCAN latency, dedup counts.
- Evidence-level metrics (dedup removed, per-cluster sizes) now reflect the per-genre cap because duplicates are filtered before persistence; inspect `recap_cluster_evidence` row counts when diagnosing empty evidence links upstream.
- Logs: JSON via `structlog`; include `job_id`, `genre`, `run_id`. Warnings emitted when queue depth exceeds `RECAP_SUBWORKER_QUEUE_WARNING_THRESHOLD` or when pipeline timeouts occur.
- Health: `/health/live` + `/health/ready` (FastAPI standard). Always verify `/health/ready` before triggering recap-worker manual batches; otherwise you’ll see `Connection refused` for all genres.
- Warmup: `curl -X POST http://localhost:8002/admin/warmup` (optionally include sample sentences in body) after container start to prime embeddings.
- Restart behavior: Gunicorn `max_requests/max_requests_jitter` forces periodic worker recycling; if the pipeline worker pool process wedges, `docker compose restart recap-subworker` clears it.

## Recent updates
- Recap worker now persists deduplicated `recap_cluster_evidence`, so the subworker enforces the per-genre cap before writing evidence and relies on the table to feed the public `/v1/recap/7days` API; check the `recap_cluster_evidence` counts and diagnostics when you adjust clustering thresholds or pipeline timeout figures.

## Testing
| Type | Command |
| --- | --- |
| Unit | `uv run pytest tests/unit` |
| Integration (requires Postgres) | `uv run pytest tests/integration` |
| Lint / Type | `uv run ruff check` / `uv run mypy` |

CI note: the repo currently blocks network egress in some environments; set `UV_CACHE_DIR` to a writable path with cached wheels when running tests offline.

## Troubleshooting
- **All genres fail with `Connection refused`**: recap-worker started before recap-subworker finished booting. Wait for gunicorn workers to log `Application startup complete`, confirm `/health/ready`, then retry.
- **Persistent timeouts per genre**: inspect recap-subworker logs for `run.process.timeout` or `pipeline timed out`. Increase `RECAP_SUBWORKER_RUN_EXECUTION_TIMEOUT_SECONDS` or add capacity (more pipeline processes, larger `MAX_BACKGROUND_RUNS`). If clusters return with zero supporting IDs, confirm `recap_cluster_evidence` is populated (missing rows fall back to sentence queries upstream, which is slower).
- **High CPU**: Check `PipelineTaskRunner` workers (`ps` inside container). Misbehaving runs can be killed by restarting the service; long term, adjust clustering params (`HDBSCAN` min cluster size/samples) or embedder backend.
