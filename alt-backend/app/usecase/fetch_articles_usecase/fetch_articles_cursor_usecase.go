package fetch_articles_usecase

import (
	"alt/domain"
	"alt/port/article_content_cache_port"
	"alt/port/article_cursor_port"
	"alt/port/fetch_articles_port"
	"alt/utils/logger"
	"context"
	"errors"
	"time"
)

type FetchArticlesCursorUsecase struct {
	fetchArticlesGateway fetch_articles_port.FetchArticlesPort
	articleCursorPort    article_cursor_port.FetchArticleCursorPort
	articleCache         article_content_cache_port.ArticleContentCachePort
}

func NewFetchArticlesCursorUsecase(fetchArticlesGateway fetch_articles_port.FetchArticlesPort) *FetchArticlesCursorUsecase {
	return &FetchArticlesCursorUsecase{fetchArticlesGateway: fetchArticlesGateway}
}

func NewFetchArticlesCursorUsecaseWithCache(
	articleCursorPort article_cursor_port.FetchArticleCursorPort,
	articleCache article_content_cache_port.ArticleContentCachePort,
) *FetchArticlesCursorUsecase {
	return &FetchArticlesCursorUsecase{
		articleCursorPort: articleCursorPort,
		articleCache:      articleCache,
	}
}

func (u *FetchArticlesCursorUsecase) Execute(ctx context.Context, cursor *time.Time, limit int) ([]*domain.Article, error) {
	if limit <= 0 {
		logger.Logger.ErrorContext(ctx, "invalid limit: must be greater than 0", "limit", limit)
		return nil, errors.New("limit must be greater than 0")
	}
	if limit > 100 {
		logger.Logger.ErrorContext(ctx, "invalid limit: cannot exceed 100", "limit", limit)
		return nil, errors.New("limit cannot exceed 100")
	}

	logger.Logger.InfoContext(ctx, "fetching articles with cursor", "cursor", cursor, "limit", limit)

	if u.articleCursorPort != nil && u.articleCache != nil {
		ids, err := u.articleCursorPort.FetchArticleIDsWithCursor(ctx, cursor, limit)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "failed to fetch article ids with cursor", "error", err, "cursor", cursor, "limit", limit)
			return nil, err
		}

		articles, err := u.articleCache.GetArticles(ctx, ids)
		if err != nil {
			logger.Logger.ErrorContext(ctx, "failed to fetch cached articles", "error", err, "cursor", cursor, "limit", limit)
			return nil, err
		}

		logger.Logger.InfoContext(ctx, "successfully fetched cached articles with cursor", "count", len(articles))
		return articles, nil
	}

	articles, err := u.fetchArticlesGateway.FetchArticlesWithCursor(ctx, cursor, limit)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch articles with cursor", "error", err, "cursor", cursor, "limit", limit)
		return nil, err
	}

	logger.Logger.InfoContext(ctx, "successfully fetched articles with cursor", "count", len(articles))
	return articles, nil
}
