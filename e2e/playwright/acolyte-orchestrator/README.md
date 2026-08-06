# acolyte-orchestrator Playwright API suite

Black-box coverage for the versioned report-generation control plane — Python
3.14 / Starlette / Connect-RPC, one listener on `:8090`. Migrated from
`e2e/hurl/acolyte-orchestrator/`.

HTTP only: no spec touches `page`, `browser` or `context`, so Playwright never
launches a browser and the runner needs no browser binaries.

## Run

```bash
bash e2e/playwright/acolyte-orchestrator/run.sh

KEEP_STACK=1 bash e2e/playwright/acolyte-orchestrator/run.sh   # leave the stack up
PW_GREP='@smoke' bash e2e/playwright/acolyte-orchestrator/run.sh
```

`run.sh` is the only supported entry point. It renders an isolated compose
slice, brings the stack up with `--build`, installs dependencies on the default
bridge, runs the suite *inside* the staging network (which is `internal: true`,
so joining it is the only way the `acolyte-orchestrator` DNS name resolves),
and tears everything down. See `../_lib/suite.sh`.

Reports land in `e2e/reports/acolyte-orchestrator-<run_id>/`.

## Stack

| Service | Why it is in the slice |
|---|---|
| `acolyte-db` | Postgres 16, ephemeral, fresh per run. |
| `acolyte-db-migrator` | `atlas migrate apply`, then exits. The orchestrator's `service_completed_successfully` gate blocks uvicorn until it succeeds. |
| `acolyte-orchestrator` | The service under test. `MTLS_ENFORCE=false`, `PEER_IDENTITY_TRUSTED=off`, `CHECKPOINT_ENABLED=false`. |
| `news-creator-ollama-stub` | `NEWS_CREATOR_URL` points here. Acolyte's `OllamaGateway` calls `/api/generate` and `/api/chat`; the real news-creator FastAPI exposes neither. |
| `meilisearch` + `stub-backend` + `search-indexer` | Evidence retrieval for the gatherer node. Seeded by `setup/global-setup.ts`. |

## What the suite asserts

| File | Surface |
|---|---|
| `tests/health.spec.ts` | `GET /health` and the `HealthCheck` RPC; the peer-identity middleware's non-strict, untrusted-header behaviour. |
| `tests/connect-surface.spec.ts` | Every unary procedure is registered and answers from its own handler; unknown paths do not resolve; the Connect error envelope is decodable by a generated client. |
| `tests/reports-crud.spec.ts` | Create / read / delete, and the full `scope` ↔ `report_briefs` round-trip including the derived `P7D` window and CSV normalisation. |
| `tests/reports-list.spec.ts` | `ListReports` projection fields, `limit`, `hasMore`/`nextCursor`, cursor paging, and the default page size. |
| `tests/run-lifecycle.spec.ts` | `StartReportRun` → pipeline → `succeeded`, the committed version and sections, `activeRun` absence, `latestRunStatus` propagation, `RerunSection`, and the active-run guard. |
| `tests/errors.spec.ts` | Every `ConnectError` raised by `connect_service.py` — code *and* the HTTP status Connect pairs with it. |
| `tests/unimplemented.spec.ts` | The three declared-but-unimplemented procedures, including the streaming one's end-of-stream envelope. |
| `tests/topology.spec.ts` | The listener surface is `/health` + the Connect mount and nothing else; the pki-agent sidecar port is closed. |

Tags: `@smoke` (broken-deployment detectors), `@contract` (shapes a caller
depends on), `@authz` (negatives), `@slow` (drives the LangGraph pipeline).

## Where each retired Hurl scenario went

