package track_home_action_usecase

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
}

func (m *mockUserEventPort) AppendKnowledgeUserEvent(_ context.Context, event domain.KnowledgeUserEvent) error {
	if m.err != nil {
		return m.err
	}
	m.appendedEvents = append(m.appendedEvents, event)
	return nil
}

// mockKnowledgeEventPort implements knowledge_event_port.AppendKnowledgeEventPort.
type mockKnowledgeEventPort struct {
	appendedEvents []domain.KnowledgeEvent
	err            error
}

func (m *mockKnowledgeEventPort) AppendKnowledgeEvent(_ context.Context, event domain.KnowledgeEvent) error {
	if m.err != nil {
		return m.err
	}
	m.appendedEvents = append(m.appendedEvents, event)
	return nil
}

// mockRecallSignalPort implements recall_signal_port.AppendRecallSignalPort.
type mockRecallSignalPort struct {
	appendedSignals []domain.RecallSignal
	err             error
}

func (m *mockRecallSignalPort) AppendRecallSignal(_ context.Context, signal domain.RecallSignal) error {
	if m.err != nil {
		return m.err
	}
	m.appendedSignals = append(m.appendedSignals, signal)
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

func TestTrackHomeActionUsecase_Execute(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	tenantID := uuid.New()

	tests := []struct {
		name            string
		actionType      string
		itemKey         string
		metadataJSON    string
		userEventPort   *mockUserEventPort
		knowledgePort   *mockKnowledgeEventPort
		flagPort        *mockFeatureFlagPort
		wantErr         bool
		wantErrContains string
		wantUserEvents  int
		wantKnEvents    int
	}{
		{
			name:           "success - open action",
			actionType:     "open",
			itemKey:        "article:test-uuid",
			userEventPort:  &mockUserEventPort{},
			knowledgePort:  &mockKnowledgeEventPort{},
			flagPort:       nil,
			wantUserEvents: 1,
			wantKnEvents:   1,
		},
		{
			name:           "success - dismiss action",
			actionType:     "dismiss",
			itemKey:        "article:test-uuid",
			userEventPort:  &mockUserEventPort{},
			knowledgePort:  &mockKnowledgeEventPort{},
			flagPort:       nil,
			wantUserEvents: 1,
			wantKnEvents:   1,
		},
		{
			name:            "error - invalid action type",
			actionType:      "invalid",
			itemKey:         "article:test-uuid",
			userEventPort:   &mockUserEventPort{},
			knowledgePort:   &mockKnowledgeEventPort{},
			flagPort:        nil,
			wantErr:         true,
			wantErrContains: "invalid action type",
		},
		{
			name:            "error - empty item key",
			actionType:      "open",
			itemKey:         "",
			userEventPort:   &mockUserEventPort{},
			knowledgePort:   &mockKnowledgeEventPort{},
			flagPort:        nil,
			wantErr:         true,
			wantErrContains: "item_key is required",
		},
		{
			name:           "tracking disabled - no events",
			actionType:     "open",
			itemKey:        "article:test-uuid",
			userEventPort:  &mockUserEventPort{},
			knowledgePort:  &mockKnowledgeEventPort{},
			flagPort:       &mockFeatureFlagPort{enabled: false},
			wantUserEvents: 0,
			wantKnEvents:   0,
		},
		{
			name:           "tracking enabled - events recorded",
			actionType:     "open",
			itemKey:        "article:test-uuid",
			userEventPort:  &mockUserEventPort{},
			knowledgePort:  &mockKnowledgeEventPort{},
			flagPort:       &mockFeatureFlagPort{enabled: true},
			wantUserEvents: 1,
			wantKnEvents:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewTrackHomeActionUsecase(tt.userEventPort, tt.knowledgePort, tt.flagPort)
			err := uc.Execute(context.Background(), userID, tenantID, tt.actionType, tt.itemKey, tt.metadataJSON)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)
				return
			}

			require.NoError(t, err)
			assert.Len(t, tt.userEventPort.appendedEvents, tt.wantUserEvents)
			assert.Len(t, tt.knowledgePort.appendedEvents, tt.wantKnEvents)
		})
	}
}

