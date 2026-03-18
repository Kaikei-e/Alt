package knowledge_event_port

import (
	"alt/domain"
	"context"

	"github.com/google/uuid"
)

// AppendKnowledgeEventPort appends events to the knowledge event store.
type AppendKnowledgeEventPort interface {
	AppendKnowledgeEvent(ctx context.Context, event domain.KnowledgeEvent) error
}

// ListKnowledgeEventsPort reads events from the knowledge event store.
type ListKnowledgeEventsPort interface {
	ListKnowledgeEventsSince(ctx context.Context, afterSeq int64, limit int) ([]domain.KnowledgeEvent, error)
}

// ListKnowledgeEventsForUserPort reads events scoped to a specific user.
type ListKnowledgeEventsForUserPort interface {
	ListKnowledgeEventsSinceForUser(ctx context.Context, userID uuid.UUID, afterSeq int64, limit int) ([]domain.KnowledgeEvent, error)
}

// LatestKnowledgeEventSeqForUserPort returns the latest sequence visible to a user.
type LatestKnowledgeEventSeqForUserPort interface {
	GetLatestKnowledgeEventSeqForUser(ctx context.Context, userID uuid.UUID) (int64, error)
}
