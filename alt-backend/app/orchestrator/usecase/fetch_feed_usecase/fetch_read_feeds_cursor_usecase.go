package fetch_feed_usecase

import (
	"alt/domain"
	"alt/orchestrator/port/fetch_feed_port"
	"alt/utils/logger"
	"context"
	"time"
)

type FetchReadFeedsListCursorUsecase struct {
	fetchFeedsListGateway fetch_feed_port.ReadFeedCursorPort
}

func NewFetchReadFeedsListCursorUsecase(fetchFeedsListGateway fetch_feed_port.ReadFeedCursorPort) *FetchReadFeedsListCursorUsecase {
	return &FetchReadFeedsListCursorUsecase{fetchFeedsListGateway: fetchFeedsListGateway}
}

func (u *FetchReadFeedsListCursorUsecase) Execute(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error) {
	// ビジネスルール検証
	if err := validateCursorLimit(limit); err != nil {
		logger.Logger.ErrorContext(ctx, "invalid limit: "+err.Error(), "limit", limit)
		return nil, err
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
