# alt-backend — Playwright API E2E suite

End-to-end scenarios for the alt-backend REST surface (`:9000`), the
user-facing Connect-RPC listener (`:9101`), the loopback operator listener
(`:9102`) and the shared ops listener (`:9110`), against a real Postgres with
Atlas migrations applied.

This **replaces** the Hurl suite that used to live at `e2e/hurl/alt-backend/`.
It follows the same dispatch convention ([ADR-000766](../../../docs/ADR/000766.md)),
extended from Hurl to Playwright by
[`e2e/playwright/alt-frontend-sv/`](../alt-frontend-sv/): alt-deploy's
`release-deploy.yaml` e2e matrix probes both framework directories, so
`bash e2e/<framework>/<svc>/run.sh` remains the contract.

Upstream services (search-indexer, pre-processor, recap-worker,
rag-orchestrator, knowledge-sovereign, mq-hub) are replaced by a single
multi-alias FastAPI stub
([`e2e/stubs/alt-backend-deps-stub`](../../stubs/alt-backend-deps-stub/)).
`alt-data-hub` is the one upstream that is **not** stubbed: after ADR-000954
Wave 3 it is the only owner of `alt_db` and `cmd/backend` holds no DB pool at
all, so the real binary runs in this slice and alt-backend reaches it over
mutual TLS. `run.sh` mints the throwaway PKI for that hop.

## Running

```bash
# Full suite: install deps, bring the slice up, run, tear down
bash e2e/playwright/alt-backend/run.sh

# One shard of four (what CI does)
SHARD=1/4 PW_BLOB=1 bash e2e/playwright/alt-backend/run.sh

# A subset
PW_GREP="CSRF" bash e2e/playwright/alt-backend/run.sh

# Debug: leave the stack up for inspection
KEEP_STACK=1 bash e2e/playwright/alt-backend/run.sh
docker compose -f compose/compose.staging.yaml -p alt-staging logs alt-backend
```

Reports land under `e2e/reports/alt-backend-<RUN_ID>/` — HTML + JUnit for a
plain run, a blob for a sharded one (see **Fast CI** below).

The script expects the alt-backend and alt-data-hub images to exist as
`ghcr.io/${GHCR_OWNER:-kaikei-e}/alt-alt-{backend,data-hub}:${IMAGE_TAG:-ci}`.
CI builds them in preceding steps; locally:

```bash
docker build --build-arg BINARY=backend \
  -t ghcr.io/kaikei-e/alt-alt-backend:ci -f alt-backend/Dockerfile.backend alt-backend
docker build --build-arg BINARY=datahub \
  -t ghcr.io/kaikei-e/alt-alt-data-hub:ci -f alt-backend/Dockerfile.backend alt-backend
docker build -t ghcr.io/kaikei-e/alt-alt-backend-deps-stub:ci e2e/stubs/alt-backend-deps-stub
```

`--build-arg BINARY=` is required, not optional: one Dockerfile and one Go
module produce all three split binaries, so `BINARY` selects `./cmd/<BINARY>`
and the Dockerfile fails the build outright when it is unset (ADR-000954).

### Iterating without Docker

`playwright.config.ts` reads every endpoint from the environment, so a suite
run against an already-running stack needs no container at all:

```bash
cd e2e/playwright/alt-backend
PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 npm ci
BASE_URL=http://localhost:9000 CONNECT_URL=http://localhost:9101 \
INTERNAL_URL=http://localhost:9102 OPS_URL=http://localhost:9110 \
JWT_FILE=../../fixtures/alt-backend/test-jwt.txt \
npx playwright test --ui
```

## Why Playwright rather than Hurl

| | Hurl | Playwright |
|---|---|---|
| Assertions | one JSONPath at a time | whole-envelope Zod schemas |
| Isolation | file-ordered, shared fixtures | per-worker data islands |
| Parallelism | `--jobs 4` across files | `fullyParallel` across tests + `--shard` across machines |
| Multi-step logic | none (linear request list) | round-trips, retries, conditional follow-ups |
| Failure output | assertion line | body preview + full request/response trace |

