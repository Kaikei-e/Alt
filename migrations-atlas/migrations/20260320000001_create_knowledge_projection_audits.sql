-- Create table for tracking projection audit results.
-- Audits sample items from a projection and verify correctness
-- by re-computing from source events.
CREATE TABLE knowledge_projection_audits (
  audit_id            UUID PRIMARY KEY,
  projection_name     TEXT NOT NULL,
  projection_version  TEXT NOT NULL,
  checked_at          TIMESTAMPTZ NOT NULL,
  sample_size         INTEGER NOT NULL,
  mismatch_count      INTEGER NOT NULL,
  details_json        JSONB NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_projection_audits_name ON knowledge_projection_audits (projection_name, checked_at DESC);
