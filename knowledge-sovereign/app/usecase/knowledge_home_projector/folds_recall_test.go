package knowledge_home_projector

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

// ── RecallSnoozed / RecallDismissed ──
//
// alt-backend's recall_snooze_usecase/recall_dismiss_usecase append these
// events after already writing recall_candidate_view directly (write-through).
// A full TRUNCATE + reproject replay must reach the same snoozed_until /
// dismissed_at state, so the projector must fold these two event types too —
// they must not fall into the "unknown event types are silently skipped"
// default case.

func TestProjector_FoldsRecallSnoozed(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	itemKey := "article:" + uuid.New().String()
	occurredAt := time.Date(2026, 7, 14, 19, 0, 0, 0, time.UTC)
	until := occurredAt.Add(24 * time.Hour)

	payload := mustJSON(t, map[string]any{
		"item_key":      itemKey,
		"snooze_hours":  24,
		"snoozed_until": until.Format(time.RFC3339),
	})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "RecallSnoozed", itemKey, occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	snooze, ok := repo.snoozed[itemKey]
	require.True(t, ok, "RecallSnoozed must reach SnoozeRecallCandidate on reproject, not be silently skipped")
	assert.Equal(t, user.String(), snooze.UserID)
	gotUntil, err := time.Parse(time.RFC3339Nano, snooze.Until)
	require.NoError(t, err)
	assert.True(t, until.Equal(gotUntil))
	gotOccurredAt, err := time.Parse(time.RFC3339Nano, snooze.OccurredAt)
	require.NoError(t, err)
	assert.True(t, occurredAt.Equal(gotOccurredAt), "occurred_at must derive from event.OccurredAt, not wall clock")
	assert.Equal(t, int64(1), repo.checkpoint)
}

func TestProjector_FoldsRecallDismissed(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	itemKey := "article:" + uuid.New().String()
	occurredAt := time.Date(2026, 7, 14, 19, 30, 0, 0, time.UTC)

	payload := mustJSON(t, map[string]any{
		"item_key": itemKey,
	})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "RecallDismissed", itemKey, occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	dismiss, ok := repo.recallDismissed[itemKey]
	require.True(t, ok, "RecallDismissed must reach DismissRecallCandidate on reproject, not be silently skipped")
	assert.Equal(t, user.String(), dismiss.UserID)
	gotOccurredAt, err := time.Parse(time.RFC3339Nano, dismiss.OccurredAt)
	require.NoError(t, err)
	assert.True(t, occurredAt.Equal(gotOccurredAt), "occurred_at must derive from event.OccurredAt, not wall clock")
	assert.Equal(t, int64(1), repo.checkpoint)
}

func TestProjector_RecallSnoozed_RepositoryFailureStopsBatch(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	itemKey := "article:" + uuid.New().String()
	occurredAt := time.Date(2026, 7, 14, 19, 0, 0, 0, time.UTC)

	payload := mustJSON(t, map[string]any{
		"item_key":      itemKey,
		"snooze_hours":  24,
		"snoozed_until": occurredAt.Add(24 * time.Hour).Format(time.RFC3339),
	})
	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "RecallSnoozed", itemKey, occurredAt, tenant, user, payload),
	}
	repo := newFakeRepo(events)
	repo.snoozeRecallErr = fmt.Errorf("recall_candidate_view unavailable")
	p := NewProjector(repo, nil, Config{})

	err := p.RunBatch(context.Background())
	require.Error(t, err, "unlike the digest/recall-candidate side effects on other events, the snooze write IS the event's entire purpose — a failure must not advance past it")
	assert.Equal(t, int64(0), repo.checkpoint, "checkpoint must not advance past a failed RecallSnoozed fold")
}

func TestBuildRecallCandidateSnoozeAndDismiss(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	occurredAt := time.Date(2026, 8, 1, 19, 0, 0, 0, time.UTC)

	t.Run("buildSnoozeRecallCandidateWrite valid", func(t *testing.T) {
		evt := sovereign_db.KnowledgeEvent{
			TenantID:   tenantID,
			UserID:     &userID,
			OccurredAt: occurredAt,
			Payload: mustJSON(t, map[string]any{
				"item_key":      "article:123",
				"snoozed_until": "2026-08-02T19:00:00Z",
			}),
		}
		w, err := buildSnoozeRecallCandidateWrite(evt)
		require.NoError(t, err)
		assert.Equal(t, "article:123", w.ItemKey)
		assert.Equal(t, "2026-08-02T19:00:00Z", w.Until)
		assert.Equal(t, userID.String(), w.UserID)
	})

	t.Run("buildSnoozeRecallCandidateWrite missing item_key", func(t *testing.T) {
		evt := sovereign_db.KnowledgeEvent{
			TenantID: tenantID,
			Payload:  mustJSON(t, map[string]any{"item_key": ""}),
		}
		_, err := buildSnoozeRecallCandidateWrite(evt)
		require.Error(t, err)
	})

	t.Run("buildDismissRecallCandidateWrite valid", func(t *testing.T) {
		evt := sovereign_db.KnowledgeEvent{
			TenantID:   tenantID,
			UserID:     &userID,
			OccurredAt: occurredAt,
			Payload:    mustJSON(t, map[string]any{"item_key": "article:456"}),
		}
		w, err := buildDismissRecallCandidateWrite(evt)
		require.NoError(t, err)
		assert.Equal(t, "article:456", w.ItemKey)
		assert.Equal(t, userID.String(), w.UserID)
	})

	t.Run("buildDismissRecallCandidateWrite missing item_key", func(t *testing.T) {
		evt := sovereign_db.KnowledgeEvent{
			TenantID: tenantID,
			Payload:  mustJSON(t, map[string]any{"item_key": ""}),
		}
		_, err := buildDismissRecallCandidateWrite(evt)
		require.Error(t, err)
	})
}
