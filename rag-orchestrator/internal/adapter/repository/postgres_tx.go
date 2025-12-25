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

type postgresTransactionManager struct {
	pool *pgxpool.Pool
}

// NewPostgresTransactionManager creates a new transaction manager.
func NewPostgresTransactionManager(pool *pgxpool.Pool) domain.TransactionManager {
	return &postgresTransactionManager{pool: pool}
}

func (tm *postgresTransactionManager) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := tm.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback(ctx)
			panic(p)
		} else if err != nil {
			tx.Rollback(ctx)
		} else {
			err = tx.Commit(ctx)
		}
	}()

	ctxWithTx := InjectTx(ctx, tx)
	err = fn(ctxWithTx)
	return err
}
