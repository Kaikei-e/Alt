package feed_stats_gateway

import (
	"alt/driver/alt_db"
	"alt/utils/logger"
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type TotalArticlesCountGateway struct {
	altDBRepository *alt_db.AltDBRepository
}

func NewTotalArticlesCountGateway(pool *pgxpool.Pool) *TotalArticlesCountGateway {
	return &TotalArticlesCountGateway{
		altDBRepository: alt_db.NewAltDBRepositoryWithPool(pool),
	}
}

func (g *TotalArticlesCountGateway) Execute(ctx context.Context) (int, error) {
	if g.altDBRepository == nil {
		logger.SafeErrorContext(ctx, "database repository is nil",
			"gateway", "TotalArticlesCountGateway")
		return 0, fmt.Errorf("database connection not available")
	}

	totalCount, err := g.altDBRepository.FetchTotalArticlesCount(ctx)
	if err != nil {
		logger.SafeErrorContext(ctx, "failed to fetch total articles count from database",
			"error", err,
			"gateway", "TotalArticlesCountGateway")
		return 0, fmt.Errorf("failed to fetch total articles count: %w", err)
	}

	logger.SafeInfoContext(ctx, "total articles count fetched from database successfully",
		"count", totalCount,
		"gateway", "TotalArticlesCountGateway")
	return totalCount, nil
}
