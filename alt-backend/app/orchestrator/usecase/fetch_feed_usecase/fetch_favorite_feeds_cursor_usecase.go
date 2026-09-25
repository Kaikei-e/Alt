package fetch_feed_usecase

import (
	"alt/domain"
	"alt/orchestrator/port/fetch_feed_port"
	"alt/utils/logger"
	"context"
	"time"
)

type FetchFavoriteFeedsListCursorUsecase struct {
	fetchFeedsListGateway fetch_feed_port.FavoriteFeedCursorPort
}

func NewFetchFavoriteFeedsListCursorUsecase(fetchFeedsListGateway fetch_feed_port.FavoriteFeedCursorPort) *FetchFavoriteFeedsListCursorUsecase {
	return &FetchFavoriteFeedsListCursorUsecase{fetchFeedsListGateway: fetchFeedsListGateway}
}

func (u *FetchFavoriteFeedsListCursorUsecase) Execute(ctx context.Context, cursor *time.Time, limit int) ([]*domain.FeedItem, error) {
	if err := validateCursorLimit(limit); err != nil {
		logger.Logger.ErrorContext(ctx, "invalid limit: "+err.Error(), "limit", limit)
		return nil, err
	}

	logger.Logger.InfoContext(ctx, "fetching favorite feeds with cursor", "cursor", cursor, "limit", limit)

	feeds, err := u.fetchFeedsListGateway.FetchFavoriteFeedsListCursor(ctx, cursor, limit)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch favorite feeds with cursor", "error", err, "cursor", cursor, "limit", limit)
		return nil, err
	}

	logger.Logger.InfoContext(ctx, "successfully fetched favorite feeds with cursor", "count", len(feeds))
	return feeds, nil
}
