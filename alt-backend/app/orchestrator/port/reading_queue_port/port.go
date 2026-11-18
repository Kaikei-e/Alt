package reading_queue_port

import (
	"context"
	"time"

	"github.com/google/uuid"

	"alt/domain"
)

type GetReadingQueuePort interface {
	GetReadingQueue(ctx context.Context, userID uuid.UUID, asOf time.Time) ([]domain.ReadingQueueEntry, error)
}

type CountReadingSourcesSincePort interface {
	CountReadingSourcesSince(ctx context.Context, userID uuid.UUID, since time.Time) (int, error)
}

type FetchArticlesByIDsPort interface {
	FetchArticlesByIDs(ctx context.Context, articleIDs []uuid.UUID) ([]*domain.Article, error)
}

type GetOgImageURLsPort interface {
	GetOgImageURLsByArticleIDs(ctx context.Context, articleIDs []string) (map[string]string, error)
}
