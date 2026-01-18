package fetch_inoreader_summary_gateway

import (
	"alt/domain"
	"alt/driver/models"
	"alt/port/fetch_inoreader_summary_port"
	"alt/utils"
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
	logger.Logger.InfoContext(ctx, "Gateway: fetching inoreader summaries",
		"url_count", len(urls),
		"urls", urls)

	// Apply rate limiting as per CLAUDE.md requirements (5 second intervals)
	if err := g.limiter.Wait(ctx); err != nil {
		logger.Logger.ErrorContext(ctx, "Rate limit wait failed", "error", err)
		return nil, fmt.Errorf("rate limit exceeded: %w", err)
	}

	// Handle empty input
	if len(urls) == 0 {
		logger.Logger.InfoContext(ctx, "No URLs provided, returning empty result")
		return []*domain.InoreaderSummary{}, nil
	}

	// Normalize URLs for matching: include both original and normalized URLs
	allURLs := make(map[string]bool)
	originalToNormalized := make(map[string]string)

	for _, rawURL := range urls {
		allURLs[rawURL] = true
		normalized, err := utils.NormalizeURL(rawURL)
		if err != nil {
			logger.Logger.WarnContext(ctx, "Failed to normalize URL, using original", "url", rawURL, "error", err)
			normalized = rawURL
		}
		originalToNormalized[rawURL] = normalized
		if normalized != rawURL {
			allURLs[normalized] = true
		}
	}

	// Convert map to slice for query
	allURLsSlice := make([]string, 0, len(allURLs))
	for url := range allURLs {
		allURLsSlice = append(allURLsSlice, url)
	}

	logger.Logger.InfoContext(ctx, "Gateway: normalized URLs for matching",
		"original_count", len(urls),
		"all_urls_count", len(allURLsSlice))

	// Call database driver with both original and normalized URLs
	modelSummaries, err := g.db.FetchInoreaderSummariesByURLs(ctx, allURLsSlice)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Database query failed", "error", err, "url_count", len(allURLsSlice))
		return nil, fmt.Errorf("database query failed: %w", err)
	}

	// Filter results by normalizing database URLs and comparing with requested URLs
	domainSummaries := make([]*domain.InoreaderSummary, 0, len(modelSummaries))
	for _, modelSummary := range modelSummaries {
		// Normalize database URL
		normalizedDBURL, err := utils.NormalizeURL(modelSummary.ArticleURL)
		if err != nil {
			logger.Logger.WarnContext(ctx, "Failed to normalize DB URL, using original", "url", modelSummary.ArticleURL, "error", err)
			normalizedDBURL = modelSummary.ArticleURL
		}

		// Check if this URL matches any of the requested URLs (original or normalized)
		matched := false
		for _, reqURL := range urls {
			normalizedReqURL := originalToNormalized[reqURL]
			// Compare using URLsEqual for case-insensitive percent-encoding
			if utils.URLsEqual(modelSummary.ArticleURL, reqURL) || utils.URLsEqual(normalizedDBURL, normalizedReqURL) {
				matched = true
				break
			}
		}

		if matched {
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
	}

	logger.Logger.InfoContext(ctx, "Gateway: successfully converted summaries",
		"matched_count", len(domainSummaries),
		"requested_count", len(urls))

	return domainSummaries, nil
}
