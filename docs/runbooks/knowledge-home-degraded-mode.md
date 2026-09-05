---
title: "Knowledge Home Degraded Mode"
date: 2026-03-18
tags:
  - runbook
  - knowledge-home
  - alt-backend
---

# Knowledge Home Degraded Mode

Runbook for investigating and resolving degraded Knowledge Home responses.

Related: [[000418]], [[knowledge-home-projection-recovery]]

> **Note:** `knowledge_events`, `knowledge_home_items`, and
> `knowledge_projection_checkpoints` live exclusively in
> `knowledge-sovereign-db` (database `knowledge_sovereign`, user
> `sovereign`) — see [[knowledge-home-projection-recovery]]'s Model section.
> The projector runs inside the `knowledge-sovereign` service, not
> `alt-backend`. Commands below target `alt-knowledge-sovereign-db-1`
> accordingly; `alt-db` no longer has these tables.

## Alerts

| Alert | Severity | Condition |
|-------|----------|-----------|
| `KnowledgeHomeDegradedResponseHigh` | ticket | > 5% degraded for 10m |
| `KnowledgeHomeAvailabilityBurnRateHigh` | page | 14.4x burn rate (5m+1h) |
| `KnowledgeHomeAvailabilityBurnRateElevated` | ticket | 6x burn rate (30m+6h) |

## Symptoms

- `KnowledgeHomeDegradedResponseHigh` or `KnowledgeHomeAvailabilityBurnRate*` alert fires.
- Users see amber/orange degraded banner on Knowledge Home.
- `alt_home_degraded_responses_total` counter is climbing.
- Knowledge Home shows stale items or partial content.

## Investigation

### 1. Check projection freshness

```bash
docker exec alt-knowledge-sovereign-db-1 \
  psql -U sovereign -d knowledge_sovereign -P pager=off -c \
  "SELECT projector_name, last_event_seq, updated_at,
     EXTRACT(EPOCH FROM (now() - updated_at)) AS lag_seconds
   FROM knowledge_projection_checkpoints;"
```

If `lag_seconds` is high, the projector is behind. See [[knowledge-home-projection-recovery]].

### 2. Check knowledge-sovereign and alt-backend logs

```bash
docker compose -f compose/compose.yaml -p alt logs knowledge-sovereign --since=15m 2>&1 | grep -i "projector\|degraded\|batch failed"
docker compose -f compose/compose.yaml -p alt logs alt-backend --since=15m 2>&1 | grep -i "knowledge\|degraded"
```

Look for:
- `knowledge_home_projector batch failed` (knowledge-sovereign) -- projector stuck on a poison-pill event; see [[knowledge-home-projection-recovery]].
- `connection refused` -- knowledge-sovereign cannot reach `knowledge-sovereign-db`.
- alt-backend's `alt_home_degraded_responses_total` counter incrementing confirms degraded responses are actually being served (the code path is `DegradedMode: result.Degraded` in `orchestrator/connect/v2/knowledge_home/home_query.go`).

### 3. Check database connectivity

`knowledge-sovereign` dials `knowledge-sovereign-db:5432` directly -- there is
no connection pooler on this path (PgBouncer fronts `alt-db` and Kratos
only, per [[knowledge-home-projection-recovery]]'s Notes). Check the
sovereign DB's own connection count instead of PgBouncer:

```bash
docker exec alt-knowledge-sovereign-db-1 \
  psql -U sovereign -d knowledge_sovereign -c \
  "SELECT state, count(*) FROM pg_stat_activity GROUP BY state;"
```

### 4. Check the feature flag surface

`GetKnowledgeHomeResponse.feature_flags` (`orchestrator/gateway/feature_flag_gateway`)
is config-based, not a runtime toggle read from logs. Check the resolved
config alt-backend started with rather than grepping for a log line:

```bash
docker compose -f compose/compose.yaml -p alt logs alt-backend --since=1h 2>&1 | grep -i "feature_flag"
```

## Resolution

### Stale projection

1. Check `knowledge-sovereign` logs for projector errors (see Investigation
   step 2 above).
2. If the projector is stuck on a poison-pill event, do **not** just
   restart -- follow the poison-pill triage in
   [[knowledge-home-projection-recovery]] first.
3. If restarting is appropriate (e.g. a transient DB disconnect):
   ```bash
   docker compose -f compose/compose.yaml -p alt restart knowledge-sovereign
   ```
4. If projection state itself is corrupt, follow [[knowledge-home-projection-recovery]]'s Recovery Procedure.
5. There is no `/v1/admin/knowledge-home/backfill` REST route -- the admin
   surface is a Connect-RPC service alt-backend exposes internally on
   :9102, driven by `altctl home backfill trigger`. **That command
   currently fails immediately** (`ErrNoBackfillExecutor`: no executor has
   been wired since ADR-000944 removed the reproject/backfill job without a
   replacement -- see [[knowledge-home-reproject-operations]]). Use
   [[knowledge-home-projection-recovery]]'s admin rebuild endpoint (preferred) or its direct-SQL fallback instead.

### Database error

`knowledge-sovereign` dials `knowledge-sovereign-db` directly with no
pooler in front of it (see Investigation step 3). PgBouncer only fronts
`alt-db`/Kratos and is not on the Knowledge Home read/write path.

1. Check PostgreSQL connection limits on the sovereign DB:
   ```bash
   docker exec alt-knowledge-sovereign-db-1 \
     psql -U sovereign -d knowledge_sovereign -c "SHOW max_connections;"
   ```
2. If connections are exhausted, investigate long-running queries:
   ```bash
   docker exec alt-knowledge-sovereign-db-1 \
     psql -U sovereign -d knowledge_sovereign -c \
     "SELECT pid, state, query_start, query FROM pg_stat_activity
      WHERE state != 'idle' ORDER BY query_start LIMIT 10;"
   ```
3. If `knowledge-sovereign-db` itself is unresponsive:
   ```bash
   docker compose -f compose/compose.yaml -p alt restart knowledge-sovereign-db
   ```

### Availability burn rate

If the burn rate alert fired but degraded mode is not the cause:
1. Check for HTTP 5xx errors in alt-backend logs.
2. Check upstream dependencies (auth-hub, pre-processor).
3. Review recent deployments that may have introduced regressions.

## Verification

After resolution, confirm:
- `alt_home_degraded_responses_total` rate returns to near zero.
- Projector lag (via the `knowledge_projection_checkpoints` query in step 1; `alt_home_projector_lag_seconds` is declared but never recorded) is below 60 seconds.
- Knowledge Home UI no longer shows the degraded banner.
- `curl http://localhost:9000/v1/health` returns 200.
