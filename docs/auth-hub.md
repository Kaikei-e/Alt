# Auth Hub

_Last reviewed: November 10, 2025_

**Location:** `auth-hub`

## Role
- Identity-Aware Proxy fronting Nginx `auth_request` calls, translating Kratos sessions into `X-Alt-*` headers.
- Provides `/validate`, `/health`, and future metrics/CSRF endpoints while caching session metadata to reduce Kratos load.

## Service Snapshot
| Component | Purpose |
| --- | --- |
| `handler/validate_handler.go` | Main auth flow (cookie → cache → Kratos WhoAmI → identity headers). |
| `cache/session_cache.go` | TTL cache with 1-minute cleanup ticker; default TTL 5 minutes. |
| `client/kratos_client.go` | HTTP client for Kratos `/sessions/whoami`. |
| `config/config.go` | Env parsing for `KRATOS_URL`, `PORT`, `CACHE_TTL`. |
| `handler/health_handler.go` | Lightweight readiness endpoint for load balancers. |

## Code Status
- Validate handler distinguishes auth vs. service failures by inspecting error strings; consider upgrading to typed errors when Kratos client adds structured responses.
- Cache entries store `UserID`, `TenantID`, `Email`, plus expiry; expired entries are lazily evicted and proactively cleaned via ticker.
- `main.go` wires Echo middleware (request logging, panic recovery) and registers future metrics routes so Prometheus scraping can be added without reshaping handlers.
- CSRF handler scaffolding exists but stays disabled until ingress paths require token validation.

## Integrations & Data
- **Kratos connectivity:** defaults to `http://kratos:4433`; ensure NetworkPolicy allows pod-to-pod access. TLS termination is handled upstream (within Kratos).
- **Nginx contract:** `auth_request` must forward `Cookie` headers and consume response headers: `X-Alt-User-Id`, `X-Alt-Tenant-Id`, `X-Alt-User-Email`. Update both auth-hub and Nginx config when introducing new identity headers.
- **Caching:** TTL is configurable via `CACHE_TTL`; align `session_cache.cleanupLoop` interval with TTL to avoid stale entries.

## Testing & Tooling
- `go test ./...` (table-driven). Focus suites:
  - `handler/validate_handler_test.go`: covers happy path, missing cookie, cache hit/miss, Kratos 401, and upstream failures.
  - `cache/session_cache_test.go`: manipulates `cleanup()` directly to skip waiting for ticker.
- Use `mockgen` for `KratosClient` when testing new branches; stub errors to simulate 401 vs 500.

## Operational Runbook
1. **Health probe:** `curl -i http://localhost:8888/health`. Expect `200 OK`.
2. **Cache flush:** restart pod or expose `SessionCache` reset endpoint (todo) when invalid tokens flood the cache.
3. **Kratos outage handling:** monitor retry/backoff at the ingress layer; if Kratos is down, `auth-hub` returns 401 for auth failures and 500 for infrastructure errors.
4. **Header changes:** update `validate_handler.go`, adjust Nginx `proxy_set_header`, then run `go test ./handler`.

## Observability
- Structured logs include `request_id`, `session_id`, and `cache_hit` boolean.
- Future `handler/metrics.go` will export: cache hit ratio, Kratos latency, validate call rate.
- For now, use log-based metrics via Loki/ClickHouse to watch `cache_miss` frequency.
- Recap-worker and alt-backend Recap endpoints rely on the cached headers (`X-Alt-*`) with a 5-minute TTL to hydrate their sinks; keep cache hits high so `/v1/recap/7days` and service-initiated workflows do not trigger repeated Kratos calls.

## LLM Notes
- When asking an LLM to modify auth-hub, specify whether change lives in handler, cache, or config.
- Make clear that cookies are named `ory_kratos_session` and identity headers must remain authoritative to downstream services.
