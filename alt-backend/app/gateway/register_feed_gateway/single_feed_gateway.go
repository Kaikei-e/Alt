package register_feed_gateway

import (
	"alt/driver/alt_db"
	"alt/utils/logger"
	"context"

	"github.com/jackc/pgx/v5"
)

type RegisterFeedGateway struct {
	alt_db *alt_db.AltDBRepository
}

func NewRegisterFeedGateway(db *pgx.Conn) *RegisterFeedGateway {
	return &RegisterFeedGateway{alt_db: alt_db.NewAltDBRepository(db)}
}

func (g *RegisterFeedGateway) RegisterRSSFeedLink(ctx context.Context, link string) error {
	err := g.alt_db.RegisterRSSFeedLink(ctx, link)
	if err != nil {
		logger.Logger.Error("Error registering RSS feed link", "error", err)
		return err
	}
	logger.Logger.Info("RSS feed link registered", "link", link)
	return nil
}
