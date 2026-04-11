# Knowledge Home Projection Recovery

This runbook restores Knowledge Home when the append-only event log is healthy but the read model is empty, stale, or malformed.

## Model

- `knowledge_events` is the canonical append-only event store.
- `knowledge_home_items`, `today_digest_view`, and `recall_candidate_view` are disposable projections.
- `knowledge_projection_checkpoints` stores projector progress only. It is not a source of truth.

This follows the repository's append-first / projection-later design and the immutable event model used by Knowledge Home.

## Symptoms

- Knowledge Home UI shows the warming-up empty state for active users.
- `knowledge_events` has rows but `knowledge_home_items` is empty or far behind.
- `alt-backend` logs show projection failures such as JSONB encoding errors.
- `knowledge_projection_checkpoints` is missing or not advancing.

## Preconditions

- The projection writer bug has already been fixed and deployed.
- `alt-backend` is healthy and connected through PgBouncer.
- You have shell access to the running Postgres container.

## Inspect Current State

```bash
docker exec alt-db sh -lc \
  "psql -U alt_db_user -d alt -P pager=off -c \
  \"SELECT count(*) AS events FROM knowledge_events;
    SELECT count(*) AS home_items FROM knowledge_home_items;
    SELECT projector_name, last_event_seq, updated_at FROM knowledge_projection_checkpoints;
    SELECT job_id, status, projection_version, total_events, processed_events, completed_at
      FROM knowledge_backfill_jobs
      ORDER BY created_at DESC LIMIT 5;\""
```

Expected healthy shape:

- `knowledge_events` is non-zero.
- `knowledge_home_items` is non-zero.
- `knowledge-home-projector` checkpoint exists and advances over time.

## Recovery Procedure

1. Stop or scale down `alt-backend` so the projector is not mutating state during cleanup.
2. Keep `knowledge_events` untouched.
3. Reset only the disposable projection state.

```bash
docker exec alt-db sh -lc \
  "psql -U alt_db_user -d alt -c \
  \"BEGIN;
     DELETE FROM knowledge_home_items;
     DELETE FROM today_digest_view;
     DELETE FROM recall_candidate_view;
     DELETE FROM knowledge_projection_checkpoints
       WHERE projector_name = 'knowledge-home-projector';
   COMMIT;\""
```

4. Start `alt-backend` again.
5. Let the projector replay from `knowledge_events`.
6. If historical synthetic events are missing, trigger a fresh Knowledge Home backfill after the projector is healthy.

## Verify Recovery

```bash
docker exec alt-db sh -lc \
  "psql -U alt_db_user -d alt -P pager=off -c \
  \"SELECT count(*) AS events FROM knowledge_events;
    SELECT count(*) AS home_items FROM knowledge_home_items;
    SELECT projector_name, last_event_seq, updated_at FROM knowledge_projection_checkpoints;\""
```

Also confirm:

- `alt-backend` logs no longer show `invalid input syntax for type json`.
- `knowledge_home_items` count increases.
- Knowledge Home UI no longer shows the warming-up empty state for affected users.

## Notes

- Do not patch `knowledge_home_items` manually. Rebuild it from the event log.
- Do not delete `knowledge_events` during this recovery.
- PgBouncer remains in transaction pooling mode for this workflow. The recovery assumes pgx simple-protocol compatibility is preserved.
