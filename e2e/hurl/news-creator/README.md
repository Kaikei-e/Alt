# news-creator — E2E specification

Fifth Hurl-driven end-to-end suite in the Alt monorepo, after
`search-indexer`, `mq-hub`, `knowledge-sovereign`, and `tag-generator`.
Covers the news-creator service (Python 3.14 / FastAPI / Ollama proxy +
HybridPrioritySemaphore + RAG helpers) — its HTTP contract surface plus
the queue / streaming / backpressure paths that Pact CDC alone cannot
pin.

news-creator's production backend is Ollama on an RTX 4060 GPU. CI has
no GPU, so the staging slice swaps Ollama for a tiny FastAPI stub
(`compose/news-creator-ollama-stub/app.py`) that answers `/api/tags`,
`/api/generate` (sync + NDJSON streaming), and `/api/chat` (sync +
NDJSON streaming) with fixed responses. Response shapes are chosen by
inspecting the request: `format` field present + schema with `sections`
+ `lead` → `MorningLetterContent`; other `format` schemas → `RecapSummary`;
`/api/chat` without `format` → `QueryPlan`; `/api/generate` without
`format` → multi-line ASCII (works for both summarize and expand-query).

## Phased rollout

- **Phase 1** ([[000768]]) — readiness gate + `/health` schema +
  `/queue/status` schema (`00`-`02`).
- **Phase 2** ([[000769]]) — summarize / generate / recap happy paths
  (`03`-`06`).
- **Phase 3** ([[000770]]) — RAG endpoints chat / expand-query /
  plan-query (`07`-`09`). `/v1/rerank` deferred (cross-encoder model
  download blocked by `internal: true` network).
- **Phase 4** (this commit) — streaming, validation errors, morning
  letter (`10`-`13`). Queue-saturation (HTTP 429) deferred to a
  follow-up Phase, since reliably triggering `QueueFullError` from a
  serial Hurl suite needs parallel client orchestration.

news-creator surface covered by the suite:

- `GET /health` — `{status, service, models[]}`
- `GET /queue/status` — HybridPrioritySemaphore depth + `accepting`
- `POST /api/v1/summarize` — non-stream (`03`) + SSE (`10`)
- `POST /api/generate` — Ollama pass-through (`04`)
- `POST /v1/summary/generate(/batch)` — recap structured output (`05`,`06`)
- `POST /api/v1/expand-query` — RAG query expansion (`07`)
- `POST /api/chat` — non-stream (`08`) + NDJSON stream (`11`)
- `POST /api/v1/plan-query` — Augur structured planning (`09`)
- `POST /v1/morning-letter/generate` — daily briefing (`13`)
- Validation guards on `/api/v1/summarize` and `/api/generate` (`12`)

Out of scope: `/v1/rerank` (HF model download), queue-saturation
(parallel-client orchestration), real Ollama on GPU, mTLS strict mode,
distributed BE.

## Prerequisites

Hurl 7.1.0+ on the host (the repo ships `hurl_7.1.0_amd64.deb`), plus
Docker Compose. Fixtures live under `e2e/fixtures/news-creator/`.

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

Wall-clock note: scenario `10-summarize-streaming.hurl` runs in ~10 s
because `summarize_handler.stream_with_heartbeat` keeps the SSE
connection alive for one full `heartbeat_interval` (10 s) after the
LLM stream closes — production behavior we don't suppress for the test.
The other 12 scenarios all complete in single-digit milliseconds, so
total wall-clock is dominated by container start + that one heartbeat
window.

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
queue depth are shared, so even read-only scenarios benefit from
deterministic ordering — the future queue-saturation work will rely
on it absolutely.

## Scenarios

### 00 — Readiness probe (pre-flight)
Polls `GET :11434/health` until 200 (30 × 500 ms).

### 01 — /health schema
`{status: healthy, service: news-creator, models[]}`. `models[0].name`
== stub's `gemma3:4b-it-qat`. `error` MUST be absent (stub responded
200 to `/api/tags`).

### 02 — /queue/status schema
Idle: `rt_queue=0`, `be_queue=0`, `total_slots>=1`, `accepting=true`,
`max_queue_depth=10`.

### 03 — POST /api/v1/summarize (non-streaming)
≥100-char fixture article. Asserts `{success, article_id, summary,
model}` and that `summary` is non-empty.

### 04 — POST /api/generate
Ollama pass-through. Asserts `{model, response, done, done_reason,
prompt_eval_count, eval_count, total_duration}`.

### 05 — POST /v1/summary/generate
recap-worker single-genre call. Asserts `{job_id, genre, summary:
{title, bullets[], language}, metadata: {model, summary_length_bullets,
is_degraded}}`.

