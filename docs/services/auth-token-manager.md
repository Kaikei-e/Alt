# Auth Token Manager

_Last reviewed: September 5, 2026_

**Location:** `auth-token-manager`

## Role
- Deno 2.x CLI that keeps Inoreader OAuth2 credentials refreshed, validates configuration, and persists tokens to a `.env` secret file (`TOKEN_STORAGE_PATH`).
- Also serves the tokens over HTTP: `GET /api/token` (protected by `X-Internal-Auth: <INTERNAL_AUTH_TOKEN>`) returns `access_token`/`expires_at`/etc. as JSON (never `refresh_token`). `pre-processor-sidecar`'s `RemoteTokenRepository` is the actual consumer today — it calls this API (`AUTH_TOKEN_MANAGER_URL`, default `http://auth-token-manager:9201`) rather than reading the `.env` file directly.
- Runs either command-by-command (authorize, refresh, monitor, etc.) or as the default `daemon` process, which exposes the callback + token-API HTTP server and a periodic token monitor/refresh loop.
- Structured logging sanitizes every call site so tokens or secrets never appear in plaintext outputs.

## CLI Commands
| Command | Summary | Key details |
| --- | --- | --- |
| `daemon` | Default when no argument is provided. | Starts the persistent OAuth callback listener, runs `checkAndRefreshToken` immediately, and repeats every five minutes. Logs when tokens are still valid and triggers refresh if `< 2h` remain. |
| `authorize` | One-time interactive bootstrap. | Prints the authorization URL, starts a short-lived `Deno.serve` callback, validates the `state` parameter, exchanges the code, and writes `OAUTH2_*` values to the configured secret file. |
| `refresh` | Force refresh via `RefreshTokenUsecase`. | Runs the connectivity check, calls the refresh endpoint, and persists the response. Errors throw if the refresh token is missing/invalid. |
| `health` | Validates config, env, storage, and expiry horizon. | Tracks five sub-checks (config, env vars, storage access, refresh token, expiry > 1h) and exits with `1` when the service becomes `unhealthy`. |
| `validate` | Configuration gate for CI/automation. | Loads the config once, prints diagnostics (including a masked client ID suffix), and returns success/failure without contacting Inoreader. |
| `monitor` | Horizon alerting for token freshness. | Computes `time_until_expiry`, `time_since_update`, and a set of human-friendly alerts with thresholds at 5m/30m/1h/2h/6h plus staleness checks (>12h/24h). Reports `alert_level` (info/warning/critical) and exits `0/1/2`. |
| `help` | Prints usage guidance. | Documents the required `--allow-*` permissions and available commands. |

## Configuration

### Required credentials & callback
- `INOREADER_CLIENT_ID` & `INOREADER_CLIENT_SECRET`: must exist, be ≥5 characters, and cannot be placeholders such as `demo-client-id`/`placeholder`.
- `INOREADER_REDIRECT_URI` (default `http://localhost:8080/callback`): dictates where both the one-off `authorize` flow and the daemon’s HTTP listener expect the OAuth callback, and which port the daemon's HTTP server listens on.
- `INTERNAL_AUTH_TOKEN` (no default; `_FILE` suffix supported): shared bearer that gates the daemon's `/auth` (re-authorization) and `/api/token` (token retrieval) endpoints. Without it, both fail closed (401/503) rather than serving unauthenticated.

