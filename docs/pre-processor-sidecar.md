# Pre-processor Sidecar

_Last reviewed: December 23, 2025_

**Location:** `pre-processor-sidecar/app`

## Purpose
- Orchestrates Inoreader ingestion for the pre-processor by pairing a scheduler loop with a resilient OAuth2 token system (`cmd/main.go`, `service/simple_token_service.go`).
- Bridges Kubernetes Secrets (or alternative token repositories) with `auth-token-manager` so Inoreader calls stay within quota while rotating tokens safely (`service/token_management_service.go`, `repository`).
- Provides HTTP hooks (`/admin/oauth2/*`, `/admin/trigger/*`) for observability, manual syncs, and secret-level token updates without restarting the CronJob (`handler/admin_api_handler.go`, `handler/schedule_handler.go`).

## Execution Modes & CLI
- `--health-check`: runs `performHealthCheckWithOutput` (performs config validation, DB connection, OAuth2 endpoints) and exits early (`cmd/main.go` lines 70‑110).
- `--oauth2-init`: waits 10 seconds for Linkerd, pings Postgres, and bootstraps the first tokens before exiting (`performOAuth2Initialization`).
- `--schedule-mode`: enables the dual-scheduler pipeline (30‑minute article fetch + 12‑hour subscription sync by default) plus token monitoring, admin API, and rotation processing (`runScheduleMode`).
- Default CronJob mode still executes `runScheduleMode` once so Kubernetes handles concurrency, but `--schedule-mode` keeps the loops running for debugging or local `make up` testing.

## Architecture Overview
`runScheduleMode` wires together configuration, token storage, the scheduler, rotation manager, and HTTP surface. The diagram below summarizes the live data paths.

```mermaid
flowchart LR
    CLI[CLI flags<br>(--health-check, --oauth2-init, --schedule-mode)]
    CLI --> Health[`performHealthCheckWithOutput`]
    CLI --> OAuthInit[`performOAuth2Initialization`]
    CLI --> ScheduleMode[`runScheduleMode` (CronJob/Debug)]

    ScheduleMode --> Config[config.LoadConfig()]
    Config --> TokenRepo[(Token repo: kubernetes_secret | file | env_var)]
    TokenRepo --> SimpleTokenService
    Secrets[OAuth2 Secret Service<br>(`OAUTH2_TOKEN_SECRET_NAME`)] --> SecretWatch[Secret watch<br>(`ENABLE_SECRET_WATCH`)]
    SecretWatch --> SimpleTokenService
    TokenRepo --> SecretWatch
    SimpleTokenService --> TokenManagementService
    TokenManagementService --> TokenRotationManager
    TokenManagementService --> OAuth2Client[OAuth2 client<br>(`driver.NewOAuth2Client`)]
    OAuth2Client --> InoreaderAPI[Inoreader token & reader APIs]
    SimpleTokenService --> StatusTicker[Status ticker ⭢ logging `SimpleServiceStatus`]

    InoreaderAPI --> InoreaderService
    RateLimitManager --> InoreaderService
    InoreaderService --> ArticleFetchService
    InoreaderService --> SubscriptionSyncService
    ArticleFetchService --> SubscriptionRotator
    SubscriptionRotator --> SchedulerNode[Scheduler<br>(`service/scheduler`)]
    SchedulerNode --> SyncStateRepo[(SyncState repo)]

    ScheduleMode --> DB[(Postgres)]
    DB --> ArticleRepo[(Article repo)]
    DB --> SubRepo[(Subscription repo)]
    DB --> SyncStateRepo

    ScheduleMode --> AdminAPI[Admin API server :8080]
    AdminAPI --> ScheduleHandler[ScheduleHandler]
    ScheduleHandler --> ArticleFetchService
    ScheduleHandler --> SubscriptionSyncService
    Security[Authenticator + Memory rate limiter + OWASP validator] --> AdminAPI
    AdminAPI --> ManualTriggers[Manual fetch/sync endpoints]

    ArticleFetchService --> RateLimitManager
```

