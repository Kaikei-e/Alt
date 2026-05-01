package knowledge_loop_usecase

import (
	"alt/domain"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func makeEvent(t *testing.T, eventType, entryKey string, seq int64, payload map[string]any) domain.KnowledgeEvent {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	userID := uuid.New()
	return domain.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      seq,
		OccurredAt:    time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC),
		TenantID:      uuid.New(),
		UserID:        &userID,
		EventType:     eventType,
		AggregateType: "article",
		AggregateID:   entryKey,
		Payload:       body,
	}
}

func TestClassifyLoopStreamUpdate_EntryAppendedForArticleEvents(t *testing.T) {
	cases := []string{
		domain.EventSummaryVersionCreated,
		domain.EventHomeItemsSeen,
		domain.EventHomeItemAsked,
	}
	for _, et := range cases {
		t.Run(et, func(t *testing.T) {
			ev := makeEvent(t, et, "article:42", 10, map[string]any{"entry_key": "article:42"})
			frame, ok := ClassifyLoopStreamUpdate(&ev)
			require.True(t, ok, "article event must produce a stream frame")
			require.Equal(t, StreamUpdateKindAppended, frame.Kind)
			require.Equal(t, "article:42", frame.EntryKey)
			require.Equal(t, int64(10), frame.Revision)
		})
	}
}

func TestClassifyLoopStreamUpdate_EntryRevisedForOpened(t *testing.T) {
	// HomeItemOpened from /feeds mutates an existing entry (it doesn't appear out of
	// nowhere — the entry already exists). Emit Revised so foreground is not disturbed.
	ev := makeEvent(t, domain.EventHomeItemOpened, "article:42", 11, map[string]any{"entry_key": "article:42"})
	frame, ok := ClassifyLoopStreamUpdate(&ev)
	require.True(t, ok)
	require.Equal(t, StreamUpdateKindRevised, frame.Kind)
	require.Equal(t, "article:42", frame.EntryKey)
}

func TestClassifyLoopStreamUpdate_SupersededCarriesNewEntryKey(t *testing.T) {
	ev := makeEvent(t, domain.EventHomeItemSuperseded, "article:42", 12, map[string]any{
		"entry_key":     "article:42",
		"new_entry_key": "article:43",
	})
	frame, ok := ClassifyLoopStreamUpdate(&ev)
	require.True(t, ok)
	require.Equal(t, StreamUpdateKindSuperseded, frame.Kind)
	require.Equal(t, "article:42", frame.EntryKey)
	require.Equal(t, "article:43", frame.NewEntryKey)
}

func TestClassifyLoopStreamUpdate_DismissedBecomesReviewAppend(t *testing.T) {
	ev := makeEvent(t, domain.EventHomeItemDismissed, "article:42", 13, map[string]any{"entry_key": "article:42"})
	frame, ok := ClassifyLoopStreamUpdate(&ev)
	require.True(t, ok)
	require.Equal(t, StreamUpdateKindAppended, frame.Kind)
	require.Equal(t, "article:42", frame.EntryKey)
}

func TestClassifyLoopStreamUpdate_LoopTransitionTriggersRebalance(t *testing.T) {
	// Loop transition events affect session state; for the stream consumer the
	// visible side effect is that foreground/bucket composition may have changed,
	// so emit SurfaceRebalanced rather than per-entry frames.
	ev := makeEvent(t, domain.EventKnowledgeLoopActed, "article:42", 14, map[string]any{
		"entry_key":    "article:42",
		"lens_mode_id": "default",
		"to_stage":     "LOOP_STAGE_ACT",
	})
	frame, ok := ClassifyLoopStreamUpdate(&ev)
	require.True(t, ok)
	require.Equal(t, StreamUpdateKindSurfaceRebalanced, frame.Kind)
	require.Equal(t, int64(14), frame.Revision)
}

func TestClassifyLoopStreamUpdate_ReviewedTriggersRebalance(t *testing.T) {
	ev := makeEvent(t, domain.EventKnowledgeLoopReviewed, "article:42", 18, map[string]any{
		"entry_key": "article:42",
		"trigger":   "TRANSITION_TRIGGER_RECHECK",
	})
	frame, ok := ClassifyLoopStreamUpdate(&ev)
	require.True(t, ok)
	require.Equal(t, StreamUpdateKindSurfaceRebalanced, frame.Kind)
	require.Equal(t, int64(18), frame.Revision)
}

func TestClassifyLoopStreamUpdate_ObservedIsSuppressed(t *testing.T) {
	// KnowledgeLoopObserved is very high-frequency (fires on tile dwell). Emitting
	// one stream frame per observed event would flood the client; the canonical
	// contract §9 says EntryRevised is for silent updates and observed alone
	// doesn't change projection-visible state. Drop it from the stream.
	ev := makeEvent(t, domain.EventKnowledgeLoopObserved, "article:42", 15, map[string]any{"entry_key": "article:42"})
	_, ok := ClassifyLoopStreamUpdate(&ev)
	require.False(t, ok, "observed events must not flood the stream")
}

func TestClassifyLoopStreamUpdate_UnknownEventNoFrame(t *testing.T) {
	ev := makeEvent(t, "UnknownEventType", "article:42", 16, nil)
	_, ok := ClassifyLoopStreamUpdate(&ev)
	require.False(t, ok)
}

func TestClassifyLoopStreamUpdate_SystemEventWithoutUserIgnored(t *testing.T) {
	ev := makeEvent(t, domain.EventSummaryVersionCreated, "article:42", 17, nil)
	ev.UserID = nil
	_, ok := ClassifyLoopStreamUpdate(&ev)
	require.False(t, ok, "events without user_id cannot be scoped to a stream subscriber")
}
