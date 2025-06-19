package feed_stats_gateway

import (
	"alt/driver/alt_db"
	"context"
	"errors"

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
		return 0, errors.New("database connection not available")
	}
	return g.altDBRepository.FetchFeedAmount(ctx)
}
