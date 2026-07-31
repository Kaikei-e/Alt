---
title: Knowledge Home Projection Recovery
date: 2026-04-11
tags:
  - runbook
  - knowledge-home
  - projection
updated: 2026-07-31
---

# Knowledge Home Projection Recovery

This runbook restores Knowledge Home when the append-only event log is healthy but the read model is empty, stale, or malformed.

## Model

- `knowledge_events` is the canonical append-only event store.
- `knowledge_home_items`, `today_digest_view`, and `recall_candidate_view` are disposable projections.
- `knowledge_projection_checkpoints` stores projector progress only. It is not a source of truth.
- All of the above live in `knowledge-sovereign-db` (database `knowledge_sovereign`, user `sovereign`), and the projector runs inside the `knowledge-sovereign` service — not `alt-backend`. Migration `20260323100000_drop_sovereign_tables.sql` dropped every one of these tables from `alt-db`, so any recovery command aimed at `alt-db` now fails with `relation ... does not exist`.

This follows the repository's append-first / projection-later design and the immutable event model used by Knowledge Home.

## Symptoms

- Knowledge Home UI shows the warming-up empty state for active users.
- `knowledge_events` has rows but `knowledge_home_items` is empty or far behind.
- `knowledge-sovereign` logs show `knowledge_home_projector batch failed` on every tick, always naming the same `seq=` (a poison-pill event — see below).
- `knowledge_projection_checkpoints` is missing or not advancing.

## Preconditions

- The projection writer bug has already been fixed and deployed.
- `knowledge-sovereign` is healthy and its DB migrations have completed.
- You have shell access to the running Postgres container.

## Inspect Current State

```bash
docker exec alt-knowledge-sovereign-db-1 \
  psql -U sovereign -d knowledge_sovereign -P pager=off -c \
  "SELECT count(*) AS events FROM knowledge_events;
   SELECT count(*) AS home_items FROM knowledge_home_items;
   SELECT projector_name, last_event_seq, updated_at FROM knowledge_projection_checkpoints;
   SELECT job_id, status, projection_version, total_events, processed_events, completed_at
     FROM knowledge_backfill_jobs
     ORDER BY created_at DESC LIMIT 5;"
```

Expected healthy shape:

- `knowledge_events` is non-zero.
- `knowledge_home_items` is non-zero.
- `knowledge-home-projector` checkpoint exists and advances over time.

## Poison-Pill Events

A checkpoint that never advances while `knowledge_events` keeps growing is usually **not** a case for the reset below. It means one event's fold returns an error, `RunBatch` stops at it, and the `knowledge-sovereign` tick retries the same `seq` every `KNOWLEDGE_SOVEREIGN_PROJECTOR_TICK_INTERVAL` (default 5s) forever. Clearing the read model and the checkpoint replays into that same event, so the procedure below cannot fix it on its own.

Identify it from the projector's own log line — it names the type and sequence:

```bash
docker compose -f compose/compose.yaml -p alt logs knowledge-sovereign --since 10m \
  | grep 'knowledge_home_projector batch failed'
# error="fold event <EventType> (seq=<N>): <cause>"
```

Then read the offending event before deciding anything:

```bash
docker exec alt-knowledge-sovereign-db-1 \
  psql -U sovereign -d knowledge_sovereign -P pager=off -c \
  "SELECT event_seq, event_type, occurred_at, user_id, payload
   FROM knowledge_events WHERE event_seq = <N>;"
```

Known-benign folds (no action needed — the projector already advances past them):

- **`HomeItemDismissed` for an item_key with no `knowledge_home_items` row.** A client can dismiss any non-empty `item_key`, and the event is appended independently of the write-through, so orphan dismisses are expected. This folds as a no-op and logs `knowledge_home_projector: dismiss target row not found, folding as no-op` with `event_id` / `item_key` / `projection_version`, per [[000473]] (the same condition alt-backend's write-through already treats as non-fatal). Seeing that WARN at volume points at a client sending stale item keys, not at a stuck projector.

For anything else, **fix the fold, then replay** — do not hand-advance `last_event_seq` past the event and do not delete it from `knowledge_events`. [[000456]] explicitly rejected "skip the failing event and advance the checkpoint" as a policy: it fixes the checkpoint at the cost of a permanently missing row in the read model, which is exactly the reproducibility an event-sourced projection exists to provide. Deploy the corrected fold first; the retry then clears the wedge by itself, and only if the read model is also corrupted do you need the reset below.

## Recovery Procedure

0. **First rule out a poison-pill event (previous section).** If one is wedging the checkpoint, the reset below replays straight back into it and the read model stalls at the same `seq` again — clearing state does not help until the fold that rejects the event is fixed.
1. Stop or scale down `knowledge-sovereign` so the projector is not mutating state during cleanup.
2. Keep `knowledge_events` untouched.
3. Reset only the disposable projection state.

```bash
docker exec alt-knowledge-sovereign-db-1 \
  psql -U sovereign -d knowledge_sovereign -c \
  "BEGIN;
     DELETE FROM knowledge_home_items;
     DELETE FROM today_digest_view;
     DELETE FROM recall_candidate_view;
     DELETE FROM knowledge_projection_checkpoints
       WHERE projector_name = 'knowledge-home-projector';
   COMMIT;"
```

4. Start `knowledge-sovereign` again.
5. Let the projector replay from `knowledge_events`.
6. If historical synthetic events are missing, trigger a fresh Knowledge Home backfill after the projector is healthy.

## Verify Recovery

```bash
docker exec alt-knowledge-sovereign-db-1 \
  psql -U sovereign -d knowledge_sovereign -P pager=off -c \
  "SELECT count(*) AS events FROM knowledge_events;
   SELECT count(*) AS home_items FROM knowledge_home_items;
   SELECT projector_name, last_event_seq, updated_at FROM knowledge_projection_checkpoints;"
```

Also confirm:

- `knowledge-sovereign` logs no longer show `knowledge_home_projector batch failed`.
- `knowledge_home_items` count increases.
- Knowledge Home UI no longer shows the warming-up empty state for affected users.

## Notes

- Do not patch `knowledge_home_items` manually. Rebuild it from the event log.
- Do not delete `knowledge_events` during this recovery, and do not hand-advance a checkpoint past a failing event ([[000456]]).
- PgBouncer remains in transaction pooling mode for this workflow. The recovery assumes pgx simple-protocol compatibility is preserved.
