# Auth Token Manager

_Last reviewed: February 28, 2026_

**Location:** `auth-token-manager`

## Role
- Deno 2.x CLI that keeps Inoreader OAuth2 credentials refreshed, validates configuration, and writes a shared `.env` secret that downstream services consume (`pre-processor-sidecar`, `pre-processor`, etc.).
- Runs either command-by-command (authorize, refresh, monitor, etc.) or as the default `daemon` process, which exposes a callback server and periodic token monitor/refresh loop.
- Structured logging sanitizes every call site so tokens or secrets never appear in plaintext outputs.

## CLI Commands
| Command | Summary | Key details |
| --- | --- | --- |
| `daemon` | Default when no argument is provided. | Starts the persistent OAuth callback listener, runs `checkAndRefreshToken` immediately, and repeats every five minutes. Logs when tokens are still valid and triggers refresh if `< 2h` remain. |
| `authorize` | One-time interactive bootstrap. | Prints the authorization URL, starts a short-lived `Deno.serve` callback, validates the `state` parameter, exchanges the code, and writes `OAUTH2_*` values to the configured secret file. |
| `refresh` | Force refresh via `InoreaderTokenManager`. | Initializes the token manager, runs network/connectivity checks, calls the refresh endpoint, and persists the response. Errors throw if the refresh token is missing/invalid. |
| `health` | Validates config, env, storage, and expiry horizon. | Tracks five sub-checks (config, env vars, storage access, refresh token, expiry > 1h) and exits with `1` when the service becomes `unhealthy`. |
| `validate` | Configuration gate for CI/automation. | Loads the config once, prints diagnostics (including a masked client ID suffix), and returns success/failure without contacting Inoreader. |
| `monitor` | Horizon alerting for token freshness. | Computes `time_until_expiry`, `time_since_update`, and a set of human-friendly alerts with thresholds at 5m/30m/1h/2h/6h plus staleness checks (>12h/24h). Reports `alert_level` (info/warning/critical) and exits `0/1/2`. |
| `help` | Prints usage guidance. | Documents the required `--allow-*` permissions and available commands. |

## Configuration

### Required credentials & callback
- `INOREADER_CLIENT_ID` & `INOREADER_CLIENT_SECRET`: must exist, be ≥5 characters, and cannot be placeholders such as `demo-client-id`/`placeholder`.
- `INOREADER_REDIRECT_URI` (default `http://localhost:8080/callback`): dictates where both the one-off `authorize` flow and the daemon’s HTTP listener expect the OAuth callback.

### Retry & network knobs
- `RETRY_MAX_ATTEMPTS` (default `3`), `RETRY_BASE_DELAY` (`1000ms`), `RETRY_MAX_DELAY` (`30000ms`), `RETRY_BACKOFF_FACTOR` (`2`): control the exponential backoff inside `InoreaderTokenManager.retryOperation`.
- `HTTP_TIMEOUT` (`30000ms`), `CONNECTIVITY_TIMEOUT` (`10000ms`), and `CONNECTIVITY_CHECK` (`true` unless set to `false`) tune the HTTP layer and the proactive `checkNetworkConnectivity()` head request to `https://www.inoreader.com`.

### Proxy & fallback support
- Honors `HTTP_PROXY`/`HTTPS_PROXY` with an initial proxy attempt, a targeted 10s connectivity probe, and optional fallback to a direct connection when `NETWORK_FALLBACK_TO_DIRECT=true`. Proxy env vars are temporarily unset during the fallback attempt to avoid loops.

### Logging toggles
- `LOG_LEVEL` (default `INFO`), `LOG_INCLUDE_TIMESTAMP`, `LOG_INCLUDE_STACK_TRACE`, plus `NODE_ENV`/`DENO_ENV` (used by `StructuredLogger.info/debug`) adjust verbosity. Debug statements only emit when `NODE_ENV` equals `development`.

### Storage path
- `TOKEN_STORAGE_PATH` (default `/app/secrets/oauth2_token.env`) is where `EnvFileSecretManager` persists tokens. Adjust this path when the file must live on shared volumes for sidecars or CronJobs.

## DI Container Pattern

`main.ts` serves as a manual DI container: it constructs the gateway layer (`EnvFileSecretManager`, `FetchHttpClient`, `InoreaderTokenClient`), wires them into usecases (`RefreshTokenUsecase`, `HealthCheckUsecase`, `MonitorTokenUsecase`, `AuthorizeUsecase`), then passes usecases to handler layer (`CliHandler`, `OAuthServer`, `DaemonLoop`). No framework is used—dependency resolution is explicit and top-down.

## Secret storage & consumption
- `EnvFileSecretManager` preserves every non-token line in the destination file, removes stale `OAUTH2_*` entries, and rewrites the five canonical keys: `OAUTH2_ACCESS_TOKEN`, `OAUTH2_REFRESH_TOKEN`, `OAUTH2_TOKEN_TYPE`, `OAUTH2_EXPIRES_AT`, `OAUTH2_EXPIRES_IN`.
- `getTokenSecret()` parses the same keys, defaults `scope` to `read write`, and backfills `updated_at` with the current timestamp because the `.env` format does not track it explicitly. `checkSecretExists()` simply checks for the file.
- `pre-processor-sidecar` (and indirectly `pre-processor`) read this `.env` file, so keep `TOKEN_STORAGE_PATH` synchronized across services. When those consumers enable `ENABLE_SECRET_WATCH=true`, they hot-reload whenever `Auth Token Manager` rewrites the file.

