package knowledge_loop_projector

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

func actedEvent(t *testing.T, intent string, occurredAt time.Time, seq int64) sovereign_db.KnowledgeEvent {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"entry_key":    "loop-entry-fixture",
		"acted_intent": intent,
		"continue_flag": true,
	})
	require.NoError(t, err)
	uid := uuid.New()
	return sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      seq,
		OccurredAt:    occurredAt,
		TenantID:      uuid.New(),
		UserID:        &uid,
		EventType:     EventKnowledgeLoopActed,
		AggregateType: "loop_session",
		AggregateID:   "loop-entry-fixture",
		Payload:       payload,
	}
}

func TestBuildContinueContextFromActed_DescendingSeqYieldsLatestFirst(t *testing.T) {
	t1 := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	t2 := t1.Add(1 * time.Minute)
	t3 := t2.Add(1 * time.Minute)

	// Caller passes events newest-first per
	// ListKnowledgeLoopActedEventsForEntry's contract.
	recent := []sovereign_db.KnowledgeEvent{
		actedEvent(t, "DECISION_INTENT_REVISIT", t3, 30),
		actedEvent(t, "DECISION_INTENT_ASK", t2, 20),
		actedEvent(t, "DECISION_INTENT_OPEN", t1, 10),
	}

	body := buildContinueContextFromActed(recent)
	require.NotNil(t, body)

	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))
	labels, _ := got["recent_action_labels"].([]any)
	require.Equal(t, []any{"revisited", "asked", "opened"}, labels)
	require.Equal(t, "Revisited recently; ready to continue.", got["summary"])
	require.Equal(t, t3.UTC().Format(time.RFC3339), got["last_interacted_at"])
}

func TestBuildContinueContextFromActed_BoundedToFiveDistinctLabels(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	intents := []string{
		"DECISION_INTENT_REVISIT",
		"DECISION_INTENT_ASK",
		"DECISION_INTENT_OPEN",
		"DECISION_INTENT_SAVE",
		"DECISION_INTENT_COMPARE",
		"DECISION_INTENT_SNOOZE", // 6th — must be dropped by bound
	}
	recent := make([]sovereign_db.KnowledgeEvent, 0, len(intents))
	for i, it := range intents {
		recent = append(recent, actedEvent(t, it, now.Add(time.Duration(-i)*time.Minute), int64(100-i)))
	}
	body := buildContinueContextFromActed(recent)
	require.NotNil(t, body)
	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))
	labels, _ := got["recent_action_labels"].([]any)
	require.Len(t, labels, recentActionLabelsBound)
}

func TestBuildContinueContextFromActed_DedupesIdenticalIntents(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	recent := []sovereign_db.KnowledgeEvent{
		actedEvent(t, "DECISION_INTENT_OPEN", now, 30),
		actedEvent(t, "DECISION_INTENT_OPEN", now.Add(-1*time.Minute), 20),
		actedEvent(t, "DECISION_INTENT_ASK", now.Add(-2*time.Minute), 10),
	}
	body := buildContinueContextFromActed(recent)
	require.NotNil(t, body)
	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))
	labels, _ := got["recent_action_labels"].([]any)
	require.Equal(t, []any{"opened", "asked"}, labels)
}

func TestBuildContinueContextFromActed_EmptyOrUnknownReturnsNil(t *testing.T) {
	require.Nil(t, buildContinueContextFromActed(nil))

	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	recent := []sovereign_db.KnowledgeEvent{
		actedEvent(t, "DECISION_INTENT_UNSPECIFIED", now, 1),
	}
	require.Nil(t, buildContinueContextFromActed(recent))
}

func TestSemanticActionLabel_HandlesShortAndProtoForms(t *testing.T) {
	cases := map[string]string{
		"open":                    "opened",
		"DECISION_INTENT_OPEN":    "opened",
		"ask":                     "asked",
		"DECISION_INTENT_ASK":     "asked",
		"save":                    "saved",
		"DECISION_INTENT_SAVE":    "saved",
		"compare":                 "compared",
		"DECISION_INTENT_COMPARE": "compared",
		"revisit":                 "revisited",
		"DECISION_INTENT_REVISIT": "revisited",
		"snooze":                  "snoozed",
		"DECISION_INTENT_SNOOZE":  "snoozed",
		"":                        "",
		"unknown":                 "",
	}
	for in, want := range cases {
		require.Equal(t, want, semanticActionLabel(in), "in=%q", in)
	}
}
