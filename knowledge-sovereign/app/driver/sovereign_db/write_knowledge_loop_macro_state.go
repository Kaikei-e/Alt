package sovereign_db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// KnowledgeLoopMacroStateRow is the driver-level row shape for
// knowledge_loop_macro_state. It mirrors the proto KnowledgeLoopMacroState
// 1-to-1 but in DB-friendly types so the usecase layer never has to deal
// with pgx types directly.
//
// CognitiveLoadHint is the enum string serialisation
// ("unspecified" | "light" | "medium" | "heavy") matching the
// knowledge_loop_cognitive_load_hint Postgres enum created in
// migration 00020. The macro_state_builder publishes the same
// strings via its CognitiveLoadHint typed alias so callers can pass
// the value through unchanged.
type KnowledgeLoopMacroStateRow struct {
	UserID                  uuid.UUID
	TenantID                uuid.UUID
	LensModeID              string
	ActiveContinueThreads   uint32
	PendingReviewCount      uint32
	RecentInternalizedCount uint32
	CognitiveLoadHint       string
	WindowStartAt           time.Time
	WindowEndAt             time.Time
	SeqHiwater              int64
	LensWeightsVersion      int32
}

const upsertKnowledgeLoopMacroStateQuery = `
INSERT INTO knowledge_loop_macro_state (
  user_id, tenant_id, lens_mode_id,
  active_continue_threads, pending_review_count, recent_internalized_count,
  cognitive_load_hint,
  window_start_at, window_end_at,
  seq_hiwater, lens_weights_version
) VALUES (
  $1, $2, $3,
  $4, $5, $6,
  $7::knowledge_loop_cognitive_load_hint,
  $8, $9,
  $10, $11
)
ON CONFLICT (user_id, tenant_id, lens_mode_id) DO UPDATE SET
  active_continue_threads   = EXCLUDED.active_continue_threads,
  pending_review_count      = EXCLUDED.pending_review_count,
  recent_internalized_count = EXCLUDED.recent_internalized_count,
  cognitive_load_hint       = EXCLUDED.cognitive_load_hint,
  window_start_at           = EXCLUDED.window_start_at,
  window_end_at             = EXCLUDED.window_end_at,
  seq_hiwater               = EXCLUDED.seq_hiwater,
  lens_weights_version      = EXCLUDED.lens_weights_version,
  projected_at              = NOW()
WHERE knowledge_loop_macro_state.seq_hiwater <= EXCLUDED.seq_hiwater
RETURNING seq_hiwater
`

// UpsertKnowledgeLoopMacroState writes the macro projection row for a
// (user, tenant, lens) tuple. Merge-safe: the seq_hiwater guard in the
// WHERE clause makes out-of-order replays no-ops. Returns
// SkippedBySeqHiwater=true when the row is unchanged.
//
// Reproject-safety invariants enforced at this boundary:
//   - The row is the full snapshot — no incremental delta arithmetic in SQL.
//   - cognitive_load_hint is cast to the Postgres enum so a typo lands as a
//     query-time error rather than silent data corruption.
//   - The window timestamps are accepted verbatim from the caller so the
//     usecase layer's event-time purity discipline is not undone here.
func (r *Repository) UpsertKnowledgeLoopMacroState(
	ctx context.Context,
	row KnowledgeLoopMacroStateRow,
) (*KnowledgeLoopUpsertResult, error) {
	if row.LensModeID == "" {
		return nil, errors.New("sovereign_db: UpsertKnowledgeLoopMacroState: empty lens_mode_id")
	}
	if row.CognitiveLoadHint == "" {
		// The enum has an explicit 'unspecified' value; default rather
		// than panicking so the upsert remains additive even when the
		// builder publishes an empty hint string.
		row.CognitiveLoadHint = "unspecified"
	}

	var seqHiwater int64
	scanRow := r.pool.QueryRow(ctx, upsertKnowledgeLoopMacroStateQuery,
		row.UserID, row.TenantID, row.LensModeID,
		row.ActiveContinueThreads, row.PendingReviewCount, row.RecentInternalizedCount,
		row.CognitiveLoadHint,
		row.WindowStartAt, row.WindowEndAt,
		row.SeqHiwater, row.LensWeightsVersion,
	)
	if err := scanRow.Scan(&seqHiwater); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &KnowledgeLoopUpsertResult{SkippedBySeqHiwater: true}, nil
		}
		return nil, fmt.Errorf("upsert knowledge_loop_macro_state: %w", err)
	}
	return &KnowledgeLoopUpsertResult{
		Applied:              true,
		ProjectionSeqHiwater: seqHiwater,
	}, nil
}

const getKnowledgeLoopMacroStateQuery = `
SELECT
  user_id, tenant_id, lens_mode_id,
  active_continue_threads, pending_review_count, recent_internalized_count,
  cognitive_load_hint::text,
  window_start_at, window_end_at,
  seq_hiwater, lens_weights_version
FROM knowledge_loop_macro_state
WHERE user_id = $1 AND tenant_id = $2 AND lens_mode_id = $3
`

// GetKnowledgeLoopMacroState returns the macro projection row for a
// (user, tenant, lens) tuple. Returns (nil, nil) when no row exists yet
// — callers treat the absence as "macro byline hidden".
func (r *Repository) GetKnowledgeLoopMacroState(
	ctx context.Context,
	userID, tenantID uuid.UUID,
	lensModeID string,
) (*KnowledgeLoopMacroStateRow, error) {
	out := &KnowledgeLoopMacroStateRow{}
	err := r.pool.QueryRow(ctx, getKnowledgeLoopMacroStateQuery,
		userID, tenantID, lensModeID,
	).Scan(
		&out.UserID, &out.TenantID, &out.LensModeID,
		&out.ActiveContinueThreads, &out.PendingReviewCount, &out.RecentInternalizedCount,
		&out.CognitiveLoadHint,
		&out.WindowStartAt, &out.WindowEndAt,
		&out.SeqHiwater, &out.LensWeightsVersion,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("select knowledge_loop_macro_state: %w", err)
	}
	return out, nil
}
