# rag-orchestrator Playwright API suite

Black-box coverage for Alt's RAG control plane — Go 1.26, Echo REST on `:9010`
and a Connect-RPC mux on `:9011`. Migrated from `e2e/hurl/rag-orchestrator/`.

HTTP only: no spec touches `page`, `browser` or `context`, so Playwright never
launches a browser and the runner needs no browser binaries.

## Run

```bash
bash e2e/playwright/rag-orchestrator/run.sh

KEEP_STACK=1 bash e2e/playwright/rag-orchestrator/run.sh   # leave the stack up
PW_GREP='@smoke'  bash e2e/playwright/rag-orchestrator/run.sh
PW_GREP='@authz'  bash e2e/playwright/rag-orchestrator/run.sh
```

`run.sh` is the only supported entry point. It mints throwaway mutual-TLS
material, renders an isolated compose slice, brings the stack up with `--build`,
seeds the Augur fixtures through `psql`, installs dependencies on the default
bridge, runs the suite *inside* the staging network (which is `internal: true`,
so joining it is the only way the `rag-orchestrator` DNS name resolves), and
tears everything down. See `../_lib/suite.sh`.

Build the image first, or you are testing the previous one:

```bash
python3 e2e/playwright/_lib/build-images.py rag-orchestrator
```

Reports land in `e2e/reports/rag-orchestrator-<run_id>/`.

## Stack

| Service | Why it is in the slice |
|---|---|
| `rag-db` | Postgres 18 with pgvector baked in — same build context as prod (`../rag-db`), ephemeral, fresh per run. |
| `rag-db-migrator` | `atlas migrate apply`, then exits. rag-orchestrator's `service_completed_successfully` gate blocks the server until it succeeds. |
| `rag-orchestrator` | The service under test. `PEER_IDENTITY_MODE=disabled`, so `:9011` is plaintext h2c; every other upstream (`EMBEDDER_EXTERNAL`, `AUGUR_EXTERNAL`, `SEARCH_INDEXER_URL`, `QUERY_EXPANSION_URL`, `RERANK_URL`) is parked at `http://127.0.0.1:9`. |

`run.sh` still mints a client leaf even though nothing reaches the data plane:
`config.loadDataHub` panics on an unset `DATAHUB_MTLS_URL` and
`httpclient.NewDataHubClient` reads the CA at construction, because after
ADR-000954 D7 there is no plaintext route to `alt_db` to degrade onto.

## What the suite asserts

| File | Surface |
|---|---|
| `tests/health.spec.ts` | `/healthz`, `/readyz`, `/connect/health` bodies, and `/metrics` as Prometheus exposition text. |
| `tests/connect-surface.spec.ts` | Every procedure `SetupConnectHandlers` mounts is registered; the two streaming ones are mounted-but-wrong-codec (415); `MorningLetterReadService` does not resolve; the Connect error envelope is decodable by a generated client. |
| `tests/augur-authz.spec.ts` | The `X-Alt-User-Id` trust boundary: absent, malformed and whitespace-only all fail closed; one user can neither read, list nor **delete** another user's conversation. |
| `tests/augur-history.spec.ts` | The read paths against the seeded rows: the empty page, the derived `messageCount` / `lastMessagePreview` / `lastActivityAt`, message ordering, citation-kind reconstruction, the ADR-926 UUID-title fence, the `related_citations` snapshot, and every `not_found` / `invalid_argument` / delete-idempotency edge. |
| `tests/augur-pagination.spec.ts` | Keyset paging: ordering, `page_size`, the cursor round-trip, the full-final-page cursor, the 100-row cap, and both halves of `decodePageToken`'s validation. |
| `tests/rag-rest.spec.ts` | All seven REST routes are registered; the `query is required` gates; the four `backfill` validation gates and a real enqueue; the `X-Embedder-URL` allowlist (SSRF guard). |
| `tests/topology.spec.ts` | The two listeners serve disjoint surfaces, and nothing is bound on `:9443`. |

Tags: `@smoke` (broken-deployment detectors), `@contract` (shapes a caller
depends on), `@authz` (negatives). Nothing is `@slow` — no test in this suite
touches an LLM, an embedder or a search backend, which is what keeps the whole
run in seconds.

## Where each retired Hurl scenario went

