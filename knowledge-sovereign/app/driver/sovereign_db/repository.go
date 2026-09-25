package sovereign_db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgxIface is the subset of the pgx pool the Repository depends on. It lets unit
// tests substitute a fake pool (see mock_pgx_test.go) without a live database.
type PgxIface interface {
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	// Begin starts a transaction. Required by any repository method that
	// must make more than one INSERT/UPDATE atomic (e.g., dedupe-key +
	// event append, or deactivate + activate a projection version) so a
	// mid-sequence crash can never leave the two statements half-applied.
	Begin(ctx context.Context) (pgx.Tx, error)
}

var _ PgxIface = (*pgxpool.Pool)(nil)

// rowScanner abstracts scanning destination values from a single row.
type rowScanner interface {
	Scan(dest ...any) error
}

// Repository provides database operations for Knowledge Sovereign.
type Repository struct {
	pool PgxIface
}

// NewRepository creates a new sovereign DB repository.
func NewRepository(pool PgxIface) *Repository {
	return &Repository{pool: pool}
}

// ErrDismissTargetNotFound is returned when the dismiss target does not exist.
var ErrDismissTargetNotFound = fmt.Errorf("dismiss target not found")
