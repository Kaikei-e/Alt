-- Knowledge Trail branch read model (CQRS projection, rebuildable from
-- trail.branch_proposed.v1 / trail.branch_resolved.v1 events). A branch is a
-- system-proposed next step on the spine. The four-tuple (relation_kind, why,
-- evidence_refs, confidence) is NOT NULL at the schema level — a branch row
-- cannot exist without it, enforcing "no untyped branch" (the Loop decorated-feed
-- failure) end to end.
CREATE TABLE knowledge_trail_branches (
  user_id            UUID NOT NULL,
  tenant_id          UUID NOT NULL,
  branch_key         TEXT NOT NULL,
  anchor_item_key    TEXT NOT NULL,
  relation_kind      TEXT NOT NULL,
  why                TEXT NOT NULL,
  evidence_refs_json JSONB NOT NULL DEFAULT '[]',
  confidence         TEXT NOT NULL,
  target_item_key    TEXT NOT NULL,
  target_title       TEXT NOT NULL DEFAULT '',
  state              TEXT NOT NULL DEFAULT 'open',
  created_at         TIMESTAMPTZ NOT NULL,
  projection_version INT NOT NULL DEFAULT 1,
  PRIMARY KEY (user_id, branch_key)
);

-- Open branches are read per user; resolved branches drop out of the hot path.
CREATE INDEX idx_trail_branches_user_open
  ON knowledge_trail_branches (user_id, created_at DESC)
  WHERE state = 'open';

COMMENT ON TABLE knowledge_trail_branches IS 'Knowledge Trail branch read model (CQRS projection, rebuildable from trail.branch_* events)';
