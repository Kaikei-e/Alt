package knowledge_home

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/domain"
)

// mockListEventsPort implements knowledge_event_port.ListKnowledgeEventsPort.
type mockListEventsPort struct {
	events []domain.KnowledgeEvent
	err    error
}

func (m *mockListEventsPort) ListKnowledgeEventsSince(_ context.Context, _ int64, _ int) ([]domain.KnowledgeEvent, error) {
	return m.events, m.err
}

func TestMapToCanonicalStreamType(t *testing.T) {
	tests := []struct {
		domainEvent   string
		canonicalType string
	}{
		{domain.EventArticleCreated, "item_added"},
		{domain.EventSummaryVersionCreated, "item_updated"},
		{domain.EventTagSetVersionCreated, "item_updated"},
		{domain.EventSummarySuperseded, "item_updated"},
		{domain.EventTagSetSuperseded, "item_updated"},
		{domain.EventHomeItemSuperseded, "item_updated"},
		{domain.EventHomeItemsSeen, "digest_changed"},
		{domain.EventHomeItemOpened, "digest_changed"},
		{domain.EventHomeItemDismissed, "digest_changed"},
		{domain.EventHomeItemAsked, "digest_changed"},
		{domain.EventHomeItemListened, "digest_changed"},
		{domain.EventRecallSnoozed, "recall_changed"},
		{domain.EventReasonMerged, "item_updated"},
		{domain.EventRecallDismissed, "recall_changed"},
		{"UnknownEventType", "digest_changed"},
	}

	for _, tt := range tests {
		t.Run(tt.domainEvent, func(t *testing.T) {
			result := mapToCanonicalStreamType(tt.domainEvent)
			assert.Equal(t, tt.canonicalType, result, "event %s should map to %s", tt.domainEvent, tt.canonicalType)
		})
	}
}

func TestStreamPayload_ItemAdded_ContainsItemKey(t *testing.T) {
	aggID := uuid.New().String()
	event := domain.KnowledgeEvent{
		EventType:     domain.EventArticleCreated,
		AggregateType: domain.AggregateArticle,
		AggregateID:   aggID,
		OccurredAt:    time.Now(),
	}

	canonicalType := mapToCanonicalStreamType(event.EventType)
	assert.Equal(t, "item_added", canonicalType)

	update := buildStreamResponse(event)
	assert.Equal(t, "item_added", update.EventType)
	require.NotNil(t, update.Item, "item_added should include Item")
	assert.Equal(t, "article:"+aggID, update.Item.ItemKey)
}

func TestStreamPayload_ItemUpdated_ContainsItemKey(t *testing.T) {
	aggID := uuid.New().String()
	event := domain.KnowledgeEvent{
		EventType:     domain.EventSummaryVersionCreated,
		AggregateType: domain.AggregateArticle,
		AggregateID:   aggID,
		OccurredAt:    time.Now(),
	}

	update := buildStreamResponse(event)
	assert.Equal(t, "item_updated", update.EventType)
	require.NotNil(t, update.Item, "item_updated should include Item")
	assert.Equal(t, "article:"+aggID, update.Item.ItemKey)
}

func TestStreamPayload_DigestChanged_NoItem(t *testing.T) {
	event := domain.KnowledgeEvent{
		EventType:     domain.EventHomeItemOpened,
		AggregateType: domain.AggregateHomeSession,
		AggregateID:   uuid.New().String(),
		OccurredAt:    time.Now(),
	}

	update := buildStreamResponse(event)
	assert.Equal(t, "digest_changed", update.EventType)
	assert.Nil(t, update.Item, "digest_changed should NOT include Item")
	assert.Nil(t, update.DigestChange, "digest_changed should NOT include DigestChange")
}

