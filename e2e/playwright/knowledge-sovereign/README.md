# knowledge-sovereign — Playwright API E2E suite

End-to-end scenarios for Alt's single owner of durable knowledge state
(ADR-000532): the append-only `knowledge_events` log, the read models projected
from it, lenses, snapshots and retention.

This **replaces** the Hurl suite that used to live at
`e2e/hurl/knowledge-sovereign/`. It follows the same dispatch convention
([ADR-000766](../../../docs/ADR/000766.md)), so `bash e2e/<framework>/<svc>/run.sh`
remains the contract alt-deploy's `release-deploy.yaml` probes.

## The stack

Two containers plus a migrator, from the `knowledge-sovereign` profile of
`compose/compose.staging.yaml`:

| Service | Role |
|---|---|
| `knowledge-sovereign-db` | Postgres 16 |
| `knowledge-sovereign-db-migrator` | Atlas, run to completion (30 migrations) |
| `knowledge-sovereign` | the service, built from `../knowledge-sovereign` |

No stubs and no mutual TLS: this service has no upstreams. It owns its database
and does its own projecting in-process.

Two listeners, both plain HTTP inside the `internal: true` staging network:

| Port | Surface |
|---|---|
| `:9500` | Connect-RPC JSON — `services.sovereign.v1.KnowledgeSovereignService` (46 procedures) — plus `GET /health` |
| `:9501` | operator — `GET /health`, `GET /metrics`, and `/admin/{snapshots,retention,storage}/*` |

The staging slice runs with `ADMIN_AUTH=disabled`, so `/admin/*` answers
unauthenticated. That is asserted explicitly in `tests/topology.spec.ts` rather
than left implied; production supplies `ADMIN_TOKEN_FILE` via
`compose/sovereign.yaml`, where an unset token is a startup failure.

## Running

```bash
# Full suite: bring the slice up, install deps, run, tear down
bash e2e/playwright/knowledge-sovereign/run.sh

# One shard of two
SHARD=1/2 PW_BLOB=1 bash e2e/playwright/knowledge-sovereign/run.sh

# A subset
PW_GREP="@smoke" bash e2e/playwright/knowledge-sovereign/run.sh

# Skip the minute-long observability wait
PW_GREP="^(?!.*@slow)" bash e2e/playwright/knowledge-sovereign/run.sh

# Debug: leave the stack up
KEEP_STACK=1 bash e2e/playwright/knowledge-sovereign/run.sh
```

Reports land under `e2e/reports/knowledge-sovereign-<RUN_ID>/`.

### Against an already-running stack

```bash
cd e2e/playwright/knowledge-sovereign
BASE_URL=http://localhost:19510 METRICS_URL=http://localhost:19511 npx playwright test
```

`19510`/`19511` are the host publishes in `compose.staging.yaml`. `src/env.ts`
has no defaults, so a forgotten variable is a named error rather than a
connection refused on the first spec (CLAUDE.md rule 9).

### Checking your work without Docker

```bash
cd e2e/playwright && npx tsc --noEmit -p knowledge-sovereign/tsconfig.json
cd knowledge-sovereign && BASE_URL=http://x:9500 METRICS_URL=http://x:9501 npx playwright test --list
```

## Parallelism and isolation

`fullyParallel: true`, `workers: 4`. The ceiling is the service's **pgx pool**,
not CPU: `main.go` builds it with no `pool_max_conns`, so pgxpool defaults to
`max(4, NumCPU)`, and four in-process background loops already draw on it (two
projectors on a 2s tick in staging, the partition maintainer, the
projection-health exporter).

Isolation comes from **naming, not teardown**. Every test gets its own
`principal.userId` from a test-scoped fixture, and every read in the suite is
scoped by it. That is what lets assertions like "exactly one lens" or "exactly
one event" survive parallel execution — they are claims about the driver's
`WHERE` clause, not about the order files ran in. `tenantId` is worker-scoped
so the cross-tenant negative has a real neighbour to be excluded from.

A second project, `admin` (`workers: 1`), holds the three `admin-*.spec.ts`
files. `GET /admin/snapshots/latest` answers "the newest valid snapshot in the
database", which two concurrent snapshot writers would make a coin flip. Each
admin test still seeds its own event and its own snapshot, so the project is
order-independent — the single worker constrains *concurrency*, not sequence.

