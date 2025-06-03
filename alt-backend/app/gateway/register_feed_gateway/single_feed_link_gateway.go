package register_feed_gateway

import (
	"alt/driver/alt_db"
	"alt/utils/logger"
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/mmcdole/gofeed"
)

type RegisterFeedGateway struct {
	alt_db *alt_db.AltDBRepository
}

func NewRegisterFeedLinkGateway(db *pgx.Conn) *RegisterFeedGateway {
	return &RegisterFeedGateway{alt_db: alt_db.NewAltDBRepository(db)}
}

func (g *RegisterFeedGateway) RegisterRSSFeedLink(ctx context.Context, link string) error {
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(link)
	if err != nil {
		logger.Logger.Error("Error parsing RSS feed link", "error", err)
		return err
	}

	if feed.Link == "" {
		logger.Logger.Error("RSS feed link is empty", "link", link)
		return errors.New("RSS feed link is empty")
	}

	err = g.alt_db.RegisterRSSFeedLink(ctx, feed.FeedLink)
	if err != nil {
		logger.Logger.Error("Error registering RSS feed link", "error", err)
		return err
	}
	logger.Logger.Info("RSS feed link registered", "link", link)

	return nil
}
