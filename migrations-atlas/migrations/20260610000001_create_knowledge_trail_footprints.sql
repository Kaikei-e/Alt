-- Knowledge Trail spine read model (CQRS projection, rebuildable from knowledge_events).
-- A footprint is the pure projection of one user cognitive act (read / asked /
-- returned / listened / dismissed) derived from the append-only event log. The
-- projector reads event payload only; this table is disposable and re-derivable.
--
-- Display fields (title / excerpt / tags) are intentionally NOT stored here: the
-- read path enriches footprints via a LEFT JOIN to knowledge_home_items by
-- (user_id, item_key). That keeps the projector a pure event fold (no cross-model
-- reads during projection) while still giving the spine human-readable labels.
CREATE TABLE knowledge_trail_footprints (
  user_id            UUID NOT NULL,
  tenant_id          UUID NOT NULL,
  footprint_key      TEXT NOT NULL,
  verb               TEXT NOT NULL,
  item_key           TEXT NOT NULL,
  note               TEXT,
  source_event_type  TEXT NOT NULL,
  occurred_at        TIMESTAMPTZ NOT NULL,
  projection_version INT NOT NULL DEFAULT 1,
  PRIMARY KEY (user_id, footprint_key)
);

-- Spine read is a reverse-chronological scan per user; the cursor walks
-- (occurred_at, footprint_key) descending.
CREATE INDEX idx_trail_footprints_user_time
  ON knowledge_trail_footprints (user_id, occurred_at DESC, footprint_key DESC);

COMMENT ON TABLE knowledge_trail_footprints IS 'Knowledge Trail spine read model (CQRS projection, rebuildable from knowledge_events)';
