package feed_stats_gateway

import (
	"alt/driver/alt_db"
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type FeedAmountGateway struct {
	altDBRepository *alt_db.AltDBRepository
}

func NewFeedAmountGateway(pool *pgxpool.Pool) *FeedAmountGateway {
	return &FeedAmountGateway{
		altDBRepository: alt_db.NewAltDBRepository(pool),
	}
}

func (g *FeedAmountGateway) Execute(ctx context.Context) (int, error) {
	return g.altDBRepository.FetchFeedAmount(ctx)
}
