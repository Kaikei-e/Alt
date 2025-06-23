package repository

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"pre-processor/driver"
	"pre-processor/models"
	"time"
)

// ExternalAPIRepository implementation
type externalAPIRepository struct {
	logger *slog.Logger
	client *http.Client
}

// NewExternalAPIRepository creates a new external API repository
func NewExternalAPIRepository(logger *slog.Logger) ExternalAPIRepository {
	return &externalAPIRepository{
		logger: logger,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SummarizeArticle summarizes an article using external API
func (r *externalAPIRepository) SummarizeArticle(ctx context.Context, article *models.Article) (*models.SummarizedContent, error) {
	r.logger.Info("summarizing article", "article_id", article.ID)

	// Use existing driver function
	driverSummary, err := driver.ArticleSummarizerAPIClient(ctx, article)
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

// CheckHealth checks the health of an external service
func (r *externalAPIRepository) CheckHealth(ctx context.Context, serviceURL string) error {
	r.logger.Info("checking service health", "url", serviceURL)

	// GREEN PHASE: Basic health check implementation
	req, err := http.NewRequestWithContext(ctx, "GET", serviceURL+"/api/tags", nil)
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
