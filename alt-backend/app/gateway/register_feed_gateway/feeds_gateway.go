package register_feed_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/driver/models"
	"alt/utils/logger"
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type RegisterFeedsGateway struct {
	alt_db *alt_db.AltDBRepository
}

func NewRegisterFeedsGateway(db *pgx.Conn) *RegisterFeedsGateway {
	return &RegisterFeedsGateway{alt_db: alt_db.NewAltDBRepository(db)}
}

func (g *RegisterFeedsGateway) RegisterFeeds(ctx context.Context, feeds []*domain.FeedItem) error {
	var items []models.Feed
	for _, feedItem := range feeds {
		feedModel := &models.Feed{
			Title:       feedItem.Title,
			Description: feedItem.Description,
			Link:        feedItem.Link,
			PubDate:     feedItem.PublishedParsed,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		logger.Logger.Info("Feed model link", "feedModel", feedModel.Link)
		items = append(items, *feedModel)
	}

	err := g.alt_db.RegisterMultipleFeeds(ctx, items)
	if err != nil {
		logger.Logger.Error("Error registering multiple feeds", "error", err)
		return err
	}

	logger.Logger.Info("Feeds registered", "number of feeds", len(items))

	return nil
}
