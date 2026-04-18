# knowledge-sovereign — E2E specification

Third Hurl-driven end-to-end suite in the Alt monorepo (after
`search-indexer` and `mq-hub`), covering the Knowledge Sovereign service
defined in ADR-000532 — Alt's single owner of durable knowledge state
(`knowledge_events`, projections, lenses, snapshots, retention).

The sovereign service exposes:

- **Connect-RPC JSON on `:9500`** — `services.sovereign.v1.KnowledgeSovereignService`
  (`POST /{fqn}/{method}` with `Content-Type: application/json`,
  `Connect-Protocol-Version: 1`, proto3-JSON body).
- **Plain HTTP admin + health on `:9501`** —
  `GET /health`, `POST /admin/snapshots/create`,
  `GET /admin/snapshots/{list,latest}`,
  `POST /admin/retention/run`, `GET /admin/retention/{status,eligible}`,
  `GET /admin/storage/stats`.

## Prerequisites

Hurl 7.1.0+ on the host (the repo ships `hurl_7.1.0_amd64.deb`), plus
Docker Compose. No Python fixtures: all request bodies are inline and
templated with `--variable` at run time.

The staging stack is brought up by `run.sh` automatically. Manual:

```sh
docker compose -f compose/compose.staging.yaml -p alt-staging \
  --profile knowledge-sovereign \
  up -d --wait knowledge-sovereign-db knowledge-sovereign
```

Ports published for host-local debugging: `19510:9500` (RPC),
`19511:9501` (admin + health). The `alt-staging` network is
`internal: true`, so Hurl must run inside it — `run.sh` handles that.

## Running

```sh
bash e2e/hurl/knowledge-sovereign/run.sh
```

Env overrides:

| Var | Default | Purpose |
|-----|---------|---------|
| `BASE_URL` | `http://knowledge-sovereign:9500` | Connect-RPC endpoint (in-network DNS) |
| `METRICS_URL` | `http://knowledge-sovereign:9501` | admin + health endpoint |
| `HURL_IMAGE` | `ghcr.io/orange-opensource/hurl:7.1.0` | Hurl container |
| `RUN_ID` | `$(date +%s)` | isolates `dedupe_key`, lens names across parallel runs |
| `KEEP_STACK` | `0` | set to `1` to leave the stack up on exit |

`run.sh` generates a fresh UUID set per run (`tenant_id`, `user_id`,
`event_id`, `lens_id`, `lens_version_id`, `signal_id`) and injects them
via `--variable`. Hurl does not template `file,…;` bodies so scenarios
embed JSON inline with `{{var}}` substitution.

Reports land under `e2e/reports/knowledge-sovereign-<RUN_ID>/`
(gitignored): JUnit XML + HTML.

Debugging a failure:

```sh
KEEP_STACK=1 bash e2e/hurl/knowledge-sovereign/run.sh
docker compose -f compose/compose.staging.yaml -p alt-staging \
  exec knowledge-sovereign-db psql -U sovereign knowledge_sovereign \
  -c "SELECT event_seq, tenant_id, user_id, event_type, dedupe_key
      FROM knowledge_events ORDER BY event_seq DESC LIMIT 5;"
```

## Scenario ordering

The suite runs serially — lens creation depends on the `run.sh`-seeded
`lens_id` being unique, snapshot creation depends on the event appended
by scenario 03, and retention-run checks for a valid snapshot. Parallel
execution would break the immutable-design invariants it's supposed to
verify.

## Scenarios

### 00 — Readiness probe (pre-flight)

- **Given** the staging stack is starting.
- **When** `GET :9500/health` is polled.
- **Then** it returns `{"status":"ok","service":"knowledge-sovereign"}`
  within 15 s (30 × 500 ms).

### 01 — Health schema on metrics port

- **Given** the service is ready.
- **When** `GET :9501/health` is called.
- **Then** the metrics port returns the same schema as the RPC port.
  (Container healthcheck targets `:9501`; `:9500` has no bound
  healthcheck.)

### 02 — GetActiveProjectionVersion (RPC baseline)

- **Given** migration `00001_initial_schema.sql` seeds projection
  version 1 as `active`.
- **When** `GetActiveProjectionVersion` is called.
- **Then** the response returns `version.version == 1` and
  `version.status == "active"`.

### 03 — AppendKnowledgeEvent (happy path, captures event_seq)

- **Given** a freshly generated tenant/user UUID pair and a run-scoped
  `dedupe_key`.
- **When** `AppendKnowledgeEvent` is called with a full `ArticleCreated`
  event payload.
- **Then** the response returns a numeric `event_seq` (string-encoded,
  per proto3-JSON int64 rules). Captured for scenario 04.

### 04 — GetLatestEventSeq

- **Given** scenario 03 has persisted an event for (tenant, user).
- **When** `GetLatestEventSeq` is called with the same `tenant_id` and
  `user_id`.
- **Then** the response returns the same `event_seq`.

### 05 — ListKnowledgeEvents (tenant-scoped)

