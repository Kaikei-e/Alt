---
title: "Knowledge Home GameDay Checklist"
date: 2026-03-18
tags:
  - runbook
  - knowledge-home
  - gameday
  - chaos-engineering
---

# Knowledge Home GameDay Checklist

Structured chaos scenarios to validate Knowledge Home SLO alerting, degraded mode, and recovery procedures.

Related: [[000418]]

## Prerequisites

- All Knowledge Home services are healthy (`curl http://localhost:9000/v1/health`).
- Grafana SLO dashboard is open and visible.
- At least one user has Knowledge Home items populated.
- Alert notification channel is configured and tested.
- A dedicated GameDay time window is scheduled (minimum 2 hours).

## Scenario 1: Projector Stop

**Goal**: Verify SLI-B (freshness) degrades, alert fires, degraded banner appears.

### Setup

```bash
# Record baseline projector lag
curl -s http://localhost:9000/metrics | grep alt_home_projector_lag

# Stop the projector by pausing alt-backend's projector goroutine
# Option A: Kill the LISTEN connection
docker exec alt-db sh -lc \
  "psql -U alt_db_user -d alt -c \
  \"SELECT pg_terminate_backend(pid)
   FROM pg_stat_activity
   WHERE query LIKE '%LISTEN knowledge_home%';\""
```

### Expected Behavior

| Time | Expected |
|------|----------|
| +0m | Projector stops advancing; lag starts climbing |
| +5m | `KnowledgeHomeFreshnessStale` alert fires (lag > 600s) if lag was already near threshold, or continues climbing |
| +10m | `alt_home_projector_lag_seconds` exceeds 600 |
| +10-15m | Degraded banner appears in Knowledge Home UI |
| +30m | `KnowledgeHomeFreshnessCritical` fires (lag > 1800s) |

### Verification

- [ ] `alt_home_projector_lag_seconds` metric is climbing in Grafana.
- [ ] `KnowledgeHomeFreshnessStale` alert fires within expected window.
- [ ] Knowledge Home UI shows amber degraded banner.
- [ ] `alt_home_degraded_responses_total` counter is incrementing.

### Cleanup

```bash
# Restart alt-backend to restore projector
docker compose -f compose/compose.yaml -p alt restart alt-backend

# Verify projector catches up
watch -n5 'curl -s http://localhost:9000/metrics | grep alt_home_projector_lag'
```

- [ ] Projector lag returns below 60 seconds.
- [ ] Alert resolves automatically.
- [ ] Degraded banner disappears.

---

## Scenario 2: Stale Projection Injection

**Goal**: Verify SLI-E (correctness) degrades and empty home alert fires.

### Setup

```bash
# Record current projection state
docker exec alt-db sh -lc \
  "psql -U alt_db_user -d alt -P pager=off -c \
  \"SELECT count(*) FROM knowledge_home_items;\""

# Simulate stale projection by deleting items for a test user
# (Use a known test user ID)
docker exec alt-db sh -lc \
  "psql -U alt_db_user -d alt -c \
  \"DELETE FROM knowledge_home_items WHERE user_id = '<test-user-id>';\""
```

To trigger a broader empty response spike (for alert testing), temporarily increase the threshold by deleting more items:

```bash
docker exec alt-db sh -lc \
  "psql -U alt_db_user -d alt -c \
  \"BEGIN;
   -- Save backup
   CREATE TEMP TABLE khi_backup AS SELECT * FROM knowledge_home_items;
   -- Delete a percentage of items to trigger empty responses
   DELETE FROM knowledge_home_items
   WHERE id IN (SELECT id FROM knowledge_home_items ORDER BY random() LIMIT 100);
   COMMIT;\""
```

### Expected Behavior

| Time | Expected |
|------|----------|
| +0m | Some users get empty Knowledge Home responses |
| +10m | `KnowledgeHomeEmptyResponseHigh` alert fires (> 1% empty for 10m) |

### Verification

- [ ] `alt_home_empty_responses_total` rate increases in Grafana.
- [ ] `KnowledgeHomeEmptyResponseHigh` alert fires.
- [ ] Affected users see the "warming up" empty state.

### Cleanup

```bash
# Restore deleted items by triggering a reproject or backfill
docker compose -f compose/compose.yaml -p alt restart alt-backend

# Or restore from backup table if created
docker exec alt-db sh -lc \
  "psql -U alt_db_user -d alt -c \
  \"INSERT INTO knowledge_home_items SELECT * FROM khi_backup
   ON CONFLICT (id) DO NOTHING;\""
```

- [ ] `alt_home_empty_responses_total` rate returns to baseline.
- [ ] Alert resolves.

---

## Scenario 3: Stream Disconnect

**Goal**: Verify client reconnects and falls back to unary after 3 failures.

### Setup

```bash
# Monitor stream metrics baseline
curl -s http://localhost:9000/metrics | grep alt_home_stream

# Simulate stream disconnects by killing backend connections
# This will cause all active streams to drop
docker compose -f compose/compose.yaml -p alt restart alt-backend
```

For a more targeted test, terminate LISTEN/NOTIFY connections:

```bash
docker exec alt-db sh -lc \
  "psql -U alt_db_user -d alt -c \
  \"SELECT pg_terminate_backend(pid)
   FROM pg_stat_activity
   WHERE query LIKE '%LISTEN%';\""
```

