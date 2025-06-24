package feed_stats_gateway

import (
	"alt/driver/alt_db"
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

type SummarizedArticlesCountGateway struct {
	altDBRepository *alt_db.AltDBRepository
}

func NewSummarizedArticlesCountGateway(pool *pgxpool.Pool) *SummarizedArticlesCountGateway {
	return &SummarizedArticlesCountGateway{
		altDBRepository: alt_db.NewAltDBRepositoryWithPool(pool),
	}
}

func (g *SummarizedArticlesCountGateway) Execute(ctx context.Context) (int, error) {
	if g.altDBRepository == nil {
		return 0, errors.New("database connection not available")
	}
	return g.altDBRepository.FetchSummarizedArticlesCount(ctx)
}