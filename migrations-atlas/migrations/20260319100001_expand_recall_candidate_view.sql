-- Expand recall_candidate_view with projection version, eligibility, and snooze support.
ALTER TABLE recall_candidate_view
  ADD COLUMN IF NOT EXISTS projection_version INTEGER NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS first_eligible_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS snoozed_until TIMESTAMPTZ;

CREATE INDEX idx_recall_candidates_suggest
  ON recall_candidate_view (user_id, next_suggest_at ASC)
  WHERE next_suggest_at IS NOT NULL AND snoozed_until IS NULL;
