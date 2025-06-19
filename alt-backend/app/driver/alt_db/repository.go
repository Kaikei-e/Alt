package alt_db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgxIface defines the interface for pgx operations that we use
type PgxIface interface {
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
	Close()
}

// Ensure pgxpool.Pool implements PgxIface
var _ PgxIface = (*pgxpool.Pool)(nil)

type AltDBRepository struct {
	pool PgxIface
}

func NewAltDBRepository(pool PgxIface) *AltDBRepository {
	return &AltDBRepository{pool: pool}
}

// NewAltDBRepositoryWithPool creates a repository with a concrete pgxpool.Pool
func NewAltDBRepositoryWithPool(pool *pgxpool.Pool) *AltDBRepository {
	return &AltDBRepository{pool: pool}
}
