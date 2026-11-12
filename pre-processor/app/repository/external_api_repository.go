package repository

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"pre-processor/config"
	"pre-processor/domain"
	"pre-processor/driver"
)

// ExternalAPIRepository implementation.
type externalAPIRepository struct {
	logger *slog.Logger
	client *http.Client
	config *config.Config
}

// NewExternalAPIRepository creates a new external API repository.
func NewExternalAPIRepository(cfg *config.Config, logger *slog.Logger, newsClient *http.Client) ExternalAPIRepository {
	return &externalAPIRepository{
		logger: logger,
		config: cfg,
		client: newsClient,
	}
}

// SummarizeArticle summarizes an article using external API.
func (r *externalAPIRepository) SummarizeArticle(ctx context.Context, article *domain.Article, priority string) (*domain.SummarizedContent, error) {
	// Input validation
	if article == nil {
		r.logger.ErrorContext(ctx, "article cannot be nil")
		return nil, fmt.Errorf("article cannot be nil")
	}

	if article.ID == "" {
		r.logger.ErrorContext(ctx, "article ID cannot be empty")
		return nil, fmt.Errorf("article ID cannot be empty")
	}

	if article.Content == "" {
		r.logger.ErrorContext(ctx, "article content cannot be empty", "article_id", article.ID)
		return nil, fmt.Errorf("article content cannot be empty")
	}

	r.logger.InfoContext(ctx, "summarizing article", "article_id", article.ID)

	payload := driver.ArticlePayload{
		ID:      article.ID,
		Title:   article.Title,
		Content: article.Content,
	}

	// Use existing driver function
	driverSummary, err := driver.ArticleSummarizerAPIClient(ctx, payload, r.config, r.logger, priority, r.client)
	if err != nil {
		// Handle content too short as a normal case, not an error
		if errors.Is(err, driver.ErrContentTooShort) {
			r.logger.InfoContext(ctx, "skipping summarization: content too short", "article_id", article.ID)
			return nil, domain.ErrContentTooShort
		}
		if errors.Is(err, driver.ErrContentTooLong) {
			r.logger.ErrorContext(ctx, "failed to summarize article", "error", err, "article_id", article.ID)
			return nil, domain.ErrContentTooLong
		}
		// Handle 429 (service overloaded) - propagate for backpressure
		if errors.Is(err, driver.ErrServiceOverloaded) {
			r.logger.WarnContext(ctx, "downstream service overloaded", "article_id", article.ID)
			return nil, domain.ErrServiceOverloaded
		}
		// Handle 422 (content not processable) - retried through bounded path, dead-lettered once retries are exhausted
		if errors.Is(err, driver.ErrContentNotProcessable) {
			r.logger.WarnContext(ctx, "content not processable by model", "article_id", article.ID)
			return nil, domain.ErrContentNotProcessable
		}
		if errors.Is(err, driver.ErrUpstreamBusy) {
			r.logger.WarnContext(ctx, "failed to summarize article: upstream busy", "error", err, "article_id", article.ID)
			return nil, fmt.Errorf("%w: %w", domain.ErrUpstreamBusy, err)
		}
		if errors.Is(err, driver.ErrInvalidRequest) {
			r.logger.ErrorContext(ctx, "failed to summarize article", "error", err, "article_id", article.ID)
			return nil, fmt.Errorf("%w: %w", domain.ErrInvalidRequest, err)
		}
		r.logger.ErrorContext(ctx, "failed to summarize article", "error", err, "article_id", article.ID)
		return nil, fmt.Errorf("failed to summarize article: %w", err)
	}

	// Convert driver type to models type
	summarizedContent := &domain.SummarizedContent{
		ArticleID:       driverSummary.ArticleID,
		SummaryJapanese: driverSummary.SummaryJapanese,
	}

	r.logger.InfoContext(ctx, "article summarized successfully", "article_id", article.ID)

	return summarizedContent, nil
}

