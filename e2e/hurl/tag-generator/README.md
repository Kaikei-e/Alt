# tag-generator — E2E specification

Fourth Hurl-driven end-to-end suite in the Alt monorepo, after
`search-indexer`, `mq-hub`, and `knowledge-sovereign`. Covers the
tag-generator service (Python 3.14 / FastAPI / KeyBERT + SBERT) —
its HTTP contract plus the Redis-Streams round trip that mq-hub's
`GenerateTagsForArticle` RPC wraps synchronously.

tag-generator exposes:

- **Plain HTTP JSON on `:9400`** (FastAPI, hand-rolled Pydantic models,
  snake_case response fields — not proto3-JSON):
  - `GET /health`
  - `POST /api/v1/extract-tags` — stateless text → tags inference
  - (other endpoints — `/api/v1/generate-tags`,
    `/api/v1/user-preferences` — are out of scope for this slice; see
    *Out of scope* below. The former `/api/v1/tags/batch` surface was
    removed per ADR-000241 / ADR-000397, with the replacement contract
    on alt-backend's `BatchGetTagsByArticleIDs` RPC.)
- **Redis Streams consumer** on `alt:events:tags` (TagGenerationRequested
  → TagGenerationCompleted reply), driven indirectly via mq-hub's
  `services.mqhub.v1.MQHubService/GenerateTagsForArticle` RPC.

## Prerequisites

Hurl 7.1.0+ on the host (the repo ships `hurl_7.1.0_amd64.deb`), plus
Docker Compose. No Python fixtures — every request body is either a
small JSON file under `e2e/fixtures/tag-generator/` or inlined into the
scenario (scenario 04 inlines because Hurl 7.1 does not template
`file,…;` bodies).

`run.sh` brings the staging slice up automatically. Manual:

```sh
docker compose -f compose/compose.staging.yaml -p alt-staging \
  --profile tag-generator \
  up -d --wait redis-streams stub-backend mq-hub tag-generator
```

Ports published for host-local debugging: `19400:9400` (tag-generator),
`19500:9500` (mq-hub). The `alt-staging` network is `internal: true`,
so Hurl must run inside it — `run.sh` handles that.

## Running

```sh
bash e2e/hurl/tag-generator/run.sh
```

Env overrides:

| Var | Default | Purpose |
|-----|---------|---------|
| `BASE_URL` | `http://tag-generator:9400` | tag-generator URL (in-network DNS) |
| `MQHUB_BASE_URL` | `http://mq-hub:9500` | mq-hub URL for scenario 04 |
| `HURL_IMAGE` | `ghcr.io/orange-opensource/hurl:7.1.0` | Hurl container |
| `IMAGE_TAG` | `main` | ghcr.io tag for tag-generator + mq-hub images |
| `GHCR_OWNER` | `kaikei-e` | GHCR namespace |
| `RUN_ID` | `$(date +%s)` | isolates `articleId` across parallel runs |
| `KEEP_STACK` | `0` | set to `1` to leave the stack up on exit |

Reports land under `e2e/reports/tag-generator-<RUN_ID>/` (gitignored):
JUnit XML + HTML.

Debugging a failure:

```sh
KEEP_STACK=1 bash e2e/hurl/tag-generator/run.sh
docker compose -f compose/compose.staging.yaml -p alt-staging \
  logs --tail=200 tag-generator
docker compose -f compose/compose.staging.yaml -p alt-staging \
  logs --tail=200 mq-hub
docker compose -f compose/compose.staging.yaml -p alt-staging \
  exec redis-streams redis-cli XLEN alt:events:tags
```

## Scenario ordering

The suite runs serially (`--jobs 1`). Scenario 03 triggers a real SBERT
inference that warms the model cache, so scenario 04's round trip fits
inside `timeoutMs=15000` even on a cold CI runner where first-invocation
inference can be several seconds.

## Scenarios

### 00 — Readiness probe (pre-flight)
- **Given** the staging stack is starting.
- **When** `GET :9400/health` is polled.
- **Then** it returns `{"status":"healthy","service":"tag-generator"}`
  within 15 s (30 × 500 ms).

### 01 — /health schema
- **Given** tag-generator is ready.
- **When** `GET /health` is called.
- **Then** the response is `application/json` with `status=healthy` and
  `service=tag-generator`. Schema is snake_case — this is FastAPI's
  default encoder, not proto3-JSON.

### 02 — POST /api/v1/extract-tags smoke
- **Given** tag-generator is ready but the FastAPI lifespan may still
  be initializing `_background_tag_service` in a sidecar thread.
- **When** `POST /api/v1/extract-tags` is called with a minimal body
  (`{"title":"hurl smoke","content":"..."}`).
- **Then** the response is 200 with `success=true`, `tags` as a
  collection (possibly empty), `confidence` and `inference_ms` numeric,
  and `language` present. The retry (20 × 500 ms) absorbs the race
  between `/health` returning 200 and the background service becoming
  reachable.

### 03 — POST /api/v1/extract-tags keyword content
- **Given** tag-generator is ready.
- **When** `POST /api/v1/extract-tags` is called with a keyword-dense
  English paragraph about ML / Python / FastAPI.
- **Then** the response is 200 with `tags` containing at least one
  entry and `confidence > 0`. Doubles as an SBERT warm-up for
  scenario 04.

### 04 — Redis Streams round trip via mq-hub
- **Given** tag-generator's Redis Streams consumer has joined the
  `alt:events:tags` group (it does so during FastAPI lifespan, before
  `/health` returns 200).
- **When** `POST /services.mqhub.v1.MQHubService/GenerateTagsForArticle`
  is called on mq-hub with an inline body containing the run-scoped
  `articleId`, `timeoutMs=15000`.
- **Then** the response is 200 with `success=true`, `articleId`
  matching the request, `tags` as a collection, and `inferenceMs > 0`.
  Proves the round trip (mq-hub publishes → tag-generator consumes →
  tag-generator replies → mq-hub returns) works end-to-end.
  Response fields are proto3-JSON camelCase (mq-hub is Connect-RPC).

### 05 — POST /api/v1/extract-tags missing title
- **Given** the server is ready.
- **When** `POST /api/v1/extract-tags` is called with a body that omits
  `title`.
- **Then** FastAPI + Pydantic reject the request at schema validation
  and respond 422 with a populated `detail` array. The ML pipeline is
  never reached.

### 06 — POST /api/v1/extract-tags missing content
- **Given** the server is ready.
- **When** `POST /api/v1/extract-tags` is called with a body that omits
  `content`.
- **Then** the response is 422 with Pydantic's `detail` array.

### 07 — POST /api/v1/extract-tags empty strings
- **Given** the server is ready.
- **When** `POST /api/v1/extract-tags` is called with both `title` and
  `content` as empty strings.
- **Then** the response is 200 with `success=true`, `tags=[]`,
  `confidence=0`, and a detected `language`. This pins the graceful
  no-signal arm of the ML pipeline.

## Out of scope (deferred)

- **`POST /api/v1/extract-tags` oversized content** — no server-side
  length constraint today; would need a Pydantic `max_length` change
  before the scenario is meaningful.
- **Readiness race (503 before lifespan finished)** — requires a cold
  start probe inside the compose slice; the existing scenario 02 retry
  budget covers it implicitly.
- **`TagGenerationRequested` validation failure** — the reply arm that
  sets `success=false` + `error_message`.
- **`ArticleCreated` Redis Streams round trip** — needs an observable
  side effect (alt-backend in staging or tag-generator metrics) to
  assert against.
- **`POST /api/v1/generate-tags` + `/api/v1/user-preferences`** —
  authenticated endpoints. Staging currently runs a no-op auth
  fallback; gating these makes sense once `alt-auth` is in the
  staging stack.
- **mTLS peer-identity enforcement** — tag-generator runs with
  `MTLS_ENFORCE=false` and `PEER_IDENTITY_TRUSTED=off` in staging,
  matching the mq-hub and knowledge-sovereign suites.
- **Multi-tenant / concurrent clients** — parallel runs against
  distinct `articleId`s.

## References

- ADR-000763 — Hurl E2E pipeline adoption
- ADR-000764 — mq-hub Hurl suite (profile pattern precedent,
  `GenerateTagsForArticle` timeout variant in scenario 14)
- ADR-000765 — knowledge-sovereign Hurl suite (happy-path-first
  phasing, proto3-JSON vs encoding/json split)
- `tag-generator/app/auth_service.py` — source of truth for the
  `/health` and `/api/v1/extract-tags` contracts
- `mq-hub/app/connect/v1/mqhub/handler.go` — mq-hub RPC entry point
  for scenario 04
