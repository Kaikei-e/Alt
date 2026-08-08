-- Transactional outbox for user-facing push notifications.
--
-- This table lives in pre-processor-db, not in alt-db, and that placement is
-- the whole point. An outbox only provides its guarantee — the message is sent
-- if and only if the business transaction commits — when the outbox row and the
-- business state change share one local commit. Summarize job state lives here,
-- so the outbox row has to live here too. Writing it into alt-db instead would
-- be a dual write wearing an outbox's name: the job would complete and the
-- notification would be silently lost whenever the second write failed.
--
-- A relay in pre-processor forwards rows to alt-data-hub over mTLS and marks
-- them forwarded. Delivery to the device is tracked separately, in alt-db's
-- push_deliveries.
CREATE TABLE notification_outbox (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Derived from the business fact (e.g. 'summary:<job_id>'), never generated
    -- at send time: a retry at any layer has to produce the same key, which is
    -- what makes re-forwarding harmless.
    dedupe_key      TEXT NOT NULL UNIQUE,
    user_id         UUID NOT NULL,
    kind            TEXT NOT NULL,
    payload         JSONB NOT NULL,
    -- Business time, supplied by the producer. Deliberately not defaulted: a
    -- wall-clock default would make the row's meaning depend on when it was
    -- inserted rather than on when the thing happened.
    occurred_at     TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    state           TEXT NOT NULL DEFAULT 'pending'
                      CHECK (state IN ('pending', 'forwarding', 'forwarded', 'dead')),
    attempts        INT NOT NULL DEFAULT 0,
    -- Doubles as the claim lease: the relay stamps it forward when it claims a
    -- row, so a row orphaned by a crashed relay re-enters the same claim query
    -- once the lease expires. There is no separate reclaim sweeper to forget.
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    locked_by       TEXT,
    last_error      TEXT,
    forwarded_at    TIMESTAMPTZ
);

-- Partial over non-terminal states only, so the index is sized by backlog
-- rather than by history and does not accumulate dead entries as rows finalize.
CREATE INDEX notification_outbox_claim_idx
    ON notification_outbox (next_attempt_at, id)
    WHERE state IN ('pending', 'forwarding');

COMMENT ON TABLE notification_outbox IS 'Transactional outbox for push notifications (forwarded to alt-data-hub by the pre-processor relay)';
