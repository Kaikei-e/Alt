package fetch_feed_usecase

import (
	"alt/domain"
	"alt/orchestrator/port/fetch_feed_port"
	"alt/utils/logger"
	"context"
	"time"

	"github.com/google/uuid"
)

type FetchFeedsListCursorUsecase struct {
	fetchFeedsListGateway fetch_feed_port.FeedCursorPort
}

func NewFetchFeedsListCursorUsecase(fetchFeedsListGateway fetch_feed_port.FeedCursorPort) *FetchFeedsListCursorUsecase {
	return &FetchFeedsListCursorUsecase{fetchFeedsListGateway: fetchFeedsListGateway}
}
func (u *FetchFeedsListCursorUsecase) Execute(ctx context.Context, cursor *time.Time, limit int, excludeFeedLinkIDs []uuid.UUID) ([]*domain.FeedItem, error) {
	// Validate limit
	if err := validateCursorLimit(limit); err != nil {
		logger.Logger.ErrorContext(ctx, "invalid limit: "+err.Error(), "limit", limit)
		return nil, err
	}

	logger.Logger.InfoContext(ctx, "fetching feeds with cursor", "cursor", cursor, "limit", limit)

	feeds, err := u.fetchFeedsListGateway.FetchFeedsListCursor(ctx, cursor, limit, excludeFeedLinkIDs)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch feeds with cursor", "error", err, "cursor", cursor, "limit", limit)
		return nil, err
	}

	logger.Logger.InfoContext(ctx, "successfully fetched feeds with cursor", "count", len(feeds))
	return feeds, nil
}
