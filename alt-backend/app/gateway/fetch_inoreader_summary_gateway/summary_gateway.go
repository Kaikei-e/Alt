package fetch_inoreader_summary_gateway

import (
	"alt/domain"
	"alt/driver/models"
	"alt/port/fetch_inoreader_summary_port"
	"alt/utils/logger"
	"context"
	"fmt"
	"time"

	"golang.org/x/time/rate"
)

// Database interface for dependency injection
type Database interface {
	FetchInoreaderSummariesByURLs(ctx context.Context, urls []string) ([]*models.InoreaderSummary, error)
}

type inoreaderSummaryGateway struct {
	db      Database
	limiter *rate.Limiter
}

// NewInoreaderSummaryGateway creates a new inoreader summary gateway
func NewInoreaderSummaryGateway(db Database) fetch_inoreader_summary_port.FetchInoreaderSummaryPort {
	// Rate limiter: 5 second intervals as per CLAUDE.md guidelines
	limiter := rate.NewLimiter(rate.Every(5*time.Second), 1)

	return &inoreaderSummaryGateway{
		db:      db,
		limiter: limiter,
	}
}

// FetchSummariesByURLs implements the port interface
func (g *inoreaderSummaryGateway) FetchSummariesByURLs(ctx context.Context, urls []string) ([]*domain.InoreaderSummary, error) {
	logger.Logger.Info("Gateway: fetching inoreader summaries",
		"url_count", len(urls))

	// Apply rate limiting as per CLAUDE.md requirements (5 second intervals)
	if err := g.limiter.Wait(ctx); err != nil {
		logger.Logger.Error("Rate limit wait failed", "error", err)
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	// Handle empty input
	if len(urls) == 0 {
		logger.Logger.Info("No URLs provided, returning empty result")
		return []*domain.InoreaderSummary{}, nil
	}

	// Call database driver
	modelSummaries, err := g.db.FetchInoreaderSummariesByURLs(ctx, urls)
	if err != nil {
		logger.Logger.Error("Database query failed", "error", err, "url_count", len(urls))
		return nil, fmt.Errorf("database query failed: %w", err)
	}

	// Convert models to domain entities (Anti-corruption layer)
	domainSummaries := make([]*domain.InoreaderSummary, 0, len(modelSummaries))
	for _, modelSummary := range modelSummaries {
		domainSummary := &domain.InoreaderSummary{
			ArticleURL:  modelSummary.ArticleURL,
			Title:       modelSummary.Title,
			Author:      modelSummary.Author,
			Content:     modelSummary.Content,
			ContentType: modelSummary.ContentType,
			PublishedAt: modelSummary.PublishedAt,
			FetchedAt:   modelSummary.FetchedAt,
			InoreaderID: modelSummary.InoreaderID,
		}
		domainSummaries = append(domainSummaries, domainSummary)
	}

	logger.Logger.Info("Gateway: successfully converted summaries",
		"matched_count", len(domainSummaries),
		"requested_count", len(urls))

	return domainSummaries, nil
}
