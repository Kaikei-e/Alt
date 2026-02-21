package fetch_feed_usecase

import (
	"alt/domain"
	"alt/port/fetch_feed_port"
	"alt/utils/logger"
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

type FetchUnreadFeedsListCursorUsecase struct {
	fetchFeedsListGateway fetch_feed_port.FetchFeedsPort
}

func NewFetchUnreadFeedsListCursorUsecase(fetchFeedsListGateway fetch_feed_port.FetchFeedsPort) *FetchUnreadFeedsListCursorUsecase {
	return &FetchUnreadFeedsListCursorUsecase{fetchFeedsListGateway: fetchFeedsListGateway}
}

func (u *FetchUnreadFeedsListCursorUsecase) Execute(ctx context.Context, cursor *time.Time, limit int, excludeFeedLinkID *uuid.UUID) ([]*domain.FeedItem, bool, error) {
	// ビジネスルール検証
	if limit <= 0 {
		logger.Logger.ErrorContext(ctx, "invalid limit: must be greater than 0", "limit", limit)
		return nil, false, errors.New("limit must be greater than 0")
	}
	if limit > 100 {
		logger.Logger.ErrorContext(ctx, "invalid limit: cannot exceed 100", "limit", limit)
		return nil, false, errors.New("limit cannot exceed 100")
	}

	logger.Logger.InfoContext(ctx, "fetching unread feeds with cursor", "cursor", cursor, "limit", limit)

	// Fetch limit+1 to detect whether more data exists.
	fetchLimit := limit + 1
	feeds, err := u.fetchFeedsListGateway.FetchUnreadFeedsListCursor(ctx, cursor, fetchLimit, excludeFeedLinkID)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch unread feeds with cursor", "error", err, "cursor", cursor, "limit", limit)
		return nil, false, err
	}

	hasMore := len(feeds) > limit
	if hasMore {
		feeds = feeds[:limit]
	}

	logger.Logger.InfoContext(ctx,
		"successfully fetched unread feeds with cursor",
		"count", len(feeds),
		"has_more", hasMore,
	)
	return feeds, hasMore, nil
}
