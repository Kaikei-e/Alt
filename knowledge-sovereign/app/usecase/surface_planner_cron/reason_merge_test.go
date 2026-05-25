package surface_planner_cron

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

// DiffWhyCodes / BuildReasonMergedEvent (ADR-000913 §D-6) must be pure so
// reproject yields the same emission set when the same event log replays.

func TestDiffWhyCodes_NoChange_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	added, removed := DiffWhyCodes([]string{"new_unread", "tag_match"}, []string{"tag_match", "new_unread"})
	require.Empty(t, added)
	require.Empty(t, removed)
}

func TestDiffWhyCodes_AddedCodes_ReturnsAdditions(t *testing.T) {
	t.Parallel()

	added, removed := DiffWhyCodes([]string{"new_unread"}, []string{"new_unread", "topic_affinity", "tag_match"})
	require.Equal(t, []string{"tag_match", "topic_affinity"}, added,
		"additions must be sorted ascending so reproject is deterministic")
	require.Empty(t, removed)
}

func TestDiffWhyCodes_RemovedCodes_ReturnsRemovals(t *testing.T) {
	t.Parallel()

	added, removed := DiffWhyCodes([]string{"new_unread", "stale"}, []string{"new_unread"})
	require.Empty(t, added)
	require.Equal(t, []string{"stale"}, removed)
}

func TestBuildReasonMergedEvent_NoDiff_ReturnsNil(t *testing.T) {
	t.Parallel()

	anchor := makeAnchor(uuid.New())
	_, emit, err := BuildReasonMergedEvent(anchor, "article:1", "article:1", nil, nil, 100)
	require.NoError(t, err)
	require.False(t, emit, "no diff must skip the emit so reruns are no-ops")
}

func TestBuildReasonMergedEvent_OnAddition_EmitsEvent(t *testing.T) {
	t.Parallel()

	anchor := makeAnchor(uuid.New())
	ev, emit, err := BuildReasonMergedEvent(anchor, "article:1", "article:1", []string{"topic_affinity"}, nil, 100)
	require.NoError(t, err)
	require.True(t, emit)
	require.Equal(t, EventReasonMerged, ev.EventType)
	require.Equal(t, "system", ev.ActorType)
	require.Equal(t, "ReasonMerged:article:1:100", "ReasonMerged:"+ev.AggregateID+":100",
		"sanity: dedupe key format must include item_key + batch_max_seq")
	require.Contains(t, ev.DedupeKey, "article:1")
	require.Contains(t, ev.DedupeKey, "100")

	var body map[string]any
	require.NoError(t, json.Unmarshal(ev.Payload, &body))
	require.Equal(t, []any{"topic_affinity"}, body["added_why_codes"])
}

func TestBuildReasonMergedEvent_DedupeKeyStableAcrossReruns(t *testing.T) {
	t.Parallel()

	anchor := makeAnchor(uuid.New())
	ev1, _, err := BuildReasonMergedEvent(anchor, "article:1", "article:1", []string{"topic_affinity"}, nil, 100)
	require.NoError(t, err)
	ev2, _, err := BuildReasonMergedEvent(anchor, "article:1", "article:1", []string{"topic_affinity"}, nil, 100)
	require.NoError(t, err)
	require.Equal(t, ev1.DedupeKey, ev2.DedupeKey,
		"same inputs must produce the same dedupe_key so reruns coalesce at the AppendKnowledgeEvent layer")
	require.Equal(t, ev1.Payload, ev2.Payload, "payload must be byte-identical")
	require.True(t, ev1.OccurredAt.Equal(ev2.OccurredAt))
}

func makeAnchor(userID uuid.UUID) sovereign_db.KnowledgeEvent {
	uid := userID
	return sovereign_db.KnowledgeEvent{
		EventID:    uuid.New(),
		OccurredAt: time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC),
		TenantID:   uuid.New(),
		UserID:     &uid,
	}
}