- **Given** (tenant, user) from scenarios 03–04.
- **When** `ListKnowledgeEvents` is called with `after_seq: 0`,
  `tenant_id`, `user_id`.
- **Then** the response contains exactly one event matching the
  `event_id`, `event_type`, and `dedupe_key` from scenario 03.
  (ADR-000749 tenant boundary; cross-tenant rejection is Phase 2.)

### 06 — GetKnowledgeHomeItems (empty for fresh user)

- **Given** a fresh user with no projected items.
- **When** `GetKnowledgeHomeItems` is called.
- **Then** the response returns `has_more: false` and zero items.
  (Projector wiring is out of scope; event → projection mapping belongs
  to the `mq-hub → projector` E2E.)

### 07 — GetTodayDigest (no digest row for fresh user)

- **Given** a fresh user with no `today_digest_view` row.
- **When** `GetTodayDigest` is called for today's date.
- **Then** the `digest` field is absent (proto3 omits nil messages).

### 08 — GetRecallCandidates (empty for fresh user)

- **Given** a fresh user with no `recall_candidate_view` rows.
- **When** `GetRecallCandidates` is called.
- **Then** the `candidates` list is empty.

### 09 — CreateLens

- **Given** a client-assigned `lens_id`.
- **When** `CreateLens` is called.
- **Then** HTTP 200 with empty body (the response message has no fields).

### 10 — CreateLensVersion

- **Given** the lens from 09.
- **When** `CreateLensVersion` is called referencing that `lens_id`.
- **Then** HTTP 200. The version is required before 12 because
  `knowledge_current_lens.lens_version_id` has an FK into
  `knowledge_lens_versions`.

### 11 — ListLenses

- **When** `ListLenses` is called with the run's `user_id`.
- **Then** the response contains one lens matching `lens_id`,
  `user_id`, `tenant_id`, and the generated name.

### 12 — SelectCurrentLens → GetCurrentLensSelection

- **Given** the lens + version from 09–10.
- **When** `SelectCurrentLens` is called, then `GetCurrentLensSelection`.
- **Then** the selection is persisted and readable:
  `found: true`, `selection.lens_id` and `selection.lens_version_id`
  match the seeded values.

### 13 — AppendRecallSignal

- **When** `AppendRecallSignal` is called with a `view` signal tied to
  the run's `user_id`.
- **Then** HTTP 200 with empty body.

### 14 — POST /admin/snapshots/create

- **Given** at least one event in `knowledge_events` (from 03).
- **When** `POST /admin/snapshots/create` is called on the metrics port.
- **Then** the response records:
  - `SnapshotType: "full"`, `Status: "valid"`
  - `EventSeqBoundary > 0`
  - `ProjectorBuildRef: "staging"` (matches the compose env)
  - `SchemaVersion` present
  - `ItemsChecksum`, `DigestChecksum`, `RecallChecksum` are all
    `sha256:<hex>`.
- The admin handler marshals `sovereign_db.SnapshotMetadata` via
  `encoding/json` without tags, so field names are PascalCase.

### 15 — GET /admin/snapshots/latest

- **When** `GET /admin/snapshots/latest` is called.
- **Then** the response matches the snapshot created in 14.

### 16 — GET /admin/retention/eligible

- **When** `GET /admin/retention/eligible` is called.
- **Then** the response is a 2-element array covering
  `knowledge_events` and `knowledge_user_events`. The `eligible` arrays
  are expected empty on a fresh stack — the contract under test is the
  shape, not the policy cutoff.

### 17 — POST /admin/retention/run (dry-run)

- **Given** a valid snapshot exists (from 14).
- **When** `POST /admin/retention/run` is called with
  `{"dry_run": true}`.
- **Then** the response has `dry_run: true` and no `error` field. The
  actions list may be empty.

### 18 — GET /admin/storage/stats

- **When** `GET /admin/storage/stats` is called.
- **Then** the response is a JSON array (contents depend on DB state).

## Out of scope (deferred)

- **ListKnowledgeEvents cross-tenant reject** — ADR-000749 negative path.
- **AppendKnowledgeEvent invalid payload** — 4xx / InvalidArgument.
- **ArchiveLens** — removes lens from `ListLenses`.
- **/admin/retention/run without snapshot** — 500 with explicit error.
- **AreArticlesVisibleInLens** — bulk visibility probe.
- **WatchProjectorEvents** — server-streaming; Hurl's streaming support
  is limited.
- **JWT authentication** — staging bypasses auth; ADR-000749's full
  JWT/tenant binding is covered by service-level unit tests and a
  future gateway-level E2E.
- **End-to-end projector flow** — requires `mq-hub → projector →
  sovereign`, which is a multi-service suite distinct from this one.

## References

- ADR-000532 — Knowledge Sovereign service definition
- ADR-000749 — tenant/user/lens boundary hardening
- ADR-000763 — Hurl E2E pipeline adoption
- ADR-000764 — mq-hub Hurl Phase 2 (profile pattern precedent)
- `docs/info/plan/knowledge-sovereign-bounded-context.md`
- `proto/services/sovereign/v1/sovereign.proto`
