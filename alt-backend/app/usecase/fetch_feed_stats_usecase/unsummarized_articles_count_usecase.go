package fetch_feed_stats_usecase

import (
	"alt/port/feed_stats_port"
	"alt/utils/logger"
	"context"
	"errors"
)

type UnsummarizedArticlesCountUsecase struct {
	unsummarizedArticlesCountPort feed_stats_port.UnsummarizedArticlesCountPort
}

func NewUnsummarizedArticlesCountUsecase(unsummarizedArticlesCountPort feed_stats_port.UnsummarizedArticlesCountPort) *UnsummarizedArticlesCountUsecase {
	return &UnsummarizedArticlesCountUsecase{unsummarizedArticlesCountPort: unsummarizedArticlesCountPort}
}

func (u *UnsummarizedArticlesCountUsecase) Execute(ctx context.Context) (int, error) {
	count, err := u.unsummarizedArticlesCountPort.Execute(ctx)
	if err != nil {
		logger.Logger.Error("failed to fetch unsummarized articles count", "error", err)
		return 0, errors.New("failed to fetch unsummarized articles count")
	}

	logger.Logger.Info("unsummarized articles count fetched successfully", "count", count)
	return count, nil
}