## What the Hurl suite needed `--jobs 1` for, and where it went

Its `run.sh` said: *"CreateLens → CreateLensVersion → Select → ListLenses, and
AppendKnowledgeEvent → CreateSnapshot, require strict ordering."* Both chains
are gone:

- The **lens chain** became `seedLens()` in `tests/lenses.spec.ts` — the FK from
  `knowledge_lens_versions` to `knowledge_lenses` is real, so the version still
  needs its lens, but that is now two lines inside the test that needs it
  instead of four files that must run in order.
- The **snapshot chain** became `seedSnapshot()` in `src/admin.ts`.
  `CreateSnapshot` genuinely refuses when `max(event_seq)` is 0, so each admin
  test appends an event of its own first.
- The **shared identity set** minted in `run.sh` and threaded through twenty
  scenarios with `--variable` became the per-test `principal` fixture.

Nothing in this suite is ordered. There is no `test.describe.configure({ mode:
"serial" })` anywhere in it.

## Scenario map

Every retired `.hurl` file and where its assertions landed.

| Retired scenario | Landed in | Note |
|---|---|---|
| `00-setup` (health, retry) | `setup/global-setup.ts` | A readiness gate, not a test — `fullyParallel` has no "run this first". Strengthened: also waits for `GetActiveProjectionVersion` to answer `active`, because `/health` is an unconditional 200 that stays green with the database on fire. |
| `01-health-metrics-port` | `health.spec.ts` › *GET /health on the operator listener*, *both listeners serve the identical health document* | The file's stated intent ("same schema as :9500") is now an actual comparison of the two documents. |
| `02-rpc-projection-version` | `projection-infra.spec.ts` › *GetActiveProjectionVersion returns the seeded version 1* | `jsonpath "$.version.description" exists` → a non-empty-string schema; `activated_at` added. |
| `03-event-append-happy` | `events.spec.ts` › *AppendKnowledgeEvent returns the assigned event_seq* | `matches "^[0-9]+$"` accepted `"0"`, which is the driver's "duplicate, nothing written" signal. Now `> 0`. |
| `04-event-latest-seq` | `events.spec.ts` › *GetLatestEventSeq returns the seq of the event just appended* | The Hurl file **captured** `event_seq` in 03 and never compared it — it only re-asserted the regex. Now an equality. |
| `05-event-list-tenant-scoped` | `events.spec.ts` › *ListKnowledgeEvents returns exactly the principal's event, payload intact* | All six assertions kept; whole-envelope schema and a base64 payload round trip added. |
| `06-home-items-empty` | `projections.spec.ts` › *GetKnowledgeHomeItems returns nothing* | Three `not exists` checks → one whole-document comparison. Moved onto a principal that genuinely has no events (see **Why 06–08 were wrong**). |
| `07-today-digest-empty` | `projections.spec.ts` › *GetTodayDigest returns no digest* | Same. |
| `08-recall-candidates-empty` | `projections.spec.ts` › *GetRecallCandidates returns nothing* | Same. |
| `09-lens-create` | `lenses.spec.ts` › *CreateLens answers an empty response and the lens becomes listable* | Asserted HTTP 200 only; the response message has no fields, so 200 was all Hurl could see. Verified by effect now. |
| `10-lens-version-create` | `lenses.spec.ts` › *GetLens returns the lens with its current version* | Asserted HTTP 200 only and never read the version back. Every field now round-trips. |
| `11-lens-list` | `lenses.spec.ts` › *CreateLens … becomes listable* | All four field assertions kept; `currentVersion` from the LATERAL join added. |
| `12-lens-select-current` | `lenses.spec.ts` › *SelectCurrentLens then GetCurrentLensSelection round-trips* | All four assertions kept. |
| `13-recall-signal-append` | `recall-signals.spec.ts` › *AppendRecallSignal persists a signal that ListRecallSignals can read* | Asserted HTTP 200 only. Read-back added. |
| `14-admin-snapshot-create` | `admin-snapshots.spec.ts` › *records the full immutable-design envelope* | All nine assertions kept; checksum regex tightened to 64 hex; `event_seq_boundary > 0` → `>=` the event this test appended; seven unasserted fields covered by the schema. |
| `15-admin-snapshot-latest` | `admin-snapshots.spec.ts` › *returns a valid snapshot no older than the one just created* | Self-seeds so the comparison is real. |
| `16-admin-retention-eligible` | `admin-retention.spec.ts` › *returns the flat, table-tagged envelope* | `isCollection` → a per-row schema. The file's reason for not asserting a count (hardcoded monthly partitions, date drift) is respected: the array may still be empty. |
| `17-admin-retention-run-dry` | `admin-retention.spec.ts` › *dry_run:true plans without archiving* | Both assertions kept; `actions` (a nullable array — no `omitempty`) and per-action status added. |
| `18-admin-storage-stats` | `admin-storage.spec.ts` › *reports typed, table-level metrics* | Five `exists` checks — all true of `null` — became a typed schema plus `total_bytes` / `is_partitioned`, which the Hurl file never looked at. |
| `19-admin-snapshot-list` | `admin-snapshots.spec.ts` › *is wrapped in a snapshots object* | ADR-000942 wrapper fence kept; ordering (by `event_seq_boundary`, not by time) added. |
| `20-admin-retention-status` | `admin-retention.spec.ts` › *is wrapped in a logs object*, *a dry run appends no rows to the retention log* | `logs count == 0` was a claim about the fixture. Replaced with the causal claim: a dry run never reaches `logAction`, asserted before/after. |

