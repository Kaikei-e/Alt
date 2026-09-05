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

Journey-level feeds / login / search burn-rate pages: [[user-journey-slo-gameday]].

> **Note:** `knowledge_events`, `knowledge_home_items`, and
> `knowledge_projection_checkpoints` live exclusively in
> `knowledge-sovereign-db` (database `knowledge_sovereign`, user
> `sovereign`), and the projector's `LISTEN`/`NOTIFY` channel is
> `knowledge_projector` on that same database — see
> [[knowledge-home-projection-recovery]]'s Model section. Scenarios below
> target `alt-knowledge-sovereign-db-1` accordingly; `alt-db` no longer has
> these tables or that channel.

## Prerequisites

- All Knowledge Home services are healthy (`curl http://localhost:9000/v1/health`).
- Grafana SLO dashboard is open and visible.
- At least one user has Knowledge Home items populated.
- Alert notification channel is configured and tested.
- A dedicated GameDay time window is scheduled (minimum 2 hours).

## Scenario 1: Projector Stop

**Goal**: Verify SLI-B (freshness) degrades and the degraded banner appears. `KnowledgeHomeFreshnessStale`/`FreshnessCritical` cannot be exercised by this scenario -- see the note below.

### Setup

```bash
# Record baseline projection lag from knowledge_projection_checkpoints
# (alt_home_projector_lag_seconds is never recorded -- see the note below --
# so this SQL query is the only working baseline):
docker exec alt-knowledge-sovereign-db-1 \
  psql -U sovereign -d knowledge_sovereign -P pager=off -c \
  "SELECT projector_name, EXTRACT(EPOCH FROM (now() - updated_at)) AS lag_seconds
   FROM knowledge_projection_checkpoints;"

# Stop the projector WITHOUT stopping the rest of the service. alt-backend
# reads the entire Knowledge Home payload from knowledge-sovereign over
# Connect-RPC (di/knowledge_module.go) and has no local fallback, so pausing
# or stopping the container is a full read outage -- it will trip an
# availability alert, not the freshness alert this scenario targets.
#
# The projector runs as an in-process ticker (poll interval
# KNOWLEDGE_SOVEREIGN_PROJECTOR_TICK_INTERVAL, default 5s,
# knowledge-sovereign/app/config/config.go) -- not LISTEN/NOTIFY-driven --
# so raising that interval freezes it while the read RPCs keep serving. The
# variable is not wired into compose/sovereign.yaml's environment list
# today, so set it for the duration of the test via a scratch override file
# instead of pausing the container:
cat > /tmp/gameday-tick-freeze.yaml <<'EOF'
services:
  knowledge-sovereign:
    environment:
      - KNOWLEDGE_SOVEREIGN_PROJECTOR_TICK_INTERVAL=24h
EOF
docker compose -f compose/compose.yaml -f /tmp/gameday-tick-freeze.yaml \
  -p alt up -d --force-recreate knowledge-sovereign
```

### Expected Behavior

> **`alt_home_projector_lag_seconds` is never recorded.** The gauge is
> declared (`alt-backend/app/utils/otel/knowledge_home_metrics.go`) and has a
> setter (`MetricsSnapshot.RecordProjectorLag`), but nothing in the codebase
> calls the setter, so the series is absent from `/metrics` and
> `KnowledgeHomeFreshnessStale`/`FreshnessCritical` (which alert on it) can
> never fire regardless of how stale the projection gets. Track freshness via
> `altctl home slo` (backed by `GetSLOStatus`, `knowledge_slo_usecase`) or the
> `knowledge_projection_checkpoints.updated_at` lag query from
> [[knowledge-home-degraded-mode]] step 1 instead.

| Time | Expected |
|------|----------|
| +0m | Projector stops advancing; lag starts climbing |
| +10m | `knowledge_projection_checkpoints` lag (see query above) exceeds 600s |
| +10-15m | Degraded banner appears in Knowledge Home UI |

### Verification

- [ ] `knowledge_projection_checkpoints.updated_at` lag is climbing (see query in [[knowledge-home-degraded-mode]] step 1; `alt_home_projector_lag_seconds` is not recorded, so do not rely on it).
- [ ] Knowledge Home UI shows amber degraded banner.
- [ ] `alt_home_degraded_responses_total` counter is incrementing.

### Cleanup

