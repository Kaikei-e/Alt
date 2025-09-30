-- Seed baseline records for local development runs.
-- Idempotent so that repeated executions refresh the sample data safely.

BEGIN;

WITH upsert_feed AS (
    INSERT INTO feeds (title, description, link, pub_date)
    VALUES (
        'Alt Sample Feed',
        'Sample RSS feed to keep local environments populated after database resets.',
        'https://example.com/rss',
        NOW()
    )
    ON CONFLICT (link) DO UPDATE
        SET title = EXCLUDED.title,
            description = EXCLUDED.description,
            pub_date = EXCLUDED.pub_date,
            updated_at = NOW()
    RETURNING id
),
seed_articles AS (
    SELECT id AS feed_id FROM upsert_feed
)
INSERT INTO articles (title, content, url, feed_id, created_at)
SELECT payload.title, payload.content, payload.url, seed.feed_id, payload.created_at
FROM seed_articles seed
CROSS JOIN (
    VALUES
        (
            'Sample Article: Welcome to Alt',
            'This is placeholder content created during local seeding. Feel free to replace it with real data.',
            'https://example.com/articles/welcome-to-alt',
            NOW()
        ),
        (
            'Sample Article: Staying Productive',
            'Another sample article injected for development use. Update or delete as needed.',
            'https://example.com/articles/staying-productive',
            NOW() - INTERVAL '1 day'
        )
) AS payload(title, content, url, created_at)
ON CONFLICT (url) DO UPDATE
    SET title = EXCLUDED.title,
        content = EXCLUDED.content,
        feed_id = EXCLUDED.feed_id,
        created_at = EXCLUDED.created_at;

COMMIT;
