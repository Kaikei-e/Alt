# Tag Generator

_Last reviewed: November 14, 2025_

**Location:** `tag-generator/app`

## Role
- Python 3.13 service that continuously processes untagged articles, extracts ML-driven tags, and upserts them into Postgres for personalization/search.
- Runs as a long-lived worker with manual memory management to keep NLP workloads stable.

## Service Snapshot
| Module | Responsibility |
| --- | --- |
| `ArticleFetcher` | Pulls batches of untagged articles using pagination (timestamp + ID cursor). |
| `TagExtractor` | Applies NLP + sanitization layers and returns `TagExtractionOutcome` metrics for cascade decisions. |
| `TagInserter` | Performs upserts back into Postgres tables, exposing `batch_upsert_tags` and `batch_upsert_tags_no_commit`. |
| `tag_generator.cascade.CascadeController` | Cost-sensitive gate that decides when tags need cascade refinement (confidence, tag count, inference latency, and rate budget). |
| `TagGeneratorService` | Orchestrates fetch → extract → cascade → insert pipeline, manages cursors + health counters, and logs cascade metadata. |

## Service APIs
- `main.py` exposes `FastAPI` routes via `auth_service.py`, wires `AuthenticatedTagGeneratorService`, and runs `lifespan` hooks that warm the tag service and start the background `TagGeneratorService` thread (`_run_background_tag_generation`). The background thread uses the same config and logging stack so job metrics stay centralised inside the FastAPI process while the API remains responsive.
- `/api/v1/generate-tags` keeps serving user-specific requests (the authenticated `generate_tags_endpoint` logs `user_id`, `article_id`, and the sanitized tag list), but new instrumentation now records cascade outcomes and inference latency for both the API and background worker.
- `/api/v1/tags/batch` is a service-to-service endpoint guarded by `verify_service_token`: it demands an `X-Service-Token` header that matches the configured `SERVICE_SECRET` (missing secrets emit 500, missing/invalid headers emit 401/403). Requests accept `{"article_ids":[... up to 1000 ...]}` and delegate to `tag_fetcher.fetch_tags_by_article_ids`, which queries `articles`, `article_tags`, and `feed_tags` for tag names, confidences, and timestamps before returning the consolidated map.
- `tag_fetcher.py` builds its DSN from the same `DB_TAG_GENERATOR_*` envs as the worker, uses a connection pool, and returns ISO-formatted `updated_at` values. The new module ensures the `batch` endpoint avoids duplicating DSN logic and keeps structured logs (`article_count`, `tags_with_data`).

The Compose `tag-generator` service now mounts `./tag-generator/models/onnx` at `/models/onnx`, so `TagExtractor` (via `TAG_ONNX_MODEL_PATH` / `use_onnx_runtime`) loads the quantized ONNX sentence-transformer. If the mount is empty, the runtime gracefully falls back to the SentenceTransformer/KeyBERT graph while still emitting cascade metrics.

## Code Status
- `TagGeneratorConfig` (dataclass) controls `processing_interval`, `batch_limit`, retry delays, GC cadence, health thresholds, and cursor recovery behavior.
- Service tracks `last_processed_created_at` and `last_processed_id` to resume work; `cursor poisoning` detection compares timestamps to current UTC and falls back to recovery queries when windows look suspicious.
- DB connections use direct `psycopg2.connect` (pool disabled) with autocommit and explicit retry loops (`max_connection_retries`, `connection_retry_delay`).
- Health metrics (total cycles, empty cycles, articles processed) feed into logs for dashboards.
- `TagExtractor` now returns `TagExtractionOutcome` (tags, confidence, inference latency, sanitized length) and the service uses `CascadeController` to gate whether to refine/re-run downstream work before handing data to `TagInserter`.
- `TagInserter` exposes both auto-commit and caller-managed batch paths (`batch_upsert_tags`, `batch_upsert_tags_no_commit`) so transactions can stay open until cascade signals settle.
- `TagExtractor` loads the ONNX Runtime path by default (toggle via `TAG_ONNX_MODEL_PATH`); when the model is missing it transparently falls back to the SentenceTransformer/KeyBERT graph while preserving the cascade diagnostics.

## Integrations & Data
- Env vars: `DB_TAG_GENERATOR_USER`, `DB_TAG_GENERATOR_PASSWORD`, `DB_HOST`, `DB_PORT`, `DB_NAME`. Missing vars raise `ValueError`.
- Logging: `structlog` configured via `tag_generator/logging_config.py` (JSON output, friendly to Rask pipeline). Logs include cursor state, batch stats, GC actions.
- Tag extractor models: place new assets in `tag_extractor/models`; when `use_onnx_runtime` is enabled, the service loads the quantized ONNX sentence-transformer, tokenizes with the transformers fast tokenizer (FlashTokenizer/Rust-based), and reuses `TagExtractionOutcome` metrics for cascade decisions.

## Testing & Tooling
- `uv run pytest` covers `tests/unit` (extractor, fetcher, inserter) + `tests/integration` (pipeline); unit tests now exercise `extract_tags_with_metrics`, cascade heuristics, and the updated service contracts.
- Static analysis: `uv run mypy`, `uv run ruff check`, `uv run ruff format`.
- When adjusting cursor logic, add tests in `tests/unit/test_service_cursor.py` (create if missing) to simulate poisoning scenarios.

## Operational Runbook
1. Ensure DB env vars exist, then run `uv run python main.py` (or use Compose service).
2. Monitor logs for `Tagged batch` entries—each now also emits `cascade` metadata (`needs_refine`, `reason`, `confidence`, `refine_ratio`, `inference_ms`) alongside `batch_size`, `cursor`, `total_articles_processed`.
3. GC tuning: set `ENABLE_GC_COLLECTION=false` only when diagnosing GC thrash; prefer adjusting `memory_cleanup_interval`.
4. Recovery mode: if you manually backfill articles, restart the service so it re-evaluates cursor poisoning and re-enters recovery if necessary.

## LLM Notes
- When generating edits, specify whether change touches `ArticleFetcher`, `TagExtractor`, `TagInserter`, or the orchestration service.
- Provide exact env names (`DB_TAG_GENERATOR_USER`, etc.) and note that DSN assembly lives in `_get_database_dsn()`—LLMs should edit that function rather than duplicating DSN logic elsewhere.

## Supporting Scripts
- `tag-generator/app/scripts/build_label_graph.py`: builds the rolling `tag_label_graph` used by Recap Worker. The script reads from `recap_genre_learning_results`, aggregates high-confidence tags per genre, and upserts results into `recap-db`. Run it with `RECAP_DB_DSN` (or `--dsn`), optionally overriding windows (`--windows 7,30`), `--max-tags`, and `--min-support`. Use `--dry-run` during verification—successful executions log how many edges were refreshed per window.
- Compose passes `TAG_LABEL_GRAPH_WINDOW` (default `7d`) and `TAG_LABEL_GRAPH_TTL_SECONDS` (default `900`) so the offline script, background worker, and recap-worker align on the same graph windows/TTL before persisting the results.
