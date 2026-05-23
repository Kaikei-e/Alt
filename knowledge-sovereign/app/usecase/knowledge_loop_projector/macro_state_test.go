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
)

// macroEvent assembles a minimal knowledge_loop event payload for the macro
// test. It keeps the surface area small so the wiring test stays focused on
// the projector → macro_state_builder → driver path rather than the
// peripheral fields the projector also writes (session, surfaces).
func macroEvent(t *testing.T, evType, entryKey string, payload map[string]any, occurredAt time.Time, seq int64, userID, tenantID uuid.UUID) sovereign_db.KnowledgeEvent {
	t.Helper()
	if payload == nil {
		payload = map[string]any{}
	}
	payload["entry_key"] = entryKey
	payload["lens_mode_id"] = "default"
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	return sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      seq,
		OccurredAt:    occurredAt,
		TenantID:      tenantID,
		UserID:        &userID,
		EventType:     evType,
		AggregateType: "knowledge_loop_entry",
		AggregateID:   entryKey,
		Payload:       body,
	}
}

// TestRunBatch_PopulatesMacroStateAfterActedAndReviewed exercises the
// outside-in wiring: the projector consumes Acted + Reviewed +
// ActOutcome(internalized) events for one user and writes a single
// macro_state row reflecting the reduced 7d window. This is the
// integration-level companion of macro_state_builder_test.go (pure-fn
// unit suite in usecase/knowledge_loop_session_state).
func TestRunBatch_PopulatesMacroStateAfterActedReviewedAndInternalized(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	tenantID := uuid.New()
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	t1 := now.Add(-4 * 24 * time.Hour)
	t2 := now.Add(-3 * 24 * time.Hour)
	t3 := now.Add(-2 * 24 * time.Hour)
	t4 := now.Add(-1 * 24 * time.Hour)

	repo := &fakeRepo{
		events: []sovereign_db.KnowledgeEvent{
			macroEvent(t, EventKnowledgeLoopActed, "entry-a",
				map[string]any{"continue_flag": true, "acted_intent": "DECISION_INTENT_OPEN"},
				t1, 10, userID, tenantID),
			macroEvent(t, EventKnowledgeLoopActed, "entry-b",
				map[string]any{"continue_flag": true, "acted_intent": "DECISION_INTENT_OPEN"},
				t2, 20, userID, tenantID),
			macroEvent(t, EventKnowledgeLoopReviewed, "entry-c",
				map[string]any{"trigger": "TRANSITION_TRIGGER_RECHECK"},
				t3, 30, userID, tenantID),
			macroEvent(t, EventKnowledgeLoopActOutcome, "entry-b",
				map[string]any{"outcome": "internalized"},
				t4, 40, userID, tenantID),
		},
	}

	p := NewProjector(repo, slog.New(slog.DiscardHandler), Config{BatchSize: 100})

	require.NoError(t, p.RunBatch(context.Background()))

	// All four events advance macro_state under the consumer-current
	// occurred_at window-end, so we expect the final row to reflect the
	// last consumed event (seq=40).
	require.NotEmpty(t, repo.macroStates, "projector must write at least one macro_state row")

	latest := repo.macroStates[len(repo.macroStates)-1]
	require.Equal(t, userID, latest.UserID)
	require.Equal(t, tenantID, latest.TenantID)
	require.Equal(t, "default", latest.LensModeID)
	require.Equal(t, int64(40), latest.SeqHiwater, "windowEnd = consumed event occurred_at; seq_hiwater = max seq in window")
	require.Equal(t, uint32(1), latest.ActiveContinueThreads, "entry-a still continuing; entry-b graduated")
	require.Equal(t, uint32(1), latest.PendingReviewCount, "entry-c recheck still pending")
	require.Equal(t, uint32(1), latest.RecentInternalizedCount, "entry-b internalized in window")
	require.Equal(t, "light", latest.CognitiveLoadHint, "load=2 stays below medium threshold of 3 (default lens)")
	require.Equal(t, t4.UTC(), latest.WindowEndAt, "windowEnd = consumed event occurred_at (event-time purity)")
	require.Equal(t, t4.UTC().Add(-7*24*time.Hour), latest.WindowStartAt)
}

// TestRunBatch_RecomputeIsBoundedToMacroEventTypes confirms that non-Loop
// events (e.g. SummaryVersionCreated) do NOT trigger macro recompute. The
// macro layer should only churn on Acted / Reviewed / ActOutcome — write
// amplification matters at scale.
func TestRunBatch_DoesNotRecomputeMacroOnUnrelatedEvents(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	tenantID := uuid.New()
	t0 := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC).Add(-1 * time.Hour)

	repo := &fakeRepo{
		events: []sovereign_db.KnowledgeEvent{
			macroEvent(t, EventSummaryVersionCreated, "entry-x",
				map[string]any{"summary_version_id": uuid.New().String(), "article_title": "x"},
				t0, 50, userID, tenantID),
			macroEvent(t, EventHomeItemsSeen, "entry-x",
				nil, t0.Add(time.Minute), 51, userID, tenantID),
		},
	}

	p := NewProjector(repo, slog.New(slog.DiscardHandler), Config{BatchSize: 100})
	require.NoError(t, p.RunBatch(context.Background()))

	require.Empty(t, repo.macroStates, "non-Loop events must not trigger macro recompute")
}
