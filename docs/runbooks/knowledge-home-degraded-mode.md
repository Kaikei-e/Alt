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
docker exec alt-db sh -lc \
  "psql -U alt_db_user -d alt -P pager=off -c \
  \"SELECT projector_name, last_event_seq, updated_at,
     EXTRACT(EPOCH FROM (now() - updated_at)) AS lag_seconds
   FROM knowledge_projection_checkpoints;\""
```

If `lag_seconds` is high, the projector is behind. See [[knowledge-home-projection-recovery]].

### 2. Check alt-backend logs

```bash
docker compose -f compose/compose.yaml -p alt logs alt-backend --since=15m 2>&1 | grep -i "knowledge\|projector\|degraded"
```

Look for:
- `projection stale, serving degraded` -- confirms degraded mode is active.
- `JSONB encoding error` -- projector bug; needs code fix + reproject.
- `connection refused` / `pool exhausted` -- DB connectivity issue.

### 3. Check database connectivity

```bash
# PgBouncer stats
docker exec alt-pgbouncer sh -lc "psql -p 6432 -U alt_db_user pgbouncer -c 'SHOW POOLS;'"

# Active connections
docker exec alt-db sh -lc \
  "psql -U alt_db_user -d alt -c \
  \"SELECT state, count(*) FROM pg_stat_activity GROUP BY state;\""
```

### 4. Check feature flags

```bash
# If degraded mode was manually activated
docker compose -f compose/compose.yaml -p alt logs alt-backend --since=1h 2>&1 | grep "feature_flag\|force_degraded"
```

## Resolution

### Stale projection

1. Check projector job logs for errors.
2. If projector is stuck, restart alt-backend:
   ```bash
   docker compose -f compose/compose.yaml -p alt restart alt-backend
   ```
3. If projection is corrupt, follow [[knowledge-home-projection-recovery]].
4. If backfill is needed:
   ```bash
   # Trigger backfill via admin UI or API
   curl -X POST http://localhost:9000/v1/admin/knowledge-home/backfill \
     -H "Authorization: Bearer $ADMIN_TOKEN"
   ```

### Database error

1. Check PgBouncer pool usage. If `sv_active` + `sv_used` is at `max_db_connections`:
   ```bash
   # Restart PgBouncer to release stale connections
   docker compose -f compose/compose.yaml -p alt restart alt-pgbouncer
   ```
2. Check PostgreSQL connection limits:
   ```bash
   docker exec alt-db sh -lc "psql -U alt_db_user -d alt -c \"SHOW max_connections;\""
   ```
3. If connections are exhausted, investigate long-running queries:
   ```bash
   docker exec alt-db sh -lc \
     "psql -U alt_db_user -d alt -c \
     \"SELECT pid, state, query_start, query FROM pg_stat_activity
      WHERE state != 'idle' ORDER BY query_start LIMIT 10;\""
   ```

### Availability burn rate

If the burn rate alert fired but degraded mode is not the cause:
1. Check for HTTP 5xx errors in alt-backend logs.
2. Check upstream dependencies (auth-hub, pre-processor).
3. Review recent deployments that may have introduced regressions.

## Verification

After resolution, confirm:
- `alt_home_degraded_responses_total` rate returns to near zero.
- `alt_home_projector_lag_seconds` is below 60.
- Knowledge Home UI no longer shows the degraded banner.
- `curl http://localhost:9000/v1/health` returns 200.
