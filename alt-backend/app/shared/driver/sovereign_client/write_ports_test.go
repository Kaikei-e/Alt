package sovereign_client

import (
	"alt/orchestrator/port/knowledge_home_port"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDismissKnowledgeHomeItem_MapsNotFoundToErrDismissTargetNotFound(t *testing.T) {
	userID := uuid.New()
	dismissedAt := time.Date(2026, 7, 7, 22, 30, 0, 0, time.UTC)

	t.Run("CodeNotFound wraps ErrDismissTargetNotFound and keeps Connect error in chain", func(t *testing.T) {
		handler := &mockSovereignHandler{
			returnErr: connect.NewError(connect.CodeNotFound, errors.New("dismiss target not found")),
		}
		client, cleanup := setupMockServer(handler)
		defer cleanup()

		err := client.DismissKnowledgeHomeItem(context.Background(), userID, "article:missing", 1, dismissedAt)
		require.Error(t, err)
		assert.True(t, errors.Is(err, knowledge_home_port.ErrDismissTargetNotFound))
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	})

	t.Run("CodeUnavailable does not match ErrDismissTargetNotFound", func(t *testing.T) {
		handler := &mockSovereignHandler{
			returnErr: connect.NewError(connect.CodeUnavailable, errors.New("upstream unavailable")),
		}
		client, cleanup := setupMockServer(handler)
		defer cleanup()

		err := client.DismissKnowledgeHomeItem(context.Background(), userID, "article:missing", 1, dismissedAt)
		require.Error(t, err)
		assert.False(t, errors.Is(err, knowledge_home_port.ErrDismissTargetNotFound))
		assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
	})
}

// TestSnoozeRecallCandidate_PayloadIncludesOccurredAtAndUntil pins the wire
// shape knowledge-sovereign's SnoozeRecallCandidate driver requires:
// recall_candidate_view is a reproject-safe projection, so the driver rejects
// snooze mutations whose payload lacks occurred_at (it would otherwise fall
// back to SQL now(), making replay non-deterministic). This also guards
// against the "until" field regressing back to the previously-mismatched
// "snoozed_until" key, which the driver never read.
func TestSnoozeRecallCandidate_PayloadIncludesOccurredAtAndUntil(t *testing.T) {
	handler := &mockSovereignHandler{}
	client, cleanup := setupMockServer(handler)
	defer cleanup()

	userID := uuid.New()
	until := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	occurredAt := time.Date(2026, 7, 7, 22, 30, 0, 0, time.UTC)

	err := client.SnoozeRecallCandidate(context.Background(), userID, "item-1", until, occurredAt)
	require.NoError(t, err)

	var got struct {
		UserID     string `json:"user_id"`
		ItemKey    string `json:"item_key"`
		Until      string `json:"until"`
		OccurredAt string `json:"occurred_at"`
	}
	require.NoError(t, json.Unmarshal(handler.lastPayload, &got))

	assert.Equal(t, userID.String(), got.UserID)
	assert.Equal(t, "item-1", got.ItemKey)
	assert.Equal(t, until.Format(time.RFC3339Nano), got.Until)
	require.NotEmpty(t, got.OccurredAt, "occurred_at must be populated: knowledge-sovereign rejects an empty occurred_at")
	parsed, err := time.Parse(time.RFC3339Nano, got.OccurredAt)
	require.NoError(t, err)
	assert.True(t, parsed.Equal(occurredAt))
}

// TestDismissRecallCandidate_PayloadIncludesOccurredAt pins the wire shape
// knowledge-sovereign's DismissRecallCandidate driver requires, for the same
// reproject-determinism reason as SnoozeRecallCandidate above.
func TestDismissRecallCandidate_PayloadIncludesOccurredAt(t *testing.T) {
	handler := &mockSovereignHandler{}
	client, cleanup := setupMockServer(handler)
	defer cleanup()

	userID := uuid.New()
	occurredAt := time.Date(2026, 7, 7, 22, 30, 0, 0, time.UTC)

	err := client.DismissRecallCandidate(context.Background(), userID, "item-2", occurredAt)
	require.NoError(t, err)

	var got struct {
		UserID     string `json:"user_id"`
		ItemKey    string `json:"item_key"`
		OccurredAt string `json:"occurred_at"`
	}
	require.NoError(t, json.Unmarshal(handler.lastPayload, &got))

	assert.Equal(t, userID.String(), got.UserID)
	assert.Equal(t, "item-2", got.ItemKey)
	require.NotEmpty(t, got.OccurredAt, "occurred_at must be populated: knowledge-sovereign rejects an empty occurred_at")
	parsed, err := time.Parse(time.RFC3339Nano, got.OccurredAt)
	require.NoError(t, err)
	assert.True(t, parsed.Equal(occurredAt))
}
