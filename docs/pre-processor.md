# Pre-processor

_Last reviewed: November 10, 2025_

**Location:** `pre-processor/app`

## Role
- Go 1.24 worker that fetches feeds, summarizes articles, and enforces quality gates before downstream LLMs consume content.
- Runs cron-style jobs (feed processor, summarizer, quality checker) plus exposes `/api/v1/summarize` and `/api/v1/health` for ad-hoc access.

## Service Snapshot
| Component | Description |
| --- | --- |
| `handler/job_handler.go` | Schedules feed/summarization/quality jobs (feed job currently disabled for ethics hold). |
| `service/*` | Business logic: `FeedProcessorService`, `ArticleSummarizerService`, `QualityCheckerService`, `HealthCheckerService`. |
| `repository/*` | Postgres repositories for feeds, articles, summaries, plus external API repository. |
| `config/config.go` | Comprehensive env loader (server, HTTP client, retry, rate limit, DLQ, metrics, News Creator). |
| `utils/logger` | Structured slog logger with optional Rask forwarding. |

## Code Status
- `main.go` loads logger config, initializes Postgres via `driver.Init`, builds repositories/services, and spins up Echo with `/api/v1/summarize` + `/api/v1/health`.
- Feed processing loop is commented out (ethical pause); summarization + quality jobs start automatically and respect `BATCH_SIZE` + `NEWS_CREATOR_URL` constants unless overridden by env.
- HTTP server hides behind `HTTP_PORT` (defaults to 9200) and uses middleware for logging, recovery, and permissive CORS (adjust before exposing publicly).
- `service.NewArticleSummarizerService` coordinates article fetch, call to `news-creator`, and summary persistence; rate limiting and circuit breakers wrap outbound calls.

## Integrations & Data
- **Postgres:** Article/feed/summary repositories share a pgx pool; ensure env vars (`DB_HOST`, `DB_USER`, etc.) align with Compose service names.
- **News Creator:** External API repository hits `http://news-creator:11434/api/v1/summarize` (configurable). Timeout defaults to 240s; adjust via `NEWS_CREATOR_TIMEOUT`.
- **HTTP fetchers:** Use `HTTPConfig` to control user-agent rotation, Envoy proxying, redirect policy, min content length, and adaptive rate limits (>=5s host spacing).
- **Circuit breaking:** `service.NewArticleFetcherServiceWithFactory` instantiates `mercari/go-circuitbreaker` wrappers to fail fast on flaky hosts.
- **Tag label graph:** The deduped articles produced here are fed into tag-generator to rebuild the `tag_label_graph` priors that recap-worker consumes, so run this service before `docker compose --profile recap` and make sure the tag-generator batch endpoints stay healthy.

## Testing & Tooling
- `go test ./...` across services, repositories, and handlers. Use `service/testutil` for fixtures and fake HTTP clients.
- Repository suites can run against pgxmock or ephemeral Postgres; ensure `PGX_LOG_LEVEL=error` to keep logs quiet in CI.
- When modifying HTTP client logic, add table-driven tests verifying proxy, UA rotation, and timeout settings.

## Operational Runbook
1. `make up` to launch dependencies (`db`, `news-creator`). Service auto-starts jobs.
2. Monitor summarizer logs for `news_creator_call` entries; each includes latency + token usage.
3. Trigger summarize API manually: `curl -X POST :9200/api/v1/summarize -d '{"article_id":"test","content":"..."}'`.
4. To re-enable feed processing, uncomment `jobHandler.StartFeedProcessingJob(ctx)` in `main.go`, ensure compliance approval, then redeploy.

## Observability
- Logs enriched with `operation`, `feed_url`, `article_id`, and `batch_size`.
- Health metrics collector (`service.NewHealthMetricsCollector`) reports dependency checks; wire to Prometheus when ready.
- Consider enabling metrics HTTP server (`cfg.Metrics.Enabled`) to export job durations and circuit breaker states.

## LLM Notes
- When adding new jobs, clarify if they belong under `handler/job_handler.go` (orchestration) or `service/*` (business logic). LLMs should respect Clean Architecture boundaries.
- Provide explicit config keys (e.g., `HTTP_USER_AGENT_ROTATION`, `RATE_LIMIT_DOMAIN_INTERVALS`) so generated code plumbs env vars correctly.
