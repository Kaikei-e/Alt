# Pre-processor Sidecar

_Last reviewed: November 10, 2025_

**Location:** `pre-processor-sidecar/app`

## Role
- Go scheduler that maintains Inoreader ingestion health: token rotation, subscription sync, batch fetch triggers.
- Runs as a Kubernetes CronJob (Forbid concurrency) but supports `--schedule-mode` deployments for continuous processing or debugging.

## Service Snapshot
| Flag / Mode | Behavior |
| --- | --- |
| `--health-check` | Performs dependency checks (config, DB, OAuth2 endpoints) then exits. |
| `--oauth2-init` | Bootstraps tokens, waits for Linkerd (10s sleep), persists secrets, exits. |
| `--schedule-mode` | Dual-schedule loop for subscription sync (12h) + article fetch (30m). |
| default | Runs cron-style execution once, intended for CronJob pods. |

## Code Status
- `cmd/main.go` parses flags, configures slog, loads config (`config.LoadConfig()`), and constructs `SimpleTokenService`.
- Simple Token config pulls from env (`INOREADER_CLIENT_ID/SECRET`, `INOREADER_ACCESS_TOKEN`, `KUBERNETES_NAMESPACE`, secret name) plus config-provided OAuth2 base URLs.
- `service.NewSimpleTokenService` adds secret watching (`EnableSecretWatch`) and `singleflight`-style guards (via `golang.org/x/sync/singleflight`) to avoid duplicate refreshes.
- `runScheduleMode` coordinates timers (30m fetch, 12h subscription) respecting `cfg.RateLimit.DailyLimit`; concurrency is limited via mutexes to honor API quotas.
- Health/OAuth init flows open Postgres connections (via `cfg.Database`) to ensure credentials exist before rotating.

## Integrations & Data
- **Inoreader API:** Base URL from `cfg.OAuth2.BaseURL`; token refresh uses `SimpleTokenService`, while subscription syncing uses HTTP clients with shared rate limits.
- **Kubernetes Secrets:** Namespace autodetected from `/var/run/secrets/.../namespace` fallback to `KUBERNETES_NAMESPACE`. Works in tandem with `auth-token-manager` by watching the same secret for updates.
- **Database:** Some admin flows (OAuth2 init) need DB credentials to validate ingestion state; connection string assembled from config.

## Testing & Tooling
- `go test ./...`; mocks under `mocks/` simulate OAuth2 providers, repositories, schedulers.
- Time manipulation: many services accept `Clock` interfaces (`domain/time`) to deterministically test token expiry and scheduling drift.
- Use `make generate-mocks` when adding new interfaces to maintain gomock parity.

## Operational Runbook
1. **CronJob safety:** Keep `concurrencyPolicy: Forbid` + `startingDeadlineSeconds` in manifests inside `k8s/` directory.
2. **Manual run:** `go run cmd/main.go --health-check` to ensure env + secrets resolve before scheduling.
3. **Continuous monitoring:** Deploy with `--schedule-mode` when debugging rate limits; check logs for `TOKEN_REFRESH` and `ARTICLE_FETCH` events.
4. **Secret sync:** If tokens rotate in `auth-token-manager`, ensure `ENABLE_SECRET_WATCH=true` so the sidecar reloads without restart.

## Observability
- Logs (JSON) include `component`, `service`, `subscription_sync_interval`, `article_fetch_interval`.
- Consider hooking SimpleAdminAPIMetricsCollector outputs into Prometheus once the admin API solidifies.
- Watch for `Admin API rate limit hit` messages; they indicate manual overrides need throttling.

## LLM Notes
- When editing scheduling logic, specify which interval to touch (subscription vs. article) and mention `SimpleTokenService` interface expectations.
- Provide explicit env or config keys (e.g., `MAX_DAILY_ROTATIONS`, `ENABLE_SECRET_WATCH`) so generated code wires settings correctly.
