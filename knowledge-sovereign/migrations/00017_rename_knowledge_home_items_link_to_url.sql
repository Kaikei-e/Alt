-- Rename knowledge_home_items.link → url to align with the Article URL
-- canonical naming established in docs/glossary/ubiquitous-language.md (and
-- ADR-000867). Metadata-only change in PostgreSQL — no row rewrite, no
-- index rebuild (no index on this column references it by name). The
-- KnowledgeHomeItem.Link Go field and the proto KnowledgeHomeItem.link
-- field rename in lockstep with this column.
--
-- Order safety: Atlas migrations run before service container start, so
-- the new sovereign / alt-backend binaries will only ever see the new
-- column name. There is no rolling-deploy window where the old binary
-- could query the old name (per docs/runbooks/deploy.md sequencing).

ALTER TABLE knowledge_home_items RENAME COLUMN link TO url;

COMMENT ON COLUMN knowledge_home_items.url IS
  'Article source URL. Canonical Article URL per docs/glossary/ubiquitous-language.md. Renamed from `link` 2026-04-28.';
