-- augur_conversations: append-first rows. title is write-once at creation,
-- no updated_at, no denormalized counters. See ADR on Ask Augur chat persistence.
CREATE TABLE augur_conversations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    title TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_augur_conversations_user_created
    ON augur_conversations (user_id, created_at DESC);

-- augur_messages: append-only log of chat turns. No UPDATE, only INSERT and
-- CASCADE DELETE when the parent conversation is removed by the user.
CREATE TABLE augur_messages (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    conversation_id UUID NOT NULL REFERENCES augur_conversations(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
    content TEXT NOT NULL,
    citations JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_augur_messages_conversation_created
    ON augur_messages (conversation_id, created_at);

-- Disposable read projection for the history list UI. "Last activity",
-- message count and preview are derived from augur_messages; they are never
-- stored on augur_conversations to keep writes append-only.
CREATE VIEW augur_conversation_index AS
SELECT
    c.id,
    c.user_id,
    c.title,
    c.created_at,
    COALESCE(m.last_activity_at, c.created_at) AS last_activity_at,
    COALESCE(m.message_count, 0)               AS message_count,
    m.last_message_preview
FROM augur_conversations c
LEFT JOIN LATERAL (
    SELECT
        MAX(created_at) AS last_activity_at,
        COUNT(*)::int   AS message_count,
        (SELECT LEFT(content, 140)
            FROM augur_messages
            WHERE conversation_id = c.id
            ORDER BY created_at DESC
            LIMIT 1) AS last_message_preview
    FROM augur_messages
    WHERE conversation_id = c.id
) m ON TRUE;
