---
title: "Knowledge Home Empty Response Spike"
date: 2026-03-18
tags:
  - runbook
  - knowledge-home
  - alt-backend
  - correctness
---

# Knowledge Home Empty Response Spike

Runbook for investigating a spike in empty Knowledge Home responses.

Related: [[000418]]

> **Note:** `knowledge_events`, `knowledge_home_items`, and
> `knowledge_projection_checkpoints` live exclusively in
> `knowledge-sovereign-db` (database `knowledge_sovereign`, user
> `sovereign`) — see [[knowledge-home-projection-recovery]]'s Model section.
> `alt-backend` (`cmd/backend`) has no direct Postgres access of any kind
> (enforced by `di/import_boundary_test.go`); commands below target
> `alt-knowledge-sovereign-db-1`.

## Alerts

| Alert | Severity | Condition |
|-------|----------|-----------|
| `KnowledgeHomeEmptyResponseHigh` | ticket | > 1% empty for 10m |

## Symptoms

- `KnowledgeHomeEmptyResponseHigh` alert fires.
- Users see the "warming up" empty state on Knowledge Home despite having subscribed feeds.
- `alt_home_empty_responses_total` counter is climbing.

## Investigation

### 1. Check projector lag

```bash
docker exec alt-knowledge-sovereign-db-1 \
  psql -U sovereign -d knowledge_sovereign -P pager=off -c \
  "SELECT projector_name, last_event_seq, updated_at,
     EXTRACT(EPOCH FROM (now() - updated_at)) AS lag_seconds
   FROM knowledge_projection_checkpoints;"
```

If the projector is significantly behind, empty responses are expected until it catches up.

### 2. Check event store health

```bash
docker exec alt-knowledge-sovereign-db-1 \
  psql -U sovereign -d knowledge_sovereign -P pager=off -c \
  "SELECT count(*) AS total_events,
     max(occurred_at) AS latest_event,
     EXTRACT(EPOCH FROM (now() - max(occurred_at))) AS seconds_since_last
   FROM knowledge_events;"
```

`knowledge_events` has no `created_at` column -- the business timestamp is
`occurred_at`. If `seconds_since_last` is very high, no new events are being
generated. Check the article ingestion pipeline.

### 3. Check if empty responses are concentrated on specific users

```bash
docker compose -f compose/compose.yaml -p alt logs alt-backend --since=15m 2>&1 | grep -i "empty"
```

If the empty responses are only for new users who have no feed subscriptions, this is expected behavior and not an incident.

### 4. Check article ingestion pipeline

```bash
# Check pre-processor is running
docker compose -f compose/compose.yaml -p alt logs pre-processor --since=30m 2>&1 | tail -20

# Check mq-hub queue depth
curl -s http://localhost:9500/metrics | grep mq_queue_depth
```

If the pre-processor is stalled or mq-hub has a large backlog, events may not be flowing into the knowledge event store.

### 5. Check projection table directly

```bash
docker exec alt-knowledge-sovereign-db-1 \
  psql -U sovereign -d knowledge_sovereign -P pager=off -c \
  "SELECT count(*) AS total_items,
     count(DISTINCT user_id) AS users_with_items,
     max(updated_at) AS latest_update
   FROM knowledge_home_items;"
```

If `total_items` is zero or very low, the projection is likely corrupt or was reset. Follow [[knowledge-home-projection-recovery]].

## Resolution

### Projector is behind

1. Wait for the projector to catch up. `alt_home_projector_lag_seconds` is declared but never recorded, so monitor lag via the `knowledge_projection_checkpoints` query in Investigation step 1 instead.
2. If the projector is stuck (lag not decreasing), check `knowledge-sovereign` logs for a poison-pill event before restarting (see [[knowledge-home-projection-recovery]]'s Poison-Pill Events section):
   ```bash
   docker compose -f compose/compose.yaml -p alt restart knowledge-sovereign
   ```

### Event store is not receiving events

1. Verify the article ingestion pipeline is healthy:
   ```bash
   curl http://localhost:9000/v1/health
   curl http://localhost:9200/health
   ```
2. Check if backfill is needed for missed events. There is no
   `/v1/admin/knowledge-home/backfill` REST route -- the admin surface is a
   Connect-RPC service alt-backend exposes internally on :9102, driven by
   `altctl home backfill trigger`. **That command currently fails
   immediately** (`ErrNoBackfillExecutor`: no executor has been wired since
   ADR-000944 -- see [[knowledge-home-reproject-operations]]). Use
   [[knowledge-home-projection-recovery]]'s admin rebuild endpoint (preferred) or its direct-SQL fallback instead.

### Projection is empty or corrupt

Follow [[knowledge-home-projection-recovery]] to rebuild projections from the event log.

## Verification

After resolution:
- `alt_home_empty_responses_total` rate drops below 1% of total requests.
- `knowledge_home_items` table has rows for active users.
- Knowledge Home UI displays items for users with subscribed feeds.
