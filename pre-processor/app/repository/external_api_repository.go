package repository

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"pre-processor/config"
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
func (r *externalAPIRepository) SummarizeArticle(ctx context.Context, article *models.Article) (*models.SummarizedContent, error) {
	// Input validation
	if article == nil {
		r.logger.Error("article cannot be nil")
		return nil, fmt.Errorf("article cannot be nil")
	}

	if article.ID == "" {
		r.logger.Error("article ID cannot be empty")
		return nil, fmt.Errorf("article ID cannot be empty")
	}

	if article.Content == "" {
		r.logger.Error("article content cannot be empty", "article_id", article.ID)
		return nil, fmt.Errorf("article content cannot be empty")
	}

	r.logger.Info("summarizing article", "article_id", article.ID)

	// Use existing driver function
	driverSummary, err := driver.ArticleSummarizerAPIClient(ctx, article, r.config, r.logger)
	if err != nil {
		r.logger.Error("failed to summarize article", "error", err, "article_id", article.ID)
		return nil, fmt.Errorf("failed to summarize article: %w", err)
	}

	// Convert driver type to models type
	summarizedContent := &models.SummarizedContent{
		ArticleID:       driverSummary.ArticleID,
		SummaryJapanese: driverSummary.SummaryJapanese,
	}

	r.logger.Info("article summarized successfully", "article_id", article.ID)

	return summarizedContent, nil
}

// CheckHealth checks the health of an external service.
func (r *externalAPIRepository) CheckHealth(ctx context.Context, serviceURL string) error {
	// Input validation
	if serviceURL == "" {
		r.logger.Error("service URL cannot be empty")
		return fmt.Errorf("service URL cannot be empty")
	}

	// Validate URL format
	if !strings.HasPrefix(serviceURL, "http://") && !strings.HasPrefix(serviceURL, "https://") {
		r.logger.Error("invalid service URL", "url", serviceURL)
		return fmt.Errorf("invalid service URL: must start with http:// or https://")
	}

	// Parse URL to ensure it's valid
	parsedURL, err := url.Parse(serviceURL)
	if err != nil {
		r.logger.Error("invalid service URL format", "url", serviceURL, "error", err)
		return fmt.Errorf("invalid service URL: %w", err)
	}

	r.logger.Info("checking service health", "url", serviceURL)

	// GREEN PHASE: Basic health check implementation
	healthEndpoint := parsedURL.String() + "/api/tags"

	req, err := http.NewRequestWithContext(ctx, "GET", healthEndpoint, http.NoBody)
	if err != nil {
		r.logger.Error("failed to create health check request", "error", err)
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		r.logger.Error("health check request failed", "error", err)
		return fmt.Errorf("health check request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		r.logger.Error("service not healthy", "status", resp.StatusCode)
		return fmt.Errorf("service not healthy: status %d", resp.StatusCode)
	}

	r.logger.Info("service is healthy")

	return nil
}
