package fetch_feed_port

//go:generate mockgen -source=fetch_port.go -destination=../../mocks/mock_fetch_feed_port.go -package=mocks

import (
	"alt/domain"
	"context"
	"time"

	"github.com/google/uuid"
)

type FetchSingleFeedPort interface {
	FetchSingleFeed(ctx context.Context) (*domain.RSSFeed, error)
}

// Small, consumer-specific interfaces (ISP)

type FeedsByLinkPort interface {
	FetchFeeds(ctx context.Context, link string) ([]*domain.FeedItem, error)
}

type FeedListPort interface {
	FetchFeedsList(ctx context.Context) ([]*domain.FeedItem, error)
}

type FeedListLimitPort interface {
	FetchFeedsListLimit(ctx context.Context, offset int) ([]*domain.FeedItem, error)
}

type FeedListPagePort interface {
	FetchFeedsListPage(ctx context.Context, page int) ([]*domain.FeedItem, error)
}

type FeedCursorPort interface {
	FetchFeedsListCursor(ctx context.Context, cursor *time.Time, limit int, excludeFeedLinkID *uuid.UUID) ([]*domain.FeedItem, error)
}

type UnreadFeedCursorPort interface {
	FetchUnreadFeedsListCursor(ctx context.Context, cursor *time.Time, limit int, excludeFeedLinkID *uuid.UUID) ([]*domain.FeedItem, error)
}

type ReadFeedCursorPort interface {
	FetchReadFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error)
}

type FavoriteFeedCursorPort interface {
	FetchFavoriteFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error)
}

// FeedListAllPort is for consumers that need list, limit, and page operations.
type FeedListAllPort interface {
	FeedListPort
	FeedListLimitPort
	FeedListPagePort
}

// ReadAndFavoriteFeedCursorPort is for consumers that delegate read and favorite cursor queries.
type ReadAndFavoriteFeedCursorPort interface {
	ReadFeedCursorPort
	FavoriteFeedCursorPort
}

// Composite interface for gateway implementations (backward compat)
type FetchFeedsPort interface {
	FeedsByLinkPort
	FeedListPort
	FeedListLimitPort
	FeedListPagePort
	FeedCursorPort
	UnreadFeedCursorPort
	ReadFeedCursorPort
	FavoriteFeedCursorPort
}
