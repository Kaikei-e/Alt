-- Knowledge Trail act-outcome side table (CQRS projection, rebuildable from
-- knowledge_events). One row per observed consequence of a taken branch:
-- trail.act_outcome.v1 rows carry the raw dwell_ms; historical
-- knowledge_loop.act_outcome.v1 rows keep their era's classified label
-- verbatim in legacy_outcome (never faked into milliseconds). Outcomes never
-- add rows to the spine — the read path joins this table into the path-wear
-- derivation only. Insert-only; first write wins per (user_id, outcome_key).
CREATE TABLE knowledge_trail_act_outcomes (
  user_id            UUID NOT NULL,
  tenant_id          UUID NOT NULL,
  outcome_key        TEXT NOT NULL,
  branch_key         TEXT NOT NULL DEFAULT '',
  item_key           TEXT NOT NULL,
  dwell_ms           BIGINT,
  legacy_outcome     TEXT NOT NULL DEFAULT '',
  source_event_type  TEXT NOT NULL,
  occurred_at        TIMESTAMPTZ NOT NULL,
  projection_version INT NOT NULL DEFAULT 2,
  PRIMARY KEY (user_id, outcome_key)
);

-- Path wear aggregates engagement per (user, item) at read time.
CREATE INDEX idx_trail_act_outcomes_user_item
  ON knowledge_trail_act_outcomes (user_id, item_key);

COMMENT ON TABLE knowledge_trail_act_outcomes IS 'Knowledge Trail act-outcome side table (CQRS projection, rebuildable from knowledge_events; feeds path wear, never the spine)';
