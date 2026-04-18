# acolyte-orchestrator Hurl E2E suite

Black-box coverage for the versioned report-generation control plane
(Python 3.14 / Starlette / Connect-RPC, port 8090). Boots a dedicated
PostgreSQL via Atlas migration, then exercises the Connect-RPC surface
against the running orchestrator inside the `alt-staging` Docker
network.

Convention follows ADR-000763 (Hurl framework), ADR-000764 (Connect-RPC
over HTTP/1.1+JSON), ADR-000765 (DB-backed services use `--jobs 1`).

## Run

```bash
bash e2e/hurl/acolyte-orchestrator/run.sh
```

Reports land in `e2e/reports/acolyte-orchestrator-<run_id>/{junit.xml,html/}`.

`KEEP_STACK=1` skips teardown for log inspection.

## Stack

`run.sh` activates the `acolyte-orchestrator` Compose profile, which
brings up:

- `acolyte-db` — Postgres 16, ephemeral, fresh per run.
- `acolyte-db-migrator` — runs `atlas migrate apply` once and exits;
  the orchestrator's `service_completed_successfully` gate blocks
  startup until migration succeeds.
- `acolyte-orchestrator` — uvicorn on :8090, mTLS off, peer-identity
  unauthenticated, checkpointer disabled.

Hurl runs inside `alt-staging` (the network is `internal: true`, which
silently drops host port publishes — joining the network is the only
portable way to reach the SUT).

## Scenarios

All Connect-RPC requests use `POST /alt.acolyte.v1.AcolyteService/<Method>`
with headers `Content-Type: application/json` and
`Connect-Protocol-Version: 1`. proto3-JSON wire format is camelCase;
proto3 zero-defaults are omitted from responses. Hurl `[Captures]` are
file-scoped, so each lifecycle chain (create → read → delete) lives in
a single `.hurl` file.

| File | What it proves |
|---|---|
| `00-setup.hurl` | `GET /health` retries until uvicorn binds and the Atlas-migrated DB pool is warm (30×500ms). Asserts `status == "ok"`. |
| `01-health-rpc.hurl` | Connect-RPC `HealthCheck` returns `{"status":"ok"}`. Proves the route table is wired and proto3-JSON round-trips. |
| `02-crud-no-scope.hurl` | Full CRUD lifecycle for a no-scope report: `CreateReport` → `GetReport` (asserts ISO 8601 createdAt, proto3-omitted currentVersion) → `ListReportVersions` (empty collection) → `DeleteReport` → `GetReport` returns Connect `not_found` (HTTP 404). |
| `03-crud-with-scope.hurl` | Same lifecycle for a report with `scope.topic` set, exercising the `report_briefs` insert path at `connect_service.py:71-74`. Asserts `report.scope.topic` and `report.scope.dateRange` are echoed back. |
| `04-list-reports.hurl` | Creates two reports, asserts both ids and titles appear in `ListReports`, cleans up. |

## Out of scope

- Async run lifecycle (`StartReportRun` / `GetRunStatus` / `RerunSection`)
  — depends on news-creator + search-indexer; covered separately.
- Pact CDC (acolyte-orchestrator → news-creator, → search-indexer) is
  in `acolyte-orchestrator/tests/contract/`, run via `uv run pytest`.
- mTLS-on path coverage. Staging runs `MTLS_ENFORCE=false` and
  `PEER_IDENTITY_TRUSTED=off`.
- LangGraph checkpointer resume. Staging sets `CHECKPOINT_ENABLED=false`.
