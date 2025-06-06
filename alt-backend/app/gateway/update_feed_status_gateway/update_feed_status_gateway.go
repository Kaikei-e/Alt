package update_feed_status_gateway

import (
	"alt/driver/alt_db"
	"context"
	"net/url"

	"github.com/jackc/pgx/v5"
)

type UpdateFeedStatusGateway struct {
	db *alt_db.AltDBRepository
}

func NewUpdateFeedStatusGateway(db *pgx.Conn) *UpdateFeedStatusGateway {
	return &UpdateFeedStatusGateway{db: alt_db.NewAltDBRepository(db)}
}

func (g *UpdateFeedStatusGateway) UpdateFeedStatus(ctx context.Context, feedURL url.URL) error {
	return g.db.UpdateFeedStatus(ctx, feedURL)
}
