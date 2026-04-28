---
title: "Knowledge Home Reproject Operations"
date: 2026-03-18
tags:
  - runbook
  - knowledge-home
  - alt-backend
  - reproject
---

# Knowledge Home Reproject Operations

Runbook for rebuilding Knowledge Home projections using the `altctl home reproject` command. Use this when projections need rebuilding due to schema changes, data corruption, or projection version upgrades.

Related: [[000421]], [[000429]], [[knowledge-home-phase0-canonical-contract]], [[knowledge-home-projection-recovery]]

## When to Reproject

| Scenario | Trigger |
|----------|---------|
| Schema migration changes projection shape | After deploying migration |
| Data corruption in projection tables | After investigation confirms event log is healthy |
| Projection version upgrade | New projector logic deployed |
| Backfill generated events that need re-projection | After backfill completes |

## Prerequisites

- `altctl` binary is built and accessible.
- alt-backend is running and healthy.
- Event log (`knowledge_events`) is intact and healthy.
- You have verified the new projector code is deployed if this is a version upgrade.
- You have confirmed the target change does not require backfill first. Use backfill for missing events, reproject for rebuilding read models from an existing event log.

## Safety Constraints

- Never rebuild directly into the active projection in production.
- Always run `dry_run` first, then `compare`, then `swap`.
- Do not swap if the diff contains unexplained removals, malformed `why_json`, or large `summary_state` regressions.
- Keep rollback-ready state until the post-swap monitoring window completes.
- Use `item_key`, `article_id`, and `projection_version` as the primary correlation keys during investigation.

## Procedure

### Step 1: Dry Run

Run a dry-run reproject to validate the new projection without affecting production:

```bash
altctl home reproject start \
  --mode=dry_run \
  --from=1 \
  --to=2
```

- `--from=1`: current projection version.
- `--to=2`: target projection version.
- `--mode=dry_run`: writes to a shadow table, not the live projection.

This returns a `run-id` (UUID). Save it for subsequent steps.

### Step 2: Monitor Progress

```bash
altctl home reproject status --run-id=<uuid>
```

Output includes:
- Total events to process.
- Events processed so far.
- Estimated time remaining.
- Error count.

For large event stores, this may take several minutes. Monitor `alt_home_reproject_events_total` in Grafana for throughput.

### Step 3: Compare Results

After the dry run completes, compare the shadow table with the current live projection:

```bash
altctl home reproject compare --run-id=<uuid>
```

Review the diff output:
- **Added items**: New items that the old projector missed.
- **Removed items**: Items the old projector included but the new one does not.
- **Changed items**: Fields that differ between old and new projection.

If the diff is unexpected, investigate before proceeding. Do not swap if there are unexplained removals.

### Step 4: Swap to New Version

Once the diff is acceptable, atomically swap the new projection into production:

```bash
altctl home reproject swap --run-id=<uuid>
```

This performs:
1. Renames the shadow table to the live table name (atomic rename).
2. Updates `knowledge_projection_checkpoints` to the new version.
3. Restarts the projector to continue from the new checkpoint.

### Step 5: Monitor SLO Dashboard

After the swap, monitor the Knowledge Home SLO dashboard for 30 minutes:

- `alt_home_empty_responses_total` rate should not spike.
- `alt_home_malformed_why_total` rate should remain near zero.
- `alt_home_projector_lag_seconds` should stabilize below 60.
- `alt_home_degraded_responses_total` should not increase.
- `alt_home_request_duration_seconds` should remain within the normal p95 band for `knowledge_home_get_latency`.
- `alt_home_stream_connections_total` should not show an abnormal reconnect surge if stream clients are active.

Canonical metric mapping is defined in [[knowledge-home-phase0-canonical-contract]].

### Step 6: Rollback (if needed)

If issues are detected within the monitoring window:

```bash
altctl home reproject rollback --run-id=<uuid>
```

This:
1. Swaps back to the previous projection table.
2. Restores the old checkpoint version.
3. Restarts the projector with the old version.

After rollback, verify all SLI metrics return to normal.

## Database Verification

At any point, inspect the projection state directly:

```bash
docker exec alt-db sh -lc \
  "psql -U alt_db_user -d alt -P pager=off -c \
  \"SELECT projector_name, last_event_seq, updated_at, projection_version
   FROM knowledge_projection_checkpoints;
   SELECT count(*) AS home_items FROM knowledge_home_items;
   SELECT count(*) AS events FROM knowledge_events;\""
```

## Troubleshooting

