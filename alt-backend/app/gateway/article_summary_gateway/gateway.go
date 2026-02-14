package article_summary_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/port/article_summary_port"
	"context"
	"net/url"
)

// Verify interface compliance at compile time.
var _ article_summary_port.FetchArticleSummaryPort = (*Gateway)(nil)

// Gateway implements FetchArticleSummaryPort using AltDBRepository.
type Gateway struct {
	repo *alt_db.AltDBRepository
}

// NewGateway creates a new article summary gateway.
func NewGateway(repo *alt_db.AltDBRepository) *Gateway {
	return &Gateway{repo: repo}
}

// FetchFeedSummary retrieves the AI-generated summary for a given feed URL.
func (g *Gateway) FetchFeedSummary(ctx context.Context, feedURL *url.URL) (*domain.FeedSummary, error) {
	return g.repo.FetchFeedSummary(ctx, feedURL)
}
