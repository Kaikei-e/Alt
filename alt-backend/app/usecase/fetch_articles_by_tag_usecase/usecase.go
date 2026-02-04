package fetch_articles_by_tag_usecase

import (
	"alt/domain"
	"alt/port/fetch_articles_by_tag_port"
	"alt/utils/logger"
	"context"
	"errors"
	"strings"
	"time"
)

// FetchArticlesByTagUsecase handles fetching articles by tag for the Tag Trail feature.
type FetchArticlesByTagUsecase struct {
	port fetch_articles_by_tag_port.FetchArticlesByTagPort
}

// NewFetchArticlesByTagUsecase creates a new usecase instance.
func NewFetchArticlesByTagUsecase(port fetch_articles_by_tag_port.FetchArticlesByTagPort) *FetchArticlesByTagUsecase {
	return &FetchArticlesByTagUsecase{
		port: port,
	}
}

// Execute fetches articles associated with a specific tag.
func (u *FetchArticlesByTagUsecase) Execute(ctx context.Context, tagID string, cursor *time.Time, limit int) ([]*domain.TagTrailArticle, error) {
	// Business rule validation
	if strings.TrimSpace(tagID) == "" {
		logger.Logger.ErrorContext(ctx, "invalid tag_id: must not be empty")
		return nil, errors.New("tag_id must not be empty")
	}

	if limit <= 0 {
		logger.Logger.ErrorContext(ctx, "invalid limit: must be greater than 0", "limit", limit)
		return nil, errors.New("limit must be greater than 0")
	}

	if limit > 100 {
		logger.Logger.ErrorContext(ctx, "invalid limit: cannot exceed 100", "limit", limit)
		return nil, errors.New("limit cannot exceed 100")
	}

	logger.Logger.InfoContext(ctx, "fetching articles by tag", "tagID", tagID, "cursor", cursor, "limit", limit)

	articles, err := u.port.FetchArticlesByTag(ctx, tagID, cursor, limit)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch articles by tag", "error", err, "tagID", tagID)
		return nil, err
	}

	logger.Logger.InfoContext(ctx, "successfully fetched articles by tag", "tagID", tagID, "count", len(articles))
	return articles, nil
}

// ExecuteByTagName fetches articles associated with a specific tag name across all feeds.
func (u *FetchArticlesByTagUsecase) ExecuteByTagName(ctx context.Context, tagName string, cursor *time.Time, limit int) ([]*domain.TagTrailArticle, error) {
	// Business rule validation
	if strings.TrimSpace(tagName) == "" {
		logger.Logger.ErrorContext(ctx, "invalid tag_name: must not be empty")
		return nil, errors.New("tag_name must not be empty")
	}

	if limit <= 0 {
		logger.Logger.ErrorContext(ctx, "invalid limit: must be greater than 0", "limit", limit)
		return nil, errors.New("limit must be greater than 0")
	}

	if limit > 100 {
		logger.Logger.ErrorContext(ctx, "invalid limit: cannot exceed 100", "limit", limit)
		return nil, errors.New("limit cannot exceed 100")
	}

	logger.Logger.InfoContext(ctx, "fetching articles by tag name", "tagName", tagName, "cursor", cursor, "limit", limit)

	articles, err := u.port.FetchArticlesByTagName(ctx, tagName, cursor, limit)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch articles by tag name", "error", err, "tagName", tagName)
		return nil, err
	}

	logger.Logger.InfoContext(ctx, "successfully fetched articles by tag name", "tagName", tagName, "count", len(articles))
	return articles, nil
}
