package fetch_feed_stats_usecase

import (
	"alt/port/feed_stats_port"
	"alt/utils/logger"
	"context"
	"errors"
)

type FeedsCountUsecase struct {
	feedsCountPort feed_stats_port.FeedAmountPort
}

func NewFeedsCountUsecase(feedsCountPort feed_stats_port.FeedAmountPort) *FeedsCountUsecase {
	return &FeedsCountUsecase{feedsCountPort: feedsCountPort}
}

func (u *FeedsCountUsecase) Execute(ctx context.Context) (int, error) {
	amount, err := u.feedsCountPort.Execute(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch feeds count", "error", err)
		return 0, errors.New("failed to fetch feeds count")
	}

	logger.Logger.InfoContext(ctx, "feeds count fetched successfully", "amount", amount)
	return amount, nil
}