### Retry & network knobs
- `RETRY_MAX_ATTEMPTS` (default `3`), `RETRY_BASE_DELAY` (`1000ms`), `RETRY_MAX_DELAY` (`30000ms`), `RETRY_BACKOFF_FACTOR` (`2`): control `retryWithBackoff()`'s exponential backoff (`src/infra/retry.ts`). It wraps only the idempotent read-existing-token and persist-token steps of a refresh, never the Inoreader token-exchange call itself (that call rotates `refresh_token` server-side, so retrying it risks resending an already-consumed token). `retryWithBackoff` fails fast on `PermanentError`, but in the refresh path that guard is currently inert — the only source of `PermanentError` (`InoreaderTokenClient.refreshToken`, `invalid_grant` / `invalid_client` / `unsupported_grant_type` / `invalid_request`) is deliberately outside every retry boundary, so those errors surface directly to the caller.
- `HTTP_TIMEOUT` (`30000ms`) bounds every HTTP call made through `FetchHttpClient`, including the `checkNetworkConnectivity()` HEAD request to `https://www.inoreader.com`. `CONNECTIVITY_CHECK` (`true` unless set to `false`) gates whether that HEAD request runs before a refresh. `CONNECTIVITY_TIMEOUT` is parsed into config but not currently applied anywhere — the connectivity probe shares `HTTP_TIMEOUT`, not a dedicated timeout.

### Proxy & fallback support
- Deno's `fetch` honors `HTTP_PROXY`/`HTTPS_PROXY` automatically, so a configured proxy needs no extra handling for the normal case. When `NETWORK_FALLBACK_TO_DIRECT=true` **and** a proxy is configured, `FetchHttpClient` builds a dedicated `Deno.createHttpClient({})` (no proxy) and sends the request through it instead — a per-request bypass decided up front, not a "try proxy, catch failure, retry direct" sequence. (Deno reads `HTTP_PROXY`/`HTTPS_PROXY` once at process start for its default client, so toggling the env vars mid-request would have no effect and would race concurrent requests — which is why this went through a dedicated client instead.)

### Logging toggles
- `LOG_LEVEL` (default `INFO`), `LOG_INCLUDE_TIMESTAMP`, `LOG_INCLUDE_STACK_TRACE`, plus `NODE_ENV`/`DENO_ENV` (used by `StructuredLogger.info/debug`) adjust verbosity. Debug statements only emit when `NODE_ENV` equals `development`.

### Storage path
- `TOKEN_STORAGE_PATH` (default `/app/secrets/oauth2_token.env`) is where `EnvFileSecretManager` persists tokens. Adjust this path when the file must live on shared volumes for sidecars or CronJobs.

## DI Container Pattern

`main.ts` serves as a manual DI container: it constructs the gateway layer (`EnvFileSecretManager`, `FetchHttpClient`, `InoreaderTokenClient`), wires them into usecases (`RefreshTokenUsecase`, `HealthCheckUsecase`, `MonitorTokenUsecase`, `AuthorizeUsecase`), then passes usecases to handler layer (`CliHandler`, `OAuthServer`, `DaemonLoop`). No framework is used—dependency resolution is explicit and top-down.

## Secret storage & consumption
- `EnvFileSecretManager` preserves every non-token line in the destination file, removes stale `OAUTH2_*` entries, and rewrites seven canonical keys: `OAUTH2_ACCESS_TOKEN`, `OAUTH2_REFRESH_TOKEN`, `OAUTH2_TOKEN_TYPE`, `OAUTH2_SCOPE`, `OAUTH2_EXPIRES_AT`, `OAUTH2_EXPIRES_IN`, `OAUTH2_UPDATED_AT`.
- The write path is tmpfile + `chmod(0o640)` + rename; it does **not** call `fsync` (`Deno.fsync`) before the rename. This falls short of the "tmpfile + rename + fsync" remediation PM-2026-043 prescribes below — the fsync step is not currently implemented.
- `getTokenSecret()` parses the same keys and defaults `scope` to `read` (`INOREADER_OAUTH_SCOPE`) if absent; `OAUTH2_UPDATED_AT` is read back as written, not backfilled. `checkSecretExists()` simply checks for the file.
- The file is still written on every refresh, but `pre-processor-sidecar` no longer reads it: its `RemoteTokenRepository` calls this service's `GET /api/token` HTTP endpoint instead (see Role above). Keep `TOKEN_STORAGE_PATH` set regardless — the daemon and CLI commands operate on it directly, and it remains the durable record other tooling can inspect.

