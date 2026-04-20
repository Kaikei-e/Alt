# alt-frontend-sv — Playwright E2E dispatch

End-to-end suite for the SvelteKit frontend (`alt-frontend-sv`). Drives
43 spec files across the `auth`, `desktop-chromium`, and `mobile-chrome`
projects defined in [`alt-frontend-sv/playwright.config.ts`](../../../alt-frontend-sv/playwright.config.ts).

This directory extends the common E2E dispatch convention established in
[ADR-000766](../../../docs/ADR/000766.md) from Hurl (`e2e/hurl/<svc>/run.sh`)
to Playwright (`e2e/playwright/<svc>/run.sh`). alt-deploy's
`release-deploy.yaml` e2e matrix probes both directories so a single
`bash e2e/<framework>/<svc>/run.sh` remains the dispatch contract.

## Why Playwright, not Hurl

The Hurl suites elsewhere in this tree exercise HTTP + Connect-RPC
surfaces. alt-frontend-sv renders browser-interactive SvelteKit pages
(auth flows, swipe gestures, tag-verse canvas, streaming summaries);
verifying them requires a real browser engine. Playwright is already the
framework of record for the PR-time CI (`alt-frontend-sv.yml`).

## Self-contained stack

Unlike the Hurl suites, this runner **does not bring up
`compose.staging.yaml`**:

- `playwright.config.ts` webServer boots `bun run build && node build`
  on port 4174 inside the test process.
- [`tests/e2e/global-setup.ts`](../../../alt-frontend-sv/tests/e2e/global-setup.ts)
  binds mock Kratos on `127.0.0.1:4001`, mock AuthHub on `4002`, and a
  mock backend on `4003`.

The Playwright container runs with `--network host` so both the
webServer and the mock listeners share one namespace. When the future
need arises to verify against a real stack (butterfly-facade + backend +
auth-hub), add a sibling `integration` dispatch instead of reworking
this one.

## Running

```bash
# Full default suite (auth + desktop-chromium + mobile-chrome, no shard split)
bash e2e/playwright/alt-frontend-sv/run.sh

# Matrix shard (mirrors alt-frontend-sv.yml)
SHARD=1/3 bash e2e/playwright/alt-frontend-sv/run.sh
SHARD=2/3 bash e2e/playwright/alt-frontend-sv/run.sh
SHARD=3/3 bash e2e/playwright/alt-frontend-sv/run.sh

# Custom project selection
PROJECTS="--project=auth" bash e2e/playwright/alt-frontend-sv/run.sh

# Reports (HTML + JUnit via Playwright's default reporter set) land under:
ls e2e/reports/alt-frontend-sv-*/
```

## Environment overrides

| Var | Default | Purpose |
|-----|---------|---------|
| `PLAYWRIGHT_IMAGE` | `mcr.microsoft.com/playwright:v1.59.1-jammy` | container image; matches `alt-frontend-sv.yml` |
| `BUN_VERSION` | `1.2.14` | bun toolchain version installed inside the container |
| `SHARD` | `1/1` | Playwright `--shard` value; matrix sets `1/3`, `2/3`, `3/3` |
| `PROJECTS` | `--project=auth --project=desktop-chromium --project=mobile-chrome` | space-separated `--project` flags |
| `RUN_ID` | `$(date +%s)` | unique run identifier used in the report directory |
| `REPORT_DIR` | `e2e/reports/alt-frontend-sv-<RUN_ID>` | override report output path |
| `KEEP_STACK` | `0` | no-op; accepted for parity with Hurl run.sh ergonomics |

## In-container sequence

The Playwright container step sequence is kept in lockstep with
`.github/workflows/alt-frontend-sv.yml` so PR-time green and
deploy-time green cannot diverge:

1. `apt-get install unzip ca-certificates curl` (with 3-attempt retry)
2. `curl -fsSL https://bun.sh/install | bash -s -- bun-v${BUN_VERSION}`
3. `bun install --frozen-lockfile`
4. `bunx playwright test ${PROJECTS} --shard=${SHARD}`

If this sequence ever drifts from the CI workflow, the dispatch
becomes an unreliable gate: flakes reproduce in only one of the two
paths. Update both together.

## Out of scope

- `desktop-webkit` / `mobile-safari` projects (WebKit binary install
  cost; tracked separately).
- `visual-regression` project (snapshot baseline runbook deferred).
- `integration` project + `ALT_RUNTIME_URL` (post-deploy smoke against
  a real stack; deferred).
- Pact CDC coverage for alt-frontend-sv — the frontend is not a Pact
  pacticipant, so Playwright is the sole contract gate on the deploy
  path. Mock drift from production butterfly-facade / backend is
  detected separately.
