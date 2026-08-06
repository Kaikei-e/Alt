-- e2e/playwright/rag-orchestrator/setup/db-seed.sql
--
-- Augur chat history for the Playwright suite, applied by run.sh through
-- `docker compose exec -T rag-db psql` after the Atlas migrator has completed.
--
-- Why SQL and not the API
-- -----------------------
-- The rule in e2e/playwright/README.md is that every test seeds what it reads.
-- Augur has no write RPC that can honour it: the only path that creates an
-- `augur_conversations` row is `AugurService/StreamChat`, which runs the whole
-- RAG pipeline through the embedder, search-indexer and the LLM — all three of
-- which are parked at `http://127.0.0.1:9` in this slice
-- (compose/compose.staging.yaml). So the rows arrive out of band, and the
-- parallel-safety property is bought a different way: **one owner per role**.
-- Every user_id below is touched by exactly one group of tests, and the single
-- destructive test owns a conversation nothing else reads. See src/seed.ts,
-- which mirrors these identifiers and documents the ownership table; the two
-- files must be edited together.
--
-- Append-only, and idempotent via ON CONFLICT DO NOTHING so a `KEEP_STACK=1`
-- debug loop that reuses the rag-db volume re-establishes the pre-state instead
-- of failing on the primary key. (The delete test destroys one conversation, so
-- re-running against a warm stack genuinely needs the re-insert.)

BEGIN;

-- ---------------------------------------------------------------------------
-- History: one conversation with two turns, read by tests/augur-history.spec.ts
-- and by the seed probe in setup/global-setup.ts. Read-only for every test.
-- ---------------------------------------------------------------------------
INSERT INTO augur_conversations (id, user_id, title, created_at) VALUES
    ('0e2e0001-0000-0000-0000-000000000001',
     '00000000-0000-0000-0000-00000e2e0001',
     'E2E seed conversation',
     TIMESTAMP WITH TIME ZONE '2026-01-01 00:00:00+00')
ON CONFLICT (id) DO NOTHING;

INSERT INTO augur_messages (id, conversation_id, role, content, citations, related_citations, created_at) VALUES
    ('0e2e1001-0000-0000-0000-000000000001',
     '0e2e0001-0000-0000-0000-000000000001',
     'user',
     'What did the RSS reader log yesterday?',
     '[]'::jsonb,
     '[]'::jsonb,
     TIMESTAMP WITH TIME ZONE '2026-01-01 00:00:01+00'),
    -- The citation array is the point of this row. It carries one of each
    -- discriminator the read path has to reconstruct through
    -- `protoCitationKind` (augur/handler.go:655) plus the ADR-926 regression
    -- case: a citation whose title is a bare UUID, which
    -- `sanitizeCitationTitle` (handler.go:539) must blank so the UI never
    -- renders an internal identifier as visible text.
    ('0e2e1001-0000-0000-0000-000000000002',
     '0e2e0001-0000-0000-0000-000000000001',
     'assistant',
     'Seeded assistant reply for E2E.',
     '[
        {"url": "", "title": "Seeded article citation", "kind": "article",
         "ref_id": "0e2e9001-0000-0000-0000-000000000001", "published_at": "2026-01-01T00:00:00Z"},
        {"url": "https://example.invalid/e2e-seed-web", "title": "Seeded web citation", "kind": "web"},
        {"url": "", "title": "0e2e9001-0000-0000-0000-000000000002", "kind": "article",
         "ref_id": "0e2e9001-0000-0000-0000-000000000002"}
      ]'::jsonb,
     '[
        {"url": "https://example.invalid/e2e-seed-related", "title": "Seeded related citation", "kind": "web"}
      ]'::jsonb,
     TIMESTAMP WITH TIME ZONE '2026-01-01 00:00:02+00')
