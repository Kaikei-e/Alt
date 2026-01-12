package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEvent(t *testing.T) {
	t.Run("creates event with valid parameters", func(t *testing.T) {
		payload := []byte(`{"article_id": "123"}`)
		metadata := map[string]string{"trace_id": "abc"}

		event, err := NewEvent(
			EventTypeArticleCreated,
			"alt-backend",
			payload,
			metadata,
		)

		require.NoError(t, err)
		assert.NotEmpty(t, event.EventID)
		assert.Equal(t, EventTypeArticleCreated, event.EventType)
		assert.Equal(t, "alt-backend", event.Source)
		assert.Equal(t, payload, event.Payload)
		assert.Equal(t, metadata, event.Metadata)
		assert.WithinDuration(t, time.Now(), event.CreatedAt, time.Second)
	})

	t.Run("generates unique event IDs", func(t *testing.T) {
		event1, _ := NewEvent(EventTypeArticleCreated, "test", nil, nil)
		event2, _ := NewEvent(EventTypeArticleCreated, "test", nil, nil)

		assert.NotEqual(t, event1.EventID, event2.EventID)
	})
}

func TestEvent_Validate(t *testing.T) {
	tests := []struct {
		name    string
		event   Event
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid event",
			event: Event{
				EventID:   "550e8400-e29b-41d4-a716-446655440000",
				EventType: EventTypeArticleCreated,
				Source:    "alt-backend",
				CreatedAt: time.Now(),
				Payload:   []byte(`{}`),
			},
			wantErr: false,
		},
		{
			name: "empty event ID",
			event: Event{
				EventID:   "",
				EventType: EventTypeArticleCreated,
				Source:    "alt-backend",
				CreatedAt: time.Now(),
			},
			wantErr: true,
			errMsg:  "event_id is required",
		},
		{
			name: "empty event type",
			event: Event{
				EventID:   "550e8400-e29b-41d4-a716-446655440000",
				EventType: "",
				Source:    "alt-backend",
				CreatedAt: time.Now(),
			},
			wantErr: true,
			errMsg:  "event_type is required",
		},
		{
			name: "empty source",
			event: Event{
				EventID:   "550e8400-e29b-41d4-a716-446655440000",
				EventType: EventTypeArticleCreated,
				Source:    "",
				CreatedAt: time.Now(),
			},
			wantErr: true,
			errMsg:  "source is required",
		},
		{
			name: "zero created_at",
			event: Event{
				EventID:   "550e8400-e29b-41d4-a716-446655440000",
				EventType: EventTypeArticleCreated,
				Source:    "alt-backend",
				CreatedAt: time.Time{},
			},
			wantErr: true,
			errMsg:  "created_at is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestEventType_Constants(t *testing.T) {
	// Verify event type constants are defined correctly
	assert.Equal(t, EventType("ArticleCreated"), EventTypeArticleCreated)
	assert.Equal(t, EventType("SummarizeRequested"), EventTypeSummarizeRequested)
	assert.Equal(t, EventType("ArticleSummarized"), EventTypeArticleSummarized)
	assert.Equal(t, EventType("TagsGenerated"), EventTypeTagsGenerated)
	assert.Equal(t, EventType("IndexArticle"), EventTypeIndexArticle)
}
