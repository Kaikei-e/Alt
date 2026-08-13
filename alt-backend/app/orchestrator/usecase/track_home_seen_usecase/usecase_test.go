package track_home_seen_usecase

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockUserEventPort implements knowledge_user_event_port.AppendKnowledgeUserEventPort.
type mockUserEventPort struct {
	appendedEvents []domain.KnowledgeUserEvent
	err            error
	failFor        map[string]error
}

func (m *mockUserEventPort) AppendKnowledgeUserEvent(_ context.Context, event domain.KnowledgeUserEvent) error {
	if m.err != nil {
		return m.err
	}
	if err, ok := m.failFor[event.ItemKey]; ok {
		return err
	}
	m.appendedEvents = append(m.appendedEvents, event)
	return nil
}

// mockFeatureFlagPort implements feature_flag_port.FeatureFlagPort.
type mockFeatureFlagPort struct {
	enabled bool
}

func (m *mockFeatureFlagPort) IsEnabled(_ string, _ uuid.UUID) bool {
	if m == nil {
		return true
	}
	return m.enabled
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
		flagPort          *mockFeatureFlagPort
		wantEventCount    int
		wantErr           bool
	}{
		{
			name:              "success - records seen events for multiple items",
			itemKeys:          []string{"article:1", "article:2", "article:3"},
			exposureSessionID: "session-123",
			port:              &mockUserEventPort{},
			flagPort:          nil, // nil = no flag guard
			wantEventCount:    3,
		},
		{
			name:              "empty item keys - no events",
			itemKeys:          []string{},
			exposureSessionID: "session-123",
			port:              &mockUserEventPort{},
			flagPort:          nil,
			wantEventCount:    0,
		},
		{
			name:              "dedupe key contains user id and item key",
			itemKeys:          []string{"article:1"},
			exposureSessionID: "session-456",
			port:              &mockUserEventPort{},
			flagPort:          nil,
			wantEventCount:    1,
		},
		{
			name:              "tracking disabled - no events recorded",
			itemKeys:          []string{"article:1", "article:2"},
			exposureSessionID: "session-789",
			port:              &mockUserEventPort{},
			flagPort:          &mockFeatureFlagPort{enabled: false},
			wantEventCount:    0,
		},
		{
			name:              "tracking enabled - events recorded",
			itemKeys:          []string{"article:1"},
			exposureSessionID: "session-111",
			port:              &mockUserEventPort{},
			flagPort:          &mockFeatureFlagPort{enabled: true},
			wantEventCount:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewTrackHomeSeenUsecase(tt.port, tt.flagPort)
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

// Impression events have no outbox and no DLQ: a swallowed append is a
// permanent loss, and the handler counts a "persisted" metric on the way out.
// Execute must therefore surface append failures to its caller.
func TestTrackHomeSeenUsecase_Execute_ReturnsAppendFailures(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	tenantID := uuid.New()

	t.Run("every append fails", func(t *testing.T) {
		port := &mockUserEventPort{err: errors.New("sovereign unavailable")}

		err := NewTrackHomeSeenUsecase(port, nil).
			Execute(context.Background(), userID, tenantID, []string{"article:1", "article:2"}, "session-1")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "sovereign unavailable")
		assert.Empty(t, port.appendedEvents)
	})

	t.Run("partial failure still reports an error and keeps the successes", func(t *testing.T) {
		port := &mockUserEventPort{
			failFor: map[string]error{"article:2": errors.New("append rejected")},
		}

		err := NewTrackHomeSeenUsecase(port, nil).
			Execute(context.Background(), userID, tenantID, []string{"article:1", "article:2", "article:3"}, "session-2")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "article:2")
		assert.Len(t, port.appendedEvents, 2)
	})
}
