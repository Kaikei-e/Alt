package fetch_feed_usecase

import (
	"alt/domain"
	"alt/port/fetch_feed_port"
	"alt/utils/logger"
	"context"
	"errors"
	"time"
)

type FetchReadFeedsListCursorUsecase struct {
	fetchFeedsListGateway fetch_feed_port.FetchFeedsPort
}

func NewFetchReadFeedsListCursorUsecase(fetchFeedsListGateway fetch_feed_port.FetchFeedsPort) *FetchReadFeedsListCursorUsecase {
	return &FetchReadFeedsListCursorUsecase{fetchFeedsListGateway: fetchFeedsListGateway}
}

func (u *FetchReadFeedsListCursorUsecase) Execute(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error) {
	// ビジネスルール検証
	if limit <= 0 {
		logger.Logger.ErrorContext(ctx, "invalid limit: must be greater than 0", "limit", limit)
		return nil, errors.New("limit must be greater than 0")
	}
	if limit > 100 {
		logger.Logger.ErrorContext(ctx, "invalid limit: cannot exceed 100", "limit", limit)
		return nil, errors.New("limit cannot exceed 100")
	}

	logger.Logger.InfoContext(ctx, "fetching read feeds with cursor", "cursor", cursor, "limit", limit)

	feeds, err := u.fetchFeedsListGateway.FetchReadFeedsListCursor(ctx, cursor, limit)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch read feeds with cursor", "error", err, "cursor", cursor, "limit", limit)
		return nil, err
	}

	logger.Logger.InfoContext(ctx, "successfully fetched read feeds with cursor", "count", len(feeds))
	return feeds, nil
}
