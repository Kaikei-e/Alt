package fetch_trend_stats_usecase

import (
	"context"
	"errors"

	"alt/port/trend_stats_port"
	"alt/utils/logger"
)

// FetchTrendStatsUsecase handles the business logic for fetching trend statistics
type FetchTrendStatsUsecase struct {
	trendStatsPort trend_stats_port.TrendStatsPort
}

// NewFetchTrendStatsUsecase creates a new FetchTrendStatsUsecase
func NewFetchTrendStatsUsecase(port trend_stats_port.TrendStatsPort) *FetchTrendStatsUsecase {
	return &FetchTrendStatsUsecase{trendStatsPort: port}
}

// Execute fetches trend statistics for the given time window
func (u *FetchTrendStatsUsecase) Execute(ctx context.Context, window string) (*trend_stats_port.TrendDataResponse, error) {
	result, err := u.trendStatsPort.Execute(ctx, window)
	if err != nil {
		logger.Logger.Error("failed to fetch trend stats",
			"error", err,
			"window", window)
		return nil, errors.New("failed to fetch trend stats")
	}

	logger.Logger.Info("trend stats fetched successfully",
		"window", window,
		"data_points", len(result.DataPoints),
		"granularity", result.Granularity)

	return result, nil
}
