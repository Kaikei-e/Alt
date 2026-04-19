-- e2e/fixtures/rag-orchestrator/db-seed.sql
--
-- Seeded augur chat history for the rag-orchestrator Hurl suite. Driven from
-- run.sh via `psql -v user_id=... -v conv_id=...` so UUIDs come from the
-- committed fixture files and the seed stays deterministic.
--
-- Append-only: ON CONFLICT DO NOTHING keeps the seed idempotent against a
-- persistent rag-db volume (the staging stack normally runs `down -v`, but a
-- `KEEP_STACK=1` debug loop would otherwise collide on the PK).

BEGIN;

INSERT INTO augur_conversations (id, user_id, title, created_at)
VALUES (
    :'conv_id'::uuid,
    :'user_id'::uuid,
    'E2E seed conversation',
    TIMESTAMP WITH TIME ZONE '2026-01-01 00:00:00+00'
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO augur_messages (id, conversation_id, role, content, citations, created_at)
VALUES
    (
        '22222222-2222-2222-2222-222222222221',
        :'conv_id'::uuid,
        'user',
        'What did the RSS reader log yesterday?',
        '[]'::jsonb,
        TIMESTAMP WITH TIME ZONE '2026-01-01 00:00:01+00'
    ),
    (
        '22222222-2222-2222-2222-222222222222',
        :'conv_id'::uuid,
        'assistant',
        'Seeded assistant reply for E2E.',
        '[]'::jsonb,
        TIMESTAMP WITH TIME ZONE '2026-01-01 00:00:02+00'
    )
ON CONFLICT (id) DO NOTHING;

COMMIT;