## Refresh strategy & daemon
- `RefreshTokenUsecase.execute()` (`src/usecase/refresh_token.ts`) is the refresh core: it runs `checkNetworkConnectivity()` when enabled, reads the existing token (retried via `retryWithBackoff`), short-circuits if it is still valid, and otherwise calls `InoreaderTokenClient.refreshToken()` — a POST to `https://www.inoreader.com/oauth2/token` with `grant_type=refresh_token`, `User-Agent: Auth-Token-Manager/2.0.0`, and the configured client credentials. That call itself is deliberately outside the retry loop (see Retry & network knobs above); only the subsequent persistence step is retried.
- `FetchHttpClient` (`src/gateway/fetch_http_client.ts`) is the shared HTTP layer: it enforces `HTTP_TIMEOUT` via `AbortController` and applies the proxy-bypass decision described above. It does not itself retry or fail over.
- Persistence goes through `EnvFileSecretManager.updateTokenSecret()`. If persistence fails after all retries, the newly-rotated `refresh_token` only exists in memory and is lost — this is logged distinctly as a case requiring manual re-authorization, since Inoreader has already invalidated the old one server-side.
- `daemon` mode (`DaemonLoop.start()`) starts `OAuthServer.start()` (callback + `/api/token` + `/healthz` HTTP listener) in the background, immediately invokes `checkAndRefreshToken()`, and then repeats it every five minutes. `checkAndRefreshToken()` warns when no refresh token exists, refreshes when the stored `expires_at` is missing/invalid or within two hours, and otherwise logs that the token is still healthy.

