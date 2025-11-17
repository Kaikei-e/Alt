package fetch_feed_usecase

import (
	"alt/domain"
	"alt/port/fetch_feed_port"
	"alt/utils/logger"
	"context"
	"errors"
	"time"
)

type FetchUnreadFeedsListCursorUsecase struct {
	fetchFeedsListGateway fetch_feed_port.FetchFeedsPort
}

func NewFetchUnreadFeedsListCursorUsecase(fetchFeedsListGateway fetch_feed_port.FetchFeedsPort) *FetchUnreadFeedsListCursorUsecase {
	return &FetchUnreadFeedsListCursorUsecase{fetchFeedsListGateway: fetchFeedsListGateway}
}

func (u *FetchUnreadFeedsListCursorUsecase) Execute(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error) {
	// ビジネスルール検証
	if limit <= 0 {
		logger.Logger.Error("invalid limit: must be greater than 0", "limit", limit)
		return nil, errors.New("limit must be greater than 0")
	}
	if limit > 100 {
		logger.Logger.Error("invalid limit: cannot exceed 100", "limit", limit)
		return nil, errors.New("limit cannot exceed 100")
	}

	logger.Logger.Info("fetching unread feeds with cursor", "cursor", cursor, "limit", limit)

	feeds, err := u.fetchFeedsListGateway.FetchUnreadFeedsListCursor(ctx, cursor, limit)
	if err != nil {
		logger.Logger.Error("failed to fetch unread feeds with cursor", "error", err, "cursor", cursor, "limit", limit)
		return nil, err
	}

	logger.Logger.Info("successfully fetched unread feeds with cursor", "count", len(feeds))
	return feeds, nil
}
