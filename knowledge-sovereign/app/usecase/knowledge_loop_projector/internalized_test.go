package knowledge_loop_projector

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// TestRunBatch_InternalizedFlipsDismissState exercises the ADR-000914
// "I got this" producer end-to-end through RunBatch:
//
//  1. A pre-existing entry sits in the fakeRepo with dismiss_state=ACTIVE.
//  2. A knowledge_loop.internalized.v1 event lands for that user / entry.
//  3. The projector's projectInternalized branch must call
//     PatchKnowledgeLoopEntryDismissState with DISMISS_STATE_INTERNALIZED.
//  4. The patch must record exactly that flip and nothing else (the patch
//     driver preserves freshness / why / surface_bucket / continue_context
//     — covered by other tests; here we only assert the dispatch).
func TestRunBatch_InternalizedFlipsDismissStateToInternalized(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	tenantID := uuid.New()
	t0 := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)

	// Seed an existing entry the internalize event targets.
	preExisting := &sovereignv1.KnowledgeLoopEntry{
		UserId:       userID.String(),
		TenantId:     tenantID.String(),
		LensModeId:   "default",
		EntryKey:     "entry-grad-1",
		DismissState: sovereignv1.DismissState_DISMISS_STATE_ACTIVE,
	}

	payload, err := json.Marshal(map[string]any{
		"entry_key":    "entry-grad-1",
		"lens_mode_id": "default",
		"from_stage":   "LOOP_STAGE_DECIDE",
		"to_stage":     "LOOP_STAGE_DECIDE",
		"trigger":      "TRANSITION_TRIGGER_INTERNALIZE",
	})
	require.NoError(t, err)

	repo := &fakeRepo{
		entries: []*sovereignv1.KnowledgeLoopEntry{preExisting},
		events: []sovereign_db.KnowledgeEvent{
			{
				EventID:       uuid.New(),
				EventSeq:      77,
				OccurredAt:    t0,
				TenantID:      tenantID,
				UserID:        &userID,
				EventType:     EventKnowledgeLoopInternalized,
				AggregateType: "knowledge_loop_entry",
				AggregateID:   "entry-grad-1",
				Payload:       payload,
			},
		},
	}

	p := NewProjector(repo, slog.New(slog.DiscardHandler), Config{BatchSize: 10})
	require.NoError(t, p.RunBatch(context.Background()))

	require.Len(t, repo.dismissPatches, 1, "internalize event must trigger exactly one dismiss_state patch")
	got := repo.dismissPatches[0]
	require.Equal(t, "entry-grad-1", got.EntryKey)
	require.Equal(t, "default", got.LensModeID)
	require.Equal(t, int64(77), got.EventSeq)
	require.Equal(t, sovereignv1.DismissState_DISMISS_STATE_INTERNALIZED, got.DismissState,
		"projector must flip dismiss_state to INTERNALIZED on knowledge_loop.internalized.v1")
}

// TestRunBatch_InternalizedWithoutEntryKeyIsNoOp guards the malformed
// payload path: the classifier validator rejects empty entry_key, but if
// a stray event survives ingest the projector must not panic / fail the
// batch. Logging is the only side effect.
func TestRunBatch_InternalizedWithoutEntryKeyIsNoOp(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	tenantID := uuid.New()
	t0 := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)

	payload, err := json.Marshal(map[string]any{
		"lens_mode_id": "default",
		// entry_key intentionally omitted
	})
	require.NoError(t, err)

	repo := &fakeRepo{
		events: []sovereign_db.KnowledgeEvent{
			{
				EventID:       uuid.New(),
				EventSeq:      88,
				OccurredAt:    t0,
				TenantID:      tenantID,
				UserID:        &userID,
				EventType:     EventKnowledgeLoopInternalized,
				AggregateType: "knowledge_loop_entry",
				AggregateID:   "", // also empty
				Payload:       payload,
			},
		},
	}

	p := NewProjector(repo, slog.New(slog.DiscardHandler), Config{BatchSize: 10})
	require.NoError(t, p.RunBatch(context.Background()))
	require.Empty(t, repo.dismissPatches, "empty entry_key must skip without firing the patch")
}
