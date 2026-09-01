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
//
// RelatedCitations carries the inline-projected snapshot of articles
// semantically and lexically near the direct citations at write time. It is
// written on the same INSERT as Citations and never backfilled afterwards;
// a future recomputation must yield a new turn instead of mutating this row.
// Fallback, when set, marks the turn as an answer the pipeline was not willing
// to stand behind. It rides in the citations JSONB (see the repository) rather
// than a dedicated column, because augur_messages already carries two JSONB
// columns and a marker is not worth a migration.
type AugurMessage struct {
	ID               uuid.UUID
	ConversationID   uuid.UUID
	Role             string // "user" or "assistant"
	Content          string
	Citations        []AugurCitation
	RelatedCitations []AugurCitation
	Fallback         *AugurFallback
	CreatedAt        time.Time
}

// AugurFallback records why an answer was degraded. Keeping these turns in
// history is what stops every quality figure from being computed over the
// subset of answers that happened to succeed.
type AugurFallback struct {
	// Code is the usecase-level FallbackCategory ("retrieval_empty",
	// "short_under_grounded", …); stable enough to group by.
	Code string `json:"code"`
	// Reason is the human-readable explanation attached to the turn.
	Reason string `json:"reason,omitempty"`
}

// CitationKind discriminates how AugurCitation.URL / RefID should be used
// when the UI builds a click target. Mirrors alt.augur.v2.CitationKind so
// the handler/domain layers don't need to import the generated proto types.
type CitationKind string

const (
	CitationKindUnspecified CitationKind = ""
	CitationKindWeb         CitationKind = "web"
	CitationKindArticle     CitationKind = "article"
	CitationKindSummary     CitationKind = "summary"
)

// AugurCitation is persisted inside augur_messages.citations (JSONB).
//
// Field semantics by Kind:
//
//	web        → URL is an absolute https URL; RefID empty.
//	article    → RefID is alt-db articles.id (UUID); URL empty.
//	summary    → RefID is alt-db summary_versions.summary_version_id (UUID); URL empty.
//	"" (legacy) → URL may contain anything the upstream emitter wrote (potentially a
//	              raw UUID); the UI must NOT link these. Kept for rolling-deploy
//	              backward compatibility and historical citations persisted before
//	              the kind field existed.
type AugurCitation struct {
	URL         string       `json:"url"`
	Title       string       `json:"title"`
	PublishedAt string       `json:"published_at,omitempty"`
	Kind        CitationKind `json:"kind,omitempty"`
	RefID       string       `json:"ref_id,omitempty"`
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
	// ID and CreatedAt. Citations is serialized to JSONB; a non-nil Fallback
	// switches that column to its envelope form.
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