The concrete cost of the first two rows: the Hurl suite asserted
`jsonpath "$" exists` in nine places — an assertion that passes for `null`,
`[]`, `{}` and an error envelope alike — and chained
`10-register` → `11-list` → `13-delete` through shared rows, so the delete
scenario removed `$[0].id`, whichever feed that happened to be.

## Structure

```
playwright.config.ts   API-only config: no browsers, blob reporting under PW_BLOB=1
setup/global-setup.ts  readiness gate (the old 00-setup.hurl)
src/env.ts             required-env parsing, fail-fast (CLAUDE.md rule 9)
src/fixtures.ts        worker-scoped clients, CSRF token, seeded feeds, client IP
src/schemas.ts         Zod response envelopes
src/http.ts            status/schema assertions that print the body on failure
tests/*.spec.ts        one file per surface
```

### Parallelism and isolation

`fullyParallel: true` distributes individual tests, not files, so shards stay
balanced. Isolation comes from **naming, not teardown**: each worker seeds
three RSS feeds under a URL prefix nothing else uses, and every mutating test
registers its own row first. Assertions are containment ("my three URLs are in
the list"), never cardinality ("there are at least three rows"), so a sibling
worker's data is invisible rather than interfering.

### The `X-Real-IP` fixture

alt-backend's DoS middleware runs with `trustForwardedHeaders=true` (it is
always deployed behind an nginx that rewrites the header), keys its token
bucket on the client IP, and **blocks an IP for five minutes** once the bucket
empties — 100 req/min sustained, burst 200. Six workers sharing the runner
container's single source address would drain one shared bucket partway through
the run and 429 into a five-minute hole.

So each worker sends a distinct `X-Real-IP` in RFC 6598 space. This is not a
workaround for the limiter — it is what production looks like, where every
browser is a distinct address. The limiter itself is asserted deliberately, on
a dedicated address, in `tests/rate-limit.spec.ts`.

## Coverage

25 spec files, ~211 tests. Everything the Hurl suite asserted is preserved,
including its recorded known bugs. New surfaces:

| Area | What was missing |
|---|---|
| CSRF middleware | both rejection branches and the exemption list — only the success path was exercised |
| Security headers | CSP / HSTS / nosniff / frame-options / referrer-policy, request-id, CORS preflight allow-headers |
| Body limits | the 2MB ceiling and the `/stream` skipper that exists to bypass it |
| DoS protection | rate limiting, per-IP scoping, whitelist bypass — untested entirely |
| Connect-RPC mux | 8 of 9 registered services never probed (see below) |
| `/v1/feeds/summarize/stream` | explicitly excluded by the Hurl suite |
| `/v1/admin/scraping-domains/:id/refresh-robots` | no assertion of any kind |
| Auth boundary | probed one endpoint; now one per route group |
| Cursor pagination | `next_cursor` was never followed |
| ETag / `If-None-Match` | the 304 branch was dead code from the suite's view |
| OPML | import never round-tripped through the list |

### Connect-RPC registration as a wiring gate

`tests/connect-surface.spec.ts` asserts each of the eight unconditionally
mounted services answers **401** (auth interceptor ran → handler is mounted)
rather than **404** (path resolved to nothing → never registered). That is
CLAUDE.md rule 8's failure mode — "DI forgot to wire" being indistinguishable
from "intentionally disabled" — pushed out to the boundary where the only thing
that can tell them apart is whether the path resolves.

## Known gaps pinned by this suite

These assert **current** behaviour, not desired behaviour, and each carries the
reasoning inline. When the underlying issue is fixed, the test fails — which is
the intended signal.

### Carried over from the Hurl suite

