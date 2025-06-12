package alt_db

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

type AltDBRepository struct {
	pool *pgxpool.Pool
}

func NewAltDBRepository(pool *pgxpool.Pool) *AltDBRepository {
	return &AltDBRepository{pool: pool}
}
