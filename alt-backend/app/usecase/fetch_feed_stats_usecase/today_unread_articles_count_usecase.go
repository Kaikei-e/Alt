package fetch_feed_stats_usecase

import (
	"alt/port/feed_stats_port"
	"alt/utils/logger"
	"context"
	"errors"
	"time"
)

type TodayUnreadArticlesCountUsecase struct {
	todayUnreadArticlesCountPort feed_stats_port.TodayUnreadArticlesCountPort
}

func NewTodayUnreadArticlesCountUsecase(port feed_stats_port.TodayUnreadArticlesCountPort) *TodayUnreadArticlesCountUsecase {
	return &TodayUnreadArticlesCountUsecase{todayUnreadArticlesCountPort: port}
}

func (u *TodayUnreadArticlesCountUsecase) Execute(ctx context.Context, since time.Time) (int, error) {
	count, err := u.todayUnreadArticlesCountPort.Execute(ctx, since)
	if err != nil {
		logger.Logger.Error("failed to fetch today's unread articles count", "error", err)
		return 0, errors.New("failed to fetch today's unread articles count")
	}

	logger.Logger.Info("today's unread articles count fetched successfully", "count", count)
	return count, nil
}
