-- Daily digest projection for TodayBar
CREATE TABLE today_digest_view (
  user_id               UUID NOT NULL,
  digest_date           DATE NOT NULL,
  new_articles          INTEGER NOT NULL DEFAULT 0,
  summarized_articles   INTEGER NOT NULL DEFAULT 0,
  unsummarized_articles INTEGER NOT NULL DEFAULT 0,
  top_tags_json         JSONB NOT NULL DEFAULT '[]',
  pulse_refs_json       JSONB NOT NULL DEFAULT '[]',
  updated_at            TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (user_id, digest_date)
);

COMMENT ON TABLE today_digest_view IS 'Daily digest projection for TodayBar';
