package feed_inflow_port

import (
	"context"
	"time"

	"alt/domain"

	"github.com/google/uuid"
)

// FeedInflowReadPort reads subscribed feed inflow counts and article->feed_link
// mappings from the feed catalog store.
type FeedInflowReadPort interface {
	GetFeedInflowByUser(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]domain.FeedInflow, error)
	BatchGetFeedLinkIDsByArticleIDs(ctx context.Context, userID uuid.UUID, articleIDs []uuid.UUID) (map[uuid.UUID]uuid.UUID, error)
}

// ReadingSourcesWindowPort reads first-read attribution rows from knowledge-sovereign.
type ReadingSourcesWindowPort interface {
	ListReadingSourcesInWindow(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]domain.ReadingSourceEntry, error)
}