func TestTrackHomeActionUsecase_RecallSignal(t *testing.T) {
	logger.InitLogger()

	userID := uuid.New()
	tenantID := uuid.New()
	itemKey := "article:test-uuid"

	t.Run("open action appends SignalOpened", func(t *testing.T) {
		recallPort := &mockRecallSignalPort{}
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, &mockKnowledgeEventPort{}, nil)
		uc.SetRecallSignalPort(recallPort)

		err := uc.Execute(context.Background(), userID, tenantID, "open", itemKey, "")
		require.NoError(t, err)
		require.Len(t, recallPort.appendedSignals, 1)
		assert.Equal(t, domain.SignalOpened, recallPort.appendedSignals[0].SignalType)
		assert.Equal(t, itemKey, recallPort.appendedSignals[0].ItemKey)
		assert.Equal(t, userID, recallPort.appendedSignals[0].UserID)
	})

	t.Run("ask action appends SignalAugurReferenced", func(t *testing.T) {
		recallPort := &mockRecallSignalPort{}
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, &mockKnowledgeEventPort{}, nil)
		uc.SetRecallSignalPort(recallPort)

		err := uc.Execute(context.Background(), userID, tenantID, "ask", itemKey, "")
		require.NoError(t, err)
		require.Len(t, recallPort.appendedSignals, 1)
		assert.Equal(t, domain.SignalAugurReferenced, recallPort.appendedSignals[0].SignalType)
	})

	t.Run("listen action appends SignalTagInterest", func(t *testing.T) {
		recallPort := &mockRecallSignalPort{}
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, &mockKnowledgeEventPort{}, nil)
		uc.SetRecallSignalPort(recallPort)

		err := uc.Execute(context.Background(), userID, tenantID, "listen", itemKey, "")
		require.NoError(t, err)
		require.Len(t, recallPort.appendedSignals, 1)
		assert.Equal(t, domain.SignalTagInterest, recallPort.appendedSignals[0].SignalType)
	})

	t.Run("dismiss action does not append signal", func(t *testing.T) {
		recallPort := &mockRecallSignalPort{}
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, &mockKnowledgeEventPort{}, nil)
		uc.SetRecallSignalPort(recallPort)

		err := uc.Execute(context.Background(), userID, tenantID, "dismiss", itemKey, "")
		require.NoError(t, err)
		assert.Empty(t, recallPort.appendedSignals)
	})

	t.Run("recall signal failure is non-fatal", func(t *testing.T) {
		recallPort := &mockRecallSignalPort{err: errors.New("db error")}
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, &mockKnowledgeEventPort{}, nil)
		uc.SetRecallSignalPort(recallPort)

		err := uc.Execute(context.Background(), userID, tenantID, "open", itemKey, "")
		require.NoError(t, err)
	})

	t.Run("nil recall signal port does not panic", func(t *testing.T) {
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, &mockKnowledgeEventPort{}, nil)
		// No SetRecallSignalPort call

		err := uc.Execute(context.Background(), userID, tenantID, "open", itemKey, "")
		require.NoError(t, err)
	})

	t.Run("tracking disabled skips signal generation", func(t *testing.T) {
		recallPort := &mockRecallSignalPort{}
		uc := NewTrackHomeActionUsecase(&mockUserEventPort{}, &mockKnowledgeEventPort{}, &mockFeatureFlagPort{enabled: false})
		uc.SetRecallSignalPort(recallPort)

		err := uc.Execute(context.Background(), userID, tenantID, "open", itemKey, "")
		require.NoError(t, err)
		assert.Empty(t, recallPort.appendedSignals)
	})
}