| Problem | Resolution |
|---------|------------|
| Dry run fails with encoding errors | Check event payloads for malformed JSON; fix events before retrying |
| Compare shows massive diff | Likely a projector logic bug; review the new projector code before swapping |
| Swap fails with lock timeout | Another process holds a lock on the projection table; retry after checking `pg_locks` |
| Rollback not available | The old table has been cleaned up (default: 24h retention); use [[knowledge-home-projection-recovery]] instead |
| Reproject is too slow | Increase batch size with `--batch-size=1000` or run during low-traffic hours |
| Mode=Dry Run was selected and Swap was clicked | Previously this would activate an empty v_new (PM-2026-041 / ADR-000867 cause). ADR-000867 added a usecase guard that rejects SwapReproject for `dry_run` runs. Re-run with `--mode=full` and Swap again. |
| Article cards all show `Archived · No source URL` after Reproject Swap | See **Post-tag-fix backfill** below — this is the PM-2026-041 / ADR-000867 symptom |

## Post-tag-fix backfill (ADR-000867)

When a wire-form fix lands that corrects how the producer marshals the article URL into `knowledge_events.payload` (e.g. ADR-000865, ADR-000867), the **historical events stay immutable** with their old payload schema. A Full Reproject by itself will read those old events with the new consumer struct and project empty URLs — exactly the PM-2026-041 / 2026-04-28 symptom on `v5`.

The recovery procedure is a one-shot **append-first** corrective wave: emit `ArticleUrlBackfilled` events, run a Full Reproject, then Swap.

### Step 1 — Emit corrective events

Run on the alt-backend host (as a one-shot job, until the admin endpoint lands in a follow-up PR). The script reads `articles.url` for every article whose Knowledge Home row has an empty `url` and appends one `ArticleUrlBackfilled` knowledge event per article. Idempotent via `dedupe_key = "article-url-backfill:<article_id>"`.

```bash
docker exec alt-alt-backend-1 sh -lc '
psql "$DATABASE_URL" -At -c "
  SELECT a.id, a.user_id, a.url
  FROM articles a
  WHERE a.url IS NOT NULL AND a.url != ''\'''\''
    AND a.url ~* '\''^https?://[^[:space:]]+$'\''
  LIMIT 50000
" | while IFS="|" read -r article_id user_id url; do
  payload=$(printf "{\"article_id\":\"%s\",\"url\":\"%s\"}" "$article_id" "$url")
  curl -fsS -X POST \
    "$KNOWLEDGE_SOVEREIGN_URL/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent" \
    -H "Content-Type: application/json" \
    -d "$(jq -nc --arg eid "$(uuidgen)" --arg now "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        --arg tid "$user_id" --arg uid "$user_id" --arg aid "$article_id" \
        --arg ddk "article-url-backfill:$article_id" --arg pl "$payload" \
        '\''{event:{eventId:$eid,occurredAt:$now,tenantId:$tid,userId:$uid,actorType:"service",actorId:"backfill-cli",eventType:"ArticleUrlBackfilled",aggregateType:"article",aggregateId:$aid,dedupeKey:$ddk,payload:($pl|@base64)}}'\'')"
done
'
```

URL allowlist (`^https?://[^[:space:]]+$`) is enforced at the source query — non-HTTP schemes are filtered out before any event is appended (security-auditor F-001 defense layer 1; the projector and SQL apply layers 2 and 3).

### Step 2 — Verify event emission

```bash
docker exec alt-knowledge-sovereign-db-1 \
  psql -U sovereign -d knowledge_sovereign -c \
  "SELECT count(*), max(occurred_at) FROM knowledge_events
   WHERE event_type='ArticleUrlBackfilled';"
```

Count should match the number of articles processed in Step 1.

### Step 3 — Full Reproject (Mode=Full only — Dry Run is rejected at swap)

```bash
altctl home reproject start --mode=full --from=<v_active> --to=<v_active+1>
altctl home reproject status --run-id=<uuid>   # wait for Swappable
altctl home reproject compare --run-id=<uuid>  # diff should show ~12k+ "url added" rows
altctl home reproject swap --run-id=<uuid>
```

### Step 4 — Verify URL recovery

```bash
docker exec alt-knowledge-sovereign-db-1 \
  psql -U sovereign -d knowledge_sovereign -c \
  "SELECT projection_version, count(*) AS total,
          count(*) FILTER (WHERE url='') AS empty_url
   FROM knowledge_home_items
   WHERE item_type='article'
   GROUP BY 1 ORDER BY 1 DESC LIMIT 3;"
```

Expected: the new `v_active` has `empty_url` ≤ original-no-URL count (≈14,946 as of 2026-04-28). Articles with `articles.url` truly missing in the source-of-truth table stay empty (legitimate `Archived` kicker).

### Step 5 — UI verification

Browse `/home`. The `Archived · No source URL` kicker should now appear only on the small subset of articles whose `articles.url` was never recorded — not the majority.

If many cards still show the kicker, re-emit Step 1 with the corrected SQL filter and repeat Steps 2–4.
