package search_feed_usecase

import (
	"alt/domain"
	"alt/port/feed_search_port"
	"context"
	"log/slog"
)

type SearchFeedByTitleUsecase struct {
	searchByTitlePort feed_search_port.SearchByTitlePort
	logger            *slog.Logger
}

func NewSearchFeedByTitleUsecase(searchByTitlePort feed_search_port.SearchByTitlePort) *SearchFeedByTitleUsecase {
	return &SearchFeedByTitleUsecase{
		searchByTitlePort: searchByTitlePort,
		logger:            slog.Default(),
	}
}

func (u *SearchFeedByTitleUsecase) Execute(ctx context.Context, query string) ([]*domain.FeedItem, error) {
	// contextからuser取得
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		u.logger.Error("user context not found", "error", err)
		return nil, err
	}

	u.logger.Info("executing feed search by title",
		"query", query,
		"user_id", user.UserID)

	feeds, err := u.searchByTitlePort.SearchFeedsByTitle(ctx, query, user.UserID.String())
	if err != nil {
		u.logger.Error("failed to search feeds by title",
			"error", err,
			"query", query,
			"user_id", user.UserID)
		return nil, err
	}

	u.logger.Info("feed search by title completed",
		"query", query,
		"user_id", user.UserID,
		"results_count", len(feeds))

	return feeds, nil
}
