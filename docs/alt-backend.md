# Alt Backend

_Last reviewed: November 10, 2025_

**Location:** `alt-backend/app`

## Role
- Primary HTTP API for feeds, articles, recaps, SSE streams, and media helpers.
- Implements five-layer Clean Architecture (REST → Usecase → Port → Gateway → Driver) on Go 1.24 + Echo.

## Service Snapshot
| Item | Details |
| --- | --- |
| Public endpoints | `/v1/feeds/*`, `/v1/articles/*`, `/v1/recap/*`, `/v1/sse/*`, `/v1/health`, `/security/csp-report` |
| Background jobs | Hourly recap refresh (`job/hourly_job.go`), summarization fan-out, search-index sync triggers |
| Persistence | Postgres (via `driver/alt_db` + `domain/*` models) |
| Messaging | SSE fan-out + optional recap webhooks (feature flagged) |

### Interfaces
- **Inbound:** Echo router defined in `rest/routes.go`, layered with ten ordered middlewares (RequestID → Recover → Secure headers → CORS → DOS guard → Timeout → Validation → Logging → Compression) plus group-specific middleware for `/v1`.
- **Outbound:** Drivers for Postgres, Meilisearch (`driver/search_indexer`), HTTP scrapers (`utils/secure_http_client.go`), and HTML sanitizers (Bluemonday via `utils/html_parser`).

## Code Status
- Usecase directories (`usecase/fetch_feed_usecase`, `usecase/recap_usecase`, etc.) expose constructor functions that accept interfaces from `port`, keeping tests isolated and enabling gomock generation.
- Gateways map domain entities to driver DTOs; for example, feed registration flows pass through `gateway/register_feed_gateway` before reaching `driver/alt_db`.
- SSE plumbing is centralized in `rest/fetch_article_routes.go` and `usecase/fetch_articles_usecase`, sharing rate-limit aware clients so per-tenant fan-out cannot starve the global worker pool.
- Background orchestration is handled by `job/hourly_job.go` plus `usecase/recap_articles_usecase`; both reuse the dependency container in `di/container.go`.

## Integrations & Data
- **Postgres:** `driver/alt_db` wraps pgx/v5 with prepared statements and connection pooling; domain invariants (e.g., `feed_reading_status.go`) protect against duplicate entries.
- **Search index:** After mutating articles, gateways notify `driver/search_indexer`, which forwards normalized payloads to the Meilisearch service through gRPC-like HTTP calls.
- **Streaming:** `/v1/sse/*` routes exclude gzip/timeouts to keep connections warm; when adding new event types, ensure middleware skippers continue to cover them.
- **Defense-in-depth:** `config.RateLimit.DOSProtection` white-lists `/v1/health`, `/v1/sse/*`, `/security/csp-report` while enforcing >=5s spacing for other paths.

## Configuration & Environment
- `config/config.go` validates server timeouts, rate limits, and downstream hosts; malformed durations or missing envs fail fast during boot.
- `di/container.go` wires repositories, usecases, middleware helpers, and job schedulers; use this container when adding new handlers to keep lifecycle hooks centralized.
- Critical env vars: `ALT_DB_DSN`, `SEARCH_INDEXER_HOST`, `RATE_LIMIT_DOS_THRESHOLD`, `SSE_KEEPALIVE_INTERVAL`, `HTTP_READ_TIMEOUT`, `HTTP_WRITE_TIMEOUT`.

## Testing & Tooling
- `go test ./...` (table-driven) with gomock fakes in `mocks/` and `pgxmock` for verifying SQL interactions.
- Handler suites rely on `httptest.NewServer` to exercise middleware order; SSE tests stub contexts with fake clocks to cover timeout skippers.
- Run `make generate-mocks` after interface changes; run `golangci-lint` (optional) before raising PRs.

## Operational Runbook
1. **Health:** `curl http://localhost:9000/v1/health` and confirm JSON `{status:"ok"}`; if latency >1s, inspect middleware for blocking operations.
2. **Jobs:** Check hourly job logs (`job/hourly_job.go`) for `recap_refresh_completed` events; re-run manually with `go run ./cmd/job_runner`.
3. **Schema drift:** Use `make db-migrate` followed by `go test ./driver/...` to ensure new columns propagate to adapters.
4. **Rollback plan:** Disable recap SSE pushes via feature flag env (`SSE_RECAP_ENABLED=false`) if streaming load spikes.

## Observability & Metrics
- Structured logs via `log/slog`, enriched with fields `route`, `tenant_id`, `request_id`.
- Emit counters for `feeds.register.success`, `articles.fetch.duration_ms`, and rate-limit rejections (`middleware/dos_protection`).
- When onboarding new handlers, register histogram metrics in `utils/logger/metrics.go` to keep dashboards consistent.

## LLM Consumption Tips
- Mention the middleware stack explicitly when prompting to add new routes (middleware order matters).
- Provide required DTO names (`domain/rss_feed`, `usecase/register_feed_usecase`) so generated code hooks into existing layers.
- Clarify whether work runs in REST handler path or background job since dependency injection differs.
