package knowledge_event_port

import (
	"alt/domain"
	"context"
)

// AppendKnowledgeEventPort appends events to the knowledge event store.
type AppendKnowledgeEventPort interface {
	AppendKnowledgeEvent(ctx context.Context, event domain.KnowledgeEvent) error
}

// ListKnowledgeEventsPort reads events from the knowledge event store.
type ListKnowledgeEventsPort interface {
	ListKnowledgeEventsSince(ctx context.Context, afterSeq int64, limit int) ([]domain.KnowledgeEvent, error)
}
