# alt-frontend-sv — Playwright E2E dispatch

End-to-end suite for the SvelteKit frontend (`alt-frontend-sv`). Drives
43 spec files across the `auth`, `desktop-chromium`, and `mobile-chrome`
projects defined in [`alt-frontend-sv/playwright.config.ts`](../../../alt-frontend-sv/playwright.config.ts).

This directory follows the common E2E dispatch convention established in
[ADR-000766](../../../docs/ADR/000766.md) (`bash e2e/<framework>/<svc>/run.sh`).

## Why Playwright, not Hurl

The API suites elsewhere in this tree exercise HTTP + Connect-RPC
surfaces. `alt-frontend-sv` renders browser-interactive SvelteKit pages
(auth flows, swipe gestures, tag-verse canvas, streaming summaries);
verifying them requires a real browser engine.

## Self-contained stack against the pre-built image

Unlike the staging-profile suites, this runner **does not bring up
`compose.staging.yaml`**:

- `run.sh` starts the pre-built `ghcr.io/${GHCR_OWNER}/alt-alt-frontend-sv:${ALT_FRONTEND_SV_IMAGE_TAG}`
  container on `--network host` bound to `127.0.0.1:${ALT_E2E_PORT:-4174}`
  with runtime environment variables pointing at the mock servers and
  `PROTOCOL_HEADER=x-forwarded-proto` standing in for the production TLS
  proxy, waits for `/health`, and passes `PW_NO_WEBSERVER=1` and
  `E2E_FORWARDED_PROTO=http` to Playwright so the suite exercises the
  built image over plain HTTP rather than recompiling SvelteKit from
  source on every shard.
- [`tests/e2e/global-setup.ts`](../../../alt-frontend-sv/tests/e2e/global-setup.ts)
  binds mock Kratos on `127.0.0.1:4001`, mock AuthHub on `4002`, and a
  mock backend on `4003` inside the Playwright runner container.

Both containers run with `--network host` so the frontend container and
the mock listeners share loopback. Whereas PR CI
(`.github/workflows/alt-frontend-sv.yml`) builds from source and runs on
Node, `run.sh` tests the production container image (`bun build/index.js`).

## Running

Build or pull the frontend image first when running locally:

```bash
docker build -t ghcr.io/kaikei-e/alt-alt-frontend-sv:ci alt-frontend-sv

# Full default suite (auth + desktop-chromium + mobile-chrome, no shard split)
bash e2e/playwright/alt-frontend-sv/run.sh

# Matrix shard
SHARD=1/3 bash e2e/playwright/alt-frontend-sv/run.sh
SHARD=2/3 bash e2e/playwright/alt-frontend-sv/run.sh
SHARD=3/3 bash e2e/playwright/alt-frontend-sv/run.sh

# Custom project selection
PROJECTS="--project=auth" bash e2e/playwright/alt-frontend-sv/run.sh

# HTML report and JSON/test artifacts are written by alt-frontend-sv/playwright.config.ts under:
ls alt-frontend-sv/playwright-report/ alt-frontend-sv/test-results/
```

## Environment overrides

| Var | Default | Purpose |
|-----|---------|---------|
| `IMAGE_TAG` | `ci` | Docker tag for the service image |
| `GHCR_OWNER` | `kaikei-e` | GHCR namespace |
| `ALT_FRONTEND_SV_IMAGE_TAG` | `$IMAGE_TAG` | override tag for `alt-alt-frontend-sv` |
| `ALT_FRONTEND_SV_IMAGE` | `ghcr.io/$GHCR_OWNER/alt-alt-frontend-sv:$ALT_FRONTEND_SV_IMAGE_TAG` | full image reference override |
| `ALT_E2E_PORT` | `4174` | loopback port for the frontend container |
| `PLAYWRIGHT_IMAGE` | `mcr.microsoft.com/playwright:v<version>-jammy` | browser runner container image, derived from `@playwright/test` in `e2e/playwright/package.json` |
| `SHARD` | `1/1` | Playwright `--shard` value; matrix sets `1/3`, `2/3`, `3/3` |
| `PROJECTS` | `--project=auth --project=desktop-chromium --project=mobile-chrome` | space-separated `--project` flags |
| `RUN_ID` | `$(date +%s)` | unique run identifier used in the report directory and container name |
| `REPORT_DIR` | `e2e/reports/alt-frontend-sv-<RUN_ID>` | created and mounted at `/work/reports` for dispatch-caller compatibility (Playwright itself currently writes reports to `alt-frontend-sv/playwright-report/` and `alt-frontend-sv/test-results/`) |
| `KEEP_STACK` | `0` | when `1`, leaves the frontend container running on exit for inspection |

## Out of scope

- `desktop-webkit` / `mobile-safari` projects (WebKit binary install
  cost; tracked separately).
- `visual-regression` project (snapshot baseline runbook deferred).
- `integration` project + `ALT_RUNTIME_URL` (post-deploy smoke against
  a real stack; deferred).
- Pact CDC coverage for `alt-frontend-sv` — the frontend is not a Pact
  pacticipant, so Playwright is the contract gate on the browser surface.
