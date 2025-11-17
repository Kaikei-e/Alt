package feed_link_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FeedLinkGateway struct {
	altDB *alt_db.AltDBRepository
}

func NewFeedLinkGateway(pool *pgxpool.Pool) *FeedLinkGateway {
	return &FeedLinkGateway{altDB: alt_db.NewAltDBRepositoryWithPool(pool)}
}

func (g *FeedLinkGateway) ListFeedLinks(ctx context.Context) ([]*domain.FeedLink, error) {
	if g.altDB == nil {
		return nil, errors.New("database connection not available")
	}
	return g.altDB.FetchFeedLinks(ctx)
}

func (g *FeedLinkGateway) DeleteFeedLink(ctx context.Context, id uuid.UUID) error {
	if g.altDB == nil {
		return errors.New("database connection not available")
	}
	return g.altDB.DeleteFeedLink(ctx, id)
}
