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
