# tag-generator — Playwright API E2E suite

End-to-end scenarios for tag-generator (Python 3.14 / FastAPI / KeyBERT over a
multilingual MiniLM SBERT): its HTTP contract on `:9400`, its request
validation, its authentication boundary, and the Redis Streams round trip that
mq-hub's `GenerateTagsForArticle` RPC wraps synchronously.

This **replaces** the Hurl suite at `e2e/hurl/tag-generator/`. It keeps the
same dispatch convention ([ADR-000766](../../../docs/ADR/000766.md)):
`bash e2e/<framework>/<svc>/run.sh` remains the contract, and alt-deploy's
`release-deploy.yaml` e2e matrix probes both framework directories.

## The stack

`run.sh` renders the `tag-generator` profile of `compose/compose.staging.yaml`
into an isolated slice and brings up four services:

| Service | Why it is in the slice |
|---|---|
| `tag-generator` | the service under test, FastAPI on `:9400` |
| `redis-streams` | `alt:events:tags` and the per-request reply streams |
| `mq-hub` | Connect-RPC on `:9500`; the only way an HTTP suite can drive a Redis Streams consumer |
| `stub-backend` | nginx answering 200, satisfying `BACKEND_API_URL` reachability at startup |

tag-generator runs with `MTLS_ENFORCE=false`, `PEER_IDENTITY_TRUSTED=off`,
`HF_HUB_OFFLINE=1` and `TRANSFORMERS_OFFLINE=1` — the last two because the
staging network is `internal: true`, so a `HEAD huggingface.co` on model load
would hang until `warmup()` raised and left `_background_tag_service` as
`None` forever.

## Running

```bash
# Full suite: install deps, bring the slice up, run, tear down
bash e2e/playwright/tag-generator/run.sh

# A subset
PW_GREP="@smoke" bash e2e/playwright/tag-generator/run.sh

# Debug: leave the stack up for inspection
KEEP_STACK=1 bash e2e/playwright/tag-generator/run.sh
docker compose -p alt-staging-tag-generator logs tag-generator
```

Reports land under `e2e/reports/tag-generator-<RUN_ID>/`.

The script expects the tag-generator and mq-hub images to exist as
`ghcr.io/${GHCR_OWNER:-kaikei-e}/alt-{tag-generator,mq-hub}:${IMAGE_TAG:-ci}`.
Locally:

```bash
docker build -t ghcr.io/kaikei-e/alt-tag-generator:ci \
  -f tag-generator/Dockerfile.tag-generator tag-generator
docker build -t ghcr.io/kaikei-e/alt-mq-hub:ci -f mq-hub/Dockerfile mq-hub
```

> `run.sh` forwards `IMAGE_TAG` into **both** `TAG_GENERATOR_IMAGE_TAG` and
> `MQ_HUB_IMAGE_TAG`. The Hurl script forwarded only the former, so a CI job
> that had just built `alt-mq-hub:ci` ran the stack against
> `alt-mq-hub:main`.

### Iterating without Docker

Every endpoint is read from the environment, so the suite runs against an
already-running stack with no container at all:

```bash
cd e2e/playwright/tag-generator
BASE_URL=http://localhost:19400 \
MQHUB_BASE_URL=http://localhost:19500 \
TLS_SIDECAR_URL=https://localhost:19443 \
npx playwright test --ui
```

## What the readiness gate does

`setup/global-setup.ts` is where `00-setup.hurl` went, and it absorbed two
more things the Hurl suite expressed as ordering:

1. `/health` returns 200 as soon as FastAPI's lifespan has *spawned* the
   background thread, so `/api/v1/extract-tags` answers `503 Tag extraction
   service not ready` for a window afterwards. The gate polls until a real
   inference is served (budget: 5 minutes — `TagExtractor.warmup()` has been
   seen past 20 s on a 2-core runner).
2. That first inference **is** the SBERT warm-up scenario 03 existed to
   provide for scenario 04. It is a property of the stack, not of a test, so
   it happens once, before anything runs.
3. A zero-inference probe through mq-hub proves tag-generator's
   `redis-streams-tags-consumer` thread has actually joined its group — the
   window scenario 04 covered with `retry: 3`. It sends a request with no
   `articleId`, which the consumer's Pydantic validator rejects before
   touching the model, so the reply comes back immediately.

Once the gate returns, **no test depends on any other**, and `--jobs 1` is
gone.

## Coverage

6 spec files, 34 tests.

### Retired Hurl scenarios

