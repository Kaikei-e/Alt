package fetch_feed_stats_usecase

import (
	"alt/port/feed_stats_port"
	"alt/utils/logger"
	"context"
	"errors"
)

type SummarizedArticlesCountUsecase struct {
	summarizedArticlesCountPort feed_stats_port.SummarizedArticlesCountPort
}

func NewSummarizedArticlesCountUsecase(summarizedArticlesCountPort feed_stats_port.SummarizedArticlesCountPort) *SummarizedArticlesCountUsecase {
	return &SummarizedArticlesCountUsecase{summarizedArticlesCountPort: summarizedArticlesCountPort}
}

func (u *SummarizedArticlesCountUsecase) Execute(ctx context.Context) (int, error) {
	count, err := u.summarizedArticlesCountPort.Execute(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch summarized articles count", "error", err)
		return 0, errors.New("failed to fetch summarized articles count")
	}

	logger.Logger.InfoContext(ctx, "summarized articles count fetched successfully", "count", count)
	return count, nil
}
