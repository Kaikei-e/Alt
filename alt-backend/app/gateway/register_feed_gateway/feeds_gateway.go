package register_feed_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/driver/models"
	"alt/utils/logger"
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type RegisterFeedsGateway struct {
	alt_db *alt_db.AltDBRepository
}

func NewRegisterFeedsGateway(pool *pgxpool.Pool) *RegisterFeedsGateway {
	return &RegisterFeedsGateway{alt_db: alt_db.NewAltDBRepositoryWithPool(pool)}
}

func (g *RegisterFeedsGateway) RegisterFeeds(ctx context.Context, feeds []*domain.FeedItem) error {
	if g.alt_db == nil {
		return errors.New("database connection not available")
	}
	var items []models.Feed
	for _, feedItem := range feeds {
		// Additional validation: Skip feeds with empty titles as a safety net
		if strings.TrimSpace(feedItem.Title) == "" {
			logger.Logger.Warn("Skipping feed registration with empty title", 
				"link", feedItem.Link,
				"description", feedItem.Description)
			continue
		}

		feedModel := &models.Feed{
			Title:       strings.TrimSpace(feedItem.Title),
			Description: feedItem.Description,
			Link:        feedItem.Link,
			PubDate:     feedItem.PublishedParsed,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}

		logger.SafeInfo("Feed model link", "feedModel", feedModel.Link)
		items = append(items, *feedModel)
	}

	err := g.alt_db.RegisterMultipleFeeds(ctx, items)
	if err != nil {
		logger.SafeError("Error registering multiple feeds", "error", err)
		return err
	}

	logger.SafeInfo("Feeds registered", "number of feeds", len(items))

	return nil
}
