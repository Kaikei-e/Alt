package sovereign_db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// KnowledgeLoopReprojectResult reports what the reproject transaction did so
// the operator can verify post-conditions (the runbook's "Post-check" section).
// Counts are pre-truncate; the projector picks up from event_seq=0 on its next
// scheduler tick.
type KnowledgeLoopReprojectResult struct {
	EntriesTruncated  int64 `json:"entries_truncated"`
	SessionTruncated  int64 `json:"session_state_truncated"`
	SurfacesTruncated int64 `json:"surfaces_truncated"`
	CheckpointReset   bool  `json:"checkpoint_reset"`
}

// TruncateKnowledgeLoopProjections runs the disposable-projection reproject
// procedure for the Knowledge Loop read model: TRUNCATE the three projection
// tables and reset the projector checkpoint to zero, all in one transaction
// so a partial failure leaves the projector either fully reset or fully
// untouched.
//
// The dedupe table (`knowledge_loop_transition_dedupes`) is intentionally NOT
// truncated. It is the ingest-side idempotency barrier (canonical contract
// §3 invariant 8); wiping it would open a window for duplicate event append
// on client retry. The runbook documents this explicitly.
//
// Reproject is destructive but idempotent: a second call after a successful
// run is a no-op (TRUNCATE on empty tables is fine, checkpoint reset to 0
// when already 0 is fine).
func (r *Repository) TruncateKnowledgeLoopProjections(
	ctx context.Context,
) (KnowledgeLoopReprojectResult, error) {
	out := KnowledgeLoopReprojectResult{}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadWrite})
	if err != nil {
		return out, fmt.Errorf("TruncateKnowledgeLoopProjections: BeginTx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Pre-truncate counts give the operator a number to compare against the
	// "row count within 1-5% of pre-snapshot" post-check in the runbook.
	for _, t := range []struct {
		name string
		dst  *int64
	}{
		{"knowledge_loop_entries", &out.EntriesTruncated},
		{"knowledge_loop_session_state", &out.SessionTruncated},
		{"knowledge_loop_surfaces", &out.SurfacesTruncated},
	} {
		row := tx.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", t.name))
		if err := row.Scan(t.dst); err != nil {
			return out, fmt.Errorf("count %s: %w", t.name, err)
		}
	}

	// TRUNCATE the three projection tables. We do NOT touch
	// `knowledge_loop_transition_dedupes` (canonical contract §3 invariant 8 —
	// dedupe is ingest-side, not a projection).
	if _, err := tx.Exec(ctx, `TRUNCATE knowledge_loop_entries, knowledge_loop_session_state, knowledge_loop_surfaces`); err != nil {
		return out, fmt.Errorf("TRUNCATE projections: %w", err)
	}

	// Reset the projector checkpoint so the scheduler replays from event_seq=0
	// on its next tick. UPDATE matches the runbook step 2 exactly.
	tag, err := tx.Exec(ctx,
		`UPDATE knowledge_projection_checkpoints
		 SET last_event_seq = 0, updated_at = NOW()
		 WHERE projector_name = 'knowledge-loop-projector'`)
	if err != nil {
		return out, fmt.Errorf("reset checkpoint: %w", err)
	}
	out.CheckpointReset = tag.RowsAffected() > 0

	if err := tx.Commit(ctx); err != nil {
		return out, fmt.Errorf("commit: %w", err)
	}
	return out, nil
}
