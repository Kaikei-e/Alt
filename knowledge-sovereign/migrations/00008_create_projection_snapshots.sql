-- Projection Snapshots: periodic full state captures of projection tables.
-- Enables fast reproject (snapshot + incremental replay) and safe event archival.
-- After a verified snapshot, events older than event_seq_boundary can be archived.

CREATE TABLE knowledge_projection_snapshots (
  snapshot_id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  snapshot_type       TEXT NOT NULL DEFAULT 'full',
  projection_version  INT NOT NULL,
  projector_build_ref TEXT NOT NULL,
  schema_version      TEXT NOT NULL,
  snapshot_at         TIMESTAMPTZ NOT NULL,
  event_seq_boundary  BIGINT NOT NULL,
  snapshot_data_path  TEXT NOT NULL,
  items_row_count     INT NOT NULL,
  items_checksum      TEXT NOT NULL,
  digest_row_count    INT NOT NULL,
  digest_checksum     TEXT NOT NULL,
  recall_row_count    INT NOT NULL,
  recall_checksum     TEXT NOT NULL,
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  status              TEXT NOT NULL DEFAULT 'pending'
);

CREATE INDEX idx_kps_status_seq
  ON knowledge_projection_snapshots (status, event_seq_boundary DESC);

COMMENT ON TABLE knowledge_projection_snapshots
  IS 'Projection snapshots for fast reproject and safe event archival';
COMMENT ON COLUMN knowledge_projection_snapshots.status
  IS 'pending = export in progress, valid = verified and usable, invalidated = schema/projector changed, archived = old snapshot moved to cold storage';
COMMENT ON COLUMN knowledge_projection_snapshots.event_seq_boundary
  IS 'Events with event_seq <= this value are covered by the snapshot and can be archived after verification';
