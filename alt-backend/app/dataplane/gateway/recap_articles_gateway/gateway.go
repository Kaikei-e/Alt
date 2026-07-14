package recap_articles_gateway

import (
	"alt/dataplane/port/recap_articles_port"
	"alt/domain"
	"alt/shared/driver/alt_db"
	"context"
)

// Ensure the gateway satisfies the port interface.
var _ recap_articles_port.RecapArticlesPort = (*Gateway)(nil)

// Gateway adapts the recap articles port to the Alt DB repository.
type Gateway struct {
	repo *alt_db.AltDBRepository
}

// NewGateway constructs a recap articles gateway.
func NewGateway(repo *alt_db.AltDBRepository) *Gateway {
	return &Gateway{repo: repo}
}

// FetchRecapArticles delegates to the underlying repository.
func (g *Gateway) FetchRecapArticles(ctx context.Context, query domain.RecapArticlesQuery) (*domain.RecapArticlesPage, error) {
	return g.repo.FetchRecapArticles(ctx, query)
}
