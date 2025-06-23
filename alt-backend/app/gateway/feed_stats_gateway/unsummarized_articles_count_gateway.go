package feed_stats_gateway

import (
	"alt/driver/alt_db"
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

type UnsummarizedArticlesCountGateway struct {
	altDBRepository *alt_db.AltDBRepository
}

func NewUnsummarizedArticlesCountGateway(pool *pgxpool.Pool) *UnsummarizedArticlesCountGateway {
	return &UnsummarizedArticlesCountGateway{
		altDBRepository: alt_db.NewAltDBRepositoryWithPool(pool),
	}
}

func (g *UnsummarizedArticlesCountGateway) Execute(ctx context.Context) (int, error) {
	if g.altDBRepository == nil {
		return 0, errors.New("database connection not available")
	}
	return g.altDBRepository.FetchUnsummarizedArticlesCount(ctx)
}