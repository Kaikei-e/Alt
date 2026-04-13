package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AugurConversation is the parent row for a persisted Ask Augur chat.
// All fields are write-once: the row is INSERTed at conversation start and
// never UPDATEd. The "last activity" signal used to sort the history list is
// derived from AugurMessage rows via the augur_conversation_index view.
type AugurConversation struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Title     string
	CreatedAt time.Time
}

// AugurMessage is a single append-only turn in a conversation. No UPDATE path.
type AugurMessage struct {
	ID             uuid.UUID
	ConversationID uuid.UUID
	Role           string // "user" or "assistant"
	Content        string
	Citations      []AugurCitation
	CreatedAt      time.Time
}

// AugurCitation is persisted inside augur_messages.citations (JSONB).
type AugurCitation struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	PublishedAt string `json:"published_at,omitempty"`
}

// AugurConversationSummary is the disposable read model for the history list.
// Mirrors the augur_conversation_index view; never persisted as a table.
type AugurConversationSummary struct {
	ID                 uuid.UUID
	UserID             uuid.UUID
	Title              string
	CreatedAt          time.Time
	LastActivityAt     time.Time
	MessageCount       int
	LastMessagePreview string
}

// AugurConversationRepository manages augur_conversations and augur_messages.
// All writes are INSERTs (or cascading DELETE). Read paths that need activity
// ordering must go through ListSummaries, which queries the view.
type AugurConversationRepository interface {
	// CreateConversation inserts a new conversation row. Title must be non-empty
	// (derived from the first user turn). CreatedAt is set by the caller.
	CreateConversation(ctx context.Context, conv *AugurConversation) error

	// GetConversation loads a conversation by id, enforcing user ownership.
	// Returns nil, nil if not found or owned by a different user.
	GetConversation(ctx context.Context, id uuid.UUID, userID uuid.UUID) (*AugurConversation, error)

	// AppendMessage inserts a single message. Caller is responsible for setting
	// ID and CreatedAt. Citations is serialized to JSONB.
	AppendMessage(ctx context.Context, msg *AugurMessage) error

	// ListMessages returns every message in a conversation, ordered by created_at.
	ListMessages(ctx context.Context, conversationID uuid.UUID) ([]AugurMessage, error)

	// ListSummaries returns the caller's conversations sorted by last activity
	// (derived from augur_conversation_index). Keyset pagination uses
	// (last_activity_at, id) descending.
	ListSummaries(ctx context.Context, userID uuid.UUID, limit int, afterActivity *time.Time, afterID *uuid.UUID) ([]AugurConversationSummary, error)

	// DeleteConversation removes the conversation and cascades its messages.
	// No-op if the row is missing or owned by a different user.
	DeleteConversation(ctx context.Context, id uuid.UUID, userID uuid.UUID) error
}
