package fetch_feed_usecase

import (
	"alt/domain"
	"alt/port/fetch_feed_port"
	"alt/utils/logger"
	"context"
	"errors"
	"time"
)

type FetchFeedsListCursorUsecase struct {
	fetchFeedsListGateway fetch_feed_port.FetchFeedsPort
}

func NewFetchFeedsListCursorUsecase(fetchFeedsListGateway fetch_feed_port.FetchFeedsPort) *FetchFeedsListCursorUsecase {
	return &FetchFeedsListCursorUsecase{fetchFeedsListGateway: fetchFeedsListGateway}
}

func (u *FetchFeedsListCursorUsecase) Execute(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error) {
	// Validate limit
	if limit <= 0 {
		logger.Logger.Error("invalid limit: must be greater than 0", "limit", limit)
		return nil, errors.New("limit must be greater than 0")
	}
	if limit > 100 {
		logger.Logger.Error("invalid limit: cannot exceed 100", "limit", limit)
		return nil, errors.New("limit cannot exceed 100")
	}

	logger.Logger.Info("fetching feeds with cursor", "cursor", cursor, "limit", limit)
	
	feeds, err := u.fetchFeedsListGateway.FetchFeedsListCursor(ctx, cursor, limit)
	if err != nil {
		logger.Logger.Error("failed to fetch feeds with cursor", "error", err, "cursor", cursor, "limit", limit)
		return nil, err
	}
	
	logger.Logger.Info("successfully fetched feeds with cursor", "count", len(feeds))
	return feeds, nil
}