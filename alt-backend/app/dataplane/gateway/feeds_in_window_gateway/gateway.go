package feeds_in_window_gateway

import (
	"alt/dataplane/port/feeds_in_window_port"
	"alt/domain"
	"alt/shared/driver/alt_db"
	"context"
)

// Ensure the gateway satisfies the port interface.
var _ feeds_in_window_port.FeedsInWindowPort = (*Gateway)(nil)

// Gateway adapts the feeds in window port to the Alt DB repository.
type Gateway struct {
	repo *alt_db.AltDBRepository
}

// NewGateway constructs a feeds in window gateway.
func NewGateway(repo *alt_db.AltDBRepository) *Gateway {
	return &Gateway{repo: repo}
}

// FetchFeedsInWindow delegates to the underlying repository.
func (g *Gateway) FetchFeedsInWindow(ctx context.Context, query domain.FeedsInWindowQuery) (*domain.FeedsInWindowPage, error) {
	return g.repo.FetchFeedsInWindow(ctx, query)
}
