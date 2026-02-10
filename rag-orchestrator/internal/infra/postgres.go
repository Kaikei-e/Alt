package infra

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxvector "github.com/pgvector/pgvector-go/pgx"
)

// PoolConfig holds tunable parameters for the PostgreSQL connection pool.
type PoolConfig struct {
	MaxConns int
	MinConns int
}

// NewPostgresDB creates a new PostgreSQL connection pool.
func NewPostgresDB(ctx context.Context, dsn string, opts ...PoolConfig) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Apply pool config if provided, otherwise use defaults
	if len(opts) > 0 && opts[0].MaxConns > 0 {
		config.MaxConns = int32(opts[0].MaxConns)
	} else {
		config.MaxConns = 10
	}
	if len(opts) > 0 && opts[0].MinConns > 0 {
		config.MinConns = int32(opts[0].MinConns)
	} else {
		config.MinConns = 2
	}

	config.MaxConnLifetime = 1 * time.Hour
	config.MaxConnIdleTime = 30 * time.Minute

	// Register pgvector types
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgxvector.RegisterTypes(ctx, conn)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping db: %w", err)
	}

	return pool, nil
}
