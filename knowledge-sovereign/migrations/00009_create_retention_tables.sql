-- Retention infrastructure: log table + user event daily aggregates

-- 1. Retention execution log
CREATE TABLE knowledge_retention_log (
  log_id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  action        TEXT NOT NULL,   -- export, verify, aggregate, detach, drop
  target_table  TEXT NOT NULL,
  target_partition TEXT,
  rows_affected BIGINT NOT NULL DEFAULT 0,
  archive_path  TEXT,
  checksum      TEXT,
  dry_run       BOOLEAN NOT NULL DEFAULT true,
  status        TEXT NOT NULL DEFAULT 'success', -- success, failed
  error_message TEXT,
  metadata_json JSONB NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_retention_log_run
  ON knowledge_retention_log (run_at DESC);

COMMENT ON TABLE knowledge_retention_log
  IS 'Audit log for retention/archival operations';

-- 2. User event daily aggregates (analysis purposes, not for reproject)
CREATE TABLE knowledge_user_event_aggregates (
  aggregate_id    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id         UUID NOT NULL,
  tenant_id       UUID NOT NULL,
  aggregate_date  DATE NOT NULL,
  event_type      TEXT NOT NULL,
  event_count     INT NOT NULL,
  unique_items    INT NOT NULL,
  payload_summary JSONB,
  UNIQUE (user_id, aggregate_date, event_type)
);

CREATE INDEX idx_kue_agg_user_date
  ON knowledge_user_event_aggregates (user_id, aggregate_date DESC);

COMMENT ON TABLE knowledge_user_event_aggregates
  IS 'Daily aggregated user events for analysis (raw events archived after retention window)';