```bash
# Recreate knowledge-sovereign without the override to restore the projector
docker compose -f compose/compose.yaml -p alt up -d --force-recreate knowledge-sovereign
rm -f /tmp/gameday-tick-freeze.yaml

# Verify projector catches up (alt_home_projector_lag_seconds is not
# recorded -- read the checkpoint lag directly instead)
watch -n5 'docker exec alt-knowledge-sovereign-db-1 \
  psql -U sovereign -d knowledge_sovereign -P pager=off -c \
  "SELECT projector_name, EXTRACT(EPOCH FROM (now() - updated_at)) AS lag_seconds
   FROM knowledge_projection_checkpoints;"'
```

- [ ] Projector lag returns below 60 seconds.
- [ ] Degraded banner disappears.

---

## Scenario 2: Stale Projection Injection

**Goal**: Verify SLI-E (correctness) degrades and empty home alert fires.

### Setup

```bash
# Record current projection state
docker exec alt-knowledge-sovereign-db-1 \
  psql -U sovereign -d knowledge_sovereign -P pager=off -c \
  "SELECT count(*) FROM knowledge_home_items;"

# Simulate stale projection by deleting items for a test user
# (Use a known test user ID)
docker exec alt-knowledge-sovereign-db-1 \
  psql -U sovereign -d knowledge_sovereign -c \
  "DELETE FROM knowledge_home_items WHERE user_id = '<test-user-id>';"
```

To trigger a broader empty response spike (for alert testing), temporarily
increase the threshold by deleting more items. `knowledge_home_items` has no
`id` column -- its primary key is `(user_id, item_key, projection_version)`:

```bash
docker exec alt-knowledge-sovereign-db-1 \
  psql -U sovereign -d knowledge_sovereign -c \
  "BEGIN;
   -- Save backup
   CREATE TEMP TABLE khi_backup AS SELECT * FROM knowledge_home_items;
   -- Delete a percentage of items to trigger empty responses
   DELETE FROM knowledge_home_items
   WHERE (user_id, item_key, projection_version) IN (
     SELECT user_id, item_key, projection_version
     FROM knowledge_home_items ORDER BY random() LIMIT 100
   );
   COMMIT;"
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
# Restarting knowledge-sovereign does NOT restore the deleted rows -- the
# projector's checkpoint has already advanced past the events that produced
# them, so it will not re-derive them on its own. Restore from the backup
# table created above (composite PK, no single "id" column):
docker exec alt-knowledge-sovereign-db-1 \
  psql -U sovereign -d knowledge_sovereign -c \
  "INSERT INTO knowledge_home_items SELECT * FROM khi_backup
   ON CONFLICT (user_id, item_key, projection_version) DO NOTHING;"
```

If the backup table was not created (e.g. only the single-user delete above
ran): `altctl home reproject`/`altctl home backfill trigger` currently fail
immediately regardless of flags (no executor wired since ADR-000944, see
[[knowledge-home-reproject-operations]]), so restore via
[[knowledge-home-projection-recovery]]'s admin rebuild endpoint (preferred) or its direct-SQL fallback instead.

- [ ] `alt_home_empty_responses_total` rate returns to baseline.
- [ ] Alert resolves.

---

## Scenario 3: Stream Disconnect

**Goal**: Verify client reconnects and falls back to unary after 3 failures.

### Setup

```bash
# Monitor stream metrics baseline. alt-backend's ops listener (:9110,
# /metrics) is not published to the host -- run this from inside alt-network.
docker run --rm --network alt_alt-network busybox:1.37 \
  wget -qO- http://alt-backend:9110/metrics | grep alt_home_stream

# Simulate stream disconnects by killing backend connections
# This will cause all active streams to drop
docker compose -f compose/compose.yaml -p alt restart alt-backend
```

For a more targeted test: alt-backend's stream is fed by a Connect-RPC call
(`WatchProjectorEvents`) to `knowledge-sovereign`, which itself holds a
`LISTEN knowledge_projector` connection on `knowledge-sovereign-db` (not
`alt-db`). Terminate that connection:

