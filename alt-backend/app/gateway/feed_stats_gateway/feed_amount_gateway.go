package feed_stats_gateway

import (
	"alt/driver/alt_db"
	"alt/utils/errors"
	"alt/utils/logger"
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type FeedAmountGateway struct {
	altDBRepository *alt_db.AltDBRepository
}

func NewFeedAmountGateway(pool *pgxpool.Pool) *FeedAmountGateway {
	return &FeedAmountGateway{
		altDBRepository: alt_db.NewAltDBRepositoryWithPool(pool),
	}
}

func (g *FeedAmountGateway) Execute(ctx context.Context) (int, error) {
	if g.altDBRepository == nil {
		dbErr := errors.DatabaseError("database connection not available", nil, map[string]interface{}{
			"gateway": "FeedAmountGateway",
			"method":  "Execute",
		})
		errors.LogError(logger.Logger, dbErr, "database_connection_check")
		return 0, dbErr
	}

	count, err := g.altDBRepository.FetchFeedAmount(ctx)
	if err != nil {
		dbErr := errors.DatabaseError("failed to fetch feed amount", err, map[string]interface{}{
			"gateway": "FeedAmountGateway",
			"method":  "FetchFeedAmount",
		})
		errors.LogError(logger.Logger, dbErr, "fetch_feed_amount")
		return 0, dbErr
	}

	return count, nil
}
