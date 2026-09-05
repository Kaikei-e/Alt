---
title: Knowledge Home Projection Recovery
date: 2026-04-11
tags:
  - runbook
  - knowledge-home
  - projection
updated: 2026-09-05
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

Known non-fatal folds (the projector already advances past these, so they are never the cause of a stuck checkpoint — but "non-fatal" is not the same as "ignorable", see the triage section):

- **`HomeItemDismissed` for an item_key with no `knowledge_home_items` row.** A client can dismiss any non-empty `item_key`, and the event is appended independently of the write-through, so orphan dismisses are expected. This folds as a no-op and logs `knowledge_home_projector: dismiss target row not found, folding as no-op` with `event_id` / `item_key` / `projection_version`, per [[000473]] (the same condition alt-backend's write-through already treats as non-fatal). The projector is **not** stuck — but that WARN now carries two very different meanings, so triage it (next section) instead of filtering it out.

For anything else, **fix the fold, then replay** — do not hand-advance `last_event_seq` past the event and do not delete it from `knowledge_events`. [[000456]] explicitly rejected "skip the failing event and advance the checkpoint" as a policy: it fixes the checkpoint at the cost of a permanently missing row in the read model, which is exactly the reproducibility an event-sourced projection exists to provide. Deploy the corrected fold first; the retry then clears the wedge by itself, and only if the read model is also corrupted do you need the reset below.

## Triaging the dismiss no-op WARN

Because this fold is deliberately non-fatal, the WARN is the *only* signal for both a harmless and a serious condition. [[000473]] accepted the condition on an "observability 前提" basis — that observability is this section.

1. **Benign — a client sent a stale or foreign `item_key`.** The key genuinely never produced a row. Nothing to do.
2. **Systemic — real user dismissals are being silently dropped.** The dismiss targets a version nobody reads: a projection-version cutover where live dismisses still name rows that exist only at the previous version, or a `user_id` / active-version resolution bug. The user keeps seeing items they already dismissed, `knowledge_home_items.dismissed_at` stays NULL, and nothing else in the system errors.

Both look identical in the log, so distinguish them by version. Take `projection_version=<V>` and `event_id=<E>` from the WARN:

```bash
docker exec alt-knowledge-sovereign-db-1 \
  psql -U sovereign -d knowledge_sovereign -P pager=off -c \
  "SELECT version, status, activated_at FROM knowledge_projection_versions ORDER BY version;
   SELECT user_id, payload->>'item_key' AS item_key, aggregate_id
     FROM knowledge_events WHERE event_id = '<E>';"
```

Then, with the `user_id` that returns:

```bash
docker exec alt-knowledge-sovereign-db-1 \
  psql -U sovereign -d knowledge_sovereign -P pager=off -c \
  "SELECT projection_version, count(*) AS total_rows, count(dismissed_at) AS dismissed
     FROM knowledge_home_items WHERE user_id = '<U>'
     GROUP BY projection_version ORDER BY projection_version;"
```

Read it as:

- `<V>` equals the `status='active'` version **and** that user already has rows at `<V>` → benign. The projector is writing where the read path reads; only this one key was never projected.
- `<V>` is not the active version, **or** the user's rows sit at a different `projection_version` than `<V>` → systemic. Every dismiss for that user is landing on a version that has no rows. Fix the version resolution or finish the cutover, then reproject (Recovery Procedure below) so the dismissals actually land.
- A WARN rate that steps up at a deploy or a version flip, rather than trickling, is the strongest tell for case 2. A steady low trickle spread across many users and keys is case 1.

## Recovery Procedure

0. **First rule out a poison-pill event (previous section).** If one is wedging the checkpoint, the reset below replays straight back into it and the read model stalls at the same `seq` again — clearing state does not help until the fold that rejects the event is fixed.

### Preferred: admin rebuild endpoint

`knowledge-sovereign` ships an admin endpoint that does exactly what Steps
1–3 below do by hand, but in one transaction and with a table allowlist
that refuses to touch `knowledge_events` / `knowledge_event_dedupes` /
`knowledge_user_events`. It is published at `127.0.0.1:9511` (the metrics
port, `:9501` in-container) and gated by the same admin bearer token as the
other `/admin/*` routes.

Preview which tables a rebuild would empty and which checkpoint it would reset:

```bash
export SOVEREIGN_ADMIN_TOKEN="$(cat secrets/sovereign_admin_token.txt)"
curl -s -H "Authorization: Bearer $SOVEREIGN_ADMIN_TOKEN" \
  http://127.0.0.1:9511/admin/projections/rebuild/targets
```

Then run the rebuild for the Knowledge Home target:

```bash
curl -s -X POST http://127.0.0.1:9511/admin/projections/rebuild \
  -H "Authorization: Bearer $SOVEREIGN_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"target":"knowledge-home"}'
```

The response is `{"target", "tables", "tables_truncated", "projector_name", "checkpoint_before"}`.
This empties `knowledge_home_items`, `today_digest_view`, and
`recall_candidate_view` and resets the `knowledge-home-projector` checkpoint
to 0 in the same transaction, so there is no gap for the always-running
projector to advance through unfolded (the failure mode PM-2026-010
documented for the old two-statement version of this reset). `knowledge-sovereign`
does not need to be stopped first — the in-process projector re-folds the
event log from the next tick.

### Fallback: manual reset

Use this only if the admin endpoint is unavailable.

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
6. If historical synthetic events are missing, trigger a fresh Knowledge Home backfill after the projector is healthy. `altctl home backfill trigger` currently fails immediately (`ErrNoBackfillExecutor`: no executor has been wired since ADR-000944 removed the job that used to drain it) -- see [[knowledge-home-reproject-operations]].

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
- There is no connection pooler on this path. `knowledge-sovereign` dials `knowledge-sovereign-db:5432` directly (`compose/sovereign.yaml`); PgBouncer fronts `alt-db` (`compose/core.yaml`) and Kratos (`compose/auth.yaml`) only. Transaction-pooling constraints — prepared-statement limits, `DISCARD ALL` between transactions, simple-protocol compatibility — do not apply to any command in this runbook.