### Why 06–08 were wrong, not merely weak

Those three scenarios asserted that a projection was **empty**, and their
comments explained the intent: *"Phase 1 scope validates the structure contract
without requiring projector wiring"*. That was true when they were written. It
is not true now — `main.go` runs `knowledge_home_projector` and
`knowledge_trail_projector` in-process on a ticker.

So the suite appended an event for a user in scenario 03 and asserted the same
user's Knowledge Home was empty in scenario 06. It passed for a reason nobody
intended: the payload it sent, `{"title":"hurl-e2e"}`, carries no `article_id`,
`foldArticleCreated`'s `uuid.Parse` fails, and `RunBatch` stops without
advancing the checkpoint — so the projector jammed on that event and retried it
every two seconds for the life of the stack. **A green suite was recording a
wedged projector.**

The empty-projection assertions are kept here, moved onto principals that
genuinely have no events. The positive direction — the one CLAUDE.md rule 8
cares about — is added alongside.

## New coverage

Grounded in the source, not invented. The file each addition came from is named
in a comment at the assertion.

| Area | What was missing | Where |
|---|---|---|
| **Connect registration** | 34 of 46 procedures had no assertion of any kind. `SovereignHandler` embeds `UnimplementedKnowledgeSovereignServiceHandler`, so a missing method still *compiles* and answers 501 from a generated stub — a silent fallback with the compiler's blessing. 404-vs-501-vs-business-answer is the discriminator. | `connect-surface.spec.ts` |
| **Input validation** | 28 procedures have an explicit `parseUUIDField` / nil-message guard whose whole purpose is that an empty `user_id` must never become `uuid.Nil` and scope a read to the wrong principal. None was exercised. | `connect-surface.spec.ts` |
| **Listener topology** | Nothing asserted that `/admin/*` and `/metrics` are absent from :9500, or that the Connect service is absent from :9501. The mux split is the *only* access control the operator surface has. | `topology.spec.ts` |
| **Admin method binding** | The handlers register Go 1.22 method-aware patterns. A lost `POST ` prefix would make `GET /admin/snapshots/create` create a snapshot — a destructive side effect on the verb crawlers and prefetch issue freely. 405-vs-404 is the discriminator. | `topology.spec.ts` |
| **Admin auth posture** | `ADMIN_AUTH=disabled` was implied by every admin call quietly working. Now stated, with a bogus Bearer token to prove the gate is *off* rather than accepting anything. | `topology.spec.ts` |
| **Projector wiring** | The single biggest gap. Seeded events now converge into `knowledge_home_items`, `today_digest_view`, `recall_candidate_view` and the Trail spine, and the checkpoint is asserted to advance — which distinguishes a *stuck* projector from an idle one. | `projections.spec.ts` |
| **Observability** | `/metrics` untouched despite the port being named for it. Includes the `knowledge_event_last_occurrence_age_seconds` gauge, whose presence is the only external proof the `projection_health` exporter goroutine was wired at all (a `promauto` GaugeVec publishes no series until `Set` is called). | `health.spec.ts` |
| **Tenant boundary** | Deferred by the Hurl README. Cross-tenant reads, `user_id` without `tenant_id`, and the deliberate `user_id IS NULL` tenant-wide admission. | `events.spec.ts` |
| **Idempotency** | `knowledge_event_dedupes` exists so at-least-once producers can resend (CLAUDE.md rule 10). A duplicate answers seq 0, which protojson omits — the wire answer is `{}`. Nothing tested it. | `events.spec.ts` |
| **Cursor edges** | `after_seq` and `limit` were only ever `0` and `10`. Every projector and stream consumer resumes on `event_seq > $1`. | `events.spec.ts` |
| **Lens lifecycle** | `ArchiveLens`, `ClearCurrentLens`, `ResolveLensFilter`, `GetLens`, the `SelectCurrentLens` upsert branch — all unexercised. `ResolveLensFilter` is the read every Knowledge Home request makes first. | `lenses.spec.ts` |
| **Projection infrastructure** | Checkpoints, freshness, lag, reproject runs, backfill jobs, audits, `CompareProjections` — the plumbing a reproject depends on, none of it tested. | `projection-infra.spec.ts` |
| **Retention safety** | `DryRun *bool` is a pointer so an *absent* field defaults to the safe path. Changing it to a plain `bool` — the obvious simplification, invisible to a unit test of `RunRetention` — would make an empty POST to an unauthenticated endpoint archive the event log for real. | `admin-retention.spec.ts` |
| **Signal windowing** | `since_days` had no test; a window that ignored it would weigh a year-old glance like this morning's read. | `recall-signals.spec.ts` |

