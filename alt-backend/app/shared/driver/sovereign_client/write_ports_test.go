package sovereign_client

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
