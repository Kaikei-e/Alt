-- Counts of embedding-cache hits and misses per cards job, kept out of cards_dropped so drop-rate metrics stay pure.
ALTER TABLE recap_card_job_stats
    ADD COLUMN embed_cache_hits   INT NOT NULL DEFAULT 0,
    ADD COLUMN embed_cache_misses INT NOT NULL DEFAULT 0;
