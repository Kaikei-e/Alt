package sovereign_db

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// withUserContext runs `fn` inside a short-lived transaction that has
// `alt.user_id` bound via `SELECT set_config(name, value, true)`. The
// `is_local = true` parameter scopes the value to the active transaction,
// which:
//
//  1. Plays nicely with pgbouncer transaction-pooling (a SET would leak
//     across pooled sessions; SET LOCAL clears at COMMIT/ROLLBACK).
//  2. Lets a future RLS policy (knowledge-sovereign migration 00014)
//     read the value via `current_setting('alt.user_id', true)` and
//     fail-closed when the caller forgot to bind the user.
//
// The transaction is opened ReadOnly so the read methods cannot
// accidentally take row locks. Wave 4-D Phase 1 wires this on the read
// path; Phase 2 adds the matching policy. Bringing them in opposite
// order would render `knowledge_loop_entries` unreadable in production.
//
// Reproject-safety: this helper does not read or write any business
// state. It is purely an isolation primitive.
func (r *Repository) withUserContext(
	ctx context.Context,
	userID uuid.UUID,
	fn func(pgx.Tx) error,
) error {
	if userID == uuid.Nil {
		// Refuse to bind the zero uuid. Treat this as a programmer error —
		// every Knowledge Loop read enters here only after a higher layer
		// authenticated the caller. Letting Nil through would mean
		// "no user scope" which, post-Phase-2, would silently match every
		// row whose user_id is also zero (none, by schema CHECK) and
		// behave as a stealth bypass.
		return fmt.Errorf("withUserContext: refusing to bind nil user_id")
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("withUserContext: BeginTx: %w", err)
	}
	defer func() {
		// Rollback is a no-op after a successful Commit. Logging the
		// error is intentional but non-fatal — the read result has
		// already been observed by the caller at this point.
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, "SELECT set_config('alt.user_id', $1, true)", userID.String()); err != nil {
		return fmt.Errorf("withUserContext: set_config alt.user_id: %w", err)
	}
	slog.DebugContext(ctx, "knowledge_loop: user-scoped read",
		slog.String("user_id", userID.String()),
		slog.Bool("alt_user_id_set", true),
	)

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("withUserContext: commit: %w", err)
	}
	return nil
}
