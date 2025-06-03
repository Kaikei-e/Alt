package register_feed_gateway

import (
	"alt/driver/alt_db"
	"alt/port/register_feed_port"
	"alt/utils/logger"
	"context"

	"github.com/jackc/pgx/v5"
)

type RegisterFeedGateway struct {
	registerFeedPort register_feed_port.RegisterFeedPort
	alt_db           *alt_db.AltDBRepository
}

func NewRegisterFeedGateway(registerFeedPort register_feed_port.RegisterFeedPort, db *pgx.Conn) *RegisterFeedGateway {
	return &RegisterFeedGateway{registerFeedPort: registerFeedPort, alt_db: alt_db.NewAltDBRepository(db)}
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
