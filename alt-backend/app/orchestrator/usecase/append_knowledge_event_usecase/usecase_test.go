package append_knowledge_event_usecase

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockEventPort struct {
	appendedEvents []domain.KnowledgeEvent
	err            error
}

func (m *mockEventPort) AppendKnowledgeEvent(_ context.Context, event domain.KnowledgeEvent) (int64, error) {
	if m.err != nil {
		return 0, m.err
	}
	m.appendedEvents = append(m.appendedEvents, event)
	return int64(len(m.appendedEvents)), nil
}

func TestAppendKnowledgeEventUsecase_Execute(t *testing.T) {
	logger.InitLogger()

	tenantID := uuid.New()

	tests := []struct {
		name    string
		event   domain.KnowledgeEvent
		port    *mockEventPort
		wantErr bool
	}{
		{
			name: "success - full event",
			event: domain.KnowledgeEvent{
				EventID:       uuid.New(),
				OccurredAt:    time.Now(),
				TenantID:      tenantID,
				ActorType:     domain.ActorSystem,
				EventType:     domain.EventArticleCreated,
				AggregateType: domain.AggregateArticle,
				AggregateID:   uuid.New().String(),
				DedupeKey:     "test-dedupe",
				Payload:       []byte(`{}`),
			},
			port: &mockEventPort{},
		},
		{
			name: "success - generates missing fields",
			event: domain.KnowledgeEvent{
				TenantID:      tenantID,
				ActorType:     domain.ActorSystem,
				EventType:     domain.EventArticleCreated,
				AggregateType: domain.AggregateArticle,
				AggregateID:   "article-123",
				Payload:       []byte(`{}`),
			},
			port: &mockEventPort{},
		},
		{
			name: "error - missing event_type",
			event: domain.KnowledgeEvent{
				AggregateType: domain.AggregateArticle,
				AggregateID:   "article-123",
			},
			port:    &mockEventPort{},
			wantErr: true,
		},
		{
			name: "error - missing aggregate_type",
			event: domain.KnowledgeEvent{
				EventType:   domain.EventArticleCreated,
				AggregateID: "article-123",
			},
			port:    &mockEventPort{},
			wantErr: true,
		},
		{
			name: "error - missing aggregate_id",
			event: domain.KnowledgeEvent{
				EventType:     domain.EventArticleCreated,
				AggregateType: domain.AggregateArticle,
			},
			port:    &mockEventPort{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewAppendKnowledgeEventUsecase(tt.port)
			err := uc.Execute(context.Background(), tt.event)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Len(t, tt.port.appendedEvents, 1)
			appended := tt.port.appendedEvents[0]
			assert.NotEqual(t, uuid.Nil, appended.EventID)
			assert.False(t, appended.OccurredAt.IsZero())
			assert.NotEmpty(t, appended.DedupeKey)
		})
	}
}

func TestAppendKnowledgeEventUsecase_WrapsPortError(t *testing.T) {
	logger.InitLogger()
	portErr := assert.AnError
	uc := NewAppendKnowledgeEventUsecase(&mockEventPort{err: portErr})
	err := uc.Execute(context.Background(), domain.KnowledgeEvent{
		TenantID:      uuid.New(),
		ActorType:     domain.ActorSystem,
		EventType:     domain.EventArticleCreated,
		AggregateType: domain.AggregateArticle,
		AggregateID:   "article-123",
		Payload:       []byte(`{}`),
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, portErr)
	assert.Contains(t, err.Error(), "append knowledge event")
}

func TestNormalizeEvent(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		event   domain.KnowledgeEvent
		now     time.Time
		wantErr bool
		verify  func(t *testing.T, evt domain.KnowledgeEvent)
	}{
		{
			name: "missing event_type",
			event: domain.KnowledgeEvent{
				AggregateType: "agg",
				AggregateID:   "1",
			},
			now:     now,
			wantErr: true,
		},
		{
			name: "missing aggregate_type",
			event: domain.KnowledgeEvent{
				EventType:   "evt",
				AggregateID: "1",
			},
			now:     now,
			wantErr: true,
		},
		{
			name: "missing aggregate_id",
			event: domain.KnowledgeEvent{
				EventType:     "evt",
				AggregateType: "agg",
			},
			now:     now,
			wantErr: true,
		},
		{
			name: "defaults ID, timestamp, dedupe key",
			event: domain.KnowledgeEvent{
				EventType:     "custom.event",
				AggregateType: "item",
				AggregateID:   "item-123",
			},
			now:     now,
			wantErr: false,
			verify: func(t *testing.T, evt domain.KnowledgeEvent) {
				assert.NotEqual(t, uuid.Nil, evt.EventID)
				assert.Equal(t, now, evt.OccurredAt)
				assert.Equal(t, "custom.event:item-123:"+evt.EventID.String(), evt.DedupeKey)
			},
		},
		{
			name: "preserves explicitly set fields",
			event: domain.KnowledgeEvent{
				EventID:       uuid.MustParse("00000000-0000-0000-0000-000000000099"),
				OccurredAt:    now.Add(-time.Hour),
				EventType:     "custom.event",
				AggregateType: "item",
				AggregateID:   "item-123",
				DedupeKey:     "custom-dedupe",
			},
			now:     now,
			wantErr: false,
			verify: func(t *testing.T, evt domain.KnowledgeEvent) {
				assert.Equal(t, uuid.MustParse("00000000-0000-0000-0000-000000000099"), evt.EventID)
				assert.Equal(t, now.Add(-time.Hour), evt.OccurredAt)
				assert.Equal(t, "custom-dedupe", evt.DedupeKey)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeEvent(tt.event, tt.now)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.verify != nil {
					tt.verify(t, got)
				}
			}
		})
	}
}
