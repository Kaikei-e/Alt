package fetch_articles_usecase

import (
	"alt/domain"
	"alt/port/fetch_articles_port"
	"alt/utils/logger"
	"context"
	"errors"
	"time"
)

type FetchArticlesCursorUsecase struct {
	fetchArticlesGateway fetch_articles_port.FetchArticlesPort
}

func NewFetchArticlesCursorUsecase(fetchArticlesGateway fetch_articles_port.FetchArticlesPort) *FetchArticlesCursorUsecase {
	return &FetchArticlesCursorUsecase{fetchArticlesGateway: fetchArticlesGateway}
}

func (u *FetchArticlesCursorUsecase) Execute(ctx context.Context, cursor *time.Time, limit int) ([]*domain.Article, error) {
	// Validate limit
	if limit <= 0 {
		logger.Logger.Error("invalid limit: must be greater than 0", "limit", limit)
		return nil, errors.New("limit must be greater than 0")
	}
	if limit > 100 {
		logger.Logger.Error("invalid limit: cannot exceed 100", "limit", limit)
		return nil, errors.New("limit cannot exceed 100")
	}

	logger.Logger.Info("fetching articles with cursor", "cursor", cursor, "limit", limit)

	articles, err := u.fetchArticlesGateway.FetchArticlesWithCursor(ctx, cursor, limit)
	if err != nil {
		logger.Logger.Error("failed to fetch articles with cursor", "error", err, "cursor", cursor, "limit", limit)
		return nil, err
	}

	logger.Logger.Info("successfully fetched articles with cursor", "count", len(articles))
	return articles, nil
}
