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
// Used by the projector path which intentionally consumes all events.
type ListKnowledgeEventsPort interface {
	ListKnowledgeEventsSince(ctx context.Context, afterSeq int64, limit int) ([]domain.KnowledgeEvent, error)
}

// ListKnowledgeEventsForUserPort reads events scoped to a (tenant, user) pair.
// Both identifiers are required so cross-tenant events cannot leak through
// the stream subscriber path.
type ListKnowledgeEventsForUserPort interface {
	ListKnowledgeEventsSinceForUser(ctx context.Context, tenantID, userID uuid.UUID, afterSeq int64, limit int) ([]domain.KnowledgeEvent, error)
}

// LatestKnowledgeEventSeqForUserPort returns the latest sequence visible to
// the given (tenant, user) pair.
type LatestKnowledgeEventSeqForUserPort interface {
	GetLatestKnowledgeEventSeqForUser(ctx context.Context, tenantID, userID uuid.UUID) (int64, error)
}

// IsArticleVisibleInLensPort checks, for a batch of article IDs, which ones
// would appear in the user's lens-filtered Knowledge Home view. Used at
// stream delivery time to drop events whose underlying article is not in
// the subscriber's active lens scope.
type IsArticleVisibleInLensPort interface {
	AreArticlesVisibleInLens(ctx context.Context, tenantID, userID uuid.UUID, articleIDs []uuid.UUID, filter *domain.KnowledgeHomeLensFilter) (map[uuid.UUID]bool, error)
}
