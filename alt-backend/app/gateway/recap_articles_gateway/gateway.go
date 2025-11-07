package recap_articles_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/port/recap_articles_port"
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
