-- The Web Push dispatcher's delivery queue: one row per (notification,
-- device).
--
-- This is deliberately NOT called notification_outbox, and it is not one. A
-- transactional outbox is written in the same transaction as the business
-- state change it describes, and none of the three producers' state lives in
-- this database: summarize job state is in pre-processor-db, report_jobs is in
-- acolyte-db, recap job state is in recap-db. A producer inserting into an
-- alt-db outbox would be performing the dual write the outbox pattern exists
-- to remove. Each producer therefore keeps its own notification_outbox in its
-- own database and a relay forwards to data-hub; this table is downstream of
-- that relay.
--
-- The name also has to differ from the producers'. ADR-000961 gates on the
-- same table name being claimed by two Atlas directories, which is how a table
-- moved to a new service but never dropped from the old database is detected.
-- Three service databases holding an identically-shaped notification_outbox is
-- fine under that rule; alt-db holding one too would look exactly like the
-- defect the gate hunts for, and the fix for that is a different name rather
-- than an exemption entry.
--
-- The grain is per device because one enqueue fans out to every subscription
-- the user has registered, and the devices succeed and fail independently: a
-- phone that answers 410 must not stop the desktop's copy.
--
-- Two columns carry the rest of the design and neither is negotiable:
--
--   dedupe_key is derived from the business fact ("recap:<job_id>"), never
--   generated at send time. A relayed retry carries the same key, so
--   UNIQUE (dedupe_key, subscription_id) collapses it; a key minted per
--   attempt would turn every retry into a second push to the user's phone.
--
--   next_attempt_at *is* the lease. Claiming sets state='sending' and pushes
--   next_attempt_at forward by the lease window, so a row orphaned by a
--   crashed dispatcher re-enters the very same claim query once the lease
--   expires. There is no separate reclaim sweeper, and therefore no separate
--   reclaim sweeper to forget to write or to deploy.
--
-- occurred_at is business time and is always supplied by the producer: it is
-- when the thing happened, not when this row reached the database, and a
-- default here would quietly make the two the same.
CREATE TABLE push_deliveries (
  id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  dedupe_key      TEXT        NOT NULL,
  subscription_id UUID        NOT NULL REFERENCES push_subscriptions (id) ON DELETE CASCADE,
  user_id         UUID        NOT NULL,
  kind            TEXT        NOT NULL,
  payload         JSONB       NOT NULL,
  occurred_at     TIMESTAMPTZ NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  state           TEXT        NOT NULL DEFAULT 'pending'
                              CHECK (state IN ('pending', 'sending', 'sent', 'dead', 'expired')),
  attempts        INT         NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  expires_at      TIMESTAMPTZ NOT NULL,
  locked_by       TEXT,
  last_status     INT,
  last_error      TEXT,
  finalized_at    TIMESTAMPTZ,
  -- Re-delivering the same enqueue to the same device is a no-op rather than
  -- a second push.
  UNIQUE (dedupe_key, subscription_id)
);

-- The claim index, partial over non-terminal states only. Its size is bounded
-- by the backlog rather than by history: sent/dead/expired rows accumulate for
-- forensics and never enter this index, so the claim plan does not degrade as
-- the table grows.
CREATE INDEX idx_push_deliveries_claim
  ON push_deliveries (next_attempt_at, id)
  WHERE state IN ('pending', 'sending');

-- A newer daily digest supersedes an unsent older one instead of stacking. The
-- digest is "today's entrance", sent once per day; two of them queued behind a
-- device that was offline overnight would deliver yesterday's as well. Keyed
-- by subscription rather than by user because the collapse is per device — one
-- device being offline says nothing about another's queue — and scoped to
-- state='pending' so a digest already handed to a dispatcher is not blocked by
-- the next day's enqueue.
CREATE UNIQUE INDEX idx_push_deliveries_pending_digest
  ON push_deliveries (subscription_id, kind)
  WHERE state = 'pending' AND kind = 'today_entrance_ready';

COMMENT ON TABLE push_deliveries IS 'Web Push dispatcher delivery queue, one row per (notification, device); next_attempt_at doubles as the claim lease';

COMMENT ON COLUMN push_deliveries.dedupe_key IS
  'Derived from the business fact (e.g. recap:<job_id>). Must be reproducible by a retry of the producing operation — never generated at send time.';

COMMENT ON COLUMN push_deliveries.occurred_at IS
  'Business time, always supplied by the producer. Deliberately has no default: a wall-clock default would make it a record of when the row was written.';

COMMENT ON COLUMN push_deliveries.next_attempt_at IS
  'Both the backoff schedule and the claim lease. A sending row whose lease has expired is claimable again, which is how a crashed dispatcher recovers without a reclaim sweeper.';
