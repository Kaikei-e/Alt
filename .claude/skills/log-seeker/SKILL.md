---
name: log-seeker
description: >-
  Investigates Alt's container logs and databases read-only to diagnose incidents.
  Reads `docker compose logs`, the ClickHouse `rask_logs` store (otel_logs / http_logs /
  otel_traces / otel_http_requests / otel_error_logs / sli_metrics), and the PostgreSQL
  databases (alt-db, pre-processor-db, recap-db, rag-db, acolyte-db, kratos-db,
  knowledge-sovereign-db), correlates log errors with trace_id and DB state, and returns
  an evidence-backed root-cause report. Use when the user says 「ログ調べて」「DB見て」
  「コンテナログ精査して」「なんで〇〇がエラー/落ちてる/遅い」, "check the logs", "why is X
  failing/crashing/slow", "investigate this incident", or right after an outage or alert.
allowed-tools: Bash, Read, Grep, Glob
argument-hint: "[service-or-symptom] [--since=30m] [--deep]"
---

# log-seeker

Read-only investigator for the Alt stack: container logs (`docker compose logs`),
the ClickHouse log/trace store, and the PostgreSQL databases. It runs a structured,
blameless troubleshooting pass (triage → examine → diagnose → propose) and returns an
evidence-backed report. It never mutates anything.

## Scope & guardrails

- **Read-only. Always.** No writes, no DDL, no migrations, no `docker restart`/`up`/`down`,
  no `redis-cli SET/DEL`, no ClickHouse `ALTER`/`INSERT`. SQL is `SELECT`/`SHOW`/`EXPLAIN`
  only. If a fix is needed, *propose* it in the report — do not apply it.
- **Never print secret values.** Read passwords from `/run/secrets/*` *inside* the container
  only; never echo them, never paste them into a host command line. Derive DB names/users
  from container env (`$POSTGRES_USER`, `$CLICKHOUSE_DB`, …), not hardcoded literals.
- **Mask production hostnames/domains** in any quoted log or query output (project policy).
- **Stay inside the time window.** ClickHouse retention is ~1 day — querying older than ~24h
  returns nothing; say so rather than guessing.
- **One external-call rule:** if a step needs an external API call, keep ≥5s between calls.
  (Most of this skill is local; this rarely applies.)

## Inputs

`$ARGUMENTS` may be:
- a **service name** (`alt-backend`, `pre-processor`, `recap-worker`, …) → focus there,
- a **symptom phrase** (`"knowledge home is empty"`, `"500s on /v1/feeds"`, `"OOM"`) → match it
  to a recipe in [reference/recipes.md](reference/recipes.md),
- **empty** → run a stack-wide health pass; if the symptom is unclear, ask the user one
  focused question before digging.

Flags: `--since=<dur>` (default `30m`; passed through to `seek.sh` and `docker compose logs`),
`--deep` (widen the window, pull traces, and run the reuse scripts in `reference/recipes.md`).

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

1. **Frame.** Restate the symptom, the affected service(s), and the time window. Note what
   changed recently — `git log --oneline -10`, recent deploy. Write down the working theories.
2. **Snapshot.** `docker compose -f compose/compose.yaml -p alt ps`, then run the bundler:
   ```bash
   bash ${CLAUDE_SKILL_DIR}/scripts/seek.sh --since <window> <service...>
   ```
   It prints a summary and a bundle directory under `/tmp/log-seeker-<ts>/`. Read the summary
   first; open `error-summary.txt`, `logs/<svc>.log`, `clickhouse-errors.txt`, `pg-health.txt`,
   `pgbouncer.txt` only as the trail leads you there.
3. **Container logs.** For each suspect service:
   ```bash
   docker compose -f compose/compose.yaml -p alt logs --since=<window> --timestamps <svc>
   ```
   Grep for `ERROR|WARN|panic|fatal|traceback|OOMKilled|exit code`. Check restart counts /
   `unhealthy` from `docker compose ps`; if a container is flapping,
   `docker inspect --format '{{.RestartCount}} {{.State.OOMKilled}} {{.State.ExitCode}}' <container>`.
4. **Aggregated logs / traces (ClickHouse).** The `rask-log-aggregator` is ingest-only — query
   ClickHouse directly (connect recipe in [reference/targets.md](reference/targets.md)):
   error counts by service from `otel_error_logs`, recent error rows, request latency
   p50/p95/p99 from `otel_http_requests`, and trace lookups by `trace_id` across
   `otel_traces` + `otel_logs`. Pick the matching query from [reference/recipes.md](reference/recipes.md).
5. **Database state.** Identify the owning database from [reference/targets.md](reference/targets.md):
   **alt-db** = RSS working set + summaries + reports + `outbox_events`;
   **knowledge-sovereign-db** = the immutable knowledge model (`knowledge_events`,
   `knowledge_home_items`, `knowledge_projection_checkpoints`, `today_digest_view`,
   `recall_candidate_view`, OODA-loop tables); **recap-db / rag-db / acolyte-db /
   pre-processor-db / kratos-db** = their own domains. Check, **SELECT-only**: projector lag
   (`knowledge_projection_checkpoints` on knowledge-sovereign-db), Home freshness
   (`knowledge_home_items`), event-log size, `outbox_events` backlog (alt-db), suspect-table row
   counts / newest rows, and `pg_stat_activity` for stuck/long queries and locks. For pool
   pressure use the `pgbouncer` container logs + `pg_stat_activity` on the backing DB — the
   pgbouncer admin console (`SHOW POOLS`) is not provisioned in this stack.
6. **Correlate & eliminate.** Line up: log error ↔ `trace_id` ↔ DB row/state ↔ timeline. Cross
   off theories the evidence rules out. If a documented runbook covers the situation, follow it
   (see **Runbooks** below) instead of improvising.
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

## Reference

- Symptom-specific recipes (exact log/SQL/grep commands, reuse of `scripts/analyze_*`):
  [reference/recipes.md](reference/recipes.md)
- Full topology — every DB and store, container/service names, ports, env var names,
  key tables, and connect recipes: [reference/targets.md](reference/targets.md)

## Runbooks (escalate to the canonical procedure)

- `docs/runbooks/knowledge-home-empty-spike.md` — Home returns empty
- `docs/runbooks/knowledge-home-malformed-why-spike.md` — corrupted `why` payloads
- `docs/runbooks/knowledge-home-reproject-operations.md` / `knowledge-loop-reproject.md` — projection rebuild
- `docs/runbooks/acolyte-checkpoint-resume.md` — Acolyte pipeline recovery
- `docs/runbooks/admin-observability.md` — metrics & admin UI
- `docs/runbooks/backup-restore.md` — DB backup/restore
