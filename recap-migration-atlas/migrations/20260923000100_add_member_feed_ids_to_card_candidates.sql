-- Full sorted member feed IDs for candidate clusters to support judgment inheritance across re-clustering.
ALTER TABLE recap_card_candidates
    ADD COLUMN member_feed_ids UUID[] NOT NULL DEFAULT '{}';