| Hurl scenario | Where it landed |
|---|---|
| `00-setup.hurl` | `setup/global-setup.ts` (readiness gate, not a test) |
| `01-health-schema.hurl` | `tests/health.spec.ts` — "GET /health reports healthy and names the service" |
| `02-extract-tags-smoke.hurl` | `tests/extract-tags.spec.ts` — "a minimal payload returns a well-formed extraction" |
| `03-extract-tags-keyword.hurl` | `tests/extract-tags.spec.ts` — "keyword-dense English text yields tags"; its warm-up role moved to global setup |
| `04-generate-tags-via-mqhub.hurl` | `tests/stream-round-trip.spec.ts` — "a request round-trips through the consumer and comes back with tags" |
| `05-extract-tags-missing-title.hurl` | `tests/extract-tags-validation.spec.ts` — "a missing title is rejected with a field-level error" |
| `06-extract-tags-missing-content.hurl` | `tests/extract-tags-validation.spec.ts` — "a missing content is rejected with a field-level error" |
| `07-extract-tags-empty-strings.hurl` | `tests/extract-tags.spec.ts` — "empty strings degrade gracefully instead of erroring" |

Nothing was dropped. Three assertions were strengthened rather than ported
literally:

- `jsonpath "$.language" exists` became `language === "und"` on the
  no-signal arms. `"und"` is not a langdetect output — it is written by
  exactly the two early returns in `extract_tags_with_metrics`, and it is the
  only externally visible signal that the pipeline *declined* rather than
  *ran and found nothing*.
- `jsonpath "$.tags" isCollection` on the mq-hub round trip became an
  assertion on the element shape (`{id: uuid, name, confidence}`), which is
  what mq-hub's `parseReply` and every downstream consumer actually depend on.
- `detail[0].loc isCollection` became "the error names the field that was
  wrong", which is the part a caller acts on.

`extractTagsSchema` additionally pins two cross-field invariants that no
per-JSONPath assertion can express: `confidence > 0` exactly when `tags` is
non-empty, and `language === "und"` implies `tags`, `confidence` and
`inference_ms` are all zero.

### New coverage

| Area | What was missing |
|---|---|
| Route registration | `/openapi.json` is generated from the live route table; a deleted decorator or a module that failed to import now fails here rather than in production |
| `/api/v1/generate-tags`, `/api/v1/user-preferences` | never probed at all — both now asserted to be mounted *and* to refuse anonymous callers |
| Peer-identity forgery | a self-written `X-Alt-Peer-Identity` must not unlock anything |
| mTLS sidecar port | asserted closed, so the plaintext trust model is checked rather than assumed |
| Japanese pipeline | `_detect_language` → the fugashi/unidic path was completely uncovered |
| Sanitizer branches | below-minimum-length input, the repetition DoS guard, HTML/script stripping |
| Validation | non-string and null fields, non-JSON and absent bodies, and that both missing fields report *both* errors |
| Determinism | the same text must extract the same tags — the property that makes re-tagging an article reprojection-safe |
| Stream reply arms | the `success=false` / `error_message` arm, reached with an over-long `article_id` |
| Correlation-id routing | two concurrent round trips must each get their own reply — impossible to test under `--jobs 1` |
| Consumer liveness | `/health` re-checked after a round trip, since it reports 503 once a consumer thread dies |

### Parallelism

`workers: 2`, sized against the service and not the runner. tag-generator is
one uvicorn process with `OMP_NUM_THREADS=1`, runs extraction through
`asyncio.to_thread`, and handles `alt:events:tags` on a single consumer
thread. Two workers let the cheap validation and routing tests overlap the ML
ones; more would only make each ML test slower by the same factor.

No test seeds durable state — `/api/v1/extract-tags` is a pure function of its
body and mq-hub deletes its own reply streams — so isolation reduces to one
rule: **every round-trip test mints its own `articleId`**, as a UUID, because
`TagGenerationRequestPayload.article_id` is `Field(max_length=36)` and a
longer token would silently land every test on the validation arm.

## Known behaviour pinned by this suite

These assert **current** behaviour, and each carries its reasoning inline.
When the underlying issue is fixed, the test fails — which is the signal.

| Where | What |
|---|---|
| `tests/authz.spec.ts` | `/api/v1/extract-tags` authenticates nobody on `:9400`. `verify_service_token` is a documented no-op; reachability is the entire control |
| `tests/authz.spec.ts` | the two authenticated routes answer 422 *or* 503 — the fail-closed `require_auth` wrapper is reached only when FastAPI's own validation of the wrapped signature passes first |
| `tests/route-surface.spec.ts` | `/docs` is served, because `FastAPI(...)` is constructed without `docs_url=None` |

## Out of scope

- **Oversized content.** `_truncate_content` clips at 100 000 characters
  rather than rejecting, so a meaningful test would post a 100 KB body and pay
  for KeyBERT candidate generation over all of it.
- **`ArticleCreated` round trip.** Needs an observable side effect — a real
  alt-backend in the slice, or metrics tag-generator does not export.
- **mTLS peer-identity enforcement.** `strict=False` in this slice; the
  positive direction needs the nginx sidecar, which the slice deliberately
  does not run.
- **Consumer crash / XAUTOCLAIM reclaim.** Requires killing a thread inside
  the container, which is a chaos test rather than an E2E one.
