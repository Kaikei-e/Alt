# search-indexer — E2E specification

This is the first Hurl-driven end-to-end suite in the Alt monorepo. Each
scenario is a paired narrative (this file) plus an executable `NN-<feature>.hurl`.
Treat the README as the human contract and the `.hurl` as the enforcement —
they move together.

## Prerequisites

The staging stack must be up and healthy:

```sh
IMAGE_TAG=sha-<short> GHCR_OWNER=kaikei \
  docker compose -f compose/compose.staging.yaml -p alt-staging up -d --wait
```

Meilisearch's master key for staging is baked into `compose/compose.staging.yaml`
(`alt-staging-test-master-key`) — it is **only** valid inside the ephemeral
staging project.

## Running

From the repo root:

```sh
# Seed first (serial — later tests depend on this state).
hurl --test \
  --variable meili_master_key=alt-staging-test-master-key \
  e2e/hurl/search-indexer/00-seed-meilisearch.hurl

# Then run the test suite (parallel is safe).
hurl --test --jobs 4 --retry 5 --retry-interval 500 \
  --report-junit e2e/reports/junit.xml \
  --report-html  e2e/reports/html \
  e2e/hurl/search-indexer/0[1-9]-*.hurl
```

In CI, the E2E job runs Hurl inside a container attached to the `alt-staging`
network so service DNS names (`meilisearch`, `search-indexer`) resolve
directly. Locally, publish ports 17700/19300 (see `compose.staging.yaml`)
and swap the service names for `localhost:<published port>` if invoking
Hurl from the host.

## Scenarios

### 00 — Seed Meilisearch fixtures (pre-flight)
- **Given** search-indexer has started and called `bootstrap.EnsureIndex`
  (filterable attributes on `tags`, `user_id` are configured)
- **When** we POST the fixture corpus to `meilisearch:7700/indexes/articles/documents`
- **Then** the indexing task reaches status `succeeded` before later files run
- Executor: `00-seed-meilisearch.hurl`

### 01 — Health probe
- **Given** the search-indexer process is up
- **When** we `GET /health` on `:9300`
- **Then** the response is 200 with body `{"status":"ok"}` and JSON content-type
- Executor: `01-health.hurl`

### 02 — Basic unfiltered search
- **Given** the articles index has been seeded
- **When** we `GET /v1/search?q=rust&limit=10` (no `user_id`)
- **Then** we hit the internal RAG path (`SearchArticlesUsecase`)
- **And** we get at least two hits, each with `id`, `title`, `content`, `tags[]`, `score`
- Executor: `02-search-basic.hurl`

### 03 — Limit cap
- **Given** the same seeded index
- **When** we `GET /v1/search?q=rust&limit=1`
- **Then** exactly one hit comes back, and `total == 1`
- Note: the REST endpoint exposes `total = len(hits)`, not Meilisearch's
  `estimatedTotalHits`. Clients needing a true total must call the
  Connect-RPC path on `:9301`.
- Executor: `03-search-pagination-limit.hurl`

### 04 — User-scoped search
- **Given** alice owns `doc-rust-tokio` and bob owns `doc-rust-borrow`
- **When** we `GET /v1/search?q=rust&user_id=alice`
- **Then** exactly one hit is returned: `doc-rust-tokio`
- **And** the same query with `user_id=bob` returns only `doc-rust-borrow`
- **And** `user_id=nobody` returns an empty result set (not a 4xx)
- Executor: `04-search-user-scoped.hurl`

### 05 — Missing query → 400
- **Given** any service state
- **When** we `GET /v1/search` (no `q`, or `q=`)
- **Then** the response is 400 with body containing `"query parameter required"`
- Executor: `05-search-empty-query.hurl`

### 06 — Limit bounds
- **Given** the seeded index
- **When** we request `limit=0`, `limit=1001`, or `limit=notanumber`
- **Then** the handler silently falls back to its default (50) and returns
  200 with full matches (2 for `q=rust`)
- Executor: `06-search-limit-bounds.hurl`

## Out of scope for this suite

- **Connect-RPC on `:9301`** — HTTP/2 h2c, not practical to drive from Hurl.
  Covered by existing Go integration tests under `search-indexer/app/`.
- **mTLS listener on `:9443`** — covered by provider contract tests.
- **Ingestion via Redis Streams** — the fixture corpus is written directly
  to Meilisearch; the streams consumer is exercised by its own integration
  tests.

## Pattern note

Paired markdown + Hurl gives us a Gauge-style executable spec without
pulling the JVM into our stack. When (if) we need reusable step definitions
across services, promote to **godog** (Go-native Cucumber). Until then,
this convention is the whole framework.
