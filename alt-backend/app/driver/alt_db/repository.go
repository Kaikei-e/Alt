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
	if pool == nil {
		return nil
	}
	return &AltDBRepository{pool: pool}
}

// NewAltDBRepositoryWithPool creates a repository with a concrete pgxpool.Pool
// Returns nil if pool is nil, which should be handled by the caller
func NewAltDBRepositoryWithPool(pool *pgxpool.Pool) *AltDBRepository {
	if pool == nil {
		return nil
	}
	return &AltDBRepository{pool: pool}
}
