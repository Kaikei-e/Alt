package latest_article_gateway

import (
	"alt/domain"
	"alt/orchestrator/port/latest_article_port"
	"context"

	"github.com/google/uuid"
)

// latestArticleSource is catalog §2.C W3-C7, served by alt-data-hub since
// ADR-000954 Wave 3 batch 2.
type latestArticleSource interface {
	FetchLatestArticleByFeedID(ctx context.Context, feedID uuid.UUID) (*domain.ArticleContent, error)
}

// Verify interface compliance at compile time.
var _ latest_article_port.FetchLatestArticlePort = (*Gateway)(nil)

// Gateway implements FetchLatestArticlePort.
type Gateway struct {
	repo latestArticleSource
}

// NewGateway panics on a nil source. FetchRandomFeed is a user-facing route,
// so a nil here would surface as a panic mid-request instead of at boot.
func NewGateway(repo latestArticleSource) *Gateway {
	if repo == nil {
		panic("latest_article_gateway: an article source is required — articles moved to alt-data-hub in ADR-000954 Wave 3")
	}
	return &Gateway{repo: repo}
}

// FetchLatestArticleByFeedID retrieves the most recent article for a given feed.
func (g *Gateway) FetchLatestArticleByFeedID(ctx context.Context, feedID uuid.UUID) (*domain.ArticleContent, error) {
	return g.repo.FetchLatestArticleByFeedID(ctx, feedID)
}
