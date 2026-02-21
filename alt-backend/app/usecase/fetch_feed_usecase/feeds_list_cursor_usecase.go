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

type FetchFeedsListCursorUsecase struct {
	fetchFeedsListGateway fetch_feed_port.FetchFeedsPort
}

func NewFetchFeedsListCursorUsecase(fetchFeedsListGateway fetch_feed_port.FetchFeedsPort) *FetchFeedsListCursorUsecase {
	return &FetchFeedsListCursorUsecase{fetchFeedsListGateway: fetchFeedsListGateway}
}

func (u *FetchFeedsListCursorUsecase) Execute(ctx context.Context, cursor *time.Time, limit int, excludeFeedLinkID *uuid.UUID) ([]*domain.FeedItem, error) {
	// Validate limit
	if limit <= 0 {
		logger.Logger.ErrorContext(ctx, "invalid limit: must be greater than 0", "limit", limit)
		return nil, errors.New("limit must be greater than 0")
	}
	if limit > 100 {
		logger.Logger.ErrorContext(ctx, "invalid limit: cannot exceed 100", "limit", limit)
		return nil, errors.New("limit cannot exceed 100")
	}

	logger.Logger.InfoContext(ctx, "fetching feeds with cursor", "cursor", cursor, "limit", limit)

	feeds, err := u.fetchFeedsListGateway.FetchFeedsListCursor(ctx, cursor, limit, excludeFeedLinkID)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch feeds with cursor", "error", err, "cursor", cursor, "limit", limit)
		return nil, err
	}

	logger.Logger.InfoContext(ctx, "successfully fetched feeds with cursor", "count", len(feeds))
	return feeds, nil
}
