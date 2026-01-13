package trend_stats_gateway

import (
	"context"

	"alt/driver/alt_db"
	"alt/port/trend_stats_port"
	"alt/utils/errors"
	"alt/utils/logger"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TrendStatsGateway implements the TrendStatsPort interface
type TrendStatsGateway struct {
	altDBRepository *alt_db.AltDBRepository
}

// NewTrendStatsGateway creates a new TrendStatsGateway with the given database pool
func NewTrendStatsGateway(pool *pgxpool.Pool) *TrendStatsGateway {
	return &TrendStatsGateway{
		altDBRepository: alt_db.NewAltDBRepositoryWithPool(pool),
	}
}

// Execute fetches trend statistics for the given time window
func (g *TrendStatsGateway) Execute(ctx context.Context, window string) (*trend_stats_port.TrendDataResponse, error) {
	if g.altDBRepository == nil {
		dbErr := errors.DatabaseError("database connection not available", nil, map[string]interface{}{
			"gateway": "TrendStatsGateway",
			"method":  "Execute",
		})
		errors.LogError(logger.Logger, dbErr, "database_connection_check")
		return nil, dbErr
	}

	result, err := g.altDBRepository.FetchTrendStats(ctx, window)
	if err != nil {
		dbErr := errors.DatabaseError("failed to fetch trend stats", err, map[string]interface{}{
			"gateway": "TrendStatsGateway",
			"method":  "FetchTrendStats",
			"window":  window,
		})
		errors.LogError(logger.Logger, dbErr, "fetch_trend_stats")
		return nil, dbErr
	}

	return result, nil
}