## Authorization, monitoring & health
- `authorize` spins up a one-shot `OAuthServer.startOneShot()` listener on the configured callback port, redirects the user to the Inoreader consent page with a randomly generated `state`, validates the returned `state` (5-minute TTL), exchanges the `code`, and updates the token file before exiting.
- The daemon's `OAuthServer` also serves `GET /healthz` (plain `{"status":"ok"}`, unauthenticated), `GET|POST /` and `GET|POST /auth`, and `GET /api/token`. `/auth` is protected by `INTERNAL_AUTH_TOKEN` and the token is never accepted in the URL (CWE-598): an unauthenticated browser GET answers 401 with a small HTML form, and the token travels in the POST body before the 302 to Inoreader's consent page. Scripts can instead send `Authorization: Bearer <INTERNAL_AUTH_TOKEN>` on the GET and follow the `Location` header. `/api/token` uses the same token but presented as `X-Internal-Auth` (matching `pre-processor-sidecar`'s `RemoteTokenRepository`), and returns every token field except `refresh_token`. Both endpoints fail closed (503) when `INTERNAL_AUTH_TOKEN` is not configured. Refreshed tokens are stored whenever the callback path hits `?code=...`.
- `monitor` inspects the token file, computes `time_until_expiry` and `time_since_update`, collects alerts for expiration windows (𝑡 < 5m or 30m → `critical`, 𝑡 < 1h or 2h → `warning`, < 6h → informational) and staleness (>12h/<24h). Missing or short refresh tokens immediately raise `critical` alerts. The command logs `token_status`, `system_status.configuration_valid`, and a `needs_immediate_refresh` flag plus exit codes (`2` for critical, `1` for warning, `0` for info) so orchestrators can react.
- `health` runs five checks (config validity, credentials, storage access, refresh token presence, expiry > 1h). It logs each result, summarizes the `status` (`healthy`, `degraded`, `unhealthy`), and exits `1` if too many checks fail. `validate` simply loads the configuration and echoes success/failure, making it safe for CI.

## Observability & logging
- `StructuredLogger` wraps `@std/log` with a JSON formatter that attaches `component`, `service`, and `version`, and delegates argument sanitization to `DataSanitizer`. The sanitizer redacts OAuth tokens matched by regexes, strips every `SENSITIVE_FIELDS` entry (e.g., `access_token`, `password`, `client_secret`), and preserves harmless data.
- Signal handlers (`SIGINT`, `SIGTERM`) and `globalThis` listeners for `error`/`unhandledrejection` emit sanitized logs before cleanly exiting, which keeps the monitoring loop resilient.
- `src/infra/otel.ts` also exports logs over OTLP/HTTP when `OTEL_ENABLED` (default `true`). `OTEL_SERVICE_NAME` (default `auth-token-manager`), `OTEL_EXPORTER_OTLP_ENDPOINT` (default `http://localhost:4318`), `SERVICE_VERSION`, and `DEPLOYMENT_ENV` configure the exporter; `shutdownOTel()` flushes on process exit/error.
- `tests/security/logger_security_test.ts` ensures that every sensitive string is replaced with `[REDACTED]` in the console output and that non-sensitive fields survive the filtering.

## Runbook
1. Set `INOREADER_CLIENT_ID`, `INOREADER_CLIENT_SECRET`, `INOREADER_REDIRECT_URI` (or accept the default callback), `TOKEN_STORAGE_PATH`, and any optional retry/network overrides before you start anything.
2. Start the always-on service with `deno run --allow-env --allow-net --allow-read --allow-write main.ts daemon`. The daemon logs when the OAuth server is listening, when tokens are refreshed, and when it skips refreshes due to a healthy token.
3. If no tokens exist yet, run `deno run --allow-env --allow-net main.ts authorize`, open the printed URL in a browser, and follow the flow. The command stores `OAUTH2_*` entries and exits as soon as the callback completes.
4. For manual refreshes (e.g., after rotating credentials), run `deno run --allow-env --allow-net main.ts refresh`. This command reuses the same retry/backoff logic the daemon uses.
5. Schedule `deno run --allow-env --allow-net main.ts monitor` (exit `0/1/2`) in your Cron jobs or health dashboards to detect horizon issues without hitting Inoreader.
6. Use `deno run --allow-env --allow-net main.ts health` or `deno run --allow-env --allow-net main.ts validate` before deployments to confirm the CLI can read secrets and that the config is sane.
7. `pre-processor-sidecar` reaches this service over HTTP (`AUTH_TOKEN_MANAGER_URL`, default `http://auth-token-manager:9201`) via `GET /api/token`, authenticated with the shared `INTERNAL_AUTH_TOKEN` sent as `X-Internal-Auth`. Keep that token identical on both sides — the API fails closed (401/503) otherwise.

## Known failure patterns

- Inoreader ingestion silently dead for 65h → disk-full non-atomic write truncated `oauth2_token.env` to 0 bytes; downstream consumers got 404s and their circuit breaker oscillated OPEN↔HALF_OPEN forever while health kept reporting `token_manager_available: true`. Persist the token file via tmpfile + rename + fsync. → PM-2026-043
- Health check green during total functional outage → "the process is up" and "the token is usable" are separate responsibilities; validate file content and freshness, not existence. Consumers now expose a typed sentinel (`ErrTokenUnavailable`) and an `/admin/health` `ingestion_silent` signal. → PM-2026-043
- Alerts and tests cannot branch on failures → opaque string errors from the token path are indistinguishable; use typed sentinel errors so retry/alert policy can discriminate. → PM-2026-043
- Root trigger was host disk exhaustion from a retry storm (148GB of logs) in an unrelated service → low-frequency supplementary paths like token refresh need automatic observability the most; unused features rot. → PM-2026-042, [[runbooks/crystallized-knowledge]] §14

## Testing & tooling
- `deno test` runs `tests/unit/**` (gateway, handler, usecase, infra) plus `tests/security/logger_security_test.ts`, which stubs console APIs to verify that sanitized logs never leak tokens or secrets.
- `deno fmt` and `deno lint` keep the codebase consistent before commits.
