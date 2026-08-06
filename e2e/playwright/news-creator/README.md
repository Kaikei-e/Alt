# news-creator — Playwright API E2E

Replaces `e2e/hurl/news-creator/`. HTTP-only: no spec touches `page`,
`browser` or `context`, so Playwright never launches a browser.

news-creator is Alt's LLM front door — a Python 3.14 / FastAPI service that
proxies Ollama behind a `HybridPrioritySemaphore` and adds the structured-output
paths its callers need (recap-worker's genre summaries, rag-orchestrator's
query expansion and planning, alt-backend's Morning Letter). It owns no
database and persists nothing, so every test here is a request/response
contract.

## The stack

`run.sh` renders the `news-creator` profile of `compose/compose.staging.yaml`
into an isolated slice and brings up two containers:

| Service | What it is |
|---|---|
| `news-creator` | the service under test, one uvicorn process on `:11434` |
| `news-creator-ollama-stub` | `compose/news-creator-ollama-stub/app.py` — a FastAPI stand-in for Ollama on `:11435` |

Production's backend is Ollama on an RTX 4060. CI has no GPU, so the stub
answers `/api/tags`, `/api/generate` and `/api/chat` (sync and NDJSON
streaming) with fixed payloads, choosing the shape from the request's `format`
field: a schema with `sections` + `lead` → `MorningLetterContent`, any other
schema → `RecapSummary`, `/api/chat` without `format` → `QueryPlan`,
`/api/generate` without `format` → three lines of ASCII.

The slice runs unauthenticated (`MTLS_ENFORCE=false`,
`PEER_IDENTITY_TRUSTED=off`) with `WARMUP_ENABLED=false`,
`MODEL_ROUTING_ENABLED=false`, `CACHE_ENABLED=false`, `OTEL_ENABLED=false`,
`MAX_QUEUE_DEPTH=10` — the same configuration the Hurl suite ran against.

## Running

```bash
bash e2e/playwright/news-creator/run.sh          # stack up, tests, teardown
KEEP_STACK=1 bash e2e/playwright/news-creator/run.sh
PW_GREP='@smoke'  bash e2e/playwright/news-creator/run.sh
```

Without Docker, the two checks that catch most of what goes wrong in a
rewrite:

```bash
cd e2e/playwright && npx tsc --noEmit -p news-creator/tsconfig.json
cd e2e/playwright/news-creator && \
  BASE_URL=http://x:11434 UNBOUND_URL=http://x:8001 \
  STUB_MODEL=stub MAX_QUEUE_DEPTH=10 npx playwright test --list
```

## What it covers

| Surface | Spec |
|---|---|
| `GET /health` | `health.spec.ts` |
| `GET /queue/status` | `queue.spec.ts`, `queue-drain.spec.ts` |
| `POST /api/v1/summarize` | `summarize.spec.ts`, `streaming.spec.ts` |
| `POST /api/generate` | `generate.spec.ts` |
| `POST /v1/summary/generate(/batch)` | `recap-summary.spec.ts` |
| `POST /api/v1/expand-query`, `/api/v1/plan-query`, `/v1/rerank` | `rag.spec.ts` |
| `POST /api/chat` | `chat.spec.ts`, `streaming.spec.ts` |
| `POST /v1/morning-letter/generate` | `morning-letter.spec.ts` |
| `GET /openapi.json`, `GET /metrics` | `routing.spec.ts` |
| unproxied Ollama admin API, listener topology | `topology.spec.ts` |

## Where each retired Hurl scenario landed

| Hurl scenario | Landed in |
|---|---|
| `00-setup.hurl` | `setup/global-setup.ts` — readiness is a gate, not a test |
| `01-health-schema.hurl` | `health.spec.ts` › "reports healthy, names the service and lists models" |
| `02-queue-status.hurl` | `queue.spec.ts` (the load-independent half) + `queue-drain.spec.ts` (the exact-idle half) |
| `03-summarize-non-stream.hurl` | `summarize.spec.ts` › "returns a summary and echoes the caller's article_id" |
| `04-generate.hurl` | `generate.spec.ts` › "answers the Ollama-shaped envelope" |
| `05-recap-summary.hurl` | `recap-summary.spec.ts` › "generates a Japanese recap for one genre" |
| `06-recap-batch.hurl` | `recap-summary.spec.ts` › "batch preserves request order and reports no errors" |
| `07-expand-query.hurl` | `rag.spec.ts` › "expands a query and echoes the original" |
| `08-chat-non-stream.hurl` | `chat.spec.ts` › "proxies a non-streaming completion" |
| `09-plan-query.hurl` | `rag.spec.ts` › "returns a structured retrieval plan" |
| `10-summarize-streaming.hurl` | `streaming.spec.ts` › "frames every token as an SSE data event" |
| `11-chat-streaming.hurl` | `streaming.spec.ts` › "emits one JSON object per line, terminated by done" |
| `12-validation-errors.hurl` | split across `summarize.spec.ts` (short content, malformed JSON) and `generate.spec.ts` (empty prompt) |
| `13-morning-letter.hurl` | `morning-letter.spec.ts` › "generates a letter from recap summaries" |

## What is new, and why

Everything below was uncovered by the Hurl suite. Each is grounded in a file
in `news-creator/app/`, named in the spec's comments.

- **Route registration** (`routing.spec.ts`). main.py makes nine
  `app.include_router` calls; a lost one turns an endpoint into a 404 that
  reads like "no data". `/openapi.json` is checked path-and-method against the
  `@router.*` decorators. This is CLAUDE.md rule 8 at the E2E boundary — the
  FastAPI counterpart of alt-backend's `connect-surface.spec.ts`.
- **`/metrics`** (`routing.spec.ts`). A Starlette *mount*, so it is invisible
  to the OpenAPI check. A lost mount is a silent-observability failure.
- **`/v1/rerank`** (`rag.spec.ts`). The Hurl suite deferred it because the
  cross-encoder is a 568MB HuggingFace download the `internal: true` network
  blocks. Pydantic validates before the handler body runs, so an invalid
  request proves the route is mounted without touching the model.
- **The unproxied Ollama admin API** (`topology.spec.ts`). `/api/pull`,
  `/api/delete`, `/api/embeddings` and friends must 404. A "forward the whole
  Ollama API" refactor would expose model management on the GPU host.
- **The `__main__` port** (`topology.spec.ts`). main.py's development block
  binds 8001, the image binds 11434; `expectConnectionRefused` pins that,
  which Hurl could not express at all.
- **Semaphore slot accounting** (`queue.spec.ts`). `available_slots +
  acquired_slots` versus `total_slots` is the invariant behind the class's own
  leak watchdog. From a parallel worker it is asserted as a band —
  `total_slots - 1 <= sum <= total_slots` — because `release()` documents a
  legal "in transit" reading while a woken waiter has not yet re-tracked the
  slot it was handed (`hybrid_priority_semaphore.py:630-643`). The strict
  equality is asserted at rest, in `queue-drain.spec.ts`. `acquired_slots` was
  never read by the Hurl suite.
- **Concurrent queueing and drain** (`queue-drain.spec.ts`). The
  parallel-client orchestration the Hurl README explicitly deferred. Note that
  this is the *queueing* half only: three concurrent requests against
  `MAX_QUEUE_DEPTH=10` never trip `QueueFullError`, so the 429 +
  `Retry-After: 30` backpressure path the Hurl suite deferred is **still not
  gated by E2E** — closing it needs a slice with `MAX_QUEUE_DEPTH=1` and no
  concurrently-running sibling project, since the semaphore is process-global.
  The header of `queue.spec.ts` records the reasoning.
- **The morning-letter degraded branch** (`morning-letter.spec.ts`), and
  `is_degraded` / `model != "extractive-fallback"` on the happy path. Every
  failure in that usecase answers **200** with an extractive fallback, so
  status alone cannot tell a working pipeline from a broken one.
- **Boundary validation** grounded in `domain/models.py`: unknown `priority`
  values, strict-mode type rejection, empty `clusters` / `messages` /
  `candidates` / `requests`, out-of-range `max_bullets` / `japanese_count` /
  `top_k`, a non-UUID `job_id`, a missing `overnight_groups`, whitespace-only
  content, and the 240,000-character prompt cap on both sides of the boundary
  (the cap was named in a Hurl comment but never exercised).
- **405-not-404** on every endpoint, which distinguishes "wrong verb" from
  "router never included".

## Parallelism

`workers: 4`, sized against the **HybridPrioritySemaphore**, not CPU:
`total_slots` is `OLLAMA_NUM_PARALLEL` defaulted to 1, and the ceiling above
it is `MAX_QUEUE_DEPTH=10`. The arithmetic that keeps worst-case queue depth
under that ceiling is in `playwright.config.ts`.

The Hurl suite ran `--jobs 1`. That was necessary for the semaphore's global
counters and for nothing else: no scenario consumed a capture from another.
`queue-drain.spec.ts` — the only spec that reads those counters — gets a
`workers: 1` project of its own and still asserts convergence with
`eventually` rather than a point reading. Every other spec mints its own
`article_id` / `job_id` / query and asserts the echo, which is a stronger
claim than the Hurl fixtures' shared constants could make.

`10-summarize-streaming.hurl`'s `--delay`-shaped wall-clock cost is unchanged
and unavoidable: `stream_with_heartbeat`'s heartbeat task is parked in a 10s
sleep and the response loop waits for both sentinels, so the SSE body closes
~10s after the last token. That is production behaviour, so the suite timeout
is 45s rather than the fleet default of 30s.