## Known gaps pinned by this suite

These assert **current** behaviour. When the underlying issue is fixed the test
fails, which is the intended signal.

| Where | What |
|---|---|
| `admin-snapshots.spec.ts` | `CreateSnapshot` never sets `meta.CreatedAt`, so the create response carries Go's zero time (`0001-01-01T00:00:00Z`) while the stored row carries `DEFAULT now()`. A client trusting the response would date every snapshot to the year 1. |
| `lenses.spec.ts` | `CreateLensVersion` for a non-existent lens is a foreign-key violation — a caller error — but the handler wraps every driver error in `connect.CodeInternal`, so it surfaces as a page-an-operator 500 instead of `failed_precondition`. |
| `schemas.ts` (`embeddedRecallItemSchema`) | The `item` embedded in a recall candidate is not a `KnowledgeHomeItem`: `GetRecallCandidates` fills only the columns its LEFT JOIN selected, so `tenantId` is the all-zero UUID and `projectionVersion` is 0. A consumer must not read either off it. |
| `admin-snapshots.spec.ts` | `config.Load` hardcodes `SchemaVersion: "00009"` while `knowledge-sovereign/migrations/` has reached `00030`. The assertion is deliberately "a non-empty string" — pinning `"00009"` would guard a value that is already wrong — and the drift is recorded here instead. |

## Not covered

- **`WatchProjectorEvents` semantics.** Registration is asserted; the stream
  itself is not. `APIRequestContext` buffers a response, so a long-lived
  server-streaming RPC with a 3s heartbeat needs a different client than the
  rest of this fleet uses.
- **`/admin/retention/run` with no valid snapshot.** The Hurl README deferred
  it and it stays deferred: the refusal is only observable before any snapshot
  exists, which is not a state a self-seeding suite can arrange without a
  global ordering constraint — precisely the thing this migration removed.
- **A real (`dry_run: false`) retention run.** It detaches and archives
  partitions of the append-only log. The dry-run planner, the safety gate and
  the log-write boundary are asserted instead.
- **Closed-port assertions** (`expectConnectionRefused` /
  `expectTlsHandshakeRejected` from `_shared/net.ts`). `main.go` binds exactly
  two servers and there is no third port, no plaintext-next-to-mTLS pair, and
  no operator-only socket to prove shut. The equivalent claim here is a
  cross-port 404, which `topology.spec.ts` makes for every route on both
  listeners.
- **Cross-service flow.** `mq-hub → projector → sovereign` is a multi-service
  suite distinct from this one; the Pact contract lives in
  `pacts/recap-worker-knowledge-sovereign.json`.
