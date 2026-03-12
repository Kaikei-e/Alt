-- Seed baseline records for local development runs.
-- Idempotent so that repeated executions refresh the sample data safely.
-- NOTE: Tables are created by Atlas migrations which run AFTER this init script.
-- On first boot the tables don't exist yet, so we skip seeding gracefully.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'feeds') THEN
        RAISE NOTICE 'Skipping seed: feeds table does not exist yet (will be created by migrations)';
        RETURN;
    END IF;

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
            updated_at = NOW();

    INSERT INTO articles (title, content, url, feed_id, created_at)
    SELECT payload.title, payload.content, payload.url, f.id, payload.created_at
    FROM feeds f
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
    WHERE f.link = 'https://example.com/rss'
    ON CONFLICT (url) DO UPDATE
        SET title = EXCLUDED.title,
            content = EXCLUDED.content,
            feed_id = EXCLUDED.feed_id,
            created_at = EXCLUDED.created_at;
END;
$$;
