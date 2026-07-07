package repository

import (
	"context"
	"fmt"
	"rag-orchestrator/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type txKey struct{}

// InjectTx injects the transaction into the context
func InjectTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// ExtractTx extracts the transaction from the context
func ExtractTx(ctx context.Context) pgx.Tx {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return nil
}

// txBeginner is the subset of *pgxpool.Pool that RunInTx depends on. It
// exists so tests can substitute a fake transaction whose Commit fails,
// without standing up a real Postgres connection.
type txBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type postgresTransactionManager struct {
	pool txBeginner
}

// NewPostgresTransactionManager creates a new transaction manager.
func NewPostgresTransactionManager(pool *pgxpool.Pool) domain.TransactionManager {
	return &postgresTransactionManager{pool: pool}
}

// RunInTx begins a transaction, runs fn, and commits on success or rolls
// back on error/panic. The return value MUST be the named `err` so the
// defer's `err = tx.Commit(ctx)` assignment actually propagates to the
// caller — a bare `error` return type here would let a failed (rolled-back)
// Commit be silently reported as success.
func (tm *postgresTransactionManager) RunInTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	tx, err := tm.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		} else if err != nil {
			_ = tx.Rollback(ctx)
		} else {
			err = tx.Commit(ctx)
		}
	}()

	ctxWithTx := InjectTx(ctx, tx)
	err = fn(ctxWithTx)
	return err
}
