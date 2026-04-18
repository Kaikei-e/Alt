# news-creator — E2E specification

Fifth Hurl-driven end-to-end suite in the Alt monorepo, after
`search-indexer`, `mq-hub`, `knowledge-sovereign`, and `tag-generator`.
Covers the news-creator service (Python 3.14 / FastAPI / Ollama proxy +
HybridPrioritySemaphore + RAG helpers) — its HTTP contract surface plus
the queue / streaming / backpressure paths that Pact CDC alone cannot
pin.

news-creator's production backend is Ollama on an RTX 4060 GPU. CI has
no GPU, so the staging slice swaps Ollama for a tiny FastAPI stub
(`compose/news-creator-ollama-stub/app.py`) that answers a fixed
`/api/tags` response. Subsequent phases extend the stub with
`/api/generate`, `/api/chat`, and an admin endpoint for queue-saturation
control.

## Phased rollout

- **Phase 1 (this commit)** — readiness gate + `/health` schema +
  `/queue/status` schema. Establishes the staging compose profile, the
  Ollama stub, the `run.sh` skeleton, and the CI job. Out of scope:
  any LLM call, streaming, error paths, queue saturation.
- Phase 2 — summarize / generate / recap happy paths (`/api/v1/summarize`
  non-streaming, `/api/generate`, `/v1/summary/generate`,
  `/v1/summary/generate/batch`).
- Phase 3 — RAG endpoints (`/api/chat` non-streaming,
  `/api/v1/expand-query`, `/api/v1/plan-query`, `/v1/rerank`).
- Phase 4 — streaming (SSE for summarize, NDJSON for chat),
  HTTP 429 + `Retry-After: 30` queue-saturation, validation errors,
  morning-letter.

news-creator exposes (Phase 1 surface only):

- `GET /health` — `{status, service, models[]}` (FastAPI snake_case)
- `GET /queue/status` — HybridPrioritySemaphore depth + `accepting` flag

The other endpoints (`/api/v1/summarize`, `/api/generate`,
`/v1/summary/generate*`, `/api/chat`, `/api/v1/expand-query`,
`/api/v1/plan-query`, `/v1/rerank`, `/v1/morning-letter/generate`) come
online in Phase 2-4.

## Prerequisites

Hurl 7.1.0+ on the host (the repo ships `hurl_7.1.0_amd64.deb`), plus
Docker Compose. No fixture files yet — Phase 1 inlines all assertions.
The `e2e/fixtures/news-creator/` directory is reserved for Phase 2's
request bodies.

`run.sh` brings the staging slice up automatically. Manual:

```sh
docker compose -f compose/compose.staging.yaml -p alt-staging \
  --profile news-creator \
  up -d --wait --build news-creator-ollama-stub news-creator
```

Ports published for host-local debugging: `11434:11434` (news-creator).
The `alt-staging` network is `internal: true`, so Hurl must run inside
it — `run.sh` handles that.

## Running

```sh
bash e2e/hurl/news-creator/run.sh
```

Env overrides:

| Var | Default | Purpose |
|-----|---------|---------|
| `BASE_URL` | `http://news-creator:11434` | news-creator URL (in-network DNS) |
| `HURL_IMAGE` | `ghcr.io/orange-opensource/hurl:7.1.0` | Hurl container |
| `IMAGE_TAG` | `main` | ghcr.io tag for alt-news-creator image |
| `GHCR_OWNER` | `kaikei-e` | GHCR namespace |
| `RUN_ID` | `$(date +%s)` | report directory suffix |
| `KEEP_STACK` | `0` | set to `1` to leave the stack up on exit |

Reports land under `e2e/reports/news-creator-<RUN_ID>/` (gitignored):
JUnit XML + HTML.

Debugging a failure:

```sh
KEEP_STACK=1 bash e2e/hurl/news-creator/run.sh
docker compose -f compose/compose.staging.yaml -p alt-staging \
  logs --tail=200 news-creator
docker compose -f compose/compose.staging.yaml -p alt-staging \
  logs --tail=200 news-creator-ollama-stub
```

## Scenario ordering

The suite runs serially (`--jobs 1`). HybridPrioritySemaphore state and
queue depth are shared, so even Phase 1's stateless reads benefit from
deterministic ordering — Phase 4 will rely on it for queue saturation.

## Scenarios

### 00 — Readiness probe (pre-flight)
- **Given** the staging stack is starting.
- **When** `GET :11434/health` is polled.
- **Then** it returns `{"status":"healthy","service":"news-creator"}`
  within 15 s (30 × 500 ms).

### 01 — /health schema
- **Given** news-creator is ready.
- **When** `GET /health` is called.
- **Then** the response is `application/json` with `status=healthy`,
  `service=news-creator`, and `models[]` containing at least the stub
  model name (`gemma3:4b-it-qat`). The `error` field MUST be absent
  because the stub responded 200 to `/api/tags`.

### 02 — /queue/status schema
- **Given** news-creator is ready and idle (no in-flight LLM work).
- **When** `GET /queue/status` is called.
- **Then** the response is `application/json` with
  `rt_queue=0`, `be_queue=0`, `total_slots>=1`, `available_slots>=1`,
  `accepting=true`, `max_queue_depth=10` (the value compose sets).

## Out of scope (deferred — see plan in this PR's description)

- All LLM-touching endpoints (Phase 2-4).
- Streaming response shapes (SSE for summarize, NDJSON for chat) —
  Phase 4. Hurl 7.1 reads chunked bodies to completion before
  asserting; per-chunk assertions are not supported and the stub will
  emit short deterministic streams (3-5 chunks) so `regex` / `matches`
  on the multiline body works.
- Queue saturation (HTTP 429 + `Retry-After: 30`) — Phase 4. Will rely
  on a stub admin endpoint to inject latency.
- Validation errors (content < 100 chars, prompt > 240 K chars,
  malformed JSON) — Phase 4.
- Real Ollama on a GPU runner — out of scope of the public CI gate.
  A `REAL_LLM=1` hybrid mode may follow Phase 4.
- mTLS strict mode (`PEER_IDENTITY_TRUSTED=on` + `X-Alt-Peer-Identity`)
  — staging matches mq-hub / tag-generator / knowledge-sovereign and
  runs unauthenticated.
- Distributed BE dispatch (`DISTRIBUTED_BE_ENABLED=true`) — production
  feature, off in staging.
- Pact contract verification — already covered by
  `news-creator/app/tests/contract/test_provider_verification.py`.

## References

- ADR-000763 — Hurl E2E pipeline adoption (search-indexer)
- ADR-000764 — mq-hub Hurl suite (Connect-RPC over HTTP/1.1+JSON)
- ADR-000765 — knowledge-sovereign Hurl suite (DB-backed strict serial)
- ADR-000766 — search-indexer run.sh + 3-service dispatch convention
- `news-creator/app/news_creator/handler/health_handler.py` — source of
  truth for `/health` and `/queue/status`
- `news-creator/app/news_creator/gateway/hybrid_priority_semaphore.py`
  — `queue_status()` shape
- `compose/news-creator-ollama-stub/app.py` — Ollama stub
