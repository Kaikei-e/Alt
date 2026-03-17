package track_home_action_usecase

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
		wantErr         bool
		wantErrContains string
	}{
		{
			name:          "success - open action",
			actionType:    "open",
			itemKey:       "article:test-uuid",
			userEventPort: &mockUserEventPort{},
			knowledgePort: &mockKnowledgeEventPort{},
		},
		{
			name:          "success - dismiss action",
			actionType:    "dismiss",
			itemKey:       "article:test-uuid",
			userEventPort: &mockUserEventPort{},
			knowledgePort: &mockKnowledgeEventPort{},
		},
		{
			name:            "error - invalid action type",
			actionType:      "invalid",
			itemKey:         "article:test-uuid",
			userEventPort:   &mockUserEventPort{},
			knowledgePort:   &mockKnowledgeEventPort{},
			wantErr:         true,
			wantErrContains: "invalid action type",
		},
		{
			name:            "error - empty item key",
			actionType:      "open",
			itemKey:         "",
			userEventPort:   &mockUserEventPort{},
			knowledgePort:   &mockKnowledgeEventPort{},
			wantErr:         true,
			wantErrContains: "item_key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewTrackHomeActionUsecase(tt.userEventPort, tt.knowledgePort)
			err := uc.Execute(context.Background(), userID, tenantID, tt.actionType, tt.itemKey, tt.metadataJSON)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrContains)
				return
			}

			require.NoError(t, err)
			assert.Len(t, tt.userEventPort.appendedEvents, 1)
			assert.Len(t, tt.knowledgePort.appendedEvents, 1)
		})
	}
}
