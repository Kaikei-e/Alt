package fetch_article_tags_usecase

import (
	"alt/domain"
	"alt/port/fetch_article_tags_port"
	"alt/utils/logger"
	"context"
	"errors"
	"strings"
)

// FetchArticleTagsUsecase handles fetching tags for a specific article.
type FetchArticleTagsUsecase struct {
	port fetch_article_tags_port.FetchArticleTagsPort
}

// NewFetchArticleTagsUsecase creates a new usecase instance.
func NewFetchArticleTagsUsecase(port fetch_article_tags_port.FetchArticleTagsPort) *FetchArticleTagsUsecase {
	return &FetchArticleTagsUsecase{
		port: port,
	}
}

// Execute fetches tags associated with a specific article.
func (u *FetchArticleTagsUsecase) Execute(ctx context.Context, articleID string) ([]*domain.FeedTag, error) {
	// Business rule validation
	if strings.TrimSpace(articleID) == "" {
		logger.Logger.ErrorContext(ctx, "invalid article_id: must not be empty")
		return nil, errors.New("article_id must not be empty")
	}

	logger.Logger.InfoContext(ctx, "fetching tags for article", "articleID", articleID)

	tags, err := u.port.FetchArticleTags(ctx, articleID)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch article tags", "error", err, "articleID", articleID)
		return nil, err
	}

	logger.Logger.InfoContext(ctx, "successfully fetched article tags", "articleID", articleID, "count", len(tags))
	return tags, nil
}
