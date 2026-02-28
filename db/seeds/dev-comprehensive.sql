-- 開発用リッチシードデータ
-- Usage: altctl seed dev
-- Idempotent: ON CONFLICT で安全に再実行可能

BEGIN;

-- ========================================
-- Feeds (複数フィード)
-- ========================================
INSERT INTO feeds (title, description, link, pub_date)
VALUES
    ('Hacker News', 'Links for the intellectually curious', 'https://news.ycombinator.com/rss', NOW()),
    ('TechCrunch', 'Startup and Technology News', 'https://techcrunch.com/feed/', NOW()),
    ('The Verge', 'Technology, Science, Art, and Culture', 'https://www.theverge.com/rss/index.xml', NOW()),
    ('Ars Technica', 'Serving the Technologist', 'https://feeds.arstechnica.com/arstechnica/index', NOW()),
    ('CSS Tricks', 'Tips, Tricks, and Techniques on using CSS', 'https://css-tricks.com/feed/', NOW()),
    ('Go Blog', 'The Go Programming Language Blog', 'https://go.dev/blog/feed.atom', NOW()),
    ('Svelte Blog', 'News from the Svelte team', 'https://svelte.dev/blog/rss.xml', NOW())
ON CONFLICT (link) DO UPDATE
    SET title = EXCLUDED.title,
        description = EXCLUDED.description,
        pub_date = EXCLUDED.pub_date,
        updated_at = NOW();

-- ========================================
-- Articles (各フィードに複数記事)
-- ========================================
WITH feed_ids AS (
    SELECT id, link FROM feeds
    WHERE link IN (
        'https://news.ycombinator.com/rss',
        'https://techcrunch.com/feed/',
        'https://www.theverge.com/rss/index.xml',
        'https://go.dev/blog/feed.atom',
        'https://svelte.dev/blog/rss.xml'
    )
)
INSERT INTO articles (title, content, url, feed_id, created_at)
SELECT payload.title, payload.content, payload.url, f.id, payload.created_at
FROM feed_ids f
CROSS JOIN LATERAL (
    VALUES
        (
            'Understanding Go Generics in 2025',
            'A comprehensive guide to using generics effectively in Go.',
            f.link || '/article/go-generics-2025',
            NOW() - INTERVAL '1 hour'
        ),
        (
            'Building Scalable Microservices',
            'Best practices for microservice architecture with Docker Compose.',
            f.link || '/article/scalable-microservices',
            NOW() - INTERVAL '3 hours'
        ),
        (
            'The Future of Web Components',
            'How web components are evolving with new browser APIs.',
            f.link || '/article/web-components-future',
            NOW() - INTERVAL '1 day'
        )
) AS payload(title, content, url, created_at)
ON CONFLICT (url) DO UPDATE
    SET title = EXCLUDED.title,
        content = EXCLUDED.content,
        feed_id = EXCLUDED.feed_id,
        created_at = EXCLUDED.created_at;

COMMIT;
