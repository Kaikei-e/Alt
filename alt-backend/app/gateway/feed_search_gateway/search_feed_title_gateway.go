package feed_search_gateway

import (
	"context"

	"alt/domain"
	"alt/driver/alt_db"
	"alt/utils/logger"

	"github.com/jackc/pgx/v5/pgxpool"
)

type SearchByTitleGateway struct {
	alt_db *alt_db.AltDBRepository
}

func NewSearchByTitleGateway(pool *pgxpool.Pool) *SearchByTitleGateway {
	return &SearchByTitleGateway{alt_db: alt_db.NewAltDBRepositoryWithPool(pool)}
}

func (g *SearchByTitleGateway) SearchFeedsByTitle(ctx context.Context, query string, userID string) ([]*domain.FeedItem, error) {
	logger.GlobalContext.WithContext(ctx).InfoContext(ctx, "gateway: searching feeds by title",
		"query", query,
		"user_id", userID)

	feeds, err := g.alt_db.SearchFeedsByTitle(ctx, query, userID)
	if err != nil {
		logger.GlobalContext.WithContext(ctx).ErrorContext(ctx, "gateway: failed to search feeds by title",
			"error", err,
			"query", query,
			"user_id", userID)
		return nil, err
	}

	logger.GlobalContext.WithContext(ctx).InfoContext(ctx, "gateway: feed search completed",
		"query", query,
		"user_id", userID,
		"results_count", len(feeds))

	return feeds, nil
}
