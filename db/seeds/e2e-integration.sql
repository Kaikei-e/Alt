-- E2E 統合テスト用シードデータ (固定ID)
-- Usage: altctl seed e2e
-- 確定的データ: テスト再現性のため固定IDを使用

BEGIN;

-- ========================================
-- E2E Test Feeds (固定ID)
-- ========================================
INSERT INTO feeds (id, title, description, link, pub_date)
VALUES
    ('00000000-0000-0000-0000-000000000101', 'E2E Test Feed Alpha', 'Primary test feed for E2E integration', 'https://e2e-test.example.com/alpha/rss', NOW()),
    ('00000000-0000-0000-0000-000000000102', 'E2E Test Feed Beta', 'Secondary test feed for E2E integration', 'https://e2e-test.example.com/beta/rss', NOW()),
    ('00000000-0000-0000-0000-000000000103', 'E2E Test Feed Gamma', 'Tertiary test feed for search testing', 'https://e2e-test.example.com/gamma/rss', NOW())
ON CONFLICT (id) DO UPDATE
    SET title = EXCLUDED.title,
        description = EXCLUDED.description,
        link = EXCLUDED.link,
        pub_date = EXCLUDED.pub_date,
        updated_at = NOW();

-- ========================================
-- E2E Test Articles (固定ID)
-- ========================================
INSERT INTO articles (id, title, content, url, feed_id, created_at)
VALUES
    ('00000000-0000-0000-0000-000000000201', 'Test Article: AI Trends',
     'Artificial intelligence continues to transform the technology landscape.',
     'https://e2e-test.example.com/articles/ai-trends',
     '00000000-0000-0000-0000-000000000101', NOW() - INTERVAL '2 hours'),

    ('00000000-0000-0000-0000-000000000202', 'Test Article: Svelte 5 Tips',
     'Svelte 5 introduces runes for more explicit reactivity.',
     'https://e2e-test.example.com/articles/svelte-5-tips',
     '00000000-0000-0000-0000-000000000101', NOW() - INTERVAL '4 hours'),

    ('00000000-0000-0000-0000-000000000203', 'Test Article: Docker Best Practices',
     'Container orchestration patterns for production deployments.',
     'https://e2e-test.example.com/articles/docker-best-practices',
     '00000000-0000-0000-0000-000000000102', NOW() - INTERVAL '1 day'),

    ('00000000-0000-0000-0000-000000000204', 'Test Article: Searchable Content',
     'This article contains unique keywords for search testing: e2e-search-token-xyz.',
     'https://e2e-test.example.com/articles/searchable-content',
     '00000000-0000-0000-0000-000000000103', NOW() - INTERVAL '6 hours')
ON CONFLICT (id) DO UPDATE
    SET title = EXCLUDED.title,
        content = EXCLUDED.content,
        url = EXCLUDED.url,
        feed_id = EXCLUDED.feed_id,
        created_at = EXCLUDED.created_at;

COMMIT;