| Retired `.hurl` | Landed in |
|---|---|
| `00-readiness.hurl` | `setup/global-setup.ts` — a readiness *gate*, not a test. Its `retry: 60` only worked because the Hurl runner was pinned to `--jobs 1`; `fullyParallel` has no notion of "run this one first". Its `$.status == "ready"` assertion is *also* kept as a real test in `tests/health.spec.ts`, so a body regression is a named failure rather than a 90-second timeout. |
| `01-healthz.hurl` | `tests/health.spec.ts` — "GET /healthz is a static liveness answer" (status, `Content-Type`, and the body against `healthzSchema`). |
| `02-connect-health.hurl` | `tests/health.spec.ts` — "GET /connect/health identifies the Connect listener" (both `status` and `service`). |
| `03-augur-missing-user-header.hurl` | `tests/augur-authz.spec.ts` — "… without X-Alt-User-Id is unauthenticated", now run against **all four** unary procedures instead of one, with the literal message still pinned. The registration half of the same fact is `tests/connect-surface.spec.ts`. |
| `04-augur-malformed-user-header.hurl` | `tests/augur-authz.spec.ts` — "… with a non-UUID X-Alt-User-Id is unauthenticated", also across all four, plus a new whitespace-only case that separates the trim branch from the parse branch. |
| `05-augur-list-empty.hurl` | `tests/augur-history.spec.ts` — "a user with no conversations gets the empty message". All three original assertions survive: `conversations` absent, `nextPageToken` absent, and the raw body equal to `{}`. |
| `06-augur-get-not-found.hurl` | `tests/augur-history.spec.ts` — "an unknown but well-formed id is not_found, not internal". The bare `jsonpath "$.code"` check is now a full `connectErrorSchema` parse plus the exact code. |
| `07-augur-delete-idempotent.hurl` | `tests/augur-history.spec.ts` — "an unknown id is idempotent, not not_found". `jsonpath "$.code" not exists` became a `.strict()` empty-object schema, which rejects *any* stray field rather than only `code`. |
| `08-augur-list-seeded.hurl` | `tests/augur-history.spec.ts` — "returns the seeded conversation with its derived counters" (all five original assertions) plus "lastActivityAt tracks the newest turn". |
| `09-augur-get-seeded.hurl` | `tests/augur-history.spec.ts` — "returns both turns in insertion order" (all six original assertions) plus three new citation tests the fixture was extended to support. |
| `run.sh`'s `psql` seed step | `run.sh` still runs it, now against `setup/db-seed.sql` in this directory, and `setup/global-setup.ts` verifies it landed **through `ListConversations`** before any test runs. |
| `run.sh`'s `--jobs 1` | Gone. See below. |

## What changed on the way across

**The ordering is gone — but not the seeding.** The Hurl suite ran `--jobs 1`
so that `00-readiness` could gate everything after it. That gate moved to
`globalSetup`. What could *not* move is the seed itself: Augur has no write RPC
a test could seed through — `StreamChat` is the only path that creates a
conversation and it drives the embedder, the LLM and search-indexer, none of
which exist in this slice. So the rows still arrive out of band, and the
parallel-safety the fleet requires is bought a different way: **one owner per
role**. Every `user_id` in `src/seed.ts` is exercised by exactly one group of
tests, the repository scopes every query by `user_id`, and the single
destructive test owns a conversation nothing else reads. Six workers therefore
never contend, and no spec needs a `serial` describe or a `workers: 1` project.

The one test whose pre-state cannot be re-established — the delete — asserts
only idempotent facts, so a CI retry re-runs it cleanly. That the row existed
beforehand is established once, for the whole run, by the seed probe in
`globalSetup`.

**`jsonpath "$.code"`-grade checks became schemas.** Every response is parsed
against a zod schema in `src/schemas.ts`. The schemas encode proto3 JSON's
zero-value omission explicitly — see the module docstring for why `.optional()`
there is the wire format rather than laxity.

**Registration is asserted as a property.** The Hurl suite touched two of the
six mounted procedures. `src/procedures.ts` enumerates all of them, plus the
ones that must *not* resolve, and `tests/connect-surface.spec.ts` discriminates
404 (never registered — a DI wiring failure, CLAUDE.md rule 8) from the
business answer. `_shared/connect.ts`'s `expectProcedureMounted` is not used
here: it cannot attach the per-call `X-Alt-User-Id` this service authenticates
with, and its band would have to hold both 401 (unary) and 415 (streaming)
without saying which procedure gets which.

**The whole REST surface arrived.** The Hurl README declared `/v1/rag/*` and
`/internal/rag/*` out of scope because they need news-creator, search-indexer
and alt-backend. That is true of the happy paths only: every route's bind guard
and validation gate runs in-process, so `tests/rag-rest.spec.ts` covers all
seven registrations, the `query is required` gates, `backfill`'s four
validation branches and a real enqueue, and the `X-Embedder-URL` SSRF allowlist
— none of which touch an upstream.

## Deliberately out of scope

- **Streaming RPC bodies.** `AugurService/StreamChat` and
  `MorningLetterService/StreamChat` both run the full RAG pipeline. Their
  *registration* is asserted (415 + `Accept-Post`, which is a mounted answer);
  their behaviour needs the model stack this slice does not run.
- **`/v1/rag/answer`, `/v1/rag/retrieve` and `/internal/rag/index/upsert` happy
  paths.** Same reason. Their validation and SSRF gates are covered.
- **Job execution.** `POST /internal/rag/backfill` is asserted up to the 202 and
  the job id. What the JobWorker then does with the row is deliberately not
  asserted: it fails against the unreachable embedder and grows a process-wide
  exponential backoff (`worker.go:126`), so a terminal-status assertion would
  make one test's result depend on how many others ran first.
- **`PEER_IDENTITY_MODE=mtls`.** The slice runs `disabled`; the suite asserts
  the consequences of that configuration (the listener answers cleartext, and
  `X-Alt-User-Id` is the entire identity) rather than pretending otherwise.
- **Pact CDC.** rag-orchestrator's consumer contracts live in
  `rag-orchestrator/internal/adapter/contract/` and run with `go test`.