func TestStreamPayload_RecallChanged_ContainsMinimalRecallChange(t *testing.T) {
	aggID := uuid.New().String()
	event := domain.KnowledgeEvent{
		EventType:     domain.EventRecallDismissed,
		AggregateType: domain.AggregateArticle,
		AggregateID:   aggID,
		OccurredAt:    time.Now(),
	}

	update := buildStreamResponse(event)
	assert.Equal(t, "recall_changed", update.EventType)
	require.NotNil(t, update.RecallChange)
	assert.Equal(t, "article:"+aggID, update.RecallChange.ItemKey)
	assert.Nil(t, update.Item)
}

func TestCoalesceStreamEvents_EmptyInput(t *testing.T) {
	result := coalesceStreamEvents(nil)
	assert.Nil(t, result)

	result = coalesceStreamEvents([]domain.KnowledgeEvent{})
	assert.Empty(t, result)
}

func TestCoalesceStreamEvents_SingleEvent(t *testing.T) {
	events := []domain.KnowledgeEvent{
		{EventType: domain.EventArticleCreated, AggregateType: "article", AggregateID: "1", EventSeq: 1},
	}
	result := coalesceStreamEvents(events)
	assert.Len(t, result, 1)
	assert.Equal(t, "1", result[0].AggregateID)
}

func TestCoalesceStreamEvents_DeduplicatesByAggregate(t *testing.T) {
	events := []domain.KnowledgeEvent{
		{EventType: domain.EventArticleCreated, AggregateType: "article", AggregateID: "1", EventSeq: 1},
		{EventType: domain.EventSummaryVersionCreated, AggregateType: "article", AggregateID: "1", EventSeq: 2},
		{EventType: domain.EventTagSetVersionCreated, AggregateType: "article", AggregateID: "1", EventSeq: 3},
	}
	result := coalesceStreamEvents(events)
	assert.Len(t, result, 1, "same aggregate should be deduplicated to one event")
	assert.Equal(t, int64(3), result[0].EventSeq, "should keep the latest event")
}

func TestCoalesceStreamEvents_MultipleAggregates(t *testing.T) {
	events := []domain.KnowledgeEvent{
		{AggregateType: "article", AggregateID: "1", EventSeq: 1},
		{AggregateType: "article", AggregateID: "2", EventSeq: 2},
		{AggregateType: "recap", AggregateID: "3", EventSeq: 3},
	}
	result := coalesceStreamEvents(events)
	assert.Len(t, result, 3, "different aggregates should all remain")
}

func TestDropArticleAggregates_Table(t *testing.T) {
	artID := uuid.New().String()
	tests := []struct {
		name     string
		input    []domain.KnowledgeEvent
		expected int
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: 0,
		},
		{
			name:     "empty input",
			input:    []domain.KnowledgeEvent{},
			expected: 0,
		},
		{
			name: "only articles dropped",
			input: []domain.KnowledgeEvent{
				{AggregateType: domain.AggregateArticle, AggregateID: artID},
				{AggregateType: domain.AggregateArticle, AggregateID: artID},
			},
			expected: 0,
		},
		{
			name: "only non-articles retained",
			input: []domain.KnowledgeEvent{
				{AggregateType: domain.AggregateHomeSession, AggregateID: "s1"},
				{AggregateType: domain.AggregateRecap, AggregateID: "r1"},
			},
			expected: 2,
		},
		{
			name: "mixed items filtered",
			input: []domain.KnowledgeEvent{
				{AggregateType: domain.AggregateArticle, AggregateID: artID},
				{AggregateType: domain.AggregateHomeSession, AggregateID: "s1"},
				{AggregateType: domain.AggregateArticle, AggregateID: artID},
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dropArticleAggregates(tt.input)
			assert.Len(t, got, tt.expected)
			for _, item := range got {
				assert.NotEqual(t, domain.AggregateArticle, item.AggregateType)
			}
		})
	}
}

