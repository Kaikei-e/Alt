package service

import (
	"context"
	"log/slog"
	"net/url"
	articlefetcher "pre-processor/article-fetcher"
	"pre-processor/models"
)

// ArticleFetcherService implementation
type articleFetcherService struct {
	logger *slog.Logger
}

// NewArticleFetcherService creates a new article fetcher service
func NewArticleFetcherService(logger *slog.Logger) ArticleFetcherService {
	return &articleFetcherService{
		logger: logger,
	}
}

// FetchArticle fetches an article from the given URL
func (s *articleFetcherService) FetchArticle(ctx context.Context, urlStr string) (*models.Article, error) {
	s.logger.Info("fetching article", "url", urlStr)

	// GREEN PHASE: Minimal implementation calling existing fetcher
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		s.logger.Error("failed to parse URL", "url", urlStr, "error", err)
		return nil, err
	}

	article, err := articlefetcher.FetchArticle(*parsedURL)
	if err != nil {
		s.logger.Error("failed to fetch article", "url", urlStr, "error", err)
		return nil, err
	}

	s.logger.Info("article fetched successfully", "url", urlStr)
	return article, nil
}

// ValidateURL validates a URL for security and format
func (s *articleFetcherService) ValidateURL(urlStr string) error {
	s.logger.Info("validating URL", "url", urlStr)

	// GREEN PHASE: Minimal implementation
	_, err := url.Parse(urlStr)
	if err != nil {
		s.logger.Error("URL validation failed", "url", urlStr, "error", err)
		return err
	}

	s.logger.Info("URL validated successfully", "url", urlStr)
	return nil
}