| Retired `.hurl` | Landed in |
|---|---|
| `00-setup.hurl` | `setup/global-setup.ts` — a readiness *gate*, not a test. `fullyParallel` has no notion of "run this one first", so a readiness check that lives in the suite is order-dependent by construction. Its two assertions (`status`, `service`) are also kept as a real test in `tests/health.spec.ts`. |
| `01-health-rpc.hurl` | `tests/health.spec.ts` — "HealthCheck RPC round-trips proto3 JSON". |
| `02-crud-no-scope.hurl` | `tests/reports-crud.spec.ts` — "a report with no scope is created, read back and deleted" (the whole chain, including the empty `ListReportVersions` page and the post-delete `not_found`). |
| `03-crud-with-scope.hurl` | `tests/reports-crud.spec.ts` — "topic and an unknown key survive the report_briefs round-trip". |
| `04-list-reports.hurl` | `tests/reports-list.spec.ts` — "returns the reports this test created", plus three new pagination tests the original never reached. |
| `10-start-and-poll-run.hurl` | `tests/run-lifecycle.spec.ts`, split into six tests: terminal status, the in-flight status check, the committed version/sections, `ListReportVersions`, `activeRun` absence and `RerunSection`. |
| `11-delete-during-active-run.hurl` | `tests/run-lifecycle.spec.ts` — "DeleteReport during an active run is refused or the run already finished". |
| `20-get-version-unimplemented.hurl` | `tests/unimplemented.spec.ts` — "GetReportVersion is unimplemented". |
| `21-diff-versions-unimplemented.hurl` | `tests/unimplemented.spec.ts` — "DiffReportVersions is unimplemented". |
| `22-stream-progress-unimplemented.hurl` | `tests/unimplemented.spec.ts` — "StreamRunProgress refuses inside the stream envelope". |
| `23-get-report-not-found.hurl` | `tests/errors.spec.ts` — "GetReport on an unknown but well-formed id is not_found". |
| `24-delete-report-malformed.hurl` | `tests/errors.spec.ts` — "DeleteReport with a non-UUID id is invalid_argument". |
| `25-rerun-section-empty-key.hurl` | `tests/errors.spec.ts` — "RerunSection with an empty section key is invalid_argument". |
| `run.sh`'s Meilisearch seed pre-step | `setup/global-setup.ts`, extended: the seed is now verified *through search-indexer's own API*, which is what the gatherer node actually reads. |

## What changed on the way across

**The ordering is gone.** The Hurl suite ran `--jobs 1` because captures are
file-scoped and scenarios 04-07 consumed a `report_id` produced by 02. Here
every test mints its own title from `testToken` and creates what it reads, so
four workers drive independent lifecycles through one acolyte-db. Nothing in
this suite needs a `serial` describe or a `workers: 1` project.

**The sleeps are gone.** `10`'s poll was `retry: 240` at `retry-interval: 5000`
— a twenty-minute ceiling paid in five-second granularity, for a pipeline that
converges in under a second against the stub. `waitForTerminalRun` asserts the
condition instead.

**`jsonpath "$" exists`-grade checks became schemas.** Every response is parsed
against a zod schema in `src/schemas.ts`, so a handler that changes shape fails
here instead of satisfying whichever three fields the old file happened to name.
The schemas encode proto3 JSON's zero-value omission explicitly — see the module
docstring for why `.optional()` there is the wire format rather than laxity.

**Registration is asserted as a property.** `src/procedures.ts` probes all
eleven unary procedures. It cannot use `_shared/connect.ts`'s
`expectProcedureMounted`, because that helper discriminates mounted-from-missing
by refusing 404 and four of these procedures answer 404 as their *correct*
business outcome — this service has no auth interceptor. The discriminator here
is the response body plus the exact expected Connect code.

## Deliberately out of scope

- Pact CDC (acolyte-orchestrator → news-creator, → search-indexer) lives in
  `acolyte-orchestrator/tests/contract/`, run with `uv run pytest`.
- mTLS-on and strict peer-identity paths. Staging runs `MTLS_ENFORCE=false` and
  `PEER_IDENTITY_TRUSTED=off`; `tests/topology.spec.ts` asserts the consequences
  of that configuration rather than pretending otherwise.
- LangGraph checkpointer resume (`CHECKPOINT_ENABLED=false` in the slice).
- LLM output content. The stub returns generic JSON instead of acolyte's `<plan>`
  XML and the pipeline reaches `succeeded` through its parser fallbacks, so
  asserting section bodies would couple this suite to prompt changes. Structural
  assertions (a section exists, `citationsJson` parses as an array) are made
  instead.