func TestApplyLensVisibility_Table(t *testing.T) {
	v1 := uuid.New()
	v2 := uuid.New()
	hidden := uuid.New()

	visibility := map[uuid.UUID]bool{
		v1: true,
		v2: true,
	}

	tests := []struct {
		name     string
		events   []domain.KnowledgeEvent
		vis      map[uuid.UUID]bool
		expected int
	}{
		{
			name:     "nil input",
			events:   nil,
			vis:      visibility,
			expected: 0,
		},
		{
			name: "visible articles and non-articles preserved",
			events: []domain.KnowledgeEvent{
				{AggregateType: domain.AggregateArticle, AggregateID: v1.String()},
				{AggregateType: domain.AggregateArticle, AggregateID: hidden.String()},
				{AggregateType: domain.AggregateHomeSession, AggregateID: "sess-1"},
				{AggregateType: domain.AggregateArticle, AggregateID: v2.String()},
			},
			vis:      visibility,
			expected: 3,
		},
		{
			name: "invalid uuid aggregate dropped",
			events: []domain.KnowledgeEvent{
				{AggregateType: domain.AggregateArticle, AggregateID: "not-a-uuid"},
				{AggregateType: domain.AggregateHomeSession, AggregateID: "sess-1"},
			},
			vis:      visibility,
			expected: 1,
		},
		{
			name: "empty visibility drops all articles",
			events: []domain.KnowledgeEvent{
				{AggregateType: domain.AggregateArticle, AggregateID: v1.String()},
				{AggregateType: domain.AggregateHomeSession, AggregateID: "sess-1"},
			},
			vis:      map[uuid.UUID]bool{},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyLensVisibility(tt.events, tt.vis)
			assert.Len(t, got, tt.expected)
		})
	}
}

func TestBuildStreamResponse_Table(t *testing.T) {
	artID := uuid.New().String()
	occurredAt := time.Date(2026, 3, 20, 15, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		event         domain.KnowledgeEvent
		wantType      string
		hasItem       bool
		hasRecall     bool
		wantItemKey   string
		wantRecallKey string
	}{
		{
			name: "article created maps to item_added",
			event: domain.KnowledgeEvent{
				EventType:     domain.EventArticleCreated,
				AggregateType: domain.AggregateArticle,
				AggregateID:   artID,
				OccurredAt:    occurredAt,
			},
			wantType:    "item_added",
			hasItem:     true,
			wantItemKey: "article:" + artID,
		},
		{
			name: "summary updated maps to item_updated",
			event: domain.KnowledgeEvent{
				EventType:     domain.EventSummaryVersionCreated,
				AggregateType: domain.AggregateArticle,
				AggregateID:   artID,
				OccurredAt:    occurredAt,
			},
			wantType:    "item_updated",
			hasItem:     true,
			wantItemKey: "article:" + artID,
		},
		{
			name: "recall dismissed maps to recall_changed",
			event: domain.KnowledgeEvent{
				EventType:     domain.EventRecallDismissed,
				AggregateType: domain.AggregateArticle,
				AggregateID:   artID,
				OccurredAt:    occurredAt,
			},
			wantType:      "recall_changed",
			hasRecall:     true,
			wantRecallKey: "article:" + artID,
		},
		{
			name: "session event maps to digest_changed",
			event: domain.KnowledgeEvent{
				EventType:     domain.EventHomeItemOpened,
				AggregateType: domain.AggregateHomeSession,
				AggregateID:   "session-1",
				OccurredAt:    occurredAt,
			},
			wantType: "digest_changed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildStreamResponse(tt.event)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantType, got.EventType)
			assert.Equal(t, tt.event.OccurredAt.Format(time.RFC3339), got.OccurredAt)
			if tt.hasItem {
				require.NotNil(t, got.Item)
				assert.Equal(t, tt.wantItemKey, got.Item.ItemKey)
			} else {
				assert.Nil(t, got.Item)
			}
			if tt.hasRecall {
				require.NotNil(t, got.RecallChange)
				assert.Equal(t, tt.wantRecallKey, got.RecallChange.ItemKey)
			} else {
				assert.Nil(t, got.RecallChange)
			}
		})
	}
}
