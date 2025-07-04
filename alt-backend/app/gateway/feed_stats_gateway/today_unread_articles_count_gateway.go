package feed_stats_gateway

import (
	"alt/driver/alt_db"
	"alt/utils/errors"
	"alt/utils/logger"
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type TodayUnreadArticlesCountGateway struct {
	altDBRepository *alt_db.AltDBRepository
}

func NewTodayUnreadArticlesCountGateway(pool *pgxpool.Pool) *TodayUnreadArticlesCountGateway {
	return &TodayUnreadArticlesCountGateway{
		altDBRepository: alt_db.NewAltDBRepositoryWithPool(pool),
	}
}

func (g *TodayUnreadArticlesCountGateway) Execute(ctx context.Context, since time.Time) (int, error) {
	if g.altDBRepository == nil {
		dbErr := errors.DatabaseError("database connection not available", nil, map[string]interface{}{
			"gateway": "TodayUnreadArticlesCountGateway",
			"method":  "Execute",
		})
		errors.LogError(logger.Logger, dbErr, "database_connection_check")
		return 0, dbErr
	}

	count, err := g.altDBRepository.FetchTodayUnreadArticlesCount(ctx, since)
	if err != nil {
		dbErr := errors.DatabaseError("failed to fetch today's unread articles count", err, map[string]interface{}{
			"gateway": "TodayUnreadArticlesCountGateway",
			"method":  "FetchTodayUnreadArticlesCount",
		})
		errors.LogError(logger.Logger, dbErr, "fetch_today_unread_articles_count")
		return 0, dbErr
	}

	return count, nil
}
