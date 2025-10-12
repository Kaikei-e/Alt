package search_article_usecase

import (
	"alt/domain"
	"alt/port/search_indexer_port"
	"context"
	"fmt"
	"log/slog"
)

type SearchArticleUsecase struct {
	searchIndexerPort search_indexer_port.SearchIndexerPort
	logger            *slog.Logger
}

func NewSearchArticleUsecase(searchIndexerPort search_indexer_port.SearchIndexerPort) *SearchArticleUsecase {
	return &SearchArticleUsecase{
		searchIndexerPort: searchIndexerPort,
		logger:            slog.Default(),
	}
}

func (u *SearchArticleUsecase) Execute(ctx context.Context, query string) ([]domain.SearchIndexerArticleHit, error) {
	// Get user from context
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		u.logger.Error("user context not found", "error", err)
		return nil, fmt.Errorf("authentication required: %w", err)
	}

	u.logger.Info("executing article search",
		"query", query,
		"user_id", user.UserID)

	// Search articles via search-indexer
	hits, err := u.searchIndexerPort.SearchArticles(ctx, query, user.UserID.String())
	if err != nil {
		u.logger.Error("failed to search articles",
			"error", err,
			"query", query,
			"user_id", user.UserID)
		return nil, fmt.Errorf("article search failed: %w", err)
	}

	u.logger.Info("article search completed",
		"query", query,
		"user_id", user.UserID,
		"results_count", len(hits))

	return hits, nil
}