## Refresh strategy & daemon
- `InoreaderTokenManager` is the refresh core: it validates the existing refresh token, short-circuits if the current access token expires in more than five minutes, and otherwise calls `https://www.inoreader.com/oauth2/token` with `grant_type=refresh_token`, `User-Agent: Auth-Token-Manager/2.0.0`, and the configured client credentials.
- The manager embeds a robust `fetchWithTimeout()` that prefers proxies, enforces `HTTP_TIMEOUT`, logs every attempt, falls back to direct connections, and restores proxy environment variables afterwards. `retryOperation()` retries failures per the configured backoff, and `checkNetworkConnectivity()` issues a `HEAD` request before every refresh when `CONNECTIVITY_CHECK` is enabled.
- `runTokenRefresh()` wires the manager to `EnvFileSecretManager`, persists tokens on success, and logs metadata (duration, session ID). It is used both by the `refresh` command and the `daemon` loop.
- `daemon` mode launches `startOAuthServer()` so the service can receive Inoreader callbacks, immediately invokes `checkAndRefreshToken()`, and then runs that check every five minutes. `checkAndRefreshToken()` warns when no refresh token exists, refreshes when the stored `expires_at` is missing or within two hours, and otherwise logs that the token is still healthy.

## Authorization, monitoring & health
- `authorize` spins up a temporary HTTP server that listens on the configured callback port, redirects the user to the Inoreader consent page with a randomly generated `state`, validates the returned `state`, exchanges the `code`, and updates the token file before exiting.
- The daemon’s `startOAuthServer()` also handles `GET /` and `GET /auth` by redirecting to Inoreader, logging the `state` for debugging, and storing refreshed tokens whenever the callback path hits `?code=...`.
- `monitor` inspects the token file, computes `time_until_expiry` and `time_since_update`, collects alerts for expiration windows (𝑡 < 5m or 30m → `critical`, 𝑡 < 1h or 2h → `warning`, < 6h → informational) and staleness (>12h/<24h). Missing or short refresh tokens immediately raise `critical` alerts. The command logs `token_status`, `system_status.configuration_valid`, and a `needs_immediate_refresh` flag plus exit codes (`2` for critical, `1` for warning, `0` for info) so orchestrators can react.
- `health` runs five checks (config validity, credentials, storage access, refresh token presence, expiry > 1h). It logs each result, summarizes the `status` (`healthy`, `degraded`, `unhealthy`), and exits `1` if too many checks fail. `validate` simply loads the configuration and echoes success/failure, making it safe for CI.

## Observability & logging
- `StructuredLogger` wraps `@std/log` with a JSON formatter that attaches `component`, `service`, and `version`, and delegates argument sanitization to `DataSanitizer`. The sanitizer redacts OAuth tokens matched by regexes, strips every `SENSITIVE_FIELDS` entry (e.g., `access_token`, `password`, `client_secret`), and preserves harmless data.
- Signal handlers (`SIGINT`, `SIGTERM`) and `globalThis` listeners for `error`/`unhandledrejection` emit sanitized logs before cleanly exiting, which keeps the monitoring loop resilient.
- `tests/security/logger_security_test.ts` ensures that every sensitive string is replaced with `[REDACTED]` in the console output and that non-sensitive fields survive the filtering.

## Runbook
1. Set `INOREADER_CLIENT_ID`, `INOREADER_CLIENT_SECRET`, `INOREADER_REDIRECT_URI` (or accept the default callback), `TOKEN_STORAGE_PATH`, and any optional retry/network overrides before you start anything.
2. Start the always-on service with `deno run --allow-env --allow-net --allow-read --allow-write main.ts daemon`. The daemon logs when the OAuth server is listening, when tokens are refreshed, and when it skips refreshes due to a healthy token.
3. If no tokens exist yet, run `deno run --allow-env --allow-net main.ts authorize`, open the printed URL in a browser, and follow the flow. The command stores `OAUTH2_*` entries and exits as soon as the callback completes.
4. For manual refreshes (e.g., after rotating credentials), run `deno run --allow-env --allow-net main.ts refresh`. This command reuses the same retry/backoff logic the daemon uses.
5. Schedule `deno run --allow-env --allow-net main.ts monitor` (exit `0/1/2`) in your Cron jobs or health dashboards to detect horizon issues without hitting Inoreader.
6. Use `deno run --allow-env --allow-net main.ts health` or `deno run --allow-env --allow-net main.ts validate` before deployments to confirm the CLI can read secrets and that the config is sane.
7. Point `pre-processor-sidecar`/`pre-processor` at the same `TOKEN_STORAGE_PATH` (and keep `ENABLE_SECRET_WATCH=true` when you want live reloads) so everything downstream sees the newest tokens.

## Testing & tooling
- `deno test` runs the unit/security suite under `tests/security/logger_security_test.ts`, which stubs console APIs to verify that sanitized logs never leak tokens or secrets.
- `deno fmt` and `deno lint` keep the codebase consistent before commits.
