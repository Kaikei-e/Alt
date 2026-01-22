package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"pre-processor/config"
	"pre-processor/domain"
	"pre-processor/driver"
	"pre-processor/models"
)

// ExternalAPIRepository implementation.
type externalAPIRepository struct {
	logger *slog.Logger
	client *http.Client
	config *config.Config
}

// NewExternalAPIRepository creates a new external API repository.
func NewExternalAPIRepository(cfg *config.Config, logger *slog.Logger) ExternalAPIRepository {
	return &externalAPIRepository{
		logger: logger,
		config: cfg,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SummarizeArticle summarizes an article using external API.
func (r *externalAPIRepository) SummarizeArticle(ctx context.Context, article *models.Article, priority string) (*models.SummarizedContent, error) {
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

	// Use existing driver function
	driverSummary, err := driver.ArticleSummarizerAPIClient(ctx, article, r.config, r.logger, priority)
	if err != nil {
		// Handle content too short as a normal case, not an error
		if errors.Is(err, domain.ErrContentTooShort) {
			r.logger.InfoContext(ctx, "skipping summarization: content too short", "article_id", article.ID)
			return nil, domain.ErrContentTooShort
		}
		r.logger.ErrorContext(ctx, "failed to summarize article", "error", err, "article_id", article.ID)
		return nil, fmt.Errorf("failed to summarize article: %w", err)
	}

	// Convert driver type to models type
	summarizedContent := &models.SummarizedContent{
		ArticleID:       driverSummary.ArticleID,
		SummaryJapanese: driverSummary.SummaryJapanese,
	}

	r.logger.InfoContext(ctx, "article summarized successfully", "article_id", article.ID)

	return summarizedContent, nil
}

// StreamSummarizeArticle streams the summary for an article using external API.
func (r *externalAPIRepository) StreamSummarizeArticle(ctx context.Context, article *models.Article, priority string) (io.ReadCloser, error) {
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

	// Use driver function for streaming
	streamBody, err := driver.StreamArticleSummarizerAPIClient(ctx, article, r.config, r.logger, priority)
	if err != nil {
		if errors.Is(err, domain.ErrContentTooShort) {
			r.logger.InfoContext(ctx, "skipping summarization: content too short", "article_id", article.ID)
			return nil, domain.ErrContentTooShort
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

// GetSystemUserID retrieves the system user ID from alt-backend with retry logic.
func (r *externalAPIRepository) GetSystemUserID(ctx context.Context) (string, error) {
	const maxRetries = 3
	baseDelay := 2 * time.Second

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		userID, err := r.getSystemUserIDOnce(ctx)
		if err == nil {
			return userID, nil
		}
		lastErr = err

		// Don't retry on context cancellation
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		// Log retry attempt (except for last attempt)
		if attempt < maxRetries-1 {
			delay := baseDelay * time.Duration(1<<attempt) // 2s, 4s
			r.logger.WarnContext(ctx, "GetSystemUserID failed, retrying",
				"attempt", attempt+1,
				"max_attempts", maxRetries,
				"delay", delay,
				"error", err)

			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	return "", fmt.Errorf("GetSystemUserID failed after %d attempts: %w", maxRetries, lastErr)
}

// getSystemUserIDOnce performs a single attempt to retrieve the system user ID.
func (r *externalAPIRepository) getSystemUserIDOnce(ctx context.Context) (string, error) {
	targetURL := fmt.Sprintf("%s/v1/internal/system-user", r.config.AltService.Host)

	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			r.logger.WarnContext(ctx, "failed to close system user response body", "error", cerr, "url", parsedURL.String())
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result struct {
		UserID string `json:"user_id"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if result.UserID == "" {
		return "", fmt.Errorf("received empty user_id")
	}

	return result.UserID, nil
}