// StreamSummarizeArticle streams the summary for an article using external API.
func (r *externalAPIRepository) StreamSummarizeArticle(ctx context.Context, article *domain.Article, priority string) (io.ReadCloser, error) {
	// Input validation
	if article == nil {
		r.logger.ErrorContext(ctx, "article cannot be nil")
		return nil, fmt.Errorf("article cannot be nil")
	}

	if article.ID == "" {
		r.logger.ErrorContext(ctx, "article ID cannot be empty")
		return nil, fmt.Errorf("article ID cannot be empty")
	}

	if article.Content == "" {
		r.logger.ErrorContext(ctx, "article content cannot be empty", "article_id", article.ID)
		return nil, fmt.Errorf("article content cannot be empty")
	}

	r.logger.InfoContext(ctx, "streaming summary for article", "article_id", article.ID)

	payload := driver.ArticlePayload{
		ID:      article.ID,
		Title:   article.Title,
		Content: article.Content,
	}

	// Use driver function for streaming
	streamBody, err := driver.StreamArticleSummarizerAPIClient(ctx, payload, r.config, r.logger, priority, r.client)
	if err != nil {
		if errors.Is(err, driver.ErrContentTooShort) {
			r.logger.InfoContext(ctx, "skipping summarization: content too short", "article_id", article.ID)
			return nil, domain.ErrContentTooShort
		}
		if errors.Is(err, driver.ErrContentTooLong) {
			r.logger.ErrorContext(ctx, "failed to start streaming summary", "error", err, "article_id", article.ID)
			return nil, domain.ErrContentTooLong
		}
		if errors.Is(err, driver.ErrServiceOverloaded) {
			r.logger.WarnContext(ctx, "downstream service overloaded", "article_id", article.ID)
			return nil, domain.ErrServiceOverloaded
		}
		if errors.Is(err, driver.ErrContentNotProcessable) {
			r.logger.WarnContext(ctx, "content not processable by model", "article_id", article.ID)
			return nil, domain.ErrContentNotProcessable
		}
		if errors.Is(err, driver.ErrUpstreamBusy) {
			r.logger.WarnContext(ctx, "failed to start streaming summary: upstream busy", "error", err, "article_id", article.ID)
			return nil, fmt.Errorf("%w: %w", domain.ErrUpstreamBusy, err)
		}
		if errors.Is(err, driver.ErrInvalidRequest) {
			r.logger.ErrorContext(ctx, "failed to start streaming summary", "error", err, "article_id", article.ID)
			return nil, fmt.Errorf("%w: %w", domain.ErrInvalidRequest, err)
		}
		r.logger.ErrorContext(ctx, "failed to start streaming summary", "error", err, "article_id", article.ID)
		return nil, fmt.Errorf("failed to start streaming summary: %w", err)
	}

	r.logger.InfoContext(ctx, "streaming started successfully", "article_id", article.ID)
	return streamBody, nil
}

// CheckHealth checks the health of an external service.
func (r *externalAPIRepository) CheckHealth(ctx context.Context, serviceURL string) error {
	// Input validation
	if serviceURL == "" {
		r.logger.ErrorContext(ctx, "service URL cannot be empty")
		return fmt.Errorf("service URL cannot be empty")
	}

	// Validate URL format
	if !strings.HasPrefix(serviceURL, "http://") && !strings.HasPrefix(serviceURL, "https://") {
		r.logger.ErrorContext(ctx, "invalid service URL", "url", serviceURL)
		return fmt.Errorf("invalid service URL: must start with http:// or https://")
	}

	// Parse URL to ensure it's valid
	parsedURL, err := url.Parse(serviceURL)
	if err != nil {
		r.logger.ErrorContext(ctx, "invalid service URL format", "url", serviceURL, "error", err)
		return fmt.Errorf("invalid service URL: %w", err)
	}

	r.logger.InfoContext(ctx, "checking service health", "url", serviceURL)

	// GREEN PHASE: Basic health check implementation
	healthEndpoint := parsedURL.String() + "/health"

	req, err := http.NewRequestWithContext(ctx, "GET", healthEndpoint, http.NoBody)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to create health check request", "error", err)
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		r.logger.ErrorContext(ctx, "health check request failed", "error", err)
		return fmt.Errorf("health check request failed: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			r.logger.WarnContext(ctx, "failed to close health check response body", "error", cerr, "url", healthEndpoint)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		r.logger.ErrorContext(ctx, "service not healthy", "status", resp.StatusCode)
		return fmt.Errorf("service not healthy: status %d", resp.StatusCode)
	}

	r.logger.InfoContext(ctx, "service is healthy")

	return nil
}
