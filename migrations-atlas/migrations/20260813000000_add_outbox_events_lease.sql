-- Give the outbox claim a lease, the way push_deliveries has one.
--
-- The claim commits its rows as PROCESSING and the worker moves them to
-- PROCESSED or FAILED afterwards. Nothing else ever touches a PROCESSING row:
-- the claim query read PENDING only, and the prune deletes PROCESSED only. So
-- a harvester killed between the two — SIGKILL, OOM — stranded its whole batch
-- permanently. Those articles are never RAG-indexed and their ArticleCreated is
-- never emitted, with no error anywhere and no metric that moves.
--
-- next_attempt_at *is* the lease, exactly as in push_deliveries: claiming sets
-- status='PROCESSING' and pushes next_attempt_at forward by the lease window,
-- so an orphaned row re-enters the very same claim query once the lease
-- expires. There is no separate reclaim sweeper, and therefore none to forget
-- to write or to deploy.
--
-- The default is now() rather than clock_timestamp() for two reasons: the row
-- is inserted inside the business transaction that produced the event, so the
-- transaction clock is the same clock created_at on this table already uses;
-- and a non-volatile default lets ADD COLUMN take the fast path instead of
-- rewriting the table under an exclusive lock. Existing rows are backfilled to
-- the migration's own timestamp, which makes every one of them immediately due
-- -- the correct outcome for the PROCESSING rows already stranded in
-- production.
ALTER TABLE outbox_events
  ADD COLUMN next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- The claim index, partial over the non-terminal statuses only. Its size is
-- bounded by the backlog rather than by history: PROCESSED and FAILED rows sit
-- in the table until the retention prune reaches them and never enter this
-- index, so the claim plan does not degrade as they accumulate. Ordered by
-- created_at because an outbox delivers oldest event first.
CREATE INDEX idx_outbox_events_claim
  ON outbox_events (created_at)
  WHERE status IN ('PENDING', 'PROCESSING');

COMMENT ON COLUMN outbox_events.next_attempt_at IS
  'The claim lease. A PROCESSING row past this instant is claimable again, which is how a crashed harvester recovers its batch without a reclaim sweeper.';
