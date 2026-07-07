package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

// fakeTx is a minimal pgx.Tx double. Only Commit/Rollback are exercised by
// RunInTx; every other method panics if called so an unexpected code path
// change surfaces loudly instead of returning a misleading zero value.
type fakeTx struct {
	pgx.Tx
	commitErr   error
	commitCalls int
	rollbackErr error
	rbCalls     int
}

func (f *fakeTx) Commit(_ context.Context) error {
	f.commitCalls++
	return f.commitErr
}

func (f *fakeTx) Rollback(_ context.Context) error {
	f.rbCalls++
	return f.rollbackErr
}

type fakeBeginner struct {
	tx        *fakeTx
	beginErr  error
	beginCall int
}

func (f *fakeBeginner) Begin(_ context.Context) (pgx.Tx, error) {
	f.beginCall++
	if f.beginErr != nil {
		return nil, f.beginErr
	}
	return f.tx, nil
}

// TestRunInTx_CommitFailureSurfacesAsError is the RED case for the bug:
// before the named-return fix, RunInTx's unnamed `error` return meant the
// defer's `err = tx.Commit(ctx)` never reached the caller, so a failed
// (rolled-back) commit was reported as success.
func TestRunInTx_CommitFailureSurfacesAsError(t *testing.T) {
	commitErr := errors.New("commit failed: connection reset")
	tx := &fakeTx{commitErr: commitErr}
	beginner := &fakeBeginner{tx: tx}
	tm := &postgresTransactionManager{pool: beginner}

	err := tm.RunInTx(context.Background(), func(ctx context.Context) error {
		return nil // business fn succeeds; only the commit fails
	})

	if err == nil {
		t.Fatal("RunInTx must return an error when Commit fails, got nil")
	}
	if !errors.Is(err, commitErr) {
		t.Fatalf("RunInTx error = %v, want it to wrap %v", err, commitErr)
	}
	if tx.commitCalls != 1 {
		t.Fatalf("Commit call count = %d, want 1", tx.commitCalls)
	}
}

func TestRunInTx_CommitsOnSuccess(t *testing.T) {
	tx := &fakeTx{}
	beginner := &fakeBeginner{tx: tx}
	tm := &postgresTransactionManager{pool: beginner}

	err := tm.RunInTx(context.Background(), func(ctx context.Context) error {
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tx.commitCalls != 1 {
		t.Fatalf("Commit call count = %d, want 1", tx.commitCalls)
	}
	if tx.rbCalls != 0 {
		t.Fatalf("Rollback call count = %d, want 0", tx.rbCalls)
	}
}

func TestRunInTx_FnErrorRollsBackAndReturnsError(t *testing.T) {
	fnErr := errors.New("business logic failed")
	tx := &fakeTx{}
	beginner := &fakeBeginner{tx: tx}
	tm := &postgresTransactionManager{pool: beginner}

	err := tm.RunInTx(context.Background(), func(ctx context.Context) error {
		return fnErr
	})

	if !errors.Is(err, fnErr) {
		t.Fatalf("RunInTx error = %v, want %v", err, fnErr)
	}
	if tx.commitCalls != 0 {
		t.Fatalf("Commit call count = %d, want 0 (must not commit after fn error)", tx.commitCalls)
	}
	if tx.rbCalls != 1 {
		t.Fatalf("Rollback call count = %d, want 1", tx.rbCalls)
	}
}

func TestRunInTx_BeginFailurePropagates(t *testing.T) {
	beginErr := errors.New("pool exhausted")
	beginner := &fakeBeginner{beginErr: beginErr}
	tm := &postgresTransactionManager{pool: beginner}

	err := tm.RunInTx(context.Background(), func(ctx context.Context) error {
		t.Fatal("fn must not run when Begin fails")
		return nil
	})

	if !errors.Is(err, beginErr) {
		t.Fatalf("RunInTx error = %v, want it to wrap %v", err, beginErr)
	}
}

func TestRunInTx_PanicRollsBackAndRepanics(t *testing.T) {
	tx := &fakeTx{}
	beginner := &fakeBeginner{tx: tx}
	tm := &postgresTransactionManager{pool: beginner}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected RunInTx to repanic")
		}
		if tx.rbCalls != 1 {
			t.Fatalf("Rollback call count = %d, want 1", tx.rbCalls)
		}
		if tx.commitCalls != 0 {
			t.Fatalf("Commit call count = %d, want 0", tx.commitCalls)
		}
	}()

	_ = tm.RunInTx(context.Background(), func(ctx context.Context) error {
		panic("boom")
	})
}
