CREATE TABLE knowledge_projection_versions (
  version       INTEGER PRIMARY KEY,
  description   TEXT NOT NULL,
  status        TEXT NOT NULL DEFAULT 'pending',
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  activated_at  TIMESTAMPTZ
);
INSERT INTO knowledge_projection_versions (version, description, status, activated_at)
VALUES (1, 'Initial Knowledge Home projection', 'active', now());
