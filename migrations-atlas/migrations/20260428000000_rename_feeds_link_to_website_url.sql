-- Rename feeds.link → feeds.website_url to align with the ubiquitous
-- language glossary established in PR1 (ADR-000867 / docs/glossary/
-- ubiquitous-language.md): the RSS <channel><link> element value is a
-- Website URL (the URL of the website the feed describes), and we pin
-- it under that name in code/db/wire to avoid the URL-vs-Link
-- confusion class that drove PM-2026-041.
--
-- Distinct from feeds.url (RSS feed XML location) — the two stay
-- separate fields with separate names. Indexes and the unique
-- constraint follow the column rename.
--
-- PostgreSQL ALTER TABLE RENAME COLUMN is metadata-only — no row
-- rewrite, no index rebuild. Indexes get their column reference
-- updated automatically; we additionally rename the index OBJECTS
-- so DBA queries against catalog reflect the new term.

ALTER TABLE feeds RENAME COLUMN link TO website_url;

ALTER INDEX idx_feeds_link              RENAME TO idx_feeds_website_url;
ALTER INDEX idx_feeds_link_gin_trgm     RENAME TO idx_feeds_website_url_gin_trgm;
ALTER INDEX idx_feeds_id_link           RENAME TO idx_feeds_id_website_url;
ALTER INDEX idx_feeds_created_at_link   RENAME TO idx_feeds_created_at_website_url;
ALTER INDEX idx_feeds_created_desc_not_mp3 RENAME TO idx_feeds_website_url_created_desc_not_mp3;
ALTER INDEX idx_feeds_desc_not_mp3_cover RENAME TO idx_feeds_website_url_desc_not_mp3_cover;
ALTER INDEX unique_feeds_link           RENAME TO unique_feeds_website_url;

COMMENT ON COLUMN feeds.website_url IS
  'Website URL of the feed channel (RSS <channel><link> element value). Canonical Website URL per docs/glossary/ubiquitous-language.md. Renamed from `link` 2026-04-28.';
