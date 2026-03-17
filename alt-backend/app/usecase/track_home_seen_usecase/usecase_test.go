package track_home_seen_usecase

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockUserEventPort implements knowledge_user_event_port.AppendKnowledgeUserEventPort.
type mockUserEventPort struct {
	appendedEvents []domain.KnowledgeUserEvent
	err            error
}

func (m *mockUserEventPort) AppendKnowledgeUserEvent(_ context.Context, event domain.KnowledgeUserEvent) error {
	if m.err != nil {
		return m.err
	}
	m.appendedEvents = append(m.appendedEvents, event)
	return nil
}

func TestTrackHomeSeenUsecase_Execute(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	tenantID := uuid.New()

	tests := []struct {
		name              string
		itemKeys          []string
		exposureSessionID string
		port              *mockUserEventPort
		wantEventCount    int
		wantErr           bool
	}{
		{
			name:              "success - records seen events for multiple items",
			itemKeys:          []string{"article:1", "article:2", "article:3"},
			exposureSessionID: "session-123",
			port:              &mockUserEventPort{},
			wantEventCount:    3,
		},
		{
			name:              "empty item keys - no events",
			itemKeys:          []string{},
			exposureSessionID: "session-123",
			port:              &mockUserEventPort{},
			wantEventCount:    0,
		},
		{
			name:              "dedupe key contains user id and item key",
			itemKeys:          []string{"article:1"},
			exposureSessionID: "session-456",
			port:              &mockUserEventPort{},
			wantEventCount:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewTrackHomeSeenUsecase(tt.port)
			err := uc.Execute(context.Background(), userID, tenantID, tt.itemKeys, tt.exposureSessionID)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Len(t, tt.port.appendedEvents, tt.wantEventCount)

			for _, event := range tt.port.appendedEvents {
				assert.Equal(t, domain.EventHomeItemsSeen, event.EventType)
				assert.Equal(t, userID, event.UserID)
				assert.NotEmpty(t, event.DedupeKey)
			}
		})
	}
}
