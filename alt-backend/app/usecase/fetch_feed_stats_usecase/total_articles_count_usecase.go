package fetch_feed_stats_usecase

import (
	"alt/port/feed_stats_port"
	"alt/utils/logger"
	"context"
	"fmt"
)

type TotalArticlesCountUsecase struct {
	totalArticlesCountPort feed_stats_port.TotalArticlesCountPort
}

func NewTotalArticlesCountUsecase(totalArticlesCountPort feed_stats_port.TotalArticlesCountPort) *TotalArticlesCountUsecase {
	return &TotalArticlesCountUsecase{totalArticlesCountPort: totalArticlesCountPort}
}

func (u *TotalArticlesCountUsecase) Execute(ctx context.Context) (int, error) {
	totalArticlesCount, err := u.totalArticlesCountPort.Execute(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch total articles count from port",
			"error", err,
			"usecase", "TotalArticlesCountUsecase")
		return 0, fmt.Errorf("failed to fetch total articles count: %w", err)
	}

	logger.Logger.InfoContext(ctx, "total articles count fetched successfully",
		"count", totalArticlesCount,
		"usecase", "TotalArticlesCountUsecase")
	return totalArticlesCount, nil
}
