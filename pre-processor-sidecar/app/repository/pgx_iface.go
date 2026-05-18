// Package repository defines the PgxIface that lets repositories swap
// *pgxpool.Pool for a pgxmock.PgxPoolIface in tests.

package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgxIface is the subset of *pgxpool.Pool used by the PostgreSQL repositories.
// pgxmock.PgxPoolIface satisfies this interface, which is how the unit tests
// inject their fakes without touching production code.
type PgxIface interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
	Ping(ctx context.Context) error
	Close()
}

var _ PgxIface = (*pgxpool.Pool)(nil)
