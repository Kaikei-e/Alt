-- today_digest_view's additive counters need a replay guard, and until now
-- that guard was `EXCLUDED.updated_at > today_digest_view.updated_at`.
-- updated_at is the source event's occurred_at, which every producer stamps
-- with its own wall clock before the append RPC, while knowledge_home_projector
-- folds in event_seq order. Two events for the same user and day whose clocks
-- disagree therefore arrive with occurred_at going backwards, and the
-- older-looking one had its whole DO UPDATE — counter deltas included —
-- discarded, leaving TodayBar permanently one short.
--
-- event_seq is the log's own monotonic order and is exactly what the fold
-- iterates, so it is the only discriminator that means "already folded into
-- this row" instead of "some other machine's clock is ahead". This is the
-- same role last_event_seq already plays in knowledge_projection_checkpoints.
--
-- DEFAULT 0 sits below every real event_seq (knowledge_events.event_seq is
-- BIGSERIAL, hence >= 1), so rows written before this migration lose the
-- guard against the next event rather than being stranded above it.
-- today_digest_view is a disposable projection: RebuildProjection truncates
-- it and replays the log, refilling the column exactly.

ALTER TABLE today_digest_view
  ADD COLUMN IF NOT EXISTS last_event_seq BIGINT NOT NULL DEFAULT 0;

COMMENT ON COLUMN today_digest_view.last_event_seq
  IS 'Highest knowledge_events.event_seq folded into this row; replay guard for the additive counters';
