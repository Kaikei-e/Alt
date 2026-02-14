package latest_article_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/port/latest_article_port"
	"context"

	"github.com/google/uuid"
)

// Verify interface compliance at compile time.
var _ latest_article_port.FetchLatestArticlePort = (*Gateway)(nil)

// Gateway implements FetchLatestArticlePort using AltDBRepository.
type Gateway struct {
	repo *alt_db.AltDBRepository
}

// NewGateway creates a new latest article gateway.
func NewGateway(repo *alt_db.AltDBRepository) *Gateway {
	return &Gateway{repo: repo}
}

// FetchLatestArticleByFeedID retrieves the most recent article for a given feed.
func (g *Gateway) FetchLatestArticleByFeedID(ctx context.Context, feedID uuid.UUID) (*domain.ArticleContent, error) {
	return g.repo.FetchLatestArticleByFeedID(ctx, feedID)
}