### Expected Behavior

| Time | Expected |
|------|----------|
| +0s | Active streams disconnect |
| +1s | Client detects disconnect, starts reconnection attempt 1 |
| +5s | Reconnection attempt 2 (exponential backoff) |
| +15s | Reconnection attempt 3 |
| +15s+ | After 3 failures, client switches to unary polling mode |
| +5m | If disconnects persist, `KnowledgeHomeStreamDisconnectSurge` fires |

### Verification

- [ ] `alt_home_stream_disconnects_total` counter increases.
- [ ] `alt_home_stream_reconnects_total` counter increases.
- [ ] Client-side logs show reconnection attempts (check browser console).
- [ ] After 3 failures, client falls back to unary polling.
- [ ] Knowledge Home still displays data (via unary fallback).
- [ ] `KnowledgeHomeStreamDisconnectSurge` fires if disconnect rate exceeds 80%.

### Cleanup

```bash
# Ensure alt-backend is healthy
docker compose -f compose/compose.yaml -p alt restart alt-backend
```

- [ ] Streams re-establish after backend recovery.
- [ ] `alt_home_stream_connections_total` rate stabilizes.
- [ ] Client resumes streaming mode after successful reconnection.

---

## Scenario 4: Malformed Why Injection

**Goal**: Verify SLI-E correctness alert fires for malformed why explanations.

### Setup

```bash
# Inject a malformed event with a bad "why" field
docker exec alt-db sh -lc \
  "psql -U alt_db_user -d alt -c \
  \"INSERT INTO knowledge_events (user_id, event_type, payload, created_at)
   VALUES (
     '<test-user-id>',
     'article_recommended',
     '{\"article_id\": \"test-gd-001\", \"why\": \"\", \"score\": 0.9}'::jsonb,
     now()
   );\""

# Inject several more to exceed the 0.1% threshold
for i in $(seq 1 20); do
  docker exec alt-db sh -lc \
    "psql -U alt_db_user -d alt -c \
    \"INSERT INTO knowledge_events (user_id, event_type, payload, created_at)
     VALUES (
       '<test-user-id>',
       'article_recommended',
       '{\"article_id\": \"test-gd-$(printf '%03d' $i)\", \"why\": \"\"}'::jsonb,
       now()
     );\""
done
```

### Expected Behavior

| Time | Expected |
|------|----------|
| +0m | Projector processes malformed events |
| +1m | `alt_home_malformed_why_total` counter increases |
| +10m | `KnowledgeHomeMalformedWhyHigh` alert fires (> 0.1% for 10m) |

### Verification

- [ ] `alt_home_malformed_why_total` counter increases in Grafana.
- [ ] `KnowledgeHomeMalformedWhyHigh` alert fires.
- [ ] Affected items show fallback text (not garbled content).

### Cleanup

```bash
# Remove test events
docker exec alt-db sh -lc \
  "psql -U alt_db_user -d alt -c \
  \"DELETE FROM knowledge_events
   WHERE payload->>'article_id' LIKE 'test-gd-%';\""

# Reproject to clean up affected items
docker compose -f compose/compose.yaml -p alt restart alt-backend
```

- [ ] `alt_home_malformed_why_total` rate returns to zero.
- [ ] Alert resolves.
- [ ] No test data remains in projection tables.

---

## Scenario 5: Reproject Under Active Traffic

**Goal**: Verify reprojection can run during active traffic without exceeding SLI-B lag budget.

### Setup

```bash
# Record baseline metrics
curl -s http://localhost:9000/metrics | grep -E "alt_home_(projector_lag|requests_total|degraded)"

# Start a dry-run reproject
altctl home reproject start \
  --mode=dry_run \
  --from=1 \
  --to=2
```

Save the returned `run-id`.

### Expected Behavior

| Time | Expected |
|------|----------|
| +0m | Reproject starts; shadow table receives writes |
| +0-Nm | `alt_home_projector_lag_seconds` stays below 600 (existing projector unaffected) |
| +0-Nm | `alt_home_requests_total` rate remains stable |
| +0-Nm | No increase in `alt_home_degraded_responses_total` |
| Completion | `altctl home reproject status` shows 100% |

### Verification

```bash
# Check reproject status
altctl home reproject status --run-id=<uuid>
```

- [ ] Projector lag remains under 600 seconds throughout reproject.
- [ ] No `KnowledgeHomeFreshnessStale` alert fires during reproject.
- [ ] No `KnowledgeHomeDegradedResponseHigh` alert fires during reproject.
- [ ] Request latency (`alt_home_request_duration_seconds`) does not regress significantly.
- [ ] Reproject completes successfully.

### Cleanup

```bash
# If this was a dry run, clean up the shadow table
altctl home reproject rollback --run-id=<uuid>
```

- [ ] Shadow table is cleaned up.
- [ ] No lingering impact on production metrics.

---

## Post-GameDay Report

After completing all scenarios, document:

1. **Scenarios completed**: Which scenarios ran and results.
2. **Alert timing**: Actual time to fire vs. expected time for each alert.
3. **Gaps found**: Any scenarios where alerts did not fire or recovery was unclear.
4. **Action items**: Improvements to alerting thresholds, runbooks, or code.
5. **Participants**: Who was involved and their roles.

File the report as a daily note in the vault: `docs/daily/YYYY-MM-DD.md`.