ON CONFLICT (id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Paging: three conversations for one owner, with distinct last-activity
-- instants so `ORDER BY last_activity_at DESC, id DESC`
-- (repository/augur_conversation_repo.go:165) is total, not a tie broken by
-- insertion order. Read-only for every test.
-- ---------------------------------------------------------------------------
INSERT INTO augur_conversations (id, user_id, title, created_at) VALUES
    ('0e2e0002-0000-0000-0000-000000000001',
     '00000000-0000-0000-0000-00000e2e0003',
     'E2E paging conversation 1',
     TIMESTAMP WITH TIME ZONE '2026-02-01 00:00:00+00'),
    ('0e2e0002-0000-0000-0000-000000000002',
     '00000000-0000-0000-0000-00000e2e0003',
     'E2E paging conversation 2',
     TIMESTAMP WITH TIME ZONE '2026-02-02 00:00:00+00'),
    ('0e2e0002-0000-0000-0000-000000000003',
     '00000000-0000-0000-0000-00000e2e0003',
     'E2E paging conversation 3',
     TIMESTAMP WITH TIME ZONE '2026-02-03 00:00:00+00')
ON CONFLICT (id) DO NOTHING;

INSERT INTO augur_messages (id, conversation_id, role, content, citations, related_citations, created_at) VALUES
    ('0e2e1002-0000-0000-0000-000000000001',
     '0e2e0002-0000-0000-0000-000000000001',
     'user', 'Paging turn 1', '[]'::jsonb, '[]'::jsonb,
     TIMESTAMP WITH TIME ZONE '2026-02-01 00:00:10+00'),
    ('0e2e1002-0000-0000-0000-000000000002',
     '0e2e0002-0000-0000-0000-000000000002',
     'user', 'Paging turn 2', '[]'::jsonb, '[]'::jsonb,
     TIMESTAMP WITH TIME ZONE '2026-02-02 00:00:10+00'),
    ('0e2e1002-0000-0000-0000-000000000003',
     '0e2e0002-0000-0000-0000-000000000003',
     'user', 'Paging turn 3', '[]'::jsonb, '[]'::jsonb,
     TIMESTAMP WITH TIME ZONE '2026-02-03 00:00:10+00')
ON CONFLICT (id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Deletable: owned by a user nothing else touches, so the one destructive test
-- in the suite cannot race a sibling worker's list or read.
-- ---------------------------------------------------------------------------
INSERT INTO augur_conversations (id, user_id, title, created_at) VALUES
    ('0e2e0003-0000-0000-0000-000000000001',
     '00000000-0000-0000-0000-00000e2e0004',
     'E2E deletable conversation',
     TIMESTAMP WITH TIME ZONE '2026-03-01 00:00:00+00')
ON CONFLICT (id) DO NOTHING;

INSERT INTO augur_messages (id, conversation_id, role, content, citations, related_citations, created_at) VALUES
    ('0e2e1003-0000-0000-0000-000000000001',
     '0e2e0003-0000-0000-0000-000000000001',
     'user', 'This conversation exists to be deleted.', '[]'::jsonb, '[]'::jsonb,
     TIMESTAMP WITH TIME ZONE '2026-03-01 00:00:10+00')
ON CONFLICT (id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Protected: the target of the cross-user delete attempt. Its owner reads it
-- back afterwards, which is what turns "DeleteConversation answered 200" into
-- "the `AND user_id = $2` predicate in repo.DeleteConversation actually held".
-- ---------------------------------------------------------------------------
INSERT INTO augur_conversations (id, user_id, title, created_at) VALUES
    ('0e2e0004-0000-0000-0000-000000000001',
     '00000000-0000-0000-0000-00000e2e0005',
     'E2E protected conversation',
     TIMESTAMP WITH TIME ZONE '2026-04-01 00:00:00+00')
ON CONFLICT (id) DO NOTHING;

INSERT INTO augur_messages (id, conversation_id, role, content, citations, related_citations, created_at) VALUES
    ('0e2e1004-0000-0000-0000-000000000001',
     '0e2e0004-0000-0000-0000-000000000001',
     'user', 'Only my owner may delete me.', '[]'::jsonb, '[]'::jsonb,
     TIMESTAMP WITH TIME ZONE '2026-04-01 00:00:10+00'),
    ('0e2e1004-0000-0000-0000-000000000002',
     '0e2e0004-0000-0000-0000-000000000001',
     'assistant', 'Acknowledged.', '[]'::jsonb, '[]'::jsonb,
     TIMESTAMP WITH TIME ZONE '2026-04-01 00:00:11+00')
ON CONFLICT (id) DO NOTHING;

COMMIT;
