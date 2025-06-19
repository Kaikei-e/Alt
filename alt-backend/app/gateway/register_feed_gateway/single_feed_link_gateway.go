package register_feed_gateway

import (
	"alt/driver/alt_db"
	"alt/utils/logger"
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mmcdole/gofeed"
)

type RegisterFeedGateway struct {
	alt_db *alt_db.AltDBRepository
}

func NewRegisterFeedLinkGateway(pool *pgxpool.Pool) *RegisterFeedGateway {
	return &RegisterFeedGateway{alt_db: alt_db.NewAltDBRepositoryWithPool(pool)}
}

func (g *RegisterFeedGateway) RegisterRSSFeedLink(ctx context.Context, link string) error {
	if g.alt_db == nil {
		return errors.New("database repository is not initialized")
	}
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(link)
	if err != nil {
		logger.Logger.Error("Error parsing RSS feed link", "error", err)
		return err
	}

	if feed.Link == "" {
		logger.Logger.Warn("RSS feed link is empty, using the link from the RSS feed", "link", link)
		feed.Link = link
	}

	if feed.FeedLink == "" {
		logger.Logger.Warn("RSS feed feed link is empty, using the link from the RSS feed", "link", feed.Link)
		feed.FeedLink = link
	}

	err = g.alt_db.RegisterRSSFeedLink(ctx, feed.FeedLink)
	if err != nil {
		if errors.Is(err, pgx.ErrTxClosed) {
			logger.Logger.Error("Failed to register RSS feed link", "error", err)
			return errors.New("failed to register RSS feed link")
		}
		logger.Logger.Error("Error registering RSS feed link", "error", err)
		return errors.New("failed to register RSS feed link")
	}
	logger.Logger.Info("RSS feed link registered", "link", link)

	return nil
}
