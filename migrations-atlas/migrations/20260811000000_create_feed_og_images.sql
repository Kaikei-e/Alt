-- On-demand OG images, keyed on the feed.
--
-- Why a third home for an og:image, next to feeds.og_image_url and
-- article_heads.og_image_url:
--
--   feeds.og_image_url is the RSS-derived reference. It costs no extra HTTP
--   request and it points at someone else's URL rather than holding a copy of
--   their artifact, which is why it is never purged. Writing a scraped result
--   into it would quietly turn it into a derived artifact with no retention --
--   the one outcome the copyright window exists to prevent.
--
--   article_heads.og_image_url is the scraped result, and its article_id is
--   NOT NULL REFERENCES articles(id). That reference is the blocker: an
--   articles row only appears once someone fetches the content, and measured
--   against the live database only 349 of the 4305 image-less feeds inside the
--   retention window have one. Resolution keyed on the article can never reach
--   the other 92%.
--
-- So this table holds the scraped result keyed on the feed, and it lives under
-- the same 7-day retention window as article_heads: og-image-retention purges
-- it, and a reader who returns to the surface causes it to be re-acquired.
--
-- The row is also the failure memory, and that is not an optimisation. Rows are
-- written when a reader brings a card into view, so without a record of "this
-- origin already said no" every scroll past the same card would re-request
-- someone else's page. The batch job this table replaces had no failure memory
-- and re-fetched the same sixteen refusing URLs every thirty minutes for its
-- entire life, succeeding zero times.
CREATE TABLE feed_og_images (
  feed_id      UUID        PRIMARY KEY REFERENCES feeds (id) ON DELETE CASCADE,
  state        TEXT        NOT NULL CHECK (state IN ('resolved', 'unavailable')),
  og_image_url TEXT,
  -- Why the origin refused, for operators reading the table directly:
  -- 'robots_disallow', 'http_403', 'http_404', 'no_og_tag', 'fetch_error'.
  reason       TEXT,
  attempts     INT         NOT NULL DEFAULT 1,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
  -- When an 'unavailable' feed may be asked about again. NULL means never
  -- within this retention window, which is the correct answer for a robots.txt
  -- disallow: the origin's answer is a policy, not a transient fault.
  retry_after  TIMESTAMPTZ,

  -- 'resolved' means we hold a URL; 'unavailable' means we asked and were
  -- refused. Neither state may impersonate the other, because a resolved row
  -- with no URL would read to the query planner as a cache hit and suppress
  -- the retry that a genuine failure is owed.
  CONSTRAINT feed_og_images_state_matches_url CHECK (
    (state = 'resolved'    AND og_image_url IS NOT NULL AND og_image_url <> '') OR
    (state = 'unavailable' AND og_image_url IS NULL)
  )
);

-- The retention sweep deletes by age; the read path and the resolver both
-- reach a row by feed_id, which the primary key already serves.
CREATE INDEX idx_feed_og_images_created_at ON feed_og_images (created_at);

COMMENT ON TABLE feed_og_images IS
  'On-demand scraped og:image keyed on the feed, plus the failure memory that stops a refusing origin being re-requested on every scroll. Disposable: purged at 7 days by og-image-retention and re-acquired when a reader returns.';

COMMENT ON COLUMN feed_og_images.state IS
  'resolved = a URL was obtained; unavailable = the origin was asked and refused. The CHECK keeps the two from impersonating each other.';

COMMENT ON COLUMN feed_og_images.retry_after IS
  'When an unavailable feed may be asked about again. NULL means not within this retention window -- the right answer for a robots.txt disallow, which is a policy rather than a transient fault.';