```bash
docker exec alt-knowledge-sovereign-db-1 \
  psql -U sovereign -d knowledge_sovereign -c \
  "SELECT pg_terminate_backend(pid)
   FROM pg_stat_activity
   WHERE query LIKE '%LISTEN knowledge_projector%';"
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
- [ ] Client-side logs show reconnection attempts (check browser console). `alt_home_stream_reconnects_total` is declared but never incremented server-side, so reconnects are only visible in browser logs, not `/metrics`.
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

> **This scenario cannot currently be validated end-to-end.**
> `alt_home_malformed_why_total` (`alt-backend/app/utils/otel/knowledge_home_metrics.go`)
> and its `MetricsSnapshot.RecordMalformedWhy()` counterpart
> (`alt-backend/app/utils/otel/metrics_snapshot.go`) are declared but have no
> call site anywhere in the codebase today -- nothing increments either one,
> so `KnowledgeHomeMalformedWhyHigh` cannot fire regardless of what is
> injected. Run this scenario only to confirm that gap (it should stay flat
> at zero), not to exercise the alert.

`knowledge_events` also has several `NOT NULL` columns this scenario's event
must supply (`tenant_id`, `actor_type`, `aggregate_type`, `aggregate_id`,
`dedupe_key`) and no `created_at` column (use `occurred_at`), and the
projector only folds known `event_type` values (`ArticleCreated`,
`SummaryVersionCreated`, `HomeItemDismissed`, …) -- `article_recommended`
below is not one of them, so even the injected rows will not be projected.
A runnable insert (against `knowledge-sovereign-db`) looks like:

```bash
for i in $(seq 1 20); do
  docker exec alt-knowledge-sovereign-db-1 \
    psql -U sovereign -d knowledge_sovereign -c \
    "INSERT INTO knowledge_events
       (occurred_at, tenant_id, user_id, actor_type, event_type,
        aggregate_type, aggregate_id, dedupe_key, payload)
     VALUES (
       now(), '<test-tenant-id>', '<test-user-id>', 'user', 'article_recommended',
       'article', 'test-gd-$(printf '%03d' $i)',
       'gameday-malformed-why-$(printf '%03d' $i)',
       '{\"article_id\": \"test-gd-$(printf '%03d' $i)\", \"why\": \"\", \"score\": 0.9}'::jsonb
     );"
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
# Remove test events. Since event_type 'article_recommended' is not one the
# projector folds (see the note in Setup above), no knowledge_home_items rows
# should exist to clean up from this scenario as currently written.
docker exec alt-knowledge-sovereign-db-1 \
  psql -U sovereign -d knowledge_sovereign -c \
  "DELETE FROM knowledge_events
   WHERE payload->>'article_id' LIKE 'test-gd-%';"
```

- [ ] `alt_home_malformed_why_total` rate returns to zero.
- [ ] Alert resolves.
- [ ] No test data remains in projection tables.

---

## Scenario 5: Reproject Under Active Traffic

**Goal**: Verify reprojection can run during active traffic without exceeding SLI-B lag budget.

> **This scenario cannot currently be run, for two independent reasons.**
> First, no executor has advanced a reproject run past `pending` since
> ADR-000944 removed that job without a replacement
> (`ErrNoReprojectExecutor`, see [[knowledge-home-reproject-operations]]).
> Second, even once an executor exists, `altctl`'s client-side mode
> validator (`dry_run`/`shadow`/`live`) and alt-backend's server-side mode
> validator (`dry_run`/`user_subset`/`time_range`/`full`) intersect only at
> `dry_run` -- and `dry_run` runs cannot be swapped
> (see [[knowledge-home-reproject-operations]] Safety Constraints). The
> production mode this scenario needs is `full`; it is rejected by `altctl`
> itself before it ever reaches the backend. Re-run this scenario once both
> the executor and the mode mismatch are fixed; until then it can only
> confirm the command still refuses.

### Setup

```bash
# Record baseline metrics. alt-backend's ops listener (:9110, /metrics) is
# not published to the host -- run this from inside alt-network.
docker run --rm --network alt_alt-network busybox:1.37 \
  wget -qO- http://alt-backend:9110/metrics | grep -E "alt_home_(requests_total|degraded)"

# Intended production reproject (rejected today -- see the note above).
altctl home reproject start \
  --mode=full \
  --from=1 \
  --to=2
```

Save the returned `run-id`.

### Expected Behavior

| Time | Expected |
|------|----------|
| +0m | Reproject starts, writing the target projection version |
| +0-Nm | `knowledge_projection_checkpoints` lag (see [[knowledge-home-degraded-mode]] step 1) stays below 600 (existing projector unaffected) |
| +0-Nm | `alt_home_requests_total` rate remains stable |
| +0-Nm | No increase in `alt_home_degraded_responses_total` |
| Completion | `altctl home reproject status` shows 100% |

### Verification

```bash
# Check reproject status
altctl home reproject status --run-id=<uuid>
```

- [ ] Projector lag (via `knowledge_projection_checkpoints`; `alt_home_projector_lag_seconds` is not recorded) remains under 600 seconds throughout reproject.
- [ ] No `KnowledgeHomeDegradedResponseHigh` alert fires during reproject.
- [ ] Request latency (`alt_home_request_duration_seconds`) does not regress significantly.
- [ ] Reproject completes successfully.

### Cleanup

```bash
# Roll back to the previous projection version if the swap needs undoing
altctl home reproject rollback --run-id=<uuid>
```

- [ ] Rollback completes and the previous projection version is restored.
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