## Configuration & Secrets
- `config.LoadConfig()` layers service metadata, DB, Inoreader endpoints, proxy settings, rate limits, OAuth2 details, HTTP client tuning, retry/circuit breaker parameters, monitoring flags, and rotation/content guards (`config/config.go`).
- Token storage respects `TOKEN_STORAGE_TYPE` (defaults to `kubernetes_secret`) plus overrides for `TOKEN_STORAGE_PATH`. Kubernetes mode also uses `OAUTH2_TOKEN_SECRET_NAME`.
- `ENABLE_SECRET_WATCH` causes `SimpleTokenService` to watch both the Kubernetes secret and configured token repository so tokens are reloaded without API calls when `auth-token-manager` rotates them.
- Rotation and batch behavior are controlled via `ROTATION_INTERVAL_MINUTES`, `MAX_DAILY_ROTATIONS`, `BATCH_SIZE`, and the rotation sub-config (`config.Rotation`), while content processing toggles (`CONTENT_EXTRACTION_ENABLED`, `CONTENT_TRUNCATION_ENABLED`, etc.) live in `config.Content`.
- Proxy/environment overrides (`HTTPS_PROXY`, `NO_PROXY`) and client-sensitive env vars (`INOREADER_CLIENT_ID`, `INOREADER_CLIENT_SECRET`, optional `INOREADER_REFRESH_TOKEN`, `PRE_PROCESSOR_SIDECAR_DB_PASSWORD`) are loaded either directly from secrets or from files (`getSecretOrEnv` helper).

## Scheduler & Rotation
- The legacy `ScheduleHandler` still powers admin-triggered flows and rotation-aware batch processing. It starts `SubscriptionSyncService` and `ArticleFetchService`, enables rotation mode (optionally with random start), and uses two `RateLimitAwareScheduler` instances to throttle the 12‑hour subscription sync and the dynamic article-fetch interval (`handler/schedule_handler.go`).
- A new `service/scheduler` loop targets a 16‑minute fetch interval plus a 24‑hour refresh stream, pulling the oldest `SyncState`, running `ArticleFetchService.FetchArticles`, and updating continuation tokens; this ensures ~90 requests/day without manual intervention.
- `SubscriptionRotator` enforces `MAX_DAILY_ROTATIONS`, timezone-aware day resets, shuffling, and interval enforcement. The rotation stats (`RotationStats`) feed both logging and the `ScheduleHandler` batch processor so the service knows when the API budget is consumed.
- `ArticleFetchService` delegates UUID resolution to `usecase.ArticleUUIDResolutionUseCase` and writes articles via `ArticleRepository.CreateBatch`, then updates `SyncState`. Batch processing includes continuation tokens, rotation-enabled single-subscription processing, and helpers for batch jobs and timezone info.
- `SubscriptionSyncService.SyncSubscriptionsNew` now saves subscriptions (`subscriptionRepo.SaveSubscriptions`), ensures sync state rows exist, refreshes the in-memory cache used for UUID lookups, and keeps stats (`SubscriptionSyncStats`) for observability and metrics.
- A `RateLimitManager` monitors both Zone1/Zone2 budgets, applies a safety buffer, triggers alerts at 50/75/90%, and feeds `ArticleFetchHandler`/`ScheduleHandler` decisions (`service/rate_limit_manager.go`).

## Token Lifecycle & Recovery
- `SimpleTokenService` initializes `InMemoryTokenManager`, `RecoveryManager`, and optional `OAuth2SecretService`. It prefers the configured repository, falls back to Kubernetes secret or env vars, enables secret watching (`onSecretUpdate`/`ReloadFromSecret`), and logs all refresh/health events with structured metadata (`service/simple_token_service.go`).
- The `TokenManagementService` wraps that system for the scheduler: it loads tokens, applies a 30‑minute `refreshBuffer`, validates only when <2 hours from expiry, retries refresh up to three times (longer backs off on rate limits), and deduplicates refreshes using `golang.org/x/sync/singleflight`. It also tracks metrics such as refresh counts, single-flight hits, and rotation detection (`service/token_management_service.go`).
- `TokenRotationManager` keeps an eye on rotation health by proactively refreshing every 10 minutes, running a more extensive health check every 30 minutes, flagging tokens that expire within 30 minutes, and exposing `RotationHealthStatus` for dashboards (`service/token_rotation_manager.go`).
- Admin traffic hits `ScheduleHandler` through `SimpleTokenServiceAdapter`, meaning manual token status, refresh, and job triggers reuse the same safeguards as the scheduler.

