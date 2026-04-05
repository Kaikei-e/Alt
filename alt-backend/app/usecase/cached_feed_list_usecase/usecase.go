package cached_feed_list_usecase

import (
	"alt/domain"
	"alt/port/fetch_feed_port"
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

type CachedFeedListUsecase struct {
	legacyFeedPort fetch_feed_port.ReadAndFavoriteFeedCursorPort
	unreadFeedPort fetch_feed_port.UnreadFeedCursorPort
	allFeedPort    fetch_feed_port.FeedCursorPort
}

func NewCachedFeedListUsecase(
	legacyFeedPort fetch_feed_port.ReadAndFavoriteFeedCursorPort,
	unreadFeedPort fetch_feed_port.UnreadFeedCursorPort,
	allFeedPort fetch_feed_port.FeedCursorPort,
) *CachedFeedListUsecase {
	return &CachedFeedListUsecase{
		legacyFeedPort: legacyFeedPort,
		unreadFeedPort: unreadFeedPort,
		allFeedPort:    allFeedPort,
	}
}

func (u *CachedFeedListUsecase) FetchUnreadFeedsListCursor(
	ctx context.Context,
	cursor *time.Time,
	limit int,
	excludeFeedLinkID *uuid.UUID,
) ([]*domain.FeedItem, bool, error) {
	if err := validateLimit(limit); err != nil {
		return nil, false, err
	}

	// Delegate to efficient single-SQL-query port.
	// Fetches limit+1 to detect hasMore without a separate COUNT query.
	fetchLimit := limit + 1
	feeds, err := u.unreadFeedPort.FetchUnreadFeedsListCursor(ctx, cursor, fetchLimit, excludeFeedLinkID)
	if err != nil {
		return nil, false, err
	}

	hasMore := len(feeds) > limit
	if hasMore {
		feeds = feeds[:limit]
	}
	return feeds, hasMore, nil
}

func (u *CachedFeedListUsecase) FetchAllFeedsListCursor(
	ctx context.Context,
	cursor *time.Time,
	limit int,
	excludeFeedLinkID *uuid.UUID,
) ([]*domain.FeedItem, error) {
	if err := validateLimit(limit); err != nil {
		return nil, err
	}

	// Delegate to efficient single-SQL-query port.
	// The SQL query includes LEFT JOIN with read_status to set IsRead.
	return u.allFeedPort.FetchFeedsListCursor(ctx, cursor, limit, excludeFeedLinkID)
}

func (u *CachedFeedListUsecase) FetchReadFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error) {
	return u.legacyFeedPort.FetchReadFeedsListCursor(ctx, cursor, limit)
}

func (u *CachedFeedListUsecase) FetchFavoriteFeedsListCursor(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error) {
	return u.legacyFeedPort.FetchFavoriteFeedsListCursor(ctx, cursor, limit)
}

func validateLimit(limit int) error {
	if limit <= 0 {
		return errors.New("limit must be greater than 0")
	}
	if limit > 100 {
		return errors.New("limit cannot exceed 100")
	}
	return nil
}
