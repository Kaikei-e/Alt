package feed_page_cache_port

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type FeedPageEntry struct {
	FeedID      uuid.UUID
	Title       string
	Description string
	Link        string
	PubDate     time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ArticleID   *string
	OgImageURL  *string
	// Pre-computed fields (populated at cache load time to avoid per-request allocations)
	SanitizedDescription string
	FeedIDStr            string
	PublishedStr         string
}

type FeedPageCachePort interface {
	GetFeedPage(ctx context.Context, feedLinkID uuid.UUID) ([]*FeedPageEntry, error)
	InvalidateFeedPage(ctx context.Context, feedLinkID uuid.UUID) error
}
