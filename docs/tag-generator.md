# Tag Generator

_Last reviewed: November 10, 2025_

**Location:** `tag-generator/app`

## Role
- Python 3.13 service that continuously processes untagged articles, extracts ML-driven tags, and upserts them into Postgres for personalization/search.
- Runs as a long-lived worker with manual memory management to keep NLP workloads stable.

## Service Snapshot
| Module | Responsibility |
| --- | --- |
| `ArticleFetcher` | Pulls batches of untagged articles using pagination (timestamp + ID cursor). |
| `TagExtractor` | Applies NLP models from `tag_extractor/models` and validates outputs. |
| `TagInserter` | Performs upserts back into Postgres tables. |
| `TagGeneratorService` | Orchestrates fetch → extract → insert pipeline, manages cursors + health counters. |

## Code Status
- `TagGeneratorConfig` (dataclass) controls `processing_interval`, `batch_limit`, retry delays, GC cadence, health thresholds, and cursor recovery behavior.
- Service tracks `last_processed_created_at` and `last_processed_id` to resume work; `cursor poisoning` detection compares timestamps to current UTC and falls back to recovery queries when windows look suspicious.
- DB connections use direct `psycopg2.connect` (pool disabled) with autocommit and explicit retry loops (`max_connection_retries`, `connection_retry_delay`).
- Health metrics (total cycles, empty cycles, articles processed) feed into logs for dashboards.

## Integrations & Data
- Env vars: `DB_TAG_GENERATOR_USER`, `DB_TAG_GENERATOR_PASSWORD`, `DB_HOST`, `DB_PORT`, `DB_NAME`. Missing vars raise `ValueError`.
- Logging: `structlog` configured via `tag_generator/logging_config.py` (JSON output, friendly to Rask pipeline). Logs include cursor state, batch stats, GC actions.
- Tag extractor models: place new assets in `tag_extractor/models`; update `TagExtractor` to select them based on config or heuristics.

## Testing & Tooling
- `uv run pytest` covers `tests/unit` (extractor, fetcher, inserter) + `tests/integration` (pipeline).
- Static analysis: `uv run mypy`, `uv run ruff check`, `uv run ruff format`.
- When adjusting cursor logic, add tests in `tests/unit/test_service_cursor.py` (create if missing) to simulate poisoning scenarios.

## Operational Runbook
1. Ensure DB env vars exist, then run `uv run python main.py` (or use Compose service).
2. Monitor logs for `Tagged batch` entries—each includes `batch_size`, `cursor`, `total_articles_processed`.
3. GC tuning: set `ENABLE_GC_COLLECTION=false` only when diagnosing GC thrash; prefer adjusting `memory_cleanup_interval`.
4. Recovery mode: if you manually backfill articles, restart the service so it re-evaluates cursor poisoning and re-enters recovery if necessary.

## LLM Notes
- When generating edits, specify whether change touches `ArticleFetcher`, `TagExtractor`, `TagInserter`, or the orchestration service.
- Provide exact env names (`DB_TAG_GENERATOR_USER`, etc.) and note that DSN assembly lives in `_get_database_dsn()`—LLMs should edit that function rather than duplicating DSN logic elsewhere.

## Supporting Scripts
- `tag-generator/app/scripts/build_label_graph.py`: builds the rolling `tag_label_graph` used by Recap Worker. The script reads from `recap_genre_learning_results`, aggregates high-confidence tags per genre, and upserts results into `recap-db`. Run it with `RECAP_DB_DSN` (or `--dsn`), optionally overriding windows (`--windows 7,30`), `--max-tags`, and `--min-support`. Use `--dry-run` during verification—successful executions log how many edges were refreshed per window.
