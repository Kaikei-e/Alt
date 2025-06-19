package update_feed_status_gateway

import (
	"alt/driver/alt_db"
	"context"
	"errors"
	"net/url"

	"github.com/jackc/pgx/v5/pgxpool"
)

type UpdateFeedStatusGateway struct {
	db *alt_db.AltDBRepository
}

func NewUpdateFeedStatusGateway(pool *pgxpool.Pool) *UpdateFeedStatusGateway {
	return &UpdateFeedStatusGateway{db: alt_db.NewAltDBRepositoryWithPool(pool)}
}

func (g *UpdateFeedStatusGateway) UpdateFeedStatus(ctx context.Context, feedURL url.URL) error {
	if g.db == nil {
		return errors.New("database connection not available")
	}
	return g.db.UpdateFeedStatus(ctx, feedURL)
}