| Where | What |
|---|---|
| `tests/admin-scraping-domains.spec.ts` | GET and PATCH disagree on an unknown id (404 vs 500) because the driver returns a bare `errors.New` |
| `tests/morning-letter.spec.ts` | deps-stub has no `/v1/morning/updates` route; the catch-all's object fails to decode into a slice |
| `tests/augur-rag.spec.ts` | deps-stub has no `/v1/rag/retrieve` route; same drift |
| `tests/admin-dashboard.spec.ts` | deps-stub has no `/v1/dashboard/recap_jobs` route |

### Found by this suite

The first two came out of reading the code during the port; the rest came out
of the suite's first CI run, where an assertion written from the Hurl file's
own description of the API turned out to disagree with the API.

| Where | What |
|---|---|
| `tests/augur-rag.spec.ts` | `GET /v1/rag/context` and `POST /sse/v1/rag/answer` are registered without `RequireAuth()`, unlike every other `/v1` group |
| `tests/connect-surface.spec.ts` | an unknown procedure on a known service 404s from Go's `http.NotFound`, so connect-es sees a transport error rather than a `ConnectError` code |
| `tests/feeds-cursor.spec.ts` | the three cursor endpoints do not share an envelope — `/fetch/cursor` has `has_more`, `/viewed/cursor` and `/favorites/cursor` do not |
| `tests/feeds-cursor.spec.ts` | ValidationMiddleware matches `/feeds/fetch/cursor` by substring, so the viewed and favorites variants get no pagination validation and silently clamp instead of rejecting |
| `tests/rss-feed-link.spec.ts` | `DELETE /:id` unsubscribes the caller (`user_feed_subscriptions`) while `/list` is global — they read as a CRUD pair and are not one |
| `tests/rss-feed-link.spec.ts` | `DELETE` of an unknown id answers 200, while the admin surface answers 404 for the same shape of miss |
| `tests/feeds-details-tags.spec.ts` | `GET /v1/feeds/:id/tags` never parses `:id`, so a malformed one reaches the query and 500s |
| `tests/feeds-mutations.spec.ts` | `POST /v1/feeds/read` runs no validator, so an empty `feed_url` 500s instead of 400 |
| `tests/rss-feed-link.spec.ts` | a non-OPML upload to `/import/opml` 500s instead of 4xx |
| `tests/articles.spec.ts` | `/v1/articles/search` silently ignores `limit`, unlike every neighbouring paginated read |
| `tests/csrf.spec.ts` | the 403 body is not parseable as a single JSON document — something writes past the custom `HTTPErrorHandler`'s `c.JSON` |

## Fast CI

`.github/workflows/e2e-playwright-alt-backend.yml` runs the suite as a
four-shard matrix and merges the blobs:

- **No browser download.** API-only specs never launch one;
  `PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1` into `node:22-bookworm-slim` saves the
  ~1.7GB Playwright image and its pull time.
- **Images built once**, exported as an artifact, loaded by each shard —
  instead of four shards each running the same buildx.
- **`fullyParallel`** so shards split by test, not by file.
- **Blob reporter + `merge-reports`** for one HTML report and one JUnit file
  across all shards.
- **Registry layer cache** (`:buildcache`, published by `docker-build.yaml`)
  imported by the build job.

Locally, one shard is roughly `total / 4`; use the merged report's Speedboard
tab to rebalance with `--shard-weights` if the split ever skews.

## Excluded

- Real auth-hub / Kratos login flow — the pre-minted JWT removes the need to
  spin up an auth stack for E2E.
- Image-proxy signature validation — staging never sets `IMAGE_PROXY_SECRET`
  (the backend logs `image_proxy_misconfigured` at startup), so there is no
  secret to sign a valid request with. What *is* asserted is that the route is
  not registered while the proxy is disabled.
- Connect-RPC business semantics beyond registration and error envelopes —
  those are covered per-service by the Pact consumer tests under
  `alt-backend/pacts/`.