### 06 — POST /v1/summary/generate/batch
Two-request bundle. Asserts `responses[]` count == 2 and `errors[]`
count == 0.

### 07 — POST /api/v1/expand-query
rag-orchestrator query expansion. Asserts `expanded_queries[]` count
>= 1 (stub returns 3 newline-separated lines, all pass the
instruction-leak filter).

### 08 — POST /api/chat (non-streaming)
chat-proxy non-stream path. Asserts `{model, message: {role, content},
done: true}`. The stub returns a `QueryPlan`-shaped JSON string in
`content`; the test only checks the envelope.

### 09 — POST /api/v1/plan-query
Augur structured planning. Asserts `plan: {reasoning, resolved_query,
search_queries[], intent, retrieval_policy, answer_format,
should_clarify, topic_entities[]}` plus `original_query` and `model`.

### 10 — POST /api/v1/summarize?stream=true (SSE)
`summarize_handler.stream_with_heartbeat` wraps each token in
`data: <json-encoded-token>\n\n`. Asserts `Content-Type:
text/event-stream`, `X-Accel-Buffering: no`, and that the body
contains the stub's tokens (`stub-token-alpha`, `stub-token-gamma`)
in the SSE framing.

### 11 — POST /api/chat?stream=true (NDJSON)
`chat_handler.ndjson_generator` re-serializes each upstream chunk via
`json.dumps + "\n"` (default separators with space after colon). Asserts
`Content-Type: application/x-ndjson`, the chunk envelope (`"message":`,
`"content":`), the stub tokens, and the final `"done": true`.

### 12 — Validation guard rails (3 sub-cases in one file)
- `/api/v1/summarize` with content < 100 chars → 400 with `detail`
  matching `(?i)content is too short`
- `/api/generate` with empty prompt → 422 (Pydantic min_length=1)
- `/api/v1/summarize` with malformed JSON body → 422 (FastAPI default)

### 13 — POST /v1/morning-letter/generate
Daily briefing. The usecase passes `format=MorningLetterContent.model_json_schema()`;
the stub recognizes `sections` + `lead` in the schema and returns a
matching JSON string. Asserts the full response envelope including
`content.sections[0].key` matching the morning-letter section grammar
and `metadata.summary_length_bullets >= 1`.

## Out of scope (deferred)

- **`/v1/rerank`** — `RerankUsecase` lazily loads
  `BAAI/bge-reranker-v2-m3` (~568 MB) from HuggingFace at first call;
  the `alt-staging` network's `internal: true` flag blocks the
  download. Adding this scenario requires bundling the model into the
  news-creator image (independent ADR).
- **HTTP 429 + `Retry-After: 30` queue-saturation** — triggering
  `QueueFullError` reliably from a serial Hurl suite needs parallel
  client orchestration (e.g. background bash jobs, `--parallel`-ed
  Hurl invocations) and a stub admin endpoint to inject latency.
  Deferred to a follow-up Phase.
- **Real Ollama on GPU runner** — out of scope of the public CI gate.
  A `REAL_LLM=1` hybrid mode may follow once the suite stabilises.
- **mTLS strict mode** (`PEER_IDENTITY_TRUSTED=on` +
  `X-Alt-Peer-Identity`) — staging matches mq-hub / tag-generator /
  knowledge-sovereign and runs unauthenticated.
- **Distributed BE dispatch** (`DISTRIBUTED_BE_ENABLED=true`) — production
  feature, off in staging.
- **Pact contract verification** — already covered by
  `news-creator/app/tests/contract/test_provider_verification.py`.

## References

- ADR-000763 — Hurl E2E pipeline adoption (search-indexer)
- ADR-000764 — mq-hub Hurl suite (Connect-RPC over HTTP/1.1+JSON)
- ADR-000765 — knowledge-sovereign Hurl suite (DB-backed strict serial)
- ADR-000766 — search-indexer run.sh + 3-service dispatch convention
- ADR-000768 — news-creator Hurl E2E Phase 1
- ADR-000769 — news-creator Hurl E2E Phase 2
- ADR-000770 — news-creator Hurl E2E Phase 3
- ADR-000771 — news-creator Hurl E2E Phase 4 (this PR)
- `news-creator/app/news_creator/handler/health_handler.py` — source of
  truth for `/health` and `/queue/status`
- `news-creator/app/news_creator/handler/summarize_handler.py` — SSE
  framing
- `news-creator/app/news_creator/handler/chat_handler.py` — NDJSON
  framing
- `news-creator/app/news_creator/gateway/hybrid_priority_semaphore.py`
  — `queue_status()` shape
- `compose/news-creator-ollama-stub/app.py` — Ollama stub
