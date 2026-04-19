# alt-backend Hurl E2E suite

End-to-end scenarios that exercise the alt-backend REST surface (port
9000) and the Connect-RPC listener (port 9101) against a real Postgres
database with Atlas migrations applied. Upstream services (search-indexer,
pre-processor, recap-worker, rag-orchestrator, knowledge-sovereign,
mq-hub) are replaced by a single multi-alias FastAPI stub
([`e2e/stubs/alt-backend-deps-stub`](../../stubs/alt-backend-deps-stub/)).

Follows the three-service common dispatch convention established in
[ADR-000766](../../../docs/ADR/000766.md); conceptually this is
Increment 5 after search-indexer / mq-hub / knowledge-sovereign /
tag-generator / news-creator / recap-worker / acolyte-orchestrator.

## Running

```bash
# Full suite (builds alt-backend image, brings up stack, runs Hurl, tears down)
bash e2e/hurl/alt-backend/run.sh

# Debug: leave the stack up for inspection
KEEP_STACK=1 bash e2e/hurl/alt-backend/run.sh
docker compose -f compose/compose.staging.yaml -p alt-staging logs alt-backend

# Reports (JUnit + HTML) land under:
ls e2e/reports/alt-backend-*/
```

The script expects the alt-backend image to already exist as
`ghcr.io/${GHCR_OWNER:-kaikei-e}/alt-alt-backend:${IMAGE_TAG:-ci}`. CI
builds it in a preceding job step; locally, build it first:

```bash
docker build \
  -t ghcr.io/kaikei-e/alt-alt-backend:ci \
  -f alt-backend/Dockerfile.backend \
  alt-backend
```

## Authentication

Every private endpoint requires `X-Alt-Backend-Token` — an HS256 JWT
signed with the secret in
[`e2e/fixtures/staging-secrets/alt_backend_token_secret.txt`](../../fixtures/staging-secrets/alt_backend_token_secret.txt).
The token is pre-minted at
[`e2e/fixtures/alt-backend/test-jwt.txt`](../../fixtures/alt-backend/test-jwt.txt)
(role=admin, exp=2099-01-01) and injected into every scenario via the
Hurl `{{jwt}}` variable.

State-mutating endpoints also require `X-CSRF-Token`. Each scenario that
issues POST/PUT/PATCH/DELETE captures a fresh token via
`GET /v1/csrf-token` at the top of the file.

## Upstream mocking

`alt-backend-deps-stub` is a single FastAPI container registered on the
`alt-staging` network under six aliases:

| Alias | Replaces | Proto |
|---|---|---|
| `search-indexer` | search-indexer | Connect-RPC JSON |
| `pre-processor` | pre-processor | Connect-RPC JSON |
| `recap-worker` | recap-worker | HTTP/REST |
| `rag-orchestrator` | rag-orchestrator | HTTP/REST + Connect-RPC |
| `knowledge-sovereign` | knowledge-sovereign | Connect-RPC JSON |
| `mq-hub` | mq-hub | Connect-RPC JSON |

A seventh alias, `stub.invalid`, serves a minimal RSS 2.0 document at
`/alt-backend/e2e/feed-*.xml` so registration scenarios have a real
upstream to fetch. The hostname is allowlisted on alt-backend via
`FEED_ALLOWED_HOSTS=stub.invalid` to bypass the SSRF private-IP check.

## Scenarios

| File | Coverage |
|---|---|
| `00-setup.hurl` | Readiness probe for REST + Connect-RPC ports (retried) |
| `01-health-csrf.hurl` | `/v1/health`, `/v1/csrf-token`, `/metrics` (public) |
| `02-auth-negative.hurl` | Missing / malformed / wrong-signature JWT → 401 |
| `10-rss-feed-link-register.hurl` | `POST /v1/rss-feed-link/register` ×3 + SSRF negative |
| `11-rss-feed-link-list.hurl` | `GET list`, `random`, `export/opml` |
| `12-rss-feed-link-opml-import.hurl` | `POST /v1/rss-feed-link/import/opml` (multipart) |
| `13-rss-feed-link-delete.hurl` | List → capture id → `DELETE /v1/rss-feed-link/:id` |
| `20-feeds-fetch-list.hurl` | `/v1/feeds/fetch/{single,list,limit/:n,page/:n}` |
| `21-feeds-fetch-cursor.hurl` | `/v1/feeds/fetch/{cursor,viewed/cursor,favorites/cursor}` + `/count/unreads` |
| `22-feeds-read-favorite.hurl` | `POST /v1/feeds/read`, `POST /v1/feeds/register/favorite` |
| `23-feeds-search.hurl` | `POST /v1/feeds/search` → search-indexer stub |
| `24-feeds-details-tags.hurl` | `POST /v1/feeds/fetch/details`, `POST /v1/feeds/tags`, `GET /v1/feeds/:id/tags` |
| `25-feeds-stats.hurl` | `/v1/feeds/stats`, `/stats/detailed`, `/stats/trends` |
| `26-feeds-summary.hurl` | `POST /v1/feeds/fetch/summary{,/provided}` → pre-processor stub |
| `27-feeds-summarize-queue.hurl` | `POST /v1/feeds/summarize{,/queue}`, `GET /summarize/status/:id` |
| `30-articles.hurl` | `/v1/articles/{fetch/cursor,fetch/content,by-tag,:id/tags,search,archive}` |
| `40-morning-letter.hurl` | `GET /v1/morning-letter/updates` → recap-worker stub |
| `50-images.hurl` | `POST /v1/images/fetch`, `GET /v1/images/proxy/:sig/:url` |
| `60-augur-rag.hurl` | `GET /v1/rag/context`, `POST /sse/v1/rag/answer` → rag-orchestrator stub |
| `70-admin-dashboard.hurl` | `/v1/dashboard/{metrics,overview,logs,jobs,recap_jobs}` (admin-only) |
| `71-admin-scraping-domains.hurl` | `/v1/admin/scraping-domains` list/get/patch |
| `80-csp-report.hurl` | `POST /security/csp-report` (public) |

## Excluded from the initial suite

- `POST /v1/feeds/summarize/stream` — NDJSON chunked streaming; framing
  semantics deferred to a follow-up scenario.
- Real auth-hub / Kratos login flow — the pre-minted JWT removes the
  need to spin up an auth stack for E2E.
- Connect-RPC methods beyond port-readiness — exercised separately by
  per-service Pact consumer tests under `alt-backend/pacts/`.