## Admin API & Security Controls
- The Admin API runs on `:8080` with `/admin/oauth2/refresh-token`, `/admin/oauth2/token-status`, `/admin/trigger/article-fetch`, and `/admin/trigger/subscription-sync` handlers (`handler/admin_api_handler.go`, `cmd/main.go`).
- Access requires Kubernetes service account tokens validated by `security.KubernetesAuthenticator` (checks JWT claims, CA-based signing, and known admin subjects/namespaces) and rate limiting via `security.MemoryRateLimiter`.
- Inputs, especially refresh tokens, pass through `security.OWASPInputValidator`, which enforces regex patterns, controls SQL/XSS/path traversal threats, strips control characters, and escapes HTML entities before token updates are accepted.
- `SimpleAdminAPIMetricsCollector` logs request durations, rate limit hits, and auth failures so the admin surface is observable without a full metrics stack.

## Observability & Monitoring
- Structured JSON logs include fields such as `component`, `interval`, `subscription_sync_interval`, `article_fetch_interval`, `token_status`, error reasons, and rotation stats; the status ticker logs `SimpleTokenService.GetServiceStatus()` every 30 minutes (`cmd/main.go`).
- `utils.Monitor` (used by `InoreaderService`) instruments API requests, circuit breaker transitions, article processing, and token refreshes; `SimpleTokenService` reports metrics through `SimpleServiceStatus`.
- `ScheduleHandler` publishes `JobResult` callbacks on each run, injecting timing, success/failure, rotation stats, and any errors (scheduler, API calls, rotation exhaustion).
- Manual triggers and scheduler loops log their decisions (e.g., when a batch is skipped because `RemainingToday == 0`) so the team can diagnose quota burnout without extra tooling.

## Operational Runbook
1. Run `go run ./cmd --health-check` (or `cmd/main.go --health-check`) after deployments to verify config, DB connectivity, and OAuth2 readiness.
2. Use `cmd/main.go --oauth2-init` once per environment to bootstrap tokens, waiting ~10 seconds for Linkerd and ensuring Postgres is reachable.
3. Default CronJob mode invokes `runScheduleMode` once; add `--schedule-mode` when debugging to keep the scheduler, rotation manager, and admin API running in the foreground.
4. Manual triggers are available via `POST http://<pod>:8080/admin/trigger/article-fetch` and `/subscription-sync` (JSON responses include timestamps).
5. Rotate `auth-token-manager` secrets by writing to the Kubernetes secret referenced by `OAUTH2_TOKEN_SECRET_NAME`; `ENABLE_SECRET_WATCH=true` instructs `SimpleTokenService` to reload immediately (`onSecretUpdate` avoids calling Inoreader APIs during rotation).
6. Check logs for `TOKEN_REFRESH`, `SECRET_UPDATED`, and `rate limit hit` warnings; the latter suggests bumping `MAX_DAILY_ROTATIONS` or lengthening `CheckInterval`.

## Testing & Tooling
- `GO_TEST`: `go test ./...` exercises unit tests, mocks (`mocks/`), and logic under `service/`, `handler/`, `repository/`, `security/`.
- Integration coverage lives in `test/` (e.g., `token_rotation_integration_test.go`, `configuration_integration_test.go`, `monitoring_integration_test.go`, `integration_test.go`) to exercise DB-backed flows.
- When dependencies change, regenerate mocks with `make generate-mocks`; tests rely on clean `mockgen` artifacts under `mocks/`.

## Security & Compliance
- Admin access is gated by `security.KubernetesAuthenticator`, `MemoryRateLimiter`, and OWASP-aware sanitization; `MemoryRateLimiter` enforces 5 requests/hour by default.
- Secrets and tokens stay out of logs (`security/sanitizer.go` and `utils.Sanitizer` ensure sensitive fields are removed).
- Circuit breaker (`utils.CircuitBreaker`) around `InoreaderService` stops hammering the API during outages, while `RateLimitManager` enforces Zone1/Zone2 budgets with a configurable safety buffer.

## LLM Notes
- Mention `SimpleTokenService`, `TokenManagementService`, and `TokenRotationManager` when summarizing token logic; include env names `ENABLE_SECRET_WATCH`, `OAUTH2_TOKEN_SECRET_NAME`, `MAX_DAILY_ROTATIONS`, `ROTATION_INTERVAL_MINUTES`, `BATCH_SIZE`, `INOREADER_CLIENT_ID`, `INOREADER_CLIENT_SECRET`, `INOREADER_REFRESH_TOKEN`, `HTTPS_PROXY`, and `NO_PROXY` so prompts resolve to the right switches.
- Highlight that `ScheduleHandler` still feeds Admin API triggers but the new `service/scheduler` loop owns the steady 90-request/day cadence; rotation stats live in `SubscriptionRotator`.
