---
name: log-seeker
description: >-
  Investigates Alt's container logs and databases read-only to diagnose a live incident.
  Reads `docker compose logs`, the ClickHouse `rask_logs` store (otel_logs / http_logs /
  otel_traces / otel_http_requests / otel_error_logs / sli_metrics), and the PostgreSQL
  databases (alt-db, pre-processor-db, recap-db, rag-db, acolyte-db, kratos-db,
  knowledge-sovereign-db), correlates errors with trace_id and DB state, and returns an
  evidence-backed root-cause report. Use when the user says 「ログ調べて」「DB見て」
  「コンテナログ精査して」「なんで〇〇がエラー/落ちてる/遅い」, "check the logs", "why is X
  failing/crashing/slow", "investigate this incident", or right after an outage or alert.
  Prefer postmortem-writer once the cause is already known and the user wants the write-up
  rather than the investigation.
allowed-tools: Bash, Read, Grep, Glob
argument-hint: "[service-or-symptom] [--since=30m] [--deep]"
---

# log-seeker

Blameless triage → examine → diagnose → propose over the Alt stack, returning an evidence-backed
report. It never mutates anything.

## Scope & guardrails

- **Read-only. Always.** No writes, no DDL, no migrations, no `docker restart`/`up`/`down`,
  no `redis-cli SET/DEL`, no ClickHouse `ALTER`/`INSERT`. SQL is `SELECT`/`SHOW`/`EXPLAIN` only.
  If a fix is needed, *propose* it in the report — do not apply it.
- **Never print secret values.** Read passwords from `/run/secrets/*` *inside* the container only;
  never echo them, never paste them into a host command line. Derive DB names/users from container
  env (`$POSTGRES_USER`, `$CLICKHOUSE_DB`, …), not hardcoded literals.
- **Mask production hostnames/domains** in any quoted log or query output (project policy).
- **Stay inside the time window.** ClickHouse retention is ~1 day — querying older than ~24h returns
  nothing. Say so rather than guessing.

## Inputs

`$ARGUMENTS` may be a **service name** (`alt-backend`, `pre-processor`, `recap-worker`, …) → focus
there; a **symptom phrase** (`"knowledge home is empty"`, `"500s on /v1/feeds"`, `"OOM"`) → match it
to a recipe in [reference/recipes.md](reference/recipes.md); or **empty** → run a stack-wide health
pass, asking the user one focused question first if the symptom is unclear.

Flags: `--since=<dur>` (default `30m`; passed to `seek.sh` and `docker compose logs`),
`--deep` (widen the window, pull traces, run the reuse scripts listed in `reference/recipes.md`).

## Workflow

Copy this checklist into your reply and tick items as you go:

```
log-seeker progress
- [ ] 1. Frame the problem
- [ ] 2. Snapshot the stack (seek.sh)
- [ ] 3. Container logs
- [ ] 4. Aggregated logs / traces (ClickHouse)
- [ ] 5. Database state
- [ ] 6. Correlate & eliminate
- [ ] 7. Report
```

1. **Frame.** Restate the symptom, the affected service(s), and the time window. Note what changed
   recently — `git log --oneline -10`, recent deploy. Write down the working theories.
2. **Snapshot.** `docker compose -f compose/compose.yaml -p alt ps`, then run the bundler from the
   repo root:
   ```bash
   bash .claude/skills/log-seeker/scripts/seek.sh --since <window> <service...>
   ```
   It prints a summary and writes a bundle to `/tmp/log-seeker-<ts>/`. Read `SUMMARY.txt` first, then
   open `error-summary.txt`, `logs/<svc>.log`, `clickhouse-errors.txt`, `pg-health.txt`,
   `sovereign-health.txt`, `pgbouncer.txt`, `redis-streams.txt` only as the trail leads you there.
3. **Container logs.** For each suspect service:
   ```bash
   docker compose -f compose/compose.yaml -p alt logs --since=<window> --timestamps <svc>
   ```
   Grep for `ERROR|WARN|panic|fatal|traceback|OOMKilled|exit code`. Check restart counts /
   `unhealthy` from `docker compose ps`; if a container is flapping,
   `docker inspect --format '{{.RestartCount}} {{.State.OOMKilled}} {{.State.ExitCode}}' <container>`.
4. **Aggregated logs / traces (ClickHouse).** `rask-log-aggregator` is ingest-only and exposes no
   query API — query ClickHouse directly (connect recipe in
   [reference/targets.md](reference/targets.md)): error counts by service from `otel_error_logs`,
   recent error rows, request latency p50/p95/p99 from `otel_http_requests`, and trace lookups by
   `trace_id` across `otel_traces` + `otel_logs`. Pick the matching query from
   [reference/recipes.md](reference/recipes.md).
5. **Database state.** Identify the owning database from [reference/targets.md](reference/targets.md)
   — the two that matter most are **alt-db** (RSS working set, summaries, reports, `outbox_events`)
   and **knowledge-sovereign-db** (the immutable knowledge model: `knowledge_events`, projections,
   OODA-loop tables). Then check, **SELECT-only**: projector lag, Home freshness, event-log size,
   `outbox_events` backlog, suspect-table row counts / newest rows, and `pg_stat_activity` for stuck
   queries and locks. For pool pressure use the `pgbouncer` container logs plus `pg_stat_activity` on
   the backing DB — the pgbouncer admin console (`SHOW POOLS`) is not provisioned in this stack.
6. **Correlate & eliminate.** Line up: log error ↔ `trace_id` ↔ DB row/state ↔ timeline. Cross off
   theories the evidence rules out. If a runbook below covers the situation, follow it instead of
   improvising.
7. **Report.** Emit the template below. Blameless: talk about systems and signals, not people.

## Report template

```markdown
## Summary
<one line: what's wrong, since when, blast radius>

## Timeline
- <ts> <event / first error / deploy / spike>

## Evidence
- `<command run>` → <trimmed output, secrets & prod domains masked>

## Root-cause hypotheses (ranked)
1. <hypothesis> — supports: <…>; contradicts: <…>; confidence: <low/med/high>

## Recommended next actions
1. <read-only verification step>
2. <proposed fix — NOT applied; who/what would run it>

## Open questions
- <what we still can't see / needs the user>
```

## Bundled files

- `scripts/seek.sh` — **run** it at step 2 to collect the read-only bundle (ps, per-service logs,
  ClickHouse errors, alt-db + knowledge-sovereign-db health, pgbouncer, redis-streams).
- [reference/recipes.md](reference/recipes.md) — **read** the section matching the symptom for exact
  log/SQL/grep commands and the `scripts/analyze_*` diagnostics worth reusing.
- [reference/targets.md](reference/targets.md) — **read** when you need the topology: every DB and
  store, container/service names, host ports, env var names, key tables, and connect recipes.

## Runbooks (escalate to the canonical procedure)

- `docs/runbooks/knowledge-home-empty-spike.md` — Home returns empty
- `docs/runbooks/knowledge-home-malformed-why-spike.md` — corrupted `why` payloads
- `docs/runbooks/knowledge-home-reproject-operations.md` / `knowledge-loop-reproject.md` — projection rebuild
- `docs/runbooks/acolyte-checkpoint-resume.md` — Acolyte pipeline recovery
- `docs/runbooks/admin-observability.md` — metrics & admin UI
- `docs/runbooks/backup-restore.md` — DB backup/restore
