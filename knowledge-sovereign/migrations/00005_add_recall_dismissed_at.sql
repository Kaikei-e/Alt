-- Add dismissed_at column to recall_candidate_view for soft-delete dismiss.
-- Previously, dismiss was a hard DELETE causing candidates to reappear
-- on the next projector run (60s later) because signals were retained.

ALTER TABLE recall_candidate_view
  ADD COLUMN dismissed_at TIMESTAMPTZ;
