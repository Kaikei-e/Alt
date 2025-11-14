# Auth Token Manager

_Last reviewed: November 10, 2025_

**Location:** `auth-token-manager`

## Role
- Deno 2.x CLI microservice that refreshes and validates Inoreader OAuth2 tokens, persisting results to Kubernetes Secrets consumed by ingestion services.
- Commands (`authorize`, `refresh`, `health`, `validate`, `monitor`, `help`) cover onboarding, rotation, and observability scenarios.

## Service Snapshot
| Command | Purpose | Notes |
| --- | --- | --- |
| `authorize` | One-time OAuth bootstrap. | Runs browserless flow; stores initial tokens in the configured secret. |
| `refresh` | Refresh access/refresh tokens. | Uses `InoreaderTokenManager.refreshAccessToken()`; writes JSON `{access_token, refresh_token, expires_at}`. |
| `health` (default) | Validates configuration + dependencies. | Safe for CronJobs; exits non-zero if any check fails. |
| `monitor` | Long-running loop that refreshes before expiry and logs horizon. | Use when co-locating with `pre-processor-sidecar`. |

## Code Status
- `main.ts` pulls runtime config (`src/utils/config.ts`), initializes `InoreaderTokenManager` (`src/auth/oauth.ts`) with retry/network policies, then dispatches to per-command runners.
- `runTokenRefresh()` flow: load config → instantiate token manager → `refreshAccessToken()` → instantiate `K8sSecretManager` (`src/k8s/secret-manager-simple.ts`) → `updateTokenSecret()` → structured log summarizing expiry.
- `runHealthCheck()` collects Boolean checks (config validation, env readiness, Kubernetes reachability, refresh token presence, expiry horizon). Status is derived as `healthy`, `degraded`, or `unhealthy` depending on passing checks.
- Logging via `src/utils/logger.ts` automatically redacts sensitive values; avoid `console.log`.

## Integrations & Data
- **Env requirements:** `INOREADER_CLIENT_ID`, `INOREADER_CLIENT_SECRET`, `INOREADER_REFRESH_TOKEN` (if pre-seeded), plus Kubernetes metadata (`KUBERNETES_NAMESPACE`, `INOREADER_SECRET_NAME`).
- **Kubernetes:** Communicates with in-cluster API server through default service account; ensure RBAC grants `get/update` on the referenced secret. Secret payload is JSON—do not alter key names or types.
- **Pipeline signal:** The refreshed Inoreader tokens feed the pre-processor and recap ingestion loops, so keep the service running before you trigger the `recap` Compose profile or high-frequency summarization jobs.
- **Network:** HTTP calls to `https://www.inoreader.com/oauth2/token` (configurable via `configOptions.network.base_url`). Retries/backoff configured in `configOptions.retry`.

## Testing & Tooling
- `deno test`: suites under `tests/` stub `globalThis.fetch` via `@std/testing/mock` to imitate Inoreader responses and Kubernetes API interactions.
- `deno fmt`, `deno lint`: enforced by `deno.json`.
- When adding commands, follow the TDD loop—write failing tests in `tests/<area>` that stub fetch + secret manager before implementing CLI changes.

## Operational Runbook
1. **Dry run:** `deno task tm:health` (alias for `deno run main.ts health`) confirms env + secret connectivity.
2. **Rotate tokens manually:** `deno run main.ts refresh`. Watch logs for `Token refresh completed successfully`.
3. **Monitor mode:** `deno run --allow-net --allow-env main.ts monitor` inside a sidecar; the loop logs `time_until_expiry_hours`.
4. **Secrets validation:** `kubectl get secret <name> -n <ns> -o jsonpath='{.data.refresh_token}' | base64 -d` to confirm updates (avoid printing whole secret in shared terminals).

## Failure Modes
- **Invalid config:** `config.validateConfig()` returns false → health command exits 1. Fix missing envs.
- **Kubernetes connectivity:** `K8sSecretManager.getTokenSecret()` throws; check service account RBAC or in-cluster DNS.
- **Token near expiry (<1h):** Health status = degraded; run `refresh` or ensure monitor loop executes more frequently.

## LLM Tips
- When scripting new commands, mention required permissions (`--allow-net --allow-env --allow-read=/var/run/secrets/...`) so generated code compiles under Deno.
- Secret schemas are JSON; instruct models to keep `access_token`, `refresh_token`, and `expires_at` fields intact to remain compatible with downstream readers.
