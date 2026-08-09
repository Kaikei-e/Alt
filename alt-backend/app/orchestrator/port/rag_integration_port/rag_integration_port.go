package rag_integration_port

import (
	"context"
	"errors"
	"time"
)

// ErrRagUpsertTransient marks a RagIntegrationPort.UpsertArticle failure as
// transient — a transport error reaching rag-orchestrator, or a 5xx it
// returned — as opposed to a permanent rejection (a malformed payload, a
// 4xx). Implementations wrap it into the returned error with fmt.Errorf's
// %w so a caller can test for it with errors.Is instead of parsing the
// message string.
//
// The distinction exists for the outbox worker: only a transient failure is
// worth retrying, because only a transient failure has a chance of
// succeeding on the next attempt with the same payload.
var ErrRagUpsertTransient = errors.New("rag integration: transient upsert failure")

type RagContext struct {
	ChunkText       string
	URL             string
	Title           string
	PublishedAt     *time.Time
	Score           float32
	DocumentVersion int64
}

// UpsertArticleInput carries an article from the outbox row that recorded it
// to rag-orchestrator's index. The JSON tags are load-bearing, not
// decoration: the outbox worker unmarshals the enqueued payload
// (save_article_driver.go writes snake_case keys) straight into this struct,
// and encoding/json's case-insensitive fallback does not cross underscores.
// Without a tag, article_id lands in no field at all and arrives blank —
// silently, since an unmatched key is not an error — which rag-orchestrator
// rejects with a 400 the outbox then treats as terminal.
type UpsertArticleInput struct {
	ArticleID   string     `json:"article_id"`
	Body        string     `json:"body"`
	PublishedAt *time.Time `json:"published_at"`
	Title       string     `json:"title"`
	UpdatedAt   *time.Time `json:"updated_at"`
	URL         string     `json:"url"`
	UserID      string     `json:"user_id"`
}

type RagIntegrationPort interface {
	RetrieveContext(ctx context.Context, query string, candidateIDs []string) ([]RagContext, error)
	UpsertArticle(ctx context.Context, input UpsertArticleInput) error
	Answer(ctx context.Context, input AnswerInput) (<-chan string, error)
}

type AnswerInput struct {
	Query     string
	Contexts  []string // Optional: specific context IDs if needed, though usually RAG handles retrieval
	Stream    bool
	SessionID string
}